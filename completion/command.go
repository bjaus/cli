package completion

import (
	"context"
	"fmt"
	"io"

	"github.com/bjaus/cli"
)

// Command returns a [cli.Runner] that prints shell completion scripts. It has
// bash, zsh, fish, and powershell subcommands. Output is written to w.
// Typically added as a hidden "completion" subcommand:
//
//	func (a *App) Subcommands() []cli.Runner {
//	    return []cli.Runner{
//	        &serveCmd{},
//	        completion.Command(a, "myapp", os.Stdout),
//	    }
//	}
func Command(root cli.Runner, appName string, w io.Writer) cli.Runner {
	return &completionCmd{root: root, appName: appName, out: w}
}

type completionCmd struct {
	root    cli.Runner
	appName string
	out     io.Writer
}

func (c *completionCmd) Run(_ context.Context) error {
	return cli.ErrShowHelp
}

func (c *completionCmd) Name() string        { return "completion" }
func (c *completionCmd) Description() string { return "Generate shell completion scripts" }
func (c *completionCmd) Hidden() bool        { return true }

func (c *completionCmd) Subcommands() []cli.Runner {
	return []cli.Runner{
		&shellCmd{root: c.root, appName: c.appName, shell: "bash", out: c.out},
		&shellCmd{root: c.root, appName: c.appName, shell: "zsh", out: c.out},
		&shellCmd{root: c.root, appName: c.appName, shell: "fish", out: c.out},
		&shellCmd{root: c.root, appName: c.appName, shell: "powershell", out: c.out},
	}
}

type shellCmd struct {
	root    cli.Runner
	appName string
	shell   string
	out     io.Writer
}

func (s *shellCmd) Run(_ context.Context) error {
	var script string
	switch s.shell {
	case "bash":
		script = Bash(s.root, s.appName)
	case "zsh":
		script = Zsh(s.root, s.appName)
	case "fish":
		script = Fish(s.root, s.appName)
	case "powershell":
		script = PowerShell(s.root, s.appName)
	}
	_, err := fmt.Fprint(s.out, script)
	return err
}

func (s *shellCmd) Name() string { return s.shell }

func (s *shellCmd) Description() string {
	return fmt.Sprintf("Generate %s completion script", s.shell)
}
