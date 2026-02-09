// Package config provides [cli.ConfigResolver] implementations for common
// configuration sources.
//
// A [cli.ConfigResolver] is a function that maps flag names to string values.
// The cli framework handles type conversion, validation, required checks, and
// enum enforcement — resolvers only return strings. This package ships
// format-agnostic building blocks; any source that can produce a
// map[string]string works out of the box via [FromMap].
//
// Priority chain: explicit CLI flag > env var > config > default > zero value.
//
// # Provided resolvers
//
//   - [FromMap] — backed by a string map (useful for testing, in-memory config, or as the building block for custom formats)
//   - [FromJSON] — decodes a flat JSON object from an [io.Reader]
//   - [Chain] — tries multiple resolvers in order, returning the first match
//
// # Custom format adapters
//
// Because [FromMap] accepts any map[string]string, adding support for a new
// configuration format is a matter of decoding into a flat map. The examples
// below are complete, copy-paste-ready adapters.
//
// YAML (using gopkg.in/yaml.v3):
//
//	func FromYAML(r io.Reader) (cli.ConfigResolver, error) {
//	    var m map[string]string
//	    if err := yaml.NewDecoder(r).Decode(&m); err != nil {
//	        return nil, err
//	    }
//	    return config.FromMap(m), nil
//	}
//
// TOML (using github.com/BurntSushi/toml):
//
//	func FromTOML(r io.Reader) (cli.ConfigResolver, error) {
//	    var m map[string]string
//	    if _, err := toml.NewDecoder(r).Decode(&m); err != nil {
//	        return nil, err
//	    }
//	    return config.FromMap(m), nil
//	}
//
// HCL (using github.com/hashicorp/hcl/v2):
//
//	func FromHCL(r io.Reader) (cli.ConfigResolver, error) {
//	    data, err := io.ReadAll(r)
//	    if err != nil { return nil, err }
//	    var m map[string]string
//	    if err := hcl.Unmarshal(data, &m); err != nil {
//	        return nil, err
//	    }
//	    return config.FromMap(m), nil
//	}
//
// .env files (using github.com/joho/godotenv):
//
//	func FromDotenv(r io.Reader) (cli.ConfigResolver, error) {
//	    m, err := godotenv.Parse(r)
//	    if err != nil { return nil, err }
//	    return config.FromMap(m), nil
//	}
//
// # Direct resolver functions
//
// For sources that don't map cleanly to a flat file (remote stores,
// structured configs, or computed values), implement [cli.ConfigResolver]
// directly:
//
//	// Consul KV adapter
//	func FromConsul(client *consul.Client, prefix string) cli.ConfigResolver {
//	    return func(flagName string) (string, bool) {
//	        pair, _, err := client.KV().Get(prefix+"/"+flagName, nil)
//	        if err != nil || pair == nil { return "", false }
//	        return string(pair.Value), true
//	    }
//	}
//
// # Layered configuration
//
// Use [Chain] to try multiple sources in priority order. The first resolver
// that finds a value wins:
//
//	resolver := config.Chain(
//	    localOverrides,   // highest priority config source
//	    projectConfig,    // project-level config
//	    globalConfig,     // user-level defaults
//	)
package config

import (
	"encoding/json"
	"io"

	"github.com/bjaus/cli"
)

// FromMap returns a [cli.ConfigResolver] backed by a string map.
func FromMap(m map[string]string) cli.ConfigResolver {
	return func(flagName string) (string, bool) {
		v, ok := m[flagName]
		return v, ok
	}
}

// FromJSON decodes a flat JSON object from r and returns a [cli.ConfigResolver].
// The JSON must be a flat object with string values: {"port": "8080"}.
func FromJSON(r io.Reader) (cli.ConfigResolver, error) {
	var m map[string]string
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, err
	}
	return FromMap(m), nil
}

// Chain returns a [cli.ConfigResolver] that tries each resolver in order,
// returning the value from the first resolver that reports found.
func Chain(resolvers ...cli.ConfigResolver) cli.ConfigResolver {
	return func(flagName string) (string, bool) {
		for _, r := range resolvers {
			if v, ok := r(flagName); ok {
				return v, true
			}
		}
		return "", false
	}
}
