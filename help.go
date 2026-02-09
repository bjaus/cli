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

	// Subcommands (grouped by category)
	if p, ok := cmd.(Parent); ok {
		renderSubcommands(&b, p.Subcommands())
	}

	// Flags
	if len(flags) > 0 {
		b.WriteString("\nFlags:\n")

		type flagLine struct {
			left  string
			right string
		}
		lines := make([]flagLine, 0, len(flags))
		maxLeft := 0

		for i := range flags {
			f := &flags[i]
			var left string
			switch {
			case f.Negatable && f.Short != "":
				left = fmt.Sprintf("-%s, --[no-]%s", f.Short, f.Name)
			case f.Negatable:
				left = fmt.Sprintf("    --[no-]%s", f.Name)
			case f.Short != "":
				left = fmt.Sprintf("-%s, --%s", f.Short, f.Name)
			default:
				left = fmt.Sprintf("    --%s", f.Name)
			}
			if !f.IsBool && !f.IsCounter {
				left += " " + f.TypeName
			}

			var parts []string
			parts = append(parts, f.Help)
			if f.Required {
				parts = append(parts, "(required)")
			}
			if f.IsCounter {
				parts = append(parts, "(repeatable)")
			}
			if f.Enum != "" {
				parts = append(parts, fmt.Sprintf("[%s]", strings.ReplaceAll(f.Enum, ",", "|")))
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

func renderSubcommands(b *strings.Builder, subs []Runner) {
	var uncategorized []Runner
	categoryMap := make(map[string][]Runner)
	var categoryOrder []string

	for _, s := range subs {
		si := resolveInfo(s)
		if si.hidden {
			continue
		}

		if c, ok := s.(Categorizer); ok {
			cat := c.Category()
			if _, exists := categoryMap[cat]; !exists {
				categoryOrder = append(categoryOrder, cat)
			}
			categoryMap[cat] = append(categoryMap[cat], s)
		} else {
			uncategorized = append(uncategorized, s)
		}
	}

	if len(uncategorized) == 0 && len(categoryMap) == 0 {
		return
	}

	// Find max name width across all visible commands for alignment.
	maxWidth := 0
	for _, s := range subs {
		si := resolveInfo(s)
		if !si.hidden && len(si.name) > maxWidth {
			maxWidth = len(si.name)
		}
	}

	// Uncategorized commands.
	if len(uncategorized) > 0 {
		b.WriteString("\nCommands:\n")
		for _, s := range uncategorized {
			si := resolveInfo(s)
			fmt.Fprintf(b, "  %-*s  %s\n", maxWidth, si.name, si.description)
		}
	}

	// Categorized groups.
	for _, cat := range categoryOrder {
		fmt.Fprintf(b, "\n%s:\n", cat)
		for _, s := range categoryMap[cat] {
			si := resolveInfo(s)
			fmt.Fprintf(b, "  %-*s  %s\n", maxWidth, si.name, si.description)
		}
	}
}

func commandChainNames(chain []Runner) string {
	names := make([]string, len(chain))
	for i, cmd := range chain {
		names[i] = resolveInfo(cmd).name
	}
	return strings.Join(names, " ")
}
