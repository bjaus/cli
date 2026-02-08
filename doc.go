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
//   - [Versioner] — report a version string via --version / -V
//   - [Deprecater] — mark a command as deprecated with a warning message
//   - [Categorizer] — group subcommands under headings in help output
//   - [Fallbacker] — provide a fallback subcommand when no name matches
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
//	    Port    int           `flag:"port" short:"p" default:"8080" help:"Port to listen on" env:"PORT"`
//	    Host    string        `flag:"host" default:"localhost" help:"Host to bind to"`
//	    Tags    []string      `flag:"tag" short:"t" help:"Tags to apply (repeatable)"`
//	    Env     map[string]string `flag:"env" help:"Environment variables as key=value"`
//	    Format  string        `flag:"format" enum:"text,json,yaml" default:"text" help:"Output format"`
//	    Verbose int           `flag:"verbose" short:"v" counter:"true" help:"Increase verbosity"`
//	    Color   bool          `flag:"color" default:"true" negatable:"true" help:"Colorize output"`
//	}
//
// Supported types: string, int, int64, float64, bool, time.Duration,
// slices of any scalar type, map[string]string, and any type implementing
// [FlagUnmarshaler].
//
// Struct tag keys:
//
//   - flag — the flag name (required to register the field as a flag)
//   - short — single-character short form
//   - default — default value if not provided
//   - help — description shown in help output
//   - env — environment variable to read from (overrides default, overridden by explicit flag)
//   - enum — comma-separated list of allowed values
//   - required — "true" to require the flag
//   - counter — "true" to increment an int on each occurrence (-vvv)
//   - negatable — "true" to add a --no- prefix that sets a bool to false
//
// Priority: explicit flag > env var > default > zero value.
//
// Flags can appear anywhere — before or after subcommand names.
//
// # Inheritance
//
// Flags can flow from parent commands to child subcommands automatically.
// Two complementary mechanisms are supported:
//
// Automatic flag inheritance: when a parent and child both declare a flag
// with the same name and type, the child inherits the parent's parsed value
// if the child's flag was not explicitly provided via CLI args or env var.
// The child's flag still appears in help output and accepts CLI args normally.
//
//	type App struct {
//	    Env string `flag:"env" required:"true" enum:"dev,qa,prod" help:"Target environment"`
//	}
//	type ServeCmd struct {
//	    Env  string `flag:"env" help:"Target environment"`
//	    Port int    `flag:"port" default:"8080" help:"Listen port"`
//	}
//
// Inherit tag: a child field tagged with `inherit:"flagname"` receives the
// value from the nearest ancestor's matching flag without registering its
// own CLI flag. It does not appear in help output and does not accept CLI args.
//
//	type ServeCmd struct {
//	    Env  string `inherit:"env"`
//	    Port int    `flag:"port" default:"8080" help:"Listen port"`
//	}
//
// Priority for automatic flag inheritance:
// explicit child flag > child env var > inherited from parent > child default > zero value.
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
//	    cli.WithShortOptionHandling(true),
//	    cli.WithPrefixMatching(true),
//	)
package cli
