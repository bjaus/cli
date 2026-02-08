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
)

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
