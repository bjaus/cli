package help

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// compactRenderer implements HelpRenderer with minimal whitespace.
type compactRenderer struct {
	opts *Options
}

// Compact returns a help renderer that produces dense, minimal output.
//
// Example output:
//
//	myapp serve - Start the HTTP server
//	Usage: myapp serve [flags]
//	Commands: start, stop, status
//	Flags:
//	  -p, --port int     Port (default: 8080)
//	  -v, --verbose      Enable verbose output
func Compact(opts ...Option) cli.HelpRenderer {
	return &compactRenderer{opts: applyOptions(opts)}
}

// RenderHelp implements cli.HelpRenderer.
func (r *compactRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	var b strings.Builder
	c := NewColorizer(r.opts)

	info := ResolveInfo(cmd)
	chainNames := CommandPath(chain)

	// Title line: "myapp serve - description"
	if info.Description != "" {
		b.WriteString(c.Command(chainNames))
		b.WriteString(" - ")
		b.WriteString(info.Description)
	} else {
		b.WriteString(c.Command(chainNames))
	}
	b.WriteByte('\n')

	// Usage line
	argUsage := BuildArgUsage(args)
	hasFlags := HasVisibleFlags(flags) || HasVisibleFlags(globalFlags)
	if hasFlags {
		fmt.Fprintf(&b, "%s %s [flags] %s\n", c.Section("Usage:"), chainNames, argUsage)
	} else {
		fmt.Fprintf(&b, "%s %s %s\n", c.Section("Usage:"), chainNames, argUsage)
	}

	// Subcommands (single line)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in help rendering
	visible := VisibleSubcommands(allSubs)
	if len(visible) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visible, func(a, b cli.Commander) int {
				return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
			})
		}
		names := make([]string, len(visible))
		for i, s := range visible {
			names[i] = ResolveInfo(s).Name
		}
		fmt.Fprintf(&b, "%s %s\n", c.Section("Commands:"), strings.Join(names, ", "))
	}

	// Flags
	visibleFlags := VisibleFlags(flags)
	if len(visibleFlags) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visibleFlags, func(a, b cli.FlagDef) int {
				return strings.Compare(a.Name, b.Name)
			})
		}
		b.WriteString(c.Section("Flags:"))
		b.WriteByte('\n')
		maxLeft := MaxFlagWidth(visibleFlags)
		for i := range visibleFlags {
			f := &visibleFlags[i]
			left := FlagLeft(f)
			right := FlagRight(f)
			padding := maxLeft - len(left)
			fmt.Fprintf(&b, "  %s%s  %s\n", c.Flag(left), strings.Repeat(" ", padding), right)
		}
	}

	// Global flags
	visibleGlobal := VisibleFlags(globalFlags)
	if len(visibleGlobal) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visibleGlobal, func(a, b cli.FlagDef) int {
				return strings.Compare(a.Name, b.Name)
			})
		}
		b.WriteString(c.Section("Global:"))
		b.WriteByte('\n')
		maxLeft := MaxFlagWidth(visibleGlobal)
		for i := range visibleGlobal {
			f := &visibleGlobal[i]
			left := FlagLeft(f)
			right := FlagRight(f)
			padding := maxLeft - len(left)
			fmt.Fprintf(&b, "  %s%s  %s\n", c.Flag(left), strings.Repeat(" ", padding), right)
		}
	}

	return b.String()
}
