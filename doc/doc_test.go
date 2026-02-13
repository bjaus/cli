package doc_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/doc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rootCmd struct {
	serve  *serveCmd
	hidden *hiddenCmd
}

func (r *rootCmd) Run(_ context.Context) error  { return nil }
func (r *rootCmd) Name() string                 { return "myapp" }
func (r *rootCmd) Description() string          { return "My test application" }
func (r *rootCmd) Subcommands() []cli.Commander { return []cli.Commander{r.serve, r.hidden} }

type serveCmd struct {
	Port   int    `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	Host   string `flag:"host" default:"localhost" help:"Host to bind to"`
	Secret string `flag:"secret" hidden:"true" help:"Secret key"`
}

func (s *serveCmd) Run(_ context.Context) error { return nil }
func (s *serveCmd) Name() string                { return "serve" }
func (s *serveCmd) Description() string         { return "Start the server" }
func (s *serveCmd) Examples() []cli.Example {
	return []cli.Example{
		{Description: "Start on port 9090", Command: "myapp serve --port 9090"},
	}
}

type hiddenCmd struct{}

func (h *hiddenCmd) Run(_ context.Context) error { return nil }
func (h *hiddenCmd) Name() string                { return "internal" }
func (h *hiddenCmd) Hidden() bool                { return true }

func newRoot() *rootCmd {
	return &rootCmd{serve: &serveCmd{}, hidden: &hiddenCmd{}}
}

func TestGenMarkdown_Root(t *testing.T) {
	t.Parallel()
	root := newRoot()
	md := doc.GenMarkdown(root)

	assert.Contains(t, md, "# myapp")
	assert.Contains(t, md, "My test application")
	assert.Contains(t, md, "## Commands")
	assert.Contains(t, md, "serve")
	assert.NotContains(t, md, "internal") // hidden
}

func TestGenMarkdown_Subcommand(t *testing.T) {
	t.Parallel()
	root := newRoot()
	md := doc.GenMarkdown(root.serve, root, root.serve)

	assert.Contains(t, md, "# myapp serve")
	assert.Contains(t, md, "Start the server")
	assert.Contains(t, md, "`--port`")
	assert.Contains(t, md, "`-p`")
	assert.Contains(t, md, "8080")
	assert.NotContains(t, md, "secret") // hidden flag
	assert.Contains(t, md, "## Examples")
	assert.Contains(t, md, "myapp serve --port 9090")
}

func TestGenMarkdownTree(t *testing.T) {
	t.Parallel()
	root := newRoot()
	dir := t.TempDir()

	err := doc.GenMarkdownTree(root, dir)
	require.NoError(t, err)

	// Root file.
	_, err = os.Stat(filepath.Join(dir, "myapp.md"))
	require.NoError(t, err)

	// Serve subcommand file.
	_, err = os.Stat(filepath.Join(dir, "myapp_serve.md"))
	require.NoError(t, err)

	// Hidden commands should NOT be generated.
	_, err = os.Stat(filepath.Join(dir, "myapp_internal.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestGenManPage(t *testing.T) {
	t.Parallel()
	root := newRoot()
	header := &doc.ManHeader{Section: "1", Source: "myapp", Manual: "My App Manual"}

	man := doc.GenManPage(root.serve, header, root, root.serve)

	assert.Contains(t, man, ".TH")
	assert.Contains(t, man, ".SH NAME")
	assert.Contains(t, man, "serve")
	assert.Contains(t, man, ".SH OPTIONS")
	assert.Contains(t, man, "port")
	assert.NotContains(t, man, "secret") // hidden flag
}

func TestGenManTree(t *testing.T) {
	t.Parallel()
	root := newRoot()
	dir := t.TempDir()
	header := &doc.ManHeader{Section: "1", Source: "myapp"}

	err := doc.GenManTree(root, dir, header)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "myapp.1"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "myapp-serve.1"))
	require.NoError(t, err)

	// Hidden commands should NOT be generated.
	_, err = os.Stat(filepath.Join(dir, "myapp-internal.1"))
	assert.True(t, os.IsNotExist(err))
}

func TestGenManPage_DefaultSection(t *testing.T) {
	t.Parallel()
	root := newRoot()
	man := doc.GenManPage(root, nil)
	assert.Contains(t, man, "\"1\"")
}

// --- discoverer fixtures ---

type pluginCmd struct{}

func (p *pluginCmd) Run(_ context.Context) error { return nil }
func (p *pluginCmd) Name() string                { return "deploy-plugin" }
func (p *pluginCmd) Description() string         { return "Deploy via plugin" }

type discovererRoot struct {
	serve *serveCmd
}

func (d *discovererRoot) Run(_ context.Context) error  { return nil }
func (d *discovererRoot) Name() string                 { return "myapp" }
func (d *discovererRoot) Description() string          { return "App with plugins" }
func (d *discovererRoot) Subcommands() []cli.Commander { return []cli.Commander{d.serve} }
func (d *discovererRoot) Discover() ([]cli.Commander, error) {
	return []cli.Commander{&pluginCmd{}}, nil
}

func newDiscovererRoot() *discovererRoot {
	return &discovererRoot{serve: &serveCmd{}}
}

func TestGenMarkdown_IncludesDiscoveredCommands(t *testing.T) {
	t.Parallel()
	root := newDiscovererRoot()
	md := doc.GenMarkdown(root)

	assert.Contains(t, md, "deploy-plugin")
	assert.Contains(t, md, "Deploy via plugin")
	assert.Contains(t, md, "serve")
}

func TestGenManPage_IncludesDiscoveredCommands(t *testing.T) {
	t.Parallel()
	root := newDiscovererRoot()
	header := &doc.ManHeader{Section: "1", Source: "myapp"}

	man := doc.GenManPage(root, header)

	assert.Contains(t, man, "deploy-plugin")
	assert.Contains(t, man, "Deploy via plugin")
}

func TestGenMarkdownTree_IncludesDiscoveredCommands(t *testing.T) {
	t.Parallel()
	root := newDiscovererRoot()
	dir := t.TempDir()

	err := doc.GenMarkdownTree(root, dir)
	require.NoError(t, err)

	// Plugin command should get its own file.
	_, err = os.Stat(filepath.Join(dir, "myapp_deploy-plugin.md"))
	require.NoError(t, err)

	// Static subcommand should also be present.
	_, err = os.Stat(filepath.Join(dir, "myapp_serve.md"))
	require.NoError(t, err)
}

// --- LongDescriber fixtures ---

type longDescDocCmd struct{}

func (c *longDescDocCmd) Run(_ context.Context) error { return nil }
func (c *longDescDocCmd) Name() string                { return "longdesc" }
func (c *longDescDocCmd) Description() string         { return "Short description" }
func (c *longDescDocCmd) LongDescription() string {
	return "This is a detailed multi-paragraph description for documentation."
}

func TestGenMarkdown_LongDescription(t *testing.T) {
	t.Parallel()
	cmd := &longDescDocCmd{}
	md := doc.GenMarkdown(cmd)

	assert.Contains(t, md, "Short description")
	assert.Contains(t, md, "detailed multi-paragraph description")
}

func TestGenManPage_LongDescription(t *testing.T) {
	t.Parallel()
	cmd := &longDescDocCmd{}
	header := &doc.ManHeader{Section: "1", Source: "test"}

	man := doc.GenManPage(cmd, header)

	// NAME section uses short description.
	assert.Contains(t, man, "Short description")
	// DESCRIPTION section uses long description.
	assert.Contains(t, man, "detailed multi\\-paragraph description")
}

// --- args section ---

type docArgsCmd struct {
	File   string   `arg:"file" help:"Input file"`
	Output string   `arg:"output" required:"false" help:"Output file"`
	Extra  []string `arg:"extra" help:"Extra files"`
}

func (c *docArgsCmd) Run(_ context.Context) error { return nil }
func (c *docArgsCmd) Name() string                { return "convert" }
func (c *docArgsCmd) Description() string         { return "Convert files" }

func TestGenMarkdown_Args(t *testing.T) {
	t.Parallel()
	cmd := &docArgsCmd{}
	md := doc.GenMarkdown(cmd)

	assert.Contains(t, md, "## Arguments")
	assert.Contains(t, md, "`file`")
	assert.Contains(t, md, "`output`")
	assert.Contains(t, md, "`extra`")
	// Usage should show required/optional/slice syntax.
	assert.Contains(t, md, "<file>")
	assert.Contains(t, md, "[output]")
	assert.Contains(t, md, "[extra...]")
}

// --- flag details (deprecated, required, enum, alt, bool, counter) ---

type docFlagDetailsCmd struct {
	Format  string `flag:"format" alt:"fmt" enum:"json,yaml" required:"true" help:"Output format"`
	Verbose int    `flag:"verbose" short:"v" counter:"true" help:"Verbosity level"`
	Debug   bool   `flag:"debug" help:"Enable debug mode"`
	Old     string `flag:"old" deprecated:"use --format" help:"Legacy format"`
}

func (c *docFlagDetailsCmd) Run(_ context.Context) error { return nil }
func (c *docFlagDetailsCmd) Name() string                { return "flagdetails" }

func TestGenMarkdown_FlagDetails(t *testing.T) {
	t.Parallel()
	cmd := &docFlagDetailsCmd{}
	md := doc.GenMarkdown(cmd)

	// Deprecated flag.
	assert.Contains(t, md, "DEPRECATED")
	assert.Contains(t, md, "use --format")
	// Required flag.
	assert.Contains(t, md, "required")
	// Enum values.
	assert.Contains(t, md, "json")
	assert.Contains(t, md, "yaml")
	// Alt flag name.
	assert.Contains(t, md, "`--fmt`")
	// Bool/counter flags have empty type column.
	assert.Contains(t, md, "`--debug`")
	assert.Contains(t, md, "`-v`")
}

// --- no-flags command ---

type docNoFlagsCmd struct{}

func (c *docNoFlagsCmd) Run(_ context.Context) error { return nil }
func (c *docNoFlagsCmd) Name() string                { return "noop" }
func (c *docNoFlagsCmd) Description() string         { return "Does nothing" }

func TestGenMarkdown_NoFlags(t *testing.T) {
	t.Parallel()
	cmd := &docNoFlagsCmd{}
	md := doc.GenMarkdown(cmd)

	assert.NotContains(t, md, "[flags]")
	assert.NotContains(t, md, "## Flags")
}

// --- no-description man page ---

type docNoDescCmd struct{}

func (c *docNoDescCmd) Run(_ context.Context) error { return nil }
func (c *docNoDescCmd) Name() string                { return "bare" }

func TestGenManPage_NoDescription(t *testing.T) {
	t.Parallel()
	cmd := &docNoDescCmd{}
	header := &doc.ManHeader{Section: "1", Source: "test"}

	man := doc.GenManPage(cmd, header)

	assert.Contains(t, man, ".SH NAME")
	assert.Contains(t, man, "bare")
	// Should NOT have a DESCRIPTION section.
	assert.NotContains(t, man, ".SH DESCRIPTION")
}
