package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/bjaus/bind"
)

// resolvedCommand holds the result of walking the command tree.
type resolvedCommand struct {
	chain        []Commander
	chainArgs    [][]string // per-command flag args, aligned with chain
	branchArgs   [][]string // per-command positional args for branching commands
	positional   []string
	usedFallback bool // true if Fallbacker was invoked during resolution
}

// flagIndex maps flag names (--name, -n) to whether they are boolean.
type flagIndex struct {
	known map[string]bool // flag name -> isBool
}

func buildFlagIndex(cmd Commander) flagIndex {
	flags := ScanFlags(cmd)
	idx := flagIndex{known: make(map[string]bool, len(flags)*2)}
	for i := range flags {
		f := &flags[i]
		boolLike := f.IsBool || f.IsCounter
		idx.known["--"+f.Name] = boolLike
		if f.Short != "" {
			idx.known["-"+f.Short] = boolLike
		}
		if f.Negate {
			idx.known["--no-"+f.Name] = true
		}
		for _, alt := range f.Alt {
			idx.known["--"+alt] = boolLike
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
func resolveCommand(root Commander, args []string, opts *options) (*resolvedCommand, error) {
	chain := []Commander{root}
	chainArgs := [][]string{nil}
	branchArgs := make([][]string, 1) // positional args for branching commands
	remaining := args
	usedFallback := false

	for {
		idx := len(chain) - 1
		current := chain[idx]

		subs, err := allSubcommands(current)
		if err != nil {
			return nil, err
		}
		if len(subs) == 0 {
			break
		}

		fi := buildFlagIndex(current)

		if opts.shortOptionHandling {
			remaining = expandShortOptions(remaining, fi)
		}

		// For branching commands (has both arg fields and subcommands),
		// consume positional args before looking for subcommands.
		if isBranchingCommand(current) {
			cmdFlags, positional, rest := scanBranchingLevel(remaining, fi, current)
			chainArgs[idx] = cmdFlags
			branchArgs[idx] = positional
			remaining = rest

			// Now look for subcommand in remaining args.
			sub := findSubcommandInArgs(remaining, subs, opts.prefixMatching, opts.caseInsensitive)
			if sub == nil {
				// Check for Fallbacker interface.
				if d, ok := current.(Fallbacker); ok {
					chain = append(chain, d.Fallback())
					chainArgs = append(chainArgs, nil)
					branchArgs = append(branchArgs, nil)
					usedFallback = true
					continue
				}
				break
			}
			chain = append(chain, sub.sub)
			chainArgs = append(chainArgs, nil)
			branchArgs = append(branchArgs, nil)
			remaining = sub.remaining
			continue
		}

		cmdFlags, next, found := scanLevel(remaining, fi, subs, opts.prefixMatching, opts.caseInsensitive)
		chainArgs[idx] = cmdFlags

		if found == nil {
			// Check for Fallbacker interface.
			if d, ok := current.(Fallbacker); ok {
				chain = append(chain, d.Fallback())
				chainArgs = append(chainArgs, nil)
				branchArgs = append(branchArgs, nil)
				usedFallback = true
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
		branchArgs = append(branchArgs, nil)
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
		chain:        chain,
		chainArgs:    chainArgs,
		branchArgs:   branchArgs,
		positional:   positional,
		usedFallback: usedFallback,
	}, nil
}

type subMatch struct {
	sub       Commander
	remaining []string
}

// scanLevel scans args at the current command level, consuming known flags
// and looking for a subcommand match. Returns consumed flags, unconsumed
// non-flag args, and a subMatch if a subcommand was found.
func scanLevel(args []string, fi flagIndex, subs []Commander, prefixMatch, caseInsensitive bool) ([]string, []string, *subMatch) {
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
			sub := findSubcommand(subs, arg, prefixMatch, caseInsensitive)
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

func findSubcommand(subs []Commander, name string, prefixMatch, caseInsensitive bool) Commander {
	eq := func(a, b string) bool {
		if caseInsensitive {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	hasPrefix := func(s, prefix string) bool {
		if caseInsensitive {
			return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
		}
		return strings.HasPrefix(s, prefix)
	}

	// Exact match first.
	for _, s := range subs {
		info := resolveInfo(s)
		if eq(info.name, name) {
			return s
		}
		for _, alias := range info.aliases {
			if eq(alias, name) {
				return s
			}
		}
	}

	if !prefixMatch {
		return nil
	}

	// Unique prefix match.
	var match Commander
	for _, s := range subs {
		info := resolveInfo(s)
		if hasPrefix(info.name, name) {
			if match != nil {
				return nil // ambiguous
			}
			match = s
		}
		for _, alias := range info.aliases {
			if hasPrefix(alias, name) {
				if match != nil {
					return nil
				}
				match = s
			}
		}
	}
	return match
}

// scanBranchingLevel scans args for a branching command, returning flags,
// positional args for the branch, and remaining args. This consumes positional
// args before we look for subcommands.
func scanBranchingLevel(args []string, fi flagIndex, cmd Commander) ([]string, []string, []string) {
	numBranchArgs := countBranchArgs(cmd)

	var flags, positional, rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			return flags, positional, rest
		}

		// Try to consume known flag.
		if strings.HasPrefix(arg, "-") {
			if consumed, ok := tryConsumeFlag(args, i, fi); ok {
				flags = append(flags, args[i:i+consumed]...)
				i += consumed - 1
				continue
			}
		}

		// Non-flag: is it a branch arg or rest?
		if len(positional) < numBranchArgs {
			positional = append(positional, arg)
		} else {
			rest = append(rest, arg)
		}
	}
	return flags, positional, rest
}

// countBranchArgs returns the number of positional args a command expects
// (based on arg-tagged fields, not including cli.Args).
func countBranchArgs(cmd Commander) int {
	defs := ScanArgs(cmd)
	count := 0
	for i := range defs {
		if defs[i].IsSlice {
			continue // slice args consume remaining, not counted
		}
		count++
	}
	return count
}

// findSubcommandInArgs finds the first subcommand match in args.
func findSubcommandInArgs(args []string, subs []Commander, prefixMatch, caseInsensitive bool) *subMatch {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if sub := findSubcommand(subs, arg, prefixMatch, caseInsensitive); sub != nil {
			return &subMatch{sub: sub, remaining: args[i+1:]}
		}
	}
	return nil
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

func execute(ctx context.Context, root Commander, args []string, opts *options) error {
	// Strip program name if args[0] looks like a binary path.
	// This allows callers to pass os.Args directly.
	cmdName := resolveInfo(root).name
	args = stripProgramName(args, cmdName)

	// Intercept __complete before any lifecycle hooks, flag parsing, or validation.
	if len(args) > 0 && args[0] == "__complete" {
		RuntimeComplete(ctx, root, args[1:], opts.stdout)
		return nil
	}

	resolved, err := resolveCommand(root, args, opts)
	if err != nil {
		return err
	}

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

	// Initializer hooks (parent-first) — runs before any parsing.
	ctx, err = runInitializerHooks(ctx, chain)
	if err != nil {
		return err
	}

	// Check if leaf disables flag parsing (passthrough mode).
	leafPassthrough := false
	if pt, ok := leaf.(Passthrougher); ok {
		leafPassthrough = pt.Passthrough()
	}

	// Parse and validate flags for all commands in the chain.
	provided, err := parseFlagChain(resolved, chain, leafPassthrough, opts)
	if err != nil {
		return err
	}

	// Populate branch args on branching commands in the chain.
	for i, cmd := range chain {
		if i < len(resolved.branchArgs) && len(resolved.branchArgs[i]) > 0 {
			if _, err := populateArgs(cmd, resolved.branchArgs[i], opts.envVarPrefix); err != nil {
				return err
			}
		}
	}

	// Save original positional args for ArgsValidator. populateArgs
	// consumes arg-tagged fields, so the validator should see the raw
	// positional args the user provided, not the leftovers.
	originalPositional := resolved.positional

	// Populate arg-tagged struct fields on the leaf command.
	if defs := ScanArgs(leaf); len(defs) > 0 {
		remaining, err := populateArgs(leaf, resolved.positional, opts.envVarPrefix)
		if err != nil {
			return err
		}
		resolved.positional = remaining
	}

	// Defaulter hooks — compute defaults after parsing, before validation.
	if err := runDefaulterHooks(chain); err != nil {
		return err
	}

	// Validate positional args and leaf command.
	if err := runValidation(leaf, originalPositional); err != nil {
		return err
	}

	// Store parsed flag values in context for Get/Lookup access.
	ctx = storeFlags(ctx, chain)

	// Inject bound dependencies into command structs.
	// Auto-bind positional args so commands can declare an Args field.
	allBindOpts := append([]bind.Option{}, opts.bindOpts...)
	allBindOpts = append(allBindOpts, bind.Value(Args(resolved.positional)))
	ctx = bind.With(ctx, allBindOpts...)
	for _, cmd := range chain {
		if err := bind.Inject(ctx, cmd); err != nil {
			return err
		}
	}

	// Print deprecation warnings.
	printDeprecationWarnings(chain, provided, opts)

	// Store the leaf command in context so parent Before hooks can inspect it.
	ctx = context.WithValue(ctx, leafKey{}, leaf)

	// Before hooks (parent-first).
	ctx, afterHooks, err := runBeforeHooks(ctx, chain)
	if err != nil {
		return err
	}

	// Build and wrap run function.
	fn := RunFunc(leaf.Run)
	if m, ok := leaf.(Middlewarer); ok {
		fn = applyMiddleware(fn, m.Middleware())
	}

	runErr := fn(ctx)

	// After hooks (child-first, always runs).
	afterErr := runAfterHooks(ctx, afterHooks)

	return handleRunResult(runErr, afterErr, leaf, chain, opts)
}

func runInitializerHooks(ctx context.Context, chain []Commander) (context.Context, error) {
	for _, cmd := range chain {
		if init, ok := cmd.(Initializer); ok {
			var err error
			ctx, err = init.Init(ctx)
			if err != nil {
				return ctx, err
			}
		}
	}
	return ctx, nil
}

func runDefaulterHooks(chain []Commander) error {
	for _, cmd := range chain {
		if d, ok := cmd.(Defaulter); ok {
			if err := d.Default(); err != nil {
				return err
			}
		}
	}
	return nil
}

func runValidation(leaf Commander, positional []string) error {
	if av, ok := leaf.(ArgsValidator); ok {
		if err := av.ValidateArgs(positional); err != nil {
			return err
		}
	}
	if v, ok := leaf.(Validator); ok {
		if err := v.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func runBeforeHooks(ctx context.Context, chain []Commander) (context.Context, []Commander, error) {
	var afterHooks []Commander
	for _, cmd := range chain {
		if b, ok := cmd.(Beforer); ok {
			var err error
			ctx, err = b.Before(ctx)
			if err != nil {
				_ = runAfterHooks(ctx, afterHooks) //nolint:errcheck // best-effort cleanup
				return ctx, nil, err
			}
		}
		if _, ok := cmd.(Afterer); ok {
			afterHooks = append(afterHooks, cmd)
		}
	}
	return ctx, afterHooks, nil
}

func handleRunResult(runErr, afterErr error, leaf Commander, chain []Commander, opts *options) error {
	if runErr != nil {
		if hs := getHelpSignal(runErr); hs != nil {
			if hs.full {
				_ = renderHelp(leaf, chain, opts) //nolint:errcheck // help render errors are non-fatal
			} else {
				_ = renderUsage(leaf, chain, opts) //nolint:errcheck // usage render errors are non-fatal
			}
			return runErr
		}
		return runErr
	}
	return afterErr
}

func helpRequested(resolved *resolvedCommand) bool {
	// "help" as first positional arg triggers help if it wasn't matched
	// as a subcommand (explicit help subcommand or Fallbacker takes priority).
	if !resolved.usedFallback && len(resolved.positional) > 0 && resolved.positional[0] == "help" {
		return true
	}

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

func findVersioner(chain []Commander) Versioner {
	for _, cmd := range chain {
		if v, ok := cmd.(Versioner); ok {
			return v
		}
	}
	return nil
}

func parseFlagChain(resolved *resolvedCommand, chain []Commander, leafPassthrough bool, opts *options) ([]map[string]bool, error) {
	provided := make([]map[string]bool, len(chain))
	for i, cmd := range chain {
		parseOpts := opts
		if leafPassthrough && i == len(chain)-1 {
			copied := *opts
			copied.ignoreUnknown = true
			parseOpts = &copied
		}

		cmdArgs := resolved.chainArgs[i]
		if len(cmdArgs) == 0 && !hasProcessableFields(cmd) {
			continue
		}

		remaining, prov, parseErr := parseFlags(cmd, cmdArgs, parseOpts)
		if parseErr != nil {
			if opts.suggest {
				if suggestion := suggestFlag(cmd, parseErr); suggestion != "" {
					return nil, fmt.Errorf("%w\n\n%s", parseErr, suggestion)
				}
			}
			return nil, parseErr
		}
		provided[i] = prov

		if i == len(chain)-1 {
			resolved.positional = append(remaining, resolved.positional...)
		}
	}

	inheritFlags(chain, provided)

	// Prompt for missing required flags on the leaf command.
	leafIdx := len(chain) - 1
	prov, err := promptForFlags(chain[leafIdx], provided[leafIdx], opts)
	if err != nil {
		return nil, err
	}
	provided[leafIdx] = prov

	for i, cmd := range chain {
		if err := ValidateFlags(cmd, provided[i]); err != nil {
			return nil, err
		}
		if err := validateFlagGroups(cmd, provided[i]); err != nil {
			return nil, err
		}
	}

	return provided, nil
}

func parseFlags(cmd Commander, args []string, opts *options) ([]string, map[string]bool, error) {
	if opts.shortOptionHandling {
		fi := buildFlagIndex(cmd)
		args = expandShortOptions(args, fi)
	}
	if p, ok := cmd.(FlagParser); ok {
		remaining, err := p.ParseFlags(cmd, args)
		return remaining, nil, err
	}
	if opts.flagParser != nil {
		remaining, err := opts.flagParser.ParseFlags(cmd, args)
		return remaining, nil, err
	}
	return defaultParseFlags(cmd, args, opts)
}

func runAfterHooks(ctx context.Context, hooks []Commander) error {
	var firstErr error
	for i := len(hooks) - 1; i >= 0; i-- {
		if a, ok := hooks[i].(Afterer); ok {
			if err := a.After(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// renderUsage prints a brief usage line to stdout.
func renderUsage(cmd Commander, chain []Commander, opts *options) error {
	path := buildCommandPath(chain)
	usage := buildUsageLine(cmd, path)
	_, err := fmt.Fprintln(opts.stdout, usage)
	return err
}

// buildCommandPath returns the full command path (e.g., "myapp cluster list").
func buildCommandPath(chain []Commander) string {
	parts := make([]string, 0, len(chain))
	for _, cmd := range chain {
		parts = append(parts, resolveInfo(cmd).name)
	}
	return strings.Join(parts, " ")
}

// buildUsageLine constructs a usage line like "Usage: myapp serve [flags] <port>".
func buildUsageLine(cmd Commander, path string) string {
	var usage strings.Builder
	usage.WriteString("Usage: ")
	usage.WriteString(path)

	// Add [flags] if the command has any flags.
	flags := ScanFlags(cmd)
	hasVisibleFlags := false
	for i := range flags {
		if !flags[i].Hidden {
			hasVisibleFlags = true
			break
		}
	}
	if hasVisibleFlags {
		usage.WriteString(" [flags]")
	}

	// Add positional args.
	args := ScanArgs(cmd)
	for i := range args {
		switch {
		case args[i].Required:
			usage.WriteString(" <")
			usage.WriteString(args[i].Name)
			usage.WriteString(">")
		case args[i].IsSlice:
			usage.WriteString(" [")
			usage.WriteString(args[i].Name)
			usage.WriteString("...]")
		default:
			usage.WriteString(" [")
			usage.WriteString(args[i].Name)
			usage.WriteString("]")
		}
	}

	// Add [command] if it has subcommands.
	if subs, err := allSubcommands(cmd); err == nil && len(subs) > 0 {
		hasVisible := false
		for _, sub := range subs {
			if h, ok := sub.(Hider); !ok || !h.Hidden() {
				hasVisible = true
				break
			}
		}
		if hasVisible {
			usage.WriteString(" [command]")
		}
	}

	return usage.String()
}

func renderHelp(cmd Commander, chain []Commander, opts *options) error {
	if h, ok := cmd.(Helper); ok {
		_, err := fmt.Fprint(opts.stdout, h.Help())
		return err
	}

	renderer := opts.helpRenderer
	if r, ok := cmd.(HelpRenderer); ok {
		renderer = r
	}

	flags := ScanFlags(cmd)
	globalFlags := collectGlobalFlags(chain, flags)

	// Apply env var prefix for help display.
	applyEnvVarPrefix := func(defs []FlagDef) {
		if opts.envVarPrefix == "" {
			return
		}
		for i := range defs {
			if defs[i].Env == "" {
				continue
			}
			parts := strings.Split(defs[i].Env, ",")
			for j := range parts {
				parts[j] = opts.envVarPrefix + strings.TrimSpace(parts[j])
			}
			defs[i].Env = strings.Join(parts, ", ")
		}
	}
	applyEnvVarPrefix(flags)
	applyEnvVarPrefix(globalFlags)

	if opts.sortedHelp {
		sortFlags(flags)
		sortFlags(globalFlags)
	}

	args := ScanArgs(cmd)

	var text string
	if renderer != nil {
		text = renderer.RenderHelp(cmd, chain, flags, args, globalFlags)
	} else {
		text = defaultRenderHelp(cmd, chain, flags, globalFlags, opts.sortedHelp)
	}

	_, err := fmt.Fprint(opts.stdout, text)
	return err
}

// collectGlobalFlags gathers visible flags from parent commands in the chain,
// deduplicating against the leaf command's flags.
func collectGlobalFlags(chain []Commander, leafFlags []FlagDef) []FlagDef {
	if len(chain) <= 1 {
		return nil
	}

	seen := make(map[string]bool, len(leafFlags))
	for i := range leafFlags {
		seen[leafFlags[i].Name] = true
	}

	var global []FlagDef
	for _, parent := range chain[:len(chain)-1] {
		parentFlags := ScanFlags(parent)
		for i := range parentFlags {
			f := &parentFlags[i]
			if f.Hidden || seen[f.Name] {
				continue
			}
			seen[f.Name] = true
			global = append(global, *f)
		}
	}
	return global
}

func printDeprecationWarnings(chain []Commander, provided []map[string]bool, opts *options) {
	for i, cmd := range chain {
		if provided[i] != nil {
			defs := ScanFlags(cmd)
			for j := range defs {
				fd := &defs[j]
				if fd.Deprecated != "" && provided[i][fd.Name] {
					fmt.Fprintf(opts.stderr, "Warning: flag --%s is deprecated: %s\n", fd.Name, fd.Deprecated) //nolint:errcheck // best-effort warning
				}
			}
		}
		if d, ok := cmd.(Deprecator); ok {
			if msg := d.Deprecated(); msg != "" {
				fmt.Fprintf(opts.stderr, "Warning: %q is deprecated: %s\n", resolveInfo(cmd).name, msg) //nolint:errcheck // best-effort warning
			}
		}
	}
}

func suggestFlag(cmd Commander, parseErr error) string {
	if s, ok := cmd.(Suggester); ok {
		return s.Suggest(parseErr.Error())
	}
	return suggestFromError(cmd, parseErr)
}
