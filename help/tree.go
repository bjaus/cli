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

	r.writeTreeRoot(&b, c, chainNames, info)

	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in help rendering
	visible := r.sortedSubcommands(allSubs)
	visibleFlags := r.sortedFlags(flags)
	visibleGlobal := r.sortedFlags(globalFlags)

	maxCmdWidth := MaxCommandWidth(visible)
	maxFlagWidth := maxFlagWidthOf(visibleFlags, visibleGlobal)

	totalItems := len(visible) + len(visibleFlags) + len(visibleGlobal)
	if len(args) > 0 {
		totalItems++
	}
	currentItem := 0

	r.writeTreeSubcommands(&b, c, visible, maxCmdWidth, &currentItem, totalItems)
	r.writeTreeFlags(&b, c, visibleFlags, maxFlagWidth, &currentItem, totalItems, false)
	r.writeTreeArgs(&b, c, args, &currentItem, totalItems)
	r.writeTreeFlags(&b, c, visibleGlobal, maxFlagWidth, &currentItem, totalItems, true)

	return b.String()
}

func (r *treeRenderer) writeTreeRoot(b *strings.Builder, c *Colorizer, chainNames string, info CommandInfo) {
	b.WriteString(c.Command(chainNames))
	if info.Description != "" {
		b.WriteString(" - ")
		b.WriteString(info.Description)
	}
	b.WriteByte('\n')
}

func (r *treeRenderer) sortedSubcommands(allSubs []cli.Commander) []cli.Commander {
	visible := VisibleSubcommands(allSubs)
	if r.opts.Sorted && len(visible) > 0 {
		slices.SortFunc(visible, func(a, b cli.Commander) int {
			return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
		})
	}
	return visible
}

func (r *treeRenderer) sortedFlags(flags []cli.FlagDef) []cli.FlagDef {
	visible := VisibleFlags(flags)
	if r.opts.Sorted && len(visible) > 0 {
		slices.SortFunc(visible, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	return visible
}

func maxFlagWidthOf(visibleFlags, visibleGlobal []cli.FlagDef) int {
	maxFlagWidth := MaxFlagWidth(visibleFlags)
	if gw := MaxFlagWidth(visibleGlobal); gw > maxFlagWidth {
		maxFlagWidth = gw
	}
	return maxFlagWidth
}

func (r *treeRenderer) writeTreeSubcommands(b *strings.Builder, c *Colorizer, visible []cli.Commander, maxCmdWidth int, currentItem *int, totalItems int) {
	for i, s := range visible {
		*currentItem++
		prefix := treePrefix(*currentItem == totalItems)
		sInfo := ResolveInfo(s)
		fmt.Fprintf(b, "%s%-*s  %s\n", prefix, maxCmdWidth, c.Command(sInfo.Name), sInfo.Description)

		r.writeTreeChildSubcommands(b, c, s, i, len(visible), *currentItem == totalItems)
	}
}

func (r *treeRenderer) writeTreeChildSubcommands(b *strings.Builder, c *Colorizer, s cli.Commander, idx, total int, isLastParent bool) {
	childSubs, _ := cli.AllSubcommands(s) //nolint:errcheck // best-effort in help rendering
	childVisible := r.sortedSubcommands(childSubs)
	childPrefix := "│   "
	if isLastParent || idx == total-1 {
		childPrefix = "    "
	}
	for j, cs := range childVisible {
		csInfo := ResolveInfo(cs)
		childMarker := treePrefix(j == len(childVisible)-1)
		fmt.Fprintf(b, "%s%s%s  %s\n", childPrefix, childMarker, c.Command(csInfo.Name), csInfo.Description)
	}
}

func (r *treeRenderer) writeTreeFlags(b *strings.Builder, c *Colorizer, flags []cli.FlagDef, maxFlagWidth int, currentItem *int, totalItems int, isGlobal bool) {
	for i := range flags {
		f := &flags[i]
		*currentItem++
		prefix := treePrefix(*currentItem == totalItems)
		left := FlagLeft(f)
		right := FlagRight(f)
		padding := maxFlagWidth - len(left)
		if isGlobal {
			fmt.Fprintf(b, "%s%s%s  %s %s\n", prefix, c.Flag(left), strings.Repeat(" ", padding), right, c.Dim("(global)"))
		} else {
			fmt.Fprintf(b, "%s%s%s  %s\n", prefix, c.Flag(left), strings.Repeat(" ", padding), right)
		}
	}
}

func (r *treeRenderer) writeTreeArgs(b *strings.Builder, c *Colorizer, args []cli.ArgDef, currentItem *int, totalItems int) {
	if len(args) == 0 {
		return
	}
	*currentItem++
	prefix := treePrefix(*currentItem == totalItems)
	argUsage := BuildArgUsage(args)
	fmt.Fprintf(b, "%s%s %s\n", prefix, c.Section("Args:"), argUsage)
}

func treePrefix(isLast bool) string {
	if isLast {
		return "└── "
	}
	return "├── "
}
