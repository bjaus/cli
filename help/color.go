package help

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// ANSI color codes.
const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Dim        = "\033[2m"
	Italic     = "\033[3m"
	Underline  = "\033[4m"
	Cyan       = "\033[36m"
	Green      = "\033[32m"
	Yellow     = "\033[33m"
	Blue       = "\033[34m"
	Magenta    = "\033[35m"
	Red        = "\033[31m"
	White      = "\033[37m"
	BoldCyan   = "\033[1;36m"
	BoldGreen  = "\033[1;32m"
	BoldYellow = "\033[1;33m"
)

// ColorScheme defines colors for different help elements.
type ColorScheme struct {
	Section  string // section headers (e.g., "Usage:", "Flags:")
	Flag     string // flag names
	Command  string // command names
	Default  string // default values
	Required string // required indicators
	Env      string // environment variables
	Example  string // example commands
	Reset    string
}

// DefaultColorScheme returns the default color scheme.
func DefaultColorScheme() ColorScheme {
	return ColorScheme{
		Section:  BoldCyan,
		Flag:     Green,
		Command:  Cyan,
		Default:  Dim,
		Required: Yellow,
		Env:      Dim,
		Example:  Dim,
		Reset:    Reset,
	}
}

// NoColorScheme returns a scheme with no colors.
func NoColorScheme() ColorScheme {
	return ColorScheme{}
}

// Colorizer applies color codes to text.
type Colorizer struct {
	scheme  ColorScheme
	enabled bool
}

// NewColorizer creates a colorizer with the given options.
func NewColorizer(opts *Options) *Colorizer {
	enabled := shouldUseColor(opts)
	scheme := NoColorScheme()
	if enabled {
		scheme = DefaultColorScheme()
	}
	return &Colorizer{
		scheme:  scheme,
		enabled: enabled,
	}
}

// Enabled returns whether color output is enabled.
func (c *Colorizer) Enabled() bool {
	return c.enabled
}

// Section colors a section header.
func (c *Colorizer) Section(text string) string {
	return c.apply(text, c.scheme.Section)
}

// Flag colors a flag name.
func (c *Colorizer) Flag(text string) string {
	return c.apply(text, c.scheme.Flag)
}

// Command colors a command name.
func (c *Colorizer) Command(text string) string {
	return c.apply(text, c.scheme.Command)
}

// Default colors a default value.
func (c *Colorizer) Default(text string) string {
	return c.apply(text, c.scheme.Default)
}

// Required colors a required indicator.
func (c *Colorizer) Required(text string) string {
	return c.apply(text, c.scheme.Required)
}

// Env colors an environment variable.
func (c *Colorizer) Env(text string) string {
	return c.apply(text, c.scheme.Env)
}

// Example colors an example command.
func (c *Colorizer) Example(text string) string {
	return c.apply(text, c.scheme.Example)
}

func (c *Colorizer) apply(text, color string) string {
	if color == "" || !c.enabled {
		return text
	}
	return color + text + c.scheme.Reset
}

// Dim applies dim styling to text.
func (c *Colorizer) Dim(text string) string {
	return c.apply(text, Dim)
}

// shouldUseColor determines whether to use color output.
func shouldUseColor(opts *Options) bool {
	if opts.Color {
		return true
	}
	if !opts.ColorAuto {
		return false
	}
	return detectColor()
}

// detectColor checks environment and terminal for color support.
func detectColor() bool {
	// Respect NO_COLOR convention.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// Check for dumb terminal.
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// Check if stdout is a terminal.
	return isTerminal(os.Stdout)
}

// isTerminal checks if the file is a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// detectWidth determines terminal width from environment or ioctl.
func detectWidth() int {
	// Check COLUMNS environment variable.
	if cols := os.Getenv("COLUMNS"); cols != "" {
		if w, err := strconv.Atoi(cols); err == nil && w > 0 {
			return w
		}
	}
	// Try to get terminal size.
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	// Default width.
	return 80
}

// ResolveWidth returns the configured width or auto-detects.
func ResolveWidth(opts *Options) int {
	if opts.Width > 0 {
		return opts.Width
	}
	return detectWidth()
}

// Wrap wraps text to the specified width.
func Wrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		result.WriteString(wrapLine(line, width))
		result.WriteByte('\n')
	}
	return strings.TrimSuffix(result.String(), "\n")
}

func wrapLine(line string, width int) string {
	if len(line) <= width {
		return line
	}

	// Preserve leading whitespace.
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}
	indentStr := line[:indent]

	words := strings.Fields(line)
	if len(words) == 0 {
		return line
	}

	var result strings.Builder
	result.WriteString(indentStr)
	lineLen := indent

	for i, word := range words {
		wordLen := len(word)
		if i == 0 {
			result.WriteString(word)
			lineLen += wordLen
			continue
		}
		if lineLen+1+wordLen > width {
			result.WriteByte('\n')
			result.WriteString(indentStr)
			result.WriteString(word)
			lineLen = indent + wordLen
		} else {
			result.WriteByte(' ')
			result.WriteString(word)
			lineLen += 1 + wordLen
		}
	}
	return result.String()
}
