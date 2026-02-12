package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test fixtures ---

type compRootCmd struct{}

func (c *compRootCmd) Run(_ context.Context) error { return nil }
func (c *compRootCmd) Name() string                { return "myapp" }
func (c *compRootCmd) Subcommands() []cli.Commander {
	return []cli.Commander{&compServeCmd{}, &compDeployCmd{}, &compHiddenCmd{}}
}

type compServeCmd struct {
	Port   int    `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	TLS    bool   `flag:"tls" help:"Enable TLS"`
	Format string `flag:"format" enum:"json,yaml,text" help:"Output format"`
	Color  bool   `flag:"color" negate:"true" default:"true" help:"Colorize output"`
	Output string `flag:"output" alt:"out" help:"Output file"`
}

func (c *compServeCmd) Run(_ context.Context) error { return nil }
func (c *compServeCmd) Name() string                { return "serve" }
func (c *compServeCmd) Description() string         { return "Start the server" }

type compDeployCmd struct{}

func (c *compDeployCmd) Run(_ context.Context) error { return nil }
func (c *compDeployCmd) Name() string                { return "deploy" }
func (c *compDeployCmd) Description() string         { return "Deploy the app" }
func (c *compDeployCmd) Aliases() []string           { return []string{"d", "dep"} }

type compHiddenCmd struct{}

func (c *compHiddenCmd) Run(_ context.Context) error { return nil }
func (c *compHiddenCmd) Name() string                { return "internal" }
func (c *compHiddenCmd) Hidden() bool                { return true }

type compDeprecatedFlagCmd struct {
	Old string `flag:"old" deprecated:"use --new" help:"Old flag"`
	New string `flag:"new" help:"New flag"`
}

func (c *compDeprecatedFlagCmd) Run(_ context.Context) error { return nil }
func (c *compDeprecatedFlagCmd) Name() string                { return "myapp" }

// Nested command tree for deep resolution.
type compNestedRoot struct{}

func (c *compNestedRoot) Run(_ context.Context) error { return nil }
func (c *compNestedRoot) Name() string                { return "myapp" }
func (c *compNestedRoot) Subcommands() []cli.Commander {
	return []cli.Commander{&compClusterCmd{}}
}

type compClusterCmd struct{}

func (c *compClusterCmd) Run(_ context.Context) error { return nil }
func (c *compClusterCmd) Name() string                { return "cluster" }
func (c *compClusterCmd) Description() string         { return "Manage clusters" }
func (c *compClusterCmd) Subcommands() []cli.Commander {
	return []cli.Commander{&compClusterListCmd{}}
}

type compClusterListCmd struct {
	Region string `flag:"region" short:"r" help:"AWS region"`
}

func (c *compClusterListCmd) Run(_ context.Context) error { return nil }
func (c *compClusterListCmd) Name() string                { return "list" }
func (c *compClusterListCmd) Description() string         { return "List clusters" }

// Completer implementation.
type compCompleterCmd struct{}

func (c *compCompleterCmd) Run(_ context.Context) error { return nil }
func (c *compCompleterCmd) Name() string                { return "myapp" }
func (c *compCompleterCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.Completions("alpha", "beta", "gamma")
}

type compCompleterNilCmd struct{}

func (c *compCompleterNilCmd) Run(_ context.Context) error { return nil }
func (c *compCompleterNilCmd) Name() string                { return "myapp" }
func (c *compCompleterNilCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.NoCompletions()
}

func (c *compCompleterNilCmd) Subcommands() []cli.Commander {
	return []cli.Commander{&compServeCmd{}}
}

// completionValues extracts values from completion candidates.
func completionValues(completions []cli.Completion) []string {
	values := make([]string, 0, len(completions))
	for _, c := range completions {
		values = append(values, c.Value)
	}
	return values
}

// --- tests ---

func TestComputeCompletions_RootSubcommands(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)

	values := completionValues(result.Completions)
	assert.Contains(t, values, "serve")
	assert.Contains(t, values, "deploy")
	assert.NotContains(t, values, "internal") // hidden
}

func TestComputeCompletions_SubcommandFlags(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)

	values := completionValues(result.Completions)
	assert.Contains(t, values, "--port")
	assert.Contains(t, values, "--tls")
	assert.Contains(t, values, "--format")
}

func TestComputeCompletions_NestedSubcommand(t *testing.T) {
	t.Parallel()
	root := &compNestedRoot{}

	// "--" prefix shows long flags.
	result := cli.ComputeCompletions(context.Background(), root, []string{"cluster", "list", "--"})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Contains(t, completionValues(result.Completions), "--region")

	// "-" prefix shows both long and short flags.
	result2 := cli.ComputeCompletions(context.Background(), root, []string{"cluster", "list", "-"})
	values2 := completionValues(result2.Completions)
	assert.Contains(t, values2, "--region")
	assert.Contains(t, values2, "-r")
}

func TestComputeCompletions_Aliases(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)

	values := completionValues(result.Completions)
	assert.Contains(t, values, "d")
	assert.Contains(t, values, "dep")
}

func TestComputeCompletions_NegateFlags(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--"})

	assert.Contains(t, completionValues(result.Completions), "--no-color")
}

func TestComputeCompletions_EnumValues(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--format", ""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "json")
	assert.Contains(t, values, "yaml")
	assert.Contains(t, values, "text")
}

func TestComputeCompletions_DeprecatedExcluded(t *testing.T) {
	t.Parallel()
	cmd := &compDeprecatedFlagCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"--"})

	values := completionValues(result.Completions)
	assert.NotContains(t, values, "--old")
	assert.Contains(t, values, "--new")
}

func TestComputeCompletions_HiddenExcluded(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.NotContains(t, completionValues(result.Completions), "internal")
}

func TestComputeCompletions_CompleterInterface(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, completionValues(result.Completions))
}

func TestComputeCompletions_CompleterNilFallback(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterNilCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Contains(t, completionValues(result.Completions), "serve")
}

// Completer returning NoSpace directive.
type compCompleterNoSpaceCmd struct{}

func (c *compCompleterNoSpaceCmd) Run(_ context.Context) error { return nil }
func (c *compCompleterNoSpaceCmd) Name() string                { return "myapp" }
func (c *compCompleterNoSpaceCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.Completions("alpha", "beta").WithDirective(cli.ShellCompDirectiveNoSpace)
}

func TestComputeCompletions_CompleterDirective(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterNoSpaceCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoSpace, result.Directive)
	assert.Equal(t, []string{"alpha", "beta"}, completionValues(result.Completions))
}

func TestRuntimeComplete_OutputFormat(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	var buf bytes.Buffer
	cli.RuntimeComplete(context.Background(), root, []string{""}, &buf)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)

	// Last line should be a directive like ":4".
	lastLine := lines[len(lines)-1]
	assert.Regexp(t, `^:\d+$`, lastLine)
}

func TestExecute_CompleteIntercept(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	var buf bytes.Buffer

	err := cli.Execute(context.Background(), root, []string{"__complete", ""}, cli.WithStdout(&buf))
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "serve")
	assert.Contains(t, output, "deploy")
	assert.NotContains(t, output, "internal")
}

func TestComputeCompletions_PrefixFilter(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, []string{"se"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "serve")
	assert.NotContains(t, values, "deploy")
}

func TestComputeCompletions_FlagSkipDuringWalk(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	// Flags in contextArgs should be skipped; "serve" should still be resolved.
	result := cli.ComputeCompletions(context.Background(), root, []string{"--verbose", "serve", "--"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "--port")
	assert.Contains(t, values, "--tls")
}

type compAltEnumCmd struct {
	Format string `flag:"format" alt:"fmt" enum:"json,yaml,text" help:"Output format"`
}

func (c *compAltEnumCmd) Run(_ context.Context) error { return nil }
func (c *compAltEnumCmd) Name() string                { return "myapp" }

func TestComputeCompletions_AltFlagEnum(t *testing.T) {
	t.Parallel()
	cmd := &compAltEnumCmd{}
	// --fmt is the alt name for --format; should complete enum values.
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"--fmt", ""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "json")
	assert.Contains(t, values, "yaml")
	assert.Contains(t, values, "text")
}

func TestComputeCompletions_EmptyArgs(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	result := cli.ComputeCompletions(context.Background(), root, nil)

	// nil args should return root subcommands.
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "serve")
	assert.Contains(t, values, "deploy")
}

func TestComputeCompletions_CompleterWithPrefix(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterCmd{}
	// "al" is the prefix to complete; Completer returns alpha/beta/gamma.
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"al"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"alpha"}, completionValues(result.Completions))
}

// --- FlagCompleter tests ---

type compFlagCompleterCmd struct {
	Region string `flag:"region" short:"r" help:"AWS region"`
	Format string `flag:"format" enum:"json,yaml" help:"Output format"`
	Port   int    `flag:"port" help:"Port number"`
}

func (c *compFlagCompleterCmd) Run(_ context.Context) error { return nil }
func (c *compFlagCompleterCmd) Name() string                { return "myapp" }
func (c *compFlagCompleterCmd) CompleteFlag(_ context.Context, flag, _ string) cli.CompletionResult {
	if flag == "region" {
		return cli.Completions("us-east-1", "us-west-2", "eu-west-1")
	}
	return cli.NoCompletions()
}

func TestComputeCompletions_FlagCompleter(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// FlagCompleter provides completions for --region.
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"--region", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"us-east-1", "us-west-2", "eu-west-1"}, completionValues(result.Completions))
}

func TestComputeCompletions_FlagCompleterPrefix(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// Prefix "us-e" filters to "us-east-1".
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"--region", "us-e"})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"us-east-1"}, completionValues(result.Completions))
}

func TestComputeCompletions_FlagCompleterNilFallsToEnum(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// FlagCompleter returns empty for --format; falls through to enum.
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"--format", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	values := completionValues(result.Completions)
	assert.Contains(t, values, "json")
	assert.Contains(t, values, "yaml")
}

func TestComputeCompletions_FlagCompleterShortFlag(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// Short flag -r triggers FlagCompleter for "region".
	result := cli.ComputeCompletions(context.Background(), cmd, []string{"-r", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"us-east-1", "us-west-2", "eu-west-1"}, completionValues(result.Completions))
}

// --- FilterFileExt / FilterDirs directive tests ---

type compFilterFileExtCmd struct{}

func (c *compFilterFileExtCmd) Run(_ context.Context) error { return nil }
func (c *compFilterFileExtCmd) Name() string                { return "myapp" }
func (c *compFilterFileExtCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.Completions(".yaml", ".json").WithDirective(cli.ShellCompDirectiveFilterFileExt)
}

func TestComputeCompletions_FilterFileExt(t *testing.T) {
	t.Parallel()
	cmd := &compFilterFileExtCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveFilterFileExt, result.Directive)
	assert.Equal(t, []string{".yaml", ".json"}, completionValues(result.Completions))
}

type compFilterDirsCmd struct{}

func (c *compFilterDirsCmd) Run(_ context.Context) error { return nil }
func (c *compFilterDirsCmd) Name() string                { return "myapp" }
func (c *compFilterDirsCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.Completions("dir-placeholder").WithDirective(cli.ShellCompDirectiveFilterDirs)
}

func TestComputeCompletions_FilterDirs(t *testing.T) {
	t.Parallel()
	cmd := &compFilterDirsCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveFilterDirs, result.Directive)
}

func TestShellCompDirective_Values(t *testing.T) {
	t.Parallel()

	// Verify the bitfield values are correct.
	assert.Equal(t, cli.ShellCompDirective(0), cli.ShellCompDirectiveDefault)
	assert.Equal(t, cli.ShellCompDirective(2), cli.ShellCompDirectiveNoSpace)
	assert.Equal(t, cli.ShellCompDirective(4), cli.ShellCompDirectiveNoFileComp)
	assert.Equal(t, cli.ShellCompDirective(8), cli.ShellCompDirectiveError)
	assert.Equal(t, cli.ShellCompDirective(16), cli.ShellCompDirectiveFilterFileExt)
	assert.Equal(t, cli.ShellCompDirective(32), cli.ShellCompDirectiveFilterDirs)
}

func TestComputeCompletions_EnumValuePrefix(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	// After --format, complete "js" which should match "json".
	result := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--format", "js"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"json"}, completionValues(result.Completions))
}

// --- Active Help tests ---

type compActiveHelpCmd struct{}

func (c *compActiveHelpCmd) Run(_ context.Context) error { return nil }
func (c *compActiveHelpCmd) Name() string                { return "myapp" }
func (c *compActiveHelpCmd) Complete(_ context.Context, _ []string) cli.CompletionResult {
	return cli.Completions("dev", "staging", "prod").
		WithActiveHelp("Select deployment environment", "Use 'prod' with caution")
}

func TestComputeCompletions_ActiveHelp(t *testing.T) {
	t.Parallel()
	cmd := &compActiveHelpCmd{}
	result := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
	assert.Equal(t, []string{"dev", "staging", "prod"}, completionValues(result.Completions))
	assert.Equal(t, []string{"Select deployment environment", "Use 'prod' with caution"}, result.ActiveHelp)
}

func TestRuntimeComplete_ActiveHelpOutput(t *testing.T) {
	t.Parallel()
	cmd := &compActiveHelpCmd{}
	var buf bytes.Buffer
	cli.RuntimeComplete(context.Background(), cmd, []string{""}, &buf)

	output := buf.String()
	// Active help messages should be prefixed with "_activeHelp_ ".
	assert.Contains(t, output, "_activeHelp_ Select deployment environment")
	assert.Contains(t, output, "_activeHelp_ Use 'prod' with caution")
	// Regular completions should still be present.
	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "staging")
	assert.Contains(t, output, "prod")
}

func TestCompletionsWithDesc(t *testing.T) {
	t.Parallel()
	result := cli.CompletionsWithDesc(
		cli.Completion{Value: "us-east-1", Description: "N. Virginia"},
		cli.Completion{Value: "us-west-2", Description: "Oregon"},
	)

	assert.Len(t, result.Completions, 2)
	assert.Equal(t, "us-east-1", result.Completions[0].Value)
	assert.Equal(t, "N. Virginia", result.Completions[0].Description)
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, result.Directive)
}

func TestRuntimeComplete_DescriptionOutput(t *testing.T) {
	t.Parallel()
	// Test that descriptions are formatted with tab separator.
	root := &compRootCmd{}
	var buf bytes.Buffer
	cli.RuntimeComplete(context.Background(), root, []string{""}, &buf)

	output := buf.String()
	// Subcommand completions include descriptions.
	assert.Contains(t, output, "serve\tStart the server")
	assert.Contains(t, output, "deploy\tDeploy the app")
}
