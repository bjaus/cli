# Help System

The framework provides a flexible help system with multiple output formats, customization hooks, and a pluggable renderer architecture.

## Built-in Help

Every command gets `--help` and `-h` flags automatically:

```
$ app serve --help
Start the HTTP server

Usage:
  app serve [flags]

Flags:
  -p, --port int     Port to listen on (default: 8080)
  -v, --verbose      Enable verbose output

Global Flags:
      --debug        Enable debug mode
```

The special argument `help` also works:

```
$ app help serve
$ app serve help
```

## Help Renderers

The `help` subpackage provides seven built-in renderers:

| Renderer | Description |
|----------|-------------|
| `Default` | Standard CLI help (matches built-in output) |
| `Compact` | Dense, minimal whitespace |
| `Tree` | ASCII command/flag hierarchy |
| `Man` | Man page style with CAPS sections |
| `JSON` | Machine-readable JSON |
| `Markdown` | Documentation-ready Markdown |
| `Template` | Custom Go text/template |

### Using a Renderer

```go
import "github.com/bjaus/cli/help"

cli.Execute(ctx, root, args,
    cli.WithHelpRenderer(help.Compact()),
)
```

### Renderer Options

All renderers accept options for customization:

```go
help.Default(
    help.WithColor(true),      // enable ANSI colors
    help.WithSorted(),         // sort flags/subcommands alphabetically
    help.WithWidth(80),        // set terminal width
)
```

| Option | Description |
|--------|-------------|
| `WithColor(bool)` | Enable/disable ANSI color output |
| `WithColorAuto()` | Auto-detect color support |
| `WithSorted()` | Sort flags and subcommands alphabetically |
| `WithWidth(int)` | Set terminal width (0 = auto-detect) |

### Default Renderer

Matches the built-in help format:

```go
help.Default()
```

```
Start the HTTP server

Usage:
  app serve [flags]

Flags:
  -p, --port int     Port to listen on (default: 8080)
  -v, --verbose      Enable verbose output
```

### Compact Renderer

Dense, minimal whitespace:

```go
help.Compact()
```

```
app serve - Start the HTTP server
Usage: app serve [flags]
Flags:
  -p, --port int   Port (default: 8080)
  -v, --verbose    Verbose output
```

### Tree Renderer

ASCII hierarchy showing commands and flags:

```go
help.Tree()
```

```
app
├── serve       Start the HTTP server
│   ├── --port int     Port (default: 8080)
│   └── --verbose      Verbose output
├── config      Configuration commands
│   ├── get     Get a config value
│   └── set     Set a config value
└── version     Show version
```

### Man Renderer

Man page style with uppercase section headers:

```go
help.Man()
```

```
NAME
    app serve - Start the HTTP server

SYNOPSIS
    app serve [flags]

DESCRIPTION
    Start the HTTP server on the specified port.

OPTIONS
    -p, --port int
        Port to listen on. Default: 8080

    -v, --verbose
        Enable verbose output
```

### JSON Renderer

Machine-readable JSON:

```go
help.JSON()
```

```json
{
  "name": "app serve",
  "description": "Start the HTTP server",
  "usage": ["app serve [flags]"],
  "flags": [
    {
      "name": "port",
      "short": "p",
      "type": "int",
      "default": "8080",
      "help": "Port to listen on"
    }
  ]
}
```

### Markdown Renderer

Documentation-ready Markdown:

```go
help.Markdown()
```

```markdown
# app serve

Start the HTTP server.

## Usage

    app serve [flags]

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-p, --port` | int | 8080 | Port to listen on |
```

### Template Renderer

Complete control with Go text/template:

```go
tmpl := `{{.Name}}
{{.Description}}
{{range .Flags}}- --{{.Name}}: {{.Help}}
{{end}}`

renderer, err := help.Template(tmpl)
```

Available template data (see `help.Data` struct):
- `.Name` — command name/path
- `.Description` — short description
- `.LongDescription` — extended description
- `.Usage` — usage lines
- `.Commands` — subcommands
- `.Flags` — command flags
- `.GlobalFlags` — parent flags
- `.Arguments` — positional arguments
- `.Examples` — usage examples

Template functions:
- `join`, `upper`, `lower`, `title`
- `indent`, `wrap`, `repeat`
- `trimPrefix`, `trimSuffix`
- `contains`, `hasPrefix`, `hasSuffix`
- `replace`, `default`

## Per-Command Help

### Helper Interface

Override help entirely for a single command:

```go
func (c *Cmd) Help() string {
    return `Custom help text here.

This completely replaces the generated help.`
}
```

### HelpPrepender

Add sections before the main help content:

```go
func (c *Cmd) PrependHelp() []cli.HelpSection {
    return []cli.HelpSection{{
        Header: "Notice",
        Body:   "  This command requires VPN access.",
    }}
}
```

Output:

```
Notice:
  This command requires VPN access.

Start the HTTP server
...
```

### HelpAppender

Add sections after the main help content:

```go
func (c *Cmd) AppendHelp() []cli.HelpSection {
    return []cli.HelpSection{{
        Header: "See Also",
        Body:   "  app config - Configure the application\n  app version - Show version info",
    }}
}
```

Output:

```
...
See Also:
  app config - Configure the application
  app version - Show version info
```

### Per-Command Renderer

A command can specify its own renderer:

```go
func (c *Cmd) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
    // Custom rendering logic
    return "Custom help output"
}
```

## Command Metadata

Help output incorporates metadata from optional interfaces:

### Description

```go
func (c *Cmd) Description() string {
    return "Short one-line description"
}
```

### Long Description

```go
func (c *Cmd) LongDescription() string {
    return `Extended description that can span multiple lines.

This appears after the short description in help output.`
}
```

### Examples

```go
func (c *Cmd) Examples() []cli.Example {
    return []cli.Example{
        {Description: "Start on port 9000", Command: "app serve --port 9000"},
        {Description: "Enable verbose mode", Command: "app serve -v"},
    }
}
```

Output:

```
Examples:
  # Start on port 9000
  app serve --port 9000

  # Enable verbose mode
  app serve -v
```

### Aliases

```go
func (c *Cmd) Aliases() []string {
    return []string{"s", "start"}
}
```

### Categories

Group subcommands under headings:

```go
func (c *ServeCmd) Category() string { return "Server" }
func (c *ConfigCmd) Category() string { return "Configuration" }
```

Output:

```
Server:
  serve     Start the HTTP server

Configuration:
  config    Manage configuration
```

### Hidden

Hide a command from help (still functional):

```go
func (c *DebugCmd) Hidden() bool { return true }
```

## Flag Display

### Categories

Group flags under headings:

```go
type Cmd struct {
    Port    int    `flag:"port" category:"Network" help:"Port to listen on"`
    Host    string `flag:"host" category:"Network" help:"Host to bind to"`
    Verbose bool   `flag:"verbose" category:"Logging" help:"Verbose output"`
    Debug   bool   `flag:"debug" category:"Logging" help:"Debug mode"`
}
```

Output:

```
Network:
  -p, --port int     Port to listen on
      --host string  Host to bind to

Logging:
  -v, --verbose      Verbose output
      --debug        Debug mode
```

### Placeholders

Custom value names in help:

```go
type Cmd struct {
    Port int `flag:"port" placeholder:"PORT" help:"Port to listen on"`
}
```

Output:

```
  -p, --port PORT   Port to listen on
```

### Deprecation

Show deprecation warnings:

```go
type Cmd struct {
    OldFlag string `flag:"old-name" deprecated:"use --new-name instead"`
}
```

Output:

```
      --old-name string   (DEPRECATED: use --new-name instead)
```

### Help Text Interpolation

Use placeholders in help text:

```go
type Cmd struct {
    Format string `flag:"format" enum:"json,yaml,text" default:"json" help:"Output format: ${enum} (default: ${default})"`
    Token  string `flag:"token" env:"API_TOKEN" help:"API token (env: ${env})"`
}
```

Placeholders:
- `${default}` — default value
- `${enum}` — allowed values
- `${env}` — environment variable

## Programmatic Access

### Building Help Data

Access structured help data programmatically:

```go
import "github.com/bjaus/cli/help"

data := help.BuildData(cmd, chain, flags, args, globalFlags, sorted)
fmt.Println(data.Name)
fmt.Println(data.Description)
for _, f := range data.Flags {
    fmt.Printf("--%s: %s\n", f.Name, f.Help)
}
```

### Inspecting Commands

```go
flags := cli.ScanFlags(cmd)
args, err := cli.ScanArgs(cmd)
subs, _ := cli.AllSubcommands(cmd)
info := help.ResolveInfo(cmd)
```

## Help Signals

Commands can trigger help display:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if len(c.Args) == 0 {
        return cli.ErrShowHelp  // show help, exit 1
    }
    return nil
}
```

| Signal | Description |
|--------|-------------|
| `ShowHelp` | Full help, exit 0 |
| `ErrShowHelp` | Full help, exit 1 |
| `ShowUsage` | Brief usage line, exit 0 |
| `ErrShowUsage` | Brief usage line, exit 1 |

## Version Flag

Implement `Versioner` for `--version` / `-V`:

```go
func (a *App) Version() string {
    return "1.0.0"
}
```

```
$ app --version
1.0.0
```

## What's Next

- [Completion](completion.md) — Shell completion scripts
- [Errors](errors.md) — Error handling patterns
- [Configuration](configuration.md) — Config file integration
