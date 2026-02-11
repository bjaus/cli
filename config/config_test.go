package config_test

import (
	"strings"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func key(name string) cli.ConfigKey {
	return cli.ConfigKey{Name: name, Parts: []string{name}}
}

func TestFromMap(t *testing.T) {
	t.Parallel()

	m := map[string]string{"port": "8080", "host": "localhost"}
	resolver := config.FromMap(m)

	val, ok := resolver(key("port"))
	assert.True(t, ok)
	assert.Equal(t, "8080", val)

	val, ok = resolver(key("host"))
	assert.True(t, ok)
	assert.Equal(t, "localhost", val)

	_, ok = resolver(key("missing"))
	assert.False(t, ok)
}

func TestFromJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid reader", func(t *testing.T) {
		t.Parallel()

		r := strings.NewReader(`{"port": "9090", "host": "0.0.0.0"}`)
		resolver, err := config.FromJSON(r)
		require.NoError(t, err)

		val, ok := resolver(key("port"))
		assert.True(t, ok)
		assert.Equal(t, "9090", val)

		_, ok = resolver(key("missing"))
		assert.False(t, ok)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		r := strings.NewReader(`{not json`)
		_, err := config.FromJSON(r)
		require.Error(t, err)
	})
}

func TestFromEnvFile(t *testing.T) {
	t.Parallel()

	input := `
# Database config
DB_HOST=localhost
DB_PORT=5432

# With quotes
SECRET="my-secret-value"
SINGLE='single-quoted'

# With export prefix
export APP_ENV=production

# Inline comment
LOG_LEVEL=debug # this is debug mode

# Whitespace around =
TIMEOUT = 30

# Empty value
EMPTY=
`
	r := strings.NewReader(input)
	resolver, err := config.FromEnvFile(r)
	require.NoError(t, err)

	tests := map[string]string{
		"DB_HOST":   "localhost",
		"DB_PORT":   "5432",
		"SECRET":    "my-secret-value",
		"SINGLE":    "single-quoted",
		"APP_ENV":   "production",
		"LOG_LEVEL": "debug",
		"TIMEOUT":   "30",
		"EMPTY":     "",
	}

	for k, want := range tests {
		val, ok := resolver(key(k))
		assert.True(t, ok, "key %q should be found", k)
		assert.Equal(t, want, val, "key %q", k)
	}

	_, ok := resolver(key("MISSING"))
	assert.False(t, ok)
}

func TestFromEnvFile_EmptyInput(t *testing.T) {
	t.Parallel()

	r := strings.NewReader("")
	resolver, err := config.FromEnvFile(r)
	require.NoError(t, err)

	_, ok := resolver(key("anything"))
	assert.False(t, ok)
}

func TestFromEnvFile_CommentsOnly(t *testing.T) {
	t.Parallel()

	r := strings.NewReader("# just a comment\n# another comment\n")
	resolver, err := config.FromEnvFile(r)
	require.NoError(t, err)

	_, ok := resolver(key("anything"))
	assert.False(t, ok)
}

func TestFromEnvFile_QuotedWithHash(t *testing.T) {
	t.Parallel()

	r := strings.NewReader(`PASSWORD="p@ss#word"`)
	resolver, err := config.FromEnvFile(r)
	require.NoError(t, err)

	val, ok := resolver(key("PASSWORD"))
	assert.True(t, ok)
	assert.Equal(t, "p@ss#word", val)
}

func TestChain(t *testing.T) {
	t.Parallel()

	first := config.FromMap(map[string]string{"port": "1111"})
	second := config.FromMap(map[string]string{"port": "2222", "host": "second"})

	chained := config.Chain(first, second)

	// First match wins.
	val, ok := chained(key("port"))
	assert.True(t, ok)
	assert.Equal(t, "1111", val)

	// Falls through to second.
	val, ok = chained(key("host"))
	assert.True(t, ok)
	assert.Equal(t, "second", val)

	// No match.
	_, ok = chained(key("missing"))
	assert.False(t, ok)
}
