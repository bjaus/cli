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

func TestBuildData(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	data := help.BuildData(serve, chain, flags, args, globalFlags, false)

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

func TestWithWidth(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default(help.WithWidth(40))
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	if output == "" {
		t.Error("output should not be empty with custom width")
	}
}

func TestWithColorAuto(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	// ColorAuto should work without panic.
	renderer := help.Default(help.WithColorAuto())
	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)

	if output == "" {
		t.Error("output should not be empty with ColorAuto")
	}
}

func TestColorizer(t *testing.T) {
	// Test with color enabled.
	opts := &help.Options{Color: true}
	c := help.NewColorizer(opts)
	if !c.Enabled() {
		t.Error("colorizer should be enabled")
	}

	// Test Required method.
	text := c.Required("required")
	if !strings.Contains(text, "required") {
		t.Error("Required() should contain the text")
	}
	if !strings.Contains(text, "\033[") {
		t.Error("Required() should contain ANSI codes when enabled")
	}

	// Test without color.
	opts2 := &help.Options{Color: false}
	c2 := help.NewColorizer(opts2)
	if c2.Enabled() {
		t.Error("colorizer should not be enabled")
	}

	text2 := c2.Required("required")
	if text2 != "required" {
		t.Errorf("Expected 'required', got %q", text2)
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		check func(string) bool
	}{
		{
			name:  "no wrap needed",
			text:  "short",
			width: 80,
			check: func(s string) bool { return s == "short" },
		},
		{
			name:  "zero width returns unchanged",
			text:  "any text",
			width: 0,
			check: func(s string) bool { return s == "any text" },
		},
		{
			name:  "wraps long line",
			text:  "this is a long line that should wrap",
			width: 20,
			check: func(s string) bool { return strings.Contains(s, "\n") },
		},
		{
			name:  "preserves newlines",
			text:  "line one\nline two",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "\n") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := help.Wrap(tt.text, tt.width)
			if !tt.check(got) {
				t.Errorf("Wrap() = %q, check failed", got)
			}
		})
	}
}

func TestResolveWidth(t *testing.T) {
	// With explicit width.
	opts := &help.Options{Width: 100}
	if w := help.ResolveWidth(opts); w != 100 {
		t.Errorf("expected 100, got %d", w)
	}

	// With zero (auto-detect) - should return something > 0.
	opts2 := &help.Options{Width: 0}
	if w := help.ResolveWidth(opts2); w <= 0 {
		t.Errorf("expected positive width, got %d", w)
	}
}

// cmdWithCategory implements Categorizer.
type cmdWithCategory struct {
	cat string
}

func (c *cmdWithCategory) Name() string              { return "cmd" }
func (c *cmdWithCategory) Description() string       { return "desc" }
func (c *cmdWithCategory) Run(context.Context) error { return nil }
func (c *cmdWithCategory) Category() string          { return c.cat }

// hiddenCmd implements Hider.
type hiddenCmd struct{}

func (c *hiddenCmd) Name() string              { return "hidden" }
func (c *hiddenCmd) Description() string       { return "hidden cmd" }
func (c *hiddenCmd) Run(context.Context) error { return nil }
func (c *hiddenCmd) Hidden() bool              { return true }

func TestVisibleSubcommands(t *testing.T) {
	cmds := []cli.Commander{
		&testCmd{},
		&hiddenCmd{},
		&statusCmd{},
	}

	visible := help.VisibleSubcommands(cmds)
	if len(visible) != 2 {
		t.Errorf("expected 2 visible commands, got %d", len(visible))
	}
	for _, cmd := range visible {
		if h, ok := cmd.(cli.Hider); ok && h.Hidden() {
			t.Error("hidden command should not be in visible list")
		}
	}
}

func TestGroupCommandsByCategory(t *testing.T) {
	cmds := []cli.Commander{
		&cmdWithCategory{cat: "Network"},
		&testCmd{},
		&cmdWithCategory{cat: "Network"},
		&cmdWithCategory{cat: "Debug"},
	}

	groups := help.GroupCommandsByCategory(cmds)

	if len(groups.Uncategorized) != 1 {
		t.Errorf("expected 1 uncategorized, got %d", len(groups.Uncategorized))
	}
	if len(groups.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(groups.Categories))
	}
}

func TestFlagRight(t *testing.T) {
	tests := []struct {
		name string
		flag cli.FlagDef
		want []string // strings that should be present
	}{
		{
			name: "simple help",
			flag: cli.FlagDef{Help: "Simple help text"},
			want: []string{"Simple help text"},
		},
		{
			name: "with default",
			flag: cli.FlagDef{Help: "Help text", Default: "8080"},
			want: []string{"Help text", "default: 8080"},
		},
		{
			name: "with env",
			flag: cli.FlagDef{Help: "Help text", Env: "MY_VAR"},
			want: []string{"Help text", "env: MY_VAR"},
		},
		{
			name: "required",
			flag: cli.FlagDef{Help: "Help text", Required: true},
			want: []string{"Help text", "required"},
		},
		{
			name: "with enum",
			flag: cli.FlagDef{Help: "Help text", Enum: "a,b,c"},
			want: []string{"a|b|c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := help.FlagRight(&tt.flag)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("FlagRight() = %q, missing %q", got, want)
				}
			}
		})
	}
}

func TestHasVisibleFlags(t *testing.T) {
	visible := []cli.FlagDef{{Name: "a"}, {Name: "b"}}
	if !help.HasVisibleFlags(visible) {
		t.Error("expected true for non-empty visible flags")
	}

	hidden := []cli.FlagDef{{Name: "a", Hidden: true}}
	visible2 := help.VisibleFlags(hidden)
	if help.HasVisibleFlags(visible2) {
		t.Error("expected false when all flags are hidden")
	}

	if help.HasVisibleFlags(nil) {
		t.Error("expected false for nil flags")
	}
}

func TestMaxFlagWidth(t *testing.T) {
	flags := []cli.FlagDef{
		{Name: "a", Short: "a", IsBool: true},
		{Name: "verbose", Short: "v", IsBool: true},
		{Name: "config", TypeName: "string"},
	}

	width := help.MaxFlagWidth(flags)
	if width < 10 {
		t.Errorf("expected width >= 10, got %d", width)
	}
}

func TestMaxCommandWidth(t *testing.T) {
	cmds := []cli.Commander{
		&testCmd{},   // "serve"
		&statusCmd{}, // "status"
	}

	width := help.MaxCommandWidth(cmds)
	if width < 6 { // "status" is 6 chars
		t.Errorf("expected width >= 6, got %d", width)
	}
}

// Command without Namer interface for defaultName test.
type bareCmd struct{}

func (c *bareCmd) Run(context.Context) error { return nil }

func TestResolveInfoDefaultName(t *testing.T) {
	cmd := &bareCmd{}
	info := help.ResolveInfo(cmd)

	// Should derive name from type.
	if info.Name == "" {
		t.Error("expected non-empty default name")
	}
}

// aliasCmd implements Aliaser.
type aliasCmd struct{}

func (c *aliasCmd) Name() string              { return "main" }
func (c *aliasCmd) Description() string       { return "main cmd" }
func (c *aliasCmd) Run(context.Context) error { return nil }
func (c *aliasCmd) Aliases() []string         { return []string{"m", "alias"} }

func TestResolveInfoAliases(t *testing.T) {
	cmd := &aliasCmd{}
	info := help.ResolveInfo(cmd)

	if len(info.Aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(info.Aliases))
	}
}

func TestResolveInfoHidden(t *testing.T) {
	cmd := &hiddenCmd{}
	info := help.ResolveInfo(cmd)

	if !info.Hidden {
		t.Error("expected hidden=true")
	}
}

func TestResolveInfoCategory(t *testing.T) {
	cmd := &cmdWithCategory{cat: "Network"}
	info := help.ResolveInfo(cmd)

	if info.Category != "Network" {
		t.Errorf("expected category 'Network', got %q", info.Category)
	}
}

// cmdWithArgs for testing with arguments.
type cmdWithArgs struct {
	File   string   `arg:"file" help:"Input file" required:"true"`
	Output string   `arg:"output" help:"Output file"`
	Extra  []string `arg:"extra" help:"Extra files"`
}

func (c *cmdWithArgs) Name() string              { return "process" }
func (c *cmdWithArgs) Description() string       { return "Process files" }
func (c *cmdWithArgs) Run(context.Context) error { return nil }

func TestRenderersWithArgs(t *testing.T) {
	root := &testRoot{}
	cmd := &cmdWithArgs{}
	chain := []cli.Commander{root, cmd}
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	globalFlags := cli.ScanFlags(root)

	renderers := []struct {
		name     string
		renderer cli.HelpRenderer
	}{
		{"Default", help.Default()},
		{"Compact", help.Compact()},
		{"Man", help.Man()},
		{"Markdown", help.Markdown()},
		{"JSON", help.JSON()},
		{"Tree", help.Tree()},
	}

	for _, r := range renderers {
		t.Run(r.name, func(t *testing.T) {
			output := r.renderer.RenderHelp(cmd, chain, flags, args, globalFlags)
			if output == "" {
				t.Error("output should not be empty")
			}
			// All renderers should include the argument name.
			if !strings.Contains(output, "file") {
				t.Error("output missing argument 'file'")
			}
		})
	}
}

// rootWithSubs for testing subcommand rendering.
type rootWithSubs struct{}

func (c *rootWithSubs) Name() string              { return "app" }
func (c *rootWithSubs) Description() string       { return "Test app" }
func (c *rootWithSubs) Run(context.Context) error { return nil }
func (c *rootWithSubs) Subcommands() []cli.Commander {
	return []cli.Commander{
		&testCmd{},
		&statusCmd{},
		&hiddenCmd{},
	}
}

func TestRenderersWithSubcommands(t *testing.T) {
	root := &rootWithSubs{}
	chain := []cli.Commander{root}
	flags := cli.ScanFlags(root)
	args := cli.ScanArgs(root)

	renderers := []struct {
		name     string
		renderer cli.HelpRenderer
	}{
		{"Default", help.Default()},
		{"Compact", help.Compact()},
		{"Man", help.Man()},
		{"Markdown", help.Markdown()},
		{"JSON", help.JSON()},
		{"Tree", help.Tree()},
	}

	for _, r := range renderers {
		t.Run(r.name, func(t *testing.T) {
			output := r.renderer.RenderHelp(root, chain, flags, args, nil)
			if output == "" {
				t.Error("output should not be empty")
			}
			// Should show visible subcommands.
			if !strings.Contains(output, "serve") {
				t.Error("output missing subcommand 'serve'")
			}
			// Should NOT show hidden subcommand.
			if r.name != "JSON" && strings.Contains(output, "hidden cmd") {
				t.Error("output should not show hidden subcommand description")
			}
		})
	}
}

func TestTemplateFunctions(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	tests := []struct {
		name  string
		tmpl  string
		check func(string) bool
	}{
		{
			name:  "wrap function",
			tmpl:  `{{wrap 10 "this is a long line"}}`,
			check: func(s string) bool { return strings.Contains(s, "\n") },
		},
		{
			name:  "default function with value",
			tmpl:  `{{default "fallback" .Description}}`,
			check: func(s string) bool { return s == "Start the HTTP server" },
		},
		{
			name:  "default function without value",
			tmpl:  `{{default "fallback" ""}}`,
			check: func(s string) bool { return s == "fallback" },
		},
		{
			name:  "lower function",
			tmpl:  `{{lower .Name}}`,
			check: func(s string) bool { return s == "myapp serve" },
		},
		{
			name:  "title function",
			tmpl:  `{{title "hello"}}`,
			check: func(s string) bool { return s == "Hello" },
		},
		{
			name:  "repeat function",
			tmpl:  `{{repeat "-" 5}}`,
			check: func(s string) bool { return s == "-----" },
		},
		{
			name:  "trimPrefix function",
			tmpl:  `{{trimPrefix "helloworld" "hello"}}`,
			check: func(s string) bool { return s == "world" },
		},
		{
			name:  "trimSuffix function",
			tmpl:  `{{trimSuffix "helloworld" "world"}}`,
			check: func(s string) bool { return s == "hello" },
		},
		{
			name:  "contains function",
			tmpl:  `{{if contains .Name "app"}}yes{{end}}`,
			check: func(s string) bool { return s == "yes" },
		},
		{
			name:  "hasPrefix function",
			tmpl:  `{{if hasPrefix .Name "myapp"}}yes{{end}}`,
			check: func(s string) bool { return s == "yes" },
		},
		{
			name:  "hasSuffix function",
			tmpl:  `{{if hasSuffix .Name "serve"}}yes{{end}}`,
			check: func(s string) bool { return s == "yes" },
		},
		{
			name:  "replace function",
			tmpl:  `{{replace .Name "app" "APP"}}`,
			check: func(s string) bool { return s == "myAPP serve" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := help.Template(tt.tmpl)
			if err != nil {
				t.Fatalf("Template() error: %v", err)
			}
			output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)
			if !tt.check(output) {
				t.Errorf("check failed, got: %q", output)
			}
		})
	}
}

func TestTemplateError(t *testing.T) {
	_, err := help.Template("{{.Invalid")
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestTemplateRenderError(t *testing.T) {
	root := &testRoot{}
	serve := &testCmd{}
	chain := []cli.Commander{root, serve}
	flags := cli.ScanFlags(serve)
	args := cli.ScanArgs(serve)
	globalFlags := cli.ScanFlags(root)

	// Template that references non-existent field.
	renderer, err := help.Template("{{.NonExistent.Field}}")
	if err != nil {
		t.Fatalf("Template() error: %v", err)
	}

	output := renderer.RenderHelp(serve, chain, flags, args, globalFlags)
	if !strings.Contains(output, "template error") {
		t.Error("expected template error in output")
	}
}

func TestBuildDataWithSubcommands(t *testing.T) {
	root := &rootWithSubs{}
	chain := []cli.Commander{root}
	flags := cli.ScanFlags(root)
	args := cli.ScanArgs(root)

	data := help.BuildData(root, chain, flags, args, nil, false)

	// Should have visible subcommands.
	if len(data.Commands) < 2 {
		t.Errorf("expected at least 2 commands, got %d", len(data.Commands))
	}
}

func TestBuildDataSorted(t *testing.T) {
	root := &rootWithSubs{}
	chain := []cli.Commander{root}
	flags := cli.ScanFlags(root)
	args := cli.ScanArgs(root)

	data := help.BuildData(root, chain, flags, args, nil, true)

	// Sorted should work without error.
	if data.Name == "" {
		t.Error("name should not be empty")
	}
}

// rootWithCategorizedSubs has subcommands with categories.
type rootWithCategorizedSubs struct{}

func (c *rootWithCategorizedSubs) Name() string              { return "app" }
func (c *rootWithCategorizedSubs) Description() string       { return "Test app" }
func (c *rootWithCategorizedSubs) Run(context.Context) error { return nil }
func (c *rootWithCategorizedSubs) Subcommands() []cli.Commander {
	return []cli.Commander{
		&cmdWithCategory{cat: "Network"},
		&testCmd{},
		&cmdWithCategory{cat: "Debug"},
	}
}

func TestRenderersWithCategorizedCommands(t *testing.T) {
	root := &rootWithCategorizedSubs{}
	chain := []cli.Commander{root}
	flags := cli.ScanFlags(root)
	args := cli.ScanArgs(root)

	// Test with sorting enabled.
	renderer := help.Default(help.WithSorted())
	output := renderer.RenderHelp(root, chain, flags, args, nil)

	// Output should not be empty.
	if output == "" {
		t.Error("output should not be empty")
	}
	// Should contain the command descriptions at minimum.
	if !strings.Contains(output, "Test app") {
		t.Errorf("output missing description. Output:\n%s", output)
	}
}

// cmdWithCategorizedFlags has flags with categories.
type cmdWithCategorizedFlags struct {
	Port    int    `flag:"port" category:"Network" help:"Port"`
	Host    string `flag:"host" category:"Network" help:"Host"`
	Debug   bool   `flag:"debug" category:"Debug" help:"Debug mode"`
	Verbose bool   `flag:"verbose" help:"Verbose output"`
}

func (c *cmdWithCategorizedFlags) Name() string              { return "cmd" }
func (c *cmdWithCategorizedFlags) Description() string       { return "Command" }
func (c *cmdWithCategorizedFlags) Run(context.Context) error { return nil }

func TestRenderersWithCategorizedFlags(t *testing.T) {
	root := &testRoot{}
	cmd := &cmdWithCategorizedFlags{}
	chain := []cli.Commander{root, cmd}
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	globalFlags := cli.ScanFlags(root)

	renderers := []struct {
		name     string
		renderer cli.HelpRenderer
	}{
		{"Default", help.Default(help.WithSorted())},
		{"Compact", help.Compact(help.WithSorted())},
		{"Man", help.Man()},
		{"Markdown", help.Markdown()},
	}

	for _, r := range renderers {
		t.Run(r.name, func(t *testing.T) {
			output := r.renderer.RenderHelp(cmd, chain, flags, args, globalFlags)
			if output == "" {
				t.Error("output should not be empty")
			}
		})
	}
}

// deprecatedCmd implements Deprecator.
type deprecatedCmd struct{}

func (c *deprecatedCmd) Name() string              { return "old" }
func (c *deprecatedCmd) Description() string       { return "Old command" }
func (c *deprecatedCmd) Run(context.Context) error { return nil }
func (c *deprecatedCmd) Deprecated() string        { return "use 'new' instead" }

func TestResolveInfoDeprecated(t *testing.T) {
	// Test that deprecated info is captured (though not currently stored in CommandInfo).
	cmd := &deprecatedCmd{}
	info := help.ResolveInfo(cmd)

	if info.Name != "old" {
		t.Errorf("expected name 'old', got %q", info.Name)
	}
}

// cmdWithDeprecatedFlags has deprecated flags.
type cmdWithDeprecatedFlags struct {
	Old string `flag:"old" deprecated:"use --new" help:"Old flag"`
	New string `flag:"new" help:"New flag"`
}

func (c *cmdWithDeprecatedFlags) Name() string              { return "cmd" }
func (c *cmdWithDeprecatedFlags) Description() string       { return "Command" }
func (c *cmdWithDeprecatedFlags) Run(context.Context) error { return nil }

func TestRenderersWithDeprecatedFlags(t *testing.T) {
	root := &testRoot{}
	cmd := &cmdWithDeprecatedFlags{}
	chain := []cli.Commander{root, cmd}
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default()
	output := renderer.RenderHelp(cmd, chain, flags, args, globalFlags)

	// Output should not be empty.
	if output == "" {
		t.Error("output should not be empty")
	}
	// Should show the flags.
	if !strings.Contains(output, "--old") {
		t.Error("output should show --old flag")
	}
	if !strings.Contains(output, "--new") {
		t.Error("output should show --new flag")
	}
}

// cmdWithMaskedDefault has a flag with masked default.
type cmdWithMaskedDefault struct {
	Password string `flag:"password" default:"secret" mask:"****" help:"Password (default: ${default})"`
}

func (c *cmdWithMaskedDefault) Name() string              { return "cmd" }
func (c *cmdWithMaskedDefault) Description() string       { return "Command" }
func (c *cmdWithMaskedDefault) Run(context.Context) error { return nil }

func TestRenderersWithMaskedDefault(t *testing.T) {
	root := &testRoot{}
	cmd := &cmdWithMaskedDefault{}
	chain := []cli.Commander{root, cmd}
	flags := cli.ScanFlags(cmd)
	args := cli.ScanArgs(cmd)
	globalFlags := cli.ScanFlags(root)

	renderer := help.Default()
	output := renderer.RenderHelp(cmd, chain, flags, args, globalFlags)

	// Should show masked default, not actual value.
	if strings.Contains(output, "secret") {
		t.Error("output should not show actual secret value")
	}
	if !strings.Contains(output, "****") {
		t.Error("output should show masked value")
	}
}

func TestDetectWidthWithEnv(t *testing.T) {
	// Set COLUMNS environment variable.
	t.Setenv("COLUMNS", "120")

	opts := &help.Options{Width: 0}
	w := help.ResolveWidth(opts)

	if w != 120 {
		t.Errorf("expected 120 from COLUMNS, got %d", w)
	}
}

func TestDetectWidthInvalidEnv(t *testing.T) {
	// Set invalid COLUMNS.
	t.Setenv("COLUMNS", "invalid")

	opts := &help.Options{Width: 0}
	w := help.ResolveWidth(opts)

	// Should fall back to default or terminal detection.
	if w <= 0 {
		t.Errorf("expected positive width, got %d", w)
	}
}

func TestWrapLinePreservesIndent(t *testing.T) {
	text := "    This is an indented line that should wrap while preserving the indent"
	wrapped := help.Wrap(text, 30)

	lines := strings.Split(wrapped, "\n")
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "    ") {
			t.Errorf("line should preserve indent: %q", line)
		}
	}
}

func TestTemplateFileNotFound(t *testing.T) {
	_, err := help.TemplateFile("/nonexistent/path/template.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestMustTemplateFilePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustTemplateFile should panic on non-existent file")
		}
	}()
	help.MustTemplateFile("/nonexistent/path/template.txt")
}
