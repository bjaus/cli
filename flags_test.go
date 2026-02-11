package cli_test

import (
	"context"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type flaggedCmd struct {
	Port    int    `flag:"port" short:"p" default:"8080" help:"Port to listen on" env:"PORT"`
	Host    string `flag:"host" default:"localhost" help:"Host to bind to"`
	Verbose bool   `flag:"verbose" short:"v" help:"Enable verbose logging"`
}

func (c *flaggedCmd) Run(_ context.Context) error { return nil }

type requiredFlagCmd struct {
	Name string `flag:"name" required:"true" help:"Your name"`
}

func (c *requiredFlagCmd) Run(_ context.Context) error { return nil }

func TestScanFlags(t *testing.T) {
	t.Parallel()

	cmd := &flaggedCmd{}
	defs := cli.ScanFlags(cmd)

	require.Len(t, defs, 3)

	byName := make(map[string]cli.FlagDef)
	for _, d := range defs {
		byName[d.Name] = d
	}

	port := byName["port"]
	assert.Equal(t, "p", port.Short)
	assert.Equal(t, "8080", port.Default)
	assert.Equal(t, "Port to listen on", port.Help)
	assert.Equal(t, "PORT", port.Env)
	assert.Equal(t, "int", port.TypeName)
	assert.False(t, port.IsBool)

	verbose := byName["verbose"]
	assert.Equal(t, "v", verbose.Short)
	assert.True(t, verbose.IsBool)
}

func TestScanFlags_NoFlags(t *testing.T) {
	t.Parallel()

	cmd := &bareCmd{}
	defs := cli.ScanFlags(cmd)
	assert.Empty(t, defs)
}

func TestScanFlags_RequiredField(t *testing.T) {
	t.Parallel()

	cmd := &requiredFlagCmd{}
	defs := cli.ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.True(t, defs[0].Required)
	assert.Equal(t, "name", defs[0].Name)
}

type uintFlagScanCmd struct {
	Port    uint   `flag:"port"`
	MaxConn uint64 `flag:"max-conn"`
}

func (c *uintFlagScanCmd) Run(_ context.Context) error { return nil }

func TestScanFlags_UintTypeName(t *testing.T) {
	t.Parallel()

	cmd := &uintFlagScanCmd{}
	defs := cli.ScanFlags(cmd)
	require.Len(t, defs, 2)

	byName := make(map[string]cli.FlagDef)
	for _, d := range defs {
		byName[d.Name] = d
	}

	assert.Equal(t, "uint", byName["port"].TypeName)
	assert.Equal(t, "uint", byName["max-conn"].TypeName)
}
