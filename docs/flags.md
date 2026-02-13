# Flags

Flags are declared as struct fields with the `flag` tag. The framework parses command-line arguments, environment variables, configuration files, and defaults into your struct automatically.

## Basic Usage

```go
type ServeCmd struct {
    Port int    `flag:"port" help:"Port to listen on"`
    Host string `flag:"host" default:"localhost" help:"Host to bind to"`
}
```

```
$ serve --port 8080 --host 0.0.0.0
$ serve -p 8080    # short form (see short tag)
```

## Flag Name Resolution

If the `flag` tag value is empty, the name is derived from the field name using CamelCase to kebab-case conversion:

| Field Name | Flag Name |
|------------|-----------|
| `Port` | `--port` |
| `OutputFormat` | `--output-format` |
| `HTTPHost` | `--http-host` |
| `ID` | `--id` |

```go
type Cmd struct {
    OutputFormat string `flag:"" help:"Output format"`  // --output-format
    Port         int    `flag:"port"`                   // --port (explicit)
}
```

## Supported Types

The framework supports these types natively:

| Type | Example Value | Notes |
|------|---------------|-------|
| `string` | `hello` | |
| `int`, `int64` | `42` | |
| `uint`, `uint64` | `100` | |
| `float64` | `3.14` | |
| `bool` | `true`, `false`, `1`, `0` | Boolean flags don't require a value |
| `time.Duration` | `5m`, `1h30m`, `500ms` | Go duration syntax |
| `time.Time` | `2024-01-15`, `2024-01-15T10:30:00Z` | RFC3339, date, or datetime |
| `*url.URL` | `https://example.com/path` | Parsed URL |
| `net.IP` | `192.168.1.1`, `::1` | IPv4 or IPv6 |
| `[]T` | (repeatable) | Slice of any scalar type |
| `map[K]V` | `key=value` | Map with string keys/values |

### Slice Flags

Slice flags can be repeated:

```go
type Cmd struct {
    Tags []string `flag:"tag" short:"t" help:"Tags to apply (repeatable)"`
}
```

```
$ cmd --tag foo --tag bar -t baz
# Tags = ["foo", "bar", "baz"]
```

Or use `sep` to split a single value:

```go
type Cmd struct {
    Tags []string `flag:"tags" sep:"," help:"Comma-separated tags"`
}
```

```
$ cmd --tags foo,bar,baz
# Tags = ["foo", "bar", "baz"]
```

### Map Flags

Map flags use `key=value` syntax:

```go
type Cmd struct {
    Env map[string]string `flag:"env" short:"e" help:"Environment variables"`
}
```

```
$ cmd --env FOO=bar --env BAZ=qux
# Env = {"FOO": "bar", "BAZ": "qux"}
```

## Struct Tags Reference

### Core Tags

| Tag | Type | Description |
|-----|------|-------------|
| `flag` | string | Flag name. If empty, derived from field name. |
| `short` | string | Single-character short form (e.g., `-p` for `--port`) |
| `help` | string | Description shown in help output |
| `default` | string | Default value when not provided |
| `env` | string | Environment variable name(s), comma-separated |
| `required` | presence | Flag must be provided (mutually exclusive with `default`) |

### Validation Tags

| Tag | Type | Description |
|-----|------|-------------|
| `enum` | string | Comma-separated list of allowed values |

```go
type Cmd struct {
    Format string `flag:"format" enum:"json,yaml,text" default:"text" help:"Output format"`
    Level  string `flag:"level" enum:"debug,info,warn,error" required:"" help:"Log level"`
}
```

### Boolean Flag Tags

| Tag | Type | Applicable To | Description |
|-----|------|---------------|-------------|
| `counter` | presence | `int`, `uint` | Increment on each occurrence (`-vvv` → 3) |
| `negate` | presence | `bool` | Add `--no-` prefix to set false |

```go
type Cmd struct {
    Verbose int  `flag:"verbose" short:"v" counter:"" help:"Increase verbosity"`
    Color   bool `flag:"color" default:"true" negate:"" help:"Colorize output"`
}
```

```
$ cmd -vvv           # Verbose = 3
$ cmd --no-color     # Color = false
```

### Display Tags

| Tag | Type | Description |
|-----|------|-------------|
| `hidden` | presence | Hide from help output (still functional) |
| `category` | string | Group heading in help output |
| `placeholder` | string | Value name in help (e.g., `--port PORT`) |
| `mask` | string | Display instead of default in help (e.g., `****` for secrets) |
| `deprecated` | string | Warning message when flag is used |

```go
type Cmd struct {
    Token  string `flag:"token" env:"API_TOKEN" mask:"****" help:"API token"`
    Debug  bool   `flag:"debug" hidden:"" help:"Enable debug mode"`
    Output string `flag:"output" placeholder:"FILE" help:"Output file"`
    Old    string `flag:"old-name" deprecated:"use --new-name instead"`
}
```

### Alternative Names

| Tag | Type | Description |
|-----|------|-------------|
| `alt` | string | Comma-separated additional long flag names |

```go
type Cmd struct {
    Output string `flag:"output" alt:"out,o" help:"Output file"`
}
```

```
$ cmd --output file.txt
$ cmd --out file.txt     # same
$ cmd --o file.txt       # same
```

### Separator Tag

| Tag | Type | Applicable To | Description |
|-----|------|---------------|-------------|
| `sep` | string | slices | Split single value into elements |

```go
type Cmd struct {
    Hosts []string `flag:"hosts" sep:"," help:"Comma-separated hosts"`
    Ports []int    `flag:"ports" sep:":" help:"Colon-separated ports"`
}
```

## Value Priority

Values are resolved in this order (highest priority first):

1. **CLI argument** — `--port 8080`
2. **Environment variable** — `PORT=8080`
3. **Config resolver** — from config file or external source
4. **Default** — `default:"8080"`
5. **Zero value** — `0` for int, `""` for string, etc.

```go
type Cmd struct {
    Port int `flag:"port" env:"PORT" default:"8080" help:"Port to listen on"`
}
```

```bash
$ export PORT=3000

$ cmd                    # Port = 8080 (default, if no env)
$ cmd                    # Port = 3000 (env wins over default)
$ cmd --port 9000        # Port = 9000 (CLI wins over env)
```

## Environment Variables

### Single Environment Variable

```go
type Cmd struct {
    Port int `flag:"port" env:"PORT" help:"Port to listen on"`
}
```

### Multiple Environment Variables

Try multiple env vars in order (first found wins):

```go
type Cmd struct {
    Port int `flag:"port" env:"APP_PORT,PORT" help:"Port to listen on"`
}
```

### Env-Only Fields

Fields with `env` but no `flag` or `arg` tag are env/config/default-only. They don't appear in help and can't be passed via CLI:

```go
type Cmd struct {
    Token string `env:"API_TOKEN" required:"" help:"API token"`  // env-only
    Env   string `flag:"env" help:"Target environment"`          // CLI flag
}
```

This is useful for secrets that shouldn't appear in shell history.

### Global Env Prefix

Add a prefix to all env var lookups:

```go
cli.Execute(ctx, root, args, cli.WithEnvVarPrefix("MYAPP_"))
```

Now `env:"PORT"` looks up `MYAPP_PORT`.

## Embedded Structs and Prefix

### Anonymous Embedding (Flag Promotion)

Anonymous embedded structs promote their flags:

```go
type OutputFlags struct {
    Format  string `flag:"format" enum:"json,table" default:"table" help:"Output format"`
    Verbose bool   `flag:"verbose" short:"v" help:"Verbose output"`
}

type ListCmd struct {
    OutputFlags  // promoted: --format, --verbose work directly
    Limit int    `flag:"limit" default:"50" help:"Max results"`
}
```

```
$ list --format json --verbose --limit 100
```

### Named Structs with Prefix

Named struct fields with the `prefix` tag namespace their flags:

```go
type DBConfig struct {
    Host string `flag:"host" default:"localhost" help:"Database host"`
    Port int    `flag:"port" default:"5432" help:"Database port"`
}

type ServeCmd struct {
    DB   DBConfig `prefix:"db-"`  // --db-host, --db-port
    Port int      `flag:"port" default:"8080" help:"Server port"`
}
```

```
$ serve --db-host db.example.com --db-port 5433 --port 9000
```

### Nested Prefixes

Prefixes nest naturally:

```go
type Credentials struct {
    User     string `flag:"user" help:"Username"`
    Password string `flag:"password" help:"Password"`
}

type DBConfig struct {
    Host  string      `flag:"host"`
    Creds Credentials `prefix:"auth-"`  // --auth-user, --auth-password
}

type Cmd struct {
    DB DBConfig `prefix:"db-"`  // --db-host, --db-auth-user, --db-auth-password
}
```

### Shadowing

When an outer field and embedded field share a name, the outer field wins (matching Go's field promotion):

```go
type Base struct {
    Port int `flag:"port" default:"80" help:"Base port"`
}

type Cmd struct {
    Base
    Port int `flag:"port" default:"8080" help:"Server port"`  // wins
}
```

## Flag Inheritance

Flags automatically flow from parent commands to child subcommands when they share the same name and type:

```go
type App struct {
    Env string `flag:"env" required:"" enum:"dev,qa,prod" help:"Target environment"`
}

type ServeCmd struct {
    Env  string `flag:"env" help:"Target environment"`  // inherits from parent
    Port int    `flag:"port" default:"8080" help:"Port"`
}
```

```
$ app --env prod serve --port 9000
# ServeCmd.Env = "prod" (inherited from parent)
```

### Hidden Inherited Flags

To inherit without exposing in child's help:

```go
type ServeCmd struct {
    Env  string `flag:"env" hidden:""`  // inherits, not shown in help
    Port int    `flag:"port" default:"8080"`
}
```

### Inheritance Priority

1. Explicit child flag (CLI arg)
2. Child env var
3. Inherited from parent
4. Child default
5. Zero value

## Custom Types

Implement `FlagUnmarshaler` for custom parsing:

```go
type LogLevel int

const (
    LevelDebug LogLevel = iota
    LevelInfo
    LevelWarn
    LevelError
)

func (l *LogLevel) UnmarshalFlag(value string) error {
    switch strings.ToLower(value) {
    case "debug":
        *l = LevelDebug
    case "info":
        *l = LevelInfo
    case "warn":
        *l = LevelWarn
    case "error":
        *l = LevelError
    default:
        return fmt.Errorf("unknown log level: %s", value)
    }
    return nil
}

type Cmd struct {
    Level LogLevel `flag:"level" default:"info" help:"Log level"`
}
```

```
$ cmd --level debug
$ cmd --level warn
```

## Flag Groups

Constrain flag relationships using `FlagGrouper`:

```go
func (c *Cmd) FlagGroups() []cli.FlagGroup {
    return []cli.FlagGroup{
        cli.MutuallyExclusive("json", "yaml", "text"),    // at most one
        cli.RequiredTogether("username", "password"),      // all or none
        cli.OneRequired("file", "stdin"),                  // exactly one
    }
}
```

### MutuallyExclusive

At most one flag in the group may be set:

```go
type Cmd struct {
    JSON bool `flag:"json" help:"JSON output"`
    YAML bool `flag:"yaml" help:"YAML output"`
    Text bool `flag:"text" help:"Text output"`
}

func (c *Cmd) FlagGroups() []cli.FlagGroup {
    return []cli.FlagGroup{
        cli.MutuallyExclusive("json", "yaml", "text"),
    }
}
```

```
$ cmd --json --yaml    # Error: mutually exclusive: --json, --yaml
$ cmd --json           # OK
$ cmd                  # OK (none set)
```

### RequiredTogether

If any flag is set, all must be set:

```go
type Cmd struct {
    User     string `flag:"user" help:"Username"`
    Password string `flag:"password" help:"Password"`
}

func (c *Cmd) FlagGroups() []cli.FlagGroup {
    return []cli.FlagGroup{
        cli.RequiredTogether("user", "password"),
    }
}
```

```
$ cmd --user alice             # Error: required together: --user, --password
$ cmd --user alice --password secret  # OK
$ cmd                          # OK (none set)
```

### OneRequired

Exactly one flag must be set:

```go
type Cmd struct {
    File  string `flag:"file" help:"Read from file"`
    Stdin bool   `flag:"stdin" help:"Read from stdin"`
}

func (c *Cmd) FlagGroups() []cli.FlagGroup {
    return []cli.FlagGroup{
        cli.OneRequired("file", "stdin"),
    }
}
```

```
$ cmd                           # Error: one required: --file, --stdin
$ cmd --file data.txt --stdin   # Error: one required: --file, --stdin
$ cmd --file data.txt           # OK
$ cmd --stdin                   # OK
```

## Flag Syntax

The framework accepts standard flag syntax:

```
--name value     # long form with space
--name=value     # long form with equals
-n value         # short form with space
-n=value         # short form with equals
-abc             # combined short booleans (if enabled)
```

### Boolean Flags

Boolean flags don't require a value:

```
--verbose        # sets to true
--no-verbose     # sets to false (if negate tag is set)
```

### Flag Positioning

Flags can appear anywhere — before or after subcommand names:

```
$ app --verbose serve --port 8080
$ app serve --verbose --port 8080
$ app serve --port 8080 --verbose
```

### Terminator

`--` stops flag parsing; remaining args become positional:

```
$ grep --ignore-case -- --pattern
# "--pattern" is treated as a positional argument, not a flag
```

## Validation

The framework validates struct tags at parse time. Invalid combinations return `ErrInvalidTag`:

```go
// ❌ Invalid: flag and arg are mutually exclusive
Field string `flag:"foo" arg:"foo"`

// ❌ Invalid: required and default are mutually exclusive
Field string `flag:"foo" required:"" default:"bar"`

// ❌ Invalid: counter requires int type
Field string `flag:"foo" counter:""`

// ❌ Invalid: negate requires bool type
Field int `flag:"foo" negate:""`

// ❌ Invalid: sep requires slice type
Field string `flag:"foo" sep:","`

// ❌ Invalid: short requires flag
Field string `short:"f"`
```

## Advanced Options

### Short Option Handling

Enable POSIX-style combined short options:

```go
cli.Execute(ctx, root, args, cli.WithShortOptionHandling(true))
```

```
$ cmd -abc    # equivalent to -a -b -c
```

### Prefix Matching

Allow unique prefixes to match flags:

```go
cli.Execute(ctx, root, args, cli.WithPrefixMatching(true))
```

```
$ cmd --verb    # matches --verbose if unique
```

### Ignore Unknown Flags

Pass unknown flags to subcommands:

```go
cli.Execute(ctx, root, args, cli.WithIgnoreUnknown(true))
```

### Flag Normalizer

Normalize flag names (e.g., underscores to hyphens):

```go
cli.Execute(ctx, root, args, cli.WithFlagNormalizer(func(name string) string {
    return strings.ReplaceAll(name, "_", "-")
}))
```

```
$ cmd --output_format json    # normalized to --output-format
```

## Inspecting Flags

Use `ScanFlags` to inspect a command's flags programmatically:

```go
flags := cli.ScanFlags(cmd)
for _, f := range flags {
    fmt.Printf("--%s: %s (default: %s)\n", f.Name, f.Help, f.Default)
}
```

This is useful for custom help renderers, documentation generators, and testing.

## What's Next

- [Arguments](arguments.md) — Positional arguments
- [Configuration](configuration.md) — Config file integration
- [Lifecycle](lifecycle.md) — Hooks for setup and teardown
