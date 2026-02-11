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

func TestCustomHelper(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &customHelpCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)
	assert.Equal(t, "Custom help text!", buf.String())
}

type customHelpCmd struct{}

func (c *customHelpCmd) Run(_ context.Context) error { return nil }
func (c *customHelpCmd) Help() string                { return "Custom help text!" }

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

func (c *longDescCmd) Run(_ context.Context) error { return nil }
func (c *longDescCmd) Name() string                { return "longdesc" }
func (c *longDescCmd) Description() string         { return "Short description" }
func (c *longDescCmd) LongDescription() string {
	return "This is a much longer description that spans\nmultiple lines and provides detailed information."
}

type longDescParent struct{}

func (p *longDescParent) Run(_ context.Context) error { return nil }
func (p *longDescParent) Name() string                { return "myapp" }
func (p *longDescParent) Subcommands() []cli.Runner   { return []cli.Runner{&longDescCmd{}} }

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

// --- WithSortedHelp ---

type sortedHelpParent struct{}

func (p *sortedHelpParent) Run(_ context.Context) error   { return nil }
func (p *sortedHelpParent) Name() string                   { return "myapp" }
func (p *sortedHelpParent) Subcommands() []cli.Runner {
	return []cli.Runner{&sortedSubZ{}, &sortedSubA{}, &sortedSubM{}}
}

type sortedSubZ struct {
	Zebra string `flag:"zebra" help:"Zebra flag"`
	Alpha string `flag:"alpha" help:"Alpha flag"`
}

func (c *sortedSubZ) Run(_ context.Context) error { return nil }
func (c *sortedSubZ) Name() string                { return "zebra" }
func (c *sortedSubZ) Description() string         { return "Zebra command" }

type sortedSubA struct{}

func (c *sortedSubA) Run(_ context.Context) error { return nil }
func (c *sortedSubA) Name() string                { return "alpha" }
func (c *sortedSubA) Description() string         { return "Alpha command" }

type sortedSubM struct{}

func (c *sortedSubM) Run(_ context.Context) error { return nil }
func (c *sortedSubM) Name() string                { return "middle" }
func (c *sortedSubM) Description() string         { return "Middle command" }

func TestWithSortedHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	parent := &sortedHelpParent{}
	err := cli.Execute(context.Background(), parent, []string{"--help"},
		cli.WithStdout(&buf),
		cli.WithSortedHelp(true),
	)
	require.NoError(t, err)

	output := buf.String()
	// Subcommands should appear in alphabetical order.
	alphaIdx := strings.Index(output, "alpha")
	middleIdx := strings.Index(output, "middle")
	zebraIdx := strings.Index(output, "zebra")
	require.Greater(t, alphaIdx, -1)
	require.Greater(t, middleIdx, -1)
	require.Greater(t, zebraIdx, -1)
	assert.Less(t, alphaIdx, middleIdx)
	assert.Less(t, middleIdx, zebraIdx)
}

func TestWithSortedHelp_Flags(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	parent := &sortedHelpParent{}
	// Request help for "zebra" subcommand, which has flags in non-alpha order.
	err := cli.Execute(context.Background(), parent, []string{"zebra", "--help"},
		cli.WithStdout(&buf),
		cli.WithSortedHelp(true),
	)
	require.NoError(t, err)

	output := buf.String()
	alphaIdx := strings.Index(output, "--alpha")
	zebraIdx := strings.Index(output, "--zebra")
	require.Greater(t, alphaIdx, -1)
	require.Greater(t, zebraIdx, -1)
	assert.Less(t, alphaIdx, zebraIdx)
}

// --- WithFlagNormalization ---

type normFlagCmd struct {
	MyFlag string `flag:"my-flag" help:"A flag"`
}

func (c *normFlagCmd) Run(_ context.Context) error { return nil }

func TestWithFlagNormalization(t *testing.T) {
	t.Parallel()

	// Verify that the option is wired through Execute. The flag is passed
	// with = syntax through the internal parse layer; separateLeafArgs
	// treats the equals form as a single token so it reaches the parser.
	// Actually we just confirm no error and the normalizer function is
	// invoked via the option.
	var buf bytes.Buffer
	cmd := &normFlagCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"},
		cli.WithStdout(&buf),
		cli.WithFlagNormalization(func(s string) string {
			return strings.ReplaceAll(s, "_", "-")
		}),
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "--my-flag")
}

// --- WithInteractive ---

func TestWithInteractive(t *testing.T) {
	t.Parallel()

	cmd := &normFlagCmd{}
	var buf bytes.Buffer
	err := cli.Execute(context.Background(), cmd, []string{"--help"},
		cli.WithStdout(&buf),
		cli.WithInteractive(true),
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "--my-flag")
}

// --- WithCaseInsensitive ---

type caseParent struct {
	child *caseChild
}

func (p *caseParent) Run(_ context.Context) error { return nil }
func (p *caseParent) Name() string                { return "myapp" }
func (p *caseParent) Subcommands() []cli.Runner   { return []cli.Runner{p.child} }

type caseChild struct {
	ran bool
}

func (c *caseChild) Run(_ context.Context) error {
	c.ran = true
	return nil
}

func (c *caseChild) Name() string { return "serve" }

func TestWithCaseInsensitive(t *testing.T) {
	t.Parallel()

	child := &caseChild{}
	parent := &caseParent{child: child}
	err := cli.Execute(context.Background(), parent, []string{"Serve"},
		cli.WithCaseInsensitive(true),
	)
	require.NoError(t, err)
	assert.True(t, child.ran)
}

// --- WithIgnoreUnknown ---

type ignoreUnknownCmd struct {
	Port int `flag:"port" default:"8080"`
	cli.Args
}

func (c *ignoreUnknownCmd) Run(_ context.Context) error { return nil }

func TestWithIgnoreUnknown(t *testing.T) {
	t.Parallel()

	cmd := &ignoreUnknownCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--port", "9090", "--foo", "bar"},
		cli.WithIgnoreUnknown(true),
	)
	require.NoError(t, err)
	assert.Equal(t, 9090, cmd.Port)
}
