// Package doc generates documentation from a [cli.Runner] command tree.
//
// It supports two output formats:
//
//   - Markdown — suitable for websites, READMEs, and wikis
//   - Man pages — troff-formatted manual pages for Unix systems
//
// Both generators walk the full command tree recursively. Single-command
// functions return a string; tree functions write one file per command
// into a directory.
//
// # Markdown
//
//	doc.GenMarkdown(root)            // single command
//	doc.GenMarkdownTree(root, "docs/") // full tree
//
// # Man Pages
//
//	header := &doc.ManHeader{Section: "1", Source: "myapp"}
//	doc.GenManPage(root, header)           // single command
//	doc.GenManTree(root, "man/", header)   // full tree
package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bjaus/cli"
)

// ManHeader contains metadata for the man page header.
type ManHeader struct {
	Section string // man section (default "1")
	Source  string // source project name
	Manual  string // manual title
}

// GenMarkdown generates a markdown document for a single command.
// The chain parameter provides parent context for breadcrumb-style headings.
func GenMarkdown(cmd cli.Runner, chain ...cli.Runner) string {
	if len(chain) == 0 {
		chain = []cli.Runner{cmd}
	}
	return genMarkdown(cmd, chain)
}

// GenMarkdownTree generates markdown files for all commands in the tree,
// writing one file per command into dir.
func GenMarkdownTree(root cli.Runner, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {		return err
	}
	return walkTree(root, []cli.Runner{root}, func(cmd cli.Runner, chain []cli.Runner) error {
		name := commandPath(chain)
		filename := strings.ReplaceAll(name, " ", "_") + ".md"
		content := genMarkdown(cmd, chain)
		return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600)	})
}

// GenManPage generates a troff-formatted man page for a single command.
func GenManPage(cmd cli.Runner, header *ManHeader, chain ...cli.Runner) string {
	if len(chain) == 0 {
		chain = []cli.Runner{cmd}
	}
	return genManPage(cmd, chain, header)
}

// GenManTree generates man page files for all commands in the tree,
// writing one file per command into dir.
func GenManTree(root cli.Runner, dir string, header *ManHeader) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {		return err
	}
	section := manSection(header)
	return walkTree(root, []cli.Runner{root}, func(cmd cli.Runner, chain []cli.Runner) error {
		name := commandPath(chain)
		filename := strings.ReplaceAll(name, " ", "-") + "." + section
		content := genManPage(cmd, chain, header)
		return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600)	})
}

func genMarkdown(cmd cli.Runner, chain []cli.Runner) string {
	var b strings.Builder
	info := cmdInfo(cmd)
	path := commandPath(chain)

	// Title
	fmt.Fprintf(&b, "# %s\n\n", path)

	// Description
	if info.description != "" {
		fmt.Fprintf(&b, "%s\n\n", info.description)
	}

	// Usage
	b.WriteString("## Usage\n\n")
	b.WriteString("```\n")
	if p, ok := cmd.(cli.Parent); ok && len(p.Subcommands()) > 0 {
		fmt.Fprintf(&b, "%s [command]\n", path)
	}
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	argUsage := mdArgUsage(args)
	if hasVisible(flags) {
		fmt.Fprintf(&b, "%s [flags] %s\n", path, argUsage)
	} else {
		fmt.Fprintf(&b, "%s %s\n", path, argUsage)
	}
	b.WriteString("```\n\n")

	// Examples
	if e, ok := cmd.(cli.Exampler); ok {
		examples := e.Examples()
		if len(examples) > 0 {
			b.WriteString("## Examples\n\n")
			for _, ex := range examples {
				if ex.Description != "" {
					fmt.Fprintf(&b, "%s:\n\n", ex.Description)
				}
				fmt.Fprintf(&b, "```\n%s\n```\n\n", ex.Command)
			}
		}
	}

	// Subcommands
	if p, ok := cmd.(cli.Parent); ok {
		subs := visibleSubs(p.Subcommands())
		if len(subs) > 0 {
			b.WriteString("## Commands\n\n")
			b.WriteString("| Command | Description |\n")
			b.WriteString("|---------|-------------|\n")
			for _, s := range subs {
				si := cmdInfo(s)
				fmt.Fprintf(&b, "| `%s` | %s |\n", si.name, si.description)
			}
			b.WriteString("\n")
		}
	}

	// Flags
	visible := visibleFlags(flags)
	if len(visible) > 0 {
		b.WriteString("## Flags\n\n")
		b.WriteString("| Flag | Type | Default | Description |\n")
		b.WriteString("|------|------|---------|-------------|\n")
		for i := range visible {
			f := &visible[i]
			flag := "`--" + f.Name + "`"
			if f.Short != "" {
				flag = "`-" + f.Short + "`, " + flag
			}
			for _, alt := range f.Alt {
				flag += ", `--" + alt + "`"
			}
			typeName := f.TypeName
			if f.IsBool || f.IsCounter {
				typeName = ""
			}
			desc := f.Help
			if f.Deprecated != "" {
				desc += " **(DEPRECATED: " + f.Deprecated + ")**"
			}
			if f.Required {
				desc += " **(required)**"
			}
			if f.Enum != "" {
				desc += " [" + strings.ReplaceAll(f.Enum, ",", "\\|") + "]"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", flag, typeName, f.Default, desc)
		}
		b.WriteString("\n")
	}

	// Arguments
	if len(args) > 0 {
		b.WriteString("## Arguments\n\n")
		b.WriteString("| Argument | Default | Description |\n")
		b.WriteString("|----------|---------|-------------|\n")
		for i := range args {
			a := &args[i]
			desc := a.Help
			if a.Required {
				desc += " **(required)**"
			}
			if a.Enum != "" {
				desc += " [" + strings.ReplaceAll(a.Enum, ",", "\\|") + "]"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", a.Name, a.Default, desc)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func genManPage(cmd cli.Runner, chain []cli.Runner, header *ManHeader) string {
	var b strings.Builder
	info := cmdInfo(cmd)
	path := commandPath(chain)
	section := manSection(header)

	// Header
	source := ""
	manual := ""
	if header != nil {
		source = header.Source
		manual = header.Manual
	}
	fmt.Fprintf(&b, ".TH %q %q \"\" %q %q\n", strings.ToUpper(strings.ReplaceAll(path, " ", "-")), section, source, manual)

	// Name
	b.WriteString(".SH NAME\n")
	if info.description != "" {
		fmt.Fprintf(&b, "%s \\- %s\n", path, info.description)
	} else {
		fmt.Fprintf(&b, "%s\n", path)
	}

	// Synopsis
	b.WriteString(".SH SYNOPSIS\n")
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	argUsage := mdArgUsage(args)
	if hasVisible(flags) {
		fmt.Fprintf(&b, ".B %s\n[\\fIflags\\fR] %s\n", path, manEscape(argUsage))
	} else {
		fmt.Fprintf(&b, ".B %s\n%s\n", path, manEscape(argUsage))
	}

	// Description
	if info.description != "" {
		b.WriteString(".SH DESCRIPTION\n")
		fmt.Fprintf(&b, "%s\n", manEscape(info.description))
	}

	// Options
	visible := visibleFlags(flags)
	if len(visible) > 0 {
		b.WriteString(".SH OPTIONS\n")
		for i := range visible {
			f := &visible[i]
			var flag string
			if f.Short != "" {
				flag = fmt.Sprintf("\\fB-%s\\fR, \\fB--%s\\fR", f.Short, f.Name)
			} else {
				flag = fmt.Sprintf("\\fB--%s\\fR", f.Name)
			}
			for _, alt := range f.Alt {
				flag += fmt.Sprintf(", \\fB--%s\\fR", alt)
			}
			if !f.IsBool && !f.IsCounter {
				flag += " \\fI" + f.TypeName + "\\fR"
			}
			fmt.Fprintf(&b, ".TP\n%s\n", flag)
			if f.Help != "" {
				fmt.Fprintf(&b, "%s\n", manEscape(f.Help))
			}
			if f.Default != "" {
				fmt.Fprintf(&b, "Default: %s\n", manEscape(f.Default))
			}
		}
	}

	// Subcommands
	if p, ok := cmd.(cli.Parent); ok {
		subs := visibleSubs(p.Subcommands())
		if len(subs) > 0 {
			b.WriteString(".SH COMMANDS\n")
			for _, s := range subs {
				si := cmdInfo(s)
				fmt.Fprintf(&b, ".TP\n\\fB%s\\fR\n", si.name)
				if si.description != "" {
					fmt.Fprintf(&b, "%s\n", manEscape(si.description))
				}
			}
		}
	}

	return b.String()
}

// helpers

type simpleInfo struct {
	name        string
	description string
}

func cmdInfo(cmd cli.Runner) simpleInfo {
	var info simpleInfo
	if n, ok := cmd.(cli.Namer); ok {
		info.name = n.Name()
	}
	if d, ok := cmd.(cli.Describer); ok {
		info.description = d.Description()
	}
	return info
}

func commandPath(chain []cli.Runner) string {
	names := make([]string, len(chain))
	for i, cmd := range chain {
		if n, ok := cmd.(cli.Namer); ok {
			names[i] = n.Name()
		}
	}
	return strings.Join(names, " ")
}

func walkTree(cmd cli.Runner, chain []cli.Runner, fn func(cli.Runner, []cli.Runner) error) error {
	if err := fn(cmd, chain); err != nil {
		return err
	}
	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			if err := walkTree(sub, append(chain, sub), fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func visibleSubs(subs []cli.Runner) []cli.Runner {
	out := make([]cli.Runner, 0, len(subs))
	for _, s := range subs {
		if h, ok := s.(cli.Hider); ok && h.Hidden() {
			continue
		}
		out = append(out, s)
	}
	return out
}

func visibleFlags(flags []cli.FlagDef) []cli.FlagDef {
	var out []cli.FlagDef
	for i := range flags {
		if !flags[i].Hidden {
			out = append(out, flags[i])
		}
	}
	return out
}

func hasVisible(flags []cli.FlagDef) bool {
	for i := range flags {
		if !flags[i].Hidden {
			return true
		}
	}
	return false
}

func mdArgUsage(args []cli.ArgDef) string {
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

func manSection(header *ManHeader) string {
	if header != nil && header.Section != "" {
		return header.Section
	}
	return "1"
}

func manEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "-", "\\-")
	return s
}
