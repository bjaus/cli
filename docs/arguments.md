# Arguments

Positional arguments are values passed after the command name that aren't flags. The framework supports two approaches: struct-tagged named arguments and the `Args` type for dynamic access.

## Named Arguments

Use the `arg` tag to define named positional arguments:

```go
type CopyCmd struct {
    Src string `arg:"src" help:"Source file"`
    Dst string `arg:"dst" help:"Destination file"`
}

func (c *CopyCmd) Run(ctx context.Context) error {
    return copyFile(c.Src, c.Dst)
}
```

```
$ copy source.txt dest.txt
```

Arguments are assigned in struct field order.

## Argument Name Resolution

If the `arg` tag value is empty, the name is derived from the field name:

```go
type Cmd struct {
    SourceFile string `arg:"" help:"Source file"`  // arg name: source-file
    Target     string `arg:"target"`               // arg name: target
}
```

## Required vs Optional

Non-slice arguments are **required by default**. Use `required:"false"` to make optional:

```go
type Cmd struct {
    Input  string `arg:"input" help:"Input file"`                    // required
    Output string `arg:"output" required:"false" help:"Output file"` // optional
}
```

```
$ cmd input.txt              # Output = "" (optional)
$ cmd input.txt output.txt   # Output = "output.txt"
```

## Default Values

Provide defaults for optional arguments:

```go
type Cmd struct {
    Input  string `arg:"input" help:"Input file"`
    Output string `arg:"output" required:"false" default:"out.txt" help:"Output file"`
}
```

```
$ cmd input.txt         # Output = "out.txt"
$ cmd input.txt foo.txt # Output = "foo.txt"
```

## Environment Variables

Arguments can fall back to environment variables:

```go
type Cmd struct {
    Env string `arg:"env" env:"TARGET_ENV" help:"Target environment"`
}
```

```bash
$ export TARGET_ENV=production
$ cmd              # Env = "production" (from env)
$ cmd staging      # Env = "staging" (positional wins)
```

Priority: positional arg > env var > default > zero value.

## Enum Validation

Restrict arguments to specific values:

```go
type Cmd struct {
    Env string `arg:"env" enum:"dev,staging,prod" help:"Target environment"`
}
```

```
$ cmd dev      # OK
$ cmd other    # Error: env must be one of [dev,staging,prod]
```

## Slice Arguments

A slice field consumes all remaining arguments:

```go
type CatCmd struct {
    Files []string `arg:"files" help:"Files to concatenate"`
}
```

```
$ cat file1.txt file2.txt file3.txt
# Files = ["file1.txt", "file2.txt", "file3.txt"]
```

Slice arguments are **optional by default** (empty slice is valid). Use `required:""` if at least one value is needed.

### Mixed Named and Slice Arguments

Named arguments are consumed first, then the slice gets the rest:

```go
type Cmd struct {
    Output string   `arg:"output" help:"Output file"`
    Inputs []string `arg:"inputs" help:"Input files"`
}
```

```
$ cmd out.txt a.txt b.txt c.txt
# Output = "out.txt"
# Inputs = ["a.txt", "b.txt", "c.txt"]
```

## The Args Type

For dynamic argument handling, use `cli.Args`:

```go
type GrepCmd struct {
    Pattern string `arg:"pattern" help:"Search pattern"`
    Args    cli.Args  // remaining arguments (no tag needed)
}
```

```
$ grep "error" file1.txt file2.txt
# Pattern = "error"
# Args = ["file1.txt", "file2.txt"]
```

### Args Methods

`cli.Args` provides convenience methods:

| Method | Description |
|--------|-------------|
| `Len()` | Number of arguments |
| `Empty()` | Returns true if no arguments |
| `First()` | First argument or empty string |
| `Last()` | Last argument or empty string |
| `Get(i)` | Argument at index or empty string |
| `Contains(s)` | Check if argument exists |
| `Index(s)` | Index of argument or -1 |
| `Tail()` | All arguments after the first |

```go
func (g *GrepCmd) Run(ctx context.Context) error {
    if g.Args.Empty() {
        // read from stdin
    }
    for _, file := range g.Args {
        // process each file
    }
    return nil
}
```

### Args Field Rules

- Only one `cli.Args` field is allowed per command
- No tag is required on the `Args` field
- `Args` is populated after named arg fields
- `Args` receives all remaining arguments not consumed by named args

## Struct Tags Reference

| Tag | Type | Description |
|-----|------|-------------|
| `arg` | string | Argument name. If empty, derived from field name. |
| `help` | string | Description shown in help output |
| `default` | string | Default value when not provided |
| `required` | presence | Argument must be provided. Non-slice args default to required. |
| `enum` | string | Comma-separated list of allowed values |
| `env` | string | Environment variable fallback |
| `mask` | string | Display instead of default in help (e.g., `****` for secrets) |

## Argument Validators

For custom validation logic, implement `ArgsValidator`:

```go
type Cmd struct {
    Args cli.Args
}

func (c *Cmd) ValidateArgs(args []string) error {
    if len(args) == 0 {
        return errors.New("at least one file required")
    }
    for _, arg := range args {
        if !strings.HasSuffix(arg, ".txt") {
            return fmt.Errorf("invalid file: %s (must be .txt)", arg)
        }
    }
    return nil
}
```

### Built-in Validators

The framework provides common validators:

```go
func (c *Cmd) ValidateArgs(args []string) error {
    return cli.ExactArgs(2)(args)  // requires exactly 2 args
}
```

| Validator | Description |
|-----------|-------------|
| `ExactArgs(n)` | Exactly n arguments |
| `MinArgs(n)` | At least n arguments |
| `MaxArgs(n)` | At most n arguments |
| `RangeArgs(lo, hi)` | Between lo and hi arguments (inclusive) |
| `NoArgs` | No arguments allowed |

```go
// Examples
cli.ExactArgs(2)      // exactly 2
cli.MinArgs(1)        // at least 1
cli.MaxArgs(5)        // at most 5
cli.RangeArgs(1, 3)   // 1 to 3
cli.NoArgs            // none allowed (not a function)
```

## Passthrough Mode

For wrapper commands that forward args to child processes, implement `Passthrougher`:

```go
type ExecCmd struct {
    Verbose bool     `flag:"verbose" help:"Verbose output"`
    Args    cli.Args
}

func (e *ExecCmd) Passthrough() bool { return true }
```

In passthrough mode, unknown flags become positional arguments instead of errors:

```
$ exec --verbose --unknown-flag value
# Verbose = true
# Args = ["--unknown-flag", "value"]
```

## Branching Arguments

When a command has both positional arguments and embedded subcommands, arguments are consumed before subcommand resolution:

```go
type UserCmd struct {
    ID     int       `arg:"id" help:"User ID"`
    Delete DeleteCmd              // subcommand
    Rename RenameCmd              // subcommand
}

type DeleteCmd struct{}

func (d *DeleteCmd) Name() string { return "delete" }
func (d *DeleteCmd) Run(ctx context.Context) error {
    userID := cli.Get[int](ctx, "id")  // access parent's arg
    return deleteUser(userID)
}
```

```
$ app user 42 delete
# UserCmd.ID = 42
# DeleteCmd runs with ID accessible via context
```

This enables patterns like:

```
$ app user 42 delete --force
$ app user 42 rename newname
$ app resource abc-123 get
$ app resource abc-123 update --field value
```

## Context Access

Arguments are stored in context and accessible from subcommands:

```go
func (d *DeleteCmd) Run(ctx context.Context) error {
    userID := cli.Get[int](ctx, "id")
    // or with checking
    userID, ok := cli.Lookup[int](ctx, "id")
    if !ok {
        return errors.New("missing user ID")
    }
    return deleteUser(userID)
}
```

## Help Output

Arguments appear in the usage line and arguments section:

```
Usage:
  copy <src> <dst> [flags]

Arguments:
  src   Source file
  dst   Destination file
```

Optional arguments are shown in brackets:

```
Usage:
  cmd <input> [output] [flags]
```

Slice arguments are shown with ellipsis:

```
Usage:
  cat [files...] [flags]
```

## Supported Types

Arguments support the same types as flags:

- `string`
- `int`, `int64`, `uint`, `uint64`
- `float64`
- `bool`
- `time.Duration`
- `time.Time`
- `*url.URL`
- `net.IP`
- `[]T` for any scalar type T
- Custom types implementing `FlagUnmarshaler`

## What's Next

- [Subcommands](subcommands.md) — Command hierarchies
- [Lifecycle](lifecycle.md) — Hooks for setup and validation
- [Flags](flags.md) — Flag parsing in detail
