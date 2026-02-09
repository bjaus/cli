package completion

import (
	"context"
	"fmt"
	"os"

	"github.com/bjaus/cli"
)

// Command returns a [cli.Runner] that prints shell completion scripts to stdout.
// It has bash, zsh, fish, and powershell subcommands. Typically added as a
// hidden "completion" subcommand:
//
//	func (a *App) Subcommands() []cli.Runner {
//	    return []cli.Runner{
//	        &serveCmd{},
//	        completion.Command(a, "myapp"),
//	    }
//	}
func Command(root cli.Runner, appName string) cli.Runner {
	return &completionCmd{root: root, appName: appName}
}

type completionCmd struct {
	root    cli.Runner
	appName string
}

func (c *completionCmd) Run(_ context.Context, _ []string) error {
	return cli.ErrShowHelp
}

func (c *completionCmd) Name() string        { return "completion" }
func (c *completionCmd) Description() string { return "Generate shell completion scripts" }

func (c *completionCmd) Subcommands() []cli.Runner {
	return []cli.Runner{
		&shellCmd{root: c.root, appName: c.appName, shell: "bash"},
		&shellCmd{root: c.root, appName: c.appName, shell: "zsh"},
		&shellCmd{root: c.root, appName: c.appName, shell: "fish"},
		&shellCmd{root: c.root, appName: c.appName, shell: "powershell"},
	}
}

type shellCmd struct {
	root    cli.Runner
	appName string
	shell   string
}

func (s *shellCmd) Run(_ context.Context, _ []string) error {
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
	_, err := fmt.Fprint(os.Stdout, script)
	return err
}

func (s *shellCmd) Name() string { return s.shell }

func (s *shellCmd) Description() string {
	return fmt.Sprintf("Generate %s completion script", s.shell)
}
