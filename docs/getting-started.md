# Getting Started

This guide walks you through building your first CLI application.

## Installation

```bash
go get github.com/bjaus/cli
```

## Your First Command

Every command is a Go struct that implements `Commander`:

```go
type Commander interface {
    Run(ctx context.Context) error
}
```

Here's a minimal "hello world":

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/bjaus/cli"
)

type HelloCmd struct{}

func (h *HelloCmd) Run(ctx context.Context) error {
    fmt.Println("Hello, World!")
    return nil
}

func main() {
    cli.ExecuteAndExit(context.Background(), &HelloCmd{}, os.Args)
}
```

```
$ hello
Hello, World!
```

## Adding Flags

Add struct fields with the `flag` tag:

```go
type GreetCmd struct {
    Name   string `flag:"name" short:"n" default:"World" help:"Who to greet"`
    Shout  bool   `flag:"shout" short:"s" help:"Use uppercase"`
}

func (g *GreetCmd) Run(ctx context.Context) error {
    msg := fmt.Sprintf("Hello, %s!", g.Name)
    if g.Shout {
        msg = strings.ToUpper(msg)
    }
    fmt.Println(msg)
    return nil
}
```

```
$ greet
Hello, World!

$ greet --name Alice
Hello, Alice!

$ greet -n Bob -s
HELLO, BOB!
```

The framework automatically:
- Parses `--name value` and `-n value`
- Uses the default when no flag is provided
- Generates help text

## Adding Help

Implement optional interfaces to enhance your command:

```go
func (g *GreetCmd) Name() string        { return "greet" }
func (g *GreetCmd) Description() string { return "Greet someone by name" }
```

Now `--help` shows:

```
$ greet --help
Greet someone by name

Usage:
  greet [flags]

Flags:
  -n, --name string   Who to greet (default: World)
  -s, --shout         Use uppercase
```

## Subcommands

Create a parent command that returns subcommands:

```go
type App struct{}

func (a *App) Name() string        { return "myapp" }
func (a *App) Description() string { return "My CLI application" }
func (a *App) Run(ctx context.Context) error { return nil }

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &GreetCmd{},
        &VersionCmd{},
    }
}

type VersionCmd struct{}

func (v *VersionCmd) Name() string { return "version" }
func (v *VersionCmd) Run(ctx context.Context) error {
    fmt.Println("v1.0.0")
    return nil
}
```

```
$ myapp greet --name Alice
Hello, Alice!

$ myapp version
v1.0.0

$ myapp --help
My CLI application

Usage:
  myapp [command]

Commands:
  greet     Greet someone by name
  version
```

## Version Flag

Implement `Versioner` on your root command to enable `--version`:

```go
func (a *App) Version() string { return "1.0.0" }
```

```
$ myapp --version
1.0.0
```

## Environment Variables

Use the `env` tag to read from environment variables:

```go
type ServeCmd struct {
    Port int    `flag:"port" env:"PORT" default:"8080" help:"Port to listen on"`
    Host string `flag:"host" env:"HOST" default:"localhost"`
}
```

Priority order: explicit flag > environment variable > default value

```
$ export PORT=3000
$ serve
Listening on localhost:3000

$ serve --port 9000
Listening on localhost:9000   # flag wins
```

## Positional Arguments

For commands that need positional arguments, use `cli.Args`:

```go
type CatCmd struct {
    Args cli.Args
}

func (c *CatCmd) Name() string { return "cat" }
func (c *CatCmd) Run(ctx context.Context) error {
    for _, file := range c.Args {
        // read and print file
    }
    return nil
}
```

```
$ cat file1.txt file2.txt
```

Or use struct tags for named positional arguments:

```go
type CopyCmd struct {
    Src string `arg:"src" help:"Source file"`
    Dst string `arg:"dst" help:"Destination file"`
}
```

```
$ copy source.txt dest.txt
```

## Error Handling

Return errors from `Run` to indicate failure:

```go
func (c *CopyCmd) Run(ctx context.Context) error {
    if c.Src == c.Dst {
        return errors.New("source and destination must be different")
    }
    // ...
    return nil
}
```

For custom exit codes, use `cli.Exit`:

```go
return cli.Exit("file not found", 2)
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/bjaus/cli"
)

type App struct{}

func (a *App) Name() string                  { return "todo" }
func (a *App) Description() string           { return "A simple todo list manager" }
func (a *App) Version() string               { return "1.0.0" }
func (a *App) Run(ctx context.Context) error { return nil }
func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{&AddCmd{}, &ListCmd{}}
}

type AddCmd struct {
    Task     string `arg:"task" help:"Task description"`
    Priority string `flag:"priority" short:"p" enum:"low,medium,high" default:"medium"`
}

func (a *AddCmd) Name() string        { return "add" }
func (a *AddCmd) Description() string { return "Add a new task" }
func (a *AddCmd) Run(ctx context.Context) error {
    fmt.Printf("Added: %s (priority: %s)\n", a.Task, a.Priority)
    return nil
}

type ListCmd struct {
    All bool `flag:"all" short:"a" help:"Show completed tasks too"`
}

func (l *ListCmd) Name() string        { return "list" }
func (l *ListCmd) Description() string { return "List all tasks" }
func (l *ListCmd) Run(ctx context.Context) error {
    fmt.Println("Your tasks:")
    fmt.Println("  [ ] Buy groceries")
    fmt.Println("  [ ] Write documentation")
    return nil
}

func main() {
    cli.ExecuteAndExit(context.Background(), &App{}, os.Args)
}
```

## What's Next

- [Flags](flags.md) — All flag types, struct tags, and validation
- [Subcommands](subcommands.md) — Nesting, aliases, categories
- [Lifecycle Hooks](lifecycle.md) — Setup, teardown, validation
- [Dependency Injection](dependency-injection.md) — Sharing resources across commands
