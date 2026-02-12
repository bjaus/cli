# Go CLI Framework Comparison

A comprehensive technical evaluation of four Go CLI frameworks, examining their
architecture, feature sets, API design, and trade-offs.

|                     | [spf13/cobra]                         | [urfave/cli] | [alecthomas/kong] | [bjaus/cli] |
| ------------------- | ------------------------------------- | ------------ | ----------------- | ----------- |
| **GitHub Stars**    | ~39k                                  | ~23k         | ~3k               | -           |
| **First Release**   | 2013                                  | 2013         | 2018              | 2026        |
| **Current Version** | v1.9+                                 | v3.x         | v1.x              | v0.x        |
| **Runtime Deps**    | 4 (pflag, mousetrap, go-md2man, yaml) | 0            | 0                 | 0           |
| **Min Go Version**  | 1.16                                  | 1.22         | 1.20              | 1.23        |
| **License**         | Apache 2.0                            | MIT          | MIT               | MIT         |

[spf13/cobra]: https://github.com/spf13/cobra
[urfave/cli]: https://github.com/urfave/cli
[alecthomas/kong]: https://github.com/alecthomas/kong
[bjaus/cli]: https://github.com/bjaus/cli

---

## Table of Contents

1. [Architecture & Design Philosophy](#1-architecture--design-philosophy)
2. [Command Definition](#2-command-definition)
3. [Flags](#3-flags)
4. [Positional Arguments](#4-positional-arguments)
5. [Subcommands](#5-subcommands)
6. [Lifecycle Hooks & Middleware](#6-lifecycle-hooks--middleware)
7. [Help Generation](#7-help-generation)
8. [Shell Completion](#8-shell-completion)
9. [Error Handling](#9-error-handling)
10. [Configuration](#10-configuration)
11. [Dependency Injection](#11-dependency-injection)
12. [Plugin System](#12-plugin-system)
13. [Testing](#13-testing)
14. [Documentation Generation](#14-documentation-generation)
15. [Feature Matrix](#15-feature-matrix)
16. [Verdict](#16-verdict)

---

## 1. Architecture & Design Philosophy

### spf13/cobra

Cobra uses a **monolithic struct** as its core abstraction. The `cobra.Command` struct
contains ~50 fields covering identity, documentation, flags, execution hooks, and
configuration. Commands are created imperatively by instantiating this struct and
wiring children via `AddCommand()`.

```go
rootCmd := &cobra.Command{
    Use:   "app",
    Short: "My application",
    RunE: func(cmd *cobra.Command, args []string) error {
        return nil
    },
}
rootCmd.AddCommand(childCmd)
rootCmd.Execute()
```

The design follows a **tree of commands** model where configuration lives in struct
fields and behavior lives in function callbacks assigned to those fields. The
`cobra.Command` struct is both configuration and runtime — it holds parsed state, I/O
writers, flag sets, and the complete command tree.

**Philosophy**: Provide everything in one struct. Cobra values completeness over
minimalism. The API is imperative: you create objects, mutate them, and wire them
together.

### urfave/cli

urfave/cli v3 uses a **unified `Command` type** for both the root application and all
subcommands (v2 had a separate `App` type). Commands are defined declaratively via
struct literals with nested `Commands` slices.

```go
cmd := &cli.Command{
    Name:  "app",
    Usage: "My application",
    Flags: []cli.Flag{
        &cli.StringFlag{Name: "name", Value: "World"},
    },
    Action: func(ctx context.Context, cmd *cli.Command) error {
        fmt.Println(cmd.String("name"))
        return nil
    },
}
cmd.Run(context.Background(), os.Args)
```

v3 eliminated the custom `cli.Context` type, splitting its responsibilities between
Go's standard `context.Context` and methods on `*Command` itself. Handler signatures
are `func(context.Context, *Command) error`.

**Philosophy**: Declarative and progressive. Start simple, add features as needed.
Keep the core dependency-free.

### alecthomas/kong

Kong's core abstraction is **the Go struct itself**. The CLI is defined entirely
through struct fields and struct tags — no framework types needed in command
definitions. Kong uses reflection to build a command tree from the struct hierarchy.

```go
var CLI struct {
    Verbose bool       `help:"Enable verbose output." short:"v"`
    Serve   ServeCmd   `cmd:"" help:"Start the server."`
}

type ServeCmd struct {
    Port int `help:"Port to listen on." default:"8080"`
}

func (s *ServeCmd) Run() error {
    return startServer(s.Port)
}

func main() {
    ctx := kong.Parse(&CLI)
    ctx.Run()
}
```

Subcommands are nested structs tagged with `cmd:""`. Flags are struct fields.
Arguments are fields tagged with `arg:""`. Everything is declared in the type system.

**Philosophy**: Declarative to an extreme. The struct _is_ the CLI spec. Minimize
framework surface area in user code. Let the type system and tags carry the weight.

### bjaus/cli

bjaus/cli uses an **interface-driven discovery** model. The only required contract is
a single-method interface:

```go
type Commander interface {
    Run(ctx context.Context) error
}
```

Every command is a plain Go struct implementing `Commander`. All other capabilities are
opt-in through ~30 small interfaces (most with a single method), discovered at
runtime via type assertions. There is no base struct to embed, no configuration DSL,
and no global state.

```go
type Root struct{
    Env string `flag:"env" short:"e" default:"dev" help:"Target environment"`
}

func (r *Root) Name() string              { return "app" }
func (r *Root) Description() string       { return "My application" }
func (r *Root) Subcommands() []cli.Commander { return []cli.Commander{&ServeCmd{}} }
func (r *Root) Run(_ context.Context) error { return cli.ErrShowHelp }

type ServeCmd struct {
    Port int `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
}

func (s *ServeCmd) Name() string              { return "serve" }
func (s *ServeCmd) Run(_ context.Context) error { return startServer(s.Port) }

func main() {
    cli.ExecuteAndExit(context.Background(), &Root{}, os.Args)
}
```

The design is analogous to how `io.Reader`, `io.Writer`, and `fmt.Stringer` work in
the standard library: small interfaces that compose. A command "has a description" by
implementing `Descriptor`. It "has subcommands" by implementing `Subcommander`. It "has
aliases" by implementing `Aliaser`. No method is mandatory beyond `Run`. The
`Interfaces` type documents all ~30 optional interfaces in one place for IDE
discoverability.

For rapid prototyping, the embeddable `Meta` struct provides default implementations
for common metadata interfaces (`Namer`, `Descriptor`, `Aliaser`, `Categorizer`,
`Hider`, `Deprecator`). The zero value is useful — omit the name to use the struct
name default:

```go
type ServeCmd struct {
    cli.Meta
    Port int `flag:"port" default:"8080" help:"Port to listen on"`
}

// Initialize with builder methods
cmd := &ServeCmd{
    Meta: cli.Meta{}.
        WithName("serve").
        WithDescription("Start the server").
        WithAliases("s"),
}

func (s *ServeCmd) Run(ctx context.Context) error { return startServer(s.Port) }
```

**Philosophy**: Composition over configuration. Think `io.Reader` for CLIs. The
framework discovers what a command can do rather than the command declaring it in a
monolithic struct.

### Summary

| Approach               | cobra                       | urfave/cli                   | kong                        | bjaus/cli                      |
| ---------------------- | --------------------------- | ---------------------------- | --------------------------- | ------------------------------ |
| **Core type**          | `cobra.Command` struct      | `cli.Command` struct         | User-defined struct + tags  | `Commander` interface          |
| **Style**              | Imperative                  | Declarative                  | Declarative (tags)          | Interface composition          |
| **Registration**       | `AddCommand()` calls        | Nested struct literals       | Nested struct fields        | `Subcommands()` method         |
| **Flag definition**    | `cmd.Flags().StringVar()`   | `[]cli.Flag{&StringFlag{}}`  | Struct tag `default:"8080"` | Struct tag `flag:"port"`       |
| **Execution**          | `cmd.Execute()`             | `cmd.Run(ctx, args)`         | `ctx.Run()`                 | `cli.Execute(ctx, root, args)` |
| **Framework coupling** | High (embed framework type) | Medium (use framework types) | Low (plain structs + tags)  | Lowest (plain structs, no framework types) |

---

## 2. Command Definition

### cobra

Commands are `cobra.Command` struct literals. Identity is set via struct fields:

```go
cmd := &cobra.Command{
    Use:     "serve [flags]",
    Aliases: []string{"s"},
    Short:   "Start the server",
    Long:    "Start the HTTP server with the given configuration.",
    Example: "  app serve --port 8080",
    GroupID: "server",
    Hidden:  true,
    Deprecated: "use 'start' instead",
}
```

Commands are wired via `parent.AddCommand(child)`. This is imperative — the tree is
built through method calls, not through data structures. The `Use` field doubles as
both the command name and the usage synopsis.

### urfave/cli

Commands are `cli.Command` struct literals with an inline `Commands` slice:

```go
cmd := &cli.Command{
    Name:    "serve",
    Aliases: []string{"s"},
    Usage:   "Start the server",
    Commands: []*cli.Command{
        {Name: "start", Action: startAction},
        {Name: "stop",  Action: stopAction},
    },
    Action: serveAction,
}
```

The tree is declarative — subcommands are nested in the struct literal. No wiring
calls needed.

### kong

Commands are Go structs tagged with `cmd:""`:

```go
var CLI struct {
    Serve ServeCmd `cmd:"" aliases:"s" help:"Start the server." hidden:""`
}

type ServeCmd struct {
    Start StartCmd `cmd:"" help:"Start the server."`
    Stop  StopCmd  `cmd:"" help:"Stop the server."`
}
```

The struct field name (lowercased) becomes the command name. Tags control all
metadata. The tree is implicit in the struct nesting.

### bjaus/cli

Commands are Go structs implementing `Commander`. Identity is expressed through
interfaces:

```go
type ServeCmd struct {
    Port int `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
}

func (s *ServeCmd) Name() string              { return "serve" }
func (s *ServeCmd) Aliases() []string         { return []string{"s"} }
func (s *ServeCmd) Description() string       { return "Start the server" }
func (s *ServeCmd) Category() string          { return "Server" }
func (s *ServeCmd) Hidden() bool              { return false }
func (s *ServeCmd) Deprecated() string        { return "" }
func (s *ServeCmd) Subcommands() []cli.Commander { return []cli.Commander{&StartCmd{}, &StopCmd{}} }
func (s *ServeCmd) Run(ctx context.Context) error { /* ... */ }
```

Only `Run` is required. The rest are implemented only when needed. The command tree is
built by returning children from `Subcommands()`. No global state or registration
calls.

For quick bootstrapping, embed `Meta` to get common interfaces without boilerplate:

```go
type ServeCmd struct {
    cli.Meta
    Port int `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
}

func (s *ServeCmd) Subcommands() []cli.Commander { return []cli.Commander{&StartCmd{}, &StopCmd{}} }
func (s *ServeCmd) Run(ctx context.Context) error { /* ... */ }

// Initialize with builder methods:
cmd := &ServeCmd{
    Meta: cli.Meta{}.
        WithName("serve").
        WithDescription("Start the server").
        WithAliases("s").
        WithCategory("Server"),
}
```

`Meta` implements `Namer`, `Descriptor`, `LongDescriptor`, `Aliaser`, `Categorizer`,
`Hider`, `Deprecator`, and `Exampler`. Override any method by defining it on your type.

### Comparison

| Feature      | cobra                 | urfave/cli       | kong              | bjaus/cli                           |
| ------------ | --------------------- | ---------------- | ----------------- | ----------------------------------- |
| Command name | `Use` field           | `Name` field     | Struct field name | `Name()` method                     |
| Aliases      | `Aliases` field       | `Aliases` field  | `aliases:""` tag  | `Aliases()` method                  |
| Description  | `Short`/`Long` fields | `Usage` field    | `help:""` tag     | `Description()`/`LongDescription()` |
| Hidden       | `Hidden` field        | `Hidden` field   | `hidden:""` tag   | `Hidden()` method                   |
| Deprecated   | `Deprecated` field    | -                | -                 | `Deprecated()` method               |
| Categories   | `GroupID` field       | `Category` field | `group:""` tag    | `Category()` method                 |
| Examples     | `Example` field       | -                | -                 | `Examples()` method                 |

---

## 3. Flags

### Type Support

| Type            | cobra (pflag) | urfave/cli    | kong                        | bjaus/cli                   |
| --------------- | ------------- | ------------- | --------------------------- | --------------------------- |
| string          | Y             | Y             | Y                           | Y                           |
| int/int64       | Y (int8-64)   | Y             | Y                           | Y (int, int64)              |
| uint/uint64     | Y (uint8-64)  | Y             | Y                           | Y (uint, uint64)            |
| float32/float64 | Y             | Y             | Y                           | Y (float64)                 |
| bool            | Y             | Y             | Y                           | Y                           |
| time.Duration   | Y             | Y             | Y                           | Y                           |
| time.Time       | -             | Y (Timestamp) | Y                           | Y (RFC3339/date)            |
| string slice    | Y             | Y             | Y                           | Y                           |
| int slice       | Y             | Y             | Y                           | Y                           |
| map[K]V         | -             | Y (StringMap) | Y                           | Y                           |
| \*os.File       | -             | -             | Y                           | -                           |
| \*url.URL       | -             | -             | Y                           | Y                           |
| net.IP          | Y             | -             | -                           | Y                           |
| Custom types    | pflag.Value   | cli.Value     | TextUnmarshaler/MapperValue | FlagUnmarshaler             |
| Counter (-vvv)  | Y (Count)     | -             | Y (`type:"counter"`)        | Y (`counter:""`)            |

### Definition Style

**cobra** — Imperative method calls:

```go
cmd.Flags().StringVarP(&cfg.Host, "host", "H", "localhost", "server hostname")
cmd.Flags().IntVar(&cfg.Port, "port", 8080, "server port")
cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
cmd.MarkFlagRequired("host")
```

**urfave/cli** — Typed struct literals in a slice:

```go
Flags: []cli.Flag{
    &cli.StringFlag{
        Name:     "host",
        Aliases:  []string{"H"},
        Value:    "localhost",
        Usage:    "server hostname",
        Required: true,
        Sources:  cli.EnvVars("APP_HOST"),
    },
    &cli.IntFlag{
        Name:  "port",
        Value: 8080,
        Usage: "server port",
    },
}
```

**kong** — Struct tags on fields:

```go
type ServeCmd struct {
    Host string `help:"Server hostname." short:"H" default:"localhost" required:""`
    Port int    `help:"Server port." default:"8080"`
}
```

**bjaus/cli** — Struct tags on fields:

```go
type ServeCmd struct {
    Host string `flag:"host" short:"H" default:"localhost" help:"Server hostname" required:""`
    Port int    `flag:"port" default:"8080" help:"Server port"`
}
```

### Flag Scopes

| Scope                  | cobra                   | urfave/cli           | kong               | bjaus/cli                      |
| ---------------------- | ----------------------- | -------------------- | ------------------ | ------------------------------ |
| Local (command only)   | `cmd.Flags()`           | `Local: true`        | Default behavior   | Default behavior               |
| Persistent (inherited) | `cmd.PersistentFlags()` | Default (root flags) | `embed:""` parent  | Auto-inherit by name+type      |
| Global                 | Persistent on root      | Root flags           | Root struct fields | Root flag + same name on child |

bjaus/cli has a unique **automatic flag inheritance** model: when parent and child
both declare a flag with the same name and type, the child automatically receives the
parent's value if not explicitly set. No explicit `Persistent` annotation needed.

### Flag Groups

All four frameworks support declarative flag relationship constraints:

| Constraint         | cobra                        | urfave/cli               | kong          | bjaus/cli                 |
| ------------------ | ---------------------------- | ------------------------ | ------------- | ------------------------- |
| Mutually exclusive | `MarkFlagsMutuallyExclusive` | `MutuallyExclusiveFlags` | `xor:"group"` | `MutuallyExclusive` group |
| Required together  | `MarkFlagsRequiredTogether`  | -                        | `and:"group"` | `RequiredTogether` group  |
| At least one       | `MarkFlagsOneRequired`       | -                        | -             | `OneRequired` group       |

### Environment Variable Binding

| Feature              | cobra     | urfave/cli               | kong            | bjaus/cli                   |
| -------------------- | --------- | ------------------------ | --------------- | --------------------------- |
| Per-flag env binding | Via Viper | `Sources: cli.EnvVars()` | `env:"VAR"` tag | `env:"VAR"` tag             |
| Multiple env vars    | Via Viper | `EnvVars("A","B")`       | `env:"A,B"`     | `env:"A,B"` (first found)   |
| Global env prefix    | Via Viper | -                        | `envprefix:""`  | `WithEnvVarPrefix()`        |
| Env-only fields      | -         | -                        | -               | `env:"VAR"` (no `flag` tag) |

bjaus/cli uniquely supports **env-only fields**: a struct field with an `env` tag but
no `flag` or `arg` tag is populated from the environment (or config/default) but
cannot be passed on the command line and does not appear in help. This is designed for
secrets that should never appear in shell history.

### Auto-Naming

| Framework  | Auto-naming                                                                             |
| ---------- | --------------------------------------------------------------------------------------- |
| cobra      | None (explicit names required)                                                          |
| urfave/cli | None (explicit names required)                                                          |
| kong       | Struct field name lowercased: `OutputFormat` → `output-format`                          |
| bjaus/cli  | CamelCase to kebab-case: `OutputFormat` → `--output-format`, `HTTPHost` → `--http-host` |

### Advanced Flag Features

| Feature                    | cobra              | urfave/cli                      | kong                 | bjaus/cli              |
| -------------------------- | ------------------ | ------------------------------- | -------------------- | ---------------------- |
| Negatable bools (`--no-X`) | -                  | `BoolWithInverseFlag`           | `negatable:""`       | `negate:""`        |
| Flag aliases               | Flag normalization | `Aliases` field                 | `aliases:""` tag     | `alt:""` tag           |
| Enum validation            | -                  | -                               | `enum:"a,b,c"`       | `enum:"a,b,c"`         |
| Hidden flags               | `MarkHidden()`     | `Hidden: true`                  | `hidden:""`          | `hidden:""`        |
| Deprecated flags           | `MarkDeprecated()` | -                               | -                    | `deprecated:"message"` |
| Placeholder in help        | -                  | -                               | `placeholder:"HOST"` | `placeholder:"HOST"`   |
| Secret masking             | -                  | -                               | -                    | `mask:"****"`          |
| Separator for slices       | -                  | -                               | `sep:","`            | `sep:","`              |
| Flag prefix (namespacing)  | -                  | -                               | `prefix:"db-"`       | `prefix:"db-"`         |
| Per-flag validation        | -                  | `Validator: func(T) error`      | -                    | -                      |
| Per-flag action            | -                  | `Action: func(ctx,cmd,T) error` | -                    | -                      |

---

## 4. Positional Arguments

### cobra

Cobra treats positional arguments as an untyped `[]string` passed to the `RunE`
callback. Validation is done via `PositionalArgs` functions:

```go
cmd := &cobra.Command{
    Use:  "copy [src] [dst]",
    Args: cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        src, dst := args[0], args[1]
        return nil
    },
}
```

Built-in validators: `NoArgs`, `ArbitraryArgs`, `ExactArgs(n)`, `MinimumNArgs(n)`,
`MaximumNArgs(n)`, `RangeArgs(min, max)`, `OnlyValidArgs`, `MatchAll(...)`.

Arguments are always `[]string` — no typed parsing, no struct binding.

### urfave/cli

v3 introduced typed positional arguments:

```go
cmd := &cli.Command{
    Arguments: []cli.Argument{
        &cli.StringArg{Name: "src", UsageText: "source file"},
        &cli.StringArg{Name: "dst", UsageText: "destination file"},
    },
}
```

Variadic arguments are supported via `StringArgs` with `Min`/`Max` constraints. v3
also supports `Destination` binding for direct variable assignment.

### kong

Kong binds positional arguments to struct fields via the `arg:""` tag:

```go
type CopyCmd struct {
    Src string `arg:"" help:"Source file."`
    Dst string `arg:"" help:"Destination file."`
}
```

Supports **branching positional arguments** — a unique feature where positional args
can lead to different subcommands:

```go
type UserCmd struct {
    ID struct {
        ID     int       `arg:""`
        Delete DeleteCmd `cmd:""`
        Rename RenameCmd `cmd:""`
    } `arg:""`
}
// Enables: app user 42 delete
//          app user 42 rename newname
```

### bjaus/cli

Positional arguments are declared via `arg` struct tags:

```go
type CopyCmd struct {
    Src string `arg:"src" help:"Source file"`
    Dst string `arg:"dst" help:"Destination file"`
}
```

Scalar fields are required by default. Use `cli.Args` (no tag needed) to capture
all positional args or remaining args when combined with named fields:

```go
type GrepCmd struct {
    Pattern string   `arg:"pattern" help:"Search pattern"`
    Args    cli.Args // captures remaining args (files to search)
}

func (g *GrepCmd) Run(ctx context.Context) error {
    if g.Args.Empty() {
        return errors.New("no files specified")
    }
    for _, file := range g.Args {
        // search file for pattern
    }
}
```

`Args` provides convenience methods: `Len`, `Empty`, `First`, `Last`, `Get`,
`Contains`, `Index`, `Tail`.

Supports `env`, `default`, `enum`, and `required` tags on named args.

Built-in validators: `NoArgs`, `ExactArgs(n)`, `MinArgs(n)`, `MaxArgs(n)`,
`RangeArgs(lo, hi)` — returned from the `ArgsValidator` interface.

### Comparison

| Feature        | cobra             | urfave/cli          | kong               | bjaus/cli                  |
| -------------- | ----------------- | ------------------- | ------------------ | -------------------------- |
| Typed binding  | No ([]string)     | Yes (v3)            | Yes (struct tags)  | Yes (struct tags)          |
| Validation     | Function-based    | Min/Max on variadic | Struct constraints | Interface + functions      |
| Variadic       | Always ([]string) | `*Args` types       | Slice field        | `cli.Args` type            |
| Branching args | No                | No                  | Yes                | Yes (automatic)            |
| Env fallback   | No                | No                  | Yes (`env:""`)     | Yes (`env:""`)             |
| Default values | No                | Yes                 | Yes (`default:""`) | Yes (`default:""`)

---

## 5. Subcommands

### Nesting & Discovery

All four support unlimited nesting. The registration differs:

**cobra**: Imperative `AddCommand()`:

```go
root.AddCommand(server)
server.AddCommand(start, stop)
```

**urfave/cli**: Declarative slice:

```go
Commands: []*cli.Command{{Name: "start"}, {Name: "stop"}}
```

**kong**: Struct nesting:

```go
type CLI struct {
    Server struct {
        Start StartCmd `cmd:""`
        Stop  StopCmd  `cmd:""`
    } `cmd:""`
}
```

**bjaus/cli**: Struct embedding or interface method:

```go
// Struct embedding — fields implementing Commander become subcommands
type ServerCmd struct {
    Start StartCmd  // implements Commander → subcommand
    Stop  StopCmd   // implements Commander → subcommand
}

// Or via interface for dynamic commands
func (s *ServerCmd) Subcommands() []cli.Commander {
    return []cli.Commander{&PluginCmd{}}
}
```

Both approaches can be combined. Name collisions between embedded and Subcommander
return an error — the developer must fix the duplicate.

### Advanced Subcommand Features

| Feature             | cobra                    | urfave/cli                     | kong           | bjaus/cli               |
| ------------------- | ------------------------ | ------------------------------ | -------------- | ----------------------- |
| Prefix matching     | `TraverseChildren`       | Built-in                       | -              | `WithPrefixMatching`    |
| Case-insensitive    | -                        | -                              | -              | `WithCaseInsensitive`   |
| Default subcommand  | -                        | `DefaultCommand`               | `default:"1"`  | `Fallbacker` interface  |
| Command suggestions | Jaro-Winkler             | Jaro-Winkler (`Suggest: true`) | -              | Jaro-Winkler (built-in) |
| Command groups      | `GroupID` + `AddGroup()` | `Category` field               | `group:""` tag | `Category()` method     |
| Flags anywhere      | `TraverseChildren`       | No                             | No             | Yes (default)           |

bjaus/cli enables **flags-anywhere** by default: `app --verbose serve --port 8080` and
`app serve --port 8080 --verbose` both work. At each level of the command tree, the
resolver scans args looking for known flags (consuming them) and subcommand names.
Other frameworks either require flags after the owning subcommand or need explicit
opt-in.

---

## 6. Lifecycle Hooks & Middleware

### cobra

Six hook points on the `Command` struct, in execution order:

```
PersistentPreRun(E) → PreRun(E) → Run(E) → PostRun(E) → PersistentPostRun(E)
```

`Persistent*` hooks are inherited by all subcommands. With `EnableTraverseRunHooks`,
all parent hooks execute (not just the immediate parent's).

All hooks receive `(cmd *cobra.Command, args []string)`. The `E` variants return
`error`.

**Limitations**: Hooks cannot modify context or pass data forward. Hooks cannot
inspect the target leaf command. No middleware wrapping pattern.

### urfave/cli

Two hook points:

```
Before → Action → After
```

- `Before` returns `(context.Context, error)` — can modify context, abort on error
- `After` always runs, even if `Action` panics

Additionally, **per-flag actions** fire after flag parsing:

```go
&cli.StringFlag{
    Name: "config",
    Action: func(ctx context.Context, cmd *cli.Command, v string) error {
        return loadConfig(v) // Runs after --config is parsed
    },
}
```

### kong

Five hook points via interfaces:

```
BeforeReset → BeforeResolve → BeforeApply → [parse] → AfterApply → [run] → AfterRun
```

Hooks are methods on command structs. Parameters are dependency-injected:

```go
func (s *ServeCmd) BeforeApply(ctx *kong.Context) error {
    // Runs before CLI args are applied to this struct
    return nil
}
```

### bjaus/cli

Two hook points via interfaces, plus middleware:

```
Before (parent→child) → Middleware(Run) → After (child→parent)
```

- `Before` returns `(context.Context, error)` — context flows forward through the
  chain
- `After` always runs, even on error (child-first order)
- `Middlewarer` returns `[]func(next RunFunc) RunFunc` — wraps the leaf's `Run`

Unique feature: **leaf inspection in Before hooks**. `cli.Leaf(ctx)` returns the
resolved target command, enabling centralized cross-cutting logic:

```go
func (r *Root) Before(ctx context.Context) (context.Context, error) {
    if _, ok := cli.Leaf(ctx).(Authenticated); ok {
        return authenticate(ctx)
    }
    return ctx, nil
}
```

This pattern enables interface-based routing in parent hooks — checking if the leaf
command implements a custom interface (like `Authenticated`) and acting accordingly.
Cobra requires `PersistentPreRun` without knowing which leaf will run.

### Comparison

| Feature              | cobra               | urfave/cli        | kong      | bjaus/cli          |
| -------------------- | ------------------- | ----------------- | --------- | ------------------ |
| Hook points          | 6                   | 2 + per-flag      | 5         | 2 + middleware     |
| Context modification | No                  | Yes (Before)      | Via DI    | Yes (Before)       |
| Always-run cleanup   | PersistentPostRun   | After             | AfterRun  | After              |
| Middleware wrapping  | No                  | No                | No        | Yes                |
| Leaf inspection      | No                  | No                | No        | Yes (`cli.Leaf`)   |
| Hook inheritance     | Persistent variants | Root Before/After | On struct | Parent implements  |
| Per-flag callbacks   | No                  | Yes               | No        | No                 |
| DI in hooks          | No                  | No                | Yes       | Yes (bound values) |

---

## 7. Help Generation

### cobra

Auto-generated from `Use`, `Short`, `Long`, `Example` fields. Uses Go templates
internally, fully customizable:

```go
cmd.SetHelpFunc(customFunc)
cmd.SetHelpTemplate(templateString)
cmd.SetUsageFunc(customFunc)
cmd.SetUsageTemplate(templateString)
cobra.AddTemplateFunc("highlight", highlightFunc)
```

Help subcommand (`help`) and `--help` flag are auto-added. Command groups organize
subcommands under headings. Template customization is powerful but requires
understanding Go's `text/template` system.

### urfave/cli

Auto-generated with template system. Three templates: `RootCommandHelpTemplate`,
`CommandHelpTemplate`, `SubcommandHelpTemplate`. Customizable via:

```go
cli.HelpPrinter = customPrinterFunc
cmd.HideHelp = true
```

Supports flag/command categories for grouped output.

### kong

Auto-generated from struct tags. Supports variable interpolation in help strings
(`${default}`, `${enum}`, `${env}`). Configurable via `HelpOptions`:

```go
kong.Parse(&CLI, kong.ConfigureHelp(kong.HelpOptions{
    Compact:   true,
    Tree:      true,
    FlagsLast: true,
}))
```

Custom help via `Help(HelpPrinter)` option for full control.

### bjaus/cli

Default renderer produces formatted output with multiple customization points:

- `Helper` interface: override help text entirely per command
- `HelpRenderer` interface: custom rendering engine (per-command or global)
- `HelpAppender` / `HelpPrepender`: add sections without replacing the renderer
- `LongDescriber`: multi-paragraph descriptions in help mode
- `WithSortedHelp`: alphabetical sorting for subcommands and flags
- Variable interpolation: `${default}`, `${enum}`, `${env}` in help strings
- `HelpCommand(root, w)`: pre-built hidden `help` subcommand

Sections in order: description, prepended sections, usage, examples, subcommands
(by category), flags (by category, hidden filtered, required marked with `*`),
positional args, appended sections, global flags, footer.

### Comparison

| Feature                | cobra           | urfave/cli     | kong               | bjaus/cli                               |
| ---------------------- | --------------- | -------------- | ------------------ | --------------------------------------- |
| Auto `--help`          | Yes             | Yes            | Yes                | Yes                                     |
| Auto `help` subcommand | Yes             | Yes            | No                 | Yes (`HelpCommand`)                     |
| Template system        | Go templates    | Go templates   | No                 | No                                      |
| Variable interpolation | No              | No             | Yes (`${default}`) | Yes (`${default}`, `${enum}`, `${env}`) |
| Compact/Tree modes     | No              | No             | Yes                | No                                      |
| Section injection      | No              | No             | No                 | Yes (Prepend/Append)                    |
| Per-command override   | SetHelpFunc     | HelpFunc field | Help interface     | Helper interface                        |
| Global override        | SetHelpTemplate | HelpPrinter    | Help option        | WithHelpRenderer                        |
| Required flag marker   | No              | No             | No                 | Yes (`*`)                               |
| Flag categories        | No              | Yes            | Yes                | Yes                                     |
| Command categories     | Yes (GroupID)   | Yes (Category) | Yes (group)        | Yes (Category)                          |
| Global flags section   | Yes             | Yes            | No                 | Yes                                     |

---

## 8. Shell Completion

### cobra

Best-in-class completion. Supports Bash, Zsh, Fish, PowerShell.

**Static**: `ValidArgs` and `ArgAliases` fields.
**Dynamic**: `ValidArgsFunction` with typed `Completion` returns.
**Flag-level**: `RegisterFlagCompletionFunc` per flag.
**Descriptions**: `CompletionWithDesc("name", "description")`.
**Active help**: Inject help messages into completion flow.
**Directives**: `ShellCompDirectiveNoSpace`, `NoFileComp`, `FilterFileExt`,
`FilterDirs`, `KeepOrder`, `Error`.

Hidden `__complete` command handles runtime requests. Debug via
`app __complete cmd arg`.

### urfave/cli

Supports Bash, Zsh, Fish, PowerShell via `EnableShellCompletion: true`.

Generate scripts with `app completion bash|zsh|fish|powershell`.
Custom completions via `ShellCompleteFunc` per command.

**Limitation**: Custom completion functions don't receive the user's partial input.

### kong

**No built-in shell completion.** Third-party [kongplete] provides completion via
`posener/complete`. This is a notable gap compared to the other frameworks.

[kongplete]: https://github.com/willabides/kongplete

### bjaus/cli

Built-in runtime completion via hidden `__complete` protocol. Supports Bash, Zsh,
Fish, PowerShell.

**Sources**: Custom completions via `Completer` interface, dynamic flag value
completion via `FlagCompleter` interface, static flag completion (including negatable,
alt names, short flags), enum value completion, subcommand name + alias completion.

**Directives**: `ShellCompDirectiveNoSpace`, `NoFileComp`, `Error`, `FilterFileExt`,
`FilterDirs`.

**Pre-built command**: `completion.Command(root, appName, os.Stdout)` returns a
hidden `Commander` with bash/zsh/fish/powershell subcommands for script generation.

### Comparison

| Feature                   | cobra        | urfave/cli    | kong        | bjaus/cli                    |
| ------------------------- | ------------ | ------------- | ----------- | ---------------------------- |
| Built-in                  | Yes          | Yes           | **No**      | Yes                          |
| Shells                    | 4            | 4             | (3rd party) | 4                            |
| Dynamic completions       | Yes          | Yes (limited) | (3rd party) | Yes                          |
| Completion descriptions   | Yes          | No            | (3rd party) | Yes (Zsh/Fish/PS)            |
| Active help in completion | Yes          | No            | No          | No                           |
| Flag-level completion     | Yes          | No            | (3rd party) | Yes (enum + `FlagCompleter`) |
| Directive control         | 7 directives | Basic         | -           | 5 directives                 |
| Debug support             | `__complete` | No            | -           | `__complete`                 |
| Pre-built command         | -            | Built-in      | -           | `completion.Command()`       |

---

## 9. Error Handling

### cobra

Two callback patterns: `Run` (no error) and `RunE` (returns error). Errors from
`RunE` propagate to `Execute()`. Control:

- `SilenceErrors`: suppress automatic error printing
- `SilenceUsage`: suppress usage on error
- `SetFlagErrorFunc`: custom flag parsing error handling

No exit code control beyond conventional `os.Exit(1)`.

### urfave/cli

`ExitCoder` interface with `ExitCode() int`. Create with `cli.Exit("msg", code)`.

- `ExitErrHandler` override per command
- `OsExiter` global override (useful for testing)
- `MultiError` support — uses last `ExitCoder`'s code
- `Run()` returns error, doesn't call `os.Exit` directly

### kong

`ParseError` with `Unwrap()` and `ExitCode()`. Options:

- `UsageOnError()` / `ShortUsageOnError()` — usage display
- `Exit(func(int))` — override exit behavior
- Hook errors propagate through the lifecycle

### bjaus/cli

Sentinel errors for common failures: `ErrUnknownFlag`, `ErrFlagRequiresVal`,
`ErrRequiredFlag`, `ErrUnsupportedType`, `ErrInvalidFlagValue`, `ErrInvalidTag`.

`ExitCoder` interface with `ExitCode() int`. Create with `cli.Exit("msg", code)` or
`cli.Exitf("msg %s", arg, code)`.

`ErrShowHelp` — return from `Run` to trigger help display without error.

`Exiter` interface on root — custom error printing and process exit.

`Execute` returns error (testable); `ExecuteAndExit` handles exit codes.

### Comparison

| Feature               | cobra | urfave/cli  | kong            | bjaus/cli              |
| --------------------- | ----- | ----------- | --------------- | ---------------------- |
| Custom exit codes     | No    | `ExitCoder` | `ExitCode()`    | `ExitCoder`            |
| Sentinel errors       | No    | No          | `ParseError`    | 7 sentinel errors      |
| "Show help" error     | No    | No          | No              | `ErrShowHelp`          |
| Silence control       | Yes   | No          | No              | No                     |
| Custom exit behavior  | No    | `OsExiter`  | `Exit()` option | `Exiter` interface     |
| Execute returns error | Yes   | Yes         | Yes             | Yes                    |
| Strict tag validation | No    | No          | No              | Yes (catches bad tags) |

bjaus/cli's **strict tag validation** catches invalid struct tag combinations at parse
time (e.g., `flag` + `arg` on same field, `required` + `default`, `counter` on
non-int). This prevents developer mistakes at development time rather than producing
confusing runtime behavior.

---

## 10. Configuration

### cobra + Viper

Cobra itself has no configuration system. It is designed to work with [Viper]:

```go
viper.BindPFlag("port", cmd.Flags().Lookup("port"))
viper.SetEnvPrefix("APP")
viper.AutomaticEnv()
viper.SetConfigFile("config.yaml")
viper.ReadInConfig()
port := viper.GetInt("port") // Resolved by precedence
```

Precedence: CLI flag > env var > config file > default.

**Friction**: After binding a flag to Viper, the original flag variable is NOT
automatically populated. You must use `viper.Get*()` to read values. This is a
frequent source of bugs.

[Viper]: https://github.com/spf13/viper

### urfave/cli

**Built-in**: `cli.EnvVars()` and `cli.File()` value sources with explicit
precedence chain:

```go
Sources: cli.NewValueSourceChain(
    cli.EnvVars("APP_PORT"),
    cli.File("/etc/app/port"),
)
```

**Extended**: `urfave/cli-altsrc/v3` provides YAML, JSON, TOML loaders as a separate
module.

### kong

Built-in environment variable support via `env:""` tag. File-based configuration
via `Configuration(loader, paths...)` option with built-in JSON loader and
extensible `Resolver` interface for YAML, TOML, HCL, etc.

### bjaus/cli

`ConfigResolver` function type: `func(key ConfigKey) (value string, found bool)`.

`ConfigProvider` interface per command; `WithConfigResolver` option for global.

`ConfigKey` provides both flat `Name` and decomposed `Parts` for nested formats:
a flag `--db-host` (from `prefix:"db-"`) yields `Parts: ["db", "host"]`.

Built-in `config` subpackage:

- `FromMap(map[string]string)` — universal adapter
- `FromJSON(io.Reader)` — flat JSON objects
- `FromEnvFile(io.Reader)` — .env file parser (zero dependencies)
- `Chain(resolvers...)` — first match wins

Docs include copy-paste adapters for YAML, TOML, HCL, and Consul.

Priority: `CLI flag > env var > config > default > zero value`.

### Comparison

| Feature              | cobra            | urfave/cli         | kong               | bjaus/cli                  |
| -------------------- | ---------------- | ------------------ | ------------------ | -------------------------- |
| Built-in env vars    | No (Viper)       | Yes                | Yes                | Yes                        |
| Built-in file config | No (Viper)       | Plain text only    | JSON               | JSON                       |
| Extended formats     | Via Viper        | altsrc module      | External resolvers | Documented adapters        |
| Priority control     | Via Viper        | Source chain order | Tag order          | Hardcoded chain            |
| Nested key support   | Via Viper        | No                 | Resolver-dependent | `ConfigKey.Parts`          |
| Per-command config   | No               | No                 | No                 | `ConfigProvider` interface |
| Config in framework  | Separate library | Core + module      | Core               | Core                       |

---

## 11. Dependency Injection

### cobra

No built-in DI. Dependencies are passed via closures in factory functions:

```go
func NewServerCmd(db *sql.DB) *cobra.Command {
    return &cobra.Command{
        Use: "server",
        RunE: func(cmd *cobra.Command, args []string) error {
            return startServer(db) // db via closure
        },
    }
}
```

Works with external DI containers (dig, fx, wire) but requires manual integration.

### urfave/cli

No built-in DI. Dependencies passed via `context.Context` values or closures:

```go
Action: func(ctx context.Context, cmd *cli.Command) error {
    db := ctx.Value(dbKey).(*sql.DB)
    return startServer(db)
}
```

### kong

Sophisticated built-in DI system:

- `Bind(values...)` — bind by concrete type
- `BindTo(impl, iface)` — bind implementation to interface
- `BindToProvider(func)` — bind via factory function
- `BindSingletonProvider(func)` — singleton factory, called once
- `Context.Bind()` — runtime binding in hooks
- `Provide<Type>() error` — methods on structs contribute to DI

Injected into `Run()`, hook methods, and any method accepting bound types.

### bjaus/cli

Built-in DI via execution options:

- `Bind(v)` — register value matched by concrete type
- `BindTo(v, iface)` — register as interface: `cli.BindTo(myDB, (*Database)(nil))`
- `BindProvider[T](func() (T, error))` — lazy factory called per execution
- `BindSingleton[T](func() (T, error))` — singleton factory, called once and cached

Injected into struct fields across the entire command chain. Fields with `flag:`,
`arg:`, or `env:` tags are skipped. Matching: exact type first, then interface check.

`cli.Args` (positional arguments) is auto-bound so commands can declare an
`Args cli.Args` field.

### Comparison

| Feature             | cobra | urfave/cli | kong                 | bjaus/cli             |
| ------------------- | ----- | ---------- | -------------------- | --------------------- |
| Built-in DI         | No    | No         | Yes                  | Yes                   |
| Type binding        | -     | -          | Yes                  | Yes                   |
| Interface binding   | -     | -          | Yes (`BindTo`)       | Yes (`BindTo`)        |
| Provider functions  | -     | -          | Yes                  | Yes (`BindProvider`)  |
| Singleton providers | -     | -          | Yes                  | Yes (`BindSingleton`) |
| Runtime binding     | -     | -          | Yes (`Context.Bind`) | No                    |
| Auto-bound types    | -     | -          | `*Context`, `*Kong`  | `cli.Args`            |
| Inject into hooks   | -     | -          | Yes                  | Yes (bound values)    |

---

## 12. Plugin System

### cobra

No built-in plugin system. The community pattern (used by kubectl, docker, helm) is
PATH-based discovery of `<prefix>-<name>` executables. This must be implemented
manually.

### urfave/cli

No built-in plugin system.

### kong

Static plugin support via `kong.Plugins` embed type. Dynamic commands via
`DynamicCommand()` option. No external executable discovery.

### bjaus/cli

First-class plugin system with multiple discovery mechanisms:

**`Discoverer` interface**: `Discover() ([]Commander, error)` for runtime-discovered
commands.

**`Discover` function**: Scans directories and optionally `PATH` for plugin
executables.

**Plugin protocol**: Each plugin is queried with `--cli-info` (customizable) for
optional JSON metadata:

```json
{ "name": "deploy", "description": "Deploy to cloud", "aliases": ["d"] }
```

Plugins that don't support the flag still work — they just get a name derived from
the filename.

**`ExternalCommand`**: Wraps an external executable as a `Commander`, wiring
stdin/stdout/stderr to the parent process.

**Directory scanning**: All executables in configured directories become plugins.
`DefaultDirs(name)` returns conventional paths: `./name/plugins`,
`~/.config/name/plugins`, `/etc/name/plugins`.

**PATH scanning**: Executables matching `<prefix>-<command>` on PATH are discovered.

**Priority**: Directory-discovered > PATH-discovered. Built-in commands always win
on name collision.

Shell completion automatically includes discovered plugins.

### Comparison

| Feature                | cobra  | urfave/cli | kong               | bjaus/cli              |
| ---------------------- | ------ | ---------- | ------------------ | ---------------------- |
| Built-in plugins       | No     | No         | Static only        | Yes (full)             |
| External executables   | Manual | No         | No                 | Yes                    |
| PATH discovery         | Manual | No         | No                 | Yes                    |
| Directory discovery    | Manual | No         | No                 | Yes                    |
| Metadata protocol      | -      | -          | -                  | JSON via `--cli-info`  |
| Priority rules         | -      | -          | -                  | Built-in > dir > PATH  |
| Completion integration | -      | -          | -                  | Automatic              |
| Dynamic commands       | -      | -          | `DynamicCommand()` | `Discoverer` interface |

---

## 13. Testing

### cobra

```go
func TestServer(t *testing.T) {
    buf := new(bytes.Buffer)
    cmd := newServerCmd()
    cmd.SetOut(buf)
    cmd.SetErr(buf)
    cmd.SetArgs([]string{"start", "--port", "9090"})
    err := cmd.Execute()
    assert.NoError(t, err)
    assert.Contains(t, buf.String(), "listening on :9090")
}
```

- `SetArgs()` overrides `os.Args`
- `SetOut()` / `SetErr()` / `SetIn()` redirect I/O
- `Execute()` returns error (no `os.Exit`)
- No global state to clean up

**Weakness**: Flag variables bound to package-level vars complicate parallel tests.

### urfave/cli

```go
func TestGreet(t *testing.T) {
    cmd := &cli.Command{
        Name:   "greet",
        Action: greetAction,
    }
    err := cmd.Run(context.Background(), []string{"greet", "--name", "Alice"})
    assert.NoError(t, err)
}
```

- `Run()` returns error (no `os.Exit`)
- `context.WithTimeout` prevents hanging tests
- Override `OsExiter` for exit code testing
- No built-in output capture — requires `os.Pipe()` or similar

### kong

```go
func TestServe(t *testing.T) {
    var cli struct {
        Serve ServeCmd `cmd:""`
    }
    var buf bytes.Buffer
    parser := kong.Must(&cli,
        kong.Exit(func(int) { t.Fatal("unexpected exit") }),
        kong.Writers(&buf, &buf),
    )
    ctx, err := parser.Parse([]string{"serve", "--port", "9090"})
    assert.NoError(t, err)
    assert.Equal(t, 9090, cli.Serve.Port)
}
```

- `Exit()` overrides termination
- `Writers()` redirects output
- Direct struct inspection after parse (no need to capture output)
- `kong.New()` for incremental testing

### bjaus/cli

```go
func TestServe(t *testing.T) {
    var buf bytes.Buffer
    serve := &ServeCmd{}
    root := &RootCmd{serve: serve}
    err := cli.Execute(
        context.Background(), root,
        []string{"serve", "--port", "9090"},
        cli.WithStdout(&buf),
    )
    assert.NoError(t, err)
    assert.Equal(t, 9090, serve.Port)
}
```

- `Execute` returns error (no `os.Exit`)
- `WithStdout` / `WithStderr` / `WithStdin` redirect I/O
- Direct struct inspection (flag values on the struct)
- No global state — each test creates its own command tree
- Safe for `t.Parallel()`
- `ExecuteAndExit` tested via subprocess pattern

### Comparison

| Feature              | cobra               | urfave/cli          | kong          | bjaus/cli               |
| -------------------- | ------------------- | ------------------- | ------------- | ----------------------- |
| Returns error        | Yes                 | Yes                 | Yes           | Yes                     |
| I/O redirection      | SetOut/SetErr/SetIn | Manual              | Writers()     | WithStdout/Stderr/Stdin |
| Direct struct access | No (flag vars)      | No (getter methods) | Yes           | Yes                     |
| Parallel-safe        | Conditional         | Yes                 | Yes           | Yes                     |
| No global state      | Mostly              | Yes                 | Yes           | Yes                     |
| Exit override        | No                  | OsExiter            | Exit() option | Exiter interface        |

---

## 14. Documentation Generation

| Feature          | cobra                   | urfave/cli        | kong | bjaus/cli                               |
| ---------------- | ----------------------- | ----------------- | ---- | --------------------------------------- |
| Man pages        | `doc.GenManTree()`      | `cli-docs` module | No   | `doc.Man()` / `doc.ManTree()`           |
| Markdown         | `doc.GenMarkdownTree()` | `cli-docs` module | No   | `doc.Markdown()` / `doc.MarkdownTree()` |
| YAML             | `doc.GenYamlTree()`     | No                | No   | No                                      |
| reStructuredText | `doc.GenReSTTree()`     | No                | No   | No                                      |
| Tabular markdown | No                      | `cli-docs` module | No   | No                                      |
| Includes plugins | No                      | No                | N/A  | Yes                                     |
| In core          | Via dependency          | Separate module   | No   | `doc` subpackage                        |

---

## 15. Feature Matrix

| Feature                   | cobra              | urfave/cli         | kong             | bjaus/cli                 |
| ------------------------- | ------------------ | ------------------ | ---------------- | ------------------------- |
| **Runtime dependencies**  | 4                  | 0                  | 0                | 0                         |
| **Flags via struct tags** | No                 | No                 | Yes              | Yes                       |
| **Typed positional args** | No                 | Yes (v3)           | Yes              | Yes                       |
| **Flags anywhere**        | Opt-in             | No                 | No               | Yes (default)             |
| **Auto flag inheritance** | Persistent flags   | Root flags inherit | Embed            | By name+type match        |
| **Flag groups**           | 3 types            | 1 type             | 2 types          | 3 types                   |
| **Env var binding**       | Via Viper          | Built-in           | Built-in         | Built-in                  |
| **Env-only fields**       | No                 | No                 | No               | Yes                       |
| **Shell completion**      | Best-in-class      | Good               | 3rd party        | Full (5 directives)       |
| **Built-in DI**           | No                 | No                 | Yes (advanced)   | Yes (providers+singleton) |
| **Plugin system**         | No                 | No                 | Static           | Full (external exec)      |
| **Config files**          | Via Viper          | altsrc module      | Built-in JSON    | JSON + .env               |
| **Middleware**            | No                 | No                 | No               | Yes                       |
| **Leaf inspection**       | No                 | No                 | No               | Yes                       |
| **Interactive prompts**   | No                 | No                 | No               | Yes                       |
| **Doc generation**        | 4 formats          | 2 formats          | No               | 2 formats                 |
| **Strict tag validation** | No                 | No                 | No               | Yes                       |
| **"Did you mean?"**       | Yes                | Opt-in             | No               | Yes                       |
| **Negatable bools**       | No                 | Yes                | Yes              | Yes                       |
| **Counter flags**         | Yes                | No                 | Yes              | Yes                       |
| **Flag auto-naming**      | No                 | No                 | Yes              | Yes                       |
| **Secret masking**        | No                 | No                 | No               | Yes                       |
| **`ErrShowHelp`**         | No                 | No                 | No               | Yes                       |
| **Passthrough mode**      | DisableFlagParsing | No                 | `passthrough:""` | `Passthrougher`           |

---

## 16. Verdict

### When to Use Each

**spf13/cobra** is the right choice when:

- You need the most battle-tested option (kubectl, docker, gh, helm, hugo)
- Shell completion quality is critical (active help, 7 directives)
- You want the largest ecosystem and community knowledge base
- You need doc generation in 4+ formats
- You're building a CLI that will be extended by many contributors who likely
  already know cobra

**urfave/cli** is the right choice when:

- You want zero runtime dependencies with a simple, declarative API
- Binary size matters
- You prefer typed flag structs over method calls
- Per-flag validation and actions are important
- You want progressive complexity — start minimal, add features later

**alecthomas/kong** is the right choice when:

- You want minimal boilerplate — the struct _is_ the CLI spec
- You need advanced DI (runtime binding, interface binding)
- Branching positional arguments match your command model
- You prefer tag-driven configuration over imperative wiring
- Shell completion is not critical (or you're willing to use kongplete)

**bjaus/cli** is the right choice when:

- You want interface composition over monolithic structs
- Plugin discovery for external executables is important
- You need flags-anywhere behavior by default
- Middleware wrapping and leaf inspection in hooks matter
- You want env-only fields for secrets
- You prefer small, composable interfaces (`io.Reader` philosophy)
- Strict tag validation to catch developer mistakes early is valuable
- Interactive prompting for missing flags is needed

### Trade-offs at a Glance

| Priority                       | Best Choice                              |
| ------------------------------ | ---------------------------------------- |
| Community & ecosystem          | cobra                                    |
| Zero dependencies + simplicity | urfave/cli                               |
| Minimal boilerplate            | kong                                     |
| Interface composition          | bjaus/cli                                |
| Shell completion quality       | cobra                                    |
| Plugin system                  | bjaus/cli                                |
| DI sophistication              | kong or bjaus/cli                        |
| Flag flexibility               | bjaus/cli                                |
| Testing ergonomics             | kong or bjaus/cli (direct struct access) |
| Configuration management       | cobra + Viper                            |
