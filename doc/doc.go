// Package doc generates documentation from a [cli.Commander] command tree.
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
func GenMarkdown(cmd cli.Commander, chain ...cli.Commander) string {
	if len(chain) == 0 {
		chain = []cli.Commander{cmd}
	}
	return genMarkdown(cmd, chain)
}

// GenMarkdownTree generates markdown files for all commands in the tree,
// writing one file per command into dir.
func GenMarkdownTree(root cli.Commander, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return walkTree(root, []cli.Commander{root}, func(cmd cli.Commander, chain []cli.Commander) error {
		name := commandPath(chain)
		filename := strings.ReplaceAll(name, " ", "_") + ".md"
		content := genMarkdown(cmd, chain)
		return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600)
	})
}

// GenManPage generates a troff-formatted man page for a single command.
func GenManPage(cmd cli.Commander, header *ManHeader, chain ...cli.Commander) string {
	if len(chain) == 0 {
		chain = []cli.Commander{cmd}
	}
	return genManPage(cmd, chain, header)
}

// GenManTree generates man page files for all commands in the tree,
// writing one file per command into dir.
func GenManTree(root cli.Commander, dir string, header *ManHeader) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	section := manSection(header)
	return walkTree(root, []cli.Commander{root}, func(cmd cli.Commander, chain []cli.Commander) error {
		name := commandPath(chain)
		filename := strings.ReplaceAll(name, " ", "-") + "." + section
		content := genManPage(cmd, chain, header)
		return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600)
	})
}

func genMarkdown(cmd cli.Commander, chain []cli.Commander) string {
	var b strings.Builder
	info := cmdInfo(cmd)
	path := commandPath(chain)

	writeMdHeader(&b, path, info)

	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in doc generation
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)

	writeMdUsage(&b, path, args, flags, allSubs)
	writeMdExamples(&b, cmd)
	writeMdSubcommands(&b, allSubs)
	writeMdFlagsSection(&b, flags)
	writeMdArgsSection(&b, args)

	return b.String()
}

func writeMdHeader(b *strings.Builder, path string, info simpleInfo) {
	fmt.Fprintf(b, "# %s\n\n", path)
	if info.description != "" {
		fmt.Fprintf(b, "%s\n\n", info.description)
	}
	if info.longDescription != "" {
		fmt.Fprintf(b, "%s\n\n", info.longDescription)
	}
}

func writeMdUsage(b *strings.Builder, path string, args []cli.ArgDef, flags []cli.FlagDef, allSubs []cli.Commander) {
	b.WriteString("## Usage\n\n")
	b.WriteString("```\n")
	if len(allSubs) > 0 {
		fmt.Fprintf(b, "%s [command]\n", path)
	}
	argUsage := mdArgUsage(args)
	if hasVisible(flags) {
		fmt.Fprintf(b, "%s [flags] %s\n", path, argUsage)
	} else {
		fmt.Fprintf(b, "%s %s\n", path, argUsage)
	}
	b.WriteString("```\n\n")
}

func writeMdExamples(b *strings.Builder, cmd cli.Commander) {
	e, ok := cmd.(cli.Exampler)
	if !ok {
		return
	}
	examples := e.Examples()
	if len(examples) == 0 {
		return
	}
	b.WriteString("## Examples\n\n")
	for _, ex := range examples {
		if ex.Description != "" {
			fmt.Fprintf(b, "%s:\n\n", ex.Description)
		}
		fmt.Fprintf(b, "```\n%s\n```\n\n", ex.Command)
	}
}

func writeMdSubcommands(b *strings.Builder, allSubs []cli.Commander) {
	visSubs := visibleSubs(allSubs)
	if len(visSubs) == 0 {
		return
	}
	b.WriteString("## Commands\n\n")
	b.WriteString("| Command | Description |\n")
	b.WriteString("|---------|-------------|\n")
	for _, s := range visSubs {
		si := cmdInfo(s)
		fmt.Fprintf(b, "| `%s` | %s |\n", si.name, si.description)
	}
	b.WriteString("\n")
}

func writeMdFlagsSection(b *strings.Builder, flags []cli.FlagDef) {
	visible := visibleFlags(flags)
	if len(visible) == 0 {
		return
	}
	b.WriteString("## Flags\n\n")
	b.WriteString("| Flag | Type | Default | Description |\n")
	b.WriteString("|------|------|---------|-------------|\n")
	for i := range visible {
		writeMdFlagRow(b, &visible[i])
	}
	b.WriteString("\n")
}

func writeMdFlagRow(b *strings.Builder, f *cli.FlagDef) {
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
	fmt.Fprintf(b, "| %s | %s | %s | %s |\n", flag, typeName, f.Default, desc)
}

func writeMdArgsSection(b *strings.Builder, args []cli.ArgDef) {
	if len(args) == 0 {
		return
	}
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
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", a.Name, a.Default, desc)
	}
	b.WriteString("\n")
}

func genManPage(cmd cli.Commander, chain []cli.Commander, header *ManHeader) string {
	var b strings.Builder
	info := cmdInfo(cmd)
	path := commandPath(chain)

	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in doc generation

	writeManHeader(&b, path, header)
	writeManName(&b, path, info)
	writeManSynopsis(&b, path, args, flags)
	writeManDescription(&b, info)
	writeManOptions(&b, flags)
	writeManCommands(&b, allSubs)

	return b.String()
}

func writeManHeader(b *strings.Builder, path string, header *ManHeader) {
	section := manSection(header)
	source := ""
	manual := ""
	if header != nil {
		source = header.Source
		manual = header.Manual
	}
	fmt.Fprintf(b, ".TH %q %q \"\" %q %q\n", strings.ToUpper(strings.ReplaceAll(path, " ", "-")), section, source, manual)
}

func writeManName(b *strings.Builder, path string, info simpleInfo) {
	b.WriteString(".SH NAME\n")
	if info.description != "" {
		fmt.Fprintf(b, "%s \\- %s\n", path, info.description)
	} else {
		fmt.Fprintf(b, "%s\n", path)
	}
}

func writeManSynopsis(b *strings.Builder, path string, args []cli.ArgDef, flags []cli.FlagDef) {
	b.WriteString(".SH SYNOPSIS\n")
	argUsage := mdArgUsage(args)
	if hasVisible(flags) {
		fmt.Fprintf(b, ".B %s\n[\\fIflags\\fR] %s\n", path, manEscape(argUsage))
	} else {
		fmt.Fprintf(b, ".B %s\n%s\n", path, manEscape(argUsage))
	}
}

func writeManDescription(b *strings.Builder, info simpleInfo) {
	switch {
	case info.longDescription != "":
		b.WriteString(".SH DESCRIPTION\n")
		fmt.Fprintf(b, "%s\n", manEscape(info.longDescription))
	case info.description != "":
		b.WriteString(".SH DESCRIPTION\n")
		fmt.Fprintf(b, "%s\n", manEscape(info.description))
	}
}

func writeManOptions(b *strings.Builder, flags []cli.FlagDef) {
	visible := visibleFlags(flags)
	if len(visible) == 0 {
		return
	}
	b.WriteString(".SH OPTIONS\n")
	for i := range visible {
		writeManFlagEntry(b, &visible[i])
	}
}

func writeManFlagEntry(b *strings.Builder, f *cli.FlagDef) {
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
	fmt.Fprintf(b, ".TP\n%s\n", flag)
	if f.Help != "" {
		fmt.Fprintf(b, "%s\n", manEscape(f.Help))
	}
	if f.Default != "" {
		fmt.Fprintf(b, "Default: %s\n", manEscape(f.Default))
	}
}

func writeManCommands(b *strings.Builder, allSubs []cli.Commander) {
	visSubs := visibleSubs(allSubs)
	if len(visSubs) == 0 {
		return
	}
	b.WriteString(".SH COMMANDS\n")
	for _, s := range visSubs {
		si := cmdInfo(s)
		fmt.Fprintf(b, ".TP\n\\fB%s\\fR\n", si.name)
		if si.description != "" {
			fmt.Fprintf(b, "%s\n", manEscape(si.description))
		}
	}
}

// helpers

type simpleInfo struct {
	name            string
	description     string
	longDescription string
}

func cmdInfo(cmd cli.Commander) simpleInfo {
	var info simpleInfo
	if n, ok := cmd.(cli.Namer); ok {
		info.name = n.Name()
	}
	if d, ok := cmd.(cli.Descriptor); ok {
		info.description = d.Description()
	}
	if ld, ok := cmd.(cli.LongDescriptor); ok {
		info.longDescription = ld.LongDescription()
	}
	return info
}

func commandPath(chain []cli.Commander) string {
	names := make([]string, len(chain))
	for i, cmd := range chain {
		if n, ok := cmd.(cli.Namer); ok {
			names[i] = n.Name()
		}
	}
	return strings.Join(names, " ")
}

func walkTree(cmd cli.Commander, chain []cli.Commander, fn func(cli.Commander, []cli.Commander) error) error {
	if err := fn(cmd, chain); err != nil {
		return err
	}
	subs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in doc generation
	for _, sub := range subs {
		if h, ok := sub.(cli.Hider); ok && h.Hidden() {
			continue
		}
		if err := walkTree(sub, append(chain, sub), fn); err != nil {
			return err
		}
	}
	return nil
}

func visibleSubs(subs []cli.Commander) []cli.Commander {
	out := make([]cli.Commander, 0, len(subs))
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
