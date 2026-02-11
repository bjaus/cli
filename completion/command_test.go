package completion_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/completion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_Structure(t *testing.T) {
	t.Parallel()

	root := &rootCmd{}
	var buf bytes.Buffer
	cmd := completion.Command(root, "myapp", &buf)

	n, ok := cmd.(cli.Namer)
	require.True(t, ok)
	assert.Equal(t, "completion", n.Name())

	d, ok := cmd.(cli.Describer)
	require.True(t, ok)
	assert.NotEmpty(t, d.Description())

	p, ok := cmd.(cli.Parent)
	require.True(t, ok)

	subs := p.Subcommands()
	require.Len(t, subs, 4)

	wantShells := []string{"bash", "zsh", "fish", "powershell"}
	for i, sub := range subs {
		n, ok := sub.(cli.Namer)
		require.True(t, ok)
		assert.Equal(t, wantShells[i], n.Name())

		d, ok := sub.(cli.Describer)
		require.True(t, ok)
		assert.NotEmpty(t, d.Description())
	}
}

func TestCommand_Hidden(t *testing.T) {
	t.Parallel()

	root := &rootCmd{}
	var buf bytes.Buffer
	cmd := completion.Command(root, "myapp", &buf)

	h, ok := cmd.(cli.Hider)
	require.True(t, ok)
	assert.True(t, h.Hidden())
}

func TestCommand_ShowsHelp(t *testing.T) {
	t.Parallel()

	root := &rootCmd{}
	var buf bytes.Buffer
	cmd := completion.Command(root, "myapp", &buf)

	err := cmd.Run(context.Background())
	assert.ErrorIs(t, err, cli.ErrShowHelp)
}

func TestCommand_UsesWriter(t *testing.T) {
	t.Parallel()

	root := &rootCmd{}
	var buf bytes.Buffer
	cmd := completion.Command(root, "myapp", &buf)

	p, ok := cmd.(cli.Parent)
	require.True(t, ok)

	subs := p.Subcommands()
	require.NotEmpty(t, subs)

	// Run bash subcommand.
	err := subs[0].Run(t.Context())
	require.NoError(t, err)

	assert.NotEmpty(t, buf.String())
	assert.Contains(t, buf.String(), "myapp")
}

func TestCommand_ShellOutput(t *testing.T) {
	t.Parallel()

	root := &rootCmd{}

	shells := []string{"bash", "zsh", "fish", "powershell"}
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cmd := completion.Command(root, "myapp", &buf)
			p, ok := cmd.(cli.Parent)
			require.True(t, ok)

			// Find the matching shell subcommand.
			var target cli.Runner
			for _, sub := range p.Subcommands() {
				n, ok := sub.(cli.Namer)
				if ok && n.Name() == shell {
					target = sub
					break
				}
			}
			require.NotNil(t, target)

			err := target.Run(t.Context())
			require.NoError(t, err)

			assert.NotEmpty(t, buf.String())
			assert.Contains(t, buf.String(), "myapp")
		})
	}
}
