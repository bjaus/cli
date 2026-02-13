package cli

import "context"

// Interfaces documents all optional interfaces a [Commander] may implement.
// This type exists for documentation and IDE discoverability; it is never
// instantiated. Each interface is checked via type assertion at runtime —
// implement only what you need.
//
// See also [Meta] for an embeddable struct that provides default implementations
// for common metadata interfaces.
//
// # Meta
//
//   - [Namer] — override command name (default: lowercase struct name)
//   - [Descriptor] — one-line description for help listings
//   - [LongDescriptor] — extended description in command's own help
//   - [Aliaser] — alternate names for the command
//   - [Categorizer] — group commands under headings in help
//   - [Hider] — hide command from help output
//   - [Deprecator] — mark as deprecated with warning message
//   - [Versioner] — version string for --version flag
//   - [Exampler] — usage examples in help output
//
// # Structure
//
//   - [Subcommander] — declare child subcommands
//   - [Fallbacker] — default subcommand when none matches
//   - [Discoverer] — runtime-discovered commands (plugins)
//
// # Lifecycle
//
//   - [Beforer] — setup logic before Run (parent-first)
//   - [Afterer] — teardown logic after Run (child-first, always runs)
//   - [Validator] — validate command state after flag parsing
//   - [Middlewarer] — wrap Run with middleware functions
//
// # Completion
//
//   - [Completer] — shell completion candidates for arguments
//   - [FlagCompleter] — dynamic completion for flag values
//   - [Suggester] — custom "did you mean?" algorithm
//
// # Configuration
//
//   - [ConfigProvider] — per-command config resolver
//
// # Arguments
//
//   - [ArgsValidator] — validate positional arguments
//   - [Passthrougher] — pass unknown flags as positional args
//
// # Flags
//
//   - [FlagGrouper] — flag relationship constraints (mutually exclusive, etc.)
//
// # Interactive
//
//   - [Prompter] — customize prompts for missing required flags
//
// # Help
//
//   - [Helper] — override help text entirely
//   - [HelpAppender] — add sections after main help content
//   - [HelpPrepender] — add sections before main help content
//   - [HelpRenderer] — replace the help rendering engine
//
// # Extensibility
//
//   - [FlagParser] — replace the flag parsing engine
//   - [Exiter] — control process exit behavior
type Interfaces any

// --- Discovery interfaces (all optional) ---

// Namer overrides the command name. The default is the lowercase struct type name.
type Namer interface {
	Name() string
}

// Descriptor provides a one-line description shown in help output.
type Descriptor interface {
	Description() string
}

// LongDescriptor provides extended description text shown in the command's
// own help output. When implemented, this replaces [Descriptor] in the
// help body while [Descriptor] remains used in subcommand listings.
type LongDescriptor interface {
	LongDescription() string
}

// Aliaser declares alternate names for the command.
type Aliaser interface {
	Aliases() []string
}

// Subcommander declares subcommands.
type Subcommander interface {
	Subcommands() []Commander
}

// Hider hides the command from help output.
type Hider interface {
	Hidden() bool
}

// Example represents a single usage example.
type Example struct {
	Description string
	Command     string
}

// Exampler provides usage examples shown in help output.
type Exampler interface {
	Examples() []Example
}

// Versioner provides a version string, displayed when --version is passed.
type Versioner interface {
	Version() string
}

// Deprecator marks a command as deprecated. A non-empty return value
// is printed as a warning to stderr before the command runs.
type Deprecator interface {
	Deprecated() string
}

// Categorizer groups the command under a heading in help output.
type Categorizer interface {
	Category() string
}

// Fallbacker provides a fallback subcommand to run when no subcommand name matches.
type Fallbacker interface {
	Fallback() Commander
}

// Discoverer provides runtime-discovered commands (plugins). Discovered
// commands are merged with [Subcommander.Subcommands] — built-in commands take
// priority on name collisions. A command may implement both [Subcommander] and
// Discoverer; it may also implement Discoverer alone (without Subcommander) to
// have only dynamically discovered subcommands.
//
// Use [Discover] to scan directories and PATH for plugin executables:
//
//	func (a *App) Discover() ([]Commander, error) {
//	    return cli.Discover(
//	        cli.WithDirs(cli.DefaultDirs("myapp")...),
//	        cli.WithPATH("myapp"),
//	    )
//	}
type Discoverer interface {
	Discover() ([]Commander, error)
}

// Exiter controls process exit behavior. When implemented on the root command,
// [ExecuteAndExit] delegates to Exit instead of calling [os.Exit] directly.
// The implementation is responsible for printing the error and exiting the process.
type Exiter interface {
	Exit(err error)
}

// --- Lifecycle interfaces (all optional) ---
//
// Execution order:
//   Init → [parse] → Default → Validate → Before → [middleware] → Run → After

// Initializer runs setup logic before any parsing. Called parent-first
// through the command chain. Use this for tracing setup, arg preprocessing,
// or early context initialization. The returned context flows forward.
type Initializer interface {
	Init(ctx context.Context) (context.Context, error)
}

// Defaulter computes default values after parsing but before validation.
// Called on each command in the chain after all flags, args, env vars,
// config, and tag defaults are applied. Use this for computed defaults
// that can't be expressed in struct tags (e.g., cross-field logic).
type Defaulter interface {
	Default() error
}

// Validator validates command state after flag parsing and defaulting.
// Called after all values are resolved (flags, args, env, config, defaults,
// inheritance, Defaulter) giving full access to the command's final state.
type Validator interface {
	Validate() error
}

// Beforer runs setup logic before Run. Called parent-first through the
// command chain. The returned context flows forward to subsequent hooks and Run.
type Beforer interface {
	Before(ctx context.Context) (context.Context, error)
}

// Afterer runs teardown logic after Run. Called child-first through the
// command chain. After hooks always run, even if Run returned an error.
type Afterer interface {
	After(ctx context.Context) error
}

// --- UX interfaces (all optional) ---

// Completer provides shell completion candidates. Return a [CompletionResult]
// with candidates, optional active help messages, and a directive controlling
// shell behavior. Return an empty result (or use [NoCompletions]) to fall
// through to static completion of subcommands and flags.
//
//	func (c *DeployCmd) Complete(ctx context.Context, args []string) cli.CompletionResult {
//	    return cli.Completions("dev", "staging", "prod").
//	        WithActiveHelp("Select deployment environment")
//	}
type Completer interface {
	Complete(ctx context.Context, args []string) CompletionResult
}

// FlagCompleter provides dynamic completion for flag values. When a flag
// requires a value and the command implements this interface, the framework
// calls CompleteFlag with the flag name and partial value. Return an empty
// result (or use [NoCompletions]) to fall through to enum-based completion.
//
//	func (c *DeployCmd) CompleteFlag(ctx context.Context, flag, value string) cli.CompletionResult {
//	    if flag == "region" {
//	        return cli.CompletionsWithDesc(
//	            cli.Completion{Value: "us-east-1", Description: "N. Virginia"},
//	            cli.Completion{Value: "us-west-2", Description: "Oregon"},
//	        )
//	    }
//	    return cli.NoCompletions()
//	}
type FlagCompleter interface {
	CompleteFlag(ctx context.Context, flag string, value string) CompletionResult
}

// Middlewarer provides middleware that wraps the command's Run function.
type Middlewarer interface {
	Middleware() []func(next RunFunc) RunFunc
}

// Suggester provides a custom suggestion algorithm for a command.
// Given an unknown name, it returns a suggestion or empty string.
type Suggester interface {
	Suggest(name string) string
}

// --- Config interfaces (all optional) ---

// ConfigKey identifies a flag for config resolution.
//
//   - Name is the full prefixed flag name (e.g. "db-host", "db-primary-host").
//   - Parts decomposes the name by prefix boundaries, not individual dashes.
//
// Parts is useful for resolvers backed by nested configuration formats
// (YAML, TOML). Each prefix tag value (minus trailing separator) becomes
// one part, with the base flag name as the final part.
//
// Examples:
//
//	// Simple flag: --port
//	ConfigKey{Name: "port", Parts: []string{"port"}}
//
//	// Prefixed struct: DB struct with prefix:"db-"
//	// Flag: --db-host
//	ConfigKey{Name: "db-host", Parts: []string{"db", "host"}}
//
//	// Nested prefixes: Primary struct with prefix:"db-primary-"
//	// Flag: --db-primary-host
//	ConfigKey{Name: "db-primary-host", Parts: []string{"db-primary", "host"}}
//
// Note: Parts splits only on prefix boundaries, NOT all dashes. A flag
// --db-primary-host with prefix:"db-primary-" creates Parts: ["db-primary", "host"],
// not ["db", "primary", "host"]. To get finer granularity, use nested structs
// with separate prefix tags.
type ConfigKey struct {
	Name  string
	Parts []string
}

// ConfigResolver resolves flag values from an external source such as a
// config file. Given a [ConfigKey], it returns the string value and whether
// the flag was found. The framework handles type conversion. Use
// [ConfigKey.Name] for flat lookups or [ConfigKey.Parts] for nested lookups.
type ConfigResolver func(key ConfigKey) (value string, found bool)

// ConfigProvider is implemented by commands that supply their own resolver.
// Checked before the global resolver set via [WithConfigResolver].
type ConfigProvider interface {
	ConfigResolver() ConfigResolver
}

// --- Arg interfaces (all optional) ---

// ArgDef describes a positional argument for use by custom [HelpRenderer] implementations.
type ArgDef struct {
	Name     string
	Help     string
	Default  string
	Mask     string
	Env      string
	Enum     string
	Required bool
	TypeName string
	IsSlice  bool
}

// ArgsValidator validates positional arguments after flag parsing.
// Checked on the leaf command before [Validator].
type ArgsValidator interface {
	ValidateArgs(args []string) error
}

// Passthrougher enables passthrough mode for a command. When enabled,
// recognized flags declared via struct tags are parsed normally, but
// unknown flags are passed through as positional arguments instead of
// producing errors. This is useful for wrapper commands like "exec"
// that have their own flags but also forward args to child processes.
type Passthrougher interface {
	Passthrough() bool
}

// --- Flag group types and interfaces (all optional) ---

// FlagGroupKind describes the type of constraint a flag group enforces.
type FlagGroupKind int

const (
	// GroupMutuallyExclusive means at most one flag in the group may be set.
	GroupMutuallyExclusive FlagGroupKind = iota
	// GroupRequiredTogether means if any flag in the group is set, all must be set.
	GroupRequiredTogether
	// GroupOneRequired means exactly one flag in the group must be set.
	GroupOneRequired
)

// FlagGroup defines a relationship constraint between flags.
type FlagGroup struct {
	Kind  FlagGroupKind
	Flags []string
}

// FlagGrouper declares flag group constraints on a command.
// Validation runs after flag parsing and inheritance.
type FlagGrouper interface {
	FlagGroups() []FlagGroup
}

// --- Interactive interfaces (all optional) ---

// Prompter customizes how a command prompts for missing required flags in
// interactive mode. When [WithInteractive] is enabled and stdin is a terminal,
// the framework calls Prompt for each missing required flag before validation.
// Returning an empty string causes the flag to remain unset (validation will
// catch it). Return an error to abort execution.
type Prompter interface {
	Prompt(flag FlagDef) (string, error)
}

// --- Extensibility interfaces (all optional) ---

// FlagUnmarshaler allows custom types to be used as flag values.
type FlagUnmarshaler interface {
	UnmarshalFlag(value string) error
}

// HelpSection is a custom section rendered in the default help output.
// The Header is rendered as a section title (like "Flags:" or "Commands:"),
// and Body is rendered as-is beneath it. Use this with [HelpAppender] or
// [HelpPrepender] to add context-specific information (required tokens,
// environment setup, etc.) without replacing the entire help renderer.
type HelpSection struct {
	Header string
	Body   string
}

// HelpAppender declares sections appended after the main help content
// (after Arguments, before Global Flags). Sections are rendered in order.
//
//	func (j *JiraCmd) AppendHelp() []cli.HelpSection {
//	    return []cli.HelpSection{{
//	        Header: "Required Tokens",
//	        Body:   "  JIRA_TOKEN    Jira API token (env: JIRA_TOKEN)",
//	    }}
//	}
type HelpAppender interface {
	AppendHelp() []HelpSection
}

// HelpPrepender declares sections prepended before the main help content
// (before Usage). Sections are rendered in order.
//
//	func (j *JiraCmd) PrependHelp() []cli.HelpSection {
//	    return []cli.HelpSection{{
//	        Header: "Notice",
//	        Body:   "  This command requires VPN access.",
//	    }}
//	}
type HelpPrepender interface {
	PrependHelp() []HelpSection
}

// Helper overrides help text for a single command.
type Helper interface {
	Help() string
}

// FlagParser replaces the flag parsing engine. Checked on the command first,
// then falls back to the global parser set via [WithFlagParser], then to the
// default struct-tag parser.
type FlagParser interface {
	ParseFlags(cmd Commander, args []string) (remaining []string, err error)
}

// HelpRenderer replaces the help rendering engine. Checked on the command
// first, then falls back to the global renderer set via [WithHelpRenderer],
// then to the default renderer.
//
// Parameters:
//   - cmd: the leaf command being rendered
//   - chain: the full command chain from root to leaf
//   - flags: the leaf command's flags
//   - args: the leaf command's positional arg definitions
//   - globalFlags: visible flags from parent commands
type HelpRenderer interface {
	RenderHelp(cmd Commander, chain []Commander, flags []FlagDef, args []ArgDef, globalFlags []FlagDef) string
}

// FlagDef describes a single flag for use by custom [HelpRenderer] implementations.
type FlagDef struct {
	Name        string
	Short       string
	Alt         []string // additional long flag names
	Help        string
	Default     string
	Mask        string // displayed instead of Default in help (e.g. "****" for secrets)
	Env         string
	Enum        string
	Sep         string // separator for splitting values into slice elements (e.g. ",")
	Category    string
	Deprecated  string
	Placeholder string // shown in help as value name (e.g. "PORT" in --port PORT)
	Required    bool
	Hidden      bool
	TypeName    string
	IsBool      bool
	IsCounter   bool
	Negate      bool
}
