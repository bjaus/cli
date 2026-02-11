package cli

import (
	"errors"
	"fmt"
)

// Sentinel errors for flag parsing.
var (
	ErrUnknownFlag      = errors.New("unknown flag")
	ErrFlagRequiresVal  = errors.New("flag requires a value")
	ErrRequiredFlag     = errors.New("required flag not provided")
	ErrUnsupportedType  = errors.New("unsupported flag type")
	ErrInvalidFlagValue = errors.New("invalid flag value")
	ErrInvalidTag       = errors.New("invalid struct tag")
)

// ErrShowHelp can be returned from [Commander.Run] to make the framework render
// help for the current command. The error is not propagated to the caller.
var ErrShowHelp = errors.New("show help")

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

// isFlagOrCommandError returns true if err wraps one of the sentinel flag
// parsing errors, indicating the user made a usage mistake that could be
// helped by viewing --help.
func isFlagOrCommandError(err error) bool {
	return errors.Is(err, ErrUnknownFlag) ||
		errors.Is(err, ErrRequiredFlag) ||
		errors.Is(err, ErrFlagRequiresVal) ||
		errors.Is(err, ErrInvalidFlagValue) ||
		errors.Is(err, ErrUnsupportedType)
}
