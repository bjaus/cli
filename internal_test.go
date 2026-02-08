package cli

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveInfo / defaultName ---

type internalBareCmd struct{}

func (c *internalBareCmd) Run(_ context.Context, _ []string) error { return nil }

type internalFullCmd struct{}

func (c *internalFullCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalFullCmd) Name() string                            { return "serve" }
func (c *internalFullCmd) Description() string                     { return "Start the server" }
func (c *internalFullCmd) Aliases() []string                       { return []string{"s", "srv"} }
func (c *internalFullCmd) Hidden() bool                            { return true }

func (c *internalFullCmd) Examples() []Example {
	return []Example{
		{Description: "Start on port 8080", Command: "app serve --port 8080"},
	}
}

func TestResolveInfo(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd         Runner
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
		cmd      Runner
		wantName string
	}{
		"pointer to struct": {
			cmd:      &internalBareCmd{},
			wantName: "internalbarecmd",
		},
		"RunFunc": {
			cmd:      RunFunc(func(_ context.Context, _ []string) error { return nil }),
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

func (c *internalFlaggedCmd) Run(_ context.Context, _ []string) error { return nil }

type internalRequiredFlagCmd struct {
	Name string `flag:"name" required:"true" help:"Your name"`
}

func (c *internalRequiredFlagCmd) Run(_ context.Context, _ []string) error { return nil }

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

func (c *internalCustomFlagCmd) Run(_ context.Context, _ []string) error { return nil }

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
			remaining, err := defaultParseFlags(cmd, tt.args)
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
	_, err := defaultParseFlags(cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, 9999, cmd.Port)
}

func TestDefaultParseFlags_ExplicitOverridesEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	cmd := &internalFlaggedCmd{}
	_, err := defaultParseFlags(cmd, []string{"--port", "3000"})
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
			_, err := defaultParseFlags(cmd, tt.args)
			tt.assertErr(t, err)
		})
	}
}

func TestDefaultParseFlags_RequiredFlagErrorMessage(t *testing.T) {
	t.Parallel()

	cmd := &internalRequiredFlagCmd{}
	_, err := defaultParseFlags(cmd, nil)
	require.ErrorIs(t, err, ErrRequiredFlag)
	assert.Contains(t, err.Error(), "--name")
}

func TestDefaultParseFlags_CustomUnmarshaler(t *testing.T) {
	t.Parallel()

	cmd := &internalCustomFlagCmd{}
	_, err := defaultParseFlags(cmd, []string{"--tag", "foo"})
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
			_, err := defaultParseFlags(cmd, tt.args)
			require.Error(t, err)
		})
	}
}

func TestDefaultParseFlags_UnknownFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, err := defaultParseFlags(cmd, []string{"--unknown"})
	require.ErrorIs(t, err, ErrUnknownFlag)
}

func TestDefaultParseFlags_MissingValue(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, err := defaultParseFlags(cmd, []string{"--port"})
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

func (p *internalParentWithSubs) Run(_ context.Context, _ []string) error { return nil }

func (p *internalParentWithSubs) Subcommands() []Runner {
	return []Runner{
		&internalNamedCmd{n: "serve"},
		&internalNamedCmd{n: "status"},
		&internalNamedCmd{n: "deploy"},
	}
}

type internalNamedCmd struct {
	n string
}

func (c *internalNamedCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalNamedCmd) Name() string                            { return c.n }

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

func (c *internalFlagCmdForSuggest) Run(_ context.Context, _ []string) error { return nil }

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

func TestSuggestFlagName_NoFlags(t *testing.T) {
	t.Parallel()
	assert.Empty(t, suggestFlagName(&internalBareCmd{}, "--anything"))
}

// --- applyMiddleware ---

func TestApplyMiddleware(t *testing.T) {
	t.Parallel()

	var order []string

	base := RunFunc(func(_ context.Context, _ []string) error {
		order = append(order, "run")
		return nil
	})

	mw1 := func(next RunFunc) RunFunc {
		return func(ctx context.Context, args []string) error {
			order = append(order, "mw1-before")
			err := next(ctx, args)
			order = append(order, "mw1-after")
			return err
		}
	}

	mw2 := func(next RunFunc) RunFunc {
		return func(ctx context.Context, args []string) error {
			order = append(order, "mw2-before")
			err := next(ctx, args)
			order = append(order, "mw2-after")
			return err
		}
	}

	wrapped := applyMiddleware(base, []func(next RunFunc) RunFunc{mw1, mw2})
	err := wrapped(context.Background(), nil)
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
	base := RunFunc(func(_ context.Context, _ []string) error {
		called = true
		return nil
	})

	wrapped := applyMiddleware(base, nil)
	err := wrapped(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestApplyMiddleware_ErrorPropagation(t *testing.T) {
	t.Parallel()

	base := RunFunc(func(_ context.Context, _ []string) error {
		return Exit("fail", 1)
	})

	var afterCalled bool
	mw := func(next RunFunc) RunFunc {
		return func(ctx context.Context, args []string) error {
			err := next(ctx, args)
			afterCalled = true
			return err
		}
	}

	wrapped := applyMiddleware(base, []func(next RunFunc) RunFunc{mw})
	err := wrapped(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, afterCalled)
}

// --- defaultRenderHelp ---

type internalServeCmd struct {
	Port int    `flag:"port" short:"p" default:"8080" help:"Port"`
	Host string `flag:"host" default:"localhost" help:"Host"`
}

func (s *internalServeCmd) Run(_ context.Context, _ []string) error { return nil }
func (s *internalServeCmd) Name() string                            { return "serve" }
func (s *internalServeCmd) Description() string                     { return "Start the server" }

type internalRootCmd struct {
	serve *internalServeCmd
}

func (r *internalRootCmd) Run(_ context.Context, _ []string) error { return nil }
func (r *internalRootCmd) Name() string                            { return "app" }
func (r *internalRootCmd) Description() string                     { return "Test application" }
func (r *internalRootCmd) Subcommands() []Runner                   { return []Runner{r.serve} }

func TestDefaultRenderHelp_Basic(t *testing.T) {
	t.Parallel()

	cmd := &internalServeCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)

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
	chain := []Runner{root}
	flags := ScanFlags(root)

	text := defaultRenderHelp(root, chain, flags)

	assert.Contains(t, text, "Commands:")
	assert.Contains(t, text, "serve")
	assert.Contains(t, text, "[command]")
}

type internalHiddenSubCmd struct{}

func (c *internalHiddenSubCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalHiddenSubCmd) Name() string                            { return "secret" }
func (c *internalHiddenSubCmd) Hidden() bool                            { return true }

type internalParentWithHidden struct {
	child Runner
}

func (p *internalParentWithHidden) Run(_ context.Context, _ []string) error { return nil }
func (p *internalParentWithHidden) Name() string                            { return "app" }
func (p *internalParentWithHidden) Subcommands() []Runner                   { return []Runner{p.child} }

func TestDefaultRenderHelp_HiddenSubcommands(t *testing.T) {
	t.Parallel()

	hidden := &internalHiddenSubCmd{}
	parent := &internalParentWithHidden{child: hidden}
	chain := []Runner{parent}

	text := defaultRenderHelp(parent, chain, nil)
	assert.NotContains(t, text, "secret")
}

func TestDefaultRenderHelp_WithExamples(t *testing.T) {
	t.Parallel()

	cmd := &internalFullCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "Examples:")
	assert.Contains(t, text, "$ app serve --port 8080")
}

func TestDefaultRenderHelp_RequiredFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalRequiredFlagCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "(required)")
}

func TestCommandChainNames(t *testing.T) {
	t.Parallel()

	serve := &internalServeCmd{}
	root := &internalRootCmd{serve: serve}
	chain := []Runner{root, serve}

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
		cmd  Runner
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

	subs := []Runner{&internalNamedCmd{n: "serve"}, &internalNamedCmd{n: "status"}}
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

			flags, next, found := scanLevel(tt.args, fi, subs, false)
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

func (p *internalEmptyParent) Run(_ context.Context, _ []string) error { return nil }
func (p *internalEmptyParent) Name() string                            { return "empty" }
func (p *internalEmptyParent) Subcommands() []Runner                   { return nil }

func TestResolveCommand(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		root         Runner
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

			resolved := resolveCommand(tt.root, tt.args, defaults())
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

func (c *internalFlagParserCmd) Run(_ context.Context, _ []string) error { return nil }

func (c *internalFlagParserCmd) ParseFlags(_ Runner, args []string) ([]string, error) {
	c.parseCalled = true
	return args, nil
}

func TestParseFlags_CommandFlagParser(t *testing.T) {
	t.Parallel()

	cmd := &internalFlagParserCmd{}
	_, err := parseFlags(cmd, []string{"--foo"}, defaults())
	require.NoError(t, err)
	assert.True(t, cmd.parseCalled)
}

type internalGlobalParser struct {
	called bool
}

func (p *internalGlobalParser) ParseFlags(_ Runner, args []string) ([]string, error) {
	p.called = true
	return args, nil
}

func TestParseFlags_GlobalFlagParser(t *testing.T) {
	t.Parallel()

	parser := &internalGlobalParser{}
	opts := defaults()
	opts.flagParser = parser

	cmd := &internalBareCmd{}
	_, err := parseFlags(cmd, []string{"arg"}, opts)
	require.NoError(t, err)
	assert.True(t, parser.called)
}

// --- runAfterHooks ---

type internalAfterErrorCmd struct {
	errMsg string
}

func (c *internalAfterErrorCmd) Run(_ context.Context, _ []string) error { return nil }

func (c *internalAfterErrorCmd) After(_ context.Context) error {
	return fmt.Errorf("%s", c.errMsg)
}

func TestRunAfterHooks_Error(t *testing.T) {
	t.Parallel()

	hooks := []Runner{
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

	hooks := []Runner{&internalBareCmd{}}
	err := runAfterHooks(context.Background(), hooks)
	require.NoError(t, err)
}

// --- flagTypeName ---

func TestFlagTypeName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cmd      Runner
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

func (c *internalBoolValueCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_BoolEquals(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolValueCmd{}
	_, err := defaultParseFlags(cmd, []string{"--debug=true"})
	require.NoError(t, err)
	assert.True(t, cmd.Debug)
}

func TestDefaultParseFlags_BoolInvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolValueCmd{}
	_, err := defaultParseFlags(cmd, []string{"--debug=notbool"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

func TestDefaultParseFlags_NonStruct(t *testing.T) {
	t.Parallel()

	cmd := RunFunc(func(_ context.Context, _ []string) error { return nil })
	remaining, err := defaultParseFlags(cmd, []string{"arg1", "arg2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"arg1", "arg2"}, remaining)
}

type internalBadDefaultCmd struct {
	Port int `flag:"port" default:"not-a-number"`
}

func (c *internalBadDefaultCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_InvalidDefault(t *testing.T) {
	t.Parallel()

	cmd := &internalBadDefaultCmd{}
	_, err := defaultParseFlags(cmd, nil)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

type internalEnvCmd struct {
	Port int `flag:"port" env:"BAD_PORT"`
}

func (c *internalEnvCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_InvalidEnv(t *testing.T) {
	t.Setenv("BAD_PORT", "not-a-number")

	cmd := &internalEnvCmd{}
	_, err := defaultParseFlags(cmd, nil)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

func TestDefaultParseFlags_EqualsUnknown(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, err := defaultParseFlags(cmd, []string{"--unknown=value"})
	require.ErrorIs(t, err, ErrUnknownFlag)
}

func TestDefaultParseFlags_EqualsInvalidValue(t *testing.T) {
	t.Parallel()

	cmd := &internalFlaggedCmd{}
	_, err := defaultParseFlags(cmd, []string{"--port=abc"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- setFieldValue unsupported type ---

type internalUnsupportedFieldCmd struct {
	Ch chan int `flag:"ch"`
}

func (c *internalUnsupportedFieldCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_UnsupportedType(t *testing.T) {
	t.Parallel()

	cmd := &internalUnsupportedFieldCmd{}
	_, err := defaultParseFlags(cmd, []string{"--ch", "foo"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- renderHelp ---

type internalCmdLevelRenderer struct {
	internalBareCmd
}

func (c *internalCmdLevelRenderer) RenderHelp(_ Runner, _ []Runner, _ []FlagDef) string {
	return "cmd-level help"
}

func TestRenderHelp_CmdLevelRenderer(t *testing.T) {
	t.Parallel()

	var buf testWriter
	cmd := &internalCmdLevelRenderer{}
	opts := defaults()
	opts.stdout = &buf

	err := renderHelp(cmd, []Runner{cmd}, opts)
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
		chain:     []Runner{&internalBareCmd{}},
		chainArgs: [][]string{{"--help"}},
	}
	assert.True(t, helpRequested(resolved))
}

func TestHelpRequested_ShortInPositional(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:      []Runner{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"-h"},
	}
	assert.True(t, helpRequested(resolved))
}

func TestHelpRequested_NotRequested(t *testing.T) {
	t.Parallel()

	resolved := &resolvedCommand{
		chain:      []Runner{&internalBareCmd{}},
		chainArgs:  [][]string{nil},
		positional: []string{"arg1"},
	}
	assert.False(t, helpRequested(resolved))
}

// --- execute integration tests for coverage ---

// Test Before error with parent-child: parent's After should still run.
type internalBeforeParent struct {
	afterCalled bool
	child       Runner
}

func (c *internalBeforeParent) Run(_ context.Context, _ []string) error { return nil }
func (c *internalBeforeParent) Name() string                            { return "parent" }
func (c *internalBeforeParent) Subcommands() []Runner                   { return []Runner{c.child} }

func (c *internalBeforeParent) Before(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (c *internalBeforeParent) After(_ context.Context) error {
	c.afterCalled = true
	return nil
}

type internalBeforeFailChild struct{}

func (c *internalBeforeFailChild) Run(_ context.Context, _ []string) error { return nil }
func (c *internalBeforeFailChild) Name() string                            { return "child" }

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

// Test suggestion path via custom FlagParser that returns unknown flag error.
type internalErrorFlagParser struct{}

func (p *internalErrorFlagParser) ParseFlags(_ Runner, _ []string) ([]string, error) {
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

func (p *internalNoMatchFlagParser) ParseFlags(_ Runner, _ []string) ([]string, error) {
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

func (c *internalNoDescCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalNoDescCmd) Name() string                            { return "nodesc" }

func TestDefaultRenderHelp_NoDescription(t *testing.T) {
	t.Parallel()

	cmd := &internalNoDescCmd{}
	chain := []Runner{cmd}
	text := defaultRenderHelp(cmd, chain, nil)

	assert.Contains(t, text, "Usage:")
	assert.NotContains(t, text, "Flags:")
}

type internalEnvFlagCmd struct {
	Port int `flag:"port" env:"PORT" help:"Port"`
}

func (c *internalEnvFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalEnvFlagCmd) Name() string                            { return "envtest" }

func TestDefaultRenderHelp_FlagWithEnv(t *testing.T) {
	t.Parallel()

	cmd := &internalEnvFlagCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "(env: PORT)")
}

// --- suggestSubcommand with hidden and alias ---

type internalAliasedSubCmd struct{}

func (c *internalAliasedSubCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalAliasedSubCmd) Name() string                            { return "deploy" }
func (c *internalAliasedSubCmd) Aliases() []string                       { return []string{"dep"} }

type internalParentMixed struct{}

func (p *internalParentMixed) Run(_ context.Context, _ []string) error { return nil }

func (p *internalParentMixed) Subcommands() []Runner {
	return []Runner{&internalAliasedSubCmd{}, &internalHiddenSubCmd{}}
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

func (c *internalNoShortFlagCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalNoShortFlagCmd) Name() string                            { return "noshort" }

func TestDefaultRenderHelp_NoShortFlag(t *testing.T) {
	t.Parallel()

	cmd := &internalNoShortFlagCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "    --port")
}

// --- defaultRenderHelp with example without description ---

type internalExampleNoDescCmd struct{}

func (c *internalExampleNoDescCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalExampleNoDescCmd) Name() string                            { return "extest" }

func (c *internalExampleNoDescCmd) Examples() []Example {
	return []Example{{Command: "extest --flag"}}
}

func TestDefaultRenderHelp_ExampleNoDescription(t *testing.T) {
	t.Parallel()

	cmd := &internalExampleNoDescCmd{}
	chain := []Runner{cmd}

	text := defaultRenderHelp(cmd, chain, nil)
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

func (c *internalValueUnmarshalerCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_ValueReceiverUnmarshaler(t *testing.T) {
	t.Parallel()

	cmd := &internalValueUnmarshalerCmd{}
	_, err := defaultParseFlags(cmd, []string{"--val", "test"})
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

func (c *internalInt64Cmd) Run(_ context.Context, _ []string) error { return nil }

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
	_, err := defaultParseFlags(cmd, []string{"--big", "9999999999"})
	require.NoError(t, err)
	assert.Equal(t, int64(9999999999), cmd.Big)
}

// --- Slice flag support ---

type internalSliceCmd struct {
	Tags    []string `flag:"tag" short:"t" help:"Tags"`
	Ports   []int    `flag:"port" help:"Ports"`
	Weights []float64 `flag:"weight" help:"Weights"`
}

func (c *internalSliceCmd) Run(_ context.Context, _ []string) error { return nil }

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
			args:     []string{"--tag", "a", "-t", "b", "--port", "80"},
			wantTags: []string{"a", "b"},
			wantPorts: []int{80},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalSliceCmd{}
			_, err := defaultParseFlags(cmd, tt.args)
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

func (c *internalMapCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_Map(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args       []string
		wantMap    map[string]string
		assertErr  require.ErrorAssertionFunc
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
			_, err := defaultParseFlags(cmd, tt.args)
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
	Verbose bool `flag:"verbose" short:"v" negatable:"true" help:"Verbose output"`
}

func (c *internalNegatableCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_Negatable(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		wantVerbose bool
	}{
		"enable":  {args: []string{"--verbose"}, wantVerbose: true},
		"negate":  {args: []string{"--no-verbose"}, wantVerbose: false},
		"short":   {args: []string{"-v"}, wantVerbose: true},
		"enable then negate": {args: []string{"--verbose", "--no-verbose"}, wantVerbose: false},
		"negate then enable": {args: []string{"--no-verbose", "--verbose"}, wantVerbose: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &internalNegatableCmd{}
			_, err := defaultParseFlags(cmd, tt.args)
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
	assert.True(t, flags[0].Negatable)
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

func (c *internalCounterCmd) Run(_ context.Context, _ []string) error { return nil }

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
			_, err := defaultParseFlags(cmd, tt.args)
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

func (c *internalEnumCmd) Run(_ context.Context, _ []string) error { return nil }

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
			_, err := defaultParseFlags(cmd, tt.args)
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

func (c *internalEnumNoDefaultCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_EnumNoDefault(t *testing.T) {
	t.Parallel()

	// No default, no flag provided → zero value, no enum validation.
	cmd := &internalEnumNoDefaultCmd{}
	_, err := defaultParseFlags(cmd, nil)
	require.NoError(t, err)
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
		"string":   {typ: reflect.TypeOf(""), value: "hello", want: "hello", assertErr: require.NoError},
		"int":      {typ: reflect.TypeOf(0), value: "42", want: 42, assertErr: require.NoError},
		"int64":    {typ: reflect.TypeOf(int64(0)), value: "99", want: int64(99), assertErr: require.NoError},
		"float64":  {typ: reflect.TypeOf(0.0), value: "3.14", want: 3.14, assertErr: require.NoError},
		"bool":     {typ: reflect.TypeOf(false), value: "true", want: true, assertErr: require.NoError},
		"duration": {typ: reflect.TypeOf(time.Duration(0)), value: "5s", want: 5 * time.Second, assertErr: require.NoError},
		"bad int":  {typ: reflect.TypeOf(0), value: "abc", assertErr: require.Error},
		"bad float": {typ: reflect.TypeOf(0.0), value: "xyz", assertErr: require.Error},
		"bad bool":  {typ: reflect.TypeOf(false), value: "nope", assertErr: require.Error},
		"bad duration": {typ: reflect.TypeOf(time.Duration(0)), value: "bad", assertErr: require.Error},
		"unsupported": {typ: reflect.TypeOf(make(chan int)), assertErr: require.Error},
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

	subs := []Runner{
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

			result := findSubcommand(subs, tt.name, tt.prefix)
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

func (c *internalAliasedForPrefix) Run(_ context.Context, _ []string) error { return nil }
func (c *internalAliasedForPrefix) Name() string                            { return "deploy" }
func (c *internalAliasedForPrefix) Aliases() []string                       { return []string{"dep"} }

func TestFindSubcommand_PrefixMatchAliasAmbiguous(t *testing.T) {
	t.Parallel()

	subs := []Runner{&internalAliasedForPrefix{}}
	// "de" matches prefix of both "deploy" and alias "dep" — ambiguous in our impl.
	result := findSubcommand(subs, "de", true)
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
		"long":       {positional: []string{"--version"}, want: true},
		"short":      {positional: []string{"-V"}, want: true},
		"not present": {positional: []string{"arg1"}, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolved := &resolvedCommand{
				chain:      []Runner{&internalBareCmd{}},
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
		chain []Runner
		found bool
	}{
		"has versioner": {
			chain: []Runner{&internalVersionedCmd{}},
			found: true,
		},
		"no versioner": {
			chain: []Runner{&internalBareCmd{}},
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

func (p *internalDefaultParent) Run(_ context.Context, _ []string) error { return nil }
func (p *internalDefaultParent) Name() string                            { return "app" }
func (p *internalDefaultParent) Subcommands() []Runner                   { return []Runner{p.child} }
func (p *internalDefaultParent) Fallback() Runner                        { return p.def }

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

			resolved := resolveCommand(parent, tt.args, defaults())
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
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "--[no-]verbose")
}

// --- Help rendering: enum ---

func TestDefaultRenderHelp_Enum(t *testing.T) {
	t.Parallel()

	cmd := &internalEnumCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "[json|yaml|text]")
}

// --- Help rendering: counter ---

func TestDefaultRenderHelp_Counter(t *testing.T) {
	t.Parallel()

	cmd := &internalCounterCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "(repeatable)")
	assert.NotContains(t, text, "--verbose int")
}

// --- Help rendering: categories ---

type internalCatCmd struct {
	n   string
	cat string
}

func (c *internalCatCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *internalCatCmd) Name() string                            { return c.n }
func (c *internalCatCmd) Description() string                     { return c.n + " command" }
func (c *internalCatCmd) Category() string                        { return c.cat }

type internalCatParent struct {
	subs []Runner
}

func (p *internalCatParent) Run(_ context.Context, _ []string) error { return nil }
func (p *internalCatParent) Name() string                            { return "app" }
func (p *internalCatParent) Subcommands() []Runner                   { return p.subs }

func TestDefaultRenderHelp_Categories(t *testing.T) {
	t.Parallel()

	parent := &internalCatParent{
		subs: []Runner{
			&internalNamedCmd{n: "help"},
			&internalCatCmd{n: "serve", cat: "Server"},
			&internalCatCmd{n: "stop", cat: "Server"},
			&internalCatCmd{n: "deploy", cat: "Deploy"},
		},
	}
	chain := []Runner{parent}
	text := defaultRenderHelp(parent, chain, nil)

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
		subs: []Runner{
			&internalCatCmd{n: "serve", cat: "Server"},
			&internalCatCmd{n: "deploy", cat: "Deploy"},
		},
	}
	chain := []Runner{parent}
	text := defaultRenderHelp(parent, chain, nil)

	// No "Commands:" section when all are categorized
	assert.NotContains(t, text, "Commands:\n")
	assert.Contains(t, text, "Server:\n")
	assert.Contains(t, text, "Deploy:\n")
}

// --- renderSubcommands with all hidden ---

func TestRenderSubcommands_AllHidden(t *testing.T) {
	t.Parallel()

	parent := &internalParentWithHidden{child: &internalHiddenSubCmd{}}
	chain := []Runner{parent}
	text := defaultRenderHelp(parent, chain, nil)

	assert.NotContains(t, text, "Commands:")
	assert.NotContains(t, text, "secret")
}

// --- Slice with duration elements ---

type internalDurationSliceCmd struct {
	Timeouts []time.Duration `flag:"timeout" help:"Timeouts"`
}

func (c *internalDurationSliceCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_DurationSlice(t *testing.T) {
	t.Parallel()

	cmd := &internalDurationSliceCmd{}
	_, err := defaultParseFlags(cmd, []string{"--timeout", "5s", "--timeout", "10m"})
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

func (c *internalInt64SliceCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_Int64Slice(t *testing.T) {
	t.Parallel()

	cmd := &internalInt64SliceCmd{}
	_, err := defaultParseFlags(cmd, []string{"--id", "100", "--id", "200"})
	require.NoError(t, err)
	assert.Equal(t, []int64{100, 200}, cmd.IDs)
}

// --- Slice with unsupported element type ---

type internalBadSliceCmd struct {
	Items []chan int `flag:"item"`
}

func (c *internalBadSliceCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_SliceUnsupportedElem(t *testing.T) {
	t.Parallel()

	cmd := &internalBadSliceCmd{}
	_, err := defaultParseFlags(cmd, []string{"--item", "foo"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Map with bad key type ---

type internalBadMapKeyCmd struct {
	Items map[chan int]string `flag:"item"`
}

func (c *internalBadMapKeyCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_MapUnsupportedKey(t *testing.T) {
	t.Parallel()

	cmd := &internalBadMapKeyCmd{}
	_, err := defaultParseFlags(cmd, []string{"--item", "k=v"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Map with bad value type ---

type internalBadMapValCmd struct {
	Items map[string]chan int `flag:"item"`
}

func (c *internalBadMapValCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_MapUnsupportedValue(t *testing.T) {
	t.Parallel()

	cmd := &internalBadMapValCmd{}
	_, err := defaultParseFlags(cmd, []string{"--item", "k=v"})
	require.ErrorIs(t, err, ErrInvalidFlagValue)
}

// --- Enum with env var ---

type internalEnumEnvCmd struct {
	Format string `flag:"format" enum:"json,yaml" env:"TEST_FORMAT"`
}

func (c *internalEnumEnvCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_EnumEnv(t *testing.T) {
	t.Setenv("TEST_FORMAT", "xml")

	cmd := &internalEnumEnvCmd{}
	_, err := defaultParseFlags(cmd, nil)
	require.ErrorIs(t, err, ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "must be one of")
}

// --- Bool slice ---

type internalBoolSliceCmd struct {
	Flags []bool `flag:"flag" help:"Flags"`
}

func (c *internalBoolSliceCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultParseFlags_BoolSlice(t *testing.T) {
	t.Parallel()

	cmd := &internalBoolSliceCmd{}
	_, err := defaultParseFlags(cmd, []string{"--flag", "true", "--flag", "false"})
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
	child   Runner
}

func (p *internalShortOptParent) Run(_ context.Context, _ []string) error { return nil }
func (p *internalShortOptParent) Name() string                            { return "app" }
func (p *internalShortOptParent) Subcommands() []Runner                   { return []Runner{p.child} }

func TestResolveCommand_ShortOptionHandlingInScanPhase(t *testing.T) {
	t.Parallel()

	child := &internalNamedCmd{n: "serve"}
	parent := &internalShortOptParent{child: child}
	opts := defaults()
	opts.shortOptionHandling = true

	resolved := resolveCommand(parent, []string{"-v", "serve"}, opts)
	assert.Len(t, resolved.chain, 2)
	assert.Equal(t, "serve", resolveInfo(resolved.chain[1]).name)
}

// --- Prefix matching via alias ---

type internalPrefixAliased struct{}

func (c *internalPrefixAliased) Run(_ context.Context, _ []string) error { return nil }
func (c *internalPrefixAliased) Name() string                            { return "deploy" }
func (c *internalPrefixAliased) Aliases() []string                       { return []string{"dp"} }

func TestFindSubcommand_PrefixMatchAlias(t *testing.T) {
	t.Parallel()

	subs := []Runner{&internalPrefixAliased{}, &internalNamedCmd{n: "status"}}

	// "d" matches prefix of "deploy" and alias "dp" — but both are the same command.
	// In our implementation: first match on "deploy" sets match, second on "dp" sees match != nil → ambiguous.
	result := findSubcommand(subs, "d", true)
	assert.Nil(t, result) // ambiguous between name and alias

	// "sta" uniquely matches "status"
	result = findSubcommand(subs, "sta", true)
	require.NotNil(t, result)
	assert.Equal(t, "status", resolveInfo(result).name)

	// "dp" exact match via alias
	result = findSubcommand(subs, "dp", true)
	require.NotNil(t, result)
	assert.Equal(t, "deploy", resolveInfo(result).name)
}

// Test unique prefix match via alias only (name doesn't prefix-match).
type internalOnlyAliasPrefix struct{}

func (c *internalOnlyAliasPrefix) Run(_ context.Context, _ []string) error { return nil }
func (c *internalOnlyAliasPrefix) Name() string                            { return "xdeploy" }
func (c *internalOnlyAliasPrefix) Aliases() []string                       { return []string{"dp"} }

func TestFindSubcommand_PrefixMatchAliasOnly(t *testing.T) {
	t.Parallel()

	subs := []Runner{&internalOnlyAliasPrefix{}}
	// "d" does NOT prefix-match "xdeploy" but DOES prefix-match alias "dp".
	result := findSubcommand(subs, "d", true)
	require.NotNil(t, result)
	assert.Equal(t, "xdeploy", resolveInfo(result).name)
}

// --- Negatable without short flag in help ---

type internalNegatableNoShortCmd struct {
	Color bool `flag:"color" negatable:"true" help:"Colorize output"`
}

func (c *internalNegatableNoShortCmd) Run(_ context.Context, _ []string) error { return nil }

func TestDefaultRenderHelp_NegatableNoShort(t *testing.T) {
	t.Parallel()

	cmd := &internalNegatableNoShortCmd{}
	chain := []Runner{cmd}
	flags := ScanFlags(cmd)

	text := defaultRenderHelp(cmd, chain, flags)
	assert.Contains(t, text, "    --[no-]color")
}
