# Advanced Patterns

This guide covers advanced usage patterns including custom types, testing strategies, and architectural patterns.

## Custom Flag Types

### Value Interface

Implement `flag.Value` for custom parsing:

```go
type LogLevel int

const (
    LevelDebug LogLevel = iota
    LevelInfo
    LevelWarn
    LevelError
)

func (l *LogLevel) String() string {
    return [...]string{"debug", "info", "warn", "error"}[*l]
}

func (l *LogLevel) Set(s string) error {
    switch strings.ToLower(s) {
    case "debug":
        *l = LevelDebug
    case "info":
        *l = LevelInfo
    case "warn":
        *l = LevelWarn
    case "error":
        *l = LevelError
    default:
        return fmt.Errorf("invalid log level: %s", s)
    }
    return nil
}

type Cmd struct {
    Level LogLevel `flag:"level" default:"info" help:"Log level"`
}
```

### Slice Types

Custom slice parsing:

```go
type Ports []int

func (p *Ports) String() string {
    strs := make([]string, len(*p))
    for i, port := range *p {
        strs[i] = strconv.Itoa(port)
    }
    return strings.Join(strs, ",")
}

func (p *Ports) Set(s string) error {
    for _, part := range strings.Split(s, ",") {
        port, err := strconv.Atoi(strings.TrimSpace(part))
        if err != nil {
            return err
        }
        *p = append(*p, port)
    }
    return nil
}

type Cmd struct {
    Ports Ports `flag:"ports" help:"Ports to scan (comma-separated)"`
}
```

Usage: `--ports 80,443,8080`

### Map Types

Parse key-value pairs:

```go
type Labels map[string]string

func (l *Labels) String() string {
    var pairs []string
    for k, v := range *l {
        pairs = append(pairs, k+"="+v)
    }
    return strings.Join(pairs, ",")
}

func (l *Labels) Set(s string) error {
    if *l == nil {
        *l = make(map[string]string)
    }
    for _, pair := range strings.Split(s, ",") {
        k, v, ok := strings.Cut(pair, "=")
        if !ok {
            return fmt.Errorf("invalid label: %s", pair)
        }
        (*l)[k] = v
    }
    return nil
}

type Cmd struct {
    Labels Labels `flag:"label" help:"Resource labels (key=value)"`
}
```

Usage: `--label env=prod,team=platform`

## Testing Commands

### Unit Testing

Test commands in isolation:

```go
func TestServeCmd(t *testing.T) {
    cmd := &ServeCmd{}

    err := cli.Execute(context.Background(), cmd, []string{
        "--port", "9000",
        "--host", "localhost",
    })

    assert.NoError(t, err)
    assert.Equal(t, 9000, cmd.Port)
    assert.Equal(t, "localhost", cmd.Host)
}
```

### Testing with Dependencies

Inject mocks via bindings:

```go
func TestDeployCmd(t *testing.T) {
    mockDeployer := &MockDeployer{}

    cmd := &DeployCmd{}
    err := cli.Execute(context.Background(), cmd, []string{"production"},
        cli.WithBindings(
            bind.Interface(mockDeployer, (*Deployer)(nil)),
        ),
    )

    assert.NoError(t, err)
    assert.True(t, mockDeployer.DeployCalled)
    assert.Equal(t, "production", mockDeployer.Target)
}
```

### Testing Validation

Verify validation errors:

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        wantErr error
    }{
        {
            name:    "missing required flag",
            args:    []string{},
            wantErr: cli.ErrFlagRequired,
        },
        {
            name:    "invalid port",
            args:    []string{"--port", "invalid"},
            wantErr: cli.ErrFlagParse,
        },
        {
            name:    "port out of range",
            args:    []string{"--port", "99999"},
            wantErr: nil, // caught by Validate()
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := cli.Execute(context.Background(), &Cmd{}, tt.args)
            if tt.wantErr != nil {
                assert.True(t, errors.Is(err, tt.wantErr))
            }
        })
    }
}
```

### Testing Help Output

Verify help contains expected content:

```go
func TestHelpOutput(t *testing.T) {
    var buf bytes.Buffer
    cmd := &App{}

    cli.Execute(context.Background(), cmd, []string{"--help"},
        cli.WithStdout(&buf),
    )

    output := buf.String()
    assert.Contains(t, output, "Usage:")
    assert.Contains(t, output, "--port")
    assert.Contains(t, output, "serve")
}
```

### Integration Testing

Test full command chains:

```go
func TestFullWorkflow(t *testing.T) {
    // Setup test environment
    tmpDir := t.TempDir()

    // Run init command
    err := cli.Execute(context.Background(), &App{}, []string{
        "init", "--dir", tmpDir,
    })
    require.NoError(t, err)

    // Verify files created
    assert.FileExists(t, filepath.Join(tmpDir, "config.yaml"))

    // Run build command
    err = cli.Execute(context.Background(), &App{}, []string{
        "build", "--dir", tmpDir,
    })
    require.NoError(t, err)
}
```

## Context Patterns

### Passing Values via Context

Store and retrieve values from context:

```go
type contextKey string

const userKey contextKey = "user"

func (a *App) Before(ctx context.Context) (context.Context, error) {
    user, err := authenticate(ctx)
    if err != nil {
        return ctx, err
    }
    return context.WithValue(ctx, userKey, user), nil
}

func (c *Cmd) Run(ctx context.Context) error {
    user := ctx.Value(userKey).(*User)
    fmt.Printf("Running as %s\n", user.Name)
    return nil
}
```

### Cancellation

Handle context cancellation:

```go
func (c *Cmd) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := processNext(); err != nil {
                return err
            }
        }
    }
}
```

### Timeouts

Apply timeouts in Before hooks:

```go
func (c *Cmd) Before(ctx context.Context) (context.Context, error) {
    if c.Timeout > 0 {
        ctx, _ = context.WithTimeout(ctx, c.Timeout)
    }
    return ctx, nil
}
```

## Embedding Patterns

### Shared Flag Groups

Embed common flags across commands:

```go
type GlobalFlags struct {
    Verbose bool   `flag:"verbose" short:"v" help:"Verbose output"`
    Config  string `flag:"config" short:"c" help:"Config file path"`
    Format  string `flag:"format" enum:"json,yaml,text" default:"text"`
}

type ServeCmd struct {
    GlobalFlags
    Port int `flag:"port" default:"8080"`
}

type DeployCmd struct {
    GlobalFlags
    Target string `arg:"target" required:""`
}
```

### Behavior Mixins

Embed structs that implement interfaces:

```go
type Debuggable struct {
    Debug bool `flag:"debug" help:"Enable debug mode"`
}

func (d *Debuggable) Before(ctx context.Context) (context.Context, error) {
    if d.Debug {
        slog.SetLogLoggerLevel(slog.LevelDebug)
    }
    return ctx, nil
}

type Cmd struct {
    Debuggable // inherits --debug flag and Before hook
    Port int   `flag:"port"`
}
```

### Conditional Embedding

Use interfaces to conditionally include behavior:

```go
type App struct {
    // Commands embedded directly
    Serve  ServeCmd  `cmd:"serve"`
    Deploy DeployCmd `cmd:"deploy"`
}

func (a *App) Subcommands() []cli.Commander {
    cmds := []cli.Commander{&a.Serve, &a.Deploy}

    // Add admin commands if enabled
    if os.Getenv("ADMIN_MODE") == "true" {
        cmds = append(cmds, &AdminCmd{})
    }

    return cmds
}
```

## Multi-Binary Applications

### Shared Core

Create a shared command library:

```go
// pkg/commands/serve.go
package commands

type ServeCmd struct {
    Port int `flag:"port" default:"8080"`
}

func (s *ServeCmd) Name() string { return "serve" }
func (s *ServeCmd) Run(ctx context.Context) error {
    // shared implementation
}
```

### Per-Binary Entry Points

```go
// cmd/myapp/main.go
package main

import "myapp/pkg/commands"

type App struct{}

func (a *App) Name() string { return "myapp" }
func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &commands.ServeCmd{},
        &commands.DeployCmd{},
    }
}

func main() {
    cli.ExecuteAndExit(ctx, &App{}, os.Args)
}
```

```go
// cmd/myapp-lite/main.go
package main

import "myapp/pkg/commands"

type LiteApp struct{}

func (a *LiteApp) Name() string { return "myapp-lite" }
func (a *LiteApp) Subcommands() []cli.Commander {
    return []cli.Commander{
        &commands.ServeCmd{}, // only serve, no deploy
    }
}
```

## Performance Patterns

### Lazy Initialization

Defer expensive setup until needed:

```go
type Cmd struct {
    db     *sql.DB
    dbOnce sync.Once
    DBHost string `flag:"db-host" required:""`
}

func (c *Cmd) getDB() (*sql.DB, error) {
    var err error
    c.dbOnce.Do(func() {
        c.db, err = sql.Open("postgres", c.DBHost)
    })
    return c.db, err
}

func (c *Cmd) Run(ctx context.Context) error {
    // DB only connected if actually used
    if needsDB {
        db, err := c.getDB()
        if err != nil {
            return err
        }
        // use db
    }
    return nil
}
```

### Parallel Initialization

Initialize independent resources concurrently:

```go
func (a *App) Before(ctx context.Context) (context.Context, error) {
    var (
        db    *sql.DB
        cache *redis.Client
        wg    sync.WaitGroup
        errs  = make(chan error, 2)
    )

    wg.Add(2)
    go func() {
        defer wg.Done()
        var err error
        db, err = sql.Open("postgres", a.DBURL)
        if err != nil {
            errs <- err
        }
    }()

    go func() {
        defer wg.Done()
        var err error
        cache, err = redis.Connect(a.RedisURL)
        if err != nil {
            errs <- err
        }
    }()

    wg.Wait()
    close(errs)

    for err := range errs {
        if err != nil {
            return ctx, err
        }
    }

    ctx = context.WithValue(ctx, "db", db)
    ctx = context.WithValue(ctx, "cache", cache)
    return ctx, nil
}
```

## Completion Caching

Cache expensive completion results:

```go
var (
    envCache     []string
    envCacheOnce sync.Once
)

func (d *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
    envCacheOnce.Do(func() {
        envCache, _ = fetchEnvironments()
    })
    return cli.Completions(envCache...)
}
```

## Dynamic Command Registration

Register commands at runtime:

```go
var registry = make(map[string]func() cli.Commander)

func Register(name string, factory func() cli.Commander) {
    registry[name] = factory
}

func (a *App) Subcommands() []cli.Commander {
    var cmds []cli.Commander
    for _, factory := range registry {
        cmds = append(cmds, factory())
    }
    return cmds
}

// In plugin packages:
func init() {
    Register("custom", func() cli.Commander {
        return &CustomCmd{}
    })
}
```

## What's Next

- [Plugins](plugins.md) — External command discovery
- [Dependency Injection](dependency-injection.md) — Resource sharing
- [Lifecycle](lifecycle.md) — Hook execution order
