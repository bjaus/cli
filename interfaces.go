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

// Deprecater marks a command as deprecated. A non-empty return value
// is printed as a warning to stderr before the command runs.
type Deprecater interface {
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

// Exiter controls process exit behavior. When implemented on the root command,
// [ExecuteAndExit] delegates to Exit instead of calling [os.Exit] directly.
// The implementation is responsible for printing the error and exiting the process.
type Exiter interface {
	Exit(err error)
}

// --- Lifecycle interfaces (all optional) ---

// BeforeRunner runs setup logic before Run. Called parent-first through the
// command chain. The returned context flows forward to subsequent hooks and Run.
type BeforeRunner interface {
	Before(ctx context.Context) (context.Context, error)
}

// AfterRunner runs teardown logic after Run. Called child-first through the
// command chain. After hooks always run, even if Run returned an error.
type AfterRunner interface {
	After(ctx context.Context) error
}

// Validator validates command state after flag parsing and before Run.
type Validator interface {
	Validate() error
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

// --- Extensibility interfaces (all optional) ---

// FlagUnmarshaler allows custom types to be used as flag values.
type FlagUnmarshaler interface {
	UnmarshalFlag(value string) error
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
type HelpRenderer interface {
	RenderHelp(cmd Runner, chain []Runner, flags []FlagDef) string
}

// FlagDef describes a single flag for use by custom [HelpRenderer] implementations.
type FlagDef struct {
	Name      string
	Short     string
	Help      string
	Default   string
	Env       string
	Enum      string
	Required  bool
	TypeName  string
	IsBool    bool
	IsCounter bool
	Negatable bool
}

