package help

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// defaultRenderer implements HelpRenderer with the standard cli help format.
type defaultRenderer struct {
	opts *Options
}

// Default returns a help renderer that matches the built-in cli help output.
// It supports optional color output, sorting, and width configuration.
func Default(opts ...Option) cli.HelpRenderer {
	return &defaultRenderer{opts: applyOptions(opts)}
}

// RenderHelp implements cli.HelpRenderer.
func (r *defaultRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	var b strings.Builder
	c := NewColorizer(r.opts)

	info := ResolveInfo(cmd)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in help rendering

	// Description
	switch {
	case info.LongDescription != "":
		b.WriteString(info.LongDescription)
		b.WriteString("\n\n")
	case info.Description != "":
		b.WriteString(info.Description)
		b.WriteString("\n\n")
	}

	// Prepend sections
	if p, ok := cmd.(cli.HelpPrepender); ok {
		for _, section := range p.PrependHelp() {
			b.WriteString(c.Section(section.Header + ":"))
			b.WriteByte('\n')
			b.WriteString(section.Body)
			b.WriteString("\n\n")
		}
	}

	// Usage
	chainNames := CommandPath(chain)
	b.WriteString(c.Section("Usage:"))
	b.WriteByte('\n')

	if len(allSubs) > 0 {
		fmt.Fprintf(&b, "  %s [command]\n", chainNames)
	}
	argUsage := BuildArgUsage(args)

	if HasVisibleFlags(flags) || HasVisibleFlags(globalFlags) {
		fmt.Fprintf(&b, "  %s [flags] %s\n", chainNames, argUsage)
	} else {
		fmt.Fprintf(&b, "  %s %s\n", chainNames, argUsage)
	}

	// Examples
	if len(info.Examples) > 0 {
		b.WriteString("\n")
		b.WriteString(c.Section("Examples:"))
		b.WriteByte('\n')
		for _, ex := range info.Examples {
			if ex.Description != "" {
				fmt.Fprintf(&b, "  %s\n", ex.Description)
			}
			fmt.Fprintf(&b, "    %s\n", c.Example("$ "+ex.Command))
		}
	}

	// Subcommands (grouped by category)
	if len(allSubs) > 0 {
		r.renderSubcommands(&b, allSubs, c)
	}

	// Flags (grouped by category)
	r.renderFlags(&b, flags, c)

	// Arguments
	r.renderArgDefs(&b, args, c)

	// Append sections
	if a, ok := cmd.(cli.HelpAppender); ok {
		for _, section := range a.AppendHelp() {
			b.WriteString("\n")
			b.WriteString(c.Section(section.Header + ":"))
			b.WriteByte('\n')
			b.WriteString(section.Body)
			b.WriteByte('\n')
		}
	}

	// Global flags
	r.renderGlobalFlags(&b, globalFlags, c)

	// Footer
	if len(allSubs) > 0 {
		fmt.Fprintf(&b, "\nUse \"%s [command] --help\" for more information about a command.\n", chainNames)
	}

	return b.String()
}

func (r *defaultRenderer) renderSubcommands(b *strings.Builder, subs []cli.Commander, c *Colorizer) {
	visible := VisibleSubcommands(subs)
	if len(visible) == 0 {
		return
	}

	groups := GroupCommandsByCategory(visible)
	maxWidth := MaxCommandWidth(visible)

	if r.opts.Sorted {
		sortCommanders := func(cmds []cli.Commander) {
			slices.SortFunc(cmds, func(a, b cli.Commander) int {
				return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
			})
		}
		sortCommanders(groups.Uncategorized)
		for cat := range groups.ByCategory {
			sortCommanders(groups.ByCategory[cat])
		}
		slices.Sort(groups.Categories)
	}

	if len(groups.Uncategorized) > 0 {
		b.WriteString("\n")
		b.WriteString(c.Section("Commands:"))
		b.WriteByte('\n')
		for _, s := range groups.Uncategorized {
			info := ResolveInfo(s)
			fmt.Fprintf(b, "  %-*s  %s\n", maxWidth, c.Command(info.Name), info.Description)
		}
	}

	for _, cat := range groups.Categories {
		b.WriteString("\n")
		b.WriteString(c.Section(cat + ":"))
		b.WriteByte('\n')
		for _, s := range groups.ByCategory[cat] {
			info := ResolveInfo(s)
			fmt.Fprintf(b, "  %-*s  %s\n", maxWidth, c.Command(info.Name), info.Description)
		}
	}
}

func (r *defaultRenderer) renderFlags(b *strings.Builder, flags []cli.FlagDef, c *Colorizer) {
	visible := VisibleFlags(flags)
	if len(visible) == 0 {
		return
	}

	if r.opts.Sorted {
		slices.SortFunc(visible, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	groups := GroupFlagsByCategory(visible)
	maxLeft := MaxFlagWidth(visible)

	if len(groups.Uncategorized) > 0 {
		b.WriteString("\n")
		b.WriteString(c.Section("Flags:"))
		b.WriteByte('\n')
		r.writeFlagLines(b, groups.Uncategorized, maxLeft, c)
	}

	categories := groups.Categories
	if r.opts.Sorted {
		slices.Sort(categories)
	}

	for _, cat := range categories {
		b.WriteString("\n")
		b.WriteString(c.Section(cat + ":"))
		b.WriteByte('\n')
		r.writeFlagLines(b, groups.ByCategory[cat], maxLeft, c)
	}
}

func (r *defaultRenderer) renderGlobalFlags(b *strings.Builder, flags []cli.FlagDef, c *Colorizer) {
	visible := VisibleFlags(flags)
	if len(visible) == 0 {
		return
	}

	if r.opts.Sorted {
		slices.SortFunc(visible, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}

	maxLeft := MaxFlagWidth(visible)

	b.WriteString("\n")
	b.WriteString(c.Section("Global Flags:"))
	b.WriteByte('\n')
	r.writeFlagLines(b, visible, maxLeft, c)
}

func (r *defaultRenderer) writeFlagLines(b *strings.Builder, flags []cli.FlagDef, maxLeft int, c *Colorizer) {
	hasRequired := false
	for i := range flags {
		if flags[i].Required {
			hasRequired = true
			break
		}
	}

	for i := range flags {
		f := &flags[i]
		left := FlagLeft(f)
		right := FlagRight(f)

		// Apply colors to components.
		coloredLeft := c.Flag(left)

		if hasRequired {
			prefix := "  "
			if f.Required {
				prefix = c.Required("* ")
			}
			// Adjust width for color codes.
			padding := maxLeft - len(left)
			fmt.Fprintf(b, "%s%s%s  %s\n", prefix, coloredLeft, strings.Repeat(" ", padding), right)
		} else {
			padding := maxLeft - len(left)
			fmt.Fprintf(b, "  %s%s  %s\n", coloredLeft, strings.Repeat(" ", padding), right)
		}
	}
}

func (r *defaultRenderer) renderArgDefs(b *strings.Builder, args []cli.ArgDef, c *Colorizer) {
	if len(args) == 0 {
		return
	}

	b.WriteString("\n")
	b.WriteString(c.Section("Arguments:"))
	b.WriteByte('\n')

	maxLeft := 0
	for i := range args {
		if len(args[i].Name) > maxLeft {
			maxLeft = len(args[i].Name)
		}
	}

	for i := range args {
		a := &args[i]
		var parts []string
		parts = append(parts, InterpolateHelp(a.Help, a.Default, a.Mask, a.Enum, a.Env))
		if a.Required {
			parts = append(parts, c.Required("(required)"))
		}
		if a.Enum != "" {
			parts = append(parts, "["+strings.ReplaceAll(a.Enum, ",", "|")+"]")
		}
		switch {
		case a.Mask != "":
			parts = append(parts, c.Default("(default: "+a.Mask+")"))
		case a.Default != "":
			parts = append(parts, c.Default("(default: "+a.Default+")"))
		}
		if a.Env != "" {
			envDisplay := a.Env
			if strings.Contains(envDisplay, ",") {
				names := strings.Split(envDisplay, ",")
				for j := range names {
					names[j] = strings.TrimSpace(names[j])
				}
				envDisplay = strings.Join(names, ", ")
			}
			parts = append(parts, c.Env("(env: "+envDisplay+")"))
		}
		right := strings.Join(parts, " ")
		fmt.Fprintf(b, "  %-*s  %s\n", maxLeft, a.Name, right)
	}
}

func applyOptions(opts []Option) *Options {
	o := &Options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
