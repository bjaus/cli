# cli

[![Go Reference](https://pkg.go.dev/badge/github.com/bjaus/cli.svg)](https://pkg.go.dev/github.com/bjaus/cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/bjaus/cli)](https://goreportcard.com/report/github.com/bjaus/cli)
[![CI](https://github.com/bjaus/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/bjaus/cli/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/bjaus/cli/branch/main/graph/badge.svg)](https://codecov.io/gh/bjaus/cli)

**Small interfaces. Big CLIs.**

Commands are structs. Capabilities are interfaces. The framework discovers what your command can do through type assertion — no base types to embed, no configuration DSL. Just implement the interfaces you need.

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

$ greet --help
Usage:
  greet [flags]

Flags:
  -n, --name string   Who to greet (default: World)
  -h, --help          Show help
```

## Design

Commands implement `Commander`:

```go
type Commander interface {
    Run(ctx context.Context) error
}
```

Everything else is opt-in through interfaces:

```go
// Naming
type Namer interface { Name() string }
type Descriptor interface { Description() string }
type Aliaser interface { Aliases() []string }

// Structure
type Subcommander interface { Subcommands() []Commander }
type Discoverer interface { Discover() ([]Commander, error) }

// Lifecycle
type Beforer interface { Before(context.Context) (context.Context, error) }
type Afterer interface { After(context.Context) error }
type Validator interface { Validate() error }

// ... and 20+ more
```

The framework discovers capabilities at runtime. A command that implements `Namer` gets a custom name. One that doesn't uses its struct type name. There's no magic — just Go interfaces.

## Features

| Feature | Description |
|---------|-------------|
| **Struct-based flags** | `flag:"port"` tags with defaults, env vars, enums, and validation |
| **Positional arguments** | Named args with `arg:"target"`, variadic support, built-in validators |
| **Subcommands** | Nested command trees via `Subcommander` interface |
| **Lifecycle hooks** | `Before`, `After`, `Validate`, `Init`, `Default` |
| **Dependency injection** | Type-safe injection via [bind](https://github.com/bjaus/bind) |
| **Shell completion** | Bash, Zsh, Fish, PowerShell with dynamic completions |
| **Plugin discovery** | External commands from directories or PATH |
| **Configuration** | Load from JSON, YAML, env files, or remote sources |
| **Help renderers** | Default, Compact, Tree, Man, JSON, Markdown formats |

## Example: Subcommands

```go
type App struct {
    Verbose bool `flag:"verbose" short:"v" help:"Verbose output"`
}

func (a *App) Name() string        { return "myapp" }
func (a *App) Description() string { return "My application" }

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{&ServeCmd{}, &DeployCmd{}}
}

func (a *App) Run(ctx context.Context) error {
    return cli.ErrShowHelp // show help when no subcommand given
}

type ServeCmd struct {
    Port int `flag:"port" short:"p" default:"8080" env:"PORT"`
}

func (s *ServeCmd) Name() string { return "serve" }
func (s *ServeCmd) Run(ctx context.Context) error {
    fmt.Printf("Listening on :%d\n", s.Port)
    return nil
}
```

```
$ myapp serve --port 9000
Listening on :9000

$ PORT=3000 myapp serve
Listening on :3000
```

## Example: Dependency Injection

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
    Port int     `flag:"port" default:"8080"`
}

func (s *ServeCmd) Run(ctx context.Context) error {
    // s.DB is ready to use
    return nil
}
```

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting Started](docs/getting-started.md) | Installation and first command |
| [Flags](docs/flags.md) | All 19+ struct tags, types, and patterns |
| [Arguments](docs/arguments.md) | Positional args and validators |
| [Subcommands](docs/subcommands.md) | Command hierarchies and embedding |
| [Lifecycle](docs/lifecycle.md) | Init, Validate, Before, After hooks |
| [Dependency Injection](docs/dependency-injection.md) | Type-safe resource sharing |
| [Configuration](docs/configuration.md) | Config files and remote sources |
| [Completion](docs/completion.md) | Shell completion for all shells |
| [Help](docs/help.md) | Customizing help output |
| [Plugins](docs/plugins.md) | External command discovery |
| [Errors](docs/errors.md) | Error handling and exit codes |
| [Advanced](docs/advanced.md) | Custom types, testing, patterns |

## Value Priority

Values are resolved in order (highest to lowest):

1. CLI argument — `--port 8080`
2. Environment variable — `PORT=8080`
3. Config resolver — from config file
4. Default tag — `default:"8080"`
5. Zero value — `0`

## License

[MIT](LICENSE)
