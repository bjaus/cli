# Lifecycle Hooks

The framework provides lifecycle hooks that run at specific points during command execution. These enable setup, validation, teardown, and cross-cutting concerns without modifying your `Run` method.

## Execution Order

The complete lifecycle for a command chain `app → serve`:

```
1. Init (parent-first)      → App.Init() → ServeCmd.Init()
2. Parse flags/args         → CLI args, env vars, config, defaults
3. Default (parent-first)   → App.Default() → ServeCmd.Default()
4. Validate args            → ServeCmd.ValidateArgs()
5. Validate (leaf only)     → ServeCmd.Validate()
6. Before (parent-first)    → App.Before() → ServeCmd.Before()
7. Run (leaf only)          → ServeCmd.Run()
8. After (child-first)      → ServeCmd.After() → App.After()
```

## Initializer

`Init` runs **before flag parsing**, parent-first. Use it for early setup that doesn't depend on parsed values.

```go
type App struct {
    Debug bool `flag:"debug"`
}

func (a *App) Init(ctx context.Context) (context.Context, error) {
    // Set up logging before flags are parsed
    logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
    return context.WithValue(ctx, "logger", logger), nil
}
```

The returned context flows to subsequent hooks and the final `Run`.

### When to Use Init

- Configure logging or tracing before parsing
- Load environment-specific configuration
- Set up context values needed during parsing
- Initialize resources that don't depend on flags

## Defaulter

`Default` runs **after parsing, before validation**. Use it for computed defaults that depend on other parsed values.

```go
type ServeCmd struct {
    Port int    `flag:"port" default:"8080"`
    Host string `flag:"host"`
    Addr string // computed
}

func (s *ServeCmd) Default() error {
    if s.Host == "" {
        s.Host = "localhost"
    }
    s.Addr = fmt.Sprintf("%s:%d", s.Host, s.Port)
    return nil
}
```

### When to Use Default

- Compute derived values from parsed flags
- Set conditional defaults based on other flags
- Normalize or transform values before validation
- Fill in values that depend on environment detection

```go
func (s *ServeCmd) Default() error {
    if s.Port == 0 {
        // Use different default port based on environment
        if os.Getenv("ENV") == "production" {
            s.Port = 443
        } else {
            s.Port = 8080
        }
    }
    return nil
}
```

## Validator

`Validate` runs **after defaulting, before Before hooks**. Use it for custom validation beyond what struct tags provide.

```go
type DeployCmd struct {
    Env     string `flag:"env" enum:"dev,staging,prod"`
    Region  string `flag:"region"`
    Confirm bool   `flag:"confirm"`
}

func (d *DeployCmd) Validate() error {
    if d.Env == "prod" && !d.Confirm {
        return errors.New("production deploys require --confirm")
    }
    if d.Env == "prod" && d.Region == "" {
        return errors.New("production deploys require --region")
    }
    return nil
}
```

### When to Use Validate

- Cross-field validation (field A requires field B)
- Business logic validation beyond enum/required
- File existence checks
- Network reachability checks
- Permission verification

### ArgsValidator

For validating positional arguments, implement `ArgsValidator`:

```go
func (c *Cmd) ValidateArgs(args []string) error {
    if len(args) == 0 {
        return errors.New("at least one file required")
    }
    for _, arg := range args {
        if _, err := os.Stat(arg); err != nil {
            return fmt.Errorf("file not found: %s", arg)
        }
    }
    return nil
}
```

`ValidateArgs` runs before `Validate`, receiving the unconsumed positional arguments.

## Beforer

`Before` runs **after validation, before Run**, parent-first. Use it for setup that depends on validated values.

```go
type App struct {
    Verbose bool `flag:"verbose"`
    db      *sql.DB
}

func (a *App) Before(ctx context.Context) (context.Context, error) {
    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        return ctx, err
    }
    a.db = db
    return cli.Set(ctx, "db", db), nil
}
```

The returned context flows to child `Before` hooks and `Run`.

### When to Use Before

- Open database connections
- Initialize HTTP clients
- Start background services
- Set up tracing spans
- Authenticate with external services
- Share resources via context

### Context Propagation

Values set in parent `Before` are available to children:

```go
func (a *App) Before(ctx context.Context) (context.Context, error) {
    db, _ := sql.Open("postgres", connStr)
    return cli.Set(ctx, "db", db), nil
}

func (s *ServeCmd) Run(ctx context.Context) error {
    db := cli.Get[*sql.DB](ctx, "db")
    // use db
    return nil
}
```

### Leaf Inspection

Parent `Before` hooks can inspect the resolved leaf command:

```go
func (a *App) Before(ctx context.Context) (context.Context, error) {
    leaf := cli.Leaf(ctx)

    // Check if leaf requires authentication
    if _, ok := leaf.(Authenticated); ok {
        token, err := authenticate()
        if err != nil {
            return ctx, err
        }
        return cli.Set(ctx, "token", token), nil
    }

    return ctx, nil
}
```

Define marker interfaces for capability-based routing:

```go
// Marker interface for commands requiring auth
type Authenticated interface {
    RequiresAuth()
}

type AdminCmd struct{}

func (a *AdminCmd) RequiresAuth() {} // marker
func (a *AdminCmd) Run(ctx context.Context) error {
    token := cli.Get[string](ctx, "token")
    // use token
    return nil
}
```

## Afterer

`After` runs **after Run**, child-first. It **always runs**, even if `Run` returns an error.

```go
func (a *App) After(ctx context.Context) error {
    if a.db != nil {
        return a.db.Close()
    }
    return nil
}
```

### When to Use After

- Close database connections
- Flush buffers
- Clean up temporary files
- Stop background services
- Report metrics
- Release locks

### Error Handling

`After` receives the context but not the `Run` error. If `Run` failed, `After` still runs for cleanup:

```go
func (s *ServeCmd) After(ctx context.Context) error {
    // Always runs, regardless of Run result
    s.server.Shutdown(ctx)
    return nil
}
```

If both `Run` and `After` return errors, `Run`'s error takes priority.

### Execution Order

For a chain `App → SubCmd → LeafCmd`, `After` runs child-first:

```
LeafCmd.After() → SubCmd.After() → App.After()
```

This mirrors typical cleanup patterns: inner resources before outer.

## Middleware

Implement `Middlewarer` to wrap the `Run` function:

```go
func (c *Cmd) Middleware() []func(cli.RunFunc) cli.RunFunc {
    return []func(cli.RunFunc) cli.RunFunc{
        loggingMiddleware,
        timingMiddleware,
        recoveryMiddleware,
    }
}

func loggingMiddleware(next cli.RunFunc) cli.RunFunc {
    return func(ctx context.Context) error {
        log.Println("Starting command")
        err := next(ctx)
        log.Printf("Command finished: %v", err)
        return err
    }
}

func timingMiddleware(next cli.RunFunc) cli.RunFunc {
    return func(ctx context.Context) error {
        start := time.Now()
        err := next(ctx)
        log.Printf("Duration: %v", time.Since(start))
        return err
    }
}

func recoveryMiddleware(next cli.RunFunc) cli.RunFunc {
    return func(ctx context.Context) error {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("Panic recovered: %v", r)
            }
        }()
        return next(ctx)
    }
}
```

Middleware is applied in order — first wraps outermost:

```
recovery(timing(logging(Run)))
```

### When to Use Middleware

- Logging and tracing
- Timing and metrics
- Panic recovery
- Transaction management
- Rate limiting
- Retry logic

## Complete Example

```go
type App struct {
    Verbose bool   `flag:"verbose" short:"v"`
    Config  string `flag:"config" default:"~/.app/config.yaml"`
    db      *sql.DB
    logger  *slog.Logger
}

func (a *App) Init(ctx context.Context) (context.Context, error) {
    // Early setup before parsing
    handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    a.logger = slog.New(handler)
    return context.WithValue(ctx, "logger", a.logger), nil
}

func (a *App) Default() error {
    // Expand config path after parsing
    if strings.HasPrefix(a.Config, "~/") {
        home, _ := os.UserHomeDir()
        a.Config = filepath.Join(home, a.Config[2:])
    }
    return nil
}

func (a *App) Before(ctx context.Context) (context.Context, error) {
    // Setup after validation
    if a.Verbose {
        a.logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
            Level: slog.LevelDebug,
        }))
    }

    db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
    if err != nil {
        return ctx, fmt.Errorf("database connection failed: %w", err)
    }
    a.db = db

    ctx = cli.Set(ctx, "db", db)
    ctx = cli.Set(ctx, "logger", a.logger)
    return ctx, nil
}

func (a *App) After(ctx context.Context) error {
    // Cleanup
    if a.db != nil {
        a.logger.Debug("closing database connection")
        return a.db.Close()
    }
    return nil
}

type ServeCmd struct {
    Port   int    `flag:"port" default:"8080"`
    Host   string `flag:"host" default:"localhost"`
    server *http.Server
}

func (s *ServeCmd) Validate() error {
    if s.Port < 1 || s.Port > 65535 {
        return fmt.Errorf("invalid port: %d", s.Port)
    }
    return nil
}

func (s *ServeCmd) Middleware() []func(cli.RunFunc) cli.RunFunc {
    return []func(cli.RunFunc) cli.RunFunc{
        func(next cli.RunFunc) cli.RunFunc {
            return func(ctx context.Context) error {
                logger := cli.Get[*slog.Logger](ctx, "logger")
                logger.Info("starting server", "port", s.Port)
                return next(ctx)
            }
        },
    }
}

func (s *ServeCmd) Run(ctx context.Context) error {
    db := cli.Get[*sql.DB](ctx, "db")
    logger := cli.Get[*slog.Logger](ctx, "logger")

    s.server = &http.Server{
        Addr:    fmt.Sprintf("%s:%d", s.Host, s.Port),
        Handler: newHandler(db, logger),
    }

    return s.server.ListenAndServe()
}

func (s *ServeCmd) After(ctx context.Context) error {
    if s.server != nil {
        return s.server.Shutdown(ctx)
    }
    return nil
}
```

## Summary

| Hook | When | Direction | Returns |
|------|------|-----------|---------|
| `Init` | Before parsing | Parent-first | Context, error |
| `Default` | After parsing | Parent-first | error |
| `ValidateArgs` | After Default | Leaf only | error |
| `Validate` | After ValidateArgs | Leaf only | error |
| `Before` | After validation | Parent-first | Context, error |
| `Run` | Main execution | Leaf only | error |
| `After` | After Run | Child-first | error |

## What's Next

- [Dependency Injection](dependency-injection.md) — Share resources across commands
- [Flags](flags.md) — Flag parsing and validation
- [Errors](errors.md) — Error handling patterns
