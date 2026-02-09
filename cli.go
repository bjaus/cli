package cli

import (
	"context"
	"io"
	"os"
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
	flagParser          FlagParser
	helpRenderer        HelpRenderer
	suggest             bool
	shortOptionHandling bool
	prefixMatching      bool
}

func defaults() *options {
	return &options{
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		suggest: true,
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

// Execute runs the command tree rooted at root with the given args and options.
// It resolves subcommands, parses flags, runs lifecycle hooks, and executes
// the target command.
func Execute(ctx context.Context, root Runner, args []string, opts ...Option) error {
	o := defaults()
	for _, opt := range opts {
		opt(o)
	}
	return execute(ctx, root, args, o)
}

// ExecuteAndExit calls [Execute] and exits the process. If root implements
// [Exiter], its Exit method is called with the error and controls the exit
// entirely. Otherwise, if the error implements [ExitCoder], its exit code
// is used; non-nil errors default to exit code 1.
func ExecuteAndExit(ctx context.Context, root Runner, args []string, opts ...Option) {
	err := Execute(ctx, root, args, opts...)
	if err == nil {
		os.Exit(0)
	}
	if e, ok := root.(Exiter); ok {
		e.Exit(err)
		return
	}
	if ec, ok := err.(ExitCoder); ok {
		os.Exit(ec.ExitCode())
	}
	os.Exit(1)
}
