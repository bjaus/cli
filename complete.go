package cli

import (
	"context"
	"fmt"
	"io"
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

// RuntimeComplete generates completion candidates for the given args and writes
// them to w. Each candidate is one line, optionally followed by a tab and
// description. The last line is a directive in the format ":<int>".
func RuntimeComplete(ctx context.Context, root Runner, args []string, w io.Writer) {
	completions, directive := computeCompletions(ctx, root, args)
	for _, c := range completions {
		fmt.Fprintln(w, c) //nolint:errcheck // best-effort completion output
	}
	fmt.Fprintf(w, ":%d\n", int(directive)) //nolint:errcheck
}

// computeCompletions resolves the command chain and returns completion
// candidates with a directive.
func computeCompletions(ctx context.Context, root Runner, args []string) ([]string, ShellCompDirective) {
	// Split args: everything except last is context, last is the prefix to complete.
	var contextArgs []string
	var toComplete string
	if len(args) > 0 {
		contextArgs = args[:len(args)-1]
		toComplete = args[len(args)-1]
	}

	// Walk the command tree to find the target command.
	target := root
	remaining := contextArgs
	for len(remaining) > 0 {
		subs, err := allSubcommands(target)
		if err != nil || len(subs) == 0 {
			break
		}

		arg := remaining[0]

		// Skip flags during walk.
		if strings.HasPrefix(arg, "-") {
			fi := buildFlagIndex(target)
			if consumed, ok := tryConsumeFlag(remaining, 0, fi); ok {
				remaining = remaining[consumed:]
				continue
			}
			remaining = remaining[1:]
			continue
		}

		// Try to match a subcommand.
		sub := findSubcommand(subs, arg, false, false)
		if sub == nil {
			// Unknown positional arg — skip.
			remaining = remaining[1:]
			continue
		}

		target = sub
		remaining = remaining[1:]
	}

	// If the target command implements Completer, delegate.
	if c, ok := target.(Completer); ok {
		results, directive := c.Complete(ctx, args)
		if results != nil {
			filtered := filterCompletionPrefix(results, toComplete)
			if len(filtered) == 0 {
				directive = ShellCompDirectiveDefault
			}
			return filtered, directive
		}
		// nil result: fall through to static completion.
	}

	// If toComplete starts with "-", complete flags.
	if strings.HasPrefix(toComplete, "-") {
		candidates := completeCommandFlags(target, toComplete)
		directive := ShellCompDirectiveNoFileComp
		if len(candidates) == 0 {
			directive = ShellCompDirectiveDefault
		}
		return candidates, directive
	}

	// If the previous arg was a value flag, try FlagCompleter then enum.
	if len(contextArgs) > 0 {
		prev := contextArgs[len(contextArgs)-1]
		if flagName := lookupValueFlagName(target, prev); flagName != "" {
			// Try FlagCompleter interface first.
			if fc, ok := target.(FlagCompleter); ok {
				results, directive := fc.CompleteFlag(ctx, flagName, toComplete)
				if results != nil {
					filtered := filterCompletionPrefix(results, toComplete)
					if len(filtered) == 0 {
						directive = ShellCompDirectiveDefault
					}
					return filtered, directive
				}
			}

			// Fall through to enum completion.
			if enumVals := lookupFlagEnum(target, prev); enumVals != "" {
				vals := strings.Split(enumVals, ",")
				filtered := filterCompletionPrefix(vals, toComplete)
				directive := ShellCompDirectiveNoFileComp
				if len(filtered) == 0 {
					directive = ShellCompDirectiveDefault
				}
				return filtered, directive
			}
		}
	}

	// Complete subcommand names + aliases.
	subs, _ := allSubcommands(target) //nolint:errcheck
	var candidates []string
	for _, sub := range subs {
		info := resolveInfo(sub)
		if info.hidden {
			continue
		}
		if strings.HasPrefix(info.name, toComplete) {
			entry := info.name
			if info.description != "" {
				entry += "\t" + info.description
			}
			candidates = append(candidates, entry)
		}
		for _, alias := range info.aliases {
			if strings.HasPrefix(alias, toComplete) {
				entry := alias
				if info.description != "" {
					entry += "\t" + info.description
				}
				candidates = append(candidates, entry)
			}
		}
	}

	directive := ShellCompDirectiveNoFileComp
	if len(candidates) == 0 {
		directive = ShellCompDirectiveDefault
	}
	return candidates, directive
}

// completeCommandFlags returns flag completions matching prefix.
func completeCommandFlags(cmd Runner, prefix string) []string {
	flags := ScanFlags(cmd)
	var candidates []string
	for i := range flags {
		f := &flags[i]
		if f.Hidden || f.Deprecated != "" {
			continue
		}
		name := "--" + f.Name
		if strings.HasPrefix(name, prefix) {
			entry := name
			if f.Help != "" {
				entry += "\t" + f.Help
			}
			candidates = append(candidates, entry)
		}
		if f.Short != "" {
			short := "-" + f.Short
			if strings.HasPrefix(short, prefix) {
				entry := short
				if f.Help != "" {
					entry += "\t" + f.Help
				}
				candidates = append(candidates, entry)
			}
		}
		for _, alt := range f.Alt {
			altName := "--" + alt
			if strings.HasPrefix(altName, prefix) {
				entry := altName
				if f.Help != "" {
					entry += "\t" + f.Help
				}
				candidates = append(candidates, entry)
			}
		}
		if f.Negate {
			neg := "--no-" + f.Name
			if strings.HasPrefix(neg, prefix) {
				entry := neg
				if f.Help != "" {
					entry += "\tDisable " + f.Name
				}
				candidates = append(candidates, entry)
			}
		}
	}
	return candidates
}

// lookupValueFlagName returns the flag name (without dashes) if arg matches
// a non-bool, non-counter flag on cmd, or empty string if not found.
func lookupValueFlagName(cmd Runner, arg string) string {
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
func lookupFlagEnum(cmd Runner, arg string) string {
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

// filterCompletionPrefix filters candidates to those starting with prefix.
func filterCompletionPrefix(candidates []string, prefix string) []string {
	if prefix == "" {
		return candidates
	}
	var out []string
	for _, c := range candidates {
		// Match against the candidate text before any tab (description separator).
		text, _, _ := strings.Cut(c, "\t")
		if strings.HasPrefix(text, prefix) {
			out = append(out, c)
		}
	}
	return out
}
