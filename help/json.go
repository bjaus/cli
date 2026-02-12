package help

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/bjaus/cli"
)

// jsonRenderer implements HelpRenderer with JSON output.
type jsonRenderer struct {
	opts *Options
}

// JSON returns a help renderer that produces machine-readable JSON output.
// This is useful for programmatic consumption, documentation generation,
// or integration with other tools.
func JSON(opts ...Option) cli.HelpRenderer {
	return &jsonRenderer{opts: applyOptions(opts)}
}

// CommandHelp is the JSON structure for command help.
type CommandHelp struct {
	Name            string        `json:"name"`
	Description     string        `json:"description,omitempty"`
	LongDescription string        `json:"longDescription,omitempty"`
	Aliases         []string      `json:"aliases,omitempty"`
	Usage           []string      `json:"usage"`
	Commands        []CommandJSON `json:"commands,omitempty"`
	Flags           []FlagJSON    `json:"flags,omitempty"`
	Arguments       []ArgJSON     `json:"arguments,omitempty"`
	GlobalFlags     []FlagJSON    `json:"globalFlags,omitempty"`
	Examples        []ExampleJSON `json:"examples,omitempty"`
}

// CommandJSON is the JSON structure for a subcommand.
type CommandJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Category    string   `json:"category,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
}

// FlagJSON is the JSON structure for a flag.
type FlagJSON struct {
	Name        string   `json:"name"`
	Short       string   `json:"short,omitempty"`
	Alt         []string `json:"alt,omitempty"`
	Help        string   `json:"help,omitempty"`
	Default     string   `json:"default,omitempty"`
	Env         string   `json:"env,omitempty"`
	Enum        string   `json:"enum,omitempty"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	Deprecated  string   `json:"deprecated,omitempty"`
	Category    string   `json:"category,omitempty"`
	IsBool      bool     `json:"isBool,omitempty"`
	IsCounter   bool     `json:"isCounter,omitempty"`
	Negate      bool     `json:"negate,omitempty"`
	Sep         string   `json:"sep,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}

// ArgJSON is the JSON structure for a positional argument.
type ArgJSON struct {
	Name     string `json:"name"`
	Help     string `json:"help,omitempty"`
	Default  string `json:"default,omitempty"`
	Env      string `json:"env,omitempty"`
	Enum     string `json:"enum,omitempty"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	IsSlice  bool   `json:"isSlice,omitempty"`
}

// ExampleJSON is the JSON structure for an example.
type ExampleJSON struct {
	Description string `json:"description,omitempty"`
	Command     string `json:"command"`
}

// RenderHelp implements cli.HelpRenderer.
func (r *jsonRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	info := ResolveInfo(cmd)
	chainNames := CommandPath(chain)
	allSubs, _ := cli.AllSubcommands(cmd) //nolint:errcheck

	h := CommandHelp{
		Name:            chainNames,
		Description:     info.Description,
		LongDescription: info.LongDescription,
		Aliases:         info.Aliases,
	}

	// Usage lines.
	argUsage := BuildArgUsage(args)
	hasFlags := HasVisibleFlags(flags) || HasVisibleFlags(globalFlags)
	if len(allSubs) > 0 {
		h.Usage = append(h.Usage, chainNames+" [command]")
	}
	if hasFlags {
		h.Usage = append(h.Usage, chainNames+" [flags] "+argUsage)
	} else {
		h.Usage = append(h.Usage, chainNames+" "+argUsage)
	}

	// Commands.
	visible := VisibleSubcommands(allSubs)
	if r.opts.Sorted {
		slices.SortFunc(visible, func(a, b cli.Commander) int {
			return strings.Compare(ResolveInfo(a).Name, ResolveInfo(b).Name)
		})
	}
	for _, s := range visible {
		sInfo := ResolveInfo(s)
		h.Commands = append(h.Commands, CommandJSON{
			Name:        sInfo.Name,
			Description: sInfo.Description,
			Aliases:     sInfo.Aliases,
			Category:    sInfo.Category,
			Hidden:      sInfo.Hidden,
		})
	}

	// Include hidden in JSON output for completeness.
	for _, s := range allSubs {
		sInfo := ResolveInfo(s)
		if !sInfo.Hidden {
			continue
		}
		h.Commands = append(h.Commands, CommandJSON{
			Name:        sInfo.Name,
			Description: sInfo.Description,
			Aliases:     sInfo.Aliases,
			Category:    sInfo.Category,
			Hidden:      true,
		})
	}

	// Flags (include all, not just visible).
	if r.opts.Sorted {
		slices.SortFunc(flags, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	for i := range flags {
		h.Flags = append(h.Flags, flagDefToJSON(&flags[i]))
	}

	// Arguments.
	for i := range args {
		a := &args[i]
		h.Arguments = append(h.Arguments, ArgJSON{
			Name:     a.Name,
			Help:     a.Help,
			Default:  a.Default,
			Env:      a.Env,
			Enum:     a.Enum,
			Type:     a.TypeName,
			Required: a.Required,
			IsSlice:  a.IsSlice,
		})
	}

	// Global flags.
	if r.opts.Sorted {
		slices.SortFunc(globalFlags, func(a, b cli.FlagDef) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	for i := range globalFlags {
		h.GlobalFlags = append(h.GlobalFlags, flagDefToJSON(&globalFlags[i]))
	}

	// Examples.
	for _, ex := range info.Examples {
		h.Examples = append(h.Examples, ExampleJSON{
			Description: ex.Description,
			Command:     ex.Command,
		})
	}

	// Marshal to JSON.
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return "{\"error\": \"" + err.Error() + "\"}"
	}
	return string(data) + "\n"
}

func flagDefToJSON(f *cli.FlagDef) FlagJSON {
	return FlagJSON{
		Name:        f.Name,
		Short:       f.Short,
		Alt:         f.Alt,
		Help:        f.Help,
		Default:     f.Default,
		Env:         f.Env,
		Enum:        f.Enum,
		Type:        f.TypeName,
		Required:    f.Required,
		Hidden:      f.Hidden,
		Deprecated:  f.Deprecated,
		Category:    f.Category,
		IsBool:      f.IsBool,
		IsCounter:   f.IsCounter,
		Negate:      f.Negate,
		Sep:         f.Sep,
		Placeholder: f.Placeholder,
	}
}
