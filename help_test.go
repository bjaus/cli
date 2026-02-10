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
