# cli

[![Go Reference](https://pkg.go.dev/badge/github.com/bjaus/cli.svg)](https://pkg.go.dev/github.com/bjaus/cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/bjaus/cli)](https://goreportcard.com/report/github.com/bjaus/cli)
[![CI](https://github.com/bjaus/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/bjaus/cli/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/bjaus/cli/branch/main/graph/badge.svg)](https://codecov.io/gh/bjaus/cli)

A composable CLI framework for Go built on small interfaces.

Commands are Go types that implement `Runner`. The framework discovers capabilities through type assertions on optional interfaces — there is no base struct to embed and no configuration DSL. Think `io.Reader` for CLIs.

## Install

```bash
go get github.com/bjaus/cli
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/bjaus/cli"
)

type GreetCmd struct {
    Name string `flag:"name" short:"n" default:"World" help:"Who to greet"`
}

func (g *GreetCmd) Run(_ context.Context) error {
    fmt.Printf("Hello, %s!\n", g.Name)
    return nil
}

func main() {
    cli.ExecuteAndExit(context.Background(), &GreetCmd{}, os.Args)
}
```

```
$ greet --name Alice
Hello, Alice!
```

## Design Philosophy

- **Small interfaces** — each interface has one method. Implement only what you need.
- **Composition over configuration** — no YAML, no builder chains, no struct embedding. Wire commands with plain Go constructors.
- **Type assertion discovery** — the framework discovers capabilities at runtime. A command that implements `Namer` gets a custom name. One that doesn't gets its struct type name.
- **Everything is replaceable** — flag parsing, help rendering, suggestions. The framework provides defaults but imposes nothing.

## Core Interface

Every command must implement `Runner`:

```go
type Runner interface {
    Run(ctx context.Context) error
}
```

Positional arguments are available via the `Args` field (see [Positional Arguments](#positional-arguments)).

For simple cases, `RunFunc` adapts a plain function:

```go
cmd := cli.RunFunc(func(ctx context.Context) error {
    fmt.Println("Hello!")
    return nil
})
```

## Flags

The default flag parser reads struct tags. Fields tagged with `flag` become CLI flags.

```go
type ServeCmd struct {
    Port    int           `flag:"port" short:"p" default:"8080" help:"Port to listen on" env:"PORT"`
    Host    string        `flag:"host" default:"localhost" help:"Host to bind to"`
    Tags    []string      `flag:"tag" short:"t" help:"Tags to apply"`
    Env     map[string]string `flag:"env" help:"Environment variables as key=value"`
    Format  string        `flag:"format" enum:"text,json,yaml" default:"text" help:"Output format"`
    Verbose int           `flag:"verbose" short:"v" counter:"true" help:"Increase verbosity"`
    Color   bool          `flag:"color" default:"true" negate:"true" help:"Colorize output"`
}
```

### Supported types

`string`, `int`, `int64`, `float64`, `bool`, `time.Duration`, slices of any scalar type, `map[string]string`, and any type implementing `FlagUnmarshaler`.

### Struct tag reference

| Tag | Description |
|-----|-------------|
| `flag` | Flag name (required to register the field) |
| `short` | Single-character short form |
| `default` | Default value if not provided |
| `help` | Description shown in help output |
| `env` | Environment variable fallback |
| `enum` | Comma-separated allowed values |
| `required` | `"true"` to require the flag |
| `counter` | `"true"` to increment an int per occurrence |
| `negate` | `"true"` to add `--no-` prefix for bool flags |
| `alt` | Comma-separated additional long flag names |
| `sep` | Separator for splitting values into slice elements |
| `mask` | Displayed instead of default in help (e.g. `"****"`) |
| `placeholder` | Value label shown in help (e.g. `"PORT"` in `--port PORT`) |
| `prefix` | Flag name prefix for named struct fields (e.g. `"db-"`) |

### Priority

explicit flag > environment variable > default > zero value

### Flags anywhere

Flags can appear before or after subcommand names. Both of these work:

```
myapp --verbose serve --port 8080
myapp serve --verbose --port 8080
```

### Slice flags

Repeat a flag to accumulate values:

```go
Tags []string `flag:"tag" short:"t"`
```

```
$ app --tag v1 --tag latest
```

### Map flags

Pass key=value pairs:

```go
Env map[string]string `flag:"env"`
```

```
$ app --env DB_HOST=localhost --env DB_PORT=5432
```

### Enum validation

Restrict a flag to allowed values. Invalid values produce a clear error automatically.

```go
Format string `flag:"format" enum:"text,json,yaml" default:"text"`
```

```
$ app --format xml
Error: invalid flag value: --format must be one of [text,json,yaml]
```

### Counter flags

An int that increments per occurrence. Classic verbosity pattern:

```go
Verbose int `flag:"verbose" short:"v" counter:"true"`
```

```
$ app -v -v -v    # Verbose = 3
```

### Negatable bools

Add `--no-` prefix to explicitly disable a default-on flag:

```go
Color bool `flag:"color" default:"true" negate:"true"`
```

```
$ app --no-color  # Color = false
```

### Embedded structs

Anonymous embedded structs have their flags promoted:

```go
type OutputFlags struct {
    Format string `flag:"format" enum:"json,table" default:"table"`
}

type ListCmd struct {
    OutputFlags
    Limit int `flag:"limit" default:"50"`
}
```

### Prefix

Named struct fields with `prefix` namespace their flags:

```go
type DBFlags struct {
    Host string `flag:"host" default:"localhost"`
    Port int    `flag:"port" default:"5432"`
}

type ServeCmd struct {
    DB   DBFlags `prefix:"db-"`  // --db-host, --db-port
    Port int     `flag:"port" default:"8080"`
}
```

### Custom flag types

Implement `FlagUnmarshaler` for custom parsing:

```go
type LogLevel int

func (l *LogLevel) UnmarshalFlag(value string) error {
    switch value {
    case "debug":
        *l = 0
    case "info":
        *l = 1
    case "error":
        *l = 2
    default:
        return fmt.Errorf("unknown log level: %s", value)
    }
    return nil
}
```

## Subcommands

Implement `Parent` to declare subcommands:

```go
type App struct{}

func (a *App) Run(_ context.Context) error { return nil }
func (a *App) Name() string                { return "myapp" }
func (a *App) Subcommands() []cli.Runner {
    return []cli.Runner{&ServeCmd{}, &MigrateCmd{}}
}
```

```
$ myapp serve --port 9090
$ myapp migrate up
```

### Prefix matching

Enable unique prefix resolution so users can abbreviate subcommand names:

```go
cli.Execute(ctx, root, args, cli.WithPrefixMatching(true))
```

```
$ myapp ser    # matches "serve" if unambiguous
```

### Fallback command

Implement `Fallbacker` to provide a default subcommand when none is specified:

```go
func (a *App) Fallback() cli.Runner { return &ServeCmd{} }
```

```
$ myapp          # runs ServeCmd automatically
$ myapp serve    # also runs ServeCmd
```

## Discovery Interfaces

All optional. Implement any combination:

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Namer` | `Name() string` | Override command name (default: lowercase struct type) |
| `Describer` | `Description() string` | One-line description for help |
| `Aliaser` | `Aliases() []string` | Alternate names |
| `Parent` | `Subcommands() []Runner` | Declare subcommands |
| `Hider` | `Hidden() bool` | Hide from help output |
| `Exampler` | `Examples() []Example` | Usage examples |
| `Versioner` | `Version() string` | Version string for `--version` |
| `Deprecator` | `Deprecated() string` | Deprecation warning to stderr |
| `Categorizer` | `Category() string` | Group subcommands in help |
| `Fallbacker` | `Fallback() Runner` | Default subcommand |

### Version

Implement `Versioner` on your root command. `--version` or `-V` prints it and exits.

```go
func (a *App) Version() string { return "2.1.0" }
```

```
$ myapp --version
2.1.0
```

### Deprecation

Implement `Deprecator` to warn users. The command still runs, but a warning is printed to stderr.

```go
func (o *OldCmd) Deprecated() string { return "use new-cmd instead" }
```

```
$ myapp old-cmd
Warning: "old-cmd" is deprecated: use new-cmd instead
```

### Categories

Implement `Categorizer` to group subcommands under headings in help output:

```go
func (a *AdminCmd) Category() string { return "Admin Commands" }
```

```
Commands:
  run    Run the app

Admin Commands:
  users  Manage users
```

## Lifecycle Hooks

| Interface | Method | When |
|-----------|--------|------|
| `Beforer` | `Before(ctx) (ctx, error)` | Before Run, parent-first. Returns modified context. |
| `Afterer` | `After(ctx) error` | After Run, child-first. Always runs. |
| `Validator` | `Validate() error` | After flag parsing, before Run. |

```go
func (a *App) Before(ctx context.Context) (context.Context, error) {
    db, err := sql.Open("postgres", a.DSN)
    if err != nil {
        return ctx, err
    }
    return context.WithValue(ctx, dbKey, db), nil
}

func (a *App) After(_ context.Context) error {
    return a.db.Close()
}
```

## Middleware

Implement `Middlewarer` to wrap the run function:

```go
func (c *Cmd) Middleware() []func(next cli.RunFunc) cli.RunFunc {
    return []func(next cli.RunFunc) cli.RunFunc{
        func(next cli.RunFunc) cli.RunFunc {
            return func(ctx context.Context) error {
                start := time.Now()
                err := next(ctx)
                log.Printf("took %s", time.Since(start))
                return err
            }
        },
    }
}
```

## Short Option Combining

Enable POSIX-style short option combining globally:

```go
cli.Execute(ctx, root, args, cli.WithShortOptionHandling(true))
```

```
$ app -alr    # expands to -a -l -r
$ app -vvv    # works with counters too
```

All flags except the last must be bool or counter (no value). The last flag may take a value from the next argument.

## Positional Arguments

Commands that need positional arguments declare an `Args` field:

```go
type CopyCmd struct {
    Args    cli.Args
    Verbose bool `flag:"verbose" short:"v"`
}

func (c *CopyCmd) Run(_ context.Context) error {
    for _, file := range c.Args {
        fmt.Println("copying", file)
    }
    return nil
}
```

```
$ copy -v file1.txt file2.txt
copying file1.txt
copying file2.txt
```

## Dependency Injection with Bind

Use `cli.Bind` to inject dependencies into commands by type:

```go
func main() {
    db := openDB()
    cache := redis.New()

    cli.ExecuteAndExit(ctx, &App{}, os.Args,
        cli.Bind(db),                        // inject *sql.DB
        cli.BindTo(cache, (*Cache)(nil)),    // inject as interface
    )
}

type ServeCmd struct {
    DB    *sql.DB  // injected by type match
    Cache Cache    // injected as interface
    Port  int      `flag:"port" default:"8080"`
}

func (s *ServeCmd) Run(_ context.Context) error {
    // s.DB and s.Cache are ready to use
    return nil
}
```

Fields with `flag:`, `arg:`, or `env:` tags are not eligible for injection.

## Error Handling

`Execute` returns errors. `ExecuteAndExit` wraps it with `os.Exit`:

```go
// In main — pass os.Args directly, the framework strips the program name:
cli.ExecuteAndExit(ctx, root, os.Args)

// Or handle errors yourself:
if err := cli.Execute(ctx, root, os.Args); err != nil {
    log.Fatal(err)
}
```

Return `cli.Exit` for custom exit codes:

```go
return cli.Exit("port already in use", 2)
```

## Extensibility

Every major subsystem is replaceable per-command or globally:

| Interface | Method | Scope |
|-----------|--------|-------|
| `Helper` | `Help() string` | Per-command help override |
| `FlagParser` | `ParseFlags(cmd, args) (remaining, error)` | Custom flag parsing |
| `HelpRenderer` | `RenderHelp(cmd, chain, flags, args, globalFlags) string` | Custom help rendering |
| `Suggester` | `Suggest(name) string` | Custom "did you mean?" |

Global overrides via options:

```go
cli.Execute(ctx, root, args,
    cli.WithFlagParser(myParser),
    cli.WithHelpRenderer(myRenderer),
    cli.WithSuggest(false),
)
```

## Manual Dependency Injection

Beyond `cli.Bind`, two manual patterns work well:

**Constructor wiring** — parent builds children with dependencies:

```go
func (a *App) Subcommands() []cli.Runner {
    return []cli.Runner{&ServeCmd{DB: a.db, Logger: a.logger}}
}
```

**Context enrichment** — cross-cutting concerns flow through context via `Beforer`:

```go
func (a *App) Before(ctx context.Context) (context.Context, error) {
    return context.WithValue(ctx, loggerKey, a.logger), nil
}
```

## Execution Pipeline

```
1. Resolve command tree (flags-anywhere aware)
2. Check --version
3. Check --help
4. Parse flags for each command in chain
5. Validate (Validator interface on leaf)
6. Print deprecation warnings
7. Before hooks (parent-first, context flows forward)
8. Run with middleware
9. After hooks (child-first, always runs)
```

## Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithStdout(w)` | `os.Stdout` | Standard output writer |
| `WithStderr(w)` | `os.Stderr` | Standard error writer |
| `WithFlagParser(p)` | struct tags | Global flag parser |
| `WithHelpRenderer(r)` | built-in | Global help renderer |
| `WithSuggest(bool)` | `true` | "Did you mean?" suggestions |
| `WithShortOptionHandling(bool)` | `false` | POSIX short option combining |
| `WithPrefixMatching(bool)` | `false` | Unique prefix subcommand matching |
| `Bind(v)` | — | Inject dependency by concrete type |
| `BindTo(v, iface)` | — | Inject dependency as interface type |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[MIT](LICENSE)
