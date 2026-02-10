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
//   - [Deprecator] — mark a command as deprecated with a warning message
//   - [Categorizer] — group subcommands under headings in help output
//   - [Fallbacker] — provide a fallback subcommand when no name matches
//   - [Exiter] — control error printing and process exit in [ExecuteAndExit]
//   - [Discoverer] — provide runtime-discovered commands (plugins)
//
// # Lifecycle Interfaces
//
//   - [Beforer] — run setup logic before Run (parent-first), returns modified context
//   - [Afterer] — run teardown logic after Run (child-first, always runs)
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
//	    Color   bool          `flag:"color" default:"true" negate:"true" help:"Colorize output"`
//	}
//
// Supported types: string, int, int64, float64, bool, time.Duration,
// slices of any scalar type, map[string]string, and any type implementing
// [FlagUnmarshaler].
//
// Struct tag keys:
//
//   - flag — the flag name (if empty, derived from field name: OutputFormat → output-format)
//   - short — single-character short form
//   - default — default value if not provided
//   - help — description shown in help output
//   - env — environment variable name; standalone (without flag/arg) for env/config/default-only fields
//   - enum — comma-separated list of allowed values
//   - required — "true" to require the flag
//   - counter — "true" to increment an int on each occurrence (-vvv)
//   - negate — "true" to add a --no- prefix that sets a bool to false
//   - alt — comma-separated additional long flag names (e.g. "output,out")
//   - sep — separator for splitting a single value into slice elements (e.g. ",")
//   - hidden — "true" to hide the flag from help output (flag still works)
//   - deprecated — message shown when the flag is used; prints a warning to stderr
//   - category — group heading for the flag in help output
//   - mask — displayed instead of default in help (e.g. "****" for secrets)
//   - placeholder — value name shown in help (e.g. "PORT" in --port PORT)
//   - prefix — flag name prefix for named struct fields (e.g. "db-" yields --db-host)
//
// Priority: explicit flag > env var > config > default > zero value.
//
// Use [WithEnvVarPrefix] to scope all env var lookups under a common prefix.
// For example, WithEnvVarPrefix("APP_") causes `env:"PORT"` to look up APP_PORT.
//
// Flags can appear anywhere — before or after subcommand names.
//
// The framework performs strict tag validation at parse time. Invalid
// combinations (flag + arg, required + default, flag-only tags without
// flag, etc.) return [ErrInvalidTag].
//
// # Env-Only Fields
//
// A field with an env tag but no flag or arg tag is env/config/default-only.
// It does not appear in help output and cannot be passed via command-line
// arguments. This is useful for secrets that should never appear in shell
// history:
//
//	type DeployCmd struct {
//	    Token string `env:"DEPLOY_TOKEN" required:"true"`
//	    Env   string `flag:"env" enum:"prod,staging,dev" help:"Target environment"`
//	}
//
// The logical name is derived from the field name (Token → token) and is used
// for config resolver lookups and context storage via [Set]/[Get].
//
// Priority for env-only fields: env var > config > default > zero value.
//
// # Embedded Structs and Prefix
//
// Anonymous embedded structs have their flags promoted, just like Go's own
// field promotion:
//
//	type OutputFlags struct {
//	    Format string `flag:"format" enum:"json,table" default:"table" help:"Output format"`
//	}
//	type ListCmd struct {
//	    OutputFlags // promoted: --format works as if declared on ListCmd
//	    Limit int   `flag:"limit" default:"50"`
//	}
//
// Named struct fields with the prefix tag namespace their flags:
//
//	type DBFlags struct {
//	    Host string `flag:"host" default:"localhost" help:"Database host"`
//	    Port int    `flag:"port" default:"5432" help:"Database port"`
//	}
//	type ServeCmd struct {
//	    DB   DBFlags `prefix:"db-"`  // --db-host, --db-port
//	    Port int     `flag:"port" default:"8080" help:"Listen port"`
//	}
//
// Prefixes nest: a prefix:"a-" containing prefix:"b-" yields --a-b-name.
// When an outer and embedded field share a flag name, the outer field wins
// (matching Go's own promotion semantics).
//
// # Inheritance
//
// Flags flow from parent commands to child subcommands via automatic flag
// inheritance: when a parent and child both declare a flag with the same
// name and type, the child inherits the parent's parsed value if the
// child's flag was not explicitly provided via CLI args or env var.
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
// To inherit a parent flag without exposing it as a child flag, use a
// hidden flag with the same name:
//
//	type ServeCmd struct {
//	    Env  string `flag:"env" hidden:"true"`
//	    Port int    `flag:"port" default:"8080" help:"Listen port"`
//	}
//
// Priority for automatic flag inheritance:
// explicit child flag > child env var > inherited from parent > child default > zero value.
//
// # Context Values
//
// The framework automatically stores every parsed flag value in the context
// before [Beforer] hooks run. Subcommands can retrieve any ancestor's
// flag value without declaring struct fields:
//
//	func (s *ServeCmd) Run(ctx context.Context, args []string) error {
//	    env := cli.Get[string](ctx, "env") // from parent's --env flag
//	    // ...
//	}
//
// Three functions are provided:
//
//   - [Set] — store a named value in the context (returns new context)
//   - [Get] — retrieve a value by name; returns zero value if missing or type mismatch
//   - [Lookup] — retrieve a value by name; returns (value, ok) for safe checking
//
// User code can also call [Set] in a [Beforer.Before] hook to share
// arbitrary values (database connections, loggers, etc.) with downstream
// commands:
//
//	func (a *App) Before(ctx context.Context) (context.Context, error) {
//	    return cli.Set(ctx, "db", a.db), nil
//	}
//
// This complements struct-based inheritance: use context values when you want
// loose coupling or need to share non-flag data.
//
// # Config
//
// Flag values can be loaded from external configuration sources via a
// [ConfigResolver]. A resolver is a single function:
//
//	type ConfigResolver func(key ConfigKey) (value string, found bool)
//
// Given a [ConfigKey], it returns the string value and whether it was found.
// The key provides both the full flag name ([ConfigKey.Name]) and decomposed
// parts ([ConfigKey.Parts]) for resolvers backed by nested formats.
// The framework handles all type conversion, validation, required checks,
// and enum enforcement — the resolver only needs to return strings.
//
// Priority chain: explicit CLI flag > env var > config > default > zero value.
//
// Set a global resolver via [WithConfigResolver]:
//
//	f, _ := os.Open("config.json")
//	resolver, _ := config.FromJSON(f)
//	cli.Execute(ctx, root, os.Args[1:],
//	    cli.WithConfigResolver(resolver),
//	)
//
// Or implement [ConfigProvider] on a command for per-command resolvers:
//
//	func (c *ServeCmd) ConfigResolver() cli.ConfigResolver {
//	    return config.FromMap(map[string]string{"port": "9090"})
//	}
//
// Command-level resolvers take priority over the global resolver. Use
// [config.Chain] to try multiple sources in order:
//
//	resolver := config.Chain(
//	    config.FromMap(overrides),
//	    jsonResolver,
//	)
//
// The [config] subpackage ships [config.FromMap], [config.FromJSON], and
// [config.Chain]. Because [ConfigResolver] is a plain function and
// [config.FromMap] accepts any map[string]string, adding support for any
// configuration format — YAML, TOML, HCL, .env files, remote stores —
// is a matter of decoding into a map and calling [config.FromMap].
// See the [config] package documentation for copy-paste adapter examples.
//
// # Flag Groups
//
// Flags can be constrained to enforce relationships using [FlagGrouper]:
//
//	func (c *Cmd) FlagGroups() []cli.FlagGroup {
//	    return []cli.FlagGroup{
//	        cli.MutuallyExclusive("json", "yaml", "text"),
//	        cli.RequiredTogether("username", "password"),
//	        cli.OneRequired("file", "stdin"),
//	    }
//	}
//
// Three constraint kinds are supported:
//
//   - [MutuallyExclusive] — at most one flag in the group may be set
//   - [RequiredTogether] — if any flag in the group is set, all must be set
//   - [OneRequired] — exactly one flag in the group must be set
//
// Validation runs after flag parsing and inheritance, before [Validator].
//
// # Plugins
//
// Commands can be extended at runtime with external executables (plugins).
// A command that implements [Discoverer] provides additional subcommands
// discovered at runtime, merged with any static subcommands from [Parent].
// Built-in commands always take priority on name collisions.
//
// The [Discover] function scans directories and optionally the system
// PATH for plugin executables:
//
//	func (a *App) Discover() ([]cli.Runner, error) {
//	    return cli.Discover("myapp",
//	        cli.WithDirs(cli.DefaultDirs("myapp")...),
//	        cli.WithPATH(),
//	    )
//	}
//
// [DefaultDirs] returns conventional plugin directories in priority order:
//
//  1. ./<name>/plugins — project-level (highest priority)
//  2. $HOME/.config/<name>/plugins — user-level
//  3. /etc/<name>/plugins — system-level (Unix only)
//
// These paths are configurable. Use [WithDir] and [WithDirs] to specify
// custom directories in any order, and [WithPATH] to optionally scan PATH
// for executables matching "<prefix>-<command>".
//
// # Plugin Metadata Protocol
//
// When a plugin is discovered, the framework executes it with the --cli-info
// flag (customizable via [WithInfoFlag]) and expects optional JSON:
//
//	{"name":"deploy","description":"Deploy to cloud","aliases":["d"]}
//
// If the plugin does not support the flag or returns invalid JSON, it still
// works — it just has no description or aliases in help output. The only
// requirement for a plugin is that it be an executable file.
//
// This means plugins can be written in any language:
//
//	#!/bin/bash
//	if [ "$1" = "--cli-info" ]; then
//	    echo '{"name":"deploy","description":"Deploy to cloud environments"}'
//	    exit 0
//	fi
//	echo "deploying to $1..."
//
// # Plugin Discovery Modes
//
// Directory-based discovery (primary): all executable files in the scanned
// directories become plugins. The filename is the command name.
//
// PATH-based discovery (via [WithPATH]): executables matching "<prefix>-*"
// on the system PATH are discovered. The prefix and hyphen are stripped to
// derive the command name (e.g., "myapp-deploy" → "deploy").
//
// Priority: directories are scanned first in the order added. PATH results
// have lower priority than any directory result. First match wins on name
// collision.
//
// # Enumerating All Subcommands
//
// [AllSubcommands] returns the merged set of static and discovered
// subcommands for a command. This is useful for custom [HelpRenderer]
// implementations, documentation generators, and shell completion scripts:
//
//	subs, err := cli.AllSubcommands(cmd)
//
// # Extensibility
//
// Every major subsystem is replaceable:
//
//   - [FlagParser] — replace the flag parsing engine per-command or globally
//   - [HelpRenderer] — replace help rendering per-command or globally
//   - [Helper] — override help text for a single command
//   - [HelpAppender] / [HelpPrepender] — add custom sections to the default help output
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
//
// # Documentation Generation
//
// The doc subpackage generates documentation from the command tree:
//
//	doc.GenMarkdown(root)              // markdown for a single command
//	doc.GenMarkdownTree(root, "docs/") // markdown files for all commands
//	doc.GenManPage(root, header)       // man page for a single command
//	doc.GenManTree(root, "man/", header) // man pages for all commands
//
// Hidden commands and flags are excluded from generated documentation.
//
// # Shell Completion
//
// The completion subpackage generates shell completion scripts:
//
//	completion.Bash(root, "myapp")       // bash completion script
//	completion.Zsh(root, "myapp")        // zsh completion script
//	completion.Fish(root, "myapp")       // fish completion script
//	completion.PowerShell(root, "myapp") // PowerShell completion script
//
// Generated scripts call the binary at runtime via the __complete protocol:
// when the binary receives "__complete" as the first argument, it runs
// [RuntimeComplete] instead of normal execution. This avoids side effects
// (no lifecycle hooks, flag parsing, or validation) and provides dynamic
// completions based on the current command tree.
//
// Commands implementing [Completer] can provide custom completion candidates.
// When a command implements Completer, its Complete method is called during
// tab-completion and the returned strings are offered as candidates. If
// Complete returns nil, the framework falls back to static completion of
// subcommands and flags.
package cli
