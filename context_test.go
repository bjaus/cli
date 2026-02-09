package cli_test

import (
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSet_And_Get(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "env", "prod")
	assert.Equal(t, "prod", cli.Get[string](ctx, "env"))
}

func TestSet_Overwrites(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "port", 8080)
	ctx = cli.Set(ctx, "port", 9090)
	assert.Equal(t, 9090, cli.Get[int](ctx, "port"))
}

func TestLookup_Found(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "verbose", true)
	val, ok := cli.Lookup[bool](ctx, "verbose")
	require.True(t, ok)
	assert.True(t, val)
}

func TestLookup_NotFound_NoStore(t *testing.T) {
	t.Parallel()

	val, ok := cli.Lookup[string](context.Background(), "missing")
	assert.False(t, ok)
	assert.Empty(t, val)
}

func TestLookup_NotFound_KeyMissing(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "host", "localhost")
	val, ok := cli.Lookup[string](ctx, "port")
	assert.False(t, ok)
	assert.Empty(t, val)
}

func TestLookup_TypeMismatch(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "port", 8080)
	val, ok := cli.Lookup[string](ctx, "port")
	assert.False(t, ok)
	assert.Empty(t, val)
}

func TestGet_NotFound_ReturnsZero(t *testing.T) {
	t.Parallel()

	assert.Empty(t, cli.Get[string](context.Background(), "missing"))
	assert.Zero(t, cli.Get[int](context.Background(), "missing"))
	assert.False(t, cli.Get[bool](context.Background(), "missing"))
}

func TestGet_TypeMismatch_ReturnsZero(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "port", 8080)
	assert.Empty(t, cli.Get[string](ctx, "port"))
}

func TestLookup_NoStore(t *testing.T) {
	t.Parallel()

	// Fresh context with no store at all.
	val, ok := cli.Lookup[int](context.Background(), "anything")
	assert.False(t, ok)
	assert.Zero(t, val)
}

func TestSet_MultipleKeys(t *testing.T) {
	t.Parallel()

	ctx := cli.Set(context.Background(), "host", "localhost")
	ctx = cli.Set(ctx, "port", 8080)
	ctx = cli.Set(ctx, "verbose", true)

	assert.Equal(t, "localhost", cli.Get[string](ctx, "host"))
	assert.Equal(t, 8080, cli.Get[int](ctx, "port"))
	assert.True(t, cli.Get[bool](ctx, "verbose"))
}

// --- Integration: flags are available via Get/Lookup after Execute ---

type contextParent struct {
	Env string `flag:"env" default:"dev"`
	t   *testing.T
}

func (c *contextParent) Run(_ context.Context, _ []string) error { return nil }
func (c *contextParent) Name() string                            { return "app" }
func (c *contextParent) Subcommands() []cli.Runner {
	return []cli.Runner{&contextChild{t: c.t}}
}

type contextChild struct {
	Port int `flag:"port" default:"8080"`
	t    *testing.T
}

func (c *contextChild) Run(ctx context.Context, _ []string) error {
	assert.Equal(c.t, "prod", cli.Get[string](ctx, "env"))
	assert.Equal(c.t, 9090, cli.Get[int](ctx, "port"))
	return nil
}

func (c *contextChild) Name() string { return "serve" }

func TestExecute_FlagsAvailableViaContext(t *testing.T) {
	t.Parallel()

	parent := &contextParent{t: t}
	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve", "--port", "9090"})
	require.NoError(t, err)
}
