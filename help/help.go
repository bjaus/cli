// Package help provides configurable help format renderers for the cli package.
//
// Each renderer implements [cli.HelpRenderer] and can be used with
// [cli.WithHelpRenderer]:
//
//	cli.Execute(ctx, root, args, cli.WithHelpRenderer(help.Compact()))
//
// Renderers accept options for customization:
//
//	help.Default(help.WithColor(true), help.WithSorted())
//
// Available renderers:
//   - [Default] — matches the built-in cli help output
//   - [Compact] — dense, minimal whitespace
//   - [Tree] — ASCII command hierarchy
//   - [Man] — man page style with CAPS sections
//   - [JSON] — machine-readable JSON
//   - [Markdown] — Markdown format for documentation
//   - [Template] — custom Go text/template
//
// # Custom Templates
//
// The [Template] renderer allows complete control over help output using Go's
// text/template syntax. Templates receive a [HelpData] struct with all command
// metadata:
//
//	tmpl := `{{.Name}} - {{.Description}}
//	{{range .Flags}}- --{{.Name}}: {{.Help}}
//	{{end}}`
//	renderer, _ := help.Template(tmpl)
//
// Use [BuildHelpData] to access the structured help data programmatically.
package help

import (
	"reflect"
	"strings"

	"github.com/bjaus/cli"
)

// Options configures help renderer behavior.
type Options struct {
	Width     int  // terminal width (0 = auto-detect)
	Color     bool // force color output
	ColorAuto bool // auto-detect color support
	Sorted    bool // sort flags/subcommands alphabetically
}

// Option configures a help renderer.
type Option func(*Options)

// WithWidth sets the terminal width for text wrapping. A value of 0 enables
// auto-detection using the COLUMNS environment variable or terminal ioctl.
func WithWidth(w int) Option {
	return func(o *Options) { o.Width = w }
}

// WithColor enables ANSI color output.
func WithColor(enabled bool) Option {
	return func(o *Options) {
		o.Color = enabled
		if enabled {
			o.ColorAuto = false
		}
	}
}

// WithColorAuto enables automatic color detection based on terminal
// capabilities, NO_COLOR environment variable, and TERM value.
func WithColorAuto() Option {
	return func(o *Options) {
		o.ColorAuto = true
		o.Color = false
	}
}

// WithSorted sorts flags and subcommands alphabetically.
func WithSorted() Option {
	return func(o *Options) { o.Sorted = true }
}

// CommandInfo holds resolved command metadata.
type CommandInfo struct {
	Name            string
	Description     string
	LongDescription string
	Aliases         []string
	Hidden          bool
	Examples        []cli.Example
	Category        string
}

// ResolveInfo extracts metadata from a command via optional interfaces.
func ResolveInfo(cmd cli.Commander) CommandInfo {
	info := CommandInfo{}
	if n, ok := cmd.(cli.Namer); ok {
		info.Name = n.Name()
	}
	if info.Name == "" {
		info.Name = defaultName(cmd)
	}
	if d, ok := cmd.(cli.Descriptor); ok {
		info.Description = d.Description()
	}
	if ld, ok := cmd.(cli.LongDescriptor); ok {
		info.LongDescription = ld.LongDescription()
	}
	if a, ok := cmd.(cli.Aliaser); ok {
		info.Aliases = a.Aliases()
	}
	if h, ok := cmd.(cli.Hider); ok {
		info.Hidden = h.Hidden()
	}
	if e, ok := cmd.(cli.Exampler); ok {
		info.Examples = e.Examples()
	}
	if c, ok := cmd.(cli.Categorizer); ok {
		info.Category = c.Category()
	}
	return info
}

// CommandPath builds the full command path from a chain (e.g., "app sub1 sub2").
func CommandPath(chain []cli.Commander) string {
	names := make([]string, len(chain))
	for i, cmd := range chain {
		names[i] = ResolveInfo(cmd).Name
	}
	return strings.Join(names, " ")
}

// BuildArgUsage builds the argument usage string (e.g., "<required> [optional...]").
func BuildArgUsage(args []cli.ArgDef) string {
	if len(args) == 0 {
		return "[args...]"
	}
	parts := make([]string, 0, len(args))
	for i := range args {
		a := &args[i]
		switch {
		case a.IsSlice:
			parts = append(parts, "["+a.Name+"...]")
		case a.Required:
			parts = append(parts, "<"+a.Name+">")
		default:
			parts = append(parts, "["+a.Name+"]")
		}
	}
	return strings.Join(parts, " ")
}

// VisibleFlags filters out hidden flags.
func VisibleFlags(flags []cli.FlagDef) []cli.FlagDef {
	var visible []cli.FlagDef
	for i := range flags {
		if !flags[i].Hidden {
			visible = append(visible, flags[i])
		}
	}
	return visible
}

// VisibleSubcommands filters out hidden subcommands.
func VisibleSubcommands(subs []cli.Commander) []cli.Commander {
	var visible []cli.Commander
	for _, s := range subs {
		if !ResolveInfo(s).Hidden {
			visible = append(visible, s)
		}
	}
	return visible
}

// GroupedFlags groups flags by category.
type GroupedFlags struct {
	Uncategorized []cli.FlagDef
	Categories    []string
	ByCategory    map[string][]cli.FlagDef
}

// GroupFlagsByCategory groups flags by their Category field.
func GroupFlagsByCategory(flags []cli.FlagDef) GroupedFlags {
	g := GroupedFlags{
		ByCategory: make(map[string][]cli.FlagDef),
	}
	for i := range flags {
		f := &flags[i]
		if f.Category != "" {
			if _, exists := g.ByCategory[f.Category]; !exists {
				g.Categories = append(g.Categories, f.Category)
			}
			g.ByCategory[f.Category] = append(g.ByCategory[f.Category], *f)
		} else {
			g.Uncategorized = append(g.Uncategorized, *f)
		}
	}
	return g
}

// GroupedCommands groups commands by category.
type GroupedCommands struct {
	Uncategorized []cli.Commander
	Categories    []string
	ByCategory    map[string][]cli.Commander
}

// GroupCommandsByCategory groups commands by their Category field.
func GroupCommandsByCategory(subs []cli.Commander) GroupedCommands {
	g := GroupedCommands{
		ByCategory: make(map[string][]cli.Commander),
	}
	for _, s := range subs {
		info := ResolveInfo(s)
		if info.Category != "" {
			if _, exists := g.ByCategory[info.Category]; !exists {
				g.Categories = append(g.Categories, info.Category)
			}
			g.ByCategory[info.Category] = append(g.ByCategory[info.Category], s)
		} else {
			g.Uncategorized = append(g.Uncategorized, s)
		}
	}
	return g
}

// HasVisibleFlags returns true if any flag is not hidden.
func HasVisibleFlags(flags []cli.FlagDef) bool {
	for i := range flags {
		if !flags[i].Hidden {
			return true
		}
	}
	return false
}

// FlagLeft builds the left column for a flag (e.g., "-v, --verbose").
func FlagLeft(f *cli.FlagDef) string {
	var left string
	switch {
	case f.Negate && f.Short != "":
		left = "-" + f.Short + ", --[no-]" + f.Name
	case f.Negate:
		left = "    --[no-]" + f.Name
	case f.Short != "":
		left = "-" + f.Short + ", --" + f.Name
	default:
		left = "    --" + f.Name
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

// FlagRight builds the right column for a flag (description, default, etc.).
func FlagRight(f *cli.FlagDef) string {
	var parts []string
	parts = append(parts, InterpolateHelp(f.Help, f.Default, f.Mask, f.Enum, f.Env))
	if f.Deprecated != "" {
		parts = append(parts, "(DEPRECATED: "+f.Deprecated+")")
	}
	if f.Required {
		parts = append(parts, "(required)")
	}
	if f.IsCounter {
		parts = append(parts, "(repeatable)")
	}
	if f.Sep != "" {
		parts = append(parts, "(separator: \""+f.Sep+"\")")
	}
	if f.Enum != "" {
		parts = append(parts, "["+strings.ReplaceAll(f.Enum, ",", "|")+"]")
	}
	switch {
	case f.Mask != "":
		parts = append(parts, "(default: "+f.Mask+")")
	case f.Default != "":
		parts = append(parts, "(default: "+f.Default+")")
	}
	if f.Env != "" {
		envDisplay := f.Env
		if strings.Contains(envDisplay, ",") {
			names := strings.Split(envDisplay, ",")
			for i := range names {
				names[i] = strings.TrimSpace(names[i])
			}
			envDisplay = strings.Join(names, ", ")
		}
		parts = append(parts, "(env: "+envDisplay+")")
	}
	return strings.Join(parts, " ")
}

// InterpolateHelp replaces ${default}, ${enum}, and ${env} placeholders in help text.
func InterpolateHelp(help, def, mask, enum, env string) string {
	if !strings.Contains(help, "${") {
		return help
	}
	defaultVal := def
	if mask != "" {
		defaultVal = mask
	}
	help = strings.ReplaceAll(help, "${default}", defaultVal)
	help = strings.ReplaceAll(help, "${enum}", enum)
	help = strings.ReplaceAll(help, "${env}", env)
	return help
}

// MaxFlagWidth calculates the maximum left column width across flags.
func MaxFlagWidth(flags []cli.FlagDef) int {
	maxLeft := 0
	for i := range flags {
		left := FlagLeft(&flags[i])
		if len(left) > maxLeft {
			maxLeft = len(left)
		}
	}
	return maxLeft
}

// MaxCommandWidth calculates the maximum name width across commands.
func MaxCommandWidth(subs []cli.Commander) int {
	maxWidth := 0
	for _, s := range subs {
		info := ResolveInfo(s)
		if !info.Hidden && len(info.Name) > maxWidth {
			maxWidth = len(info.Name)
		}
	}
	return maxWidth
}

func defaultName(cmd cli.Commander) string {
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return strings.ToLower(t.Name())
}
