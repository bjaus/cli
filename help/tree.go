package help

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// treeRenderer implements HelpRenderer with ASCII tree format.
type treeRenderer struct {
	opts *Options
}

// Tree returns a help renderer that displays command hierarchy as an ASCII tree.
//
// Example output:
//
//	myapp
//	├── serve       Start the server
//	│   ├── --port int     Port (default: 8080)
//	│   └── --verbose      Enable verbose output
//	├── deploy      Deploy the application
//	│   ├── prod    Deploy to production
//	│   └── staging Deploy to staging
//	└── status      Show status
func Tree(opts ...Option) cli.HelpRenderer {
	return &treeRenderer{opts: applyOptions(opts)}
}

// RenderHelp implements cli.HelpRenderer.
func (r *treeRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	var b strings.Builder
	c := NewColorizer(r.opts)

	info := ResolveInfo(cmd)
	chainNames := CommandPath(chain)

	// Root command line.
	b.WriteString(c.Command(chainNames))
	if info.Description != "" {
		b.WriteString(" - ")
		b.WriteString(info.Description)
	}
	b.WriteByte('\n')

	// Collect all items to render.
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck
	visible := VisibleSubcommands(allSubs)
	if r.opts.Sorted {
		slices.SortFunc(visible, func(a, b cli.Commander) int {
			return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
		})
	}

	visibleFlags := VisibleFlags(flags)
	if r.opts.Sorted {
		slices.SortFunc(visibleFlags, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	visibleGlobal := VisibleFlags(globalFlags)
	if r.opts.Sorted {
		slices.SortFunc(visibleGlobal, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	// Calculate max widths.
	maxCmdWidth := MaxCommandWidth(visible)
	maxFlagWidth := MaxFlagWidth(visibleFlags)
	if gw := MaxFlagWidth(visibleGlobal); gw > maxFlagWidth {
		maxFlagWidth = gw
	}

	// Count total items for determining last item.
	totalItems := len(visible) + len(visibleFlags) + len(visibleGlobal)
	if len(args) > 0 {
		totalItems++
	}
	currentItem := 0

	// Render subcommands.
	for i, s := range visible {
		currentItem++
		isLast := currentItem == totalItems
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}
		sInfo := ResolveInfo(s)
		fmt.Fprintf(&b, "%s%-*s  %s\n", prefix, maxCmdWidth, c.Command(sInfo.Name), sInfo.Description)

		// Render nested subcommands recursively.
		childSubs, _ := cli.AllSubcommands(s) //nolint:errcheck
		childVisible := VisibleSubcommands(childSubs)
		if r.opts.Sorted && len(childVisible) > 0 {
			slices.SortFunc(childVisible, func(a, b cli.Commander) int {
				return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
			})
		}
		childPrefix := "│   "
		if isLast || i == len(visible)-1 {
			childPrefix = "    "
		}
		for j, cs := range childVisible {
			csInfo := ResolveInfo(cs)
			childIsLast := j == len(childVisible)-1
			childMarker := "├── "
			if childIsLast {
				childMarker = "└── "
			}
			fmt.Fprintf(&b, "%s%s%s  %s\n", childPrefix, childMarker, c.Command(csInfo.Name), csInfo.Description)
		}
	}

	// Render flags.
	for i := range visibleFlags {
		f := &visibleFlags[i]
		currentItem++
		isLast := currentItem == totalItems
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}
		left := FlagLeft(f)
		right := FlagRight(f)
		padding := maxFlagWidth - len(left)
		fmt.Fprintf(&b, "%s%s%s  %s\n", prefix, c.Flag(left), strings.Repeat(" ", padding), right)
	}

	// Render args summary.
	if len(args) > 0 {
		currentItem++
		isLast := currentItem == totalItems
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}
		argUsage := BuildArgUsage(args)
		fmt.Fprintf(&b, "%s%s %s\n", prefix, c.Section("Args:"), argUsage)
	}

	// Render global flags.
	for i := range visibleGlobal {
		f := &visibleGlobal[i]
		currentItem++
		isLast := currentItem == totalItems
		prefix := "├── "
		if isLast {
			prefix = "└── "
		}
		left := FlagLeft(f)
		right := FlagRight(f)
		padding := maxFlagWidth - len(left)
		fmt.Fprintf(&b, "%s%s%s  %s %s\n", prefix, c.Flag(left), strings.Repeat(" ", padding), right, c.Dim("(global)"))
	}

	return b.String()
}
