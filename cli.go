package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// Runner is the core interface every command must implement.
type Runner interface {
	Run(ctx context.Context, args []string) error
}

// RunFunc adapts a plain function into a [Runner].
type RunFunc func(ctx context.Context, args []string) error

// Run implements [Runner].
func (f RunFunc) Run(ctx context.Context, args []string) error {
	return f(ctx, args)
}

// Option configures the execution environment.
type Option func(*options)

type options struct {
	stdout              io.Writer
	stderr              io.Writer
	stdin               io.Reader
	flagParser          FlagParser
	helpRenderer        HelpRenderer
	configResolver      ConfigResolver
	flagNormalizer      func(string) string
	envVarPrefix        string
	suggest             bool
	shortOptionHandling bool
	prefixMatching      bool
	caseInsensitive     bool
	ignoreUnknown       bool
	sortedHelp          bool
	signalHandling      bool
	interactive         bool
	isTerminal          func() bool
}

func defaults() *options {
	return &options{
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		stdin:      os.Stdin,
		suggest:    true,
		isTerminal: defaultIsTerminal,
	}
}

// WithStdout sets the writer used for standard output (e.g., help text).
func WithStdout(w io.Writer) Option {
	return func(o *options) { o.stdout = w }
}

// WithStderr sets the writer used for standard error output.
func WithStderr(w io.Writer) Option {
	return func(o *options) { o.stderr = w }
}

// WithFlagParser sets a global flag parser, overriding the default struct-tag parser.
func WithFlagParser(p FlagParser) Option {
	return func(o *options) { o.flagParser = p }
}

// WithHelpRenderer sets a global help renderer, overriding the default renderer.
func WithHelpRenderer(r HelpRenderer) Option {
	return func(o *options) { o.helpRenderer = r }
}

// WithSuggest enables or disables "did you mean?" suggestions for unknown
// commands and flags. Enabled by default.
func WithSuggest(enabled bool) Option {
	return func(o *options) { o.suggest = enabled }
}

// WithShortOptionHandling enables POSIX-style short option combining.
// When enabled, -abc is expanded to -a -b -c (all but last must be bool/counter).
func WithShortOptionHandling(enabled bool) Option {
	return func(o *options) { o.shortOptionHandling = enabled }
}

// WithPrefixMatching enables unique prefix matching for subcommand names.
// When enabled, "ser" matches "serve" if no other subcommand starts with "ser".
func WithPrefixMatching(enabled bool) Option {
	return func(o *options) { o.prefixMatching = enabled }
}

// WithEnvVarPrefix sets a prefix prepended to all env var names declared via
// the env struct tag. For example, WithEnvVarPrefix("APP_") causes a flag
// tagged with `env:"PORT"` to look up the APP_PORT environment variable.
func WithEnvVarPrefix(prefix string) Option {
	return func(o *options) { o.envVarPrefix = prefix }
}

// WithCaseInsensitive enables case-insensitive subcommand matching.
// When enabled, "Serve" matches "serve".
func WithCaseInsensitive(enabled bool) Option {
	return func(o *options) { o.caseInsensitive = enabled }
}

// WithIgnoreUnknown causes unknown flags to be treated as positional args
// instead of returning an error. Useful for wrapper tools that forward
// flags to child processes.
func WithIgnoreUnknown(enabled bool) Option {
	return func(o *options) { o.ignoreUnknown = enabled }
}

// WithSortedHelp sorts subcommands and flags alphabetically in help output.
func WithSortedHelp(enabled bool) Option {
	return func(o *options) { o.sortedHelp = enabled }
}

// WithFlagNormalization sets a function that normalizes flag names before
// lookup. For example, to treat underscores as dashes:
//
//	cli.WithFlagNormalization(func(s string) string {
//	    return strings.ReplaceAll(s, "_", "-")
//	})
func WithFlagNormalization(fn func(string) string) Option {
	return func(o *options) { o.flagNormalizer = fn }
}

// WithConfigResolver sets a global config resolver for flag values.
// Config values have lower priority than env vars and explicit CLI flags,
// but higher priority than defaults: explicit flag > env > config > default > zero.
func WithConfigResolver(r ConfigResolver) Option {
	return func(o *options) { o.configResolver = r }
}

// WithSignalHandling enables automatic signal handling. When enabled,
// [Execute] wraps the context with [signal.NotifyContext] for SIGINT and
// SIGTERM, causing the context to be canceled when either signal is received.
func WithSignalHandling(enabled bool) Option {
	return func(o *options) { o.signalHandling = enabled }
}

// WithInteractive enables interactive prompting for missing required flags
// when stdin is a terminal. Commands can implement [Prompter] to customize
// the prompting behavior.
func WithInteractive(enabled bool) Option {
	return func(o *options) { o.interactive = enabled }
}

// WithStdin sets the reader used for standard input (e.g., interactive prompts).
func WithStdin(r io.Reader) Option {
	return func(o *options) { o.stdin = r }
}

// Execute runs the command tree rooted at root with the given args and options.
// It resolves subcommands, parses flags, runs lifecycle hooks, and executes
// the target command.
func Execute(ctx context.Context, root Runner, args []string, opts ...Option) error {
	o := defaults()
	for _, opt := range opts {
		opt(o)
	}
	if o.signalHandling {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
	}
	return execute(ctx, root, args, o)
}

// ExecuteAndExit calls [Execute] and exits the process. If root implements
// [Exiter], its Exit method is called with the error and controls the exit
// entirely. Otherwise, the error is printed to stderr. For flag and command
// errors (unknown flag, missing required flag, etc.) a usage hint is appended.
// If the error implements [ExitCoder], its exit code is used; non-nil errors
// default to exit code 1.
func ExecuteAndExit(ctx context.Context, root Runner, args []string, opts ...Option) {
	o := defaults()
	for _, opt := range opts {
		opt(o)
	}
	var stop context.CancelFunc
	if o.signalHandling {
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	}

	err := execute(ctx, root, args, o)
	if stop != nil {
		stop()
	}
	if err == nil {
		os.Exit(0)
	}

	if e, ok := root.(Exiter); ok {
		e.Exit(err)
		return
	}

	formatError(o.stderr, root, err)
	os.Exit(exitCode(err))
}

// formatError writes the error message and optional usage hint to w.
func formatError(w io.Writer, root Runner, err error) {
	fmt.Fprintf(w, "Error: %s\n", err) //nolint:errcheck

	if _, isExit := err.(ExitCoder); !isExit && isFlagOrCommandError(err) {
		name := resolveInfo(root).name
		fmt.Fprintf(w, "Run '%s --help' for usage.\n", name) //nolint:errcheck
	}
}

// exitCode returns the process exit code for an error.
func exitCode(err error) int {
	if ec, ok := err.(ExitCoder); ok {
		return ec.ExitCode()
	}
	return 1
}
