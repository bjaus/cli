package completion_test

import (
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/completion"
	"github.com/stretchr/testify/assert"
)

type rootCmd struct {
	serve  *serveCmd
	hidden *hiddenCmd
}

func (r *rootCmd) Run(_ context.Context, _ []string) error { return nil }
func (r *rootCmd) Name() string                            { return "myapp" }
func (r *rootCmd) Description() string                     { return "My test app" }
func (r *rootCmd) Subcommands() []cli.Runner               { return []cli.Runner{r.serve, r.hidden} }

type serveCmd struct {
	Port int  `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	TLS  bool `flag:"tls" help:"Enable TLS"`
}

func (s *serveCmd) Run(_ context.Context, _ []string) error { return nil }
func (s *serveCmd) Name() string                            { return "serve" }
func (s *serveCmd) Description() string                     { return "Start the server" }

type hiddenCmd struct{}

func (h *hiddenCmd) Run(_ context.Context, _ []string) error { return nil }
func (h *hiddenCmd) Name() string                            { return "internal" }
func (h *hiddenCmd) Hidden() bool                            { return true }

func newRoot() *rootCmd {
	return &rootCmd{serve: &serveCmd{}, hidden: &hiddenCmd{}}
}

// --- aliased command ---

type aliasedCmd struct{}

func (a *aliasedCmd) Run(_ context.Context, _ []string) error { return nil }
func (a *aliasedCmd) Name() string                            { return "deploy" }
func (a *aliasedCmd) Description() string                     { return "Deploy the app" }
func (a *aliasedCmd) Aliases() []string                       { return []string{"d", "dep"} }

type aliasRoot struct{}

func (r *aliasRoot) Run(_ context.Context, _ []string) error { return nil }
func (r *aliasRoot) Name() string                            { return "myapp" }
func (r *aliasRoot) Subcommands() []cli.Runner               { return []cli.Runner{&aliasedCmd{}} }

// --- negate flag command ---

type negateFlagCmd struct {
	Color bool `flag:"color" negate:"true" default:"true" help:"Colorize output"`
}

func (n *negateFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (n *negateFlagCmd) Name() string                            { return "myapp" }

// --- deprecated flag command ---

type deprecatedFlagCmd struct {
	Old string `flag:"old" deprecated:"use --new" help:"Old flag"`
	New string `flag:"new" help:"New flag"`
}

func (d *deprecatedFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (d *deprecatedFlagCmd) Name() string                            { return "myapp" }

// --- enum flag command ---

type enumFlagCmd struct {
	Format string `flag:"format" enum:"json,yaml,text" help:"Output format"`
}

func (e *enumFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (e *enumFlagCmd) Name() string                            { return "myapp" }

// --- alt flag command ---

type altFlagCmd struct {
	Output string `flag:"output" short:"o" alt:"out" help:"Output file"`
}

func (a *altFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (a *altFlagCmd) Name() string                            { return "myapp" }

// --- discoverer root ---

type discoveredSub struct{}

func (d *discoveredSub) Run(_ context.Context, _ []string) error { return nil }
func (d *discoveredSub) Name() string                            { return "plugin" }
func (d *discoveredSub) Description() string                     { return "A discovered plugin" }

type discovererRoot struct{}

func (d *discovererRoot) Run(_ context.Context, _ []string) error { return nil }
func (d *discovererRoot) Name() string                            { return "myapp" }
func (d *discovererRoot) Discover() ([]cli.Runner, error) {
	return []cli.Runner{&discoveredSub{}}, nil
}

// --- bool/value flag command for type hints ---

type typeHintCmd struct {
	Verbose bool   `flag:"verbose" short:"v" help:"Verbose output"`
	Count   int    `flag:"count" help:"Repeat count"`
	Output  string `flag:"output" short:"o" help:"Output file"`
}

func (t *typeHintCmd) Run(_ context.Context, _ []string) error { return nil }
func (t *typeHintCmd) Name() string                            { return "myapp" }

// --- subcommand with flags for zsh recursion test ---

type zshSubCmd struct {
	Timeout int `flag:"timeout" short:"t" help:"Timeout in seconds"`
}

func (z *zshSubCmd) Run(_ context.Context, _ []string) error { return nil }
func (z *zshSubCmd) Name() string                            { return "deploy" }
func (z *zshSubCmd) Description() string                     { return "Deploy the app" }

type zshRoot struct{}

func (z *zshRoot) Run(_ context.Context, _ []string) error { return nil }
func (z *zshRoot) Name() string                            { return "myapp" }
func (z *zshRoot) Subcommands() []cli.Runner               { return []cli.Runner{&zshSubCmd{}} }

// === Tests ===

func TestBash(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.Bash(root, "myapp")

	assert.Contains(t, script, "_myapp_completions")
	assert.Contains(t, script, "complete -F _myapp_completions myapp")
	assert.Contains(t, script, "serve")
	assert.NotContains(t, script, "internal") // hidden
}

func TestZsh(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.Zsh(root, "myapp")

	assert.Contains(t, script, "#compdef myapp")
	assert.Contains(t, script, "_myapp")
	assert.Contains(t, script, "serve")
	assert.NotContains(t, script, "internal") // hidden
}

func TestFish(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.Fish(root, "myapp")

	assert.Contains(t, script, "complete -c myapp")
	assert.Contains(t, script, "serve")
	assert.Contains(t, script, "-l port")
	assert.NotContains(t, script, "internal") // hidden
}

func TestPowerShell(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.PowerShell(root, "myapp")

	assert.Contains(t, script, "Register-ArgumentCompleter")
	assert.Contains(t, script, "myapp")
	assert.Contains(t, script, "serve")
	assert.NotContains(t, script, "internal") // hidden
}

func TestBash_SubcommandFlags(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.Bash(root, "myapp")

	assert.Contains(t, script, "--port")
	assert.Contains(t, script, "--tls")
}

func TestFish_SubcommandFlags(t *testing.T) {
	t.Parallel()
	root := newRoot()
	script := completion.Fish(root, "myapp")

	assert.Contains(t, script, "-l port")
	assert.Contains(t, script, "-s p")
	assert.Contains(t, script, "-l tls")
}

// --- Alias tests ---

func TestBash_IncludesAliases(t *testing.T) {
	t.Parallel()
	root := &aliasRoot{}
	script := completion.Bash(root, "myapp")

	assert.Contains(t, script, "deploy")
	assert.Contains(t, script, "d ")  // alias (space to avoid false match)
	assert.Contains(t, script, "dep") // alias
}

func TestFish_IncludesAliases(t *testing.T) {
	t.Parallel()
	root := &aliasRoot{}
	script := completion.Fish(root, "myapp")

	assert.Contains(t, script, "-a deploy")
	assert.Contains(t, script, "-a d ")  // alias
	assert.Contains(t, script, "-a dep") // alias
}

// --- Negate flag tests ---

func TestBash_IncludesNegateFlags(t *testing.T) {
	t.Parallel()
	cmd := &negateFlagCmd{}
	script := completion.Bash(cmd, "myapp")

	assert.Contains(t, script, "--color")
	assert.Contains(t, script, "--no-color")
}

func TestFish_IncludesNegateFlags(t *testing.T) {
	t.Parallel()
	cmd := &negateFlagCmd{}
	script := completion.Fish(cmd, "myapp")

	assert.Contains(t, script, "-l color")
	assert.Contains(t, script, "-l no-color")
}

// --- Deprecated flag tests ---

func TestBash_ExcludesDeprecatedFlags(t *testing.T) {
	t.Parallel()
	cmd := &deprecatedFlagCmd{}
	script := completion.Bash(cmd, "myapp")

	assert.NotContains(t, script, "--old")
	assert.Contains(t, script, "--new")
}

func TestFish_ExcludesDeprecatedFlags(t *testing.T) {
	t.Parallel()
	cmd := &deprecatedFlagCmd{}
	script := completion.Fish(cmd, "myapp")

	assert.NotContains(t, script, "-l old")
	assert.Contains(t, script, "-l new")
}

// --- Zsh recursion and alt flag tests ---

func TestZsh_RecursesIntoSubcommands(t *testing.T) {
	t.Parallel()
	root := &zshRoot{}
	script := completion.Zsh(root, "myapp")

	// Should have a subcommand function defined.
	assert.Contains(t, script, "_myapp_deploy()")
	// Should contain the subcommand's flags.
	assert.Contains(t, script, "--timeout")
	assert.Contains(t, script, "-t")
	// Should have case dispatch.
	assert.Contains(t, script, "case $words[2] in")
	assert.Contains(t, script, "deploy)")
}

func TestZsh_IncludesAltFlags(t *testing.T) {
	t.Parallel()
	cmd := &altFlagCmd{}
	script := completion.Zsh(cmd, "myapp")

	assert.Contains(t, script, "--output")
	assert.Contains(t, script, "--out")
}

func TestFish_IncludesAltFlags(t *testing.T) {
	t.Parallel()
	cmd := &altFlagCmd{}
	script := completion.Fish(cmd, "myapp")

	assert.Contains(t, script, "-l output")
	assert.Contains(t, script, "-l out")
}

// --- Fish type hints ---

func TestFish_TypeHints(t *testing.T) {
	t.Parallel()
	cmd := &typeHintCmd{}
	script := completion.Fish(cmd, "myapp")

	// Bool flags should have -f (no file completion).
	assert.Contains(t, script, "-l verbose -f")
	// Value flags should have -r (requires argument).
	assert.Contains(t, script, "-l count -r")
	assert.Contains(t, script, "-l output -r")
}

// --- Fish enum values ---

func TestFish_EnumValues(t *testing.T) {
	t.Parallel()
	cmd := &enumFlagCmd{}
	script := completion.Fish(cmd, "myapp")

	assert.Contains(t, script, "-a 'json yaml text'")
}

// --- Discovered commands ---

func TestBash_IncludesDiscoveredCommands(t *testing.T) {
	t.Parallel()
	root := &discovererRoot{}
	script := completion.Bash(root, "myapp")

	assert.Contains(t, script, "plugin")
}
