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
