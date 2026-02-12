package cli

import (
	"errors"
	"fmt"
)

// sentinelError is an error that wraps a parent sentinel, enabling hierarchical
// error checking via [errors.Is]. For example, ErrRequiredArg wraps ErrArgument,
// so errors.Is(err, ErrRequiredArg) and errors.Is(err, ErrArgument) both return true.
type sentinelError struct {
	msg    string
	parent error
}

func (e *sentinelError) Error() string { return e.msg }
func (e *sentinelError) Unwrap() error { return e.parent }

func newSentinel(msg string, parent error) error {
	return &sentinelError{msg: msg, parent: parent}
}

// --- Flag errors ---
//
// Check specific errors or the parent:
//
//	errors.Is(err, cli.ErrUnknownFlag)  // specific
//	errors.Is(err, cli.ErrFlag)         // any flag error

var (
	// ErrFlag is the parent error for all flag-related errors.
	ErrFlag = errors.New("flag error")

	// ErrUnknownFlag indicates an unrecognized flag was provided.
	ErrUnknownFlag = newSentinel("unknown flag", ErrFlag)

	// ErrFlagRequiresVal indicates a flag that requires a value was not given one.
	ErrFlagRequiresVal = newSentinel("flag requires a value", ErrFlag)

	// ErrRequiredFlag indicates a required flag was not provided.
	ErrRequiredFlag = newSentinel("required flag not provided", ErrFlag)

	// ErrInvalidFlagValue indicates a flag value could not be parsed or is invalid.
	ErrInvalidFlagValue = newSentinel("invalid flag value", ErrFlag)
)

// --- Argument errors ---
//
// Check specific errors or the parent:
//
//	errors.Is(err, cli.ErrRequiredArg)  // specific
//	errors.Is(err, cli.ErrArgument)     // any argument error

var (
	// ErrArgument is the parent error for all positional argument errors.
	ErrArgument = errors.New("argument error")

	// ErrRequiredArg indicates a required positional argument was not provided.
	ErrRequiredArg = newSentinel("required argument not provided", ErrArgument)

	// ErrInvalidArgValue indicates a positional argument value could not be parsed.
	ErrInvalidArgValue = newSentinel("invalid argument value", ErrArgument)

	// ErrArgCount indicates the wrong number of positional arguments was provided.
	ErrArgCount = newSentinel("wrong number of arguments", ErrArgument)
)

// --- Command errors ---
//
// Check specific errors or the parent:
//
//	errors.Is(err, cli.ErrUnknownCommand)  // specific
//	errors.Is(err, cli.ErrCommand)         // any command error

var (
	// ErrCommand is the parent error for command resolution errors.
	ErrCommand = errors.New("command error")

	// ErrUnknownCommand indicates an unrecognized subcommand was provided.
	ErrUnknownCommand = newSentinel("unknown command", ErrCommand)
)

// --- Flag group errors ---
//
// Check specific errors or the parent:
//
//	errors.Is(err, cli.ErrMutuallyExclusive)  // specific
//	errors.Is(err, cli.ErrFlagGroup)          // any flag group error

var (
	// ErrFlagGroup is the parent error for flag group constraint violations.
	ErrFlagGroup = errors.New("flag group error")

	// ErrMutuallyExclusive indicates multiple flags in a mutually exclusive group were set.
	ErrMutuallyExclusive = newSentinel("mutually exclusive flags", ErrFlagGroup)

	// ErrRequiredTogether indicates not all flags in a required-together group were set.
	ErrRequiredTogether = newSentinel("flags must be set together", ErrFlagGroup)

	// ErrOneRequired indicates none of the flags in a one-required group were set.
	ErrOneRequired = newSentinel("exactly one flag required", ErrFlagGroup)
)

// --- Other errors ---

var (
	// ErrUnsupportedType indicates a struct field has a type that cannot be used as a flag.
	ErrUnsupportedType = errors.New("unsupported flag type")

	// ErrInvalidTag indicates a struct tag is malformed or has conflicting options.
	ErrInvalidTag = errors.New("invalid struct tag")
)

// --- Help/Usage signals ---
//
// These signals control help and usage display with appropriate exit codes.
// Return these from [Commander.Run] or lifecycle hooks to trigger display.

var (
	// ShowHelp triggers full help display and exits with code 0.
	// Use when the user explicitly requests help (e.g., "myapp help subcmd").
	ShowHelp = &helpSignal{code: 0, full: true}

	// ErrShowHelp triggers full help display and exits with code 1.
	// Use when showing help due to an error condition (e.g., missing required subcommand).
	ErrShowHelp = &helpSignal{code: 1, full: true}

	// ShowUsage triggers brief usage display and exits with code 0.
	// Use when the user explicitly requests usage information.
	ShowUsage = &helpSignal{code: 0, full: false}

	// ErrShowUsage triggers brief usage display and exits with code 1.
	// Use when showing usage due to an error condition.
	ErrShowUsage = &helpSignal{code: 1, full: false}
)

// helpSignal is a special error type that signals the framework to display
// help or usage information and exit with a specific code.
type helpSignal struct {
	code int  // exit code: 0 for success, 1 for error
	full bool // true for full help, false for brief usage
}

func (h *helpSignal) Error() string {
	if h.full {
		return "show help"
	}
	return "show usage"
}

func (h *helpSignal) ExitCode() int { return h.code }

// isHelpSignal returns true if err is a help/usage signal that should be
// intercepted by the framework for special handling.
func isHelpSignal(err error) bool {
	var hs *helpSignal
	return errors.As(err, &hs)
}

// getHelpSignal returns the helpSignal if err is one, or nil otherwise.
func getHelpSignal(err error) *helpSignal {
	var hs *helpSignal
	if errors.As(err, &hs) {
		return hs
	}
	return nil
}

// ExitCoder is implemented by errors that carry a process exit code.
type ExitCoder interface {
	ExitCode() int
}

type exitError struct {
	message string
	code    int
}

func (e *exitError) Error() string { return e.message }

func (e *exitError) ExitCode() int { return e.code }

// Exit returns an error that implements [ExitCoder] with the given message
// and exit code.
func Exit(message string, code int) error {
	return &exitError{message: message, code: code}
}

// Exitf returns an error that implements [ExitCoder] with a formatted message
// and exit code.
func Exitf(code int, format string, args ...any) error {
	return &exitError{message: fmt.Sprintf(format, args...), code: code}
}

// isUsageError returns true if err is a usage error that warrants showing
// a help hint. This includes flag errors, argument errors, command errors,
// and flag group errors.
func isUsageError(err error) bool {
	return errors.Is(err, ErrFlag) ||
		errors.Is(err, ErrArgument) ||
		errors.Is(err, ErrCommand) ||
		errors.Is(err, ErrFlagGroup)
}
