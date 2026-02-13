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

// --- Integration: args are available via Get/Lookup after Execute ---

type contextArgChild struct {
	t *testing.T
}

func (c *contextArgChild) Run(ctx context.Context) error {
	assert.Equal(c.t, 42, cli.Get[int](ctx, "user-id"))
	return nil
}

func (c *contextArgChild) Name() string { return "delete" }

type contextArgParent struct {
	UserID int             `arg:"user-id"`
	Delete contextArgChild // embedded Commander field triggers branching
	t      *testing.T
}

func (c *contextArgParent) Run(_ context.Context) error { return nil }
func (c *contextArgParent) Name() string                { return "user" }

func TestExecute_ArgsAvailableViaContext(t *testing.T) {
	parent := &contextArgParent{t: t}
	parent.Delete.t = t
	err := cli.Execute(context.Background(), parent, []string{"42", "delete"})
	require.NoError(t, err)
}
