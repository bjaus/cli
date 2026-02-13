# Error Handling

The framework provides hierarchical sentinel errors, help signals, and custom exit codes for precise error handling.

## Sentinel Errors

Sentinel errors form a hierarchy you can match with `errors.Is`:

```go
if errors.Is(err, cli.ErrFlag) {
    // any flag-related error
}

if errors.Is(err, cli.ErrUnknownFlag) {
    // specifically an unknown flag
}
```

### Error Hierarchy

```
ErrFlag
├── ErrUnknownFlag         "unknown flag"
├── ErrFlagRequiresVal     "flag requires a value"
├── ErrFlagInvalidVal      "flag has invalid value"
├── ErrFlagRequired        "required flag not set"
└── ErrFlagParse           "flag parse error"

ErrArgument
├── ErrArgRequired         "required argument not set"
├── ErrArgParse            "argument parse error"
└── ErrArgValidation       "argument validation failed"

ErrCommand
├── ErrNoCommand           "no command specified"
├── ErrUnknownCommand      "unknown command"
└── ErrCommandCollision    "command name collision"

ErrFlagGroup
├── ErrMutuallyExclusive   "mutually exclusive flags"
├── ErrRequiredTogether    "required together flags"
└── ErrOneRequired         "one required flags"
```

### Matching Errors

Use `errors.Is` to check error types:

```go
func (a *App) Run(ctx context.Context) error {
    err := doSomething()

    switch {
    case errors.Is(err, cli.ErrUnknownFlag):
        fmt.Println("Try --help for available flags")
        return cli.Exit("", 2)

    case errors.Is(err, cli.ErrFlagRequired):
        fmt.Println("Missing required flag")
        return cli.ErrShowUsage

    case errors.Is(err, cli.ErrFlag):
        // catches any flag error
        return err

    default:
        return err
    }
}
```

### Unwrapping Details

Errors wrap underlying causes:

```go
err := cli.Execute(ctx, &App{}, args)
if err != nil {
    // Get the full chain
    fmt.Println(err) // "flag parse error: strconv.ParseInt: ..."

    // Check specific type
    if errors.Is(err, cli.ErrFlagParse) {
        // Handle parse failure
    }

    // Get underlying error
    if unwrapped := errors.Unwrap(err); unwrapped != nil {
        fmt.Println(unwrapped) // "strconv.ParseInt: ..."
    }
}
```

## Help Signals

Special errors trigger help or usage display:

| Signal | Exit Code | Output |
|--------|-----------|--------|
| `ShowHelp` | 0 | Full help text |
| `ErrShowHelp` | 1 | Full help text |
| `ShowUsage` | 0 | Brief usage line |
| `ErrShowUsage` | 1 | Brief usage line |

### Triggering Help

Return signals from `Run`:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if len(c.Args) == 0 {
        return cli.ErrShowHelp // show help, exit 1
    }
    return nil
}
```

### Success with Help

Use non-error signals for help as success:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if c.ShowHelp {
        return cli.ShowHelp // exit 0
    }
    return nil
}
```

### Usage vs Help

Usage shows a brief single line; help shows the full output:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if c.Target == "" {
        // Brief: "Usage: app deploy <target> [flags]"
        return cli.ErrShowUsage
    }
    return nil
}
```

## Custom Exit Codes

### Exit Function

Create errors with specific exit codes:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if err := validate(); err != nil {
        return cli.Exit("validation failed", 2)
    }

    if err := execute(); err != nil {
        return cli.Exitf(3, "execution failed: %v", err)
    }

    return nil
}
```

### ExitCoder Interface

Any error implementing `ExitCoder` controls the exit code:

```go
type ExitCoder interface {
    ExitCode() int
}
```

Custom exit code errors:

```go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

func (e *ValidationError) ExitCode() int {
    return 65 // EX_DATAERR from sysexits.h
}
```

### Standard Exit Codes

Common conventions:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Misuse of command |
| 64 | Usage error (EX_USAGE) |
| 65 | Data error (EX_DATAERR) |
| 66 | No input (EX_NOINPUT) |
| 74 | I/O error (EX_IOERR) |
| 78 | Config error (EX_CONFIG) |

## Silencing Output

### Silent Errors

Suppress error output while keeping the exit code:

```go
func (c *Cmd) Run(ctx context.Context) error {
    // Already printed error message
    fmt.Fprintln(os.Stderr, "Error: connection failed")

    // Exit without printing again
    return cli.Exit("", 1)
}
```

### Conditional Output

Control output based on verbosity:

```go
func (c *Cmd) Run(ctx context.Context) error {
    err := doWork()
    if err != nil {
        if c.Verbose {
            return fmt.Errorf("detailed: %w", err)
        }
        return cli.Exit("operation failed", 1)
    }
    return nil
}
```

## Error Handling Patterns

### Validate Before Run

Use lifecycle hooks for early validation:

```go
func (c *Cmd) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("port must be 1-65535, got %d", c.Port)
    }
    return nil
}
```

### Wrap with Context

Add context when returning errors:

```go
func (c *Cmd) Run(ctx context.Context) error {
    if err := c.loadConfig(); err != nil {
        return fmt.Errorf("loading config: %w", err)
    }

    if err := c.connect(); err != nil {
        return fmt.Errorf("connecting to %s: %w", c.Host, err)
    }

    return nil
}
```

### Aggregate Errors

Collect multiple errors:

```go
func (c *Cmd) Validate() error {
    var errs []string

    if c.Host == "" {
        errs = append(errs, "host is required")
    }
    if c.Port == 0 {
        errs = append(errs, "port is required")
    }
    if c.Token == "" && c.User == "" {
        errs = append(errs, "token or user is required")
    }

    if len(errs) > 0 {
        return fmt.Errorf("validation failed:\n  %s", strings.Join(errs, "\n  "))
    }
    return nil
}
```

### Recover from Panics

Handle panics gracefully:

```go
func main() {
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr, "panic: %v\n", r)
            os.Exit(1)
        }
    }()

    cli.ExecuteAndExit(ctx, &App{}, os.Args)
}
```

## Complete Example

```go
type DeployCmd struct {
    Target  string `arg:"target" required:""`
    Force   bool   `flag:"force" help:"Skip confirmation"`
    Timeout int    `flag:"timeout" default:"60" help:"Timeout in seconds"`
}

func (d *DeployCmd) Validate() error {
    if d.Timeout < 1 {
        return fmt.Errorf("timeout must be positive")
    }
    return nil
}

func (d *DeployCmd) Run(ctx context.Context) error {
    // Check target exists
    if !targetExists(d.Target) {
        return cli.Exitf(2, "target %q not found", d.Target)
    }

    // Confirm unless forced
    if !d.Force {
        if !confirm("Deploy to %s?", d.Target) {
            return cli.Exit("aborted", 0)
        }
    }

    // Deploy with timeout
    ctx, cancel := context.WithTimeout(ctx, time.Duration(d.Timeout)*time.Second)
    defer cancel()

    if err := deploy(ctx, d.Target); err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            return cli.Exit("deployment timed out", 124)
        }
        return fmt.Errorf("deployment failed: %w", err)
    }

    fmt.Println("Deployed successfully")
    return nil
}
```

## What's Next

- [Lifecycle](lifecycle.md) — Hook execution and error propagation
- [Flags](flags.md) — Flag parsing and validation
- [Help](help.md) — Help display triggers
