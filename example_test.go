package cli_test

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/bjaus/cli"
)

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
