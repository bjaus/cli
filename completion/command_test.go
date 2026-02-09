package completion_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/completion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_Structure(t *testing.T) {
	t.Parallel()

	root := newRoot()
	cmd := completion.Command(root, "myapp")

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

func TestCommand_ShowsHelp(t *testing.T) {
	t.Parallel()

	root := newRoot()
	cmd := completion.Command(root, "myapp")

	err := cmd.Run(context.Background(), nil)
	assert.ErrorIs(t, err, cli.ErrShowHelp)
}

func TestCommand_ShellOutput(t *testing.T) {
	// Not parallel — we're redirecting os.Stdout which is global.
	root := newRoot()
	cmd := completion.Command(root, "myapp")
	p, ok := cmd.(cli.Parent)
	require.True(t, ok)
	subs := p.Subcommands()

	shells := []string{"bash", "zsh", "fish", "powershell"}
	for i, sub := range subs {
		t.Run(shells[i], func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)

			oldStdout := os.Stdout
			os.Stdout = w

			runErr := sub.Run(t.Context(), nil)

			os.Stdout = oldStdout
			require.NoError(t, w.Close())
			require.NoError(t, runErr)

			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)
			require.NoError(t, r.Close())

			assert.NotEmpty(t, buf.String())
			assert.Contains(t, buf.String(), "myapp")
		})
	}
}
