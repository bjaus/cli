package cli_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bjaus/cli"
)

// A complete CLI with a root command, subcommand, and flags.
// This is the typical starting point for building a CLI with this package.
type exRoot struct{}

func (r *exRoot) Name() string        { return "mytool" }
func (r *exRoot) Description() string { return "A CLI built with the cli package" }
func (r *exRoot) Version() string     { return "1.0.0" }
func (r *exRoot) Subcommands() []cli.Runner {
	return []cli.Runner{&exHello{}}
}

func (r *exRoot) Run(_ context.Context, _ []string) error {
	return cli.ErrShowHelp
}

type exHello struct {
	Recipient string `flag:"name" short:"n" default:"World" help:"Who to greet"`
}

func (h *exHello) Name() string        { return "hello" }
func (h *exHello) Description() string { return "Say hello" }
func (h *exHello) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "Hello, %s!\n", h.Recipient) //nolint:errcheck // example output
	return nil
}

func Example() {
	app := &exRoot{}
	_ = cli.Execute(context.Background(), app, []string{"hello", "--name", "Alice"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: Hello, Alice!
}

func Example_help() {
	app := &exRoot{}
	_ = cli.Execute(context.Background(), app, []string{"--help"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output:
	// A CLI built with the cli package
	//
	// Usage:
	//   mytool [command]
	//   mytool [args...]
	//
	// Commands:
	//   hello  Say hello
	//
	// Use "mytool [command] --help" for more information about a command.
}

// ErrShowHelp lets a command request help display from within Run.
// This is useful for root commands whose bare invocation should show help.
func ExampleErrShowHelp() {
	app := &exRoot{}
	// Bare invocation — root's Run returns ErrShowHelp, so the framework shows help.
	_ = cli.Execute(context.Background(), app, nil, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output:
	// A CLI built with the cli package
	//
	// Usage:
	//   mytool [command]
	//   mytool [args...]
	//
	// Commands:
	//   hello  Say hello
	//
	// Use "mytool [command] --help" for more information about a command.
}

// A minimal command using RunFunc.
func ExampleRunFunc() {
	hello := cli.RunFunc(func(_ context.Context, _ []string) error {
		fmt.Fprintln(os.Stdout, "Hello, world!") //nolint:errcheck // example output
		return nil
	})

	_ = cli.Execute(context.Background(), hello, nil, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: Hello, world!
}

// A command struct with flags parsed from struct tags.
type GreetCmd struct {
	Name    string `flag:"name" short:"n" default:"World" help:"Who to greet"`
	Excited bool   `flag:"excited" short:"e" help:"Add exclamation mark"`
}

func (g *GreetCmd) Run(_ context.Context, _ []string) error {
	punct := "."
	if g.Excited {
		punct = "!"
	}
	fmt.Fprintf(os.Stdout, "Hello, %s%s\n", g.Name, punct) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_flags() {
	cmd := &GreetCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"--name", "Alice", "-e"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: Hello, Alice!
}

// A parent command with subcommands demonstrating the tree structure.
type App struct{}

func (a *App) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "Use a subcommand. Try --help.") //nolint:errcheck // example output
	return nil
}

func (a *App) Name() string        { return "myapp" }
func (a *App) Description() string { return "My example application" }
func (a *App) Subcommands() []cli.Runner {
	return []cli.Runner{&GreetCmd{}, &VersionCmd{}}
}

type VersionCmd struct{}

func (v *VersionCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "v1.0.0") //nolint:errcheck // example output
	return nil
}

func (v *VersionCmd) Name() string        { return "version" }
func (v *VersionCmd) Description() string { return "Print version" }

func ExampleExecute_subcommands() {
	app := &App{}
	_ = cli.Execute(context.Background(), app, []string{"version"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: v1.0.0
}

// Demonstrating lifecycle hooks with Before and After.
type SetupApp struct {
	child *WorkerCmd
}

func (s *SetupApp) Run(_ context.Context, _ []string) error { return nil }
func (s *SetupApp) Name() string                            { return "app" }
func (s *SetupApp) Subcommands() []cli.Runner               { return []cli.Runner{s.child} }

func (s *SetupApp) Before(ctx context.Context) (context.Context, error) {
	fmt.Fprintln(os.Stdout, "setup: before") //nolint:errcheck // example output
	return ctx, nil
}

func (s *SetupApp) After(_ context.Context) error {
	fmt.Fprintln(os.Stdout, "setup: after") //nolint:errcheck // example output
	return nil
}

type WorkerCmd struct{}

func (w *WorkerCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "worker: run") //nolint:errcheck // example output
	return nil
}

func (w *WorkerCmd) Name() string { return "work" }

func ExampleExecute_lifecycle() {
	app := &SetupApp{child: &WorkerCmd{}}
	_ = cli.Execute(context.Background(), app, []string{"work"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output:
	// setup: before
	// worker: run
	// setup: after
}

// Demonstrating error handling with Exit.
func ExampleExit() {
	cmd := cli.RunFunc(func(_ context.Context, _ []string) error {
		return cli.Exit("port already in use", 2)
	})

	err := cli.Execute(context.Background(), cmd, nil)
	if err != nil {
		fmt.Fprintln(os.Stdout, err) //nolint:errcheck // example output
		var ec cli.ExitCoder
		if ok := errors.As(err, &ec); ok {
			fmt.Fprintf(os.Stdout, "exit code: %d\n", ec.ExitCode()) //nolint:errcheck // example output
		}
	}
	// Output:
	// port already in use
	// exit code: 2
}

// Demonstrating middleware wrapping around Run.
type LoggedCmd struct{}

func (l *LoggedCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "executing") //nolint:errcheck // example output
	return nil
}

func (l *LoggedCmd) Middleware() []func(next cli.RunFunc) cli.RunFunc {
	return []func(next cli.RunFunc) cli.RunFunc{
		func(next cli.RunFunc) cli.RunFunc {
			return func(ctx context.Context, args []string) error {
				fmt.Fprintln(os.Stdout, "middleware: before") //nolint:errcheck // example output
				err := next(ctx, args)
				fmt.Fprintln(os.Stdout, "middleware: after") //nolint:errcheck // example output
				return err
			}
		},
	}
}

func ExampleExecute_middleware() {
	cmd := &LoggedCmd{}
	_ = cli.Execute(context.Background(), cmd, nil, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output:
	// middleware: before
	// executing
	// middleware: after
}

// --- Tier 1 & 2 Feature Examples ---

// Slice flags accumulate values from repeated flags.
// Use when a flag can be specified multiple times, like --tag staging --tag prod.
type BuildCmd struct {
	Tags []string `flag:"tag" short:"t" help:"Tags to apply"`
}

func (b *BuildCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "tags: %s\n", strings.Join(b.Tags, ", ")) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_sliceFlags() {
	cmd := &BuildCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"--tag", "v1", "--tag", "latest"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: tags: v1, latest
}

// Map flags accept key=value pairs. Repeated flags add entries.
// Use for label-like data: --env DB_HOST=localhost --env DB_PORT=5432.
type DeployCmd struct {
	Env map[string]string `flag:"env" short:"e" help:"Environment variables"`
}

func (d *DeployCmd) Run(_ context.Context, _ []string) error {
	for k, v := range d.Env {
		fmt.Fprintf(os.Stdout, "%s=%s\n", k, v) //nolint:errcheck // example output
	}
	return nil
}

func ExampleExecute_mapFlags() {
	cmd := &DeployCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"--env", "REGION=us-east-1"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: REGION=us-east-1
}

// Negatable bools support --flag and --no-flag patterns.
// Use for features with sensible defaults that users may want to explicitly disable:
// --color is on by default, --no-color turns it off.
type ColorCmd struct {
	Color bool `flag:"color" default:"true" negatable:"true" help:"Colorize output"`
}

func (c *ColorCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "color: %v\n", c.Color) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_negatableBool() {
	cmd := &ColorCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"--no-color"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: color: false
}

// Versioner interface adds --version / -V support.
// Implement on your root command to report the application version.
type MyApp struct{}

func (a *MyApp) Run(_ context.Context, _ []string) error { return nil }
func (a *MyApp) Name() string                            { return "myapp" }
func (a *MyApp) Version() string                         { return "2.1.0" }

func ExampleExecute_versioner() {
	app := &MyApp{}
	_ = cli.Execute(context.Background(), app, []string{"--version"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: 2.1.0
}

// Deprecater interface prints a warning to stderr when a command runs.
// Use when retiring a command — keep it functional but warn users to migrate.
type OldCmd struct{}

func (o *OldCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "still works") //nolint:errcheck // example output
	return nil
}

func (o *OldCmd) Name() string       { return "old-cmd" }
func (o *OldCmd) Deprecated() string { return "use new-cmd instead" }

func ExampleExecute_deprecated() {
	cmd := &OldCmd{}
	// Redirect stderr to stdout so the example can capture it.
	_ = cli.Execute(context.Background(), cmd, nil, cli.WithStdout(os.Stdout), cli.WithStderr(os.Stdout)) //nolint:errcheck // example
	// Output:
	// Warning: "old-cmd" is deprecated: use new-cmd instead
	// still works
}

// Categorizer interface groups subcommands in help output.
// Use to organize large CLIs with many subcommands into logical groups.
type AdminCmd struct{}

func (a *AdminCmd) Run(_ context.Context, _ []string) error { return nil }
func (a *AdminCmd) Name() string                            { return "users" }
func (a *AdminCmd) Description() string                     { return "Manage users" }
func (a *AdminCmd) Category() string                        { return "Admin Commands" }

type CoreCmd struct{}

func (c *CoreCmd) Run(_ context.Context, _ []string) error { return nil }
func (c *CoreCmd) Name() string                            { return "run" }
func (c *CoreCmd) Description() string                     { return "Run the app" }

type CategorizedApp struct{}

func (a *CategorizedApp) Run(_ context.Context, _ []string) error { return nil }
func (a *CategorizedApp) Name() string                            { return "myapp" }
func (a *CategorizedApp) Description() string                     { return "Categorized example" }
func (a *CategorizedApp) Subcommands() []cli.Runner {
	return []cli.Runner{&CoreCmd{}, &AdminCmd{}}
}

func ExampleExecute_categories() {
	app := &CategorizedApp{}
	_ = cli.Execute(context.Background(), app, []string{"--help"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output:
	// Categorized example
	//
	// Usage:
	//   myapp [command]
	//   myapp [args...]
	//
	// Commands:
	//   run    Run the app
	//
	// Admin Commands:
	//   users  Manage users
	//
	// Use "myapp [command] --help" for more information about a command.
}

// Enum validation restricts a flag to a fixed set of values.
// The framework validates automatically — no need to check in Run.
type OutputCmd struct {
	Format string `flag:"format" short:"f" default:"text" enum:"text,json,yaml" help:"Output format"`
}

func (o *OutputCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "format: %s\n", o.Format) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_enum() {
	cmd := &OutputCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"--format", "json"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example

	// Invalid values produce a clear error.
	err := cli.Execute(context.Background(), cmd, []string{"--format", "xml"})
	fmt.Fprintln(os.Stdout, err) //nolint:errcheck // example output
	// Output:
	// format: json
	// invalid flag value: --format must be one of [text,json,yaml]
}

// Counter flags increment an int each time the flag appears.
// Classic use case: -v for info, -vv for debug, -vvv for trace.
type VerboseCmd struct {
	Verbosity int `flag:"verbose" short:"v" counter:"true" help:"Increase verbosity"`
}

func (c *VerboseCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "verbosity: %d\n", c.Verbosity) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_counter() {
	cmd := &VerboseCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"-v", "-v", "-v"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: verbosity: 3
}

// Short option combining lets users merge single-character flags.
// Enable with WithShortOptionHandling. -abc expands to -a -b -c.
// Combine with counters: -vvv is equivalent to -v -v -v.
type CompactCmd struct {
	All     bool `flag:"all" short:"a" help:"Show all"`
	Long    bool `flag:"long" short:"l" help:"Long format"`
	Reverse bool `flag:"reverse" short:"r" help:"Reverse order"`
}

func (c *CompactCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "all=%v long=%v reverse=%v\n", c.All, c.Long, c.Reverse) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_shortOptionHandling() {
	cmd := &CompactCmd{}
	_ = cli.Execute(context.Background(), cmd, []string{"-alr"}, //nolint:errcheck // example
		cli.WithStdout(os.Stdout),
		cli.WithShortOptionHandling(true),
	)
	// Output: all=true long=true reverse=true
}

// Prefix matching resolves unique prefixes to full subcommand names.
// "sta" matches "status" if no other subcommand starts with "sta".
// Ambiguous prefixes (matching multiple commands) produce an error.
type StatusCmd struct{}

func (s *StatusCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "all systems go") //nolint:errcheck // example output
	return nil
}

func (s *StatusCmd) Name() string { return "status" }

type PrefixApp struct{}

func (a *PrefixApp) Run(_ context.Context, _ []string) error { return nil }
func (a *PrefixApp) Name() string                            { return "app" }
func (a *PrefixApp) Subcommands() []cli.Runner               { return []cli.Runner{&StatusCmd{}} }

func ExampleExecute_prefixMatching() {
	app := &PrefixApp{}
	_ = cli.Execute(context.Background(), app, []string{"sta"}, //nolint:errcheck // example
		cli.WithStdout(os.Stdout),
		cli.WithPrefixMatching(true),
	)
	// Output: all systems go
}

// Fallbacker interface provides a fallback subcommand when none is specified.
// Use for CLIs where the "main" action shouldn't require a subcommand name.
type ServeCmd struct{}

func (s *ServeCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintln(os.Stdout, "serving on :8080") //nolint:errcheck // example output
	return nil
}

func (s *ServeCmd) Name() string { return "serve" }

type DefaultApp struct{}

func (a *DefaultApp) Run(_ context.Context, _ []string) error { return nil }
func (a *DefaultApp) Name() string                            { return "app" }
func (a *DefaultApp) Subcommands() []cli.Runner               { return []cli.Runner{&ServeCmd{}} }
func (a *DefaultApp) Fallback() cli.Runner                    { return &ServeCmd{} }

func ExampleExecute_defaultCommand() {
	app := &DefaultApp{}
	// No subcommand specified — Fallback() is invoked automatically.
	_ = cli.Execute(context.Background(), app, nil, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: serving on :8080
}

// Automatic flag inheritance: parent and child both declare --env.
// The child inherits the parent's parsed value when its flag is not set.
type InheritApp struct {
	Env string `flag:"env" help:"Target environment"`
}

func (a *InheritApp) Run(_ context.Context, _ []string) error { return nil }
func (a *InheritApp) Name() string                            { return "app" }
func (a *InheritApp) Subcommands() []cli.Runner               { return []cli.Runner{&InheritServe{}} }

type InheritServe struct {
	Env  string `flag:"env" help:"Target environment"`
	Port int    `flag:"port" default:"8080" help:"Listen port"`
}

func (s *InheritServe) Name() string { return "serve" }
func (s *InheritServe) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "env=%s port=%d\n", s.Env, s.Port) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_flagInheritance() {
	app := &InheritApp{}
	// --env is set on the parent; child inherits it automatically.
	_ = cli.Execute(context.Background(), app, []string{"--env", "prod", "serve"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: env=prod port=8080
}

// Hidden flag inheritance: the child gets the parent's flag value via a hidden
// flag with the same name. It does not appear in help but still participates
// in automatic flag inheritance.
type HiddenInheritApp struct {
	Env string `flag:"env" help:"Target environment"`
}

func (a *HiddenInheritApp) Run(_ context.Context, _ []string) error { return nil }
func (a *HiddenInheritApp) Name() string                            { return "app" }
func (a *HiddenInheritApp) Subcommands() []cli.Runner               { return []cli.Runner{&HiddenInheritServe{}} }

type HiddenInheritServe struct {
	Env  string `flag:"env" hidden:"true"`
	Port int    `flag:"port" default:"8080" help:"Listen port"`
}

func (s *HiddenInheritServe) Name() string { return "serve" }
func (s *HiddenInheritServe) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "env=%s port=%d\n", s.Env, s.Port) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_hiddenInherit() {
	app := &HiddenInheritApp{}
	// --env is set on the parent; child's hidden --env receives it via auto-inheritance.
	_ = cli.Execute(context.Background(), app, []string{"--env", "staging", "serve"}, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: env=staging port=8080
}

// ConfigResolver loads flag values from an external source.
// Config values sit between defaults and env vars in priority:
// explicit flag > env > config > default > zero.
type ConfigServeCmd struct {
	Port int    `flag:"port" default:"8080" help:"Listen port"`
	Host string `flag:"host" default:"localhost" help:"Host to bind to"`
}

func (c *ConfigServeCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "host=%s port=%d\n", c.Host, c.Port) //nolint:errcheck // example output
	return nil
}

func ExampleExecute_configResolver() {
	resolver := cli.ConfigResolver(func(flagName string) (string, bool) {
		m := map[string]string{"port": "9090", "host": "0.0.0.0"}
		v, ok := m[flagName]
		return v, ok
	})

	cmd := &ConfigServeCmd{}
	_ = cli.Execute(context.Background(), cmd, nil, //nolint:errcheck // example
		cli.WithStdout(os.Stdout),
		cli.WithConfigResolver(resolver),
	)
	// Output: host=0.0.0.0 port=9090
}

// ConfigProvider lets a command supply its own resolver.
// The command-level resolver takes priority over the global one.
type ConfigProviderCmd struct {
	Port int `flag:"port" default:"8080" help:"Listen port"`
}

func (c *ConfigProviderCmd) Run(_ context.Context, _ []string) error {
	fmt.Fprintf(os.Stdout, "port=%d\n", c.Port) //nolint:errcheck // example output
	return nil
}

func (c *ConfigProviderCmd) ConfigResolver() cli.ConfigResolver {
	return func(flagName string) (string, bool) {
		if flagName == "port" {
			return "3000", true
		}
		return "", false
	}
}

func ExampleExecute_configProvider() {
	cmd := &ConfigProviderCmd{}
	_ = cli.Execute(context.Background(), cmd, nil, cli.WithStdout(os.Stdout)) //nolint:errcheck // example
	// Output: port=3000
}
