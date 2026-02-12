package help_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bjaus/cli"
	"github.com/bjaus/cli/help"
)

// testCmd is a simple command for testing.
type testCmd struct {
	Port    int    `flag:"port" short:"p" default:"8080" help:"Port to listen on"`
	Verbose bool   `flag:"verbose" short:"v" help:"Enable verbose output"`
	Config  string `flag:"config" env:"CONFIG" help:"Config file path"`
}

func (c *testCmd) Name() string        { return "serve" }
func (c *testCmd) Description() string { return "Start the HTTP server" }
func (c *testCmd) LongDescription() string {
	return "Start the HTTP server with the specified configuration."
}

func (c *testCmd) Examples() []cli.Example {
	return []cli.Example{
		{Description: "Start on default port", Command: "myapp serve"},
		{Description: "Start on custom port", Command: "myapp serve --port 3000"},
	}
}
func (c *testCmd) Run(ctx context.Context) error { return nil }

// testRoot is a root command with subcommands.
type testRoot struct {
	Debug bool `flag:"debug" short:"d" help:"Enable debug mode"`
}

func (c *testRoot) Name() string                  { return "myapp" }
func (c *testRoot) Description() string           { return "My test application" }
func (c *testRoot) Run(ctx context.Context) error { return nil }
func (c *testRoot) Subcommands() []cli.Commander {
	return []cli.Commander{&testCmd{}, &statusCmd{}}
}

// statusCmd is another subcommand.
type statusCmd struct{}

func (c *statusCmd) Name() string                  { return "status" }
func (c *statusCmd) Description() string           { return "Show application status" }
func (c *statusCmd) Run(ctx context.Context) error { return nil }

func TestResolveInfo(t *testing.T) {
	cmd := &testCmd{}
	info := help.ResolveInfo(cmd)

	if info.Name != "serve" {
		t.Errorf("expected name 'serve', got %q", info.Name)
	}
	if info.Description != "Start the HTTP server" {
		t.Errorf("expected description 'Start the HTTP server', got %q", info.Description)
	}
	if info.LongDescription != "Start the HTTP server with the specified configuration." {
		t.Errorf("unexpected long description: %q", info.LongDescription)
	}
	if len(info.Examples) != 2 {
		t.Errorf("expected 2 examples, got %d", len(info.Examples))
	}
}

func TestCommandPath(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}

	path := help.CommandPath(chain)
	if path != "myapp serve" {
		t.Errorf("expected 'myapp serve', got %q", path)
	}
}

func TestBuildArgUsage(t *testing.T) {
	tests := []struct {
		name string
		args []cli.ArgDef
		want string
	}{
		{
			name: "no args",
			args: nil,
			want: "[args...]",
		},
		{
			name: "required arg",
			args: []cli.ArgDef{{Name: "file", Required: true}},
			want: "<file>",
		},
		{
			name: "optional arg",
			args: []cli.ArgDef{{Name: "file", Required: false}},
			want: "[file]",
		},
		{
			name: "slice arg",
			args: []cli.ArgDef{{Name: "files", IsSlice: true}},
			want: "[files...]",
		},
		{
			name: "mixed",
			args: []cli.ArgDef{
				{Name: "src", Required: true},
				{Name: "dst", Required: false},
				{Name: "extras", IsSlice: true},
			},
			want: "<src> [dst] [extras...]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := help.BuildArgUsage(tt.args)
			if got != tt.want {
				t.Errorf("BuildArgUsage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVisibleFlags(t *testing.T) {
	flags := []cli.FlagDef{
		{Name: "visible1"},
		{Name: "hidden", Hidden: true},
		{Name: "visible2"},
	}

	visible := help.VisibleFlags(flags)
	if len(visible) != 2 {
		t.Errorf("expected 2 visible flags, got %d", len(visible))
	}
	for _, f := range visible {
		if f.Hidden {
			t.Error("got hidden flag in visible list")
		}
	}
}

func TestFlagLeft(t *testing.T) {
	tests := []struct {
		name string
		flag cli.FlagDef
		want string
	}{
		{
			name: "simple long flag",
			flag: cli.FlagDef{Name: "verbose", IsBool: true},
			want: "    --verbose",
		},
		{
			name: "short and long",
			flag: cli.FlagDef{Name: "verbose", Short: "v", IsBool: true},
			want: "-v, --verbose",
		},
		{
			name: "with type",
			flag: cli.FlagDef{Name: "port", Short: "p", TypeName: "int"},
			want: "-p, --port int",
		},
		{
			name: "with placeholder",
			flag: cli.FlagDef{Name: "port", Placeholder: "PORT", TypeName: "int"},
			want: "    --port PORT",
		},
		{
			name: "negatable",
			flag: cli.FlagDef{Name: "color", Negate: true, IsBool: true},
			want: "    --[no-]color",
		},
		{
			name: "with alt",
			flag: cli.FlagDef{Name: "verbose", Alt: []string{"debug"}, IsBool: true},
			want: "    --verbose, --debug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := help.FlagLeft(&tt.flag)
			if got != tt.want {
				t.Errorf("FlagLeft() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Check key elements are present.
	checks := []string{
		"Start the HTTP server",
		"Usage:",
		"myapp serve",
		"--port",
		"--verbose",
		"Examples:",
		"Global Flags:",
		"--debug",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestCompactRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Compact()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Compact should have title line with description.
	if !strings.Contains(output, "myapp serve - Start the HTTP server") {
		t.Error("compact output missing title line")
	}
	// Should have Usage on same line.
	if !strings.Contains(output, "Usage:") {
		t.Error("compact output missing Usage")
	}
}

func TestTreeRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Tree()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Tree should have ASCII tree characters.
	if !strings.Contains(output, "├──") && !strings.Contains(output, "└──") {
		t.Error("tree output missing tree characters")
	}
}

func TestManRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Man()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Man page should have all caps sections.
	sections := []string{"NAME", "SYNOPSIS", "DESCRIPTION", "OPTIONS", "EXAMPLES"}
	for _, section := range sections {
		if !strings.Contains(output, section) {
			t.Errorf("man output missing section %q", section)
		}
	}
}

func TestJSONRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.JSON()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Should be valid JSON structure.
	if !strings.HasPrefix(output, "{") {
		t.Error("JSON output should start with {")
	}
	if !strings.Contains(output, `"name"`) {
		t.Error("JSON output missing name field")
	}
	if !strings.Contains(output, `"flags"`) {
		t.Error("JSON output missing flags field")
	}
}

func TestMarkdownRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Markdown()
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Markdown should have proper headers.
	if !strings.Contains(output, "# myapp serve") {
		t.Error("Markdown output missing title")
	}
	if !strings.Contains(output, "## Usage") {
		t.Error("Markdown output missing Usage section")
	}
	if !strings.Contains(output, "## Flags") {
		t.Error("Markdown output missing Flags section")
	}
	// Should have table syntax.
	if !strings.Contains(output, "| Flag |") {
		t.Error("Markdown output missing flags table")
	}
}

func TestWithColor(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default(help.WithColor(true))
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Should contain ANSI escape codes.
	if !strings.Contains(output, "\033[") {
		t.Error("color output should contain ANSI escape codes")
	}
}

func TestWithSorted(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default(help.WithSorted())
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	// Verify output exists (sorting should work without error).
	if output == "" {
		t.Error("sorted output should not be empty")
	}
}

func TestGroupFlagsByCategory(t *testing.T) {
	flags := []cli.FlagDef{
		{Name: "a", Category: "Network"},
		{Name: "b"},
		{Name: "c", Category: "Network"},
		{Name: "d", Category: "Debug"},
	}

	groups := help.GroupFlagsByCategory(flags)

	if len(groups.Uncategorized) != 1 {
		t.Errorf("expected 1 uncategorized flag, got %d", len(groups.Uncategorized))
	}
	if len(groups.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(groups.Categories))
	}
	if len(groups.ByCategory["Network"]) != 2 {
		t.Errorf("expected 2 Network flags, got %d", len(groups.ByCategory["Network"]))
	}
}

func TestInterpolateHelp(t *testing.T) {
	tests := []struct {
		helpStr string
		def     string
		mask    string
		enum    string
		env     string
		want    string
	}{
		{
			helpStr: "Port (default: ${default})",
			def:     "8080",
			want:    "Port (default: 8080)",
		},
		{
			helpStr: "Mode (${enum})",
			enum:    "dev,prod",
			want:    "Mode (dev,prod)",
		},
		{
			helpStr: "Config (env: ${env})",
			env:     "CONFIG",
			want:    "Config (env: CONFIG)",
		},
		{
			helpStr: "Default is ${default}",
			def:     "secret",
			mask:    "****",
			want:    "Default is ****",
		},
		{
			helpStr: "No placeholders",
			want:    "No placeholders",
		},
	}

	for _, tt := range tests {
		t.Run(tt.helpStr, func(t *testing.T) {
			got := help.InterpolateHelp(tt.helpStr, tt.def, tt.mask, tt.enum, tt.env)
			if got != tt.want {
				t.Errorf("InterpolateHelp() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateRenderer(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	tmpl := `# {{.Name}}
{{.Description}}

## Usage
{{range .Usage}}  {{.}}
{{end}}
## Flags
{{range .Flags}}- --{{.Name}}: {{.Help}}
{{end}}`

	renderer, err := help.Template(tmpl)
	if err != nil {
		t.Fatalf("Template() error: %v", err)
	}

	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	checks := []string{
		"# myapp serve",
		"Start the HTTP server",
		"## Usage",
		"## Flags",
		"--port",
		"--verbose",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\nOutput:\n%s", check, output)
		}
	}
}

func TestMustTemplate(t *testing.T) {
	// Valid template should not panic.
	renderer := help.MustTemplate("{{.Name}}")
	if renderer == nil {
		t.Error("MustTemplate returned nil")
	}

	// Invalid template should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustTemplate should panic on invalid template")
		}
	}()
	help.MustTemplate("{{.Invalid")
}

func TestTemplateWithFunctions(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	tmpl := `{{upper .Name}}
{{join .Usage " | "}}
{{indent 4 .Description}}`

	renderer, err := help.Template(tmpl)
	if err != nil {
		t.Fatalf("Template() error: %v", err)
	}

	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	if !strings.Contains(output, "MYAPP SERVE") {
		t.Errorf("upper function not working, got: %s", output)
	}
	if !strings.Contains(output, "    Start the HTTP server") {
		t.Errorf("indent function not working, got: %s", output)
	}
}

func TestBuildHelpData(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	data := help.BuildHelpData(serve, chain, flags, args, globalFlags, false)

	if data.Name != "myapp serve" {
		t.Errorf("expected name 'myapp serve', got %q", data.Name)
	}
	if data.Description != "Start the HTTP server" {
		t.Errorf("expected description, got %q", data.Description)
	}
	if len(data.Flags) != 3 {
		t.Errorf("expected 3 flags, got %d", len(data.Flags))
	}
	if len(data.GlobalFlags) != 1 {
		t.Errorf("expected 1 global flag, got %d", len(data.GlobalFlags))
	}
}
