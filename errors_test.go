package cli_test

import (
	"errors"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		message  string
		code     int
		wantMsg  string
		wantCode int
	}{
		"non-zero exit": {
			message:  "something failed",
			code:     42,
			wantMsg:  "something failed",
			wantCode: 42,
		},
		"zero exit": {
			message:  "ok",
			code:     0,
			wantMsg:  "ok",
			wantCode: 0,
		},
		"exit code 1": {
			message:  "generic error",
			code:     1,
			wantMsg:  "generic error",
			wantCode: 1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := cli.Exit(tt.message, tt.code)
			require.Error(t, err)
			assert.Equal(t, tt.wantMsg, err.Error())

			var ec cli.ExitCoder
			require.ErrorAs(t, err, &ec)
			assert.Equal(t, tt.wantCode, ec.ExitCode())
		})
	}
}

func TestExitf(t *testing.T) {
	t.Parallel()

	err := cli.Exitf(2, "port %d in use", 8080)
	require.Error(t, err)
	assert.Equal(t, "port 8080 in use", err.Error())

	var ec cli.ExitCoder
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, 2, ec.ExitCode())
}

func TestExitCoder_ErrorsAs(t *testing.T) {
	t.Parallel()

	err := cli.Exit("inner", 3)
	wrapped := errors.New("outer: " + err.Error())

	// Direct ExitCoder extraction works.
	var ec cli.ExitCoder
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, 3, ec.ExitCode())

	// Plain wrapped error does not satisfy ExitCoder.
	assert.False(t, errors.As(wrapped, &ec))
}
