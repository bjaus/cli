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
