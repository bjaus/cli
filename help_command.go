package cli

import (
	"context"
	"fmt"
	"io"
)

// HelpCommand returns a [Commander] that displays help for a named command. It
// accepts command names as positional arguments and prints the help output for
// that command. Typically added as a hidden "help" subcommand:
//
//	func (a *App) Subcommands() []cli.Commander {
//	    return []cli.Commander{&serveCmd{}, cli.HelpCommand(a, os.Stdout)}
//	}
//
// Usage:
//
//	myapp help serve        # shows help for "serve"
//	myapp help cluster list # shows help for "cluster list"
func HelpCommand(root Commander, w io.Writer) Commander {
	return &helpCmd{root: root, out: w}
}

type helpCmd struct {
	root Commander
	out  io.Writer
	Args Args
}

func (h *helpCmd) Run(_ context.Context) error {
	target := h.root
	chain := []Commander{h.root}

	for _, name := range h.Args {
		subs, err := allSubcommands(target)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrUnknownCommand, name)
		}
		sub := findSubcommand(subs, name, false, false)
		if sub == nil {
			return fmt.Errorf("%w: %s", ErrUnknownCommand, name)
		}
		target = sub
		chain = append(chain, sub)
	}

	flags := ScanFlags(target)
	globalFlags := collectGlobalFlags(chain, flags)
	text := defaultRenderHelp(target, chain, flags, globalFlags, false)
	_, err := fmt.Fprint(h.out, text)
	return err
}

func (h *helpCmd) Name() string        { return "help" }
func (h *helpCmd) Description() string { return "Show help for a command" }
func (h *helpCmd) Hidden() bool        { return true }
