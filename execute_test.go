package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// --- Shared test commands ---

type bareCmd struct{}

func (c *bareCmd) Run(_ context.Context, _ []string) error { return nil }

type rootCmd struct {
	serve *serveCmd
}

func (r *rootCmd) Run(_ context.Context, _ []string) error { return nil }
func (r *rootCmd) Name() string                            { return "app" }
func (r *rootCmd) Description() string                     { return "Test application" }
func (r *rootCmd) Subcommands() []cli.Runner               { return []cli.Runner{r.serve} }

type serveCmd struct {
	Port int    `flag:"port" short:"p" default:"8080" help:"Port"`
	Host string `flag:"host" default:"localhost" help:"Host"`

	gotArgs []string
}

func (s *serveCmd) Run(_ context.Context, args []string) error {
	s.gotArgs = args
	return nil
}

func (s *serveCmd) Name() string        { return "serve" }
func (s *serveCmd) Description() string { return "Start the server" }

// --- Simple execution tests ---

func TestExecute_SimpleCommand(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := cli.RunFunc(func(_ context.Context, args []string) error {
		gotArgs = args
		return nil
	})

	err := cli.Execute(context.Background(), cmd, []string{"foo", "bar"})
	require.NoError(t, err)
	assert.Equal(t, []string{"foo", "bar"}, gotArgs)
}

func TestExecute_ErrorPropagation(t *testing.T) {
	t.Parallel()

	cmd := cli.RunFunc(func(_ context.Context, _ []string) error {
		return errors.New("boom")
	})

	err := cli.Execute(context.Background(), cmd, nil)
	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

// --- Subcommand tests ---

func TestExecute_Subcommand(t *testing.T) {
	t.Parallel()

	serve := &serveCmd{}
	root := &rootCmd{serve: serve}

	err := cli.Execute(context.Background(), root, []string{"serve", "--port", "9090"})
	require.NoError(t, err)
	assert.Equal(t, 9090, serve.Port)
	assert.Equal(t, "localhost", serve.Host)
}

func TestExecute_FlagsAnywhere(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args     []string
		wantPort int
	}{
		"flags after subcommand": {
			args:     []string{"serve", "--port", "3000"},
			wantPort: 3000,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			serve := &serveCmd{}
			root := &rootCmd{serve: serve}
			err := cli.Execute(context.Background(), root, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPort, serve.Port)
		})
	}
}

// --- Lifecycle suite ---

type LifecycleSuite struct {
	suite.Suite
}

type lifecycleTracker struct {
	order        []string
	beforeCtxKey string
}

type ctxKey string

type trackedRoot struct {
	tracker *lifecycleTracker
	child   *trackedChild
}

func (r *trackedRoot) Run(_ context.Context, _ []string) error { return nil }
func (r *trackedRoot) Name() string                            { return "root" }
func (r *trackedRoot) Subcommands() []cli.Runner               { return []cli.Runner{r.child} }

func (r *trackedRoot) Before(ctx context.Context) (context.Context, error) {
	r.tracker.order = append(r.tracker.order, "root-before")
	return context.WithValue(ctx, ctxKey("root"), "enriched"), nil
}

func (r *trackedRoot) After(_ context.Context) error {
	r.tracker.order = append(r.tracker.order, "root-after")
	return nil
}

type trackedChild struct {
	tracker *lifecycleTracker
}

func (c *trackedChild) Run(ctx context.Context, _ []string) error {
	c.tracker.order = append(c.tracker.order, "child-run")
	val, ok := ctx.Value(ctxKey("root")).(string)
	if ok {
		c.tracker.beforeCtxKey = val
	}
	return nil
}

func (c *trackedChild) Name() string { return "child" }

func (c *trackedChild) Before(ctx context.Context) (context.Context, error) {
	c.tracker.order = append(c.tracker.order, "child-before")
	return ctx, nil
}

func (c *trackedChild) After(_ context.Context) error {
	c.tracker.order = append(c.tracker.order, "child-after")
	return nil
}

func (s *LifecycleSuite) TestBeforeAfterOrder() {
	tracker := &lifecycleTracker{}
	child := &trackedChild{tracker: tracker}
	root := &trackedRoot{tracker: tracker, child: child}

	err := cli.Execute(context.Background(), root, []string{"child"})
	s.Require().NoError(err)

	s.Equal([]string{
		"root-before",
		"child-before",
		"child-run",
		"child-after",
		"root-after",
	}, tracker.order)
}

func (s *LifecycleSuite) TestContextEnrichment() {
	tracker := &lifecycleTracker{}
	child := &trackedChild{tracker: tracker}
	root := &trackedRoot{tracker: tracker, child: child}

	err := cli.Execute(context.Background(), root, []string{"child"})
	s.Require().NoError(err)
	s.Equal("enriched", tracker.beforeCtxKey)
}

func (s *LifecycleSuite) TestAfterRunsOnError() {
	tracker := &lifecycleTracker{}
	failChild := &failingChild{tracker: tracker}
	wrapper := &parentWithCustomChild{tracker: tracker, child: failChild}

	err := cli.Execute(context.Background(), wrapper, []string{"fail"})
	s.Require().Error(err)
	s.Contains(tracker.order, "wrapper-after")
}

type failingChild struct {
	tracker *lifecycleTracker
}

func (c *failingChild) Run(_ context.Context, _ []string) error {
	c.tracker.order = append(c.tracker.order, "fail-run")
	return errors.New("child failed")
}

func (c *failingChild) Name() string { return "fail" }

type parentWithCustomChild struct {
	tracker *lifecycleTracker
	child   cli.Runner
}

func (p *parentWithCustomChild) Run(_ context.Context, _ []string) error { return nil }
func (p *parentWithCustomChild) Name() string                            { return "wrapper" }
func (p *parentWithCustomChild) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

func (p *parentWithCustomChild) After(_ context.Context) error {
	p.tracker.order = append(p.tracker.order, "wrapper-after")
	return nil
}

func TestLifecycleSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(LifecycleSuite))
}

// --- Validator tests ---

type validatingCmd struct {
	Name string `flag:"name" required:"true"`
}

func (c *validatingCmd) Run(_ context.Context, _ []string) error { return nil }

func (c *validatingCmd) Validate() error {
	if len(c.Name) < 3 {
		return errors.New("name must be at least 3 characters")
	}
	return nil
}

func TestExecute_Validator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		assertErr require.ErrorAssertionFunc
	}{
		"valid": {
			args:      []string{"--name", "alice"},
			assertErr: require.NoError,
		},
		"invalid": {
			args:      []string{"--name", "ab"},
			assertErr: require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &validatingCmd{}
			err := cli.Execute(context.Background(), cmd, tt.args)
			tt.assertErr(t, err)
		})
	}
}

// --- Help tests ---

func TestExecute_HelpFlag(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
	}{
		"long help":  {args: []string{"--help"}},
		"short help": {args: []string{"-h"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			cmd := &serveCmd{}
			err := cli.Execute(context.Background(), cmd, tt.args, cli.WithStdout(&buf))
			require.NoError(t, err)
			assert.Contains(t, buf.String(), "Flags:")
		})
	}
}

// --- Middleware in execution ---

type middlewareCmd struct {
	order *[]string
}

func (c *middlewareCmd) Run(_ context.Context, _ []string) error {
	*c.order = append(*c.order, "run")
	return nil
}

func (c *middlewareCmd) Middleware() []func(next cli.RunFunc) cli.RunFunc {
	return []func(next cli.RunFunc) cli.RunFunc{
		func(next cli.RunFunc) cli.RunFunc {
			return func(ctx context.Context, args []string) error {
				*c.order = append(*c.order, "mw-before")
				err := next(ctx, args)
				*c.order = append(*c.order, "mw-after")
				return err
			}
		},
	}
}

func TestExecute_WithMiddleware(t *testing.T) {
	t.Parallel()

	var order []string
	cmd := &middlewareCmd{order: &order}

	err := cli.Execute(context.Background(), cmd, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"mw-before", "run", "mw-after"}, order)
}

// --- Option tests ---

func TestWithStderr(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := cli.RunFunc(func(_ context.Context, _ []string) error { return nil })
	err := cli.Execute(context.Background(), cmd, nil, cli.WithStderr(&buf))
	require.NoError(t, err)
}

func TestWithFlagParser(t *testing.T) {
	t.Parallel()

	parsed := false
	parser := &testFlagParser{
		fn: func(_ cli.Runner, args []string) ([]string, error) {
			parsed = true
			return args, nil
		},
	}

	// serveCmd has flags, so parseFlags is not skipped.
	cmd := &serveCmd{}
	err := cli.Execute(context.Background(), cmd, nil, cli.WithFlagParser(parser))
	require.NoError(t, err)
	assert.True(t, parsed)
}

type testFlagParser struct {
	fn func(cli.Runner, []string) ([]string, error)
}

func (p *testFlagParser) ParseFlags(cmd cli.Runner, args []string) ([]string, error) {
	return p.fn(cmd, args)
}

func TestWithSuggest_Disabled(t *testing.T) {
	t.Parallel()

	// Use a custom FlagParser that returns an unknown flag error to trigger suggestion path.
	parser := &testFlagParser{
		fn: func(_ cli.Runner, _ []string) ([]string, error) {
			return nil, fmt.Errorf("unknown flag: --prot")
		},
	}

	cmd := &serveCmd{}
	err := cli.Execute(context.Background(), cmd, nil, cli.WithFlagParser(parser), cli.WithSuggest(false))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "Did you mean")
}

func TestWithSuggest_Enabled(t *testing.T) {
	t.Parallel()

	// Use a custom FlagParser that returns an unknown flag error to trigger suggestion path.
	parser := &testFlagParser{
		fn: func(_ cli.Runner, _ []string) ([]string, error) {
			return nil, fmt.Errorf("unknown flag: --prot")
		},
	}

	cmd := &serveCmd{}
	err := cli.Execute(context.Background(), cmd, nil, cli.WithFlagParser(parser), cli.WithSuggest(true))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Did you mean")
}

// --- Alias resolution ---

type aliasedCmd struct{}

func (c *aliasedCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *aliasedCmd) Name() string                            { return "deploy" }
func (c *aliasedCmd) Aliases() []string                       { return []string{"d", "dep"} }

type aliasParent struct{}

func (p *aliasParent) Run(_ context.Context, _ []string) error { return nil }
func (p *aliasParent) Name() string                            { return "app" }
func (p *aliasParent) Subcommands() []cli.Runner               { return []cli.Runner{&aliasedCmd{}} }

func TestExecute_AliasResolution(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
	}{
		"full name":   {args: []string{"deploy"}},
		"short alias": {args: []string{"d"}},
		"long alias":  {args: []string{"dep"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := &aliasParent{}
			err := cli.Execute(context.Background(), root, tt.args)
			require.NoError(t, err)
		})
	}
}

// --- ExecuteAndExit subprocess tests ---

func TestExecuteAndExit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		envMode  string
		wantCode int
	}{
		"success exits 0":        {envMode: "success", wantCode: 0},
		"exit coder exits code":  {envMode: "exitcoder", wantCode: 42},
		"generic error exits 1":  {envMode: "error", wantCode: 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestExecuteAndExitHelper") //nolint:gosec // test subprocess
			cmd.Env = append(os.Environ(), "EXEC_AND_EXIT_MODE="+tt.envMode)

			err := cmd.Run()
			if tt.wantCode == 0 {
				require.NoError(t, err)
				return
			}

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, tt.wantCode, exitErr.ExitCode())
		})
	}
}

// TestExecuteAndExitHelper is a helper for the subprocess test. It is not run
// directly; instead TestExecuteAndExit invokes it via exec.Command.
func TestExecuteAndExitHelper(t *testing.T) {
	mode := os.Getenv("EXEC_AND_EXIT_MODE")
	if mode == "" {
		return
	}

	switch mode {
	case "success":
		cli.ExecuteAndExit(context.Background(),
			cli.RunFunc(func(_ context.Context, _ []string) error { return nil }), nil)
	case "exitcoder":
		cli.ExecuteAndExit(context.Background(),
			cli.RunFunc(func(_ context.Context, _ []string) error { return cli.Exit("fail", 42) }), nil)
	case "error":
		cli.ExecuteAndExit(context.Background(),
			cli.RunFunc(func(_ context.Context, _ []string) error { return errors.New("boom") }), nil)
	}
}

// --- parseFlags remaining prepend test ---

func TestExecute_ParseRemainingPrepend(t *testing.T) {
	t.Parallel()

	// Use serveCmd (has flags) with a custom FlagParser that returns remaining args.
	parser := &testFlagParser{
		fn: func(_ cli.Runner, _ []string) ([]string, error) {
			return []string{"extra"}, nil
		},
	}

	serve := &serveCmd{}
	err := cli.Execute(context.Background(), serve, []string{"pos1"}, cli.WithFlagParser(parser))
	require.NoError(t, err)
	assert.Equal(t, []string{"extra", "pos1"}, serve.gotArgs)
}

// --- Help flag in subcommand chain ---

func TestExecute_HelpInSubcommandChain(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	serve := &serveCmd{}
	root := &rootCmd{serve: serve}

	err := cli.Execute(context.Background(), root, []string{"serve", "--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Flags:")
	assert.Contains(t, buf.String(), "--port")
}

// --- After hook error test ---

type afterErrorParent struct {
	errMsg string
}

func (p *afterErrorParent) Run(_ context.Context, _ []string) error { return nil }
func (p *afterErrorParent) Name() string                            { return "app" }

func (p *afterErrorParent) After(_ context.Context) error {
	return fmt.Errorf("%s", p.errMsg)
}

func TestExecute_AfterHookError(t *testing.T) {
	t.Parallel()

	cmd := &afterErrorParent{errMsg: "after cleanup failed"}
	err := cli.Execute(context.Background(), cmd, nil)
	require.Error(t, err)
	assert.Equal(t, "after cleanup failed", err.Error())
}

// --- ExecuteAndExit with exit code ---

func TestExecute_ExitCoderFromRun(t *testing.T) {
	t.Parallel()

	cmd := cli.RunFunc(func(_ context.Context, _ []string) error {
		return cli.Exit("port in use", 2)
	})

	err := cli.Execute(context.Background(), cmd, nil)
	require.Error(t, err)

	var ec cli.ExitCoder
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, 2, ec.ExitCode())
}

// --- ScanFlags on RunFunc ---

func TestScanFlags_RunFunc(t *testing.T) {
	t.Parallel()

	cmd := cli.RunFunc(func(_ context.Context, _ []string) error { return nil })
	defs := cli.ScanFlags(cmd)
	assert.Nil(t, defs)
}

// --- Env var satisfies required flag ---

type envRequiredCmd struct {
	Port int    `flag:"port" required:"true" env:"TEST_REQ_PORT"`
	Name string `flag:"name" required:"true"`
}

func (c *envRequiredCmd) Run(_ context.Context, _ []string) error { return nil }

func TestExecute_EnvSatisfiesRequired(t *testing.T) {
	t.Setenv("TEST_REQ_PORT", "9090")

	cmd := &envRequiredCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--name", "test"})
	require.NoError(t, err)
	assert.Equal(t, 9090, cmd.Port)
	assert.Equal(t, "test", cmd.Name)
}

// --- Validate env var satisfying required (use type assertion helper) ---

func TestScanFlags_EnvAndRequired(t *testing.T) {
	t.Parallel()

	cmd := &envRequiredCmd{}
	defs := cli.ScanFlags(cmd)

	portFlag := findFlagDef(defs, "port")
	require.NotNil(t, portFlag)
	assert.True(t, portFlag.Required)
	assert.Equal(t, "TEST_REQ_PORT", portFlag.Env)
}

func findFlagDef(defs []cli.FlagDef, name string) *cli.FlagDef {
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

// --- strconv error path for equals syntax ---

func TestExecute_InvalidIntEqualsValue(t *testing.T) {
	t.Parallel()

	cmd := &serveCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--port=abc"})
	require.Error(t, err)
}

// --- Version interface ---

type versionedRoot struct {
	serve *serveCmd
}

func (v *versionedRoot) Run(_ context.Context, _ []string) error { return nil }
func (v *versionedRoot) Name() string                            { return "myapp" }
func (v *versionedRoot) Version() string                         { return "v2.1.0" }
func (v *versionedRoot) Subcommands() []cli.Runner               { return []cli.Runner{v.serve} }

func TestExecute_Version(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args       []string
		wantOutput string
	}{
		"long version":  {args: []string{"--version"}, wantOutput: "v2.1.0\n"},
		"short version": {args: []string{"-V"}, wantOutput: "v2.1.0\n"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			root := &versionedRoot{serve: &serveCmd{}}
			err := cli.Execute(context.Background(), root, tt.args, cli.WithStdout(&buf))
			require.NoError(t, err)
			assert.Equal(t, tt.wantOutput, buf.String())
		})
	}
}

func TestExecute_VersionNoVersioner(t *testing.T) {
	t.Parallel()

	// --version on a non-Versioner becomes positional.
	var gotArgs []string
	cmd := cli.RunFunc(func(_ context.Context, args []string) error {
		gotArgs = args
		return nil
	})

	err := cli.Execute(context.Background(), cmd, []string{"--version"})
	require.NoError(t, err)
	assert.Contains(t, gotArgs, "--version")
}

// --- Deprecater interface ---

type deprecatedCmd struct{}

func (c *deprecatedCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *deprecatedCmd) Name() string                            { return "oldcmd" }
func (c *deprecatedCmd) Deprecated() string                      { return "use newcmd instead" }

func TestExecute_Deprecated(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cmd := &deprecatedCmd{}
	err := cli.Execute(context.Background(), cmd, nil, cli.WithStderr(&stderr))
	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "deprecated")
	assert.Contains(t, stderr.String(), "use newcmd instead")
}

// --- Fallbacker interface ---

type defaultParentCmd struct {
	defaultCmd cli.Runner
	child      *serveCmd
}

func (p *defaultParentCmd) Run(_ context.Context, _ []string) error { return nil }
func (p *defaultParentCmd) Name() string                            { return "app" }
func (p *defaultParentCmd) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }
func (p *defaultParentCmd) Fallback() cli.Runner                    { return p.defaultCmd }

func TestExecute_Fallback(t *testing.T) {
	t.Parallel()

	var defaultRan bool
	def := cli.RunFunc(func(_ context.Context, _ []string) error {
		defaultRan = true
		return nil
	})

	parent := &defaultParentCmd{defaultCmd: def, child: &serveCmd{}}
	err := cli.Execute(context.Background(), parent, nil)
	require.NoError(t, err)
	assert.True(t, defaultRan)
}

func TestExecute_FallbackNotUsedWhenSubcommandMatches(t *testing.T) {
	t.Parallel()

	var defaultRan bool
	def := cli.RunFunc(func(_ context.Context, _ []string) error {
		defaultRan = true
		return nil
	})

	serve := &serveCmd{}
	parent := &defaultParentCmd{defaultCmd: def, child: serve}
	err := cli.Execute(context.Background(), parent, []string{"serve", "--port", "3000"})
	require.NoError(t, err)
	assert.False(t, defaultRan)
	assert.Equal(t, 3000, serve.Port)
}

// --- Prefix matching ---

func TestExecute_PrefixMatching(t *testing.T) {
	t.Parallel()

	serve := &serveCmd{}
	root := &rootCmd{serve: serve}

	err := cli.Execute(context.Background(), root, []string{"ser", "--port", "4000"}, cli.WithPrefixMatching(true))
	require.NoError(t, err)
	assert.Equal(t, 4000, serve.Port)
}

func TestExecute_PrefixMatchingDisabled(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	root := cli.RunFunc(func(_ context.Context, args []string) error {
		gotArgs = args
		return nil
	})

	err := cli.Execute(context.Background(), root, []string{"ser"})
	require.NoError(t, err)
	assert.Contains(t, gotArgs, "ser")
}

// --- Short option handling ---

type shortOptCmd struct {
	Verbose bool `flag:"verbose" short:"v"`
	Debug   bool `flag:"debug" short:"d"`
	Port    int  `flag:"port" short:"p" default:"8080"`

	gotArgs []string
}

func (c *shortOptCmd) Run(_ context.Context, args []string) error {
	c.gotArgs = args
	return nil
}

func TestExecute_ShortOptionHandling(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		wantVerbose bool
		wantDebug   bool
		wantPort    int
	}{
		"combined bools": {
			args:        []string{"-vd"},
			wantVerbose: true,
			wantDebug:   true,
			wantPort:    8080,
		},
		"combined with value last": {
			args:        []string{"-vp", "9090"},
			wantVerbose: true,
			wantPort:    9090,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &shortOptCmd{}
			err := cli.Execute(context.Background(), cmd, tt.args, cli.WithShortOptionHandling(true))
			require.NoError(t, err)
			assert.Equal(t, tt.wantVerbose, cmd.Verbose)
			assert.Equal(t, tt.wantDebug, cmd.Debug)
			assert.Equal(t, tt.wantPort, cmd.Port)
		})
	}
}

// --- Categorizer in help ---

type catSubCmd struct {
	n   string
	cat string
}

func (c *catSubCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *catSubCmd) Name() string                            { return c.n }
func (c *catSubCmd) Description() string                     { return c.n + " desc" }
func (c *catSubCmd) Category() string                        { return c.cat }

type catParentCmd struct{}

func (p *catParentCmd) Run(_ context.Context, _ []string) error { return nil }
func (p *catParentCmd) Name() string                            { return "app" }

func (p *catParentCmd) Subcommands() []cli.Runner {
	return []cli.Runner{
		&catSubCmd{n: "serve", cat: "Server"},
		&catSubCmd{n: "deploy", cat: "Ops"},
	}
}

func TestExecute_HelpWithCategories(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cmd := &catParentCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--help"}, cli.WithStdout(&buf))
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Server:")
	assert.Contains(t, buf.String(), "Ops:")
}

// --- Slice flags via Execute ---

type sliceCmd struct {
	Tags []string `flag:"tag" short:"t"`

	gotArgs []string
}

func (c *sliceCmd) Run(_ context.Context, args []string) error {
	c.gotArgs = args
	return nil
}

func TestExecute_SliceFlags(t *testing.T) {
	t.Parallel()

	cmd := &sliceCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"--tag", "a", "-t", "b", "positional"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, cmd.Tags)
	assert.Equal(t, []string{"positional"}, cmd.gotArgs)
}

// --- Negatable via Execute ---

type negatableExtCmd struct {
	Color bool `flag:"color" negatable:"true" default:"true"`
}

func (c *negatableExtCmd) Run(_ context.Context, _ []string) error { return nil }

func TestExecute_Negatable(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		wantColor bool
	}{
		"default true":  {args: nil, wantColor: true},
		"negated":       {args: []string{"--no-color"}, wantColor: false},
		"explicit true": {args: []string{"--color"}, wantColor: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &negatableExtCmd{}
			err := cli.Execute(context.Background(), cmd, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantColor, cmd.Color)
		})
	}
}

// --- Counter via Execute ---

type counterExtCmd struct {
	Verbosity int `flag:"verbose" short:"v" counter:"true"`
}

func (c *counterExtCmd) Run(_ context.Context, _ []string) error { return nil }

func TestExecute_Counter(t *testing.T) {
	t.Parallel()

	cmd := &counterExtCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"-v", "-v", "-v"})
	require.NoError(t, err)
	assert.Equal(t, 3, cmd.Verbosity)
}

// --- Counter with short option combining ---

func TestExecute_CounterWithShortOptions(t *testing.T) {
	t.Parallel()

	cmd := &counterExtCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"-vvv"}, cli.WithShortOptionHandling(true))
	require.NoError(t, err)
	assert.Equal(t, 3, cmd.Verbosity)
}

// --- Enum via Execute ---

type enumExtCmd struct {
	Format string `flag:"format" enum:"json,yaml,text" default:"json"`
}

func (c *enumExtCmd) Run(_ context.Context, _ []string) error { return nil }

func TestExecute_Enum(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args      []string
		assertErr require.ErrorAssertionFunc
	}{
		"valid":   {args: []string{"--format", "yaml"}, assertErr: require.NoError},
		"invalid": {args: []string{"--format", "csv"}, assertErr: require.Error},
		"default": {args: nil, assertErr: require.NoError},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := &enumExtCmd{}
			err := cli.Execute(context.Background(), cmd, tt.args)
			tt.assertErr(t, err)
		})
	}
}

// --- Map flags via Execute ---

type mapExtCmd struct {
	Headers map[string]string `flag:"header" short:"H"`
}

func (c *mapExtCmd) Run(_ context.Context, _ []string) error { return nil }

func TestExecute_MapFlags(t *testing.T) {
	t.Parallel()

	cmd := &mapExtCmd{}
	err := cli.Execute(context.Background(), cmd, []string{"-H", "X-Foo=bar", "-H", "X-Baz=qux"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"X-Foo": "bar", "X-Baz": "qux"}, cmd.Headers)
}

// --- ScanFlags new fields ---

type flagScanTestCmd struct {
	Format  string   `flag:"format" enum:"json,yaml"`
	Verbose int      `flag:"verbose" counter:"true"`
	Color   bool     `flag:"color" negatable:"true"`
	Tags    []string `flag:"tag"`
}

func (c *flagScanTestCmd) Run(_ context.Context, _ []string) error { return nil }

func TestScanFlags_NewFields(t *testing.T) {
	t.Parallel()

	cmd := &flagScanTestCmd{}
	defs := cli.ScanFlags(cmd)
	require.Len(t, defs, 4)

	format := findFlagDef(defs, "format")
	require.NotNil(t, format)
	assert.Equal(t, "json,yaml", format.Enum)

	verbose := findFlagDef(defs, "verbose")
	require.NotNil(t, verbose)
	assert.True(t, verbose.IsCounter)

	color := findFlagDef(defs, "color")
	require.NotNil(t, color)
	assert.True(t, color.Negatable)

	tag := findFlagDef(defs, "tag")
	require.NotNil(t, tag)
	assert.Equal(t, "strings", tag.TypeName)
}

// --- Flag inheritance tests ---

// Parent and child both declare --env. Child inherits parent's value.
type inheritParent struct {
	Env   string `flag:"env" help:"Target environment"`
	child cli.Runner
}

func (p *inheritParent) Run(_ context.Context, _ []string) error { return nil }
func (p *inheritParent) Name() string                            { return "app" }
func (p *inheritParent) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

type inheritChild struct {
	Env  string `flag:"env" help:"Target environment"`
	Port int    `flag:"port" default:"8080" help:"Listen port"`
}

func (c *inheritChild) Run(_ context.Context, _ []string) error { return nil }
func (c *inheritChild) Name() string                            { return "serve" }

func TestExecute_FlagInheritance_ParentToChild(t *testing.T) {
	t.Parallel()

	child := &inheritChild{}
	parent := &inheritParent{Env: "", child: child}

	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "prod", child.Env)
	assert.Equal(t, 8080, child.Port)
}

func TestExecute_FlagInheritance_ChildOverrides(t *testing.T) {
	t.Parallel()

	child := &inheritChild{}
	parent := &inheritParent{child: child}

	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve", "--env", "staging"})
	require.NoError(t, err)
	assert.Equal(t, "staging", child.Env)
}

type inheritChildEnv struct {
	Env  string `flag:"env" env:"TEST_INHERIT_ENV" help:"Target environment"`
	Port int    `flag:"port" default:"8080" help:"Listen port"`
}

func (c *inheritChildEnv) Run(_ context.Context, _ []string) error { return nil }
func (c *inheritChildEnv) Name() string                            { return "serve" }

func TestExecute_FlagInheritance_ChildEnvOverrides(t *testing.T) {
	t.Setenv("TEST_INHERIT_ENV", "fromenv")

	child := &inheritChildEnv{}
	parent := &inheritParent{child: child}

	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve"})
	require.NoError(t, err)
	// Child env var takes precedence over inherited value.
	assert.Equal(t, "fromenv", child.Env)
}

type inheritRequiredChild struct {
	Env string `flag:"env" required:"true" help:"Target environment"`
}

func (c *inheritRequiredChild) Run(_ context.Context, _ []string) error { return nil }
func (c *inheritRequiredChild) Name() string                            { return "serve" }

func TestExecute_FlagInheritance_SatisfiesRequired(t *testing.T) {
	t.Parallel()

	child := &inheritRequiredChild{}
	parent := &inheritParent{child: child}

	// Child's --env is required but not set explicitly — inherited from parent.
	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "prod", child.Env)
}

type inheritEnumChild struct {
	Env string `flag:"env" enum:"dev,qa,prod" help:"Target environment"`
}

func (c *inheritEnumChild) Run(_ context.Context, _ []string) error { return nil }
func (c *inheritEnumChild) Name() string                            { return "serve" }

func TestExecute_FlagInheritance_ValidatedAgainstEnum(t *testing.T) {
	t.Parallel()

	// Inherited value passes child's enum check.
	child := &inheritEnumChild{}
	parent := &inheritParent{child: child}
	err := cli.Execute(context.Background(), parent, []string{"--env", "prod", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "prod", child.Env)

	// Inherited value that fails child's enum check.
	child2 := &inheritEnumChild{}
	parent2 := &inheritParent{child: child2}
	err = cli.Execute(context.Background(), parent2, []string{"--env", "local", "serve"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

// Multi-level: grandparent → parent → child.
type inheritGrandparent struct {
	Env   string `flag:"env" help:"Target environment"`
	child cli.Runner
}

func (p *inheritGrandparent) Run(_ context.Context, _ []string) error { return nil }
func (p *inheritGrandparent) Name() string                            { return "root" }
func (p *inheritGrandparent) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

type inheritMiddle struct {
	child cli.Runner
}

func (m *inheritMiddle) Run(_ context.Context, _ []string) error { return nil }
func (m *inheritMiddle) Name() string                            { return "middle" }
func (m *inheritMiddle) Subcommands() []cli.Runner               { return []cli.Runner{m.child} }

func TestExecute_FlagInheritance_MultiLevel(t *testing.T) {
	t.Parallel()

	child := &inheritChild{}
	middle := &inheritMiddle{child: child}
	gp := &inheritGrandparent{child: middle}

	err := cli.Execute(context.Background(), gp, []string{"--env", "qa", "middle", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "qa", child.Env)
}

// No inheritance when types don't match.
type inheritIntParent struct {
	Env   int `flag:"env" help:"Env as int"`
	child cli.Runner
}

func (p *inheritIntParent) Run(_ context.Context, _ []string) error { return nil }
func (p *inheritIntParent) Name() string                            { return "app" }
func (p *inheritIntParent) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

func TestExecute_FlagInheritance_TypeMismatch(t *testing.T) {
	t.Parallel()

	child := &inheritChild{} // Env is string
	parent := &inheritIntParent{child: child}

	err := cli.Execute(context.Background(), parent, []string{"--env", "42", "serve"})
	require.NoError(t, err)
	// Types don't match, so no inheritance; child Env stays zero.
	assert.Equal(t, "", child.Env)
}

// --- Inherit tag tests ---

type inheritTagParent struct {
	Env   string `flag:"env" help:"Target environment"`
	child cli.Runner
}

func (p *inheritTagParent) Run(_ context.Context, _ []string) error { return nil }
func (p *inheritTagParent) Name() string                            { return "app" }
func (p *inheritTagParent) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

type inheritTagChild struct {
	Env  string `inherit:"env"`
	Port int    `flag:"port" default:"8080" help:"Listen port"`
}

func (c *inheritTagChild) Run(_ context.Context, _ []string) error { return nil }
func (c *inheritTagChild) Name() string                            { return "serve" }

func TestExecute_InheritTag_Basic(t *testing.T) {
	t.Parallel()

	child := &inheritTagChild{}
	parent := &inheritTagParent{child: child}

	err := cli.Execute(context.Background(), parent, []string{"--env", "staging", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "staging", child.Env)
	assert.Equal(t, 8080, child.Port)
}

// Nearest ancestor wins for inherit tag.
type inheritTagMiddle struct {
	Env   string `flag:"env" help:"Middle env"`
	child cli.Runner
}

func (m *inheritTagMiddle) Run(_ context.Context, _ []string) error { return nil }
func (m *inheritTagMiddle) Name() string                            { return "middle" }
func (m *inheritTagMiddle) Subcommands() []cli.Runner               { return []cli.Runner{m.child} }

func TestExecute_InheritTag_NearestAncestorWins(t *testing.T) {
	t.Parallel()

	child := &inheritTagChild{}
	middle := &inheritTagMiddle{child: child}
	gp := &inheritTagParent{child: middle}

	err := cli.Execute(context.Background(), gp, []string{"--env", "from-gp", "middle", "--env", "from-middle", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "from-middle", child.Env)
}

func TestExecute_InheritTag_NotInHelp(t *testing.T) {
	t.Parallel()

	child := &inheritTagChild{}
	defs := cli.ScanFlags(child)
	// ScanFlags only returns flag-tagged fields, not inherit-tagged fields.
	for _, d := range defs {
		assert.NotEqual(t, "env", d.Name)
	}
	assert.Len(t, defs, 1) // only "port"
}

func TestExecute_InheritTag_DoesNotRegisterFlag(t *testing.T) {
	t.Parallel()

	// inherit:"env" does not register --env as a child flag.
	// Verify via ScanFlags that the inherit field is invisible.
	child := &inheritTagChild{}
	defs := cli.ScanFlags(child)
	for _, d := range defs {
		assert.NotEqual(t, "env", d.Name, "inherit field should not appear in ScanFlags")
	}
}

type inheritTagNoEnvParent struct {
	child cli.Runner
}

func (p *inheritTagNoEnvParent) Run(_ context.Context, _ []string) error { return nil }
func (p *inheritTagNoEnvParent) Name() string                            { return "app" }
func (p *inheritTagNoEnvParent) Subcommands() []cli.Runner               { return []cli.Runner{p.child} }

func TestExecute_InheritTag_NoAncestorMatch(t *testing.T) {
	t.Parallel()

	// Parent has no --env flag, child has inherit:"env" — stays zero.
	child := &inheritTagChild{}
	parent := &inheritTagNoEnvParent{child: child}

	err := cli.Execute(context.Background(), parent, []string{"serve"})
	require.NoError(t, err)
	assert.Equal(t, "", child.Env)
}
