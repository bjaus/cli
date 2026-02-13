package help

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// manRenderer implements HelpRenderer with man page style.
type manRenderer struct {
	opts *Options
}

// Man returns a help renderer that produces man page style output.
//
// Example output:
//
//	NAME
//	    myapp serve - Start the HTTP server
//
//	SYNOPSIS
//	    myapp serve [flags]
//
//	DESCRIPTION
//	    Extended description here.
//
//	OPTIONS
//	    -p, --port int
//	        Port to listen on. Default: 8080
func Man(opts ...Option) cli.HelpRenderer {
	return &manRenderer{opts: applyOptions(opts)}
}

// RenderHelp implements cli.HelpRenderer.
func (r *manRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	var b strings.Builder
	c := NewColorizer(r.opts)
	width := ResolveWidth(r.opts)

	info := ResolveInfo(cmd)
	chainNames := CommandPath(chain)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck // best-effort in help rendering
	visible := VisibleSubcommands(allSubs)

	r.writeManName(&b, c, chainNames, info)
	r.writeManSynopsis(&b, c, chainNames, args, flags, globalFlags, allSubs)
	r.writeManDescription(&b, c, info, width)
	r.writeManCommands(&b, c, visible)
	r.writeManOptions(&b, c, "OPTIONS", flags, width)
	r.writeManArguments(&b, c, args)
	r.writeManOptions(&b, c, "GLOBAL OPTIONS", globalFlags, width)
	r.writeManExamples(&b, c, info)
	r.writeManSeeAlso(&b, c, chainNames, visible)

	return b.String()
}

func (r *manRenderer) writeManName(b *strings.Builder, c *Colorizer, chainNames string, info CommandInfo) {
	b.WriteString(c.Section("NAME"))
	b.WriteByte('\n')
	if info.Description != "" {
		fmt.Fprintf(b, "    %s - %s\n", chainNames, info.Description)
	} else {
		fmt.Fprintf(b, "    %s\n", chainNames)
	}
	b.WriteByte('\n')
}

func (r *manRenderer) writeManSynopsis(b *strings.Builder, c *Colorizer, chainNames string, args []cli.ArgDef, flags, globalFlags []cli.FlagDef, allSubs []cli.Commander) {
	b.WriteString(c.Section("SYNOPSIS"))
	b.WriteByte('\n')
	argUsage := BuildArgUsage(args)
	hasFlags := HasVisibleFlags(flags) || HasVisibleFlags(globalFlags)
	if len(allSubs) > 0 {
		fmt.Fprintf(b, "    %s [command]\n", chainNames)
	}
	if hasFlags {
		fmt.Fprintf(b, "    %s [options] %s\n", chainNames, argUsage)
	} else {
		fmt.Fprintf(b, "    %s %s\n", chainNames, argUsage)
	}
	b.WriteByte('\n')
}

func (r *manRenderer) writeManDescription(b *strings.Builder, c *Colorizer, info CommandInfo, width int) {
	desc := info.LongDescription
	if desc == "" {
		desc = info.Description
	}
	if desc == "" {
		return
	}
	b.WriteString(c.Section("DESCRIPTION"))
	b.WriteByte('\n')
	for _, line := range strings.Split(desc, "\n") {
		wrapped := Wrap("    "+line, width)
		b.WriteString(wrapped)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
}

func (r *manRenderer) writeManCommands(b *strings.Builder, c *Colorizer, visible []cli.Commander) {
	if len(visible) == 0 {
		return
	}
	if r.opts.Sorted {
		slices.SortFunc(visible, func(a, b cli.Commander) int {
			return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
		})
	}
	b.WriteString(c.Section("COMMANDS"))
	b.WriteByte('\n')
	for _, s := range visible {
		sInfo := ResolveInfo(s)
		fmt.Fprintf(b, "    %s\n", c.Command(sInfo.Name))
		if sInfo.Description != "" {
			fmt.Fprintf(b, "        %s\n", sInfo.Description)
		}
		b.WriteByte('\n')
	}
}

func (r *manRenderer) writeManOptions(b *strings.Builder, c *Colorizer, section string, flags []cli.FlagDef, width int) {
	visibleFlags := VisibleFlags(flags)
	if len(visibleFlags) == 0 {
		return
	}
	if r.opts.Sorted {
		slices.SortFunc(visibleFlags, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	b.WriteString(c.Section(section))
	b.WriteByte('\n')
	for i := range visibleFlags {
		r.writeManFlag(b, &visibleFlags[i], c, width)
	}
}

func (r *manRenderer) writeManArguments(b *strings.Builder, c *Colorizer, args []cli.ArgDef) {
	if len(args) == 0 {
		return
	}
	b.WriteString(c.Section("ARGUMENTS"))
	b.WriteByte('\n')
	for i := range args {
		r.writeManArg(b, &args[i])
	}
}

func (r *manRenderer) writeManArg(b *strings.Builder, a *cli.ArgDef) {
	fmt.Fprintf(b, "    %s\n", a.Name)
	if a.Help != "" {
		help := InterpolateHelp(a.Help, a.Default, a.Mask, a.Enum, a.Env)
		fmt.Fprintf(b, "        %s\n", help)
	}
	var meta []string
	if a.Required {
		meta = append(meta, "Required")
	}
	if a.Default != "" {
		meta = append(meta, "Default: "+a.Default)
	}
	if a.Env != "" {
		meta = append(meta, "Env: "+a.Env)
	}
	if len(meta) > 0 {
		fmt.Fprintf(b, "        %s\n", strings.Join(meta, ". "))
	}
	b.WriteByte('\n')
}

func (r *manRenderer) writeManExamples(b *strings.Builder, c *Colorizer, info CommandInfo) {
	if len(info.Examples) == 0 {
		return
	}
	b.WriteString(c.Section("EXAMPLES"))
	b.WriteByte('\n')
	for _, ex := range info.Examples {
		if ex.Description != "" {
			fmt.Fprintf(b, "    %s\n", ex.Description)
		}
		fmt.Fprintf(b, "        $ %s\n", ex.Command)
		b.WriteByte('\n')
	}
}

func (r *manRenderer) writeManSeeAlso(b *strings.Builder, c *Colorizer, chainNames string, visible []cli.Commander) {
	if len(visible) == 0 {
		return
	}
	b.WriteString(c.Section("SEE ALSO"))
	b.WriteByte('\n')
	names := make([]string, len(visible))
	for i, s := range visible {
		names[i] = chainNames + " " + ResolveInfo(s).Name
	}
	fmt.Fprintf(b, "    %s\n", strings.Join(names, ", "))
}

func (r *manRenderer) writeManFlag(b *strings.Builder, f *cli.FlagDef, c *Colorizer, width int) {
	// Flag signature line.
	var sig strings.Builder
	if f.Short != "" {
		sig.WriteString("-")
		sig.WriteString(f.Short)
		sig.WriteString(", ")
	}
	sig.WriteString("--")
	sig.WriteString(f.Name)
	if f.Negate {
		sig.WriteString(", --no-")
		sig.WriteString(f.Name)
	}
	for _, alt := range f.Alt {
		sig.WriteString(", --")
		sig.WriteString(alt)
	}
	if !f.IsBool && !f.IsCounter {
		sig.WriteByte(' ')
		if f.Placeholder != "" {
			sig.WriteString(f.Placeholder)
		} else {
			sig.WriteString(f.TypeName)
		}
	}
	fmt.Fprintf(b, "    %s\n", c.Flag(sig.String()))

	// Help text.
	if f.Help != "" {
		help := InterpolateHelp(f.Help, f.Default, f.Mask, f.Enum, f.Env)
		wrapped := Wrap("        "+help, width)
		b.WriteString(wrapped)
		b.WriteByte('\n')
	}

	// Metadata.
	var meta []string
	if f.Required {
		meta = append(meta, c.Required("Required"))
	}
	if f.Deprecated != "" {
		meta = append(meta, "DEPRECATED: "+f.Deprecated)
	}
	if f.Enum != "" {
		meta = append(meta, "Values: "+strings.ReplaceAll(f.Enum, ",", ", "))
	}
	switch {
	case f.Mask != "":
		meta = append(meta, c.Default("Default: "+f.Mask))
	case f.Default != "":
		meta = append(meta, c.Default("Default: "+f.Default))
	}
	if f.Env != "" {
		meta = append(meta, c.Env("Env: "+f.Env))
	}
	if len(meta) > 0 {
		fmt.Fprintf(b, "        %s\n", strings.Join(meta, ". "))
	}
	b.WriteByte('\n')
}
