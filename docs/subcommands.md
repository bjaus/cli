# Subcommands

Subcommands create hierarchical command structures like `app serve start` or `git remote add`. The framework supports three ways to define subcommands: embedded struct fields, the `Subcommander` interface, and runtime discovery via `Discoverer`.

## Basic Subcommands

### Embedded Fields

Named struct fields implementing `Commander` automatically become subcommands:

```go
type App struct {
    Verbose bool      `flag:"verbose" short:"v"`
    Serve   ServeCmd  // subcommand
    Config  ConfigCmd // subcommand
}

func (a *App) Run(ctx context.Context) error { return nil }

type ServeCmd struct {
    Port int `flag:"port" default:"8080"`
}

func (s *ServeCmd) Name() string { return "serve" }
func (s *ServeCmd) Run(ctx context.Context) error {
    fmt.Printf("Serving on port %d\n", s.Port)
    return nil
}

type ConfigCmd struct {
    Path string `flag:"path" default:"~/.config/app"`
}

func (c *ConfigCmd) Name() string { return "config" }
func (c *ConfigCmd) Run(ctx context.Context) error {
    fmt.Printf("Config at %s\n", c.Path)
    return nil
}
```

```
$ app serve --port 9000
Serving on port 9000

$ app config --path /etc/app
Config at /etc/app

$ app --verbose serve --port 8080
Serving on port 8080
```

### Subcommander Interface

Implement `Subcommander` to return subcommands dynamically:

```go
type App struct{}

func (a *App) Run(ctx context.Context) error { return nil }

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &ServeCmd{},
        &ConfigCmd{},
        &VersionCmd{},
    }
}
```

### Combining Both

Embedded fields and `Subcommander` can be used together. Name collisions between them return an error at parse time:

```go
type App struct {
    Serve ServeCmd  // embedded subcommand
}

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &ConfigCmd{},  // OK: different name
        // &ServeCmd{}, // ERROR: would collide with embedded Serve
    }
}
```

## Command Metadata

### Name

Override the default name with `Namer`:

```go
func (s *ServeCmd) Name() string { return "serve" }
```

Without `Namer`, the name is derived from the type: `ServeCmd` → `servecmd`, `*MyCommand` → `mycommand`.

### Description

Provide a one-line description for help output:

```go
func (s *ServeCmd) Description() string {
    return "Start the HTTP server"
}
```

### Long Description

Provide extended description shown in detailed help:

```go
func (s *ServeCmd) LongDescription() string {
    return `Start the HTTP server on the specified port.

The server supports graceful shutdown on SIGINT/SIGTERM
and hot-reloading of configuration files.`
}
```

### Aliases

Declare alternate names:

```go
func (s *ServeCmd) Aliases() []string {
    return []string{"s", "start", "run"}
}
```

```
$ app serve --port 8080
$ app s --port 8080      # alias
$ app start --port 8080  # alias
$ app run --port 8080    # alias
```

### Categories

Group subcommands under headings in help output:

```go
func (s *ServeCmd) Category() string { return "Server" }
func (c *ConfigCmd) Category() string { return "Configuration" }
```

Help output:

```
Server:
  serve    Start the HTTP server

Configuration:
  config   Manage configuration files
```

### Hidden Commands

Hide a command from help while keeping it functional:

```go
func (d *DebugCmd) Hidden() bool { return true }
```

Hidden commands still work when invoked directly.

## Nested Subcommands

Subcommands can have their own subcommands:

```go
type App struct {
    Config ConfigCmd
}

type ConfigCmd struct {
    Get GetConfigCmd
    Set SetConfigCmd
}

func (c *ConfigCmd) Name() string { return "config" }
func (c *ConfigCmd) Run(ctx context.Context) error { return nil }

type GetConfigCmd struct {
    Key string `arg:"key" help:"Configuration key"`
}

func (g *GetConfigCmd) Name() string { return "get" }
func (g *GetConfigCmd) Run(ctx context.Context) error {
    fmt.Printf("Getting %s\n", g.Key)
    return nil
}
```

```
$ app config get database.host
Getting database.host
```

## Flag Inheritance

Flags flow from parent commands to child subcommands when they share the same name and type:

```go
type App struct {
    Verbose bool `flag:"verbose" short:"v" help:"Verbose output"`
}

type ServeCmd struct {
    Verbose bool `flag:"verbose" help:"Verbose output"`  // inherits from parent
    Port    int  `flag:"port" default:"8080"`
}
```

```
$ app --verbose serve --port 8080
# ServeCmd.Verbose = true (inherited)
```

See [Flags](flags.md#flag-inheritance) for details.

## Fallback Commands

When no subcommand matches, a fallback can run instead:

```go
type App struct {
    Args cli.Args
}

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{&HelpCmd{}, &VersionCmd{}}
}

func (a *App) Fallback() cli.Commander {
    return &RunCmd{}
}

type RunCmd struct{}

func (r *RunCmd) Name() string { return "run" }
func (r *RunCmd) Run(ctx context.Context) error {
    fmt.Println("Running default action...")
    return nil
}
```

```
$ app help     # runs HelpCmd
$ app version  # runs VersionCmd
$ app          # runs RunCmd (fallback)
$ app unknown  # runs RunCmd with "unknown" as arg
```

### Built-in Help Fallback

The framework has a special case: when `help` is passed as an argument and no matching subcommand exists, it shows help automatically:

```
$ app help          # shows help (if no "help" subcommand)
$ app serve help    # shows help for serve
```

## Plugin Discovery

Implement `Discoverer` to add subcommands at runtime:

```go
func (a *App) Discover() ([]cli.Commander, error) {
    return cli.Discover(
        cli.WithDirs(cli.DefaultDirs("myapp")...),
        cli.WithPATH("myapp"),
    )
}
```

See [Plugins](plugins.md) for full documentation.

### Priority

When names collide:

1. **Embedded fields** — highest priority
2. **Subcommander** — collisions with embedded fields return error
3. **Discoverer** — silently yields to built-in commands

## Branching Arguments

Commands with both positional arguments and subcommands "branch": arguments are consumed first, then the subcommand resolves:

```go
type UserCmd struct {
    ID     int       `arg:"id" help:"User ID"`
    Delete DeleteCmd
    Rename RenameCmd
}
```

```
$ app user 42 delete --force
$ app user 42 rename newname
```

Subcommands access parent arguments via context:

```go
func (d *DeleteCmd) Run(ctx context.Context) error {
    userID := cli.Get[int](ctx, "id")
    return deleteUser(userID)
}
```

See [Arguments](arguments.md#branching-arguments) for details.

## Listing Subcommands

Get all subcommands (embedded + Subcommander + Discoverer):

```go
subs, err := cli.AllSubcommands(cmd)
if err != nil {
    return err
}
for _, sub := range subs {
    info := sub.(cli.Namer)
    fmt.Println(info.Name())
}
```

## Help Output

```
My CLI Application

Usage:
  app [command]

Commands:
  serve     Start the HTTP server
  config    Manage configuration

Flags:
  -v, --verbose   Verbose output
  -h, --help      Show help

Use "app [command] --help" for more information about a command.
```

## Examples

### Simple App

```go
type App struct {
    Verbose bool `flag:"verbose" short:"v"`
}

func (a *App) Name() string        { return "myapp" }
func (a *App) Description() string { return "My application" }
func (a *App) Version() string     { return "1.0.0" }
func (a *App) Run(ctx context.Context) error { return nil }

func (a *App) Subcommands() []cli.Commander {
    return []cli.Commander{
        &ServeCmd{},
        &ConfigCmd{},
    }
}
```

### Resource-Style Commands

```go
type App struct {
    User    UserCmd
    Project ProjectCmd
}

type UserCmd struct {
    ID     int `arg:"id"`
    Get    UserGetCmd
    Update UserUpdateCmd
    Delete UserDeleteCmd
}

type ProjectCmd struct {
    Name   string `arg:"name"`
    Create ProjectCreateCmd
    List   ProjectListCmd
}
```

```
$ app user 123 get
$ app user 123 update --email new@example.com
$ app project myproj create
$ app project myproj list
```

### Git-Style Commands

```go
type Git struct {
    Remote RemoteCmd
    Branch BranchCmd
    Tag    TagCmd
}

type RemoteCmd struct {
    Add    RemoteAddCmd
    Remove RemoteRemoveCmd
    List   RemoteListCmd
}
```

```
$ git remote add origin https://...
$ git remote remove origin
$ git remote list
```

## What's Next

- [Lifecycle](lifecycle.md) — Setup and teardown hooks
- [Plugins](plugins.md) — External plugin discovery
- [Flags](flags.md) — Flag inheritance across commands
