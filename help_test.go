package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomHelper(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &customHelpCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)
	assert.Equal(t, "Custom help text!", buf.String())
}

type customHelpCmd struct{}

func (c *customHelpCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *customHelpCmd) Help() string                            { return "Custom help text!" }

func TestCustomHelpRenderer(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &serveCmd{}
	renderer := &testRenderer{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"},
		cli.WithStdout(&buf),
		cli.WithHelpRenderer(renderer),
	)
	require.NoError(t, err)
	assert.Equal(t, "rendered by test", buf.String())
}

type testRenderer struct{}

func (r *testRenderer) RenderHelp(_ cli.Runner, _ []cli.Runner, _ []cli.FlagDef, _ []cli.ArgDef, _ []cli.FlagDef) string {
	return "rendered by test"
}

// --- LongDescriber ---

type longDescCmd struct{}

func (c *longDescCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *longDescCmd) Name() string                            { return "longdesc" }
func (c *longDescCmd) Description() string                     { return "Short description" }
func (c *longDescCmd) LongDescription() string {
	return "This is a much longer description that spans\nmultiple lines and provides detailed information."
}

type longDescParent struct{}

func (p *longDescParent) Run(_ context.Context, _ []string) error { return nil }
func (p *longDescParent) Name() string                            { return "myapp" }
func (p *longDescParent) Subcommands() []cli.Runner               { return []cli.Runner{&longDescCmd{}} }

func TestLongDescription_ShownInHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &longDescCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "much longer description")
	assert.NotContains(t, output, "Short description")
}

func TestLongDescription_NotInSubcommandList(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	parent := &longDescParent{}
	err := cli.Execute(context.Background(), parent, []string{"--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)

	output := buf.String()
	// Subcommand listing should use short description.
	assert.Contains(t, output, "Short description")
	assert.NotContains(t, output, "much longer description")
}
