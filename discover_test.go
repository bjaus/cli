package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bjaus/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Discover with directories ---

func TestDiscover_Directories(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"name":"deploy","description":"Deploy to cloud"}'
  exit 0
fi`)

	writePlugin(t, filepath.Join(dir, "migrate"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"Run DB migrations"}'
  exit 0
fi`)

	// Non-executable file should be skipped.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README"), []byte("docs"), 0o600))

	// Subdirectory should be skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o750))

	runners, err := cli.Discover("myapp", cli.WithDir(dir))
	require.NoError(t, err)
	require.Len(t, runners, 2)
}

func TestDiscover_MissingDirectory(t *testing.T) {
	t.Parallel()

	runners, err := cli.Discover("myapp", cli.WithDir("/nonexistent/path/to/plugins"))
	require.NoError(t, err)
	assert.Empty(t, runners)
}

func TestDiscover_DirectoryPriority(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writePlugin(t, filepath.Join(dir1, "deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"From dir1"}'
  exit 0
fi`)

	writePlugin(t, filepath.Join(dir2, "deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"From dir2"}'
  exit 0
fi`)

	runners, err := cli.Discover("myapp", cli.WithDir(dir1), cli.WithDir(dir2))
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "deploy", ext.Name())
	assert.Equal(t, "From dir1", ext.Description())
}

// --- Discover with PATH ---

func TestDiscover_PATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "myapp-deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"Deploy via PATH"}'
  exit 0
fi`)

	// Not matching prefix — should be skipped.
	writePlugin(t, filepath.Join(dir, "other-tool"), `#!/bin/sh
echo hi`)

	// Bare prefix without command name — should be skipped.
	writePlugin(t, filepath.Join(dir, "myapp-"), `#!/bin/sh
echo hi`)

	t.Setenv("PATH", dir)

	runners, err := cli.Discover("myapp", cli.WithPATH())
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "deploy", ext.Name())
}

func TestDiscover_DirectoryBeforePATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dirPlugins := t.TempDir()
	dirPATH := t.TempDir()

	writePlugin(t, filepath.Join(dirPlugins, "deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"From directory"}'
  exit 0
fi`)

	writePlugin(t, filepath.Join(dirPATH, "myapp-deploy"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"From PATH"}'
  exit 0
fi`)

	t.Setenv("PATH", dirPATH)

	runners, err := cli.Discover("myapp", cli.WithDir(dirPlugins), cli.WithPATH())
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "From directory", ext.Description())
}

// --- WithInfoFlag ---

func TestDiscover_CustomInfoFlag(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "deploy"), `#!/bin/sh
if [ "$1" = "--metadata" ]; then
  echo '{"description":"Custom flag worked"}'
  exit 0
fi`)

	runners, err := cli.Discover("myapp", cli.WithDir(dir), cli.WithInfoFlag("--metadata"))
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "Custom flag worked", ext.Description())
}

// --- ExternalCommand interfaces ---

func TestExternalCommand_Interfaces(t *testing.T) {
	t.Parallel()

	cmd := &cli.ExternalCommand{
		Path:           "/usr/bin/test",
		Cmd:            "deploy",
		Desc:           "Deploy things",
		CommandAliases: []string{"d", "dep"},
	}

	assert.Equal(t, "deploy", cmd.Name())
	assert.Equal(t, "Deploy things", cmd.Description())
	assert.Equal(t, []string{"d", "dep"}, cmd.Aliases())

	// Verify interface satisfaction at compile time.
	var _ cli.Runner = cmd
}

// --- ExternalCommand.Run ---

func TestExternalCommand_Run(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "hello")
	writePlugin(t, path, `#!/bin/sh
echo "hello $@"`)

	cmd := &cli.ExternalCommand{Path: path, Cmd: "hello", Args: cli.Args{"world"}}
	err := cmd.Run(context.Background())
	assert.NoError(t, err)
}

func TestExternalCommand_RunFailure(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "fail")
	writePlugin(t, path, `#!/bin/sh
exit 42`)

	cmd := &cli.ExternalCommand{Path: path, Cmd: "fail"}
	err := cmd.Run(context.Background())
	assert.Error(t, err)
}

// --- DefaultDirs ---

func TestDefaultDirs(t *testing.T) {
	t.Parallel()

	dirs := cli.DefaultDirs("myapp")
	require.GreaterOrEqual(t, len(dirs), 2)

	assert.Equal(t, filepath.Join(".", "myapp", "plugins"), dirs[0])
	assert.Contains(t, dirs[1], ".config/myapp/plugins")

	if runtime.GOOS != "windows" {
		require.Len(t, dirs, 3)
		assert.Equal(t, "/etc/myapp/plugins", dirs[2])
	}
}

// --- AllSubcommands (public API) ---

func TestAllSubcommands(t *testing.T) {
	t.Parallel()

	parent := &discoverApp{
		builtins:   []cli.Runner{cli.RunFunc(func(_ context.Context) error { return nil })},
		discovered: []cli.Runner{&cli.ExternalCommand{Cmd: "plugin-cmd"}},
	}

	subs, err := cli.AllSubcommands(parent)
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}

// --- Integration: discovered command runs via Execute ---

func TestExecute_DiscoveredCommand(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()
	writePlugin(t, filepath.Join(dir, "greet"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"description":"Say hello"}'
  exit 0
fi
echo "hello"`)

	parent := &discoverFromDirApp{dir: dir, prefix: "myapp"}

	err := cli.Execute(context.Background(), parent, []string{"greet"})
	assert.NoError(t, err)
}

// --- Plugin naming override ---

func TestDiscover_InfoOverridesName(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "my-plugin"), `#!/bin/sh
if [ "$1" = "--cli-info" ]; then
  echo '{"name":"deploy","description":"Deploy it","aliases":["d"]}'
  exit 0
fi`)

	runners, err := cli.Discover("myapp", cli.WithDir(dir))
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "deploy", ext.Name())
	assert.Equal(t, "Deploy it", ext.Description())
	assert.Equal(t, []string{"d"}, ext.Aliases())
}

// --- Plugin with no --cli-info support ---

func TestDiscover_NoInfoSupport(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir := t.TempDir()

	writePlugin(t, filepath.Join(dir, "simple"), `#!/bin/sh
echo "I do stuff"`)

	runners, err := cli.Discover("myapp", cli.WithDir(dir))
	require.NoError(t, err)
	require.Len(t, runners, 1)

	ext, ok := runners[0].(*cli.ExternalCommand)
	require.True(t, ok)
	assert.Equal(t, "simple", ext.Name())
	assert.Empty(t, ext.Description())
	assert.Empty(t, ext.Aliases())
}

// --- Discover error propagation ---

func TestDiscover_UnreadableDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific permission test")
	}

	dir := t.TempDir()
	unreadable := filepath.Join(dir, "noperm")
	require.NoError(t, os.Mkdir(unreadable, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o750) //nolint:errcheck,gosec // best-effort cleanup
	})

	runners, err := cli.Discover("myapp", cli.WithDir(unreadable))
	require.Error(t, err)
	assert.Nil(t, runners)
}

// --- WithDirs convenience ---

func TestWithDirs(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writePlugin(t, filepath.Join(dir1, "a"), `#!/bin/sh
true`)
	writePlugin(t, filepath.Join(dir2, "b"), `#!/bin/sh
true`)

	runners, err := cli.Discover("myapp", cli.WithDirs(dir1, dir2))
	require.NoError(t, err)
	assert.Len(t, runners, 2)
}

// --- test helpers ---

type discoverApp struct {
	builtins   []cli.Runner
	discovered []cli.Runner
}

func (d *discoverApp) Run(_ context.Context) error { return nil }
func (d *discoverApp) Name() string                { return "myapp" }
func (d *discoverApp) Subcommands() []cli.Runner   { return d.builtins }

func (d *discoverApp) Discover() ([]cli.Runner, error) {
	return d.discovered, nil
}

type discoverFromDirApp struct {
	dir    string
	prefix string
}

func (d *discoverFromDirApp) Run(_ context.Context) error { return nil }
func (d *discoverFromDirApp) Name() string                { return d.prefix }

func (d *discoverFromDirApp) Discover() ([]cli.Runner, error) {
	return cli.Discover(d.prefix, cli.WithDir(d.dir))
}

func writePlugin(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755)) //nolint:gosec // test needs executable
}
