package cli

import (
	"fmt"
	"slices"
	"strings"
)

func defaultRenderHelp(cmd Runner, chain []Runner, flags, globalFlags []FlagDef, sorted bool) string {
	var b strings.Builder

	info := resolveInfo(cmd)

	// Description — prefer long description in own help output.
	switch {
	case info.longDescription != "":
		b.WriteString(info.longDescription)
		b.WriteString("\n\n")
	case info.description != "":
		b.WriteString(info.description)
		b.WriteString("\n\n")
	}

	// Collect all subcommands (static + discovered). Errors are ignored
	// in help rendering — we show what we can.
	allSubs, _ := allSubcommands(cmd) //nolint:errcheck // best-effort in help rendering

	// Prepend sections (before Usage)
	if p, ok := cmd.(HelpPrepender); ok {
		for _, section := range p.PrependHelp() {
			fmt.Fprintf(&b, "%s:\n", section.Header)
			b.WriteString(section.Body)
			b.WriteString("\n\n")
		}
	}

	// Usage
	chainNames := commandChainNames(chain)
	b.WriteString("Usage:\n")

	if len(allSubs) > 0 {
		fmt.Fprintf(&b, "  %s [command]\n", chainNames)
	}
	argDefs := ScanArgs(cmd)
	argUsage := buildArgUsage(argDefs)

	if hasVisibleFlags(flags) || hasVisibleFlags(globalFlags) {
		fmt.Fprintf(&b, "  %s [flags] %s\n", chainNames, argUsage)
	} else {
		fmt.Fprintf(&b, "  %s %s\n", chainNames, argUsage)
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
	if len(allSubs) > 0 {
		renderSubcommands(&b, allSubs, sorted)
	}

	// Flags (grouped by category, hidden filtered)
	renderFlags(&b, flags)

	// Arguments
	renderArgDefs(&b, argDefs)

	// Append sections (after Arguments, before Global Flags)
	if a, ok := cmd.(HelpAppender); ok {
		for _, section := range a.AppendHelp() {
			fmt.Fprintf(&b, "\n%s:\n", section.Header)
			b.WriteString(section.Body)
			b.WriteByte('\n')
		}
	}

	// Global flags (from parent commands)
	renderGlobalFlags(&b, globalFlags)

	// Footer
	if len(allSubs) > 0 {
		fmt.Fprintf(&b, "\nUse \"%s [command] --help\" for more information about a command.\n", chainNames)
	}

	return b.String()
}

func renderGlobalFlags(b *strings.Builder, flags []FlagDef) {
	var visible []FlagDef
	for i := range flags {
		if !flags[i].Hidden {
			visible = append(visible, flags[i])
		}
	}
	if len(visible) == 0 {
		return
	}

	maxLeft := 0
	for i := range visible {
		left := flagLeft(&visible[i])
		if len(left) > maxLeft {
			maxLeft = len(left)
		}
	}

	b.WriteString("\nGlobal Flags:\n")
	writeFlagLines(b, visible, maxLeft)
}

func buildArgUsage(args []ArgDef) string {
	if len(args) == 0 {
		return "[args...]"
	}
	parts := make([]string, 0, len(args))
	for i := range args {
		a := &args[i]
		switch {
		case a.IsSlice:
			parts = append(parts, fmt.Sprintf("[%s...]", a.Name))
		case a.Required:
			parts = append(parts, fmt.Sprintf("<%s>", a.Name))
		default:
			parts = append(parts, fmt.Sprintf("[%s]", a.Name))
		}
	}
	return strings.Join(parts, " ")
}

func renderArgDefs(b *strings.Builder, args []ArgDef) {
	if len(args) == 0 {
		return
	}

	b.WriteString("\nArguments:\n")

	maxLeft := 0
	for i := range args {
		if len(args[i].Name) > maxLeft {
			maxLeft = len(args[i].Name)
		}
	}

	for i := range args {
		a := &args[i]
		var parts []string
		parts = append(parts, a.Help)
		if a.Required {
			parts = append(parts, "(required)")
		}
		if a.Enum != "" {
			parts = append(parts, fmt.Sprintf("[%s]", strings.ReplaceAll(a.Enum, ",", "|")))
		}
		switch {
		case a.Mask != "":
			parts = append(parts, fmt.Sprintf("(default: %s)", a.Mask))
		case a.Default != "":
			parts = append(parts, fmt.Sprintf("(default: %s)", a.Default))
		}
		if a.Env != "" {
			envDisplay := a.Env
			if strings.Contains(envDisplay, ",") {
				names := strings.Split(envDisplay, ",")
				for i := range names {
					names[i] = strings.TrimSpace(names[i])
				}
				envDisplay = strings.Join(names, ", ")
			}
			parts = append(parts, fmt.Sprintf("(env: %s)", envDisplay))
		}
		right := strings.Join(parts, " ")
		fmt.Fprintf(b, "  %-*s  %s\n", maxLeft, a.Name, right)
	}
}

func hasVisibleFlags(flags []FlagDef) bool {
	for i := range flags {
		if !flags[i].Hidden {
			return true
		}
	}
	return false
}

func renderFlags(b *strings.Builder, flags []FlagDef) {
	// Filter hidden flags.
	var visible []FlagDef
	for i := range flags {
		if !flags[i].Hidden {
			visible = append(visible, flags[i])
		}
	}

	if len(visible) == 0 {
		return
	}

	// Compute max left column width across all visible flags.
	maxLeft := 0
	for i := range visible {
		left := flagLeft(&visible[i])
		if len(left) > maxLeft {
			maxLeft = len(left)
		}
	}

	// Group by category.
	var uncategorized []FlagDef
	categoryMap := make(map[string][]FlagDef)
	var categoryOrder []string

	for i := range visible {
		f := &visible[i]
		if f.Category != "" {
			if _, exists := categoryMap[f.Category]; !exists {
				categoryOrder = append(categoryOrder, f.Category)
			}
			categoryMap[f.Category] = append(categoryMap[f.Category], *f)
		} else {
			uncategorized = append(uncategorized, *f)
		}
	}

	// Render uncategorized flags.
	if len(uncategorized) > 0 {
		b.WriteString("\nFlags:\n")
		writeFlagLines(b, uncategorized, maxLeft)
	}

	// Render categorized groups.
	for _, cat := range categoryOrder {
		fmt.Fprintf(b, "\n%s:\n", cat)
		writeFlagLines(b, categoryMap[cat], maxLeft)
	}
}

func writeFlagLines(b *strings.Builder, flags []FlagDef, maxLeft int) {
	hasRequired := false
	for i := range flags {
		if flags[i].Required {
			hasRequired = true
			break
		}
	}

	for i := range flags {
		f := &flags[i]
		left := flagLeft(f)
		right := flagRight(f)
		if hasRequired {
			prefix := "  "
			if f.Required {
				prefix = "* "
			}
			fmt.Fprintf(b, "%s%-*s  %s\n", prefix, maxLeft, left, right)
		} else {
			fmt.Fprintf(b, "  %-*s  %s\n", maxLeft, left, right)
		}
	}
}

func flagLeft(f *FlagDef) string {
	var left string
	switch {
	case f.Negate && f.Short != "":
		left = fmt.Sprintf("-%s, --[no-]%s", f.Short, f.Name)
	case f.Negate:
		left = fmt.Sprintf("    --[no-]%s", f.Name)
	case f.Short != "":
		left = fmt.Sprintf("-%s, --%s", f.Short, f.Name)
	default:
		left = fmt.Sprintf("    --%s", f.Name)
	}
	for _, alt := range f.Alt {
		left += ", --" + alt
	}
	if !f.IsBool && !f.IsCounter {
		if f.Placeholder != "" {
			left += " " + f.Placeholder
		} else {
			left += " " + f.TypeName
		}
	}
	return left
}

func flagRight(f *FlagDef) string {
	var parts []string
	parts = append(parts, f.Help)
	if f.Deprecated != "" {
		parts = append(parts, fmt.Sprintf("(DEPRECATED: %s)", f.Deprecated))
	}
	if f.Required {
		parts = append(parts, "(required)")
	}
	if f.IsCounter {
		parts = append(parts, "(repeatable)")
	}
	if f.Sep != "" {
		parts = append(parts, fmt.Sprintf("(separator: %q)", f.Sep))
	}
	if f.Enum != "" {
		parts = append(parts, fmt.Sprintf("[%s]", strings.ReplaceAll(f.Enum, ",", "|")))
	}
	switch {
	case f.Mask != "":
		parts = append(parts, fmt.Sprintf("(default: %s)", f.Mask))
	case f.Default != "":
		parts = append(parts, fmt.Sprintf("(default: %s)", f.Default))
	}
	if f.Env != "" {
		envDisplay := f.Env
		// Normalize comma-separated env vars to "A, B" display format.
		if strings.Contains(envDisplay, ",") {
			names := strings.Split(envDisplay, ",")
			for i := range names {
				names[i] = strings.TrimSpace(names[i])
			}
			envDisplay = strings.Join(names, ", ")
		}
		parts = append(parts, fmt.Sprintf("(env: %s)", envDisplay))
	}
	return strings.Join(parts, " ")
}

func sortFlags(flags []FlagDef) {
	slices.SortFunc(flags, func(a, b FlagDef) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func renderSubcommands(b *strings.Builder, subs []Runner, sorted bool) {
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

	if sorted {
		sortRunners := func(rs []Runner) {
			slices.SortFunc(rs, func(a, b Runner) int {
				return strings.Compare(resolveInfo(a).name, resolveInfo(b).name)
			})
		}
		sortRunners(uncategorized)
		for cat := range categoryMap {
			sortRunners(categoryMap[cat])
		}
		slices.Sort(categoryOrder)
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
