package cli_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bjaus/cli"
)

type mockDB struct {
	name string
}

type Cache interface {
	Get(key string) string
}

type redisCache struct {
	prefix string
}

func (r *redisCache) Get(key string) string {
	return r.prefix + ":" + key
}

// bindTestCmd has a field that can be injected by type.
type bindTestCmd struct {
	DB   *mockDB // will be injected if bound
	Port int     `flag:"port" default:"8080"`

	ran bool
}

func (c *bindTestCmd) Run(ctx context.Context) error {
	c.ran = true
	return nil
}

func TestBind(t *testing.T) {
	db := &mockDB{name: "testdb"}
	cmd := &bindTestCmd{}

	err := cli.Execute(context.Background(), cmd, []string{},
		cli.Bind(db),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if cmd.DB != db {
		t.Errorf("DB not injected: got %v, want %v", cmd.DB, db)
	}
	if !cmd.ran {
		t.Error("Run was not called")
	}
}

type bindToCmd struct {
	Cache Cache // interface field
}

func (c *bindToCmd) Run(ctx context.Context) error {
	return nil
}

func TestBindTo(t *testing.T) {
	cache := &redisCache{prefix: "test"}
	cmd := &bindToCmd{}

	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindTo(cache, (*Cache)(nil)),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if cmd.Cache == nil {
		t.Fatal("Cache not injected")
	}
	if cmd.Cache.Get("foo") != "test:foo" {
		t.Errorf("Cache.Get returned wrong value: %s", cmd.Cache.Get("foo"))
	}
}

func TestBindNoMatchingBinding(t *testing.T) {
	// When no binding matches, field remains zero (no error).
	cmd := &bindTestCmd{}

	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindTo(&redisCache{}, (*Cache)(nil)), // bind cache, not DB
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if cmd.DB != nil {
		t.Error("DB should remain nil when not bound")
	}
}

func TestBindNoBindingsProvided(t *testing.T) {
	// When no Bind options are used, fields remain at zero value.
	cmd := &bindTestCmd{}

	err := cli.Execute(context.Background(), cmd, []string{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if cmd.DB != nil {
		t.Error("DB should remain nil")
	}
}

type flagOnlyCmd struct {
	Port int `flag:"port" default:"8080"`
}

func (c *flagOnlyCmd) Run(ctx context.Context) error { return nil }

func TestBindSkipsFlagFields(t *testing.T) {
	// Fields with flag tags should not be injected even if type matches.
	c := &flagOnlyCmd{}

	// Bind an int - it should NOT inject into Port because it has flag tag.
	err := cli.Execute(context.Background(), c, []string{},
		cli.Bind(9999),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if c.Port != 8080 {
		t.Errorf("Port should be default 8080, got %d (was incorrectly injected)", c.Port)
	}
}

// Test that bindings are available in subcommands.
type bindParentCmd struct {
	DB *mockDB
}

func (c *bindParentCmd) Subcommands() []cli.Commander {
	return []cli.Commander{&bindChildCmd{}}
}

func (c *bindParentCmd) Run(ctx context.Context) error {
	return nil
}

type bindChildCmd struct {
	DB    *mockDB
	Cache Cache
}

func (c *bindChildCmd) Name() string { return "child" }

func (c *bindChildCmd) Run(ctx context.Context) error {
	return nil
}

func TestBindInChain(t *testing.T) {
	db := &mockDB{name: "shared"}
	cache := &redisCache{prefix: "child"}
	parent := &bindParentCmd{}

	err := cli.Execute(context.Background(), parent, []string{"child"},
		cli.Bind(db),
		cli.BindTo(cache, (*Cache)(nil)),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if parent.DB != db {
		t.Error("parent DB not injected")
	}
}

type argsCmd struct {
	Args cli.Args
	ran  bool
}

func (c *argsCmd) Run(ctx context.Context) error {
	c.ran = true
	return nil
}

func TestBindArgs(t *testing.T) {
	cmd := &argsCmd{}

	err := cli.Execute(context.Background(), cmd, []string{"foo", "bar", "baz"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !cmd.ran {
		t.Error("Run was not called")
	}

	want := cli.Args{"foo", "bar", "baz"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("wrong args length: got %d, want %d", len(cmd.Args), len(want))
	}
	for i, arg := range cmd.Args {
		if arg != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, want[i])
		}
	}
}

type argsFlagsCmd struct {
	Args cli.Args
	Port int `flag:"port" default:"8080"`
}

func (c *argsFlagsCmd) Run(ctx context.Context) error {
	return nil
}

// --- BindProvider ---

type providerCmd struct {
	DB *mockDB
}

func (c *providerCmd) Run(_ context.Context) error { return nil }

func TestBindProvider(t *testing.T) {
	calls := 0
	cmd := &providerCmd{}
	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindProvider(func() (*mockDB, error) {
			calls++
			return &mockDB{name: "provided"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if cmd.DB == nil || cmd.DB.name != "provided" {
		t.Error("DB not injected by provider")
	}
	if calls != 1 {
		t.Errorf("provider called %d times, want 1", calls)
	}
}

func TestBindProvider_Error(t *testing.T) {
	cmd := &providerCmd{}
	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindProvider(func() (*mockDB, error) {
			return nil, fmt.Errorf("connection failed")
		}),
	)
	if err == nil {
		t.Fatal("expected error from provider")
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Errorf("error should contain provider error: %v", err)
	}
}

// --- BindSingleton ---

func TestBindSingleton(t *testing.T) {
	calls := 0
	db := &mockDB{name: "singleton"}
	cmd := &providerCmd{}

	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindSingleton(func() (*mockDB, error) {
			calls++
			return db, nil
		}),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if cmd.DB != db {
		t.Error("DB not injected by singleton")
	}
	if calls != 1 {
		t.Errorf("singleton called %d times, want 1", calls)
	}
}

type singletonParentCmd struct {
	DB *mockDB
}

func (c *singletonParentCmd) Run(_ context.Context) error   { return nil }
func (c *singletonParentCmd) Subcommands() []cli.Commander     { return []cli.Commander{&singletonChildCmd{}} }

type singletonChildCmd struct {
	DB *mockDB
}

func (c *singletonChildCmd) Name() string                { return "child" }
func (c *singletonChildCmd) Run(_ context.Context) error { return nil }

func TestBindSingleton_CachedAcrossChain(t *testing.T) {
	calls := 0
	parent := &singletonParentCmd{}

	err := cli.Execute(context.Background(), parent, []string{"child"},
		cli.BindSingleton(func() (*mockDB, error) {
			calls++
			return &mockDB{name: "shared"}, nil
		}),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Singleton should be called once but injected into both parent and child.
	if calls != 1 {
		t.Errorf("singleton called %d times, want 1", calls)
	}
	if parent.DB == nil || parent.DB.name != "shared" {
		t.Error("parent DB not injected")
	}
}

func TestBindSingleton_Error(t *testing.T) {
	cmd := &providerCmd{}
	err := cli.Execute(context.Background(), cmd, []string{},
		cli.BindSingleton(func() (*mockDB, error) {
			return nil, fmt.Errorf("init failed")
		}),
	)
	if err == nil {
		t.Fatal("expected error from singleton")
	}
	if !strings.Contains(err.Error(), "init failed") {
		t.Errorf("error should contain singleton error: %v", err)
	}
}

func TestBindArgsWithFlags(t *testing.T) {
	c := &argsFlagsCmd{}

	err := cli.Execute(context.Background(), c, []string{"--port", "3000", "file1.txt", "file2.txt"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if c.Port != 3000 {
		t.Errorf("Port = %d, want 3000", c.Port)
	}
	if len(c.Args) != 2 || c.Args[0] != "file1.txt" || c.Args[1] != "file2.txt" {
		t.Errorf("Args = %v, want [file1.txt file2.txt]", c.Args)
	}
}
