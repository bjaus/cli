# Dependency Injection

The framework provides built-in dependency injection to share resources across commands. Register bindings once, and they're automatically injected into struct fields.

## Basic Usage

Register bindings with `WithBindings`, then declare fields without tags to receive injected values:

```go
func main() {
    db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL"))

    cli.ExecuteAndExit(ctx, &App{}, os.Args,
        cli.WithBindings(
            bind.Value(db),
        ),
    )
}

type ServeCmd struct {
    DB   *sql.DB // injected automatically
    Port int     `flag:"port"` // NOT injected (has flag tag)
}

func (s *ServeCmd) Run(ctx context.Context) error {
    rows, _ := s.DB.Query("SELECT 1")
    // ...
}
```

## Binding Modes

Four binding modes cover different dependency patterns:

### bind.Value

Register a value matched by its concrete type:

```go
db, _ := sql.Open("postgres", connStr)
cli.WithBindings(
    bind.Value(db),  // matches *sql.DB fields
)
```

### bind.Interface

Register a value as an interface type:

```go
type Cache interface {
    Get(key string) string
    Set(key, value string)
}

type RedisCache struct{ /* ... */ }

cache := &RedisCache{}
cli.WithBindings(
    bind.Interface(cache, (*Cache)(nil)),  // matches Cache fields
)
```

The second argument is a nil pointer to the interface type — Go syntax for specifying the interface.

### bind.Provider

Lazy factory called on each injection:

```go
cli.WithBindings(
    bind.Provider(func() (*RequestID, error) {
        return &RequestID{Value: uuid.New().String()}, nil
    }),
)
```

Each command receives a fresh instance. Useful for per-request resources.

### bind.Singleton

Lazy factory called once, then cached:

```go
cli.WithBindings(
    bind.Singleton(func() (*sql.DB, error) {
        return sql.Open("postgres", os.Getenv("DATABASE_URL"))
    }),
)
```

The factory runs on first injection; subsequent injections receive the cached value.

## Declaring Dependencies

Fields without tags are eligible for injection:

```go
type ServeCmd struct {
    // Injected (no tags)
    DB     *sql.DB
    Cache  Cache
    Logger *slog.Logger

    // NOT injected (have tags)
    Port   int    `flag:"port"`
    Env    string `arg:"env"`
    Secret string `env:"SECRET"`
}
```

The injector matches fields by:
1. **Exact type** — `*sql.DB` field matches `bind.Value(db)` where db is `*sql.DB`
2. **Interface compatibility** — `Cache` field matches `bind.Interface(v, (*Cache)(nil))`

## Context Lookup

Bindings are also accessible via context in `Run`:

```go
func (s *ServeCmd) Run(ctx context.Context) error {
    db := bind.Get[*sql.DB](ctx)
    cache := bind.Get[Cache](ctx)

    // Or with existence check
    logger, ok := bind.Lookup[*slog.Logger](ctx)
    if !ok {
        logger = slog.Default()
    }

    return nil
}
```

This is useful when:
- You prefer explicit over implicit dependencies
- You need optional dependencies
- You're working with middleware or nested functions

## Auto-Bound Types

### cli.Args

`cli.Args` is automatically bound to remaining positional arguments:

```go
type GrepCmd struct {
    Pattern string   `arg:"pattern"`
    Args    cli.Args // automatically populated — no tag needed
}

func (g *GrepCmd) Run(ctx context.Context) error {
    for _, file := range g.Args {
        // process file
    }
    return nil
}
```

## Complete Example

```go
package main

import (
    "context"
    "database/sql"
    "log/slog"
    "os"

    "github.com/bjaus/bind"
    "github.com/bjaus/cli"
)

// Interfaces for abstraction
type Cache interface {
    Get(key string) (string, bool)
    Set(key, value string)
}

type Metrics interface {
    Inc(name string)
    Observe(name string, value float64)
}

func main() {
    ctx := context.Background()

    // Create shared resources
    db, _ := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    cache := newRedisCache(os.Getenv("REDIS_URL"))
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    cli.ExecuteAndExit(ctx, &App{}, os.Args,
        cli.WithBindings(
            // Concrete types
            bind.Value(db),
            bind.Value(logger),

            // Interface types
            bind.Interface(cache, (*Cache)(nil)),

            // Singleton — created once when first needed
            bind.Singleton(func() (Metrics, error) {
                return newPrometheusMetrics()
            }),

            // Provider — fresh instance each time
            bind.Provider(func() (*RequestContext, error) {
                return &RequestContext{
                    RequestID: uuid.New().String(),
                    StartTime: time.Now(),
                }, nil
            }),
        ),
    )
}

type App struct {
    Logger *slog.Logger // injected
}

func (a *App) Run(ctx context.Context) error {
    return cli.ErrShowHelp
}

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{&ServeCmd{}, &MigrateCmd{}}
}

type ServeCmd struct {
    // Injected dependencies
    DB      *sql.DB
    Cache   Cache
    Logger  *slog.Logger
    Metrics Metrics

    // Flags (not injected)
    Port int `flag:"port" default:"8080"`
}

func (s *ServeCmd) Name() string { return "serve" }

func (s *ServeCmd) Run(ctx context.Context) error {
    s.Metrics.Inc("server_starts")
    s.Logger.Info("starting server", "port", s.Port)

    // Use s.DB, s.Cache, etc.
    return nil
}

type MigrateCmd struct {
    // Only needs DB
    DB     *sql.DB
    Logger *slog.Logger

    // Flags
    Version int `flag:"version" required:""`
}

func (m *MigrateCmd) Name() string { return "migrate" }

func (m *MigrateCmd) Run(ctx context.Context) error {
    m.Logger.Info("running migration", "version", m.Version)
    // Use m.DB
    return nil
}
```

## Injection Timing

Dependencies are injected:
1. **After flag parsing** — so flag values are available
2. **Before Before hooks** — so dependencies are ready for setup
3. **For all commands in the chain** — parent and child commands

```
Parse flags → Inject dependencies → Before hooks → Run
```

## Error Handling

Provider and Singleton factories can return errors:

```go
cli.WithBindings(
    bind.Singleton(func() (*sql.DB, error) {
        db, err := sql.Open("postgres", connStr)
        if err != nil {
            return nil, fmt.Errorf("database connection failed: %w", err)
        }
        return db, nil
    }),
)
```

If a factory returns an error, execution stops and the error is returned from `Execute`.

## Testing

Dependency injection simplifies testing — inject mocks instead of real dependencies:

```go
func TestServeCmd(t *testing.T) {
    mockDB := &MockDB{}
    mockCache := &MockCache{}

    cmd := &ServeCmd{}

    err := cli.Execute(ctx, cmd, []string{"--port", "9000"},
        cli.WithBindings(
            bind.Value(mockDB),
            bind.Interface(mockCache, (*Cache)(nil)),
        ),
    )

    assert.NoError(t, err)
    assert.True(t, mockDB.WasCalled)
}
```

## Best Practices

### Use Interfaces

Prefer interface bindings for testability:

```go
// Good: interface allows mocking
type UserService interface {
    GetUser(id int) (*User, error)
}

cli.WithBindings(
    bind.Interface(svc, (*UserService)(nil)),
)

// Less flexible: concrete type
cli.WithBindings(
    bind.Value(svc),  // harder to mock
)
```

### Use Singletons for Expensive Resources

```go
cli.WithBindings(
    // Good: created once
    bind.Singleton(func() (*sql.DB, error) {
        return sql.Open("postgres", connStr)
    }),

    // Bad: creates connection on every injection
    // bind.Provider(func() (*sql.DB, error) { ... }),
)
```

### Use Providers for Per-Request Data

```go
cli.WithBindings(
    bind.Provider(func() (*TraceContext, error) {
        return &TraceContext{
            TraceID: generateTraceID(),
            SpanID:  generateSpanID(),
        }, nil
    }),
)
```

### Keep Commands Focused

Each command should only declare the dependencies it needs:

```go
// Good: only declares what it uses
type ListCmd struct {
    DB *sql.DB
}

// Avoid: declaring unused dependencies
type ListCmd struct {
    DB      *sql.DB
    Cache   Cache      // unused
    Metrics Metrics    // unused
}
```

## What's Next

- [Lifecycle](lifecycle.md) — Hooks that run before and after commands
- [Flags](flags.md) — Flag parsing and environment variables
- [Configuration](configuration.md) — External config file support
