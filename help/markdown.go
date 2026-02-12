package help

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// markdownRenderer implements HelpRenderer with Markdown output.
type markdownRenderer struct {
	opts *Options
}

// Markdown returns a help renderer that produces Markdown format output.
// This is useful for generating documentation or rendering help in
// Markdown-aware environments.
//
// Example output:
//
//	# myapp serve
//
//	Start the HTTP server.
//
//	## Usage
//
//	    myapp serve [flags]
//
//	## Flags
//
//	| Flag | Type | Default | Description |
//	|------|------|---------|-------------|
//	| `-p, --port` | int | 8080 | Port to listen on |
func Markdown(opts ...Option) cli.HelpRenderer {
	return &markdownRenderer{opts: applyOptions(opts)}
}

// RenderHelp implements cli.HelpRenderer.
func (r *markdownRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	var b strings.Builder

	info := ResolveInfo(cmd)
	chainNames := CommandPath(chain)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck

	// Title
	fmt.Fprintf(&b, "# %s\n\n", chainNames)

	// Description
	if info.LongDescription != "" {
		b.WriteString(info.LongDescription)
		b.WriteString("\n\n")
	} else if info.Description != "" {
		b.WriteString(info.Description)
		b.WriteString("\n\n")
	}

	// Usage
	b.WriteString("## Usage\n\n")
	argUsage := BuildArgUsage(args)
	hasFlags := HasVisibleFlags(flags) || HasVisibleFlags(globalFlags)
	if len(allSubs) > 0 {
		fmt.Fprintf(&b, "```\n%s [command]\n```\n\n", chainNames)
	}
	if hasFlags {
		fmt.Fprintf(&b, "```\n%s [flags] %s\n```\n\n", chainNames, argUsage)
	} else if len(allSubs) == 0 {
		fmt.Fprintf(&b, "```\n%s %s\n```\n\n", chainNames, argUsage)
	}

	// Commands
	visible := VisibleSubcommands(allSubs)
	if len(visible) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visible, func(a, b cli.Commander) int {
				return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
			})
		}
		b.WriteString("## Commands\n\n")
		b.WriteString("| Command | Description |\n")
		b.WriteString("|---------|-------------|\n")
		for _, s := range visible {
			sInfo := ResolveInfo(s)
			fmt.Fprintf(&b, "| `%s` | %s |\n", sInfo.Name, escapeMarkdown(sInfo.Description))
		}
		b.WriteString("\n")
	}

	// Flags
	visibleFlags := VisibleFlags(flags)
	if len(visibleFlags) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visibleFlags, func(a, b cli.FlagDef) int {
				return strings.Compare(a.Name, b.Name)
			})
		}
		b.WriteString("## Flags\n\n")
		b.WriteString("| Flag | Type | Default | Description |\n")
		b.WriteString("|------|------|---------|-------------|\n")
		for i := range visibleFlags {
			r.writeFlagRow(&b, &visibleFlags[i])
		}
		b.WriteString("\n")
	}

	// Arguments
	if len(args) > 0 {
		b.WriteString("## Arguments\n\n")
		b.WriteString("| Argument | Type | Required | Description |\n")
		b.WriteString("|----------|------|----------|-------------|\n")
		for i := range args {
			a := &args[i]
			required := "No"
			if a.Required {
				required = "Yes"
			}
			help := InterpolateHelp(a.Help, a.Default, a.Mask, a.Enum, a.Env)
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", a.Name, a.TypeName, required, escapeMarkdown(help))
		}
		b.WriteString("\n")
	}

	// Global Flags
	visibleGlobal := VisibleFlags(globalFlags)
	if len(visibleGlobal) > 0 {
		if r.opts.Sorted {
			slices.SortFunc(visibleGlobal, func(a, b cli.FlagDef) int {
				return strings.Compare(a.Name, b.Name)
			})
		}
		b.WriteString("## Global Flags\n\n")
		b.WriteString("| Flag | Type | Default | Description |\n")
		b.WriteString("|------|------|---------|-------------|\n")
		for i := range visibleGlobal {
			r.writeFlagRow(&b, &visibleGlobal[i])
		}
		b.WriteString("\n")
	}

	// Examples
	if len(info.Examples) > 0 {
		b.WriteString("## Examples\n\n")
		for _, ex := range info.Examples {
			if ex.Description != "" {
				b.WriteString(ex.Description)
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "```bash\n%s\n```\n\n", ex.Command)
		}
	}

	// See Also
	if len(visible) > 0 {
		b.WriteString("## See Also\n\n")
		for _, s := range visible {
			sInfo := ResolveInfo(s)
			fmt.Fprintf(&b, "- `%s %s`", chainNames, sInfo.Name)
			if sInfo.Description != "" {
				fmt.Fprintf(&b, " - %s", sInfo.Description)
			}
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (r *markdownRenderer) writeFlagRow(b *strings.Builder, f *cli.FlagDef) {
	// Build flag signature.
	var sig strings.Builder
	if f.Short != "" {
		sig.WriteString("-")
		sig.WriteString(f.Short)
		sig.WriteString(", ")
	}
	sig.WriteString("--")
	sig.WriteString(f.Name)
	for _, alt := range f.Alt {
		sig.WriteString(", --")
		sig.WriteString(alt)
	}

	// Type.
	typeName := f.TypeName
	if f.IsBool {
		typeName = "bool"
	}
	if f.IsCounter {
		typeName = "counter"
	}

	// Default.
	def := f.Default
	if f.Mask != "" {
		def = f.Mask
	}
	if def == "" {
		def = "-"
	}

	// Help.
	help := InterpolateHelp(f.Help, f.Default, f.Mask, f.Enum, f.Env)
	if f.Required {
		help = "**Required.** " + help
	}
	if f.Deprecated != "" {
		help = "⚠️ DEPRECATED: " + f.Deprecated + ". " + help
	}
	if f.Env != "" {
		help += " (env: `" + f.Env + "`)"
	}

	fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", sig.String(), typeName, escapeMarkdown(def), escapeMarkdown(help))
}

// escapeMarkdown escapes special Markdown characters in table cells.
func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
