package completion_test

import (
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/completion"
	"github.com/stretchr/testify/assert"
)

type rootCmd struct{}

func (r *rootCmd) Run(_ context.Context) error { return nil }
func (r *rootCmd) Name() string                { return "myapp" }
func (r *rootCmd) Description() string         { return "My test app" }

func TestBash_RuntimeScript(t *testing.T) {
	t.Parallel()
	root := &rootCmd{}
	script := completion.Bash(root, "myapp")

	// Script structure.
	assert.Contains(t, script, "# bash completion for myapp")
	assert.Contains(t, script, "_myapp_completions()")
	assert.Contains(t, script, "complete -o default -F _myapp_completions myapp")

	// Runtime __complete call.
	assert.Contains(t, script, "__complete")
	assert.Contains(t, script, "COMP_WORDS")

	// Directive handling.
	assert.Contains(t, script, "compgen")
	assert.Contains(t, script, "compopt")
}

func TestZsh_RuntimeScript(t *testing.T) {
	t.Parallel()
	root := &rootCmd{}
	script := completion.Zsh(root, "myapp")

	// Script structure.
	assert.Contains(t, script, "#compdef myapp")
	assert.Contains(t, script, "_myapp()")

	// Runtime __complete call.
	assert.Contains(t, script, "__complete")
	assert.Contains(t, script, "${words[@]:1}")

	// Zsh completion.
	assert.Contains(t, script, "_describe")
}

func TestFish_RuntimeScript(t *testing.T) {
	t.Parallel()
	root := &rootCmd{}
	script := completion.Fish(root, "myapp")

	// Script structure.
	assert.Contains(t, script, "# fish completion for myapp")
	assert.Contains(t, script, "complete -c myapp")
	assert.Contains(t, script, "__myapp_complete")

	// Runtime __complete call.
	assert.Contains(t, script, "__complete")
	assert.Contains(t, script, "commandline")
}

func TestPowerShell_RuntimeScript(t *testing.T) {
	t.Parallel()
	root := &rootCmd{}
	script := completion.PowerShell(root, "myapp")

	// Script structure.
	assert.Contains(t, script, "# PowerShell completion for myapp")
	assert.Contains(t, script, "Register-ArgumentCompleter")

	// Runtime __complete call.
	assert.Contains(t, script, "__complete")

	// CompletionResult.
	assert.Contains(t, script, "CompletionResult")
}

func TestBash_AppNameInScript(t *testing.T) {
	t.Parallel()
	root := &rootCmd{}
	script := completion.Bash(root, "my-tool.sh")

	// Bash-safe function names replace - and . with _.
	assert.Contains(t, script, "_my_tool_sh_completions()")
	assert.Contains(t, script, "complete -o default -F _my_tool_sh_completions my-tool.sh")

	// App name in __complete call should be original.
	assert.Contains(t, script, "my-tool.sh __complete")
}

// Verify the root Commander parameter is accepted but not walked statically.
// The scripts should not contain specific subcommand names.
type rootWithSubs struct {
	cli.Commander
}

func (r *rootWithSubs) Run(_ context.Context) error { return nil }
func (r *rootWithSubs) Name() string                { return "myapp" }
func (r *rootWithSubs) Subcommands() []cli.Commander {
	return []cli.Commander{&subCmd{}}
}

type subCmd struct{}

func (s *subCmd) Run(_ context.Context) error { return nil }
func (s *subCmd) Name() string                { return "serve" }

func TestBash_NoStaticSubcommands(t *testing.T) {
	t.Parallel()
	root := &rootWithSubs{}
	script := completion.Bash(root, "myapp")

	// Static subcommand names should NOT appear in runtime scripts.
	assert.NotContains(t, script, "serve")
	// But __complete call should be present.
	assert.Contains(t, script, "__complete")
}
