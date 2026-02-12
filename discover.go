package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

var commanderType = reflect.TypeFor[Commander]()

// PluginInfo is the optional JSON metadata a plugin can return via its
// info flag (default "--cli-info"). All fields are optional — a plugin
// that does not support the flag or returns invalid JSON still works; it
// simply has no description or aliases in help output.
type PluginInfo struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// ExternalCommand wraps an external executable as a [Commander]. When Run
// is called, it executes the binary, wiring stdin, stdout, and stderr
// to the parent process.
//
// ExternalCommand implements [Namer], [Descriptor], and [Aliaser].
type ExternalCommand struct {
	// Path is the absolute path to the plugin executable.
	Path string

	// Cmd is the command name used for subcommand matching and help output.
	Cmd string

	// Desc is the one-line description shown in help output.
	Desc string

	// CommandAliases are alternate names for the command.
	CommandAliases []string

	// Args receives positional arguments via injection.
	Args Args
}

// Name implements [Namer].
func (e *ExternalCommand) Name() string { return e.Cmd }

// Description implements [Descriptor].
func (e *ExternalCommand) Description() string { return e.Desc }

// Aliases implements [Aliaser].
func (e *ExternalCommand) Aliases() []string { return e.CommandAliases }

// Run implements [Commander].
func (e *ExternalCommand) Run(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.Path, e.Args...) //nolint:gosec // path is from directory scan or user-configured PATH
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DiscoverOption configures how [Discover] finds plugin executables.
type DiscoverOption func(*discoverConfig)

type discoverConfig struct {
	dirs     []string
	scanPATH bool
	infoFlag string
}

// WithDir adds a directory to scan for plugin executables. Directories
// are scanned in the order they are added. When multiple directories
// contain a plugin with the same command name, the first one found wins.
//
// Missing directories are silently skipped — this is not an error. Only
// directories that exist but cannot be read produce an error.
func WithDir(dir string) DiscoverOption {
	return func(c *discoverConfig) {
		c.dirs = append(c.dirs, dir)
	}
}

// WithDirs adds multiple directories to scan for plugin executables.
// Equivalent to calling [WithDir] for each directory in order.
func WithDirs(dirs ...string) DiscoverOption {
	return func(c *discoverConfig) {
		c.dirs = append(c.dirs, dirs...)
	}
}

// WithPATH enables scanning the system PATH for executables matching
// "<prefix>-<command>". For example, if the prefix is "mytool", an
// executable named "mytool-deploy" on PATH becomes the "deploy" command.
//
// PATH-discovered plugins have lower priority than directory-discovered
// plugins. Unreadable PATH entries are silently skipped.
func WithPATH() DiscoverOption {
	return func(c *discoverConfig) {
		c.scanPATH = true
	}
}

// WithInfoFlag sets the flag name used to query plugin metadata. The
// default is "--cli-info". When discovering plugins, the framework
// executes each plugin with this flag to retrieve optional [PluginInfo]
// JSON metadata. If the plugin does not support the flag, returns
// non-zero, or returns invalid JSON, the plugin is still registered —
// it just has no description or aliases.
func WithInfoFlag(flag string) DiscoverOption {
	return func(c *discoverConfig) {
		c.infoFlag = flag
	}
}

// Discover scans directories and optionally PATH for plugin executables
// and returns them as [Commander] values. Each discovered executable is
// wrapped in an [ExternalCommand].
//
// In directories, every executable file becomes a plugin. The command
// name is derived from the filename (e.g., a file named "deploy" in the
// directory becomes the "deploy" command).
//
// On PATH (enabled with [WithPATH]), executables matching "<prefix>-*"
// are discovered. The prefix and hyphen are stripped to derive the
// command name (e.g., prefix "mytool" + executable "mytool-deploy" →
// command "deploy").
//
// For each discovered executable, Discover runs it with the info flag
// (default "--cli-info") to retrieve optional [PluginInfo] JSON. If the
// executable does not support the flag, the plugin still works — it just
// has no description or aliases in help output.
//
// Priority rules:
//   - Directories are scanned in the order added via [WithDir]/[WithDirs].
//   - Within each directory, all executables are found.
//   - First match wins on command name collision across directories.
//   - PATH results (from [WithPATH]) have lower priority than any
//     directory result.
//
// Example:
//
//	func (a *App) Discover() ([]cli.Commander, error) {
//	    return cli.Discover("myapp",
//	        cli.WithDirs(cli.DefaultDirs("myapp")...),
//	        cli.WithPATH(),
//	    )
//	}
func Discover(prefix string, opts ...DiscoverOption) ([]Commander, error) {
	cfg := &discoverConfig{
		infoFlag: "--cli-info",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	seen := make(map[string]bool)
	var runners []Commander

	// Scan directories (higher priority).
	for _, dir := range cfg.dirs {
		found, err := discoverDir(dir, seen, cfg.infoFlag)
		if err != nil {
			return nil, err
		}
		runners = append(runners, found...)
	}

	// Scan PATH (lower priority).
	if cfg.scanPATH {
		found := discoverPATH(prefix, seen, cfg.infoFlag)
		runners = append(runners, found...)
	}

	return runners, nil
}

// DefaultDirs returns the conventional plugin directories for the given
// application name, in priority order:
//
//  1. ./<name>/plugins — project-level (highest priority)
//  2. $HOME/.config/<name>/plugins — user-level
//  3. /etc/<name>/plugins — system-level (lowest priority, Unix only)
//
// Missing directories are silently skipped by [Discover]. The returned
// paths are suitable for passing directly to [WithDirs]:
//
//	cli.Discover("myapp", cli.WithDirs(cli.DefaultDirs("myapp")...))
func DefaultDirs(name string) []string {
	dirs := []string{
		filepath.Join(".", name, "plugins"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", name, "plugins"))
	}
	if runtime.GOOS != "windows" {
		dirs = append(dirs, filepath.Join("/etc", name, "plugins"))
	}
	return dirs
}

// AllSubcommands returns all subcommands for a command by merging static
// subcommands from [Subcommander] with runtime-discovered subcommands from
// [Discoverer]. Built-in commands from Subcommander take priority — discovered
// commands whose name or alias collides with a built-in are silently
// dropped.
//
// This is useful for custom [HelpRenderer] implementations, documentation
// generators, and shell completion scripts that need to enumerate all
// available subcommands including plugins.
func AllSubcommands(cmd Commander) ([]Commander, error) {
	return allSubcommands(cmd)
}

func allSubcommands(cmd Commander) ([]Commander, error) {
	// Built-in commands (embedded + Subcommander) must not collide — that's a
	// developer error. Discovered commands (plugins) silently yield to built-ins.

	// 1. Scan struct fields for embedded Commander implementations.
	embedded, err := scanEmbeddedCommands(cmd)
	if err != nil {
		return nil, err
	}

	// Build name set from embedded commands (tracks source for error messages).
	type nameSource struct {
		source string // "embedded field X" or "Subcommander"
	}
	builtinNames := make(map[string]nameSource, len(embedded)*2)
	for _, s := range embedded {
		info := resolveInfo(s)
		builtinNames[info.name] = nameSource{source: "embedded field"}
		for _, alias := range info.aliases {
			builtinNames[alias] = nameSource{source: "embedded field"}
		}
	}

	// 2. Add Subcommander interface results — error on collision with embedded.
	var subs []Commander
	if p, ok := cmd.(Subcommander); ok {
		for _, s := range p.Subcommands() {
			info := resolveInfo(s)
			if src, exists := builtinNames[info.name]; exists {
				return nil, fmt.Errorf("subcommand name collision: %q defined by both %s and Subcommander interface", info.name, src.source)
			}
			for _, alias := range info.aliases {
				if src, exists := builtinNames[alias]; exists {
					return nil, fmt.Errorf("subcommand alias collision: %q defined by both %s and Subcommander interface", alias, src.source)
				}
			}
			subs = append(subs, s)
			builtinNames[info.name] = nameSource{source: "Subcommander"}
			for _, alias := range info.aliases {
				builtinNames[alias] = nameSource{source: "Subcommander"}
			}
		}
	}

	// 3. Add Discoverer interface results — silently skip collisions with built-ins.
	// Plugins should yield to built-in commands since users don't control plugin names.
	var discovered []Commander
	if d, ok := cmd.(Discoverer); ok {
		disc, err := d.Discover()
		if err != nil {
			return nil, err
		}
		for _, s := range disc {
			info := resolveInfo(s)
			if _, exists := builtinNames[info.name]; exists {
				continue // plugin yields to built-in
			}
			discovered = append(discovered, s)
			builtinNames[info.name] = nameSource{source: "Discoverer"}
			for _, alias := range info.aliases {
				builtinNames[alias] = nameSource{source: "Discoverer"}
			}
		}
	}

	// Merge all sources.
	merged := make([]Commander, 0, len(embedded)+len(subs)+len(discovered))
	merged = append(merged, embedded...)
	merged = append(merged, subs...)
	merged = append(merged, discovered...)

	return merged, nil
}

// isBranchingCommand returns true if cmd has both positional arg fields and
// embedded Commander fields. Branching commands consume their args before
// subcommand resolution, enabling patterns like: app user 42 delete
func isBranchingCommand(cmd Commander) bool {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}

	hasArgs := false
	hasCommanders := false
	t := v.Type()

	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		// Check for arg tag.
		if _, has := f.Tag.Lookup("arg"); has {
			hasArgs = true
		}

		// Check for cli.Args type (first-class arg capture).
		if f.Type == argsType {
			hasArgs = true
		}

		// Check for Commander implementation (non-anonymous only).
		if !f.Anonymous {
			fieldType := f.Type
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
			if reflect.PointerTo(fieldType).Implements(commanderType) {
				hasCommanders = true
			}
		}

		if hasArgs && hasCommanders {
			return true
		}
	}

	return false
}

// scanEmbeddedCommands scans a command's struct fields for embedded Commander
// implementations. Fields that implement Commander become subcommands.
//
// Rules:
//   - Only named (non-anonymous) exported fields are considered
//   - Field must implement Commander interface
//   - Field must NOT have flag/arg/env tags (returns error if it does)
//   - Duplicate command names return an error
func scanEmbeddedCommands(cmd Commander) ([]Commander, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, nil
	}

	t := v.Type()
	var subs []Commander
	names := make(map[string]string) // command name -> field name (for error messages)

	for i := range t.NumField() {
		f := t.Field(i)

		// Skip unexported fields.
		if !f.IsExported() {
			continue
		}

		// Skip anonymous (embedded) fields — those are for flag promotion.
		if f.Anonymous {
			continue
		}

		// Check if field type implements Commander.
		fieldType := f.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// For value types, we need pointer to check interface implementation.
		ptrType := reflect.PointerTo(fieldType)
		if !ptrType.Implements(commanderType) {
			continue
		}

		// Field implements Commander — validate it has no CLI tags.
		if _, has := f.Tag.Lookup("flag"); has {
			return nil, fmt.Errorf("%w: field %q implements Commander but has flag tag", ErrInvalidTag, f.Name)
		}
		if _, has := f.Tag.Lookup("arg"); has {
			return nil, fmt.Errorf("%w: field %q implements Commander but has arg tag", ErrInvalidTag, f.Name)
		}
		if _, has := f.Tag.Lookup("env"); has {
			return nil, fmt.Errorf("%w: field %q implements Commander but has env tag", ErrInvalidTag, f.Name)
		}

		// Get the Commander value.
		fv := v.Field(i)
		if fv.Kind() == reflect.Ptr {
			if fv.IsNil() {
				// Initialize nil pointer.
				fv.Set(reflect.New(fieldType))
			}
			fv = fv.Elem()
		}
		sub := fv.Addr().Interface().(Commander)

		// Check for name collision.
		info := resolveInfo(sub)
		if existingField, exists := names[info.name]; exists {
			return nil, fmt.Errorf("duplicate subcommand name %q: fields %q and %q", info.name, existingField, f.Name)
		}
		names[info.name] = f.Name

		subs = append(subs, sub)
	}

	return subs, nil
}

func discoverDir(dir string, seen map[string]bool, infoFlag string) ([]Commander, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	runners := make([]Commander, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if !isExecutable(path) {
			continue
		}

		name := entry.Name()
		if seen[name] {
			continue
		}
		seen[name] = true

		runners = append(runners, newExternalCommand(path, name, infoFlag))
	}
	return runners, nil
}

func discoverPATH(prefix string, seen map[string]bool, infoFlag string) []Commander {
	pathPrefix := prefix + "-"
	var runners []Commander

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			entryName := entry.Name()
			if !strings.HasPrefix(entryName, pathPrefix) {
				continue
			}
			cmdName := strings.TrimPrefix(entryName, pathPrefix)
			if cmdName == "" {
				continue
			}
			if seen[cmdName] {
				continue
			}

			path := filepath.Join(dir, entryName)
			if !isExecutable(path) {
				continue
			}
			seen[cmdName] = true

			runners = append(runners, newExternalCommand(path, cmdName, infoFlag))
		}
	}

	return runners
}

func newExternalCommand(path, name, infoFlag string) *ExternalCommand {
	ext := &ExternalCommand{
		Path: path,
		Cmd:  name,
	}

	if info := queryPluginInfo(path, infoFlag); info != nil {
		if info.Name != "" {
			ext.Cmd = info.Name
		}
		ext.Desc = info.Description
		ext.CommandAliases = info.Aliases
	}

	return ext
}

func queryPluginInfo(path, flag string) *PluginInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, flag).Output()
	if err != nil {
		return nil
	}
	var info PluginInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil
	}
	return &info
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".com"
	}
	return info.Mode()&0o111 != 0
}
