package cli

import (
	"fmt"
	"strings"
)

func defaultRenderHelp(cmd Runner, chain []Runner, flags []FlagDef) string {
	var b strings.Builder

	info := resolveInfo(cmd)

	// Description
	if info.description != "" {
		b.WriteString(info.description)
		b.WriteString("\n\n")
	}

	// Usage
	chainNames := commandChainNames(chain)
	b.WriteString("Usage:\n")

	if p, ok := cmd.(Parent); ok && len(p.Subcommands()) > 0 {
		fmt.Fprintf(&b, "  %s [command]\n", chainNames)
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, "  %s [flags] [args...]\n", chainNames)
	} else {
		fmt.Fprintf(&b, "  %s [args...]\n", chainNames)
	}

	// Examples
	if info.examples != nil {
		b.WriteString("\nExamples:\n")
		for _, ex := range info.examples {
			if ex.Description != "" {
				fmt.Fprintf(&b, "  %s\n", ex.Description)
			}
			fmt.Fprintf(&b, "    $ %s\n", ex.Command)
		}
	}

	// Subcommands
	if p, ok := cmd.(Parent); ok {
		subs := p.Subcommands()
		var visible []Runner
		for _, s := range subs {
			si := resolveInfo(s)
			if !si.hidden {
				visible = append(visible, s)
			}
		}

		if len(visible) > 0 {
			b.WriteString("\nCommands:\n")

			// Find max name width for alignment
			maxWidth := 0
			for _, s := range visible {
				si := resolveInfo(s)
				if len(si.name) > maxWidth {
					maxWidth = len(si.name)
				}
			}

			for _, s := range visible {
				si := resolveInfo(s)
				fmt.Fprintf(&b, "  %-*s  %s\n", maxWidth, si.name, si.description)
			}
		}
	}

	// Flags
	if len(flags) > 0 {
		b.WriteString("\nFlags:\n")

		type flagLine struct {
			left  string
			right string
		}
		var lines []flagLine
		maxLeft := 0

		for _, f := range flags {
			var left string
			if f.Short != "" {
				left = fmt.Sprintf("-%s, --%s", f.Short, f.Name)
			} else {
				left = fmt.Sprintf("    --%s", f.Name)
			}
			if !f.IsBool {
				left += " " + f.TypeName
			}

			var parts []string
			parts = append(parts, f.Help)
			if f.Required {
				parts = append(parts, "(required)")
			}
			if f.Default != "" {
				parts = append(parts, fmt.Sprintf("(default: %s)", f.Default))
			}
			if f.Env != "" {
				parts = append(parts, fmt.Sprintf("(env: %s)", f.Env))
			}

			right := strings.Join(parts, " ")

			if len(left) > maxLeft {
				maxLeft = len(left)
			}

			lines = append(lines, flagLine{left: left, right: right})
		}

		for _, l := range lines {
			fmt.Fprintf(&b, "  %-*s  %s\n", maxLeft, l.left, l.right)
		}
	}

	// Footer
	if p, ok := cmd.(Parent); ok && len(p.Subcommands()) > 0 {
		fmt.Fprintf(&b, "\nUse \"%s [command] --help\" for more information about a command.\n", chainNames)
	}

	return b.String()
}

func commandChainNames(chain []Runner) string {
	names := make([]string, len(chain))
	for i, cmd := range chain {
		names[i] = resolveInfo(cmd).name
	}
	return strings.Join(names, " ")
}
