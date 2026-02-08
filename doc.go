// Package cli is a composable CLI framework built on small interfaces.
//
// Commands are Go types that implement [Runner]. The framework discovers
// capabilities through type assertions on optional interfaces — there is
// no base struct to embed and no configuration DSL.
//
// # Core Interface
//
// Every command must implement [Runner]:
//
//	type Runner interface {
//	    Run(ctx context.Context, args []string) error
//	}
//
// [RunFunc] adapts a plain function into a Runner for simple cases.
//
// # Discovery Interfaces
//
// Implement any combination of these optional interfaces to declare
// metadata, subcommands, aliases, examples, and hidden status:
//
//   - [Namer] — override the command name (default: lowercase struct type name)
//   - [Describer] — provide a one-line description
//   - [Aliaser] — declare alternate names
//   - [Parent] — return subcommands
//   - [Hider] — hide the command from help output
//   - [Exampler] — provide usage examples
//
// # Lifecycle Interfaces
//
//   - [BeforeRunner] — run setup logic before Run (parent-first), returns modified context
//   - [AfterRunner] — run teardown logic after Run (child-first, always runs)
//   - [Validator] — validate state after flag parsing, before Run
//
// # Flags
//
// The default flag parser reads struct tags:
//
//	type ServeCmd struct {
//	    Port int    `flag:"port" short:"p" default:"8080" help:"Port to listen on" env:"PORT"`
//	    Host string `flag:"host" default:"localhost" help:"Host to bind to"`
//	}
//
// Supported types: string, int, int64, float64, bool, time.Duration,
// and any type implementing [FlagUnmarshaler].
//
// Priority: explicit flag > env var > default > zero value.
//
// Flags can appear anywhere — before or after subcommand names.
//
// # Extensibility
//
// Every major subsystem is replaceable:
//
//   - [FlagParser] — replace the flag parsing engine per-command or globally
//   - [HelpRenderer] — replace help rendering per-command or globally
//   - [Helper] — override help text for a single command
//   - [Middlewarer] — wrap the run function with middleware
//   - [Suggester] — custom "did you mean?" per-command
//
// Global overrides are set via [Option] functions passed to [Execute]:
//
//	cli.Execute(ctx, root, os.Args[1:],
//	    cli.WithStdout(os.Stdout),
//	    cli.WithFlagParser(myParser),
//	)
package cli
