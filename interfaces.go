package cli

import "context"

// --- Discovery interfaces (all optional) ---

// Namer overrides the command name. The default is the lowercase struct type name.
type Namer interface {
	Name() string
}

// Describer provides a one-line description shown in help output.
type Describer interface {
	Description() string
}

// Aliaser declares alternate names for the command.
type Aliaser interface {
	Aliases() []string
}

// Parent declares subcommands.
type Parent interface {
	Subcommands() []Runner
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
	Fallback() Runner
}

// Discoverer provides runtime-discovered commands (plugins). Discovered
// commands are merged with [Parent.Subcommands] — built-in commands take
// priority on name collisions. A command may implement both [Parent] and
// Discoverer; it may also implement Discoverer alone (without Parent) to
// have only dynamically discovered subcommands.
//
// Use [Discover] to scan directories and PATH for plugin executables:
//
//	func (a *App) Discover() ([]Runner, error) {
//	    return cli.Discover("myapp",
//	        cli.WithDirs(cli.DefaultDirs("myapp")...),
//	        cli.WithPATH(),
//	    )
//	}
type Discoverer interface {
	Discover() ([]Runner, error)
}

// Exiter controls process exit behavior. When implemented on the root command,
// [ExecuteAndExit] delegates to Exit instead of calling [os.Exit] directly.
// The implementation is responsible for printing the error and exiting the process.
type Exiter interface {
	Exit(err error)
}

// --- Lifecycle interfaces (all optional) ---

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

// Validator validates command state after flag parsing and before Run.
// The provided map contains the names of flags that were explicitly set
// (via CLI args, env vars, config, or inheritance) — flags with only a
// default value are not included.
type Validator interface {
	Validate(provided map[string]bool) error
}

// --- UX interfaces (all optional) ---

// Completer provides shell completion candidates.
type Completer interface {
	Complete(ctx context.Context, args []string) []string
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

// ConfigResolver resolves flag values from an external source such as a
// config file. Given a flag name, it returns the string value and whether
// the flag was found. The framework handles type conversion.
type ConfigResolver func(flagName string) (value string, found bool)

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

// Passthrougher disables flag parsing for a command. When implemented,
// all remaining args are passed directly to Run as positional arguments.
// Useful for wrapper commands like "exec" that forward args to child processes.
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
	ParseFlags(cmd Runner, args []string) (remaining []string, err error)
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
	RenderHelp(cmd Runner, chain []Runner, flags []FlagDef, args []ArgDef, globalFlags []FlagDef) string
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

