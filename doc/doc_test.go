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

func (r *rootCmd) Run(_ context.Context, _ []string) error { return nil }
func (r *rootCmd) Name() string                            { return "myapp" }
func (r *rootCmd) Description() string                     { return "My test application" }
func (r *rootCmd) Subcommands() []cli.Runner               { return []cli.Runner{r.serve, r.hidden} }

type serveCmd struct {
	Port   int    `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	Host   string `flag:"host" default:"localhost" help:"Host to bind to"`
	Secret string `flag:"secret" hidden:"true" help:"Secret key"`
}

func (s *serveCmd) Run(_ context.Context, _ []string) error { return nil }
func (s *serveCmd) Name() string                            { return "serve" }
func (s *serveCmd) Description() string                     { return "Start the server" }
func (s *serveCmd) Examples() []cli.Example {
	return []cli.Example{
		{Description: "Start on port 9090", Command: "myapp serve --port 9090"},
	}
}

type hiddenCmd struct{}

func (h *hiddenCmd) Run(_ context.Context, _ []string) error { return nil }
func (h *hiddenCmd) Name() string                            { return "internal" }
func (h *hiddenCmd) Hidden() bool                            { return true }

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
