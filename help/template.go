package help

import (
	"bytes"
	"os"
	"strings"
	"text/template"

	"github.com/bjaus/cli"
)

// templateRenderer implements HelpRenderer using Go templates.
type templateRenderer struct {
	tmpl *template.Template
	opts *Options
}

// Template returns a help renderer using a Go text/template string.
// The template receives a HelpData struct as its data context.
//
// Available template functions:
//   - join: strings.Join
//   - upper: strings.ToUpper
//   - lower: strings.ToLower
//   - title: strings.Title (deprecated but commonly expected)
//   - indent: indents each line by n spaces
//   - wrap: wraps text to n columns
//   - default: returns fallback if value is empty
//   - repeat: repeats a string n times
//   - trimPrefix: strings.TrimPrefix
//   - trimSuffix: strings.TrimSuffix
//   - contains: strings.Contains
//   - hasPrefix: strings.HasPrefix
//   - hasSuffix: strings.HasSuffix
//   - replace: strings.ReplaceAll
func Template(tmplStr string, opts ...Option) (cli.HelpRenderer, error) {
	tmpl, err := template.New("help").Funcs(templateFuncs()).Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	return &templateRenderer{
		tmpl: tmpl,
		opts: applyOptions(opts),
	}, nil
}

// MustTemplate is like Template but panics on parse error.
func MustTemplate(tmplStr string, opts ...Option) cli.HelpRenderer {
	r, err := Template(tmplStr, opts...)
	if err != nil {
		panic(err)
	}
	return r
}

// TemplateFile returns a help renderer using a template file.
// The template receives a HelpData struct as its data context.
func TemplateFile(path string, opts ...Option) (cli.HelpRenderer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Template(string(data), opts...)
}

// MustTemplateFile is like TemplateFile but panics on error.
func MustTemplateFile(path string, opts ...Option) cli.HelpRenderer {
	r, err := TemplateFile(path, opts...)
	if err != nil {
		panic(err)
	}
	return r
}

// RenderHelp implements cli.HelpRenderer.
func (r *templateRenderer) RenderHelp(cmd cli.Commander, chain []cli.Commander, flags []cli.FlagDef, args []cli.ArgDef, globalFlags []cli.FlagDef) string {
	data := BuildHelpData(cmd, chain, flags, args, globalFlags, r.opts.Sorted)

	var buf bytes.Buffer
	if err := r.tmpl.Execute(&buf, data); err != nil {
		return "template error: " + err.Error() + "\n"
	}
	return buf.String()
}

// templateFuncs returns the template function map.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"join":       strings.Join,
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      strings.Title, //nolint:staticcheck
		"indent":     indent,
		"wrap":       wrap,
		"default":    defaultVal,
		"repeat":     strings.Repeat,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"replace":    strings.ReplaceAll,
	}
}

// indent adds n spaces to the beginning of each line.
func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = pad + line
		}
	}
	return strings.Join(lines, "\n")
}

// wrap wraps text to n columns.
func wrap(n int, s string) string {
	if n <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	var line strings.Builder
	for _, word := range words {
		if line.Len() == 0 {
			line.WriteString(word)
		} else if line.Len()+1+len(word) <= n {
			line.WriteString(" ")
			line.WriteString(word)
		} else {
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(word)
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// defaultVal returns fallback if s is empty.
func defaultVal(fallback, s string) string {
	if s == "" {
		return fallback
	}
	return s
}
