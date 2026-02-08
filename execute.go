package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// resolvedCommand holds the result of walking the command tree.
type resolvedCommand struct {
	chain      []Runner
	chainArgs  [][]string // per-command flag args, aligned with chain
	positional []string
}

// flagIndex maps flag names (--name, -n) to whether they are boolean.
type flagIndex struct {
	known map[string]bool // flag name -> isBool
}

func buildFlagIndex(cmd Runner) flagIndex {
	flags := ScanFlags(cmd)
	idx := flagIndex{known: make(map[string]bool, len(flags)*2)}
	for i := range flags {
		f := &flags[i]
		boolLike := f.IsBool || f.IsCounter
		idx.known["--"+f.Name] = boolLike
		if f.Short != "" {
			idx.known["-"+f.Short] = boolLike
		}
		if f.Negatable {
			idx.known["--no-"+f.Name] = true
		}
	}
	return idx
}

func (fi flagIndex) has(name string) bool {
	_, ok := fi.known[name]
	return ok
}

func (fi flagIndex) isBool(name string) bool {
	return fi.known[name]
}

// resolveCommand walks the command tree using flags-anywhere logic.
func resolveCommand(root Runner, args []string, opts *options) *resolvedCommand {
	chain := []Runner{root}
	chainArgs := [][]string{nil}
	remaining := args

	for {
		idx := len(chain) - 1
		current := chain[idx]
		p, ok := current.(Parent)
		if !ok {
			break
		}
		subs := p.Subcommands()
		if len(subs) == 0 {
			break
		}

		fi := buildFlagIndex(current)

		if opts.shortOptionHandling {
			remaining = expandShortOptions(remaining, fi)
		}

		cmdFlags, next, found := scanLevel(remaining, fi, subs, opts.prefixMatching)
		chainArgs[idx] = cmdFlags

		if found == nil {
			// Check for Fallbacker interface.
			if d, ok := current.(Fallbacker); ok {
				chain = append(chain, d.Fallback())
				chainArgs = append(chainArgs, nil)
				if next != nil {
					remaining = next
				}
				continue
			}
			if next != nil {
				remaining = next
			}
			break
		}

		chain = append(chain, found.sub)
		chainArgs = append(chainArgs, nil)
		remaining = found.remaining
	}

	// Separate remaining into leaf flags and positional args.
	leafIdx := len(chain) - 1
	fi := buildFlagIndex(chain[leafIdx])

	if opts.shortOptionHandling {
		remaining = expandShortOptions(remaining, fi)
	}

	leafFlags, positional := separateLeafArgs(remaining, fi)
	chainArgs[leafIdx] = append(chainArgs[leafIdx], leafFlags...)

	return &resolvedCommand{
		chain:      chain,
		chainArgs:  chainArgs,
		positional: positional,
	}
}

type subMatch struct {
	sub       Runner
	remaining []string
}

// scanLevel scans args at the current command level, consuming known flags
// and looking for a subcommand match. Returns consumed flags, unconsumed
// non-flag args, and a subMatch if a subcommand was found.
func scanLevel(args []string, fi flagIndex, subs []Runner, prefixMatch bool) ([]string, []string, *subMatch) {
	var cmdFlags, next []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			next = append(next, args[i:]...)
			return cmdFlags, next, nil
		}

		if strings.HasPrefix(arg, "-") {
			if consumed, ok := tryConsumeFlag(args, i, fi); ok {
				cmdFlags = append(cmdFlags, args[i:i+consumed]...)
				i += consumed - 1
				continue
			}
		}

		if !strings.HasPrefix(arg, "-") {
			sub := findSubcommand(subs, arg, prefixMatch)
			if sub != nil {
				return cmdFlags, nil, &subMatch{sub: sub, remaining: args[i+1:]}
			}
		}

		next = append(next, arg)
	}

	return cmdFlags, next, nil
}

// tryConsumeFlag checks if args[i] is a known flag and returns how many
// args it consumes (1 for bool/equals, 2 for value flags). Returns 0, false
// if the flag is not known.
func tryConsumeFlag(args []string, i int, fi flagIndex) (int, bool) {
	arg := args[i]

	// Handle --flag=value.
	if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
		name := arg[:eqIdx]
		if fi.has(name) {
			return 1, true
		}
		return 0, false
	}

	if !fi.has(arg) {
		return 0, false
	}

	if fi.isBool(arg) {
		return 1, true
	}

	if i+1 < len(args) {
		return 2, true
	}

	return 1, true
}

// separateLeafArgs splits remaining args into flags belonging to the leaf
// command and positional args.
func separateLeafArgs(args []string, fi flagIndex) ([]string, []string) {
	var flags, positional []string
	pastDashes := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			pastDashes = true
			continue
		}
		if pastDashes {
			positional = append(positional, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if consumed, ok := tryConsumeFlag(args, i, fi); ok {
				flags = append(flags, args[i:i+consumed]...)
				i += consumed - 1
				continue
			}
			positional = append(positional, arg)
			continue
		}
		positional = append(positional, arg)
	}

	return flags, positional
}

func findSubcommand(subs []Runner, name string, prefixMatch bool) Runner {
	// Exact match first.
	for _, s := range subs {
		info := resolveInfo(s)
		if info.name == name {
			return s
		}
		for _, alias := range info.aliases {
			if alias == name {
				return s
			}
		}
	}

	if !prefixMatch {
		return nil
	}

	// Unique prefix match.
	var match Runner
	for _, s := range subs {
		info := resolveInfo(s)
		if strings.HasPrefix(info.name, name) {
			if match != nil {
				return nil // ambiguous
			}
			match = s
		}
		for _, alias := range info.aliases {
			if strings.HasPrefix(alias, name) {
				if match != nil {
					return nil
				}
				match = s
			}
		}
	}
	return match
}

// expandShortOptions expands combined short flags like -abc into -a -b -c.
// All flags except the last must be bool/counter (no-value). The last flag
// can take a value from the next argument.
func expandShortOptions(args []string, fi flagIndex) []string {
	var expanded []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			expanded = append(expanded, args[i:]...)
			break
		}

		// Candidate: starts with single -, not --, more than 2 chars, no =
		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' && !strings.Contains(arg, "=") {
			allKnown := true
			allBoolExceptLast := true

			for j := 1; j < len(arg); j++ {
				shortFlag := "-" + string(arg[j])
				if !fi.has(shortFlag) {
					allKnown = false
					break
				}
				if j < len(arg)-1 && !fi.isBool(shortFlag) {
					allBoolExceptLast = false
					break
				}
			}

			if allKnown && allBoolExceptLast {
				for j := 1; j < len(arg); j++ {
					expanded = append(expanded, "-"+string(arg[j]))
				}
				continue
			}
		}

		expanded = append(expanded, arg)
	}
	return expanded
}

func execute(ctx context.Context, root Runner, args []string, opts *options) error {
	resolved := resolveCommand(root, args, opts)

	chain := resolved.chain
	leaf := chain[len(chain)-1]

	// Check --version before help.
	if v := findVersioner(chain); v != nil && versionRequested(resolved) {
		_, err := fmt.Fprintln(opts.stdout, v.Version())
		return err
	}

	if helpRequested(resolved) {
		return renderHelp(leaf, chain, opts)
	}

	// Parse flags for each command in chain.
	for i, cmd := range chain {
		cmdArgs := resolved.chainArgs[i]
		if len(cmdArgs) == 0 && len(ScanFlags(cmd)) == 0 {
			continue
		}

		remaining, parseErr := parseFlags(cmd, cmdArgs, opts)
		if parseErr != nil {
			if opts.suggest {
				if suggestion := suggestFlag(cmd, parseErr); suggestion != "" {
					return fmt.Errorf("%w\n\n%s", parseErr, suggestion)
				}
			}
			return parseErr
		}

		if i == len(chain)-1 {
			resolved.positional = append(remaining, resolved.positional...)
		}
	}

	// Validate leaf.
	if v, ok := leaf.(Validator); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}

	// Print deprecation warnings.
	for _, cmd := range chain {
		if d, ok := cmd.(Deprecater); ok {
			if msg := d.Deprecated(); msg != "" {
				fmt.Fprintf(opts.stderr, "Warning: %q is deprecated: %s\n", resolveInfo(cmd).name, msg) //nolint:errcheck // best-effort warning
			}
		}
	}

	// Before hooks (parent-first).
	var afterHooks []Runner
	for _, cmd := range chain {
		if b, ok := cmd.(BeforeRunner); ok {
			var err error
			ctx, err = b.Before(ctx)
			if err != nil {
				_ = runAfterHooks(ctx, afterHooks) //nolint:errcheck // best-effort cleanup
				return err
			}
		}
		if _, ok := cmd.(AfterRunner); ok {
			afterHooks = append(afterHooks, cmd)
		}
	}

	// Build and wrap run function.
	fn := RunFunc(leaf.Run)
	if m, ok := leaf.(Middlewarer); ok {
		fn = applyMiddleware(fn, m.Middleware())
	}

	runErr := fn(ctx, resolved.positional)

	// After hooks (child-first, always runs).
	afterErr := runAfterHooks(ctx, afterHooks)

	if runErr != nil {
		if errors.Is(runErr, ErrShowHelp) {
			return renderHelp(leaf, chain, opts)
		}
		return runErr
	}
	return afterErr
}

func helpRequested(resolved *resolvedCommand) bool {
	for _, arg := range resolved.positional {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	for _, flagArgs := range resolved.chainArgs {
		for _, arg := range flagArgs {
			if arg == "--help" || arg == "-h" {
				return true
			}
		}
	}
	return false
}

func versionRequested(resolved *resolvedCommand) bool {
	for _, arg := range resolved.positional {
		if arg == "--version" || arg == "-V" {
			return true
		}
	}
	return false
}

func findVersioner(chain []Runner) Versioner {
	for _, cmd := range chain {
		if v, ok := cmd.(Versioner); ok {
			return v
		}
	}
	return nil
}

func parseFlags(cmd Runner, args []string, opts *options) ([]string, error) {
	if opts.shortOptionHandling {
		fi := buildFlagIndex(cmd)
		args = expandShortOptions(args, fi)
	}
	if p, ok := cmd.(FlagParser); ok {
		return p.ParseFlags(cmd, args)
	}
	if opts.flagParser != nil {
		return opts.flagParser.ParseFlags(cmd, args)
	}
	return defaultParseFlags(cmd, args)
}

func runAfterHooks(ctx context.Context, hooks []Runner) error {
	var firstErr error
	for i := len(hooks) - 1; i >= 0; i-- {
		if a, ok := hooks[i].(AfterRunner); ok {
			if err := a.After(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func renderHelp(cmd Runner, chain []Runner, opts *options) error {
	if h, ok := cmd.(Helper); ok {
		_, err := fmt.Fprint(opts.stdout, h.Help())
		return err
	}

	renderer := opts.helpRenderer
	if r, ok := cmd.(HelpRenderer); ok {
		renderer = r
	}

	flags := ScanFlags(cmd)

	var text string
	if renderer != nil {
		text = renderer.RenderHelp(cmd, chain, flags)
	} else {
		text = defaultRenderHelp(cmd, chain, flags)
	}

	_, err := fmt.Fprint(opts.stdout, text)
	return err
}

func suggestFlag(cmd Runner, parseErr error) string {
	if s, ok := cmd.(Suggester); ok {
		return s.Suggest(parseErr.Error())
	}
	return suggestFromError(cmd, parseErr)
}
