package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveInfo / defaultName ---

type internalBareCmd struct{}

func (c *internalBareCmd) Run(_ context.Context) error { return nil }

type internalMetaEmptyCmd struct {
	Meta
}

func (c *internalMetaEmptyCmd) Run(_ context.Context) error { return nil }

type internalFullCmd struct{}

func (c *internalFullCmd) Run(_ context.Context) error { return nil }
func (c *internalFullCmd) Name() string                { return "serve" }
func (c *internalFullCmd) Description() string         { return "Start the server" }
func (c *internalFullCmd) Aliases() []string           { return []string{"s", "srv"} }
func (c *internalFullCmd) Hidden() bool                { return true }

func (c *internalFullCmd) Examples() []Example {
	return []Example{
		{Description: "Start on port 8080", Command: "app serve --port 8080"},
	}
}

func TestResolveInfo(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd         Commander
		wantName    string
		wantDesc    string
		wantAliases []string
		wantHidden  bool
		wantExCount int
	}{
		"bare command uses struct name": {
			cmd:      &internalBareCmd{},
			wantName: "internalbarecmd",
		},
		"full command uses all interfaces": {
			cmd:         &internalFullCmd{},
			wantName:    "serve",
			wantDesc:    "Start the server",
			wantAliases: []string{"s", "srv"},
			wantHidden:  true,
			wantExCount: 1,
		},
		"embedded Meta with empty name falls back to struct name": {
			cmd:      &internalMetaEmptyCmd{},
			wantName: "internalmetaemptycmd",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info := resolveInfo(tt.cmd)
			assert.Equal(t, tt.wantName, info.name)
			assert.Equal(t, tt.wantDesc, info.description)
			assert.Equal(t, tt.wantAliases, info.aliases)
			assert.Equal(t, tt.wantHidden, info.hidden)
			assert.Len(t, info.examples, tt.wantExCount)
		})
	}
}

func TestDefaultName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd      Commander
		wantName string
	}{
		"pointer to struct": {
			cmd:      &internalBareCmd{},
			wantName: "internalbarecmd",
		},
		"RunFunc": {
			cmd:      RunFunc(func(_ context.Context) error { return nil }),
			wantName: "runfunc",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantName, defaultName(tt.cmd))
		})
	}
}

// --- defaultParseFlags ---

type internalFlaggedCmd struct {
	Port    int           `flag:"port" short:"p" default:"8080" help:"Port to listen on" env:"PORT"`
	Host    string        `flag:"host" default:"localhost" help:"Host to bind to"`
	Verbose bool          `flag:"verbose" short:"v" help:"Enable verbose logging"`
	Timeout time.Duration `flag:"timeout" default:"30s" help:"Request timeout"`
	Rate    float64       `flag:"rate" default:"1.5" help:"Rate limit"`
}

func (c *internalFlaggedCmd) Run(_ context.Context) error { return nil }

type internalRequiredFlagCmd struct {
	Name string `flag:"name" required:"true" help:"Your name"`
}

func (c *internalRequiredFlagCmd) Run(_ context.Context) error { return nil }

type internalCustomValue struct {
	vals []string
}

func (c *internalCustomValue) UnmarshalFlag(value string) error {
	c.vals = append(c.vals, value)
	return nil
}

type internalCustomFlagCmd struct {
	Tags internalCustomValue `flag:"tag" short:"t" help:"Tags"`
}

func (c *internalCustomFlagCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		wantPort    int
		wantHost    string
		wantVerbose bool
		wantTimeout time.Duration
		wantRate    float64
		wantRemain  []string
		assertErr   require.ErrorAssertionFunc
	}{
		"long flags": {
			args:        []string{"--port", "9090", "--host", "0.0.0.0"},
			wantPort:    9090,
			wantHost:    "0.0.0.0",
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			assertErr:   require.NoError,
		},
		"short flags": {
			args:        []string{"-p", "3000", "-v"},
			wantPort:    3000,
			wantHost:    "localhost",
			wantVerbose: true,
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			assertErr:   require.NoError,
		},
		"equals syntax": {
			args:        []string{"--port=4000", "--host=example.com"},
			wantPort:    4000,
			wantHost:    "example.com",
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			assertErr:   require.NoError,
		},
		"defaults applied": {
			args:        nil,
			wantPort:    8080,
			wantHost:    "localhost",
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			assertErr:   require.NoError,
		},
		"positional args pass through": {
			args:        []string{"--port", "8080", "file1.txt", "file2.txt"},
			wantPort:    8080,
			wantHost:    "localhost",
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			wantRemain:  []string{"file1.txt", "file2.txt"},
			assertErr:   require.NoError,
		},
		"double dash stops flag parsing": {
			args:        []string{"--port", "8080", "--", "--not-a-flag"},
			wantPort:    8080,
			wantHost:    "localhost",
			wantTimeout: 30 * time.Second,
			wantRate:    1.5,
			wantRemain:  []string{"--not-a-flag"},
			assertErr:   require.NoError,
		},
		"duration flag": {
			args:        []string{"--timeout", "5m"},
			wantPort:    8080,
			wantHost:    "localhost",
			wantTimeout: 5 * time.Minute,
			wantRate:    1.5,
			assertErr:   require.NoError,
		},
		"float flag": {
			args:        []string{"--rate", "2.5"},
			wantPort:    8080,
			wantHost:    "localhost",
			wantTimeout: 30 * time.Second,
			wantRate:    2.5,
			assertErr:   require.NoError,
		},
		"unknown flag errors": {
			args:      []string{"--unknown"},
			assertErr: require.Error,
		},
		"missing value errors": {
			args:      []string{"--port"},
			assertErr: require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalFlaggedCmd{}
			remaining, _, err := defaultParseFlags(cmd, tt.args, defaults())
			tt.assertErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantPort, cmd.Port)
			assert.Equal(t, tt.wantHost, cmd.Host)
			assert.Equal(t, tt.wantVerbose, cmd.Verbose)
			assert.Equal(t, tt.wantTimeout, cmd.Timeout)
			assert.Equal(t, tt.wantRate, cmd.Rate)
			assert.Equal(t, tt.wantRemain, remaining)
		})
	}
}

func TestDefaultParseFlags_EnvVar(t *testing.T) {
	t.Setenv("PORT", "9999")
	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, 9999, cmd.Port)
}

func TestDefaultParseFlags_ExplicitOverridesEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--port", "3000"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, 3000, cmd.Port)
}

func TestDefaultParseFlags_RequiredFlag(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		assertErr require.ErrorAssertionFunc
	}{
		"missing required flag": {
			args:      nil,
			assertErr: require.Error,
		},
		"provided required flag": {
			args:      []string{"--name", "alice"},
			assertErr: require.NoError,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalRequiredFlagCmd{}
			_, provided, err := defaultParseFlags(cmd, tt.args, defaults())
			if err == nil {
				err = ValidateFlags(cmd, provided)
			}
			tt.assertErr(t, err)
		})
	}
}

func TestDefaultParseFlags_RequiredFlagErrorMessage(t *testing.T) {
	t.Parallel()

	cmd := &internalRequiredFlagCmd{}
	_, provided, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	err = ValidateFlags(cmd, provided)
	require.ErrorIs(t, err, ErrRequiredFlag)
	assert.Contains(t, err.Error(), "--name")
}

func TestDefaultParseFlags_CustomUnmarshaler(t *testing.T) {
	t.Parallel()

	cmd := &internalCustomFlagCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--tag", "foo"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, cmd.Tags.vals)
}

func TestDefaultParseFlags_InvalidValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
	}{
		"invalid int":      {args: []string{"--port", "abc"}},
		"invalid float":    {args: []string{"--rate", "xyz"}},
		"invalid duration": {args: []string{"--timeout", "nope"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalFlaggedCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			require.Error(t, err)
		})
	}
}

func TestDefaultParseFlags_UnknownFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--unknown"}, defaults())
	require.ErrorIs(t, err, ErrUnknownFlag)
}

func TestDefaultParseFlags_MissingValue(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--port"}, defaults())
	require.ErrorIs(t, err, ErrFlagRequiresVal)
}

// --- jaroWinkler / suggest ---

func TestJaroWinkler(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		s1, s2  string
		wantMin float64
		wantMax float64
	}{
		"identical": {
			s1: "hello", s2: "hello",
			wantMin: 1.0, wantMax: 1.0,
		},
		"completely different": {
			s1: "abc", s2: "xyz",
			wantMin: 0.0, wantMax: 0.3,
		},
		"similar": {
			s1: "serve", s2: "server",
			wantMin: 0.9, wantMax: 1.0,
		},
		"typo": {
			s1: "satus", s2: "status",
			wantMin: 0.8, wantMax: 1.0,
		},
		"empty first": {
			s1: "", s2: "hello",
			wantMin: 0.0, wantMax: 0.0,
		},
		"empty second": {
			s1: "hello", s2: "",
			wantMin: 0.0, wantMax: 0.0,
		},
		"both empty": {
			s1: "", s2: "",
			wantMin: 1.0, wantMax: 1.0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			score := jaroWinkler(tt.s1, tt.s2)
			assert.GreaterOrEqual(t, score, tt.wantMin, "score %f below minimum %f", score, tt.wantMin)
			assert.LessOrEqual(t, score, tt.wantMax, "score %f above maximum %f", score, tt.wantMax)
		})
	}
}

type internalParentWithSubs struct{}

func (p *internalParentWithSubs) Run(_ context.Context) error { return nil }

func (p *internalParentWithSubs) Subcommands() []Commander {
	return []Commander{
		&internalNamedCmd{n: "serve"},
		&internalNamedCmd{n: "status"},
		&internalNamedCmd{n: "deploy"},
	}
}

type internalNamedCmd struct {
	n string
}

func (c *internalNamedCmd) Run(_ context.Context) error { return nil }
func (c *internalNamedCmd) Name() string                { return c.n }

func TestSuggestSubcommand(t *testing.T) {
	t.Parallel()

	parent := &internalParentWithSubs{}

	tests := map[string]struct {
		input string
		want  string
	}{
		"close match serve":  {input: "serv", want: "serve"},
		"close match status": {input: "satus", want: "status"},
		"no match":           {input: "zzzzz", want: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, suggestSubcommand(parent, tt.input))
		})
	}
}

type internalFlagCmdForSuggest struct {
	Port    int  `flag:"port" short:"p"`
	Verbose bool `flag:"verbose" short:"v"`
}

func (c *internalFlagCmdForSuggest) Run(_ context.Context) error { return nil }

func TestSuggestFlagName(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagCmdForSuggest{}

	tests := map[string]struct {
		input string
		want  string
	}{
		"close to port":    {input: "--prot", want: "--port"},
		"close to verbose": {input: "--verbos", want: "--verbose"},
		"no match":         {input: "--zzzzz", want: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, suggestFlagName(cmd, tt.input))
		})
	}
}

func TestSuggestSubcommand_NonParent(t *testing.T) {
	t.Parallel()
	assert.Empty(t, suggestSubcommand(&internalBareCmd{}, "anything"))
}

// --- suggestSubcommand with Discoverer ---

type internalDiscovererParent struct{}

func (p *internalDiscovererParent) Run(_ context.Context) error { return nil }
func (p *internalDiscovererParent) Name() string                { return "myapp" }
func (p *internalDiscovererParent) Subcommands() []Commander {
	return []Commander{&internalNamedCmd{n: "serve"}}
}

func (p *internalDiscovererParent) Discover() ([]Commander, error) {
	return []Commander{&internalNamedCmd{n: "deploy-plugin"}}, nil
}

func TestSuggestSubcommand_DiscoveredCommand(t *testing.T) {
	t.Parallel()

	parent := &internalDiscovererParent{}
	// Misspell the plugin name — should still suggest it.
	assert.Equal(t, "deploy-plugin", suggestSubcommand(parent, "deploy-plugi"))
}

func TestSuggestSubcommand_StaticWithDiscovererPresent(t *testing.T) {
	t.Parallel()

	parent := &internalDiscovererParent{}
	// Misspell a static subcommand — should suggest it even with plugins present.
	assert.Equal(t, "serve", suggestSubcommand(parent, "serv"))
}

func TestSuggestFlagName_NoFlags(t *testing.T) {
	t.Parallel()
	assert.Empty(t, suggestFlagName(&internalBareCmd{}, "--anything"))
}

// --- applyMiddleware ---

func TestApplyMiddleware(t *testing.T) {
	t.Parallel()

	var order []string

	base := RunFunc(func(_ context.Context) error {
		order = append(order, "run")
		return nil
	})

	mw1 := func(next RunFunc) RunFunc {
		return func(ctx context.Context) error {
			order = append(order, "mw1-before")
			err := next(ctx)
			order = append(order, "mw1-after")
			return err
		}
	}

	mw2 := func(next RunFunc) RunFunc {
		return func(ctx context.Context) error {
			order = append(order, "mw2-before")
			err := next(ctx)
			order = append(order, "mw2-after")
			return err
		}
	}

	wrapped := applyMiddleware(base, []func(next RunFunc) RunFunc{mw1, mw2})
	err := wrapped(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"mw1-before",
		"mw2-before",
		"run",
		"mw2-after",
		"mw1-after",
	}, order)
}

func TestApplyMiddleware_Empty(t *testing.T) {
	t.Parallel()

	called := false
	base := RunFunc(func(_ context.Context) error {
		called = true
		return nil
	})

	wrapped := applyMiddleware(base, nil)
	err := wrapped(context.Background())
	require.NoError(t, err)
	assert.True(t, called)
}

func TestApplyMiddleware_ErrorPropagation(t *testing.T) {
	t.Parallel()

	base := RunFunc(func(_ context.Context) error {
		return Exit("fail", 1)
	})

	var afterCalled bool
	mw := func(next RunFunc) RunFunc {
		return func(ctx context.Context) error {
			err := next(ctx)
			afterCalled = true
			return err
		}
	}

	wrapped := applyMiddleware(base, []func(next RunFunc) RunFunc{mw})
	err := wrapped(context.Background())
	require.Error(t, err)
	assert.True(t, afterCalled)
}

// --- defaultRenderHelp ---

type internalServeCmd struct {
	Port int    `flag:"port" short:"p" default:"8080" help:"Port"`
	Host string `flag:"host" default:"localhost" help:"Host"`
}

func (s *internalServeCmd) Run(_ context.Context) error { return nil }
func (s *internalServeCmd) Name() string                { return "serve" }
func (s *internalServeCmd) Description() string         { return "Start the server" }

type internalRootCmd struct {
	serve *internalServeCmd
}

func (r *internalRootCmd) Run(_ context.Context) error { return nil }
func (r *internalRootCmd) Name() string                { return "app" }
func (r *internalRootCmd) Description() string         { return "Test application" }
func (r *internalRootCmd) Subcommands() []Commander    { return []Commander{r.serve} }

func TestDefaultRenderHelp_Basic(t *testing.T) {
	t.Parallel()

	cmd := &internalServeCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	assert.Contains(t, text, "Start the server")
	assert.Contains(t, text, "Usage:")
	assert.Contains(t, text, "Flags:")
	assert.Contains(t, text, "--port")
	assert.Contains(t, text, "-p")
	assert.Contains(t, text, "(default: 8080)")
}

func TestDefaultRenderHelp_WithSubcommands(t *testing.T) {
	t.Parallel()

	serve := &internalServeCmd{}
	root := &internalRootCmd{serve: serve}
	chain := []Commander{root}
	flags := ScanFlags(root)

	text := defaultRenderHelp(root, chain, flags, nil, false)

	assert.Contains(t, text, "Commands:")
	assert.Contains(t, text, "serve")
	assert.Contains(t, text, "[command]")
}

type internalHiddenSubCmd struct{}

func (c *internalHiddenSubCmd) Run(_ context.Context) error { return nil }
func (c *internalHiddenSubCmd) Name() string                { return "secret" }
func (c *internalHiddenSubCmd) Hidden() bool                { return true }

type internalParentWithHidden struct {
	child Commander
}

func (p *internalParentWithHidden) Run(_ context.Context) error { return nil }
func (p *internalParentWithHidden) Name() string                { return "app" }
func (p *internalParentWithHidden) Subcommands() []Commander    { return []Commander{p.child} }

func TestDefaultRenderHelp_HiddenSubcommands(t *testing.T) {
	t.Parallel()

	hidden := &internalHiddenSubCmd{}
	parent := &internalParentWithHidden{child: hidden}
	chain := []Commander{parent}

	text := defaultRenderHelp(parent, chain, nil, nil, false)
	assert.NotContains(t, text, "secret")
}

func TestDefaultRenderHelp_WithExamples(t *testing.T) {
	t.Parallel()

	cmd := &internalFullCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "Examples:")
	assert.Contains(t, text, "$ app serve --port 8080")
}

func TestDefaultRenderHelp_RequiredFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalRequiredFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "(required)")
}

func TestCommandChainNames(t *testing.T) {
	t.Parallel()

	serve := &internalServeCmd{}
	root := &internalRootCmd{serve: serve}
	chain := []Commander{root, serve}

	names := commandChainNames(chain)
	assert.Equal(t, "app serve", names)
}

// --- suggestFromError ---

func TestSuggestFromError(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagCmdForSuggest{}

	tests := map[string]struct {
		err  error
		want string
	}{
		"matching flag suggestion": {
			err:  fmt.Errorf("unknown flag: --prot"),
			want: `Did you mean "--port"?`,
		},
		"no match": {
			err:  fmt.Errorf("unknown flag: --zzzzz"),
			want: "",
		},
		"non-flag error": {
			err:  fmt.Errorf("something else"),
			want: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, suggestFromError(cmd, tt.err))
		})
	}
}

// --- suggestFlag ---

type internalSuggesterCmd struct {
	internalBareCmd
}

func (c *internalSuggesterCmd) Suggest(name string) string {
	return "custom: " + name
}

func TestSuggestFlag(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd  Commander
		err  error
		want string
	}{
		"with Suggester interface": {
			cmd:  &internalSuggesterCmd{},
			err:  fmt.Errorf("unknown flag: --foo"),
			want: "custom: unknown flag: --foo",
		},
		"without Suggester falls back to suggestFromError": {
			cmd:  &internalFlagCmdForSuggest{},
			err:  fmt.Errorf("unknown flag: --prot"),
			want: `Did you mean "--port"?`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, suggestFlag(tt.cmd, tt.err))
		})
	}
}

// --- scanLevel ---

func TestScanLevel(t *testing.T) {
	t.Parallel()

	subs := []Commander{&internalNamedCmd{n: "serve"}, &internalNamedCmd{n: "status"}}
	fi := flagIndex{known: map[string]bool{
		"--port":    false,
		"-p":        false,
		"--verbose": true,
	}}

	tests := map[string]struct {
		args         []string
		wantFlags    []string
		wantNext     []string
		wantFoundSub string
	}{
		"finds subcommand": {
			args:         []string{"--verbose", "serve", "--port", "8080"},
			wantFlags:    []string{"--verbose"},
			wantFoundSub: "serve",
		},
		"double dash stops scanning": {
			args:      []string{"--", "serve"},
			wantFlags: nil,
			wantNext:  []string{"--", "serve"},
		},
		"unknown args become next": {
			args:      []string{"unknown", "--verbose"},
			wantFlags: []string{"--verbose"},
			wantNext:  []string{"unknown"},
		},
		"equals syntax consumed": {
			args:      []string{"--port=8080"},
			wantFlags: []string{"--port=8080"},
			wantNext:  nil,
		},
		"value flag consumes next arg": {
			args:      []string{"--port", "9090"},
			wantFlags: []string{"--port", "9090"},
			wantNext:  nil,
		},
		"unknown flag becomes next": {
			args:     []string{"--unknown"},
			wantNext: []string{"--unknown"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			flags, next, found := scanLevel(tt.args, fi, subs, false, false)
			assert.Equal(t, tt.wantFlags, flags)
			assert.Equal(t, tt.wantNext, next)
			if tt.wantFoundSub != "" {
				require.NotNil(t, found)
				assert.Equal(t, tt.wantFoundSub, resolveInfo(found.sub).name)
			} else {
				assert.Nil(t, found)
			}
		})
	}
}

// --- tryConsumeFlag ---

func TestTryConsumeFlag(t *testing.T) {
	t.Parallel()

	fi := flagIndex{known: map[string]bool{
		"--port":    false,
		"--verbose": true,
	}}

	tests := map[string]struct {
		args         []string
		idx          int
		wantConsumed int
		wantOK       bool
	}{
		"known equals flag": {
			args: []string{"--port=8080"}, idx: 0,
			wantConsumed: 1, wantOK: true,
		},
		"unknown equals flag": {
			args: []string{"--unknown=val"}, idx: 0,
			wantConsumed: 0, wantOK: false,
		},
		"unknown flag": {
			args: []string{"--nope"}, idx: 0,
			wantConsumed: 0, wantOK: false,
		},
		"bool flag": {
			args: []string{"--verbose"}, idx: 0,
			wantConsumed: 1, wantOK: true,
		},
		"value flag with next": {
			args: []string{"--port", "8080"}, idx: 0,
			wantConsumed: 2, wantOK: true,
		},
		"value flag at end of args": {
			args: []string{"--port"}, idx: 0,
			wantConsumed: 1, wantOK: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			consumed, ok := tryConsumeFlag(tt.args, tt.idx, fi)
			assert.Equal(t, tt.wantConsumed, consumed)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// --- separateLeafArgs ---

func TestSeparateLeafArgs(t *testing.T) {
	t.Parallel()

	fi := flagIndex{known: map[string]bool{
		"--port":    false,
		"--verbose": true,
	}}

	tests := map[string]struct {
		args           []string
		wantFlags      []string
		wantPositional []string
	}{
		"flags and positional": {
			args:           []string{"--verbose", "file.txt", "--port", "8080"},
			wantFlags:      []string{"--verbose", "--port", "8080"},
			wantPositional: []string{"file.txt"},
		},
		"double dash separator": {
			args:           []string{"--verbose", "--", "--port", "file.txt"},
			wantFlags:      []string{"--verbose"},
			wantPositional: []string{"--port", "file.txt"},
		},
		"unknown flags become positional": {
			args:           []string{"--unknown", "file.txt"},
			wantPositional: []string{"--unknown", "file.txt"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			flags, positional := separateLeafArgs(tt.args, fi)
			assert.Equal(t, tt.wantFlags, flags)
			assert.Equal(t, tt.wantPositional, positional)
		})
	}
}

// --- resolveCommand ---

type internalEmptyParent struct{}

func (p *internalEmptyParent) Run(_ context.Context) error { return nil }
func (p *internalEmptyParent) Name() string                { return "empty" }
func (p *internalEmptyParent) Subcommands() []Commander    { return nil }

func TestResolveCommand(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		root         Commander
		args         []string
		wantChainLen int
		wantPosit    []string
	}{
		"simple command": {
			root:         &internalBareCmd{},
			args:         []string{"foo", "bar"},
			wantChainLen: 1,
			wantPosit:    []string{"foo", "bar"},
		},
		"parent with empty subcommands": {
			root:         &internalEmptyParent{},
			args:         []string{"arg1"},
			wantChainLen: 1,
			wantPosit:    []string{"arg1"},
		},
		"parent with subcommand match": {
			root:         &internalRootCmd{serve: &internalServeCmd{}},
			args:         []string{"serve", "--port", "9090"},
			wantChainLen: 2,
		},
		"parent with no subcommand match": {
			root:         &internalRootCmd{serve: &internalServeCmd{}},
			args:         []string{"unknown"},
			wantChainLen: 1,
			wantPosit:    []string{"unknown"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolved, err := resolveCommand(tt.root, tt.args, defaults())
			require.NoError(t, err)
			assert.Len(t, resolved.chain, tt.wantChainLen)
			if tt.wantPosit != nil {
				assert.Equal(t, tt.wantPosit, resolved.positional)
			}
		})
	}
}

// --- parseFlags ---

type internalFlagParserCmd struct {
	parseCalled bool
}

func (c *internalFlagParserCmd) Run(_ context.Context) error { return nil }

func (c *internalFlagParserCmd) ParseFlags(_ Commander, args []string) ([]string, error) {
	c.parseCalled = true
	return args, nil
}

func TestParseFlags_CommandFlagParser(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagParserCmd{}
	_, _, err := parseFlags(cmd, []string{"--foo"}, defaults())
	require.NoError(t, err)
	assert.True(t, cmd.parseCalled)
}

type internalGlobalParser struct {
	called bool
}

func (p *internalGlobalParser) ParseFlags(_ Commander, args []string) ([]string, error) {
	p.called = true
	return args, nil
}

func TestParseFlags_GlobalFlagParser(t *testing.T) {
	t.Parallel()

	parser := &internalGlobalParser{}
	opts := defaults()
	opts.flagParser = parser

	cmd := &internalBareCmd{}
	_, _, err := parseFlags(cmd, []string{"arg"}, opts)
	require.NoError(t, err)
	assert.True(t, parser.called)
}

// --- runAfterHooks ---

type internalAfterErrorCmd struct {
	errMsg string
}

func (c *internalAfterErrorCmd) Run(_ context.Context) error { return nil }

func (c *internalAfterErrorCmd) After(_ context.Context) error {
	return fmt.Errorf("%s", c.errMsg)
}

func TestRunAfterHooks_Error(t *testing.T) {
	t.Parallel()

	hooks := []Commander{
		&internalAfterErrorCmd{errMsg: "first after error"},
		&internalAfterErrorCmd{errMsg: "second after error"},
	}

	err := runAfterHooks(context.Background(), hooks)
	require.Error(t, err)
	// Child-first (reverse order): second hook runs first, then first hook.
	// First error encountered wins.
	assert.Contains(t, err.Error(), "second after error")
}

func TestRunAfterHooks_NoAfterInterface(t *testing.T) {
	t.Parallel()

	hooks := []Commander{&internalBareCmd{}}
	err := runAfterHooks(context.Background(), hooks)
	require.NoError(t, err)
}

// --- flagTypeName ---

func TestFlagTypeName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd      Commander
		flagName string
		wantType string
	}{
		"string":   {cmd: &internalRequiredFlagCmd{}, flagName: "name", wantType: "string"},
		"int":      {cmd: &internalFlaggedCmd{}, flagName: "port", wantType: "int"},
		"bool":     {cmd: &internalFlaggedCmd{}, flagName: "verbose", wantType: "bool"},
		"duration": {cmd: &internalFlaggedCmd{}, flagName: "timeout", wantType: "duration"},
		"float":    {cmd: &internalFlaggedCmd{}, flagName: "rate", wantType: "float"},
		"custom":   {cmd: &internalCustomFlagCmd{}, flagName: "tag", wantType: "value"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			flags := ScanFlags(tt.cmd)
			for _, f := range flags {
				if f.Name == tt.flagName {
					assert.Equal(t, tt.wantType, f.TypeName)
					return
				}
			}
			t.Fatalf("flag %q not found", tt.flagName)
		})
	}
}

// --- setFieldValue edge cases ---

type internalBoolValueCmd struct {
	Debug bool `flag:"debug"`
}

func (c *internalBoolValueCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_BoolEquals(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolValueCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--debug=true"}, defaults())
	require.NoError(t, err)
	assert.True(t, cmd.Debug)
}

func TestDefaultParseFlags_BoolInvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolValueCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--debug=notbool"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

func TestDefaultParseFlags_NonStruct(t *testing.T) {
	t.Parallel()

	cmd := RunFunc(func(_ context.Context) error { return nil })
	remaining, _, err := defaultParseFlags(cmd, []string{"arg1", "arg2"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []string{"arg1", "arg2"}, remaining)
}

type internalBadDefaultCmd struct {
	Port int `flag:"port" default:"not-a-number"`
}

func (c *internalBadDefaultCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_InvalidDefault(t *testing.T) {
	t.Parallel()

	cmd := &internalBadDefaultCmd{}
	_, _, err := defaultParseFlags(cmd, nil, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

type internalEnvCmd struct {
	Port int `flag:"port" env:"BAD_PORT"`
}

func (c *internalEnvCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_InvalidEnv(t *testing.T) {
	t.Setenv("BAD_PORT", "not-a-number")

	cmd := &internalEnvCmd{}
	_, _, err := defaultParseFlags(cmd, nil, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

func TestDefaultParseFlags_EqualsUnknown(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--unknown=value"}, defaults())
	require.ErrorIs(t, err, ErrUnknownFlag)
}

func TestDefaultParseFlags_EqualsInvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--port=abc"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- setFieldValue unsupported type ---

type internalUnsupportedFieldCmd struct {
	Ch chan int `flag:"ch"`
}

func (c *internalUnsupportedFieldCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_UnsupportedType(t *testing.T) {
	t.Parallel()

	cmd := &internalUnsupportedFieldCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--ch", "foo"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- renderHelp ---

type internalCmdLevelRenderer struct {
	internalBareCmd
}

func (c *internalCmdLevelRenderer) RenderHelp(_ Commander, _ []Commander, _ []FlagDef, _ []ArgDef, _ []FlagDef) string {
	return "cmd-level help"
}

func TestRenderHelp_CmdLevelRenderer(t *testing.T) {
	t.Parallel()

	var buf testWriter
	cmd := &internalCmdLevelRenderer{}
	opts := defaults()
	opts.stdout = &buf

	err := renderHelp(cmd, []Commander{cmd}, opts)
	require.NoError(t, err)
	assert.Equal(t, "cmd-level help", buf.String())
}

type testWriter struct {
	data []byte
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *testWriter) String() string {
	return string(w.data)
}

// --- helpRequested ---

func TestHelpRequested_InChainArgs(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:     []Commander{&internalBareCmd{}},
		chainArgs: [][]string{{"--help"}},
	}
	assert.True(t, helpRequested(resolved))
}

func TestHelpRequested_ShortInPositional(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:      []Commander{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"-h"},
	}
	assert.True(t, helpRequested(resolved))
}

func TestHelpRequested_NotRequested(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:      []Commander{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"arg1"},
	}
	assert.False(t, helpRequested(resolved))
}

func TestHelpRequested_HelpAsFirstPositional(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:      []Commander{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"help"},
	}
	assert.True(t, helpRequested(resolved))
}

func TestHelpRequested_HelpNotFirstPositional(t *testing.T) {
	t.Parallel()

	// "help" in a non-first position should NOT trigger help
	resolved := &resolvedCommand{
		chain:      []Commander{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"something", "help"},
	}
	assert.False(t, helpRequested(resolved))
}

// --- execute integration tests for coverage ---

// Test Before error with parent-child: parent's After should still run.
type internalBeforeParent struct {
	afterCalled bool
	child       Commander
}

func (c *internalBeforeParent) Run(_ context.Context) error { return nil }
func (c *internalBeforeParent) Name() string                { return "parent" }
func (c *internalBeforeParent) Subcommands() []Commander    { return []Commander{c.child} }

func (c *internalBeforeParent) Before(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (c *internalBeforeParent) After(_ context.Context) error {
	c.afterCalled = true
	return nil
}

type internalBeforeFailChild struct{}

func (c *internalBeforeFailChild) Run(_ context.Context) error { return nil }
func (c *internalBeforeFailChild) Name() string                { return "child" }

func (c *internalBeforeFailChild) Before(_ context.Context) (context.Context, error) {
	return nil, fmt.Errorf("before failed")
}

func TestExecute_BeforeError(t *testing.T) {
	t.Parallel()

	child := &internalBeforeFailChild{}
	parent := &internalBeforeParent{child: child}

	err := execute(context.Background(), parent, []string{"child"}, defaults())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "before failed")
	// Parent's After should still run because parent's Before succeeded.
	assert.True(t, parent.afterCalled)
}

// --- Initializer tests ---

type initKeyType struct{}

var initKey = initKeyType{}

type initializerCmd struct {
	initCalled bool
	initCtxVal string
	Flag       string `flag:"flag"`
}

func (c *initializerCmd) Run(_ context.Context) error { return nil }
func (c *initializerCmd) Init(ctx context.Context) (context.Context, error) {
	c.initCalled = true
	return context.WithValue(ctx, initKey, "init-value"), nil
}

func (c *initializerCmd) Before(ctx context.Context) (context.Context, error) {
	// Verify context from Init is available
	if v, ok := ctx.Value(initKey).(string); ok {
		c.initCtxVal = v
	}
	return ctx, nil
}

func TestExecute_Initializer(t *testing.T) {
	t.Parallel()

	cmd := &initializerCmd{}
	err := Execute(context.Background(), cmd, []string{"--flag", "test"})
	require.NoError(t, err)

	assert.True(t, cmd.initCalled, "Init should be called")
	assert.Equal(t, "init-value", cmd.initCtxVal, "context from Init should flow to Before")
	assert.Equal(t, "test", cmd.Flag, "flags should be parsed after Init")
}

type initializerErrorCmd struct{}

func (c *initializerErrorCmd) Run(_ context.Context) error { return nil }
func (c *initializerErrorCmd) Init(_ context.Context) (context.Context, error) {
	return nil, fmt.Errorf("init failed")
}

func TestExecute_InitializerError(t *testing.T) {
	t.Parallel()

	cmd := &initializerErrorCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init failed")
}

// --- Defaulter tests ---

type defaulterCmd struct {
	Output string `flag:"output"`
	Format string `flag:"format" default:"json"`
}

func (c *defaulterCmd) Run(_ context.Context) error { return nil }
func (c *defaulterCmd) Default() error {
	// Computed default: if output not set and format is json, default to stdout.json
	if c.Output == "" && c.Format == "json" {
		c.Output = "stdout.json"
	}
	return nil
}

func TestExecute_Defaulter(t *testing.T) {
	t.Parallel()

	cmd := &defaulterCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)

	assert.Equal(t, "json", cmd.Format)
	assert.Equal(t, "stdout.json", cmd.Output, "Defaulter should compute Output")
}

func TestExecute_DefaulterExplicitOverride(t *testing.T) {
	t.Parallel()

	cmd := &defaulterCmd{}
	err := Execute(context.Background(), cmd, []string{"--output", "out.txt"})
	require.NoError(t, err)

	assert.Equal(t, "out.txt", cmd.Output, "explicit flag should not be overridden")
}

type defaulterValidatorCmd struct {
	Value     int `flag:"value"`
	validated bool
}

func (c *defaulterValidatorCmd) Run(_ context.Context) error { return nil }
func (c *defaulterValidatorCmd) Default() error {
	if c.Value == 0 {
		c.Value = 42
	}
	return nil
}

func (c *defaulterValidatorCmd) Validate() error {
	c.validated = true
	if c.Value < 10 {
		return fmt.Errorf("value must be >= 10")
	}
	return nil
}

func TestExecute_DefaulterBeforeValidator(t *testing.T) {
	t.Parallel()

	cmd := &defaulterValidatorCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)

	assert.Equal(t, 42, cmd.Value, "Defaulter should set Value")
	assert.True(t, cmd.validated, "Validator should run after Defaulter")
}

type defaulterErrorCmd struct{}

func (c *defaulterErrorCmd) Run(_ context.Context) error { return nil }
func (c *defaulterErrorCmd) Default() error {
	return fmt.Errorf("default failed")
}

func TestExecute_DefaulterError(t *testing.T) {
	t.Parallel()

	cmd := &defaulterErrorCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default failed")
}

// Test suggestion path via custom FlagParser that returns unknown flag error.
type internalErrorFlagParser struct{}

func (p *internalErrorFlagParser) ParseFlags(_ Commander, _ []string) ([]string, error) {
	return nil, fmt.Errorf("unknown flag: --prot")
}

func TestExecute_SuggestEnabled(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	opts := defaults()
	opts.suggest = true
	opts.flagParser = &internalErrorFlagParser{}

	err := execute(context.Background(), cmd, nil, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Did you mean")
}

func TestExecute_SuggestDisabled(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	opts := defaults()
	opts.suggest = false
	opts.flagParser = &internalErrorFlagParser{}

	err := execute(context.Background(), cmd, nil, opts)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "Did you mean")
}

// Test suggestion with no match returns raw error.
type internalNoMatchFlagParser struct{}

func (p *internalNoMatchFlagParser) ParseFlags(_ Commander, _ []string) ([]string, error) {
	return nil, fmt.Errorf("unknown flag: --zzzzz")
}

func TestExecute_SuggestNoMatch(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	opts := defaults()
	opts.suggest = true
	opts.flagParser = &internalNoMatchFlagParser{}

	err := execute(context.Background(), cmd, nil, opts)
	require.Error(t, err)
	assert.Equal(t, "unknown flag: --zzzzz", err.Error())
}

// --- defaultRenderHelp edge cases ---

type internalNoDescCmd struct{}

func (c *internalNoDescCmd) Run(_ context.Context) error { return nil }
func (c *internalNoDescCmd) Name() string                { return "nodesc" }

func TestDefaultRenderHelp_NoDescription(t *testing.T) {
	t.Parallel()

	cmd := &internalNoDescCmd{}
	chain := []Commander{cmd}
	text := defaultRenderHelp(cmd, chain, nil, nil, false)

	assert.Contains(t, text, "Usage:")
	assert.NotContains(t, text, "Flags:")
}

type internalEnvFlagCmd struct {
	Port int `flag:"port" env:"PORT" help:"Port"`
}

func (c *internalEnvFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalEnvFlagCmd) Name() string                { return "envtest" }

func TestDefaultRenderHelp_FlagWithEnv(t *testing.T) {
	t.Parallel()

	cmd := &internalEnvFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "(env: PORT)")
}

// --- suggestSubcommand with hidden and alias ---

type internalAliasedSubCmd struct{}

func (c *internalAliasedSubCmd) Run(_ context.Context) error { return nil }
func (c *internalAliasedSubCmd) Name() string                { return "deploy" }
func (c *internalAliasedSubCmd) Aliases() []string           { return []string{"dep"} }

type internalParentMixed struct{}

func (p *internalParentMixed) Run(_ context.Context) error { return nil }

func (p *internalParentMixed) Subcommands() []Commander {
	return []Commander{&internalAliasedSubCmd{}, &internalHiddenSubCmd{}}
}

func TestSuggestSubcommand_SkipsHidden(t *testing.T) {
	t.Parallel()

	parent := &internalParentMixed{}
	// "secre" is close to "secret" but secret is hidden, so no suggestion.
	assert.Empty(t, suggestSubcommand(parent, "secre"))
}

func TestSuggestSubcommand_MatchesAlias(t *testing.T) {
	t.Parallel()

	parent := &internalParentMixed{}
	assert.Equal(t, "dep", suggestSubcommand(parent, "de"))
}

// --- defaultRenderHelp with no-short flag ---

type internalNoShortFlagCmd struct {
	Port int `flag:"port" default:"8080" help:"Port"`
}

func (c *internalNoShortFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalNoShortFlagCmd) Name() string                { return "noshort" }

func TestDefaultRenderHelp_NoShortFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalNoShortFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "    --port")
}

// --- defaultRenderHelp with example without description ---

type internalExampleNoDescCmd struct{}

func (c *internalExampleNoDescCmd) Run(_ context.Context) error { return nil }
func (c *internalExampleNoDescCmd) Name() string                { return "extest" }

func (c *internalExampleNoDescCmd) Examples() []Example {
	return []Example{{Command: "extest --flag"}}
}

func TestDefaultRenderHelp_ExampleNoDescription(t *testing.T) {
	t.Parallel()

	cmd := &internalExampleNoDescCmd{}
	chain := []Commander{cmd}

	text := defaultRenderHelp(cmd, chain, nil, nil, false)
	assert.Contains(t, text, "$ extest --flag")
}

// --- flagTypeName unsupported type fallback ---

func TestFlagTypeName_Unsupported(t *testing.T) {
	t.Parallel()

	// chan is not a supported type and doesn't implement FlagUnmarshaler.
	cmd := &internalUnsupportedFieldCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "chan int", flags[0].TypeName)
}

// --- setFieldValue value receiver FlagUnmarshaler ---

type internalValueReceiverUnmarshaler string

func (v internalValueReceiverUnmarshaler) UnmarshalFlag(_ string) error {
	// Value receiver — tests the non-pointer path.
	return nil
}

type internalValueUnmarshalerCmd struct {
	Val internalValueReceiverUnmarshaler `flag:"val"`
}

func (c *internalValueUnmarshalerCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_ValueReceiverUnmarshaler(t *testing.T) {
	t.Parallel()

	cmd := &internalValueUnmarshalerCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--val", "test"}, defaults())
	require.NoError(t, err)
}

// --- suggestFlagName short flag ---

func TestSuggestFlagName_ShortFlagBestMatch(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagCmdForSuggest{}
	// "--v" stripped to "v" → jaroWinkler("v", "v") = 1.0, better than any long name match.
	result := suggestFlagName(cmd, "--v")
	assert.Equal(t, "-v", result)
}

// --- jaroWinkler edge case: single char strings ---

func TestJaroWinkler_SingleChar(t *testing.T) {
	t.Parallel()

	score := jaroWinkler("a", "b")
	assert.Equal(t, 0.0, score)

	score = jaroWinkler("a", "a")
	assert.Equal(t, 1.0, score)
}

// --- int64 flag type ---

type internalInt64Cmd struct {
	Big int64 `flag:"big"`
}

func (c *internalInt64Cmd) Run(_ context.Context) error { return nil }

func TestFlagTypeName_Int64(t *testing.T) {
	t.Parallel()

	cmd := &internalInt64Cmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "int", flags[0].TypeName)
}

func TestDefaultParseFlags_Int64(t *testing.T) {
	t.Parallel()

	cmd := &internalInt64Cmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--big", "9999999999"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, int64(9999999999), cmd.Big)
}

// --- Slice flag support ---

type internalSliceCmd struct {
	Tags    []string  `flag:"tag" short:"t" help:"Tags"`
	Ports   []int     `flag:"port" help:"Ports"`
	Weights []float64 `flag:"weight" help:"Weights"`
}

func (c *internalSliceCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Slice(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		wantTags    []string
		wantPorts   []int
		wantWeights []float64
	}{
		"string slice": {
			args:     []string{"--tag", "foo", "--tag", "bar"},
			wantTags: []string{"foo", "bar"},
		},
		"int slice": {
			args:      []string{"--port", "8080", "--port", "9090"},
			wantPorts: []int{8080, 9090},
		},
		"float slice": {
			args:        []string{"--weight", "1.5", "--weight", "2.5"},
			wantWeights: []float64{1.5, 2.5},
		},
		"mixed": {
			args:      []string{"--tag", "a", "-t", "b", "--port", "80"},
			wantTags:  []string{"a", "b"},
			wantPorts: []int{80},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalSliceCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			require.NoError(t, err)
			if tt.wantTags != nil {
				assert.Equal(t, tt.wantTags, cmd.Tags)
			}
			if tt.wantPorts != nil {
				assert.Equal(t, tt.wantPorts, cmd.Ports)
			}
			if tt.wantWeights != nil {
				assert.Equal(t, tt.wantWeights, cmd.Weights)
			}
		})
	}
}

func TestFlagTypeName_Slice(t *testing.T) {
	t.Parallel()

	cmd := &internalSliceCmd{}
	flags := ScanFlags(cmd)

	tagFlag := findFlagByName(flags, "tag")
	require.NotNil(t, tagFlag)
	assert.Equal(t, "strings", tagFlag.TypeName)

	portFlag := findFlagByName(flags, "port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "ints", portFlag.TypeName)
}

func findFlagByName(flags []FlagDef, name string) *FlagDef {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}

// --- Map flag support ---

type internalMapCmd struct {
	Headers map[string]string `flag:"header" short:"H" help:"HTTP headers"`
}

func (c *internalMapCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Map(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		wantMap   map[string]string
		assertErr require.ErrorAssertionFunc
	}{
		"single entry": {
			args:      []string{"--header", "Content-Type=application/json"},
			wantMap:   map[string]string{"Content-Type": "application/json"},
			assertErr: require.NoError,
		},
		"multiple entries": {
			args:      []string{"-H", "X-Foo=bar", "-H", "X-Baz=qux"},
			wantMap:   map[string]string{"X-Foo": "bar", "X-Baz": "qux"},
			assertErr: require.NoError,
		},
		"invalid format": {
			args:      []string{"--header", "noequalssign"},
			assertErr: require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalMapCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			tt.assertErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantMap, cmd.Headers)
		})
	}
}

func TestFlagTypeName_Map(t *testing.T) {
	t.Parallel()

	cmd := &internalMapCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "key=value", flags[0].TypeName)
}

// --- Negatable bool flags ---

type internalNegatableCmd struct {
	Verbose bool `flag:"verbose" short:"v" negate:"true" help:"Verbose output"`
}

func (c *internalNegatableCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Negatable(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		wantVerbose bool
	}{
		"enable":             {args: []string{"--verbose"}, wantVerbose: true},
		"negate":             {args: []string{"--no-verbose"}, wantVerbose: false},
		"short":              {args: []string{"-v"}, wantVerbose: true},
		"enable then negate": {args: []string{"--verbose", "--no-verbose"}, wantVerbose: false},
		"negate then enable": {args: []string{"--no-verbose", "--verbose"}, wantVerbose: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalNegatableCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			require.NoError(t, err)
			assert.Equal(t, tt.wantVerbose, cmd.Verbose)
		})
	}
}

func TestScanFlags_Negatable(t *testing.T) {
	t.Parallel()

	cmd := &internalNegatableCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.True(t, flags[0].Negate)
	assert.True(t, flags[0].IsBool)
}

func TestBuildFlagIndex_Negatable(t *testing.T) {
	t.Parallel()

	cmd := &internalNegatableCmd{}
	fi := buildFlagIndex(cmd)

	assert.True(t, fi.has("--verbose"))
	assert.True(t, fi.has("--no-verbose"))
	assert.True(t, fi.has("-v"))
	assert.True(t, fi.isBool("--no-verbose"))
}

// --- Counter flags ---

type internalCounterCmd struct {
	Verbosity int `flag:"verbose" short:"v" counter:"true" help:"Verbosity level"`
}

func (c *internalCounterCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Counter(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args          []string
		wantVerbosity int
	}{
		"single":   {args: []string{"--verbose"}, wantVerbosity: 1},
		"double":   {args: []string{"-v", "-v"}, wantVerbosity: 2},
		"triple":   {args: []string{"-v", "-v", "-v"}, wantVerbosity: 3},
		"explicit": {args: []string{"--verbose=5"}, wantVerbosity: 5},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalCounterCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			require.NoError(t, err)
			assert.Equal(t, tt.wantVerbosity, cmd.Verbosity)
		})
	}
}

func TestScanFlags_Counter(t *testing.T) {
	t.Parallel()

	cmd := &internalCounterCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.True(t, flags[0].IsCounter)
	assert.False(t, flags[0].IsBool)
}

func TestBuildFlagIndex_Counter(t *testing.T) {
	t.Parallel()

	cmd := &internalCounterCmd{}
	fi := buildFlagIndex(cmd)

	assert.True(t, fi.has("--verbose"))
	assert.True(t, fi.isBool("--verbose")) // counters are bool-like (no value consumption)
}

// --- Enum validation ---

type internalEnumCmd struct {
	Format string `flag:"format" enum:"json,yaml,text" default:"json" help:"Output format"`
}

func (c *internalEnumCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Enum(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		wantFmt   string
		assertErr require.ErrorAssertionFunc
	}{
		"valid default": {
			args:      nil,
			wantFmt:   "json",
			assertErr: require.NoError,
		},
		"valid explicit": {
			args:      []string{"--format", "yaml"},
			wantFmt:   "yaml",
			assertErr: require.NoError,
		},
		"invalid value": {
			args:      []string{"--format", "xml"},
			assertErr: require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalEnumCmd{}
			_, provided, err := defaultParseFlags(cmd, tt.args, defaults())
			if err == nil {
				err = ValidateFlags(cmd, provided)
			}
			tt.assertErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantFmt, cmd.Format)
		})
	}
}

type internalEnumNoDefaultCmd struct {
	Format string `flag:"format" enum:"json,yaml"`
}

func (c *internalEnumNoDefaultCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_EnumNoDefault(t *testing.T) {
	t.Parallel()

	// No default, no flag provided → zero value, no enum validation.
	cmd := &internalEnumNoDefaultCmd{}
	_, provided, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	require.NoError(t, ValidateFlags(cmd, provided))
	assert.Empty(t, cmd.Format)
}

func TestScanFlags_Enum(t *testing.T) {
	t.Parallel()

	cmd := &internalEnumCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "json,yaml,text", flags[0].Enum)
}

// --- enumContains ---

func TestEnumContains(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		enum string
		val  string
		want bool
	}{
		"found":     {enum: "a,b,c", val: "b", want: true},
		"not found": {enum: "a,b,c", val: "d", want: false},
		"trimmed":   {enum: "a, b, c", val: "b", want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, enumContains(tt.enum, tt.val))
		})
	}
}

// --- parseScalarValue ---

func TestParseScalarValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		typ       reflect.Type
		value     string
		want      any
		assertErr require.ErrorAssertionFunc
	}{
		"string":       {typ: reflect.TypeOf(""), value: "hello", want: "hello", assertErr: require.NoError},
		"int":          {typ: reflect.TypeOf(0), value: "42", want: 42, assertErr: require.NoError},
		"int64":        {typ: reflect.TypeOf(int64(0)), value: "99", want: int64(99), assertErr: require.NoError},
		"float64":      {typ: reflect.TypeOf(0.0), value: "3.14", want: 3.14, assertErr: require.NoError},
		"bool":         {typ: reflect.TypeOf(false), value: "true", want: true, assertErr: require.NoError},
		"duration":     {typ: reflect.TypeOf(time.Duration(0)), value: "5s", want: 5 * time.Second, assertErr: require.NoError},
		"bad int":      {typ: reflect.TypeOf(0), value: "abc", assertErr: require.Error},
		"bad float":    {typ: reflect.TypeOf(0.0), value: "xyz", assertErr: require.Error},
		"bad bool":     {typ: reflect.TypeOf(false), value: "nope", assertErr: require.Error},
		"bad duration": {typ: reflect.TypeOf(time.Duration(0)), value: "bad", assertErr: require.Error},
		"unsupported":  {typ: reflect.TypeOf(make(chan int)), assertErr: require.Error},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseScalarValue(tt.typ, tt.value)
			tt.assertErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.want, got.Interface())
		})
	}
}

// --- Prefix matching ---

func TestFindSubcommand_PrefixMatch(t *testing.T) {
	t.Parallel()

	subs := []Commander{
		&internalNamedCmd{n: "serve"},
		&internalNamedCmd{n: "status"},
		&internalNamedCmd{n: "deploy"},
	}

	tests := map[string]struct {
		name    string
		prefix  bool
		wantNil bool
		want    string
	}{
		"exact match no prefix":   {name: "serve", prefix: false, want: "serve"},
		"prefix match unique":     {name: "dep", prefix: true, want: "deploy"},
		"prefix match ambiguous":  {name: "s", prefix: true, wantNil: true},
		"prefix disabled":         {name: "ser", prefix: false, wantNil: true},
		"prefix match unique ser": {name: "ser", prefix: true, want: "serve"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := findSubcommand(subs, tt.name, tt.prefix, false)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.want, resolveInfo(result).name)
			}
		})
	}
}

// --- Prefix matching via alias ---

type internalAliasedForPrefix struct{}

func (c *internalAliasedForPrefix) Run(_ context.Context) error { return nil }
func (c *internalAliasedForPrefix) Name() string                { return "deploy" }
func (c *internalAliasedForPrefix) Aliases() []string           { return []string{"dep"} }

func TestFindSubcommand_PrefixMatchAliasAmbiguous(t *testing.T) {
	t.Parallel()

	subs := []Commander{&internalAliasedForPrefix{}}
	// "de" matches prefix of both "deploy" and alias "dep" — ambiguous in our impl.
	result := findSubcommand(subs, "de", true, false)
	assert.Nil(t, result)
}

// --- Short option combining ---

func TestExpandShortOptions(t *testing.T) {
	t.Parallel()

	fi := flagIndex{known: map[string]bool{
		"-v": true,  // bool
		"-d": true,  // bool
		"-p": false, // value
	}}

	tests := map[string]struct {
		args []string
		want []string
	}{
		"all bool": {
			args: []string{"-vd"},
			want: []string{"-v", "-d"},
		},
		"last takes value": {
			args: []string{"-vp", "8080"},
			want: []string{"-v", "-p", "8080"},
		},
		"not all known": {
			args: []string{"-vx"},
			want: []string{"-vx"},
		},
		"non-bool in middle": {
			args: []string{"-pv"},
			want: []string{"-pv"},
		},
		"double dash stops": {
			args: []string{"--", "-vd"},
			want: []string{"--", "-vd"},
		},
		"single short not expanded": {
			args: []string{"-v"},
			want: []string{"-v"},
		},
		"long flag not expanded": {
			args: []string{"--verbose"},
			want: []string{"--verbose"},
		},
		"with equals not expanded": {
			args: []string{"-vd=true"},
			want: []string{"-vd=true"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, expandShortOptions(tt.args, fi))
		})
	}
}

// --- Version ---

func TestVersionRequested(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		positional []string
		want       bool
	}{
		"long":        {positional: []string{"--version"}, want: true},
		"short":       {positional: []string{"-V"}, want: true},
		"not present": {positional: []string{"arg1"}, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolved := &resolvedCommand{
				chain:      []Commander{&internalBareCmd{}},
				chainArgs:  [][]string{nil},
				positional: tt.positional,
			}
			assert.Equal(t, tt.want, versionRequested(resolved))
		})
	}
}

type internalVersionedCmd struct {
	internalBareCmd
}

func (c *internalVersionedCmd) Version() string { return "v1.2.3" }

func TestFindVersioner(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		chain []Commander
		found bool
	}{
		"has versioner": {
			chain: []Commander{&internalVersionedCmd{}},
			found: true,
		},
		"no versioner": {
			chain: []Commander{&internalBareCmd{}},
			found: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := findVersioner(tt.chain)
			if tt.found {
				require.NotNil(t, v)
				assert.Equal(t, "v1.2.3", v.Version())
			} else {
				assert.Nil(t, v)
			}
		})
	}
}

// --- Fallback command ---

type internalDefaultParent struct {
	def   *internalNamedCmd
	child *internalNamedCmd
}

func (p *internalDefaultParent) Run(_ context.Context) error { return nil }
func (p *internalDefaultParent) Name() string                { return "app" }
func (p *internalDefaultParent) Subcommands() []Commander    { return []Commander{p.child} }
func (p *internalDefaultParent) Fallback() Commander         { return p.def }

func TestResolveCommand_Fallback(t *testing.T) {
	t.Parallel()

	def := &internalNamedCmd{n: "dashboard"}
	child := &internalNamedCmd{n: "serve"}
	parent := &internalDefaultParent{def: def, child: child}

	tests := map[string]struct {
		args         []string
		wantChainLen int
		wantLeaf     string
	}{
		"explicit subcommand": {
			args:         []string{"serve"},
			wantChainLen: 2,
			wantLeaf:     "serve",
		},
		"default when no match": {
			args:         []string{},
			wantChainLen: 2,
			wantLeaf:     "dashboard",
		},
		"default with args": {
			args:         []string{"some-file"},
			wantChainLen: 2,
			wantLeaf:     "dashboard",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolved, err := resolveCommand(parent, tt.args, defaults())
			require.NoError(t, err)
			assert.Len(t, resolved.chain, tt.wantChainLen)
			leaf := resolved.chain[len(resolved.chain)-1]
			assert.Equal(t, tt.wantLeaf, resolveInfo(leaf).name)
		})
	}
}

// --- Help rendering: negatable ---

func TestDefaultRenderHelp_Negatable(t *testing.T) {
	t.Parallel()

	cmd := &internalNegatableCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--[no-]verbose")
}

// --- Help rendering: enum ---

func TestDefaultRenderHelp_Enum(t *testing.T) {
	t.Parallel()

	cmd := &internalEnumCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "[json|yaml|text]")
}

// --- Help rendering: counter ---

func TestDefaultRenderHelp_Counter(t *testing.T) {
	t.Parallel()

	cmd := &internalCounterCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "(repeatable)")
	assert.NotContains(t, text, "--verbose int")
}

// --- Help rendering: categories ---

type internalCatCmd struct {
	n   string
	cat string
}

func (c *internalCatCmd) Run(_ context.Context) error { return nil }
func (c *internalCatCmd) Name() string                { return c.n }
func (c *internalCatCmd) Description() string         { return c.n + " command" }
func (c *internalCatCmd) Category() string            { return c.cat }

type internalCatParent struct {
	subs []Commander
}

func (p *internalCatParent) Run(_ context.Context) error { return nil }
func (p *internalCatParent) Name() string                { return "app" }
func (p *internalCatParent) Subcommands() []Commander    { return p.subs }

func TestDefaultRenderHelp_Categories(t *testing.T) {
	t.Parallel()

	parent := &internalCatParent{
		subs: []Commander{
			&internalNamedCmd{n: "help"},
			&internalCatCmd{n: "serve", cat: "Server"},
			&internalCatCmd{n: "stop", cat: "Server"},
			&internalCatCmd{n: "deploy", cat: "Deploy"},
		},
	}
	chain := []Commander{parent}
	text := defaultRenderHelp(parent, chain, nil, nil, false)

	// Uncategorized under "Commands:"
	assert.Contains(t, text, "Commands:\n")
	assert.Contains(t, text, "  help")

	// Categorized groups
	assert.Contains(t, text, "Server:\n")
	assert.Contains(t, text, "  serve")
	assert.Contains(t, text, "  stop")
	assert.Contains(t, text, "Deploy:\n")
	assert.Contains(t, text, "  deploy")
}

func TestDefaultRenderHelp_AllCategorized(t *testing.T) {
	t.Parallel()

	parent := &internalCatParent{
		subs: []Commander{
			&internalCatCmd{n: "serve", cat: "Server"},
			&internalCatCmd{n: "deploy", cat: "Deploy"},
		},
	}
	chain := []Commander{parent}
	text := defaultRenderHelp(parent, chain, nil, nil, false)

	// No "Commands:" section when all are categorized
	assert.NotContains(t, text, "Commands:\n")
	assert.Contains(t, text, "Server:\n")
	assert.Contains(t, text, "Deploy:\n")
}

// --- renderSubcommands with all hidden ---

func TestRenderSubcommands_AllHidden(t *testing.T) {
	t.Parallel()

	parent := &internalParentWithHidden{child: &internalHiddenSubCmd{}}
	chain := []Commander{parent}
	text := defaultRenderHelp(parent, chain, nil, nil, false)

	assert.NotContains(t, text, "Commands:")
	assert.NotContains(t, text, "secret")
}

// --- Slice with duration elements ---

type internalDurationSliceCmd struct {
	Timeouts []time.Duration `flag:"timeout" help:"Timeouts"`
}

func (c *internalDurationSliceCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_DurationSlice(t *testing.T) {
	t.Parallel()

	cmd := &internalDurationSliceCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--timeout", "5s", "--timeout", "10m"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{5 * time.Second, 10 * time.Minute}, cmd.Timeouts)
}

func TestFlagTypeName_DurationSlice(t *testing.T) {
	t.Parallel()

	cmd := &internalDurationSliceCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "durations", flags[0].TypeName)
}

// --- Slice with int64 elements ---

type internalInt64SliceCmd struct {
	IDs []int64 `flag:"id" help:"IDs"`
}

func (c *internalInt64SliceCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_Int64Slice(t *testing.T) {
	t.Parallel()

	cmd := &internalInt64SliceCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--id", "100", "--id", "200"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []int64{100, 200}, cmd.IDs)
}

// --- Slice with unsupported element type ---

type internalBadSliceCmd struct {
	Items []chan int `flag:"item"`
}

func (c *internalBadSliceCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_SliceUnsupportedElem(t *testing.T) {
	t.Parallel()

	cmd := &internalBadSliceCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--item", "foo"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Map with bad key type ---

type internalBadMapKeyCmd struct {
	Items map[chan int]string `flag:"item"`
}

func (c *internalBadMapKeyCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_MapUnsupportedKey(t *testing.T) {
	t.Parallel()

	cmd := &internalBadMapKeyCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--item", "k=v"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Map with bad value type ---

type internalBadMapValCmd struct {
	Items map[string]chan int `flag:"item"`
}

func (c *internalBadMapValCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_MapUnsupportedValue(t *testing.T) {
	t.Parallel()

	cmd := &internalBadMapValCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--item", "k=v"}, defaults())
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Enum with env var ---

type internalEnumEnvCmd struct {
	Format string `flag:"format" enum:"json,yaml" env:"TEST_FORMAT"`
}

func (c *internalEnumEnvCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_EnumEnv(t *testing.T) {
	t.Setenv("TEST_FORMAT", "xml")

	cmd := &internalEnumEnvCmd{}
	_, provided, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	err = ValidateFlags(cmd, provided)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "must be one of")
}

// --- Bool slice ---

type internalBoolSliceCmd struct {
	Flags []bool `flag:"flag" help:"Flags"`
}

func (c *internalBoolSliceCmd) Run(_ context.Context) error { return nil }

func TestDefaultParseFlags_BoolSlice(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolSliceCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--flag", "true", "--flag", "false"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, cmd.Flags)
}

// --- parseScalarValue bad int64 ---

func TestParseScalarValue_BadInt64(t *testing.T) {
	t.Parallel()

	_, err := parseScalarValue(reflect.TypeOf(int64(0)), "abc")
	require.Error(t, err)
}

// --- Short option handling in resolveCommand scan phase ---

type internalShortOptParent struct {
	Verbose bool `flag:"verbose" short:"v"`
	child   Commander
}

func (p *internalShortOptParent) Run(_ context.Context) error { return nil }
func (p *internalShortOptParent) Name() string                { return "app" }
func (p *internalShortOptParent) Subcommands() []Commander    { return []Commander{p.child} }

func TestResolveCommand_ShortOptionHandlingInScanPhase(t *testing.T) {
	t.Parallel()

	child := &internalNamedCmd{n: "serve"}
	parent := &internalShortOptParent{child: child}
	opts := defaults()
	opts.shortOptionHandling = true

	resolved, err := resolveCommand(parent, []string{"-v", "serve"}, opts)
	require.NoError(t, err)
	assert.Len(t, resolved.chain, 2)
	assert.Equal(t, "serve", resolveInfo(resolved.chain[1]).name)
}

// --- Prefix matching via alias ---

type internalPrefixAliased struct{}

func (c *internalPrefixAliased) Run(_ context.Context) error { return nil }
func (c *internalPrefixAliased) Name() string                { return "deploy" }
func (c *internalPrefixAliased) Aliases() []string           { return []string{"dp"} }

func TestFindSubcommand_PrefixMatchAlias(t *testing.T) {
	t.Parallel()

	subs := []Commander{&internalPrefixAliased{}, &internalNamedCmd{n: "status"}}

	// "d" matches prefix of "deploy" and alias "dp" — but both are the same command.
	// In our implementation: first match on "deploy" sets match, second on "dp" sees match != nil → ambiguous.
	result := findSubcommand(subs, "d", true, false)
	assert.Nil(t, result) // ambiguous between name and alias

	// "sta" uniquely matches "status"
	result = findSubcommand(subs, "sta", true, false)
	require.NotNil(t, result)
	assert.Equal(t, "status", resolveInfo(result).name)

	// "dp" exact match via alias
	result = findSubcommand(subs, "dp", true, false)
	require.NotNil(t, result)
	assert.Equal(t, "deploy", resolveInfo(result).name)
}

// Test unique prefix match via alias only (name doesn't prefix-match).
type internalOnlyAliasPrefix struct{}

func (c *internalOnlyAliasPrefix) Run(_ context.Context) error { return nil }
func (c *internalOnlyAliasPrefix) Name() string                { return "xdeploy" }
func (c *internalOnlyAliasPrefix) Aliases() []string           { return []string{"dp"} }

func TestFindSubcommand_PrefixMatchAliasOnly(t *testing.T) {
	t.Parallel()

	subs := []Commander{&internalOnlyAliasPrefix{}}
	// "d" does NOT prefix-match "xdeploy" but DOES prefix-match alias "dp".
	result := findSubcommand(subs, "d", true, false)
	require.NotNil(t, result)
	assert.Equal(t, "xdeploy", resolveInfo(result).name)
}

// --- Negatable without short flag in help ---

type internalNegatableNoShortCmd struct {
	Color bool `flag:"color" negate:"true" help:"Colorize output"`
}

func (c *internalNegatableNoShortCmd) Run(_ context.Context) error { return nil }

func TestDefaultRenderHelp_NegatableNoShort(t *testing.T) {
	t.Parallel()

	cmd := &internalNegatableNoShortCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "    --[no-]color")
}

// --- discover internals ---

func TestIsExecutable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific executable bit test")
	}

	dir := t.TempDir()

	execPath := filepath.Join(dir, "myplugin")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\necho hi"), 0o755)) //nolint:gosec // test needs executable

	noExecPath := filepath.Join(dir, "readme.txt")
	require.NoError(t, os.WriteFile(noExecPath, []byte("hello"), 0o600))

	assert.True(t, isExecutable(execPath))
	assert.False(t, isExecutable(noExecPath))
	assert.False(t, isExecutable(filepath.Join(dir, "nonexistent")))
}

func TestQueryPluginInfo(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific shell script test")
	}

	dir := t.TempDir()

	tests := map[string]struct {
		script string
		want   *PluginInfo
	}{
		"valid json": {
			script: `#!/bin/sh
echo '{"name":"deploy","description":"Deploy things","aliases":["d","dep"]}'`,
			want: &PluginInfo{
				Name:        "deploy",
				Description: "Deploy things",
				Aliases:     []string{"d", "dep"},
			},
		},
		"partial json": {
			script: `#!/bin/sh
echo '{"description":"No name set"}'`,
			want: &PluginInfo{
				Description: "No name set",
			},
		},
		"invalid json": {
			script: `#!/bin/sh
echo 'not json'`,
			want: nil,
		},
		"exits non-zero": {
			script: `#!/bin/sh
exit 1`,
			want: nil,
		},
	}

	for name, tt := range tests {
		path := filepath.Join(dir, "plugin-"+name)
		require.NoError(t, os.WriteFile(path, []byte(tt.script), 0o755)) //nolint:gosec // test needs executable

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			info := queryPluginInfo(path, "--cli-info", 2*time.Second)
			if tt.want == nil {
				assert.Nil(t, info)
			} else {
				require.NotNil(t, info)
				assert.Equal(t, tt.want, info)
			}
		})
	}
}

// --- allSubcommands ---

type internalDiscoverParent struct {
	subs       []Commander
	discovered []Commander
	discoverFn func() ([]Commander, error)
}

func (d *internalDiscoverParent) Run(_ context.Context) error { return nil }
func (d *internalDiscoverParent) Name() string                { return "root" }
func (d *internalDiscoverParent) Subcommands() []Commander    { return d.subs }

func (d *internalDiscoverParent) Discover() ([]Commander, error) {
	if d.discoverFn != nil {
		return d.discoverFn()
	}
	return d.discovered, nil
}

type internalSimpleCmd struct{ n string }

func (s *internalSimpleCmd) Run(_ context.Context) error { return nil }
func (s *internalSimpleCmd) Name() string                { return s.n }

func TestAllSubcommands_MergesParentAndDiscoverer(t *testing.T) {
	t.Parallel()

	parent := &internalDiscoverParent{
		subs:       []Commander{&internalSimpleCmd{n: "builtin"}},
		discovered: []Commander{&internalSimpleCmd{n: "plugin"}},
	}

	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 2)

	names := []string{resolveInfo(subs[0]).name, resolveInfo(subs[1]).name}
	assert.Contains(t, names, "builtin")
	assert.Contains(t, names, "plugin")
}

func TestAllSubcommands_BuiltinWinsCollision(t *testing.T) {
	t.Parallel()

	parent := &internalDiscoverParent{
		subs:       []Commander{&internalSimpleCmd{n: "deploy"}},
		discovered: []Commander{&internalSimpleCmd{n: "deploy"}},
	}

	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "deploy", resolveInfo(subs[0]).name)
}

func TestAllSubcommands_DiscoverError(t *testing.T) {
	t.Parallel()

	parent := &internalDiscoverParent{
		discoverFn: func() ([]Commander, error) {
			return nil, assert.AnError
		},
	}

	subs, err := allSubcommands(parent)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, subs)
}

type internalParentOnlyCmd struct{ subs []Commander }

func (p *internalParentOnlyCmd) Run(_ context.Context) error { return nil }
func (p *internalParentOnlyCmd) Name() string                { return "root" }
func (p *internalParentOnlyCmd) Subcommands() []Commander    { return p.subs }

func TestAllSubcommands_ParentOnly(t *testing.T) {
	t.Parallel()

	p := &internalParentOnlyCmd{
		subs: []Commander{&internalSimpleCmd{n: "serve"}},
	}

	subs, err := allSubcommands(p)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "serve", resolveInfo(subs[0]).name)
}

type internalDiscovererOnlyCmd struct{ discovered []Commander }

func (d *internalDiscovererOnlyCmd) Run(_ context.Context) error { return nil }
func (d *internalDiscovererOnlyCmd) Name() string                { return "root" }

func (d *internalDiscovererOnlyCmd) Discover() ([]Commander, error) {
	return d.discovered, nil
}

func TestAllSubcommands_DiscovererOnly(t *testing.T) {
	t.Parallel()

	cmd := &internalDiscovererOnlyCmd{
		discovered: []Commander{&internalSimpleCmd{n: "plugin-a"}, &internalSimpleCmd{n: "plugin-b"}},
	}

	subs, err := allSubcommands(cmd)
	require.NoError(t, err)
	require.Len(t, subs, 2)
}

func TestAllSubcommands_NeitherParentNorDiscoverer(t *testing.T) {
	t.Parallel()

	cmd := &internalSimpleCmd{n: "leaf"}
	subs, err := allSubcommands(cmd)
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestHelp_IncludesDiscoveredCommands(t *testing.T) {
	t.Parallel()

	parent := &internalDiscoverParent{
		subs: []Commander{&internalSimpleCmd{n: "serve"}},
		discovered: []Commander{
			&ExternalCommand{Cmd: "deploy", Desc: "Deploy things"},
			&ExternalCommand{Cmd: "migrate", Desc: "Run migrations"},
		},
	}

	flags := ScanFlags(parent)
	help := defaultRenderHelp(parent, []Commander{parent}, flags, nil, false)

	assert.Contains(t, help, "serve")
	assert.Contains(t, help, "deploy")
	assert.Contains(t, help, "migrate")
	assert.Contains(t, help, "Deploy things")
	assert.Contains(t, help, "Run migrations")
}

// --- inheritFlags ---

type internalInheritParent struct {
	Env string `flag:"env"`
}

func (p *internalInheritParent) Run(_ context.Context) error { return nil }

type internalInheritChild struct {
	Env string `flag:"env"`
}

func (c *internalInheritChild) Run(_ context.Context) error { return nil }

// Test that a non-struct ancestor (RunFunc) is safely skipped.
func TestInheritFlags_NonStructAncestor(t *testing.T) {
	t.Parallel()

	parent := RunFunc(func(_ context.Context) error { return nil })
	child := &internalInheritChild{}
	chain := []Commander{parent, child}
	provided := []map[string]bool{nil, nil}

	inheritFlags(chain, provided)
	// RunFunc is not a struct, so no inheritance occurs; child stays zero.
	assert.Empty(t, child.Env)
}

// Test provided[i] == nil branch: child has no provided map (e.g. custom parser returned nil).
func TestInheritFlags_NilProvidedMap(t *testing.T) {
	t.Parallel()

	parent := &internalInheritParent{Env: "prod"}
	child := &internalInheritChild{}
	chain := []Commander{parent, child}
	// Parent provided "env", child provided map is nil.
	provided := []map[string]bool{{"env": true}, nil}

	inheritFlags(chain, provided)
	assert.Equal(t, "prod", child.Env)
	assert.True(t, provided[1]["env"])
}

// --- applyDefaults ---

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	err := applyDefaults(v, fields)
	require.NoError(t, err)

	assert.Equal(t, 8080, cmd.Port)
	assert.Equal(t, "localhost", cmd.Host)
	assert.False(t, cmd.Verbose)

	// Defaults do NOT mark provided.
	for _, fi := range fields {
		assert.False(t, fi.provided, "default should not mark %s as provided", fi.def.Name)
	}
}

// --- applyConfig ---

func TestApplyConfig(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	resolver := ConfigResolver(func(key ConfigKey) (string, bool) {
		m := map[string]string{"port": "9090", "host": "0.0.0.0"}
		val, ok := m[key.Name]
		return val, ok
	})

	err := applyConfig(v, fields, resolver)
	require.NoError(t, err)
	assert.Equal(t, 9090, cmd.Port)
	assert.Equal(t, "0.0.0.0", cmd.Host)

	// Config values should mark provided.
	for _, fi := range fields {
		if fi.def.Name == "port" || fi.def.Name == "host" {
			assert.True(t, fi.provided, "%s should be marked provided", fi.def.Name)
		}
	}
}

func TestApplyConfig_Nil(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	// Nil resolver is a no-op.
	err := applyConfig(v, fields, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, cmd.Port)
	assert.Equal(t, "", cmd.Host)
}

func TestApplyConfig_InvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	resolver := ConfigResolver(func(key ConfigKey) (string, bool) {
		if key.Name == "port" {
			return "not-a-number", true
		}
		return "", false
	})

	err := applyConfig(v, fields, resolver)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "from config")
}

// --- applyEnv ---

func TestApplyEnv(t *testing.T) {
	t.Setenv("PORT", "7777")

	cmd := &internalFlaggedCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	err := applyEnv(v, fields, "")
	require.NoError(t, err)
	assert.Equal(t, 7777, cmd.Port)

	// Env should mark provided.
	for _, fi := range fields {
		if fi.def.Name == "port" {
			assert.True(t, fi.provided, "env should mark port as provided")
		}
	}
}

// --- resolveConfigResolver ---

type internalConfigProviderCmd struct {
	internalBareCmd
}

func (c *internalConfigProviderCmd) ConfigResolver() ConfigResolver {
	return func(key ConfigKey) (string, bool) {
		return "from-command", true
	}
}

func TestResolveConfigResolver_CommandLevel(t *testing.T) {
	t.Parallel()

	cmd := &internalConfigProviderCmd{}
	opts := defaults()
	opts.configResolver = func(key ConfigKey) (string, bool) {
		return "from-global", true
	}

	resolver := resolveConfigResolver(cmd, opts)
	require.NotNil(t, resolver)
	val, ok := resolver(ConfigKey{Name: "anything", Parts: []string{"anything"}})
	assert.True(t, ok)
	assert.Equal(t, "from-command", val)
}

func TestResolveConfigResolver_GlobalOption(t *testing.T) {
	t.Parallel()

	cmd := &internalBareCmd{}
	opts := defaults()
	opts.configResolver = func(key ConfigKey) (string, bool) {
		return "from-global", true
	}

	resolver := resolveConfigResolver(cmd, opts)
	require.NotNil(t, resolver)
	val, ok := resolver(ConfigKey{Name: "anything", Parts: []string{"anything"}})
	assert.True(t, ok)
	assert.Equal(t, "from-global", val)
}

func TestResolveConfigResolver_None(t *testing.T) {
	t.Parallel()

	cmd := &internalBareCmd{}
	resolver := resolveConfigResolver(cmd, defaults())
	assert.Nil(t, resolver)
}

// --- ConfigKey.Parts ---

func TestConfigKey_Parts_Unprefixed(t *testing.T) {
	t.Parallel()

	type cmd struct {
		Port int    `flag:"port"`
		Host string `flag:"host"`
	}
	fields := buildFieldMap(reflect.TypeOf(cmd{}))
	fi := fields["--port"]
	require.NotNil(t, fi)
	assert.Equal(t, []string{"port"}, fi.parts)

	fi = fields["--host"]
	require.NotNil(t, fi)
	assert.Equal(t, []string{"host"}, fi.parts)
}

func TestConfigKey_Parts_SinglePrefix(t *testing.T) {
	t.Parallel()

	type dbFlags struct {
		Host string `flag:"host"`
		Port int    `flag:"port"`
	}
	type cmd struct {
		DB dbFlags `prefix:"db-"`
	}
	fields := buildFieldMap(reflect.TypeOf(cmd{}))
	fi := fields["--db-host"]
	require.NotNil(t, fi)
	assert.Equal(t, []string{"db", "host"}, fi.parts)

	fi = fields["--db-port"]
	require.NotNil(t, fi)
	assert.Equal(t, []string{"db", "port"}, fi.parts)
}

func TestConfigKey_Parts_NestedPrefix(t *testing.T) {
	t.Parallel()

	type innerFlags struct {
		Host string `flag:"host"`
	}
	type outerFlags struct {
		Inner innerFlags `prefix:"b-"`
	}
	type cmd struct {
		Outer outerFlags `prefix:"a-"`
	}
	fields := buildFieldMap(reflect.TypeOf(cmd{}))
	fi := fields["--a-b-host"]
	require.NotNil(t, fi)
	assert.Equal(t, []string{"a", "b", "host"}, fi.parts)
}

func TestConfigKey_Parts_UsedInResolver(t *testing.T) {
	t.Parallel()

	type dbFlags struct {
		Host string `flag:"host" default:"localhost"`
	}
	type cmd struct {
		DB dbFlags `prefix:"db-"`
		internalBareCmd
	}

	// Resolver uses parts for nested lookup.
	nested := map[string]map[string]string{
		"db": {"host": "remotehost"},
	}
	resolver := func(key ConfigKey) (string, bool) {
		if len(key.Parts) == 2 {
			if section, ok := nested[key.Parts[0]]; ok {
				v, found := section[key.Parts[1]]
				return v, found
			}
		}
		return "", false
	}

	c := &cmd{}
	err := Execute(context.Background(), c, nil, WithConfigResolver(resolver))
	require.NoError(t, err)
	assert.Equal(t, "remotehost", c.DB.Host)
}

// --- Hidden flags ---

type internalHiddenFlagCmd struct {
	Port  int  `flag:"port" default:"8080" help:"Port"`
	Debug bool `flag:"debug" hidden:"true" help:"Debug mode"`
}

func (c *internalHiddenFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalHiddenFlagCmd) Name() string                { return "app" }

func TestScanFlags_Hidden(t *testing.T) {
	t.Parallel()

	cmd := &internalHiddenFlagCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 2)

	portFlag := findFlagByName(flags, "port")
	require.NotNil(t, portFlag)
	assert.False(t, portFlag.Hidden)

	debugFlag := findFlagByName(flags, "debug")
	require.NotNil(t, debugFlag)
	assert.True(t, debugFlag.Hidden)
}

func TestDefaultRenderHelp_HiddenFlagFiltered(t *testing.T) {
	t.Parallel()

	cmd := &internalHiddenFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--port")
	assert.NotContains(t, text, "--debug")
	assert.Contains(t, text, "[flags]")
}

type internalAllHiddenFlagCmd struct {
	Debug bool `flag:"debug" hidden:"true"`
}

func (c *internalAllHiddenFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalAllHiddenFlagCmd) Name() string                { return "app" }

func TestDefaultRenderHelp_AllFlagsHidden(t *testing.T) {
	t.Parallel()

	cmd := &internalAllHiddenFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.NotContains(t, text, "Flags:")
	assert.NotContains(t, text, "--debug")
	assert.NotContains(t, text, "[flags]")
	assert.Contains(t, text, "[args...]")
}

// --- Deprecated flags ---

type internalDeprecatedFlagCmd struct {
	Port    int `flag:"port" default:"8080" help:"Port"`
	OldPort int `flag:"old-port" deprecated:"use --port instead" help:"Legacy port"`
}

func (c *internalDeprecatedFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalDeprecatedFlagCmd) Name() string                { return "app" }

func TestScanFlags_Deprecated(t *testing.T) {
	t.Parallel()

	cmd := &internalDeprecatedFlagCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 2)

	oldPortFlag := findFlagByName(flags, "old-port")
	require.NotNil(t, oldPortFlag)
	assert.Equal(t, "use --port instead", oldPortFlag.Deprecated)

	portFlag := findFlagByName(flags, "port")
	require.NotNil(t, portFlag)
	assert.Empty(t, portFlag.Deprecated)
}

func TestDefaultRenderHelp_DeprecatedFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalDeprecatedFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--old-port")
	assert.Contains(t, text, "(DEPRECATED: use --port instead)")
}

// --- Flag categories ---

type internalFlagCategoryCmd struct {
	Host    string `flag:"host" default:"localhost" help:"Host" category:"Server"`
	Port    int    `flag:"port" default:"8080" help:"Port" category:"Server"`
	Verbose bool   `flag:"verbose" help:"Verbose output"`
	Format  string `flag:"format" default:"text" help:"Output format" category:"Output"`
}

func (c *internalFlagCategoryCmd) Run(_ context.Context) error { return nil }
func (c *internalFlagCategoryCmd) Name() string                { return "app" }

func TestScanFlags_Category(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagCategoryCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 4)

	hostFlag := findFlagByName(flags, "host")
	require.NotNil(t, hostFlag)
	assert.Equal(t, "Server", hostFlag.Category)

	verboseFlag := findFlagByName(flags, "verbose")
	require.NotNil(t, verboseFlag)
	assert.Empty(t, verboseFlag.Category)
}

func TestDefaultRenderHelp_FlagCategories(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagCategoryCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	// Uncategorized under "Flags:"
	assert.Contains(t, text, "Flags:\n")
	assert.Contains(t, text, "--verbose")

	// Categorized groups
	assert.Contains(t, text, "Server:\n")
	assert.Contains(t, text, "--host")
	assert.Contains(t, text, "--port")
	assert.Contains(t, text, "Output:\n")
	assert.Contains(t, text, "--format")
}

type internalAllCatFlagCmd struct {
	Host string `flag:"host" help:"Host" category:"Server"`
	Port int    `flag:"port" help:"Port" category:"Server"`
}

func (c *internalAllCatFlagCmd) Run(_ context.Context) error { return nil }
func (c *internalAllCatFlagCmd) Name() string                { return "app" }

func TestDefaultRenderHelp_AllFlagsCategorized(t *testing.T) {
	t.Parallel()

	cmd := &internalAllCatFlagCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.NotContains(t, text, "Flags:\n")
	assert.Contains(t, text, "Server:\n")
}

// --- hasVisibleFlags ---

func TestHasVisibleFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		flags []FlagDef
		want  bool
	}{
		"nil":         {flags: nil, want: false},
		"empty":       {flags: []FlagDef{}, want: false},
		"all hidden":  {flags: []FlagDef{{Hidden: true}}, want: false},
		"one visible": {flags: []FlagDef{{Hidden: true}, {Name: "port"}}, want: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hasVisibleFlags(tt.flags))
		})
	}
}

// --- camelToKebab ---

func TestCamelToKebab(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input string
		want  string
	}{
		"simple":            {input: "Port", want: "port"},
		"two words":         {input: "OutputFormat", want: "output-format"},
		"acronym prefix":    {input: "HTTPHost", want: "http-host"},
		"acronym suffix":    {input: "UserID", want: "user-id"},
		"single word":       {input: "ID", want: "id"},
		"three words":       {input: "MaxRetryCount", want: "max-retry-count"},
		"all caps":          {input: "RPS", want: "rps"},
		"mixed":             {input: "XMLParser", want: "xml-parser"},
		"already lowercase": {input: "port", want: "port"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, camelToKebab(tt.input))
		})
	}
}

// --- Auto flag name derivation ---

type internalAutoNameCmd struct {
	OutputFormat string `flag:"" help:"Output format"`
	Port         int    `flag:"port" help:"Port"`
	HTTPHost     string `flag:"" help:"HTTP host"`
}

func (c *internalAutoNameCmd) Run(_ context.Context) error { return nil }

func TestScanFlags_AutoName(t *testing.T) {
	t.Parallel()

	cmd := &internalAutoNameCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 3)

	assert.Equal(t, "output-format", flags[0].Name)
	assert.Equal(t, "port", flags[1].Name)
	assert.Equal(t, "http-host", flags[2].Name)
}

func TestDefaultParseFlags_AutoName(t *testing.T) {
	t.Parallel()

	cmd := &internalAutoNameCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--output-format", "json", "--port", "9090", "--http-host", "0.0.0.0"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.OutputFormat)
	assert.Equal(t, 9090, cmd.Port)
	assert.Equal(t, "0.0.0.0", cmd.HTTPHost)
}

// --- applyEnv with prefix ---

type internalEnvPrefixCmd struct {
	Port int `flag:"port" env:"PORT"`
}

func (c *internalEnvPrefixCmd) Run(_ context.Context) error { return nil }

func TestApplyEnv_WithPrefix(t *testing.T) {
	t.Setenv("APP_PORT", "4444")

	cmd := &internalEnvPrefixCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	err := applyEnv(v, fields, "APP_")
	require.NoError(t, err)
	assert.Equal(t, 4444, cmd.Port)
}

func TestApplyEnv_WithoutPrefix(t *testing.T) {
	t.Setenv("PORT", "5555")

	cmd := &internalEnvPrefixCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	err := applyEnv(v, fields, "")
	require.NoError(t, err)
	assert.Equal(t, 5555, cmd.Port)
}

func TestApplyEnv_PrefixNoMatch(t *testing.T) {
	t.Setenv("PORT", "5555") // Set without prefix

	cmd := &internalEnvPrefixCmd{}
	v := reflect.ValueOf(cmd).Elem()
	fields := buildFieldMap(v.Type())

	// With prefix, APP_PORT is looked up, not PORT
	err := applyEnv(v, fields, "APP_")
	require.NoError(t, err)
	assert.Equal(t, 0, cmd.Port) // not found
}

// --- ScanArgs ---

type internalArgCmd struct {
	Source string   `arg:"source" help:"Source file"`
	Dest   string   `arg:"dest" help:"Destination"`
	Extra  []string `arg:"extra" help:"Extra files"`
}

func (c *internalArgCmd) Run(_ context.Context) error { return nil }
func (c *internalArgCmd) Name() string                { return "copy" }

func TestScanArgs(t *testing.T) {
	t.Parallel()

	cmd := &internalArgCmd{}
	defs := ScanArgs(cmd)
	require.Len(t, defs, 3)

	assert.Equal(t, "source", defs[0].Name)
	assert.True(t, defs[0].Required)
	assert.False(t, defs[0].IsSlice)

	assert.Equal(t, "dest", defs[1].Name)
	assert.True(t, defs[1].Required)

	assert.Equal(t, "extra", defs[2].Name)
	assert.False(t, defs[2].Required) // slice defaults to optional
	assert.True(t, defs[2].IsSlice)
}

func TestScanArgs_AutoName(t *testing.T) {
	t.Parallel()

	type autoArgCmd struct {
		OutputFile string `arg:"" help:"Output file"`
	}

	cmd := &struct {
		autoArgCmd
		internalBareCmd
	}{}
	// We need a proper Commander for ScanArgs.
	// ScanArgs only reads type info, so we can use the raw struct.
	defs := ScanArgs(&struct {
		OutputFile string `arg:"" help:"Output file"`
		internalBareCmd
	}{})
	require.Len(t, defs, 1)
	assert.Equal(t, "output-file", defs[0].Name)
	_ = cmd // avoid unused
}

func TestScanArgs_NonStruct(t *testing.T) {
	t.Parallel()

	cmd := RunFunc(func(_ context.Context) error { return nil })
	defs := ScanArgs(cmd)
	assert.Nil(t, defs)
}

// --- populateArgs ---

type variadicNotLastCmd struct {
	Files  []string `arg:"files" help:"Files to process"`
	Output string   `arg:"output" help:"Output destination"`
}

func (c *variadicNotLastCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_VariadicNotLast(t *testing.T) {
	t.Parallel()

	// Variadic (slice) args must come last since they consume all remaining args.
	cmd := &variadicNotLastCmd{}
	_, err := populateArgs(cmd, []string{"a.txt", "b.txt", "out.txt"}, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrArgOrder)
	assert.Contains(t, err.Error(), "variadic argument must be last")
}

func TestPopulateArgs(t *testing.T) {
	t.Parallel()

	cmd := &internalArgCmd{}
	remaining, err := populateArgs(cmd, []string{"a.txt", "b.txt", "c.txt", "d.txt"}, "")
	require.NoError(t, err)
	assert.Equal(t, "a.txt", cmd.Source)
	assert.Equal(t, "b.txt", cmd.Dest)
	assert.Equal(t, []string{"c.txt", "d.txt"}, cmd.Extra)
	assert.Empty(t, remaining)
}

func TestPopulateArgs_MissingRequired(t *testing.T) {
	t.Parallel()

	cmd := &internalArgCmd{}
	_, err := populateArgs(cmd, []string{"a.txt"}, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRequiredArg)
}

type internalOptionalArgCmd struct {
	Name string `arg:"name" required:"false" help:"Name"`
}

func (c *internalOptionalArgCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_Optional(t *testing.T) {
	t.Parallel()

	cmd := &internalOptionalArgCmd{}
	remaining, err := populateArgs(cmd, nil, "")
	require.NoError(t, err)
	assert.Empty(t, cmd.Name)
	assert.Empty(t, remaining)
}

func TestPopulateArgs_NonStruct(t *testing.T) {
	t.Parallel()

	cmd := RunFunc(func(_ context.Context) error { return nil })
	remaining, err := populateArgs(cmd, []string{"foo"}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, remaining)
}

// --- Arg validators ---

func TestExactArgs(t *testing.T) {
	t.Parallel()

	require.NoError(t, ExactArgs(2)([]string{"a", "b"}))
	require.Error(t, ExactArgs(2)([]string{"a"}))
	require.Error(t, ExactArgs(2)([]string{"a", "b", "c"}))
}

func TestMinArgs(t *testing.T) {
	t.Parallel()

	require.NoError(t, MinArgs(1)([]string{"a", "b"}))
	require.NoError(t, MinArgs(1)([]string{"a"}))
	require.Error(t, MinArgs(1)(nil))
}

func TestMaxArgs(t *testing.T) {
	t.Parallel()

	require.NoError(t, MaxArgs(2)([]string{"a"}))
	require.NoError(t, MaxArgs(2)([]string{"a", "b"}))
	require.Error(t, MaxArgs(2)([]string{"a", "b", "c"}))
}

func TestRangeArgs(t *testing.T) {
	t.Parallel()

	require.NoError(t, RangeArgs(1, 3)([]string{"a"}))
	require.NoError(t, RangeArgs(1, 3)([]string{"a", "b", "c"}))
	require.Error(t, RangeArgs(1, 3)(nil))
	require.Error(t, RangeArgs(1, 3)([]string{"a", "b", "c", "d"}))
}

func TestNoArgs(t *testing.T) {
	t.Parallel()

	require.NoError(t, NoArgs(nil))
	require.NoError(t, NoArgs([]string{}))
	require.Error(t, NoArgs([]string{"a"}))
}

// --- buildArgUsage ---

func TestBuildArgUsage(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []ArgDef
		want string
	}{
		"no args":  {args: nil, want: "[args...]"},
		"required": {args: []ArgDef{{Name: "file", Required: true}}, want: "<file>"},
		"optional": {args: []ArgDef{{Name: "file", Required: false}}, want: "[file]"},
		"slice":    {args: []ArgDef{{Name: "files", IsSlice: true}}, want: "[files...]"},
		"mixed": {
			args: []ArgDef{
				{Name: "src", Required: true},
				{Name: "dest", Required: true},
				{Name: "extra", IsSlice: true},
			},
			want: "<src> <dest> [extra...]",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, buildArgUsage(tt.args))
		})
	}
}

// --- Help rendering with args ---

type internalArgHelpCmd struct {
	Port   int    `flag:"port" default:"8080" help:"Port"`
	Source string `arg:"source" help:"Source file"`
	Dest   string `arg:"dest" help:"Destination"`
}

func (c *internalArgHelpCmd) Run(_ context.Context) error { return nil }
func (c *internalArgHelpCmd) Name() string                { return "copy" }

// --- Flag group validation ---

type mutexGroupCmd struct {
	JSON bool `flag:"json" help:"JSON output"`
	YAML bool `flag:"yaml" help:"YAML output"`
	Text bool `flag:"text" help:"Text output"`
}

func (c *mutexGroupCmd) Run(_ context.Context) error { return nil }
func (c *mutexGroupCmd) Name() string                { return "format" }
func (c *mutexGroupCmd) FlagGroups() []FlagGroup {
	return []FlagGroup{MutuallyExclusive("json", "yaml", "text")}
}

type requiredTogetherGroupCmd struct {
	Username string `flag:"username" help:"Username"`
	Password string `flag:"password" help:"Password"`
	Host     string `flag:"host" help:"Host"`
}

func (c *requiredTogetherGroupCmd) Run(_ context.Context) error { return nil }
func (c *requiredTogetherGroupCmd) Name() string                { return "login" }
func (c *requiredTogetherGroupCmd) FlagGroups() []FlagGroup {
	return []FlagGroup{RequiredTogether("username", "password")}
}

type oneRequiredGroupCmd struct {
	File  string `flag:"file" help:"Input file"`
	Stdin bool   `flag:"stdin" help:"Read from stdin"`
	URL   string `flag:"url" help:"Fetch from URL"`
}

func (c *oneRequiredGroupCmd) Run(_ context.Context) error { return nil }
func (c *oneRequiredGroupCmd) Name() string                { return "read" }
func (c *oneRequiredGroupCmd) FlagGroups() []FlagGroup {
	return []FlagGroup{OneRequired("file", "stdin", "url")}
}

func TestValidateFlagGroups_MutuallyExclusive_OK(t *testing.T) {
	t.Parallel()
	cmd := &mutexGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"json": true})
	require.NoError(t, err)
}

func TestValidateFlagGroups_MutuallyExclusive_None(t *testing.T) {
	t.Parallel()
	cmd := &mutexGroupCmd{}
	err := validateFlagGroups(cmd, nil)
	require.NoError(t, err)
}

func TestValidateFlagGroups_MutuallyExclusive_Violation(t *testing.T) {
	t.Parallel()
	cmd := &mutexGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"json": true, "yaml": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestValidateFlagGroups_RequiredTogether_OK_All(t *testing.T) {
	t.Parallel()
	cmd := &requiredTogetherGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"username": true, "password": true})
	require.NoError(t, err)
}

func TestValidateFlagGroups_RequiredTogether_OK_None(t *testing.T) {
	t.Parallel()
	cmd := &requiredTogetherGroupCmd{}
	err := validateFlagGroups(cmd, nil)
	require.NoError(t, err)
}

func TestValidateFlagGroups_RequiredTogether_Violation(t *testing.T) {
	t.Parallel()
	cmd := &requiredTogetherGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"username": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be set together")
}

func TestValidateFlagGroups_OneRequired_OK(t *testing.T) {
	t.Parallel()
	cmd := &oneRequiredGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"file": true})
	require.NoError(t, err)
}

func TestValidateFlagGroups_OneRequired_None(t *testing.T) {
	t.Parallel()
	cmd := &oneRequiredGroupCmd{}
	err := validateFlagGroups(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOneRequired)
}

func TestValidateFlagGroups_OneRequired_TooMany(t *testing.T) {
	t.Parallel()
	cmd := &oneRequiredGroupCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"file": true, "stdin": true})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOneRequired)
}

func TestValidateFlagGroups_NoInterface(t *testing.T) {
	t.Parallel()
	cmd := &internalBareCmd{}
	err := validateFlagGroups(cmd, map[string]bool{"port": true})
	require.NoError(t, err)
}

// --- allSubcommands alias coverage ---

type internalAliasedCmd struct {
	n       string
	aliases []string
}

func (c *internalAliasedCmd) Run(_ context.Context) error { return nil }
func (c *internalAliasedCmd) Name() string                { return c.n }
func (c *internalAliasedCmd) Aliases() []string           { return c.aliases }

func TestAllSubcommands_BuiltinAliasBlocksDiscovered(t *testing.T) {
	t.Parallel()

	// Builtin "deploy" has alias "d". Discovered command named "d" should be blocked.
	parent := &internalDiscoverParent{
		subs:       []Commander{&internalAliasedCmd{n: "deploy", aliases: []string{"d", "dep"}}},
		discovered: []Commander{&internalSimpleCmd{n: "d"}},
	}

	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "deploy", resolveInfo(subs[0]).name)
}

func TestAllSubcommands_DiscoveredAliasesTracked(t *testing.T) {
	t.Parallel()

	// Discovered "plugin" has alias "p". A second discovered "p" should be blocked.
	parent := &internalDiscoverParent{
		discovered: []Commander{
			&internalAliasedCmd{n: "plugin", aliases: []string{"p"}},
			&internalSimpleCmd{n: "p"},
		},
	}

	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "plugin", resolveInfo(subs[0]).name)
}

// --- Embedded Commander subcommands ---

type embeddedServeCmd struct {
	Port int `flag:"port" default:"8080"`
	ran  bool
}

func (c *embeddedServeCmd) Name() string                { return "serve" }
func (c *embeddedServeCmd) Run(_ context.Context) error { c.ran = true; return nil }

type embeddedConfigCmd struct {
	ran bool
}

func (c *embeddedConfigCmd) Name() string                { return "config" }
func (c *embeddedConfigCmd) Run(_ context.Context) error { c.ran = true; return nil }

type embeddedParentCmd struct {
	Verbose bool             `flag:"verbose"`
	Serve   embeddedServeCmd // implements Commander → subcommand
	Config  embeddedConfigCmd
}

func (c *embeddedParentCmd) Run(_ context.Context) error { return nil }

func TestAllSubcommands_EmbeddedCommander(t *testing.T) {
	t.Parallel()

	parent := &embeddedParentCmd{}
	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 2)

	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = resolveInfo(s).name
	}
	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "config")
}

type embeddedWithInterfaceCmd struct {
	Serve embeddedServeCmd
}

func (c *embeddedWithInterfaceCmd) Run(_ context.Context) error { return nil }
func (c *embeddedWithInterfaceCmd) Subcommands() []Commander {
	return []Commander{&embeddedConfigCmd{}}
}

func TestAllSubcommands_EmbeddedAndInterface(t *testing.T) {
	t.Parallel()

	parent := &embeddedWithInterfaceCmd{}
	subs, err := allSubcommands(parent)
	require.NoError(t, err)
	require.Len(t, subs, 2) // embedded "serve" + interface "config"

	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = resolveInfo(s).name
	}
	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "config")
}

type embeddedWithFlagTagCmd struct {
	Serve embeddedServeCmd `flag:"serve"` // invalid: Commander with flag tag
}

func (c *embeddedWithFlagTagCmd) Run(_ context.Context) error { return nil }

func TestAllSubcommands_EmbeddedWithFlagTag_Error(t *testing.T) {
	t.Parallel()

	parent := &embeddedWithFlagTagCmd{}
	_, err := allSubcommands(parent)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTag)
	assert.Contains(t, err.Error(), "implements Commander but has flag tag")
}

type duplicateNameCmd1 struct{}

func (c *duplicateNameCmd1) Name() string                { return "dupe" }
func (c *duplicateNameCmd1) Run(_ context.Context) error { return nil }

type duplicateNameCmd2 struct{}

func (c *duplicateNameCmd2) Name() string                { return "dupe" }
func (c *duplicateNameCmd2) Run(_ context.Context) error { return nil }

type embeddedDuplicateNamesCmd struct {
	A duplicateNameCmd1
	B duplicateNameCmd2
}

func (c *embeddedDuplicateNamesCmd) Run(_ context.Context) error { return nil }

func TestAllSubcommands_DuplicateNames_Error(t *testing.T) {
	t.Parallel()

	parent := &embeddedDuplicateNamesCmd{}
	_, err := allSubcommands(parent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate subcommand name")
}

type embeddedAndInterfaceCollisionCmd struct {
	Serve embeddedServeCmd // name: "serve"
}

func (c *embeddedAndInterfaceCollisionCmd) Run(_ context.Context) error { return nil }
func (c *embeddedAndInterfaceCollisionCmd) Subcommands() []Commander {
	return []Commander{&embeddedServeCmd{}} // also "serve" — collision!
}

func TestAllSubcommands_EmbeddedAndInterfaceCollision_Error(t *testing.T) {
	t.Parallel()

	parent := &embeddedAndInterfaceCollisionCmd{}
	_, err := allSubcommands(parent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subcommand name collision")
	assert.Contains(t, err.Error(), "serve")
}

// --- Branching commands ---

type branchDeleteCmd struct {
	Force  bool `flag:"force"`
	userID int
}

func (c *branchDeleteCmd) Name() string { return "delete" }
func (c *branchDeleteCmd) Run(ctx context.Context) error {
	c.userID = Get[int](ctx, "id")
	return nil
}

type branchRenameCmd struct {
	NewName string `arg:"name"`
	userID  int
}

func (c *branchRenameCmd) Name() string { return "rename" }
func (c *branchRenameCmd) Run(ctx context.Context) error {
	c.userID = Get[int](ctx, "id")
	return nil
}

type branchingUserCmd struct {
	ID     int             `arg:"id"`
	Delete branchDeleteCmd // subcommand
	Rename branchRenameCmd // subcommand
}

func (c *branchingUserCmd) Name() string                { return "user" }
func (c *branchingUserCmd) Run(_ context.Context) error { return nil }

func TestIsBranchingCommand(t *testing.T) {
	t.Parallel()

	// Has both arg and Commander fields → branching
	assert.True(t, isBranchingCommand(&branchingUserCmd{}))

	// Has only Commander fields → not branching
	assert.False(t, isBranchingCommand(&embeddedParentCmd{}))

	// Has only arg fields → not branching
	assert.False(t, isBranchingCommand(&internalArgCmd{}))
}

type branchingAppCmd struct {
	User branchingUserCmd
}

func (c *branchingAppCmd) Name() string                { return "app" }
func (c *branchingAppCmd) Run(_ context.Context) error { return nil }

func TestExecute_Branching(t *testing.T) {
	t.Parallel()

	app := &branchingAppCmd{}
	err := Execute(context.Background(), app, []string{"user", "42", "delete", "--force"})
	require.NoError(t, err)

	assert.Equal(t, 42, app.User.ID)
	assert.True(t, app.User.Delete.Force)
	assert.Equal(t, 42, app.User.Delete.userID)
}

func TestExecute_BranchingWithSubcommandArg(t *testing.T) {
	t.Parallel()

	app := &branchingAppCmd{}
	err := Execute(context.Background(), app, []string{"user", "123", "rename", "newname"})
	require.NoError(t, err)

	assert.Equal(t, 123, app.User.ID)
	assert.Equal(t, "newname", app.User.Rename.NewName)
	assert.Equal(t, 123, app.User.Rename.userID)
}

func TestExecute_EmbeddedSubcommands(t *testing.T) {
	t.Parallel()

	app := &embeddedParentCmd{}
	err := Execute(context.Background(), app, []string{"--verbose", "serve", "--port", "9090"})
	require.NoError(t, err)

	assert.True(t, app.Verbose)
	assert.True(t, app.Serve.ran)
	assert.Equal(t, 9090, app.Serve.Port)
}

// --- discoverDir error on unreadable existing directory ---

func TestDiscoverDir_UnreadableDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific permission test")
	}

	dir := t.TempDir()
	unreadable := filepath.Join(dir, "noperm")
	require.NoError(t, os.Mkdir(unreadable, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o750) //nolint:errcheck,gosec // best-effort cleanup
	})

	seen := make(map[string]bool)
	runners, err := discoverDir(unreadable, seen, "--cli-info", 2*time.Second)
	require.Error(t, err)
	assert.Nil(t, runners)
}

// --- discoverPATH edge cases ---

func TestDiscoverPATH_UnreadableEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific permission test")
	}

	dir := t.TempDir()
	unreadable := filepath.Join(dir, "noperm")
	require.NoError(t, os.Mkdir(unreadable, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o750) //nolint:errcheck,gosec // best-effort cleanup
	})

	t.Setenv("PATH", unreadable)

	seen := make(map[string]bool)
	runners := discoverPATH("myapp", seen, "--cli-info", 2*time.Second)
	assert.Empty(t, runners)
}

func TestDiscoverPATH_DirectoryEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()
	// Create a directory whose name matches the prefix pattern.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "myapp-subdir"), 0o750))

	t.Setenv("PATH", dir)

	seen := make(map[string]bool)
	runners := discoverPATH("myapp", seen, "--cli-info", 2*time.Second)
	assert.Empty(t, runners)
}

func TestDiscoverPATH_NonExecutableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()
	// Create a non-executable file matching the prefix.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "myapp-tool"), []byte("not executable"), 0o600))

	t.Setenv("PATH", dir)

	seen := make(map[string]bool)
	runners := discoverPATH("myapp", seen, "--cli-info", 2*time.Second)
	assert.Empty(t, runners)
}

// --- storeArgs ---

type internalStoreArgsParent struct {
	UserID int `arg:"user-id"`
}

func (c *internalStoreArgsParent) Run(_ context.Context) error { return nil }

type internalStoreArgsChild struct {
	Name string `arg:"name"`
}

func (c *internalStoreArgsChild) Run(_ context.Context) error { return nil }

func TestStoreArgs(t *testing.T) {
	t.Parallel()

	parent := &internalStoreArgsParent{UserID: 42}
	child := &internalStoreArgsChild{Name: "alice"}
	chain := []Commander{parent, child}

	ctx := storeArgs(context.Background(), chain)

	userID, ok := Lookup[int](ctx, "user-id")
	require.True(t, ok)
	assert.Equal(t, 42, userID)

	name, ok := Lookup[string](ctx, "name")
	require.True(t, ok)
	assert.Equal(t, "alice", name)
}

func TestStoreArgs_NonStructSkipped(t *testing.T) {
	t.Parallel()

	fn := RunFunc(func(_ context.Context) error { return nil })
	chain := []Commander{fn}

	ctx := storeArgs(context.Background(), chain)

	// No args stored, Lookup returns false.
	_, ok := Lookup[string](ctx, "anything")
	assert.False(t, ok)
}

func TestStoreArgs_ChildOverwritesParent(t *testing.T) {
	t.Parallel()

	parent := &struct {
		ID int `arg:"id"`
		internalBareCmd
	}{ID: 1}
	child := &struct {
		ID int `arg:"id"`
		internalBareCmd
	}{ID: 2}
	chain := []Commander{parent, child}

	ctx := storeArgs(context.Background(), chain)

	// Child is processed after parent, so child's value wins.
	assert.Equal(t, 2, Get[int](ctx, "id"))
}

func TestStoreArgs_AutoDerivedName(t *testing.T) {
	t.Parallel()

	cmd := &struct {
		TargetEnv string `arg:""`
		internalBareCmd
	}{TargetEnv: "prod"}
	chain := []Commander{cmd}

	ctx := storeArgs(context.Background(), chain)

	val, ok := Lookup[string](ctx, "target-env")
	require.True(t, ok)
	assert.Equal(t, "prod", val)
}

func TestStoreArgs_FlagsNotStored(t *testing.T) {
	t.Parallel()

	cmd := &struct {
		Port   int    `flag:"port"`
		Target string `arg:"target"`
		internalBareCmd
	}{Port: 8080, Target: "prod"}
	chain := []Commander{cmd}

	ctx := storeArgs(context.Background(), chain)

	// Args are stored.
	target, ok := Lookup[string](ctx, "target")
	require.True(t, ok)
	assert.Equal(t, "prod", target)

	// Flags are NOT stored.
	_, ok = Lookup[int](ctx, "port")
	assert.False(t, ok)
}

// --- HelpAppender / HelpPrepender ---

type internalSectionedCmd struct {
	Port int `flag:"port" default:"8080" help:"Port"`
}

func (c *internalSectionedCmd) Run(_ context.Context) error { return nil }
func (c *internalSectionedCmd) Name() string                { return "jira" }
func (c *internalSectionedCmd) Description() string         { return "Interact with Jira" }

func (c *internalSectionedCmd) AppendHelp() []HelpSection {
	return []HelpSection{
		{
			Header: "Required Tokens",
			Body:   "  JIRA_TOKEN    Jira API token (env: JIRA_TOKEN)",
		},
		{
			Header: "See Also",
			Body:   "  https://docs.example.com/jira",
		},
	}
}

func TestDefaultRenderHelp_AppendHelp(t *testing.T) {
	t.Parallel()

	cmd := &internalSectionedCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	assert.Contains(t, text, "Required Tokens:\n")
	assert.Contains(t, text, "  JIRA_TOKEN    Jira API token (env: JIRA_TOKEN)")
	assert.Contains(t, text, "See Also:\n")
	assert.Contains(t, text, "  https://docs.example.com/jira")

	// Appended sections appear before footer.
	tokensIdx := strings.Index(text, "Required Tokens:")
	footerIdx := strings.Index(text, "Use ")
	if footerIdx >= 0 {
		assert.Greater(t, footerIdx, tokensIdx)
	}
}

type internalPrependCmd struct {
	Port int `flag:"port" default:"8080" help:"Port"`
}

func (c *internalPrependCmd) Run(_ context.Context) error { return nil }
func (c *internalPrependCmd) Name() string                { return "vpn-tool" }
func (c *internalPrependCmd) Description() string         { return "VPN management" }

func (c *internalPrependCmd) PrependHelp() []HelpSection {
	return []HelpSection{{
		Header: "Notice",
		Body:   "  This command requires VPN access.",
	}}
}

func TestDefaultRenderHelp_PrependHelp(t *testing.T) {
	t.Parallel()

	cmd := &internalPrependCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	assert.Contains(t, text, "Notice:\n")
	assert.Contains(t, text, "  This command requires VPN access.")

	// Prepended sections appear before Usage.
	noticeIdx := strings.Index(text, "Notice:")
	usageIdx := strings.Index(text, "Usage:")
	assert.Greater(t, usageIdx, noticeIdx)
}

type internalBothSectionsCmd struct {
	internalBareCmd
}

func (c *internalBothSectionsCmd) PrependHelp() []HelpSection {
	return []HelpSection{{Header: "Before", Body: "  prepend content"}}
}

func (c *internalBothSectionsCmd) AppendHelp() []HelpSection {
	return []HelpSection{{Header: "After", Body: "  append content"}}
}

func TestDefaultRenderHelp_PrependAndAppend(t *testing.T) {
	t.Parallel()

	cmd := &internalBothSectionsCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	assert.Contains(t, text, "Before:\n")
	assert.Contains(t, text, "  prepend content")
	assert.Contains(t, text, "After:\n")
	assert.Contains(t, text, "  append content")

	// Prepend before Usage, Append after Usage.
	beforeIdx := strings.Index(text, "Before:")
	usageIdx := strings.Index(text, "Usage:")
	afterIdx := strings.Index(text, "After:")
	assert.Greater(t, usageIdx, beforeIdx)
	assert.Greater(t, afterIdx, usageIdx)
}

func TestDefaultRenderHelp_NoSections(t *testing.T) {
	t.Parallel()

	cmd := &internalBareCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	// No "Required Tokens" or other custom sections.
	assert.NotContains(t, text, "Required Tokens:")
}

func TestDefaultRenderHelp_WithArgs(t *testing.T) {
	t.Parallel()

	cmd := &internalArgHelpCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "<source> <dest>")
	assert.Contains(t, text, "Arguments:\n")
	assert.Contains(t, text, "source")
	assert.Contains(t, text, "dest")
	assert.Contains(t, text, "(required)")
}

// --- Signal Handling ---

type signalCmd struct {
	ctxErr error
}

func (c *signalCmd) Run(ctx context.Context) error {
	// Send SIGINT to ourselves.
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	if err := p.Signal(os.Interrupt); err != nil {
		return err
	}
	// Give the signal handler a moment.
	time.Sleep(50 * time.Millisecond)
	c.ctxErr = ctx.Err()
	return nil
}

func TestSignalHandling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal test not supported on windows")
	}

	cmd := &signalCmd{}
	err := Execute(context.Background(), cmd, nil, WithSignalHandling(true))
	require.NoError(t, err)
	assert.ErrorIs(t, cmd.ctxErr, context.Canceled)
}

type noopCmd struct {
	ctxErr error
}

func (c *noopCmd) Run(ctx context.Context) error {
	c.ctxErr = ctx.Err()
	return nil
}

func TestSignalHandling_Disabled(t *testing.T) {
	t.Parallel()

	cmd := &noopCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.NoError(t, cmd.ctxErr)
}

// --- Error Formatting ---

func TestIsUsageError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want bool
	}{
		// Flag errors
		"unknown flag": {
			err:  fmt.Errorf("%w: --foo", ErrUnknownFlag),
			want: true,
		},
		"required flag": {
			err:  fmt.Errorf("%w: --name", ErrRequiredFlag),
			want: true,
		},
		"flag requires value": {
			err:  fmt.Errorf("%w: --port", ErrFlagRequiresVal),
			want: true,
		},
		"invalid flag value": {
			err:  fmt.Errorf("%w: --port: not a number", ErrInvalidFlagValue),
			want: true,
		},
		"parent ErrFlag": {
			err:  fmt.Errorf("something: %w", ErrFlag),
			want: true,
		},
		// Argument errors
		"required arg": {
			err:  fmt.Errorf("%w: src", ErrRequiredArg),
			want: true,
		},
		"invalid arg value": {
			err:  fmt.Errorf("%w: port: not a number", ErrInvalidArgValue),
			want: true,
		},
		"arg count": {
			err:  fmt.Errorf("%w: expected 2, got 3", ErrArgCount),
			want: true,
		},
		"parent ErrArgument": {
			err:  fmt.Errorf("something: %w", ErrArgument),
			want: true,
		},
		// Command errors
		"unknown command": {
			err:  fmt.Errorf("%w: foo", ErrUnknownCommand),
			want: true,
		},
		"parent ErrCommand": {
			err:  fmt.Errorf("something: %w", ErrCommand),
			want: true,
		},
		// Flag group errors
		"mutually exclusive": {
			err:  fmt.Errorf("%w: --a, --b", ErrMutuallyExclusive),
			want: true,
		},
		"required together": {
			err:  fmt.Errorf("%w: --a, --b", ErrRequiredTogether),
			want: true,
		},
		"one required": {
			err:  fmt.Errorf("%w: --a, --b", ErrOneRequired),
			want: true,
		},
		"parent ErrFlagGroup": {
			err:  fmt.Errorf("something: %w", ErrFlagGroup),
			want: true,
		},
		// Non-usage errors
		"unsupported type": {
			err:  fmt.Errorf("%w: complex128", ErrUnsupportedType),
			want: false,
		},
		"invalid tag": {
			err:  fmt.Errorf("%w: bad tag", ErrInvalidTag),
			want: false,
		},
		"generic error": {
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		"exit error": {
			err:  Exit("done", 1),
			want: false,
		},
		"nil error": {
			err:  nil,
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isUsageError(tt.err))
		})
	}
}

type namedRoot struct {
	Name_ string `flag:"name" required:"true"`
}

func (c *namedRoot) Run(_ context.Context) error { return nil }
func (c *namedRoot) Name() string                { return "myapp" }

func TestFormatError(t *testing.T) {
	t.Parallel()

	root := &namedRoot{}

	tests := map[string]struct {
		err       error
		wantError string
		wantHint  bool
	}{
		"flag error includes hint": {
			err:       fmt.Errorf("%w: --name", ErrRequiredFlag),
			wantError: "Error: required flag not provided: --name\n",
			wantHint:  true,
		},
		"unknown flag includes hint": {
			err:       fmt.Errorf("%w: --foo", ErrUnknownFlag),
			wantError: "Error: unknown flag: --foo\n",
			wantHint:  true,
		},
		"generic error no hint": {
			err:       errors.New("something failed"),
			wantError: "Error: something failed\n",
			wantHint:  false,
		},
		"exit coder no hint": {
			err:       Exit("shutting down", 2),
			wantError: "Error: shutting down\n",
			wantHint:  false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			opts := &options{stderr: &buf}
			formatError(opts, root, tt.err)

			output := buf.String()
			assert.Contains(t, output, tt.wantError)
			if tt.wantHint {
				assert.Contains(t, output, "Run 'myapp --help' for usage.\n")
			} else {
				assert.NotContains(t, output, "--help")
			}
		})
	}
}

func TestFormatError_SilenceErrors(t *testing.T) {
	t.Parallel()

	root := &namedRoot{}
	var buf bytes.Buffer
	opts := &options{stderr: &buf, silenceErrors: true}

	formatError(opts, root, fmt.Errorf("%w: --name", ErrRequiredFlag))

	assert.Empty(t, buf.String())
}

func TestFormatError_SilenceUsage(t *testing.T) {
	t.Parallel()

	root := &namedRoot{}
	var buf bytes.Buffer
	opts := &options{stderr: &buf, silenceUsage: true}

	formatError(opts, root, fmt.Errorf("%w: --name", ErrRequiredFlag))

	output := buf.String()
	assert.Contains(t, output, "Error: required flag not provided: --name")
	assert.NotContains(t, output, "--help")
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err      error
		wantCode int
	}{
		"exit coder": {
			err:      Exit("done", 42),
			wantCode: 42,
		},
		"generic error": {
			err:      errors.New("oops"),
			wantCode: 1,
		},
		"zero exit": {
			err:      Exit("ok", 0),
			wantCode: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantCode, exitCode(tt.err))
		})
	}
}

// --- Global Flags in Help ---

type globalFlagParent struct {
	Env    string `flag:"env" short:"e" required:"true" help:"Target environment" enum:"dev,qa,prod"`
	Format string `flag:"format" short:"f" default:"table" help:"Output format"`
	Secret string `flag:"secret" hidden:"true" help:"Hidden flag"`
}

func (c *globalFlagParent) Run(_ context.Context) error { return nil }
func (c *globalFlagParent) Name() string                { return "app" }
func (c *globalFlagParent) Subcommands() []Commander    { return []Commander{&globalFlagChild{}} }

type globalFlagChild struct {
	Port   int    `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	Format string `flag:"format" help:"Override format"` // overlaps with parent
}

func (c *globalFlagChild) Run(_ context.Context) error { return nil }
func (c *globalFlagChild) Name() string                { return "serve" }

func TestCollectGlobalFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		chain     []Commander
		leafFlags []FlagDef
		wantNames []string
	}{
		"parent flags shown": {
			chain:     []Commander{&globalFlagParent{}, &globalFlagChild{}},
			leafFlags: ScanFlags(&globalFlagChild{}),
			wantNames: []string{"env"}, // format is deduplicated, secret is hidden
		},
		"single command no globals": {
			chain:     []Commander{&globalFlagParent{}},
			leafFlags: ScanFlags(&globalFlagParent{}),
			wantNames: nil,
		},
		"all parent flags hidden": {
			chain: []Commander{
				&struct {
					Commander
					Secret string `flag:"secret" hidden:"true"`
				}{Commander: &globalFlagChild{}},
				&globalFlagChild{},
			},
			leafFlags: ScanFlags(&globalFlagChild{}),
			wantNames: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			global := collectGlobalFlags(tt.chain, tt.leafFlags)
			var names []string
			for _, f := range global {
				if !f.Hidden {
					names = append(names, f.Name)
				}
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

func TestDefaultRenderHelp_GlobalFlags(t *testing.T) {
	t.Parallel()

	parent := &globalFlagParent{}
	child := &globalFlagChild{}
	chain := []Commander{parent, child}
	leafFlags := ScanFlags(child)
	globalFlags := collectGlobalFlags(chain, leafFlags)

	text := defaultRenderHelp(child, chain, leafFlags, globalFlags, false)

	// Leaf flags in "Flags:" section.
	assert.Contains(t, text, "Flags:\n")
	assert.Contains(t, text, "--port")

	// Parent flags in "Global Flags:" section.
	assert.Contains(t, text, "Global Flags:\n")
	assert.Contains(t, text, "--env")

	// Overlapping "format" only in leaf Flags, not Global Flags.
	lines := strings.Split(text, "\n")
	inGlobal := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Global Flags:") {
			inGlobal = true
			continue
		}
		if inGlobal && strings.TrimSpace(line) == "" {
			break
		}
		if inGlobal {
			assert.NotContains(t, line, "--format")
		}
	}

	// Hidden parent flag not shown.
	assert.NotContains(t, text, "--secret")
}

func TestDefaultRenderHelp_NoGlobalFlags(t *testing.T) {
	t.Parallel()

	cmd := &globalFlagChild{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.NotContains(t, text, "Global Flags:")
}

type multiLevelGrandparent struct {
	Region string `flag:"region" help:"AWS region"`
}

func (c *multiLevelGrandparent) Run(_ context.Context) error { return nil }
func (c *multiLevelGrandparent) Name() string                { return "app" }

type multiLevelParent struct {
	Env    string `flag:"env" help:"Environment"`
	Region string `flag:"region" help:"Override region"` // overlap with grandparent
}

func (c *multiLevelParent) Run(_ context.Context) error { return nil }
func (c *multiLevelParent) Name() string                { return "deploy" }

type multiLevelChild struct {
	Port int `flag:"port" help:"Port"`
}

func (c *multiLevelChild) Run(_ context.Context) error { return nil }
func (c *multiLevelChild) Name() string                { return "run" }

func TestDefaultRenderHelp_MultiLevelGlobalFlags(t *testing.T) {
	t.Parallel()

	gp := &multiLevelGrandparent{}
	p := &multiLevelParent{}
	child := &multiLevelChild{}
	chain := []Commander{gp, p, child}
	leafFlags := ScanFlags(child)
	globalFlags := collectGlobalFlags(chain, leafFlags)

	text := defaultRenderHelp(child, chain, leafFlags, globalFlags, false)

	assert.Contains(t, text, "Global Flags:\n")
	// Region from grandparent should be deduplicated by parent's region.
	// Only one --region should appear.
	assert.Equal(t, 1, strings.Count(text, "--region"))
	assert.Contains(t, text, "--env")
}

// --- Interactive Prompts ---

func TestDefaultIsTerminal(t *testing.T) {
	t.Parallel()

	// Just verify it doesn't panic and returns a bool.
	_ = defaultIsTerminal()
}

func TestReadPrompt(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		flag       FlagDef
		input      string
		wantPrompt string
		wantValue  string
	}{
		"uses help as label": {
			flag:       FlagDef{Name: "env", Help: "Target environment"},
			input:      "prod\n",
			wantPrompt: "Target environment: ",
			wantValue:  "prod",
		},
		"falls back to name": {
			flag:       FlagDef{Name: "env"},
			input:      "dev\n",
			wantPrompt: "env: ",
			wantValue:  "dev",
		},
		"empty input": {
			flag:       FlagDef{Name: "env", Help: "Target environment"},
			input:      "\n",
			wantPrompt: "Target environment: ",
			wantValue:  "",
		},
		"EOF without newline": {
			flag:       FlagDef{Name: "env", Help: "Target environment"},
			input:      "staging",
			wantPrompt: "Target environment: ",
			wantValue:  "staging",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			scanner := bufio.NewScanner(strings.NewReader(tt.input))

			val, err := readPrompt(tt.flag, &stderr, scanner)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrompt, stderr.String())
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

type promptCmd struct {
	Name_ string `flag:"name" required:"true" help:"Your name"`
	Age   int    `flag:"age" required:"true" help:"Your age"`
	Host  string `flag:"host" default:"localhost" help:"Hostname"`
}

func (c *promptCmd) Run(_ context.Context) error { return nil }

func TestPromptForFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input       string
		provided    map[string]bool
		terminal    bool
		interactive bool
		wantName    string
		wantAge     int
		wantProv    map[string]bool
		assertErr   require.ErrorAssertionFunc
	}{
		"prompts for missing required flags": {
			input:       "Alice\n30\n",
			interactive: true,
			terminal:    true,
			wantName:    "Alice",
			wantAge:     30,
			wantProv:    map[string]bool{"name": true, "age": true},
			assertErr:   require.NoError,
		},
		"skips already provided flags": {
			input:       "25\n",
			provided:    map[string]bool{"name": true},
			interactive: true,
			terminal:    true,
			wantName:    "",
			wantAge:     25,
			wantProv:    map[string]bool{"name": true, "age": true},
			assertErr:   require.NoError,
		},
		"no prompts when not interactive": {
			input:       "",
			interactive: false,
			terminal:    true,
			assertErr:   require.NoError,
		},
		"no prompts when not terminal": {
			input:       "Alice\n30\n",
			interactive: true,
			terminal:    false,
			assertErr:   require.NoError,
		},
		"empty input skipped": {
			input:       "\n30\n",
			interactive: true,
			terminal:    true,
			wantName:    "",
			wantAge:     30,
			wantProv:    map[string]bool{"age": true},
			assertErr:   require.NoError,
		},
		"invalid type returns error": {
			input:       "Alice\nnot-a-number\n",
			interactive: true,
			terminal:    true,
			assertErr:   require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &promptCmd{}
			opts := &options{
				stderr:      &bytes.Buffer{},
				stdin:       strings.NewReader(tt.input),
				isTerminal:  func() bool { return tt.terminal },
				interactive: tt.interactive,
			}

			prov, err := promptForFlags(cmd, tt.provided, opts)
			tt.assertErr(t, err)
			if err != nil {
				return
			}
			assert.Equal(t, tt.wantProv, prov)
			assert.Equal(t, tt.wantName, cmd.Name_)
			assert.Equal(t, tt.wantAge, cmd.Age)
		})
	}
}

type customPrompterCmd struct {
	Env string `flag:"env" required:"true" help:"Target environment"`
}

func (c *customPrompterCmd) Run(_ context.Context) error { return nil }

func (c *customPrompterCmd) Prompt(flag FlagDef) (string, error) {
	if flag.Name == "env" {
		return "production", nil
	}
	return "", nil
}

func TestPromptForFlags_CustomPrompter(t *testing.T) {
	t.Parallel()

	cmd := &customPrompterCmd{}
	opts := &options{
		stderr:      &bytes.Buffer{},
		stdin:       strings.NewReader(""),
		isTerminal:  func() bool { return true },
		interactive: true,
	}

	prov, err := promptForFlags(cmd, nil, opts)
	require.NoError(t, err)
	assert.Equal(t, "production", cmd.Env)
	assert.True(t, prov["env"])
}

type interactiveCmd struct {
	Name_ string `flag:"name" required:"true" help:"Your name"`
}

func (c *interactiveCmd) Run(_ context.Context) error { return nil }

func TestExecute_InteractiveDisabled(t *testing.T) {
	t.Parallel()

	cmd := &interactiveCmd{}
	err := Execute(
		context.Background(), cmd, nil,
		WithStdin(strings.NewReader("Bob\n")),
	)
	// Without WithInteractive, prompting is disabled — required flag fails.
	require.Error(t, err)
}

func TestExecute_InteractivePrompt_MockTerminal(t *testing.T) {
	t.Parallel()

	cmd := &interactiveCmd{}
	o := defaults()
	o.interactive = true
	o.stdin = strings.NewReader("Charlie\n")
	o.isTerminal = func() bool { return true }

	err := execute(context.Background(), cmd, nil, o)
	require.NoError(t, err)
	assert.Equal(t, "Charlie", cmd.Name_)
}

// --- Required Flag Indicator ---

type requiredIndicatorCmd struct {
	Env    string `flag:"env" required:"true" help:"Target environment"`
	Format string `flag:"format" default:"table" help:"Output format"`
}

func (c *requiredIndicatorCmd) Run(_ context.Context) error { return nil }
func (c *requiredIndicatorCmd) Name() string                { return "app" }

func TestDefaultRenderHelp_RequiredFlagAsterisk(t *testing.T) {
	t.Parallel()

	cmd := &requiredIndicatorCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	// Required flags have * prefix.
	assert.Contains(t, text, "* ")
	assert.Contains(t, text, "--env")

	// Non-required flags have space prefix (no asterisk).
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.Contains(line, "--format") {
			assert.True(t, strings.HasPrefix(line, "  "), "non-required flag should have space prefix, got: %q", line)
		}
	}
}

// --- Env-only fields (standalone env tag) ---

type envOnlyCmd struct {
	Token string `env:"TEST_TOKEN" required:"true"`
	Port  int    `flag:"port" default:"8080" help:"Port to listen on"`
}

func (c *envOnlyCmd) Run(_ context.Context) error { return nil }
func (c *envOnlyCmd) Name() string                { return "app" }

func TestScanFlags_SkipsEnvOnly(t *testing.T) {
	t.Parallel()

	cmd := &envOnlyCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, "port", defs[0].Name)
}

func TestEnvOnly_PopulatedFromEnv(t *testing.T) {
	t.Setenv("TEST_TOKEN", "secret123")

	cmd := &envOnlyCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "secret123", cmd.Token)
}

func TestEnvOnly_RequiredValidation(t *testing.T) {
	t.Parallel()

	cmd := &envOnlyCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.ErrorIs(t, err, ErrRequiredFlag)
	assert.Contains(t, err.Error(), "token")
	assert.Contains(t, err.Error(), "env: TEST_TOKEN")
	assert.NotContains(t, err.Error(), "--")
}

type envOnlyDefaultCmd struct {
	Secret string `env:"TEST_SECRET" default:"fallback"`
}

func (c *envOnlyDefaultCmd) Run(_ context.Context) error { return nil }

func TestEnvOnly_DefaultApplied(t *testing.T) {
	t.Parallel()

	cmd := &envOnlyDefaultCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "fallback", cmd.Secret)
}

type envOnlyEnumCmd struct {
	Mode string `env:"TEST_MODE" enum:"fast,slow" required:"true"`
}

func (c *envOnlyEnumCmd) Run(_ context.Context) error { return nil }

func TestEnvOnly_EnumValidation(t *testing.T) {
	t.Setenv("TEST_MODE", "invalid")

	cmd := &envOnlyEnumCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "mode")
	assert.NotContains(t, err.Error(), "--")
}

func TestEnvOnly_EnumValid(t *testing.T) {
	t.Setenv("TEST_MODE", "fast")

	cmd := &envOnlyEnumCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "fast", cmd.Mode)
}

func TestEnvOnly_NotAcceptedAsCLIArg(t *testing.T) {
	t.Setenv("TEST_TOKEN", "from-env")

	cmd := &envOnlyCmd{}
	// --token is the derived name, but it is not a CLI flag.
	// It becomes a positional arg; the field is only set from env.
	err := Execute(context.Background(), cmd, []string{"--token", "val"})
	require.NoError(t, err)
	assert.Equal(t, "from-env", cmd.Token) // set from env, not from --token arg
}

func TestEnvOnly_NotInHelp(t *testing.T) {
	t.Setenv("TEST_TOKEN", "secret123")

	cmd := &envOnlyCmd{}
	flags := ScanFlags(cmd)
	text := defaultRenderHelp(cmd, []Commander{cmd}, flags, nil, false)
	assert.NotContains(t, text, "token")
	assert.NotContains(t, text, "TEST_TOKEN")
	assert.Contains(t, text, "port")
}

func TestEnvOnly_ConfigResolver(t *testing.T) {
	t.Parallel()

	cmd := &envOnlyCmd{}
	resolver := func(key ConfigKey) (string, bool) {
		if key.Name == "token" {
			return "from-config", true
		}
		return "", false
	}
	err := Execute(context.Background(), cmd, nil, WithConfigResolver(resolver))
	require.NoError(t, err)
	assert.Equal(t, "from-config", cmd.Token)
}

type envOnlyParentCmd struct {
	Token string `env:"TEST_PARENT_TOKEN"`
}

func (c *envOnlyParentCmd) Run(_ context.Context) error { return nil }
func (c *envOnlyParentCmd) Name() string                { return "parent" }
func (c *envOnlyParentCmd) Subcommands() []Commander {
	return []Commander{&envOnlyChildCmd{}}
}

type envOnlyChildCmd struct {
	Label string `flag:"label" default:"child"`
}

func (c *envOnlyChildCmd) Run(_ context.Context) error { return nil }
func (c *envOnlyChildCmd) Name() string                { return "child" }

func TestEnvOnly_InheritFlagsSkips(t *testing.T) {
	t.Setenv("TEST_PARENT_TOKEN", "parent-token")

	parent := &envOnlyParentCmd{}
	err := Execute(context.Background(), parent, []string{"child"})
	require.NoError(t, err)
	assert.Equal(t, "parent-token", parent.Token)
}

// --- Leaf context accessor ---

type leafParentCmd struct {
	capturedLeaf Commander
}

func (c *leafParentCmd) Run(_ context.Context) error { return nil }
func (c *leafParentCmd) Name() string                { return "app" }
func (c *leafParentCmd) Before(ctx context.Context) (context.Context, error) {
	c.capturedLeaf = Leaf(ctx)
	return ctx, nil
}

func (c *leafParentCmd) Subcommands() []Commander {
	return []Commander{&leafChildCmd{}}
}

type leafChildCmd struct{}

func (c *leafChildCmd) Run(_ context.Context) error { return nil }
func (c *leafChildCmd) Name() string                { return "child" }

func TestLeaf_AvailableInBefore(t *testing.T) {
	t.Parallel()

	parent := &leafParentCmd{}
	err := Execute(context.Background(), parent, []string{"child"})
	require.NoError(t, err)
	require.NotNil(t, parent.capturedLeaf)
	assert.Equal(t, "child", resolveInfo(parent.capturedLeaf).name)
}

func TestLeaf_IsLeafNotParent(t *testing.T) {
	t.Parallel()

	// When there are no subcommands, the leaf is the root itself.
	parent := &leafParentCmd{}
	err := Execute(context.Background(), parent, nil)
	require.NoError(t, err)
	require.NotNil(t, parent.capturedLeaf)
	assert.Equal(t, "app", resolveInfo(parent.capturedLeaf).name)
}

func TestLeaf_NilWithoutContext(t *testing.T) {
	t.Parallel()

	// Calling Leaf on a bare context returns nil.
	leaf := Leaf(context.Background())
	assert.Nil(t, leaf)
}

type leafCaptureCmd struct {
	captured Commander
}

func (c *leafCaptureCmd) Run(ctx context.Context) error {
	c.captured = Leaf(ctx)
	return nil
}
func (c *leafCaptureCmd) Name() string { return "child" }

func TestLeaf_AvailableInRun(t *testing.T) {
	t.Parallel()

	child := &leafCaptureCmd{}
	parent := &leafRunParent{child: child}

	err := Execute(context.Background(), parent, []string{"child"})
	require.NoError(t, err)
	require.NotNil(t, child.captured)
	assert.Equal(t, "child", resolveInfo(child.captured).name)
}

type leafRunParent struct {
	child Commander
}

func (c *leafRunParent) Run(_ context.Context) error { return nil }
func (c *leafRunParent) Name() string                { return "app" }
func (c *leafRunParent) Subcommands() []Commander {
	return []Commander{c.child}
}

// --- Leaf for auth pattern ---

type authenticated interface {
	Authenticate()
}

type authorized interface {
	Permissions() []string
}

type authRootCmd struct {
	needsAuth bool
	needsRBAC bool
	perms     []string
}

func (c *authRootCmd) Run(_ context.Context) error { return nil }
func (c *authRootCmd) Name() string                { return "myctl" }
func (c *authRootCmd) Before(ctx context.Context) (context.Context, error) {
	leaf := Leaf(ctx)
	_, c.needsAuth = leaf.(authenticated)
	if az, ok := leaf.(authorized); ok {
		c.needsRBAC = true
		c.perms = az.Permissions()
	}
	return ctx, nil
}

func (c *authRootCmd) Subcommands() []Commander {
	return []Commander{&unauthedCmd{}, &authedCmd{}, &rbacCmd{}}
}

type unauthedCmd struct{}

func (c *unauthedCmd) Run(_ context.Context) error { return nil }
func (c *unauthedCmd) Name() string                { return "status" }

type authedCmd struct{}

func (c *authedCmd) Run(_ context.Context) error { return nil }
func (c *authedCmd) Name() string                { return "list" }
func (c *authedCmd) Authenticate()               {}

type rbacCmd struct{}

func (c *rbacCmd) Run(_ context.Context) error { return nil }
func (c *rbacCmd) Name() string                { return "deploy" }
func (c *rbacCmd) Authenticate()               {}
func (c *rbacCmd) Permissions() []string       { return []string{"deploy:write"} }

func TestLeaf_AuthPattern(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		wantAuth  bool
		wantRBAC  bool
		wantPerms []string
	}{
		"unauthed": {
			args: []string{"status"},
		},
		"auth only": {
			args:     []string{"list"},
			wantAuth: true,
		},
		"auth + RBAC": {
			args:      []string{"deploy"},
			wantAuth:  true,
			wantRBAC:  true,
			wantPerms: []string{"deploy:write"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := &authRootCmd{}
			err := Execute(context.Background(), root, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAuth, root.needsAuth)
			assert.Equal(t, tt.wantRBAC, root.needsRBAC)
			assert.Equal(t, tt.wantPerms, root.perms)
		})
	}
}

type noRequiredFlagsCmd struct {
	Port int `flag:"port" default:"8080" help:"Port to listen on"`
}

func (c *noRequiredFlagsCmd) Run(_ context.Context) error { return nil }
func (c *noRequiredFlagsCmd) Name() string                { return "app" }

func TestDefaultRenderHelp_NoAsteriskWithoutRequired(t *testing.T) {
	t.Parallel()

	cmd := &noRequiredFlagsCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)

	// No required flags means no asterisk prefix anywhere in Flags section.
	lines := strings.Split(text, "\n")
	inFlags := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Flags:") {
			inFlags = true
			continue
		}
		if inFlags && strings.TrimSpace(line) == "" {
			break
		}
		if inFlags {
			assert.False(t, strings.HasPrefix(line, "* "), "unexpected asterisk in: %q", line)
		}
	}
}

// --- validateStructTags ---

func TestValidateStructTags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		makeType func() reflect.Type
		wantErr  string
	}{
		"flag + arg": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `flag:"name" arg:"name"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "flag and arg are mutually exclusive",
		},
		"required + default": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `flag:"name" required:"true" default:"x"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "required and default are mutually exclusive",
		},
		"short without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `short:"n"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "short requires flag",
		},
		"counter without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Count int `counter:"true"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "counter requires flag",
		},
		"counter on string": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `flag:"name" counter:"true"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "counter requires int or uint type",
		},
		"negate without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Force bool `negate:"true"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "negate requires flag",
		},
		"negate on int": {
			makeType: func() reflect.Type {
				type cmd struct {
					Count int `flag:"count" negate:"true"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "negate requires bool type",
		},
		"negatable tag present": {
			makeType: func() reflect.Type {
				type cmd struct {
					Color bool `flag:"color" negatable:"true"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "negatable renamed to negate",
		},
		"sep without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Tags []string `sep:","`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "sep requires flag",
		},
		"sep on string": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `flag:"name" sep:","`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "sep requires slice type",
		},
		"alt without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `alt:"alias"`
				}
				return reflect.TypeOf(cmd{})
			},
			wantErr: "alt requires flag",
		},
		"placeholder without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `placeholder:"NAME"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "placeholder requires flag",
		},
		"hidden without flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `hidden:"true"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "hidden requires flag",
		},
		"inherit tag present": {
			makeType: func() reflect.Type {
				type cmd struct {
					Env string `inherit:"env"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "inherit tag removed",
		},
		"default-mask present": {
			makeType: func() reflect.Type {
				type cmd struct {
					Secret string `flag:"secret" default-mask:"****"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "default-mask renamed to mask",
		},
		"flag dash present": {
			makeType: func() reflect.Type {
				type cmd struct {
					Token string `flag:"-" env:"TOKEN"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: `flag:"-" removed`,
		},
		"required without source": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `required:"true"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "required requires flag, arg, or env tag",
		},
		"default without source": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `default:"x"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "default requires flag, arg, or env tag",
		},
		"enum without source": {
			makeType: func() reflect.Type {
				type cmd struct {
					Mode string `enum:"a,b"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "enum requires flag, arg, or env tag",
		},
		"help without source": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `help:"something"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "help requires flag, arg, or env tag",
		},
		"mask without source": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `mask:"****"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "mask requires flag, arg, or env tag",
		},
		"valid flag": {
			makeType: func() reflect.Type {
				type cmd struct {
					Port int `flag:"port" short:"p" default:"8080" help:"Port"`
				}
				return reflect.TypeFor[cmd]()
			},
		},
		"valid standalone env": {
			makeType: func() reflect.Type {
				type cmd struct {
					Token string `env:"TOKEN" required:"true" help:"API token"`
				}
				return reflect.TypeFor[cmd]()
			},
		},
		"valid arg with enum": {
			makeType: func() reflect.Type {
				type cmd struct {
					Mode string `arg:"mode" enum:"a,b,c" help:"Mode"`
				}
				return reflect.TypeFor[cmd]()
			},
		},
		"prefix on non-struct": {
			makeType: func() reflect.Type {
				type cmd struct {
					Name string `prefix:"db-"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "prefix requires struct type",
		},
		"prefix on anonymous": {
			makeType: func() reflect.Type {
				type inner struct {
					Name string `flag:"name"`
				}
				type cmd struct {
					inner `prefix:"db-"`
				}
				return reflect.TypeFor[cmd]()
			},
			wantErr: "prefix cannot be used on anonymous",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateStructTags(tt.makeType())
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrInvalidTag)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// --- mask tag ---

type maskCmd struct {
	Secret string `flag:"secret" default:"hunter2" mask:"****" help:"A secret"`
}

func (c *maskCmd) Run(_ context.Context) error { return nil }
func (c *maskCmd) Name() string                { return "app" }

func TestMask_InHelpOutput(t *testing.T) {
	t.Parallel()

	cmd := &maskCmd{}
	flags := ScanFlags(cmd)
	text := defaultRenderHelp(cmd, []Commander{cmd}, flags, nil, false)
	assert.Contains(t, text, "(default: ****)")
	assert.NotContains(t, text, "hunter2")
}

func TestScanFlags_Mask(t *testing.T) {
	t.Parallel()

	cmd := &maskCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, "****", defs[0].Mask)
}

// --- Arg enhancements (enum, default, env, mask) ---

type argEnumCmd struct {
	Mode string `arg:"mode" enum:"a,b,c" help:"Mode"`
}

func (c *argEnumCmd) Run(_ context.Context) error { return nil }
func (c *argEnumCmd) Name() string                { return "app" }

func TestArgEnum_ValidValue(t *testing.T) {
	t.Parallel()

	cmd := &argEnumCmd{}
	err := Execute(context.Background(), cmd, []string{"b"})
	require.NoError(t, err)
	assert.Equal(t, "b", cmd.Mode)
}

func TestArgEnum_InvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &argEnumCmd{}
	err := Execute(context.Background(), cmd, []string{"x"})
	require.ErrorIs(t, err, ErrInvalidArgValue)
	assert.Contains(t, err.Error(), "must be one of [a,b,c]")
}

type argDefaultCmd struct {
	Mode string `arg:"mode" default:"x" required:"false" help:"Mode"`
}

func (c *argDefaultCmd) Run(_ context.Context) error { return nil }

func TestArgDefault_NoPositional(t *testing.T) {
	t.Parallel()

	cmd := &argDefaultCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "x", cmd.Mode)
}

func TestArgDefault_PositionalWins(t *testing.T) {
	t.Parallel()

	cmd := &argDefaultCmd{}
	err := Execute(context.Background(), cmd, []string{"y"})
	require.NoError(t, err)
	assert.Equal(t, "y", cmd.Mode)
}

type argEnvCmd struct {
	Target string `arg:"target" env:"DEPLOY_TARGET" required:"false" help:"Deploy target"`
}

func (c *argEnvCmd) Run(_ context.Context) error { return nil }

func TestArgEnv_EnvUsed(t *testing.T) {
	t.Setenv("DEPLOY_TARGET", "from-env")

	cmd := &argEnvCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-env", cmd.Target)
}

func TestArgEnv_PositionalWins(t *testing.T) {
	t.Setenv("DEPLOY_TARGET", "from-env")

	cmd := &argEnvCmd{}
	err := Execute(context.Background(), cmd, []string{"from-arg"})
	require.NoError(t, err)
	assert.Equal(t, "from-arg", cmd.Target)
}

type argPriorityCmd struct {
	Mode string `arg:"mode" env:"TEST_ARG_MODE" default:"def" required:"false" help:"Mode"`
}

func (c *argPriorityCmd) Run(_ context.Context) error { return nil }

func TestArgPriority_PositionalOverEnvOverDefault(t *testing.T) {
	t.Setenv("TEST_ARG_MODE", "from-env")

	cmd := &argPriorityCmd{}
	err := Execute(context.Background(), cmd, []string{"from-arg"})
	require.NoError(t, err)
	assert.Equal(t, "from-arg", cmd.Mode)
}

func TestArgPriority_EnvOverDefault(t *testing.T) {
	t.Setenv("TEST_ARG_MODE", "from-env")

	cmd := &argPriorityCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "from-env", cmd.Mode)
}

func TestArgPriority_DefaultUsed(t *testing.T) {
	t.Parallel()

	cmd := &argPriorityCmd{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, "def", cmd.Mode)
}

type argHelpCmd struct {
	Env    string `arg:"env" enum:"prod,staging,dev" default:"dev" required:"false" help:"Target environment"`
	Target string `arg:"target" env:"DEPLOY_TARGET" required:"false" help:"Deploy target"`
}

func (c *argHelpCmd) Run(_ context.Context) error { return nil }
func (c *argHelpCmd) Name() string                { return "app" }

func TestArgHelp_ShowsEnumDefaultEnv(t *testing.T) {
	t.Parallel()

	cmd := &argHelpCmd{}
	flags := ScanFlags(cmd)
	text := defaultRenderHelp(cmd, []Commander{cmd}, flags, nil, false)
	assert.Contains(t, text, "[prod|staging|dev]")
	assert.Contains(t, text, "(default: dev)")
	assert.Contains(t, text, "(env: DEPLOY_TARGET)")
}

func TestScanArgs_EnhancedFields(t *testing.T) {
	t.Parallel()

	cmd := &argHelpCmd{}
	defs := ScanArgs(cmd)
	require.Len(t, defs, 2)
	assert.Equal(t, "prod,staging,dev", defs[0].Enum)
	assert.Equal(t, "dev", defs[0].Default)
	assert.Equal(t, "DEPLOY_TARGET", defs[1].Env)
}

// --- sep tag ---

type sepCmd struct {
	Tags []string `flag:"tag" sep:"," help:"Tags"`
}

func (c *sepCmd) Run(_ context.Context) error { return nil }
func (c *sepCmd) Name() string                { return "app" }

func TestSep_CommaSeparated(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	err := Execute(context.Background(), cmd, []string{"--tag", "a,b,c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cmd.Tags)
}

func TestSep_Repeated(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	err := Execute(context.Background(), cmd, []string{"--tag", "a", "--tag", "b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, cmd.Tags)
}

func TestSep_Mixed(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	err := Execute(context.Background(), cmd, []string{"--tag", "a,b", "--tag", "c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cmd.Tags)
}

func TestSep_Env(t *testing.T) {
	t.Setenv("TAG", "a,b,c")

	cmd := &struct {
		Tags []string `flag:"tag" sep:"," env:"TAG"`
		internalBareCmd
	}{}
	err := Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cmd.Tags)
}

func TestSep_Config(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	resolver := func(key ConfigKey) (string, bool) {
		if key.Name == "tag" {
			return "x,y", true
		}
		return "", false
	}
	err := Execute(context.Background(), cmd, nil, WithConfigResolver(resolver))
	require.NoError(t, err)
	assert.Equal(t, []string{"x", "y"}, cmd.Tags)
}

func TestSep_TrimWhitespace(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	err := Execute(context.Background(), cmd, []string{"--tag", "a, b, c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cmd.Tags)
}

type sepPipeCmd struct {
	Items []string `flag:"item" sep:"|" help:"Items"`
}

func (c *sepPipeCmd) Run(_ context.Context) error { return nil }

func TestSep_PipeSeparator(t *testing.T) {
	t.Parallel()

	cmd := &sepPipeCmd{}
	err := Execute(context.Background(), cmd, []string{"--item", "a|b|c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cmd.Items)
}

func TestSep_InHelpOutput(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	flags := ScanFlags(cmd)
	text := defaultRenderHelp(cmd, []Commander{cmd}, flags, nil, false)
	assert.Contains(t, text, `(separator: ",")`)
}

func TestScanFlags_Sep(t *testing.T) {
	t.Parallel()

	cmd := &sepCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, ",", defs[0].Sep)
}

// --- alt tag ---

type altCmd struct {
	Format string `flag:"format" alt:"output,out" short:"f" help:"Output format"`
}

func (c *altCmd) Run(_ context.Context) error { return nil }

func TestAlt_PrimaryFlag(t *testing.T) {
	t.Parallel()

	cmd := &altCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--format", "json"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Format)
}

func TestAlt_AltFlag(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
	}{
		"output": {args: []string{"--output", "json"}},
		"out":    {args: []string{"--out", "json"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &altCmd{}
			_, _, err := defaultParseFlags(cmd, tt.args, defaults())
			require.NoError(t, err)
			assert.Equal(t, "json", cmd.Format)
		})
	}
}

func TestAlt_ShortFlag(t *testing.T) {
	t.Parallel()

	cmd := &altCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"-f", "json"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Format)
}

func TestScanFlags_Alt(t *testing.T) {
	t.Parallel()

	cmd := &altCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, []string{"output", "out"}, defs[0].Alt)
	assert.Equal(t, "f", defs[0].Short)
}

func TestAlt_InHelpOutput(t *testing.T) {
	t.Parallel()

	cmd := &altCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--format")
	assert.Contains(t, text, "--output")
	assert.Contains(t, text, "--out")
}

func TestBuildFlagIndex_Alt(t *testing.T) {
	t.Parallel()

	cmd := &altCmd{}
	fi := buildFlagIndex(cmd)

	assert.True(t, fi.has("--format"))
	assert.True(t, fi.has("--output"))
	assert.True(t, fi.has("--out"))
	assert.True(t, fi.has("-f"))
}

type altEnumCmd struct {
	Format string `flag:"format" alt:"output" enum:"json,yaml"`
}

func (c *altEnumCmd) Run(_ context.Context) error { return nil }

func TestAlt_WithEnum(t *testing.T) {
	t.Parallel()

	cmd := &altEnumCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--output", "json"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Format)
}

// --- embedded struct (anonymous promotion) ---

type embeddedOutputFlags struct {
	Format string `flag:"format" enum:"json,table" default:"table" help:"Output format"`
}

type embeddedListCmd struct {
	embeddedOutputFlags
	Limit int `flag:"limit" default:"50" help:"Max results"`
}

func (c *embeddedListCmd) Run(_ context.Context) error { return nil }

func TestEmbedded_Promoted(t *testing.T) {
	t.Parallel()

	cmd := &embeddedListCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--format", "json", "--limit", "10"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Format)
	assert.Equal(t, 10, cmd.Limit)
}

func TestEmbedded_Default(t *testing.T) {
	t.Parallel()

	cmd := &embeddedListCmd{}
	_, _, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, "table", cmd.Format)
	assert.Equal(t, 50, cmd.Limit)
}

func TestScanFlags_Embedded(t *testing.T) {
	t.Parallel()

	cmd := &embeddedListCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 2)

	names := make([]string, len(defs))
	for i := range defs {
		names[i] = defs[i].Name
	}
	assert.Contains(t, names, "format")
	assert.Contains(t, names, "limit")
}

func TestEmbedded_InHelpOutput(t *testing.T) {
	t.Parallel()

	cmd := &embeddedListCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--format")
	assert.Contains(t, text, "--limit")
}

func TestBuildFlagIndex_Embedded(t *testing.T) {
	t.Parallel()

	cmd := &embeddedListCmd{}
	fi := buildFlagIndex(cmd)
	assert.True(t, fi.has("--format"))
	assert.True(t, fi.has("--limit"))
}

// --- embedded struct arg promotion ---

type embeddedArgFlags struct {
	Target string `arg:"target" help:"Deploy target"`
}

type embeddedDeployCmd struct {
	embeddedArgFlags
	Force bool `flag:"force" help:"Force deploy"`
}

func (c *embeddedDeployCmd) Run(_ context.Context) error { return nil }

func TestEmbedded_ArgPromotion(t *testing.T) {
	t.Parallel()

	cmd := &embeddedDeployCmd{}
	defs := ScanArgs(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, "target", defs[0].Name)
}

// --- prefix tag ---

type prefixDBFlags struct {
	Host string `flag:"host" default:"localhost" help:"Database host"`
	Port int    `flag:"port" default:"5432" help:"Database port"`
}

type prefixServeCmd struct {
	DB   prefixDBFlags `prefix:"db-"`
	Port int           `flag:"port" default:"8080" help:"Listen port"`
}

func (c *prefixServeCmd) Run(_ context.Context) error { return nil }

func TestPrefix_Flags(t *testing.T) {
	t.Parallel()

	cmd := &prefixServeCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--db-host", "remotehost", "--db-port", "3306", "--port", "9090"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "remotehost", cmd.DB.Host)
	assert.Equal(t, 3306, cmd.DB.Port)
	assert.Equal(t, 9090, cmd.Port)
}

func TestPrefix_Default(t *testing.T) {
	t.Parallel()

	cmd := &prefixServeCmd{}
	_, _, err := defaultParseFlags(cmd, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, "localhost", cmd.DB.Host)
	assert.Equal(t, 5432, cmd.DB.Port)
	assert.Equal(t, 8080, cmd.Port)
}

func TestScanFlags_Prefix(t *testing.T) {
	t.Parallel()

	cmd := &prefixServeCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 3)

	names := make([]string, len(defs))
	for i := range defs {
		names[i] = defs[i].Name
	}
	assert.Contains(t, names, "db-host")
	assert.Contains(t, names, "db-port")
	assert.Contains(t, names, "port")
}

func TestPrefix_InHelpOutput(t *testing.T) {
	t.Parallel()

	cmd := &prefixServeCmd{}
	chain := []Commander{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags, nil, false)
	assert.Contains(t, text, "--db-host")
	assert.Contains(t, text, "--db-port")
	assert.Contains(t, text, "--port")
}

func TestBuildFlagIndex_Prefix(t *testing.T) {
	t.Parallel()

	cmd := &prefixServeCmd{}
	fi := buildFlagIndex(cmd)
	assert.True(t, fi.has("--db-host"))
	assert.True(t, fi.has("--db-port"))
	assert.True(t, fi.has("--port"))
}

// --- nested prefix ---

type prefixInnerFlags struct {
	Name string `flag:"name" default:"inner" help:"Inner name"`
}

type prefixOuterFlags struct {
	Inner prefixInnerFlags `prefix:"b-"`
}

type prefixNestedCmd struct {
	Outer prefixOuterFlags `prefix:"a-"`
}

func (c *prefixNestedCmd) Run(_ context.Context) error { return nil }

func TestPrefix_Nested(t *testing.T) {
	t.Parallel()

	cmd := &prefixNestedCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--a-b-name", "deep"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "deep", cmd.Outer.Inner.Name)
}

func TestScanFlags_NestedPrefix(t *testing.T) {
	t.Parallel()

	cmd := &prefixNestedCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, "a-b-name", defs[0].Name)
}

// --- embedded + prefix combined ---

type embeddedPrefixCmd struct {
	embeddedOutputFlags
	DB prefixDBFlags `prefix:"db-"`
}

func (c *embeddedPrefixCmd) Run(_ context.Context) error { return nil }

func TestEmbeddedAndPrefix_Combined(t *testing.T) {
	t.Parallel()

	cmd := &embeddedPrefixCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--format", "json", "--db-host", "remote"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Format)
	assert.Equal(t, "remote", cmd.DB.Host)
}

// --- outer field shadows embedded ---

type embeddedShadowBase struct {
	Format string `flag:"format" default:"base"`
}

type embeddedShadowCmd struct {
	embeddedShadowBase
	Format string `flag:"format" default:"outer"`
}

func (c *embeddedShadowCmd) Run(_ context.Context) error { return nil }

func TestEmbedded_OuterShadows(t *testing.T) {
	t.Parallel()

	cmd := &embeddedShadowCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--format", "custom"}, defaults())
	require.NoError(t, err)
	// Outer field should win (shallower depth).
	assert.Equal(t, "custom", cmd.Format)
}

// --- prefix alt names ---

type prefixAltFlags struct {
	Format string `flag:"format" alt:"output" help:"Output format"`
}

type prefixAltCmd struct {
	Out prefixAltFlags `prefix:"out-"`
}

func (c *prefixAltCmd) Run(_ context.Context) error { return nil }

func TestPrefix_AltNames(t *testing.T) {
	t.Parallel()

	cmd := &prefixAltCmd{}
	_, _, err := defaultParseFlags(cmd, []string{"--out-output", "json"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "json", cmd.Out.Format)
}

func TestPrefix_AltInScanFlags(t *testing.T) {
	t.Parallel()

	cmd := &prefixAltCmd{}
	defs := ScanFlags(cmd)
	require.Len(t, defs, 1)
	assert.Equal(t, "out-format", defs[0].Name)
	assert.Equal(t, []string{"out-output"}, defs[0].Alt)
}

// --- flagFieldPath ---

type flagFieldPrefixInner struct {
	Host string `flag:"host" help:"Database host"`
}

type flagFieldPrefixCmd struct {
	DB flagFieldPrefixInner `prefix:"db-"`
}

func (c *flagFieldPrefixCmd) Run(_ context.Context) error { return nil }

func TestFlagFieldPath_PrefixStruct(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[flagFieldPrefixCmd]()
	path := flagFieldPath(typ, "db-host", nil, "")
	require.NotNil(t, path)
	// Should resolve to DB.Host (indices 0, 0).
	assert.Equal(t, []int{0, 0}, path)
}

func TestFlagFieldPath_NoMatch(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[flagFieldPrefixCmd]()
	path := flagFieldPath(typ, "nonexistent", nil, "")
	assert.Nil(t, path)
}

type flagFieldEmbeddedBase struct {
	Format string `flag:"format" help:"Format"`
}

type flagFieldEmbeddedCmd struct {
	flagFieldEmbeddedBase
}

func (c *flagFieldEmbeddedCmd) Run(_ context.Context) error { return nil }

func TestFlagFieldPath_EmbeddedStruct(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[flagFieldEmbeddedCmd]()
	path := flagFieldPath(typ, "format", nil, "")
	require.NotNil(t, path)
	// Should resolve to embedded.Format (indices 0, 0).
	assert.Equal(t, []int{0, 0}, path)
}

type flagFieldAutoNameCmd struct {
	OutputDir string `flag:"" help:"Output directory"`
}

func (c *flagFieldAutoNameCmd) Run(_ context.Context) error { return nil }

func TestFlagFieldPath_AutoKebabName(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[flagFieldAutoNameCmd]()
	path := flagFieldPath(typ, "output-dir", nil, "")
	require.NotNil(t, path)
	assert.Equal(t, []int{0}, path)
}

type flagFieldNoFlagCmd struct {
	NotAFlag string
	Port     int `flag:"port"`
}

func (c *flagFieldNoFlagCmd) Run(_ context.Context) error { return nil }

func TestFlagFieldPath_SkipNonFlagField(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[flagFieldNoFlagCmd]()
	path := flagFieldPath(typ, "port", nil, "")
	require.NotNil(t, path)
	// NotAFlag at index 0 has no flag tag → skip; Port at index 1 matches.
	assert.Equal(t, []int{1}, path)
}

// --- readPrompt ---

func TestReadPrompt_EOF(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(""))
	result, err := readPrompt(FlagDef{Name: "token", Help: "API token"}, &buf, scanner)
	require.NoError(t, err)
	assert.Equal(t, "", result)
	assert.Contains(t, buf.String(), "API token: ")
}

func TestReadPrompt_NoHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader("myval\n"))
	result, err := readPrompt(FlagDef{Name: "token"}, &buf, scanner)
	require.NoError(t, err)
	assert.Equal(t, "myval", result)
	assert.Contains(t, buf.String(), "token: ")
}

type errReader struct{ err error }

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadPrompt_ScannerError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	scanner := bufio.NewScanner(&errReader{err: fmt.Errorf("read failed")})
	_, err := readPrompt(FlagDef{Name: "token", Help: "API token"}, &buf, scanner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read failed")
}

// --- promptForFlags ---

func TestPromptForFlags_NonStruct(t *testing.T) {
	t.Parallel()

	cmd := RunFunc(func(_ context.Context) error { return nil })
	opts := defaults()
	opts.interactive = true
	opts.isTerminal = func() bool { return true }
	opts.stdin = strings.NewReader("")

	provided, err := promptForFlags(cmd, nil, opts)
	require.NoError(t, err)
	assert.Nil(t, provided)
}

type promptSetFieldErrorCmd struct {
	Count int `flag:"count" required:"true" help:"A count"`
}

func (c *promptSetFieldErrorCmd) Run(_ context.Context) error { return nil }

func TestPromptForFlags_SetFieldError(t *testing.T) {
	t.Parallel()

	cmd := &promptSetFieldErrorCmd{}
	opts := defaults()
	opts.interactive = true
	opts.isTerminal = func() bool { return true }
	opts.stdin = strings.NewReader("not-a-number\n")
	opts.stderr = &bytes.Buffer{}

	_, err := promptForFlags(cmd, nil, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- stripProgramName ---

func TestStripProgramName_EmptyArgs(t *testing.T) {
	t.Parallel()
	result := stripProgramName(nil, "myapp")
	assert.Empty(t, result)
}

func TestStripProgramName_RelativeNotExecutable(t *testing.T) {
	t.Parallel()
	// Relative path matching cmdName but file doesn't exist — don't strip.
	result := stripProgramName([]string{"../bin/myapp", "serve"}, "myapp")
	assert.Equal(t, []string{"../bin/myapp", "serve"}, result)
}

func TestStripProgramName_BackslashPath(t *testing.T) {
	t.Parallel()
	result := stripProgramName([]string{`C:\bin\myapp.exe`, "serve"}, "myapp")
	if runtime.GOOS == "windows" {
		// On Windows, the backslash is recognized as a path separator and
		// filepath.IsAbs returns true for C:\..., so it's stripped.
		assert.Equal(t, []string{"serve"}, result)
	} else {
		// On non-Windows, the string contains a backslash but filepath.IsAbs
		// returns false and no executable exists, so it's not stripped.
		assert.Equal(t, []string{`C:\bin\myapp.exe`, "serve"}, result)
	}
}

func TestStripProgramName_NonPath(t *testing.T) {
	t.Parallel()
	result := stripProgramName([]string{"serve", "--port", "8080"}, "myapp")
	assert.Equal(t, []string{"serve", "--port", "8080"}, result)
}

func TestStripProgramName_AbsoluteNonMatch(t *testing.T) {
	t.Parallel()
	// Absolute path that doesn't match cmdName — don't strip.
	result := stripProgramName([]string{"/usr/bin/other", "serve"}, "myapp")
	assert.Equal(t, []string{"/usr/bin/other", "serve"}, result)
}

func TestStripProgramName_AbsoluteMatch(t *testing.T) {
	t.Parallel()
	// Absolute path that matches cmdName — always strip.
	result := stripProgramName([]string{"/usr/local/bin/myapp", "serve"}, "myapp")
	assert.Equal(t, []string{"serve"}, result)
}

func TestStripProgramName_AbsoluteMatchWithExtension(t *testing.T) {
	t.Parallel()
	// Absolute path with extension that matches cmdName — strip.
	result := stripProgramName([]string{"/usr/local/bin/myapp.exe", "serve"}, "myapp")
	assert.Equal(t, []string{"serve"}, result)
}

// --- populateArgs env/default/enum ---

type envArgCmd struct {
	File string `arg:"file" env:"TEST_ARG_FILE"`
}

func (c *envArgCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_EnvFallback(t *testing.T) {
	t.Setenv("TEST_ARG_FILE", "/tmp/test.txt")

	cmd := &envArgCmd{}
	remaining, err := populateArgs(cmd, nil, "")
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "/tmp/test.txt", cmd.File)
}

type defaultArgCmd struct {
	Mode string `arg:"mode" default:"fast" required:"false"`
}

func (c *defaultArgCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_DefaultFallback(t *testing.T) {
	t.Parallel()

	cmd := &defaultArgCmd{}
	remaining, err := populateArgs(cmd, nil, "")
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Equal(t, "fast", cmd.Mode)
}

type enumArgCmd struct {
	Env string `arg:"env" env:"TEST_ENUM_ARG" enum:"dev,staging,prod"`
}

func (c *enumArgCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_EnumValidation(t *testing.T) {
	t.Setenv("TEST_ENUM_ARG", "invalid")

	cmd := &enumArgCmd{}
	_, err := populateArgs(cmd, nil, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidArgValue)
}

// --- cli.Args as first-class type ---

type cliArgsOnlyCmd struct {
	Args Args
}

func (c *cliArgsOnlyCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_CliArgsOnly(t *testing.T) {
	t.Parallel()

	cmd := &cliArgsOnlyCmd{}
	remaining, err := populateArgs(cmd, []string{"foo", "bar", "baz"}, "")
	require.NoError(t, err)
	assert.Nil(t, remaining) // all consumed
	assert.Equal(t, Args{"foo", "bar", "baz"}, cmd.Args)
}

type cliArgsWithNamedCmd struct {
	Src  string `arg:"src"`
	Dst  string `arg:"dst"`
	Args Args
}

func (c *cliArgsWithNamedCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_CliArgsWithNamed(t *testing.T) {
	t.Parallel()

	cmd := &cliArgsWithNamedCmd{}
	remaining, err := populateArgs(cmd, []string{"a.txt", "b.txt", "c.txt", "d.txt"}, "")
	require.NoError(t, err)
	assert.Nil(t, remaining)
	assert.Equal(t, "a.txt", cmd.Src)
	assert.Equal(t, "b.txt", cmd.Dst)
	assert.Equal(t, Args{"c.txt", "d.txt"}, cmd.Args)
}

func TestPopulateArgs_CliArgsEmpty(t *testing.T) {
	t.Parallel()

	cmd := &cliArgsWithNamedCmd{}
	remaining, err := populateArgs(cmd, []string{"a.txt", "b.txt"}, "")
	require.NoError(t, err)
	assert.Nil(t, remaining)
	assert.Equal(t, "a.txt", cmd.Src)
	assert.Equal(t, "b.txt", cmd.Dst)
	assert.Empty(t, cmd.Args)
}

type multipleCliArgsCmd struct {
	Args1 Args
	Args2 Args
}

func (c *multipleCliArgsCmd) Run(_ context.Context) error { return nil }

func TestPopulateArgs_MultipleCliArgsError(t *testing.T) {
	t.Parallel()

	cmd := &multipleCliArgsCmd{}
	_, err := populateArgs(cmd, []string{"foo"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple cli.Args fields")
}

// --- Args convenience methods ---

func TestArgs_Len(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 3, Args{"a", "b", "c"}.Len())
	assert.Equal(t, 0, Args{}.Len())
}

func TestArgs_Empty(t *testing.T) {
	t.Parallel()
	assert.True(t, Args{}.Empty())
	assert.False(t, Args{"a"}.Empty())
}

func TestArgs_First(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a", Args{"a", "b", "c"}.First())
	assert.Equal(t, "", Args{}.First())
}

func TestArgs_Last(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "c", Args{"a", "b", "c"}.Last())
	assert.Equal(t, "", Args{}.Last())
}

func TestArgs_Get(t *testing.T) {
	t.Parallel()
	args := Args{"a", "b", "c"}
	assert.Equal(t, "a", args.Get(0))
	assert.Equal(t, "b", args.Get(1))
	assert.Equal(t, "c", args.Get(2))
	assert.Equal(t, "", args.Get(3))
	assert.Equal(t, "", args.Get(-1))
}

func TestArgs_Contains(t *testing.T) {
	t.Parallel()
	args := Args{"a", "b", "c"}
	assert.True(t, args.Contains("b"))
	assert.False(t, args.Contains("d"))
}

func TestArgs_Index(t *testing.T) {
	t.Parallel()
	args := Args{"a", "b", "c"}
	assert.Equal(t, 1, args.Index("b"))
	assert.Equal(t, -1, args.Index("d"))
}

func TestArgs_Tail(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Args{"b", "c"}, Args{"a", "b", "c"}.Tail())
	assert.Nil(t, Args{"a"}.Tail())
	assert.Nil(t, Args{}.Tail())
}

// --- flag normalization (internal) ---

type normInternalCmd struct {
	MyFlag string `flag:"my-flag" help:"A flag"`
}

func (c *normInternalCmd) Run(_ context.Context) error { return nil }

func TestFlagNormalization_InternalParse(t *testing.T) {
	t.Parallel()

	cmd := &normInternalCmd{}
	opts := defaults()
	opts.flagNormalizer = func(s string) string {
		return strings.ReplaceAll(s, "_", "-")
	}
	// defaultParseFlags applies the normalizer in its lookup function,
	// so --my_flag normalizes to --my-flag and matches the registered flag.
	_, _, err := defaultParseFlags(cmd, []string{"--my_flag", "hello"}, opts)
	require.NoError(t, err)
	assert.Equal(t, "hello", cmd.MyFlag)
}

// --- Multiple env vars ---

type multiEnvCmd struct {
	Host string `flag:"host" env:"APP_HOST,SERVICE_HOST"`
}

func (c *multiEnvCmd) Run(_ context.Context) error { return nil }

// --- time.Time flag type ---

type timeFlagCmd struct {
	Since time.Time `flag:"since" help:"Start time"`
	Until time.Time `flag:"until" default:"2024-06-15T12:00:00Z"`
}

func (c *timeFlagCmd) Run(_ context.Context) error { return nil }

func TestTimeFlag_RFC3339(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--since", "2024-01-15T10:30:00Z"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), c.Since)
}

func TestTimeFlag_DateOnly(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--since", "2024-01-15"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), c.Since)
}

func TestTimeFlag_DateTime(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--since", "2024-01-15 10:30:00"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), c.Since)
}

func TestTimeFlag_Default(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	_, _, err := defaultParseFlags(c, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC), c.Until)
}

func TestTimeFlag_Invalid(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--since", "not-a-time"}, defaults())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot parse")
}

type timeSliceCmd struct {
	Dates []time.Time `flag:"date"`
}

func (c *timeSliceCmd) Run(_ context.Context) error { return nil }

func TestTimeFlag_Slice(t *testing.T) {
	t.Parallel()

	c := &timeSliceCmd{}
	_, _, err := defaultParseFlags(c, []string{"--date", "2024-01-01", "--date", "2024-06-15"}, defaults())
	require.NoError(t, err)
	require.Len(t, c.Dates, 2)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), c.Dates[0])
	assert.Equal(t, time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), c.Dates[1])
}

func TestTimeFlag_Env(t *testing.T) {
	t.Setenv("SINCE", "2024-03-20T08:00:00Z")

	type cmd struct {
		Since time.Time `flag:"since" env:"SINCE"`
	}
	c := &cmd{}
	v := reflect.ValueOf(c).Elem()
	fields := buildFieldMap(v.Type())
	require.NoError(t, applyEnv(v, fields, ""))
	assert.Equal(t, time.Date(2024, 3, 20, 8, 0, 0, 0, time.UTC), c.Since)
}

func TestTimeFlag_TypeName(t *testing.T) {
	t.Parallel()

	c := &timeFlagCmd{}
	defs := ScanFlags(c)
	for _, d := range defs {
		assert.Equal(t, "time", d.TypeName)
	}
}

func TestApplyEnv_MultipleEnvVars(t *testing.T) {
	t.Run("first env var takes priority", func(t *testing.T) {
		t.Setenv("APP_HOST", "first.example.com")
		t.Setenv("SERVICE_HOST", "second.example.com")
		c := &multiEnvCmd{}
		_, _, err := defaultParseFlags(c, nil, defaults())
		require.NoError(t, err)
		assert.Equal(t, "first.example.com", c.Host)
	})

	t.Run("falls back to second env var", func(t *testing.T) {
		t.Setenv("SERVICE_HOST", "fallback.example.com")
		c := &multiEnvCmd{}
		_, _, err := defaultParseFlags(c, nil, defaults())
		require.NoError(t, err)
		assert.Equal(t, "fallback.example.com", c.Host)
	})

	t.Run("no env vars set", func(t *testing.T) {
		c := &multiEnvCmd{}
		_, _, err := defaultParseFlags(c, nil, defaults())
		require.NoError(t, err)
		assert.Equal(t, "", c.Host)
	})
}

type multiEnvPrefixCmd struct {
	Host string `flag:"host" env:"HOST_A,HOST_B"`
}

func (c *multiEnvPrefixCmd) Run(_ context.Context) error { return nil }

func TestApplyEnv_MultipleWithPrefix(t *testing.T) {
	t.Setenv("PFX_HOST_A", "prefixed.example.com")

	c := &multiEnvPrefixCmd{}
	opts := defaults()
	opts.envVarPrefix = "PFX_"
	_, _, err := defaultParseFlags(c, nil, opts)
	require.NoError(t, err)
	assert.Equal(t, "prefixed.example.com", c.Host)
}

type multiEnvSpaceCmd struct {
	Val string `flag:"val" env:"A_VAR, B_VAR"`
}

func (c *multiEnvSpaceCmd) Run(_ context.Context) error { return nil }

func TestApplyEnv_MultipleWhitespace(t *testing.T) {
	t.Setenv("B_VAR", "from-b")

	c := &multiEnvSpaceCmd{}
	_, _, err := defaultParseFlags(c, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, "from-b", c.Val)
}

// --- uint / uint64 flag types ---

type uintFlagCmd struct {
	Port    uint   `flag:"port" default:"8080"`
	MaxConn uint64 `flag:"max-conn" default:"1000"`
}

func (c *uintFlagCmd) Run(_ context.Context) error { return nil }

func TestUintFlags_ParseAndDefault(t *testing.T) {
	t.Parallel()

	c := &uintFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--port", "3000", "--max-conn", "5000"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, uint(3000), c.Port)
	assert.Equal(t, uint64(5000), c.MaxConn)
}

func TestUintFlags_DefaultValues(t *testing.T) {
	t.Parallel()

	c := &uintFlagCmd{}
	_, _, err := defaultParseFlags(c, nil, defaults())
	require.NoError(t, err)
	assert.Equal(t, uint(8080), c.Port)
	assert.Equal(t, uint64(1000), c.MaxConn)
}

func TestUintFlags_EnvVars(t *testing.T) {
	t.Setenv("TEST_PORT", "9090")

	type cmd struct {
		Port uint `flag:"port" env:"TEST_PORT"`
	}
	c := &cmd{}
	// cmd doesn't implement Commander, use the struct directly through reflection
	v := reflect.ValueOf(c).Elem()
	fields := buildFieldMap(v.Type())
	require.NoError(t, applyEnv(v, fields, ""))
	assert.Equal(t, uint(9090), c.Port)
}

type uintSliceCmd struct {
	Ports []uint `flag:"port"`
}

func (c *uintSliceCmd) Run(_ context.Context) error { return nil }

func TestUintFlags_Slice(t *testing.T) {
	t.Parallel()

	c := &uintSliceCmd{}
	_, _, err := defaultParseFlags(c, []string{"--port", "80", "--port", "443"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, []uint{80, 443}, c.Ports)
}

type uintCounterCmd struct {
	Verbosity uint `flag:"verbose" short:"v" counter:"true"`
}

func (c *uintCounterCmd) Run(_ context.Context) error { return nil }

func TestUintFlags_Counter(t *testing.T) {
	t.Parallel()

	c := &uintCounterCmd{}
	_, _, err := defaultParseFlags(c, []string{"-v", "-v", "-v"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, uint(3), c.Verbosity)
}

func TestUintFlags_InvalidValue(t *testing.T) {
	t.Parallel()

	c := &uintFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--port", "abc"}, defaults())
	require.Error(t, err)
}

func TestUintFlags_NegativeValue(t *testing.T) {
	t.Parallel()

	c := &uintFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--port", "-1"}, defaults())
	require.Error(t, err)
}

// --- url.URL and net.IP type support ---

type urlFlagCmd struct {
	Endpoint *url.URL `flag:"endpoint" help:"API endpoint URL"`
}

func (c *urlFlagCmd) Run(_ context.Context) error { return nil }

func TestURLFlag_Parse(t *testing.T) {
	t.Parallel()

	c := &urlFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--endpoint", "https://api.example.com/v1"}, defaults())
	require.NoError(t, err)
	require.NotNil(t, c.Endpoint)
	assert.Equal(t, "https", c.Endpoint.Scheme)
	assert.Equal(t, "api.example.com", c.Endpoint.Host)
	assert.Equal(t, "/v1", c.Endpoint.Path)
}

func TestURLFlag_Invalid(t *testing.T) {
	t.Parallel()

	c := &urlFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--endpoint", "://invalid"}, defaults())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid url")
}

func TestURLFlag_TypeName(t *testing.T) {
	t.Parallel()

	cmd := &urlFlagCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "url", flags[0].TypeName)
}

type urlSliceCmd struct {
	URLs []*url.URL `flag:"url" help:"URLs to fetch"`
}

func (c *urlSliceCmd) Run(_ context.Context) error { return nil }

func TestURLFlag_Slice(t *testing.T) {
	t.Parallel()

	c := &urlSliceCmd{}
	_, _, err := defaultParseFlags(c, []string{"--url", "https://a.com", "--url", "https://b.com"}, defaults())
	require.NoError(t, err)
	require.Len(t, c.URLs, 2)
	assert.Equal(t, "a.com", c.URLs[0].Host)
	assert.Equal(t, "b.com", c.URLs[1].Host)
}

type ipFlagCmd struct {
	Host net.IP `flag:"host" help:"Host IP address"`
}

func (c *ipFlagCmd) Run(_ context.Context) error { return nil }

func TestIPFlag_Parse(t *testing.T) {
	t.Parallel()

	c := &ipFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--host", "192.168.1.1"}, defaults())
	require.NoError(t, err)
	require.NotNil(t, c.Host)
	assert.Equal(t, "192.168.1.1", c.Host.String())
}

func TestIPFlag_ParseIPv6(t *testing.T) {
	t.Parallel()

	c := &ipFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--host", "::1"}, defaults())
	require.NoError(t, err)
	require.NotNil(t, c.Host)
	assert.Equal(t, "::1", c.Host.String())
}

func TestIPFlag_Invalid(t *testing.T) {
	t.Parallel()

	c := &ipFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--host", "not-an-ip"}, defaults())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ip address")
}

func TestIPFlag_TypeName(t *testing.T) {
	t.Parallel()

	cmd := &ipFlagCmd{}
	flags := ScanFlags(cmd)
	require.Len(t, flags, 1)
	assert.Equal(t, "ip", flags[0].TypeName)
}

type ipSliceCmd struct {
	Hosts []net.IP `flag:"host" help:"Host IP addresses"`
}

func (c *ipSliceCmd) Run(_ context.Context) error { return nil }

func TestIPFlag_Slice(t *testing.T) {
	t.Parallel()

	c := &ipSliceCmd{}
	_, _, err := defaultParseFlags(c, []string{"--host", "192.168.1.1", "--host", "10.0.0.1"}, defaults())
	require.NoError(t, err)
	require.Len(t, c.Hosts, 2)
	assert.Equal(t, "192.168.1.1", c.Hosts[0].String())
	assert.Equal(t, "10.0.0.1", c.Hosts[1].String())
}

// --- ConfigProvider with --config flag ---

// configFlagCmd demonstrates that ConfigProvider.ConfigResolver() is called
// AFTER CLI args are parsed, allowing --config to specify the config source.
type configFlagCmd struct {
	ConfigPath string `flag:"config" help:"Path to config file"`
	Port       int    `flag:"port" help:"Port to listen on"`
	Host       string `flag:"host" help:"Host to bind to"`
}

func (c *configFlagCmd) Run(_ context.Context) error { return nil }

func (c *configFlagCmd) ConfigResolver() ConfigResolver {
	// ConfigResolver is called after CLI parsing, so c.ConfigPath is available.
	if c.ConfigPath == "" {
		return nil
	}
	// Simulate loading config based on the --config flag value.
	configs := map[string]map[string]string{
		"prod.json": {"port": "443", "host": "0.0.0.0"},
		"dev.json":  {"port": "8080", "host": "localhost"},
	}
	if cfg, ok := configs[c.ConfigPath]; ok {
		return func(key ConfigKey) (string, bool) {
			v, found := cfg[key.Name]
			return v, found
		}
	}
	return nil
}

func TestConfigProvider_ConfigFlagAvailable(t *testing.T) {
	t.Parallel()

	// --config is parsed first, then ConfigResolver() uses it to load config.
	c := &configFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--config", "prod.json"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "prod.json", c.ConfigPath)
	assert.Equal(t, 443, c.Port)       // from config
	assert.Equal(t, "0.0.0.0", c.Host) // from config
}

func TestConfigProvider_CLIOverridesConfig(t *testing.T) {
	t.Parallel()

	// CLI args take priority over config values.
	c := &configFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{"--config", "prod.json", "--port", "9000"}, defaults())
	require.NoError(t, err)
	assert.Equal(t, 9000, c.Port)      // from CLI, overrides config
	assert.Equal(t, "0.0.0.0", c.Host) // from config (not overridden)
}

func TestConfigProvider_NoConfigFlag(t *testing.T) {
	t.Parallel()

	// Without --config, ConfigResolver returns nil, fields stay at zero.
	c := &configFlagCmd{}
	_, _, err := defaultParseFlags(c, []string{}, defaults())
	require.NoError(t, err)
	assert.Equal(t, "", c.ConfigPath)
	assert.Equal(t, 0, c.Port)
	assert.Equal(t, "", c.Host)
}

func TestWithSilenceErrors(t *testing.T) {
	t.Parallel()

	opts := defaults()
	assert.False(t, opts.silenceErrors)

	WithSilenceErrors(true)(opts)
	assert.True(t, opts.silenceErrors)

	WithSilenceErrors(false)(opts)
	assert.False(t, opts.silenceErrors)
}

func TestWithSilenceUsage(t *testing.T) {
	t.Parallel()

	opts := defaults()
	assert.False(t, opts.silenceUsage)

	WithSilenceUsage(true)(opts)
	assert.True(t, opts.silenceUsage)

	WithSilenceUsage(false)(opts)
	assert.False(t, opts.silenceUsage)
}
