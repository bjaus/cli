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

func (c *compRootCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compRootCmd) Name() string                            { return "myapp" }
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

func (c *compServeCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compServeCmd) Name() string                            { return "serve" }
func (c *compServeCmd) Description() string                     { return "Start the server" }

type compDeployCmd struct{}

func (c *compDeployCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compDeployCmd) Name() string                            { return "deploy" }
func (c *compDeployCmd) Description() string                     { return "Deploy the app" }
func (c *compDeployCmd) Aliases() []string                       { return []string{"d", "dep"} }

type compHiddenCmd struct{}

func (c *compHiddenCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compHiddenCmd) Name() string                            { return "internal" }
func (c *compHiddenCmd) Hidden() bool                            { return true }

type compDeprecatedFlagCmd struct {
	Old string `flag:"old" deprecated:"use --new" help:"Old flag"`
	New string `flag:"new" help:"New flag"`
}

func (c *compDeprecatedFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compDeprecatedFlagCmd) Name() string                            { return "myapp" }

// Nested command tree for deep resolution.
type compNestedRoot struct{}

func (c *compNestedRoot) Run(_ context.Context, _ []string) error { return nil }
func (c *compNestedRoot) Name() string                            { return "myapp" }
func (c *compNestedRoot) Subcommands() []cli.Runner {
	return []cli.Runner{&compClusterCmd{}}
}

type compClusterCmd struct{}

func (c *compClusterCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compClusterCmd) Name() string                            { return "cluster" }
func (c *compClusterCmd) Description() string                     { return "Manage clusters" }
func (c *compClusterCmd) Subcommands() []cli.Runner {
	return []cli.Runner{&compClusterListCmd{}}
}

type compClusterListCmd struct {
	Region string `flag:"region" short:"r" help:"AWS region"`
}

func (c *compClusterListCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compClusterListCmd) Name() string                            { return "list" }
func (c *compClusterListCmd) Description() string                     { return "List clusters" }

// Completer implementation.
type compCompleterCmd struct{}

func (c *compCompleterCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compCompleterCmd) Name() string                            { return "myapp" }
func (c *compCompleterCmd) Complete(_ context.Context, _ []string) []string {
	return []string{"alpha", "beta", "gamma"}
}

type compCompleterNilCmd struct{}

func (c *compCompleterNilCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *compCompleterNilCmd) Name() string                            { return "myapp" }
func (c *compCompleterNilCmd) Complete(_ context.Context, _ []string) []string {
	return nil
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
