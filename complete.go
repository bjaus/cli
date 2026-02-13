package cli

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
)

// ShellCompDirective controls shell behavior after completing.
type ShellCompDirective int

const (
	// ShellCompDirectiveDefault indicates default completion behavior.
	ShellCompDirectiveDefault ShellCompDirective = 0
	// ShellCompDirectiveNoSpace prevents adding a space after the completion.
	ShellCompDirectiveNoSpace ShellCompDirective = 1 << iota
	// ShellCompDirectiveNoFileComp disables file completion when no candidates match.
	ShellCompDirectiveNoFileComp
	// ShellCompDirectiveError indicates an error occurred during completion.
	ShellCompDirectiveError
	// ShellCompDirectiveFilterFileExt indicates completions are file extensions
	// to filter by (e.g. ".yaml", ".json"). Shells will only show files matching
	// these extensions.
	ShellCompDirectiveFilterFileExt
	// ShellCompDirectiveFilterDirs indicates only directories should be completed.
	ShellCompDirectiveFilterDirs
)

// Completion represents a single shell completion candidate.
type Completion struct {
	Value       string // the completion value
	Description string // optional description shown in shell
}

// CompletionResult holds completion candidates and metadata.
// Returned by Completer.Complete and FlagCompleter.CompleteFlag.
type CompletionResult struct {
	Completions []Completion       // completion candidates
	ActiveHelp  []string           // contextual hints shown during completion
	Directive   ShellCompDirective // controls shell behavior
}

// Completions returns a CompletionResult with the given values.
// Use for simple completions without descriptions.
//
//	return cli.Completions("foo", "bar", "baz")
func Completions(values ...string) CompletionResult {
	comps := make([]Completion, len(values))
	for i, v := range values {
		comps[i] = Completion{Value: v}
	}
	return CompletionResult{
		Completions: comps,
		Directive:   ShellCompDirectiveNoFileComp,
	}
}

// CompletionsWithDesc returns a CompletionResult with the given completions.
// Use for completions with descriptions.
//
//	return cli.CompletionsWithDesc(
//	    cli.Completion{Value: "us-east-1", Description: "N. Virginia"},
//	    cli.Completion{Value: "us-west-2", Description: "Oregon"},
//	)
func CompletionsWithDesc(comps ...Completion) CompletionResult {
	return CompletionResult{
		Completions: comps,
		Directive:   ShellCompDirectiveNoFileComp,
	}
}

// NoCompletions returns an empty CompletionResult that falls through to
// default completion behavior (e.g., file completion).
func NoCompletions() CompletionResult {
	return CompletionResult{
		Directive: ShellCompDirectiveDefault,
	}
}

// WithDirective returns a copy of the result with the given directive.
func (r CompletionResult) WithDirective(d ShellCompDirective) CompletionResult {
	r.Directive = d
	return r
}

// WithActiveHelp returns a copy of the result with active help messages.
// Active help messages are displayed by the shell during completion to provide
// contextual guidance. Supported by bash (4.4+), zsh, and fish.
//
//	return cli.Completions("dev", "staging", "prod").
//	    WithActiveHelp("Select deployment target")
func (r CompletionResult) WithActiveHelp(messages ...string) CompletionResult {
	r.ActiveHelp = append(r.ActiveHelp, messages...)
	return r
}

// FlagNameFor returns the flag name for a struct field, enabling type-safe
// flag references in CompleteFlag implementations. Pass a pointer to the
// command and a pointer to the specific field.
//
//	func (c *DeployCmd) CompleteFlag(ctx context.Context, flag, value string) cli.CompletionResult {
//	    if flag == cli.FlagNameFor(c, &c.Region) {
//	        return cli.Completions("us-east-1", "us-west-2")
//	    }
//	    return cli.NoCompletions()
//	}
//
// Returns empty string if the field is not found or has no flag tag.
func FlagNameFor[T any](cmd *T, fieldPtr any) string {
	cmdVal := reflect.ValueOf(cmd).Elem()
	cmdType := cmdVal.Type()

	// Get the address the fieldPtr points to.
	fieldVal := reflect.ValueOf(fieldPtr)
	if fieldVal.Kind() != reflect.Ptr {
		return ""
	}
	fieldAddr := fieldVal.Pointer()

	// Find which field matches this address.
	for i := range cmdType.NumField() {
		if cmdVal.Field(i).CanAddr() && cmdVal.Field(i).Addr().Pointer() == fieldAddr {
			field := cmdType.Field(i)
			if flagName := field.Tag.Get("flag"); flagName != "" {
				return flagName
			}
			// Fall back to lowercase field name if no flag tag.
			return strings.ToLower(field.Name)
		}
	}
	return ""
}

// activeHelpPrefix is prepended to active help messages. Shells recognize
// this prefix and display the message as guidance rather than a completion.
const activeHelpPrefix = "_activeHelp_ "

// RuntimeComplete generates completion candidates for the given args and writes
// them to w. Each candidate is one line, optionally followed by a tab and
// description. Active help messages are prefixed with "_activeHelp_ ".
// The last line is a directive in the format ":<int>".
func RuntimeComplete(ctx context.Context, root Commander, args []string, w io.Writer) {
	result := computeCompletions(ctx, root, args)

	// Output active help messages first.
	for _, msg := range result.ActiveHelp {
		fmt.Fprintln(w, activeHelpPrefix+msg) //nolint:errcheck // best-effort completion output
	}

	// Output completions.
	for _, c := range result.Completions {
		if c.Description != "" {
			fmt.Fprintf(w, "%s\t%s\n", c.Value, c.Description) //nolint:errcheck // best-effort completion output
		} else {
			fmt.Fprintln(w, c.Value) //nolint:errcheck // best-effort completion output
		}
	}
	fmt.Fprintf(w, ":%d\n", int(result.Directive)) //nolint:errcheck // best-effort completion output
}

// computeCompletions resolves the command chain and returns a CompletionResult.
func computeCompletions(ctx context.Context, root Commander, args []string) CompletionResult {
	contextArgs, toComplete := splitCompletionArgs(args)
	target := walkCommandTree(root, contextArgs)

	// Try Completer interface first.
	if result, ok := tryCompleterInterface(ctx, target, args, toComplete); ok {
		return result
	}

	// Complete flags if prefix starts with "-".
	if strings.HasPrefix(toComplete, "-") {
		return completeFlagNames(target, toComplete)
	}

	// Complete flag values if previous arg was a value flag.
	if result, ok := completeFlagValue(ctx, target, contextArgs, toComplete); ok {
		return result
	}

	// Complete subcommand names.
	return completeSubcommandNames(target, toComplete)
}

func splitCompletionArgs(args []string) ([]string, string) {
	if len(args) == 0 {
		return nil, ""
	}
	return args[:len(args)-1], args[len(args)-1]
}

func walkCommandTree(root Commander, contextArgs []string) Commander {
	target := root
	remaining := contextArgs
	for len(remaining) > 0 {
		subs, err := allSubcommands(target)
		if err != nil || len(subs) == 0 {
			break
		}

		arg := remaining[0]
		if strings.HasPrefix(arg, "-") {
			fi := buildFlagIndex(target)
			if consumed, ok := tryConsumeFlag(remaining, 0, fi); ok {
				remaining = remaining[consumed:]
				continue
			}
			remaining = remaining[1:]
			continue
		}

		sub := findSubcommand(subs, arg, false, false)
		if sub == nil {
			remaining = remaining[1:]
			continue
		}
		target = sub
		remaining = remaining[1:]
	}
	return target
}

func tryCompleterInterface(ctx context.Context, target Commander, args []string, toComplete string) (CompletionResult, bool) {
	c, ok := target.(Completer)
	if !ok {
		return CompletionResult{}, false
	}
	result := c.Complete(ctx, args)
	if len(result.Completions) == 0 && len(result.ActiveHelp) == 0 {
		return CompletionResult{}, false
	}
	return filterResult(result, toComplete), true
}

func filterResult(result CompletionResult, toComplete string) CompletionResult {
	filtered := filterCompletions(result.Completions, toComplete)
	directive := result.Directive
	if len(filtered) == 0 && result.Directive != ShellCompDirectiveError {
		directive = ShellCompDirectiveDefault
	}
	return CompletionResult{
		Completions: filtered,
		ActiveHelp:  result.ActiveHelp,
		Directive:   directive,
	}
}

func completeFlagNames(target Commander, toComplete string) CompletionResult {
	candidates := completeCommandFlags(target, toComplete)
	directive := ShellCompDirectiveNoFileComp
	if len(candidates) == 0 {
		directive = ShellCompDirectiveDefault
	}
	return CompletionResult{Completions: candidates, Directive: directive}
}

func completeFlagValue(ctx context.Context, target Commander, contextArgs []string, toComplete string) (CompletionResult, bool) {
	if len(contextArgs) == 0 {
		return CompletionResult{}, false
	}
	prev := contextArgs[len(contextArgs)-1]
	flagName := lookupValueFlagName(target, prev)
	if flagName == "" {
		return CompletionResult{}, false
	}

	// Try FlagCompleter interface.
	if fc, ok := target.(FlagCompleter); ok {
		result := fc.CompleteFlag(ctx, flagName, toComplete)
		if len(result.Completions) > 0 || len(result.ActiveHelp) > 0 {
			return filterResult(result, toComplete), true
		}
	}

	// Try enum completion.
	if enumVals := lookupFlagEnum(target, prev); enumVals != "" {
		vals := strings.Split(enumVals, ",")
		filtered := filterStrings(vals, toComplete)
		directive := ShellCompDirectiveNoFileComp
		if len(filtered) == 0 {
			directive = ShellCompDirectiveDefault
		}
		return CompletionResult{Completions: stringsToCompletions(filtered), Directive: directive}, true
	}

	return CompletionResult{}, false
}

func completeSubcommandNames(target Commander, toComplete string) CompletionResult {
	subs, _ := allSubcommands(target) //nolint:errcheck // best-effort in completion
	var candidates []Completion
	for _, sub := range subs {
		info := resolveInfo(sub)
		if info.hidden {
			continue
		}
		if strings.HasPrefix(info.name, toComplete) {
			candidates = append(candidates, Completion{Value: info.name, Description: info.description})
		}
		for _, alias := range info.aliases {
			if strings.HasPrefix(alias, toComplete) {
				candidates = append(candidates, Completion{Value: alias, Description: info.description})
			}
		}
	}

	directive := ShellCompDirectiveNoFileComp
	if len(candidates) == 0 {
		directive = ShellCompDirectiveDefault
	}
	return CompletionResult{Completions: candidates, Directive: directive}
}

// completeCommandFlags returns flag completions matching prefix.
func completeCommandFlags(cmd Commander, prefix string) []Completion {
	flags := ScanFlags(cmd)
	var candidates []Completion
	for i := range flags {
		f := &flags[i]
		if f.Hidden || f.Deprecated != "" {
			continue
		}
		name := "--" + f.Name
		if strings.HasPrefix(name, prefix) {
			candidates = append(candidates, Completion{Value: name, Description: f.Help})
		}
		if f.Short != "" {
			short := "-" + f.Short
			if strings.HasPrefix(short, prefix) {
				candidates = append(candidates, Completion{Value: short, Description: f.Help})
			}
		}
		for _, alt := range f.Alt {
			altName := "--" + alt
			if strings.HasPrefix(altName, prefix) {
				candidates = append(candidates, Completion{Value: altName, Description: f.Help})
			}
		}
		if f.Negate {
			neg := "--no-" + f.Name
			if strings.HasPrefix(neg, prefix) {
				candidates = append(candidates, Completion{Value: neg, Description: "Disable " + f.Name})
			}
		}
	}
	return candidates
}

// lookupValueFlagName returns the flag name (without dashes) if arg matches
// a non-bool, non-counter flag on cmd, or empty string if not found.
func lookupValueFlagName(cmd Commander, arg string) string {
	if !strings.HasPrefix(arg, "-") {
		return ""
	}
	flags := ScanFlags(cmd)
	name := strings.TrimLeft(arg, "-")
	for i := range flags {
		f := &flags[i]
		if f.Hidden || f.Deprecated != "" {
			continue
		}
		if f.IsBool || f.IsCounter {
			continue
		}
		if f.Name == name || f.Short == name || slices.Contains(f.Alt, name) {
			return f.Name
		}
	}
	return ""
}

// lookupFlagEnum returns the Enum string for a flag matching the given arg
// (e.g. "--format"), or empty if no match or no enum.
func lookupFlagEnum(cmd Commander, arg string) string {
	if !strings.HasPrefix(arg, "-") {
		return ""
	}
	flags := ScanFlags(cmd)
	name := strings.TrimLeft(arg, "-")
	for i := range flags {
		f := &flags[i]
		if f.Hidden || f.Deprecated != "" {
			continue
		}
		if f.IsBool || f.IsCounter {
			continue
		}
		if f.Enum == "" {
			continue
		}
		if f.Name == name || f.Short == name || slices.Contains(f.Alt, name) {
			return f.Enum
		}
	}
	return ""
}

// filterCompletions filters candidates to those starting with prefix.
func filterCompletions(candidates []Completion, prefix string) []Completion {
	if prefix == "" {
		return candidates
	}
	var out []Completion
	for _, c := range candidates {
		if strings.HasPrefix(c.Value, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// filterStrings filters string candidates to those starting with prefix.
func filterStrings(candidates []string, prefix string) []string {
	if prefix == "" {
		return candidates
	}
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// stringsToCompletions converts strings to Completion values.
func stringsToCompletions(vals []string) []Completion {
	comps := make([]Completion, len(vals))
	for i, v := range vals {
		comps[i] = Completion{Value: v}
	}
	return comps
}
