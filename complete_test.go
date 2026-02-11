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
func (c *compRootCmd) Subcommands() []cli.Runner {
	return []cli.Runner{&compServeCmd{}, &compDeployCmd{}, &compHiddenCmd{}}
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
func (c *compNestedRoot) Subcommands() []cli.Runner {
	return []cli.Runner{&compClusterCmd{}}
}

type compClusterCmd struct{}

func (c *compClusterCmd) Run(_ context.Context) error { return nil }
func (c *compClusterCmd) Name() string                { return "cluster" }
func (c *compClusterCmd) Description() string         { return "Manage clusters" }
func (c *compClusterCmd) Subcommands() []cli.Runner {
	return []cli.Runner{&compClusterListCmd{}}
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
func (c *compCompleterCmd) Complete(_ context.Context, _ []string) ([]string, cli.ShellCompDirective) {
	return []string{"alpha", "beta", "gamma"}, cli.ShellCompDirectiveNoFileComp
}

type compCompleterNilCmd struct{}

func (c *compCompleterNilCmd) Run(_ context.Context) error { return nil }
func (c *compCompleterNilCmd) Name() string                { return "myapp" }
func (c *compCompleterNilCmd) Complete(_ context.Context, _ []string) ([]string, cli.ShellCompDirective) {
	return nil, cli.ShellCompDirectiveDefault
}

func (c *compCompleterNilCmd) Subcommands() []cli.Runner {
	return []cli.Runner{&compServeCmd{}}
}

// completionNames extracts flag/command names from completion candidates.
func completionNames(completions []string) []string {
	names := make([]string, 0, len(completions))
	for _, c := range completions {
		name, _, _ := strings.Cut(c, "\t")
		names = append(names, name)
	}
	return names
}

// --- tests ---

func TestComputeCompletions_RootSubcommands(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)

	names := completionNames(completions)
	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "deploy")
	assert.NotContains(t, names, "internal") // hidden
}

func TestComputeCompletions_SubcommandFlags(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)

	names := completionNames(completions)
	assert.Contains(t, names, "--port")
	assert.Contains(t, names, "--tls")
	assert.Contains(t, names, "--format")
}

func TestComputeCompletions_NestedSubcommand(t *testing.T) {
	t.Parallel()
	root := &compNestedRoot{}

	// "--" prefix shows long flags.
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"cluster", "list", "--"})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completionNames(completions), "--region")

	// "-" prefix shows both long and short flags.
	completions2, _ := cli.ComputeCompletions(context.Background(), root, []string{"cluster", "list", "-"})
	names2 := completionNames(completions2)
	assert.Contains(t, names2, "--region")
	assert.Contains(t, names2, "-r")
}

func TestComputeCompletions_Aliases(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)

	names := completionNames(completions)
	assert.Contains(t, names, "d")
	assert.Contains(t, names, "dep")
}

func TestComputeCompletions_NegateFlags(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, _ := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--"})

	assert.Contains(t, completionNames(completions), "--no-color")
}

func TestComputeCompletions_EnumValues(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--format", ""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "json")
	assert.Contains(t, completions, "yaml")
	assert.Contains(t, completions, "text")
}

func TestComputeCompletions_DeprecatedExcluded(t *testing.T) {
	t.Parallel()
	cmd := &compDeprecatedFlagCmd{}
	completions, _ := cli.ComputeCompletions(context.Background(), cmd, []string{"--"})

	names := completionNames(completions)
	assert.NotContains(t, names, "--old")
	assert.Contains(t, names, "--new")
}

func TestComputeCompletions_HiddenExcluded(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, _ := cli.ComputeCompletions(context.Background(), root, []string{""})

	assert.NotContains(t, completionNames(completions), "internal")
}

func TestComputeCompletions_CompleterInterface(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, completions)
}

func TestComputeCompletions_CompleterNilFallback(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterNilCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completionNames(completions), "serve")
}

// Completer returning NoSpace directive.
type compCompleterNoSpaceCmd struct{}

func (c *compCompleterNoSpaceCmd) Run(_ context.Context) error { return nil }
func (c *compCompleterNoSpaceCmd) Name() string                { return "myapp" }
func (c *compCompleterNoSpaceCmd) Complete(_ context.Context, _ []string) ([]string, cli.ShellCompDirective) {
	return []string{"alpha", "beta"}, cli.ShellCompDirectiveNoSpace
}

func TestComputeCompletions_CompleterDirective(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterNoSpaceCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveNoSpace, directive)
	assert.Equal(t, []string{"alpha", "beta"}, completions)
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
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"se"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	names := completionNames(completions)
	assert.Contains(t, names, "serve")
	assert.NotContains(t, names, "deploy")
}

func TestComputeCompletions_FlagSkipDuringWalk(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	// Flags in contextArgs should be skipped; "serve" should still be resolved.
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"--verbose", "serve", "--"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	names := completionNames(completions)
	assert.Contains(t, names, "--port")
	assert.Contains(t, names, "--tls")
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
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"--fmt", ""})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "json")
	assert.Contains(t, completions, "yaml")
	assert.Contains(t, completions, "text")
}

func TestComputeCompletions_EmptyArgs(t *testing.T) {
	t.Parallel()
	root := &compRootCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), root, nil)

	// nil args should return root subcommands.
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	names := completionNames(completions)
	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "deploy")
}

func TestComputeCompletions_CompleterWithPrefix(t *testing.T) {
	t.Parallel()
	cmd := &compCompleterCmd{}
	// "al" is the prefix to complete; Completer returns alpha/beta/gamma.
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"al"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"alpha"}, completions)
}

// --- FlagCompleter tests ---

type compFlagCompleterCmd struct {
	Region string `flag:"region" short:"r" help:"AWS region"`
	Format string `flag:"format" enum:"json,yaml" help:"Output format"`
	Port   int    `flag:"port" help:"Port number"`
}

func (c *compFlagCompleterCmd) Run(_ context.Context) error { return nil }
func (c *compFlagCompleterCmd) Name() string                { return "myapp" }
func (c *compFlagCompleterCmd) CompleteFlag(_ context.Context, flag, _ string) ([]string, cli.ShellCompDirective) {
	if flag == "region" {
		return []string{"us-east-1", "us-west-2", "eu-west-1"}, cli.ShellCompDirectiveNoFileComp
	}
	return nil, cli.ShellCompDirectiveDefault
}

func TestComputeCompletions_FlagCompleter(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// FlagCompleter provides completions for --region.
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"--region", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"us-east-1", "us-west-2", "eu-west-1"}, completions)
}

func TestComputeCompletions_FlagCompleterPrefix(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// Prefix "us-e" filters to "us-east-1".
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"--region", "us-e"})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"us-east-1"}, completions)
}

func TestComputeCompletions_FlagCompleterNilFallsToEnum(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// FlagCompleter returns nil for --format; falls through to enum.
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"--format", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Contains(t, completions, "json")
	assert.Contains(t, completions, "yaml")
}

func TestComputeCompletions_FlagCompleterShortFlag(t *testing.T) {
	t.Parallel()
	cmd := &compFlagCompleterCmd{}

	// Short flag -r triggers FlagCompleter for "region".
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{"-r", ""})
	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"us-east-1", "us-west-2", "eu-west-1"}, completions)
}

// --- FilterFileExt / FilterDirs directive tests ---

type compFilterFileExtCmd struct{}

func (c *compFilterFileExtCmd) Run(_ context.Context) error { return nil }
func (c *compFilterFileExtCmd) Name() string                { return "myapp" }
func (c *compFilterFileExtCmd) Complete(_ context.Context, _ []string) ([]string, cli.ShellCompDirective) {
	return []string{".yaml", ".json"}, cli.ShellCompDirectiveFilterFileExt
}

func TestComputeCompletions_FilterFileExt(t *testing.T) {
	t.Parallel()
	cmd := &compFilterFileExtCmd{}
	completions, directive := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveFilterFileExt, directive)
	assert.Equal(t, []string{".yaml", ".json"}, completions)
}

type compFilterDirsCmd struct{}

func (c *compFilterDirsCmd) Run(_ context.Context) error { return nil }
func (c *compFilterDirsCmd) Name() string                { return "myapp" }
func (c *compFilterDirsCmd) Complete(_ context.Context, _ []string) ([]string, cli.ShellCompDirective) {
	return []string{"dir-placeholder"}, cli.ShellCompDirectiveFilterDirs
}

func TestComputeCompletions_FilterDirs(t *testing.T) {
	t.Parallel()
	cmd := &compFilterDirsCmd{}
	_, directive := cli.ComputeCompletions(context.Background(), cmd, []string{""})

	assert.Equal(t, cli.ShellCompDirectiveFilterDirs, directive)
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
	completions, directive := cli.ComputeCompletions(context.Background(), root, []string{"serve", "--format", "js"})

	assert.Equal(t, cli.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"json"}, completions)
}
