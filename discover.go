package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PluginInfo is the optional JSON metadata a plugin can return via its
// info flag (default "--cli-info"). All fields are optional — a plugin
// that does not support the flag or returns invalid JSON still works; it
// simply has no description or aliases in help output.
type PluginInfo struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// ExternalCommand wraps an external executable as a [Runner]. When Run
// is called, it executes the binary with the given args, wiring stdin,
// stdout, and stderr to the parent process.
//
// ExternalCommand implements [Namer], [Describer], and [Aliaser].
type ExternalCommand struct {
	// Path is the absolute path to the plugin executable.
	Path string

	// Cmd is the command name used for subcommand matching and help output.
	Cmd string

	// Desc is the one-line description shown in help output.
	Desc string

	// CommandAliases are alternate names for the command.
	CommandAliases []string
}

// Name implements [Namer].
func (e *ExternalCommand) Name() string { return e.Cmd }

// Description implements [Describer].
func (e *ExternalCommand) Description() string { return e.Desc }

// Aliases implements [Aliaser].
func (e *ExternalCommand) Aliases() []string { return e.CommandAliases }

// Run implements [Runner]. It executes the plugin binary with the given args.
func (e *ExternalCommand) Run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, e.Path, args...) //nolint:gosec // path is from directory scan or user-configured PATH
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
// and returns them as [Runner] values. Each discovered executable is
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
//	func (a *App) Discover() ([]cli.Runner, error) {
//	    return cli.Discover("myapp",
//	        cli.WithDirs(cli.DefaultDirs("myapp")...),
//	        cli.WithPATH(),
//	    )
//	}
func Discover(prefix string, opts ...DiscoverOption) ([]Runner, error) {
	cfg := &discoverConfig{
		infoFlag: "--cli-info",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	seen := make(map[string]bool)
	var runners []Runner

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
// subcommands from [Parent] with runtime-discovered subcommands from
// [Discoverer]. Built-in commands from Parent take priority — discovered
// commands whose name or alias collides with a built-in are silently
// dropped.
//
// This is useful for custom [HelpRenderer] implementations, documentation
// generators, and shell completion scripts that need to enumerate all
// available subcommands including plugins.
func AllSubcommands(cmd Runner) ([]Runner, error) {
	return allSubcommands(cmd)
}

func allSubcommands(cmd Runner) ([]Runner, error) {
	var subs []Runner
	if p, ok := cmd.(Parent); ok {
		subs = p.Subcommands()
	}

	d, ok := cmd.(Discoverer)
	if !ok {
		return subs, nil
	}

	discovered, err := d.Discover()
	if err != nil {
		return subs, err
	}

	// Build a set of existing names for collision detection.
	existing := make(map[string]bool, len(subs)*2)
	for _, s := range subs {
		info := resolveInfo(s)
		existing[info.name] = true
		for _, alias := range info.aliases {
			existing[alias] = true
		}
	}

	merged := make([]Runner, len(subs), len(subs)+len(discovered))
	copy(merged, subs)
	for _, disc := range discovered {
		info := resolveInfo(disc)
		if existing[info.name] {
			continue
		}
		merged = append(merged, disc)
		existing[info.name] = true
		for _, alias := range info.aliases {
			existing[alias] = true
		}
	}

	return merged, nil
}

func discoverDir(dir string, seen map[string]bool, infoFlag string) ([]Runner, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	runners := make([]Runner, 0, len(entries))
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

func discoverPATH(prefix string, seen map[string]bool, infoFlag string) []Runner {
	pathPrefix := prefix + "-"
	var runners []Runner

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
	return info.Mode()&0111 != 0
}
