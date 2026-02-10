// Package completion generates shell completion scripts from a [cli.Runner]
// command tree.
//
// Supported shells: bash, zsh, fish, and PowerShell. Each generator walks the
// command tree and produces a self-contained script that users source in their
// shell configuration.
//
// # Usage
//
//	script := completion.Bash(root, "myapp")
//	script := completion.Zsh(root, "myapp")
//	script := completion.Fish(root, "myapp")
//	script := completion.PowerShell(root, "myapp")
//
// A common pattern is to add a hidden "completion" subcommand:
//
//	type CompletionCmd struct {
//	    Shell string `arg:"shell" help:"Shell type (bash, zsh, fish, powershell)"`
//	}
//
//	func (c *CompletionCmd) Run(ctx context.Context, args []string) error {
//	    // root is the top-level command
//	    switch c.Shell {
//	    case "bash":
//	        fmt.Println(completion.Bash(root, "myapp"))
//	    case "zsh":
//	        fmt.Println(completion.Zsh(root, "myapp"))
//	    case "fish":
//	        fmt.Println(completion.Fish(root, "myapp"))
//	    case "powershell":
//	        fmt.Println(completion.PowerShell(root, "myapp"))
//	    }
//	    return nil
//	}
package completion

import (
	"fmt"
	"strings"

	"github.com/bjaus/cli"
)

// Bash generates a bash completion script for the command tree.
func Bash(root cli.Runner, appName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# bash completion for %s\n\n", appName)
	fmt.Fprintf(&b, "_%s_completions() {\n", bashSafe(appName))
	b.WriteString("    local cur prev commands flags\n")
	b.WriteString("    COMPREPLY=()\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n\n")

	bashWriteCommand(&b, root, appName, []string{appName}, 1)

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "complete -F _%s_completions %s\n", bashSafe(appName), appName)

	return b.String()
}

// Zsh generates a zsh completion script for the command tree.
func Zsh(root cli.Runner, appName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "#compdef %s\n\n", appName)
	fmt.Fprintf(&b, "_%s() {\n", bashSafe(appName))
	b.WriteString("    local -a commands flags\n\n")

	zshWriteCommand(&b, root, appName, 1)

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "_%s\n", bashSafe(appName))

	return b.String()
}

// Fish generates a fish completion script for the command tree.
func Fish(root cli.Runner, appName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# fish completion for %s\n\n", appName)
	fishWriteCommand(&b, root, appName, nil)

	return b.String()
}

// PowerShell generates a PowerShell completion script for the command tree.
func PowerShell(root cli.Runner, appName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# PowerShell completion for %s\n\n", appName)
	fmt.Fprintf(&b, "Register-ArgumentCompleter -CommandName %s -ScriptBlock {\n", appName)
	b.WriteString("    param($commandName, $wordToComplete, $cursorPosition)\n\n")
	b.WriteString("    $commands = @{\n")

	psWriteCommands(&b, root, []string{})

	b.WriteString("    }\n\n")
	b.WriteString("    $words = $wordToComplete -split '\\s+'\n")
	b.WriteString("    $path = ($words | Select-Object -Skip 1) -join ' '\n\n")
	b.WriteString("    foreach ($key in $commands.Keys) {\n")
	b.WriteString("        if ($key -like \"$path*\") {\n")
	b.WriteString("            $commands[$key] | ForEach-Object {\n")
	b.WriteString("                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)\n")
	b.WriteString("            }\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")

	return b.String()
}

// --- bash helpers ---

func bashWriteCommand(b *strings.Builder, cmd cli.Runner, path string, words []string, depth int) {
	indent := strings.Repeat("    ", depth)

	// Determine what completions are available at this level.
	completions := make([]string, 0)

	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			if n, ok := sub.(cli.Namer); ok {
				completions = append(completions, n.Name())
			}
		}
	}

	flags := visibleFlags(cli.ScanFlags(cmd))
	for i := range flags {
		f := &flags[i]
		completions = append(completions, "--"+f.Name)
		if f.Short != "" {
			completions = append(completions, "-"+f.Short)
		}
		for _, alt := range f.Alt {
			completions = append(completions, "--"+alt)
		}
	}

	// Generate a conditional block based on the word position.
	if depth == 1 {
		fmt.Fprintf(b, "%scommands=%q\n", indent, strings.Join(completions, " "))
		b.WriteString(indent + "COMPREPLY=( $(compgen -W \"${commands}\" -- \"${cur}\") )\n")
	} else {
		condition := bashCondition(words[1:])
		fmt.Fprintf(b, "%sif %s; then\n", indent, condition)
		fmt.Fprintf(b, "%s    COMPREPLY=( $(compgen -W %q -- \"${cur}\") )\n", indent, strings.Join(completions, " "))
		fmt.Fprintf(b, "%s    return\n", indent)
		fmt.Fprintf(b, "%sfi\n", indent)
	}

	// Recurse into subcommands.
	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			name := cmdName(sub)
			subWords := append(append([]string{}, words...), name)
			bashWriteCommand(b, sub, path+" "+name, subWords, depth)
		}
	}
}

func bashCondition(words []string) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = fmt.Sprintf("\"${COMP_WORDS[%d]}\" == %q", i+1, w)
	}
	return "[[ " + strings.Join(parts, " && ") + " ]]"
}

// --- zsh helpers ---

func zshWriteCommand(b *strings.Builder, cmd cli.Runner, _ string, depth int) {
	indent := strings.Repeat("    ", depth)

	// Flags
	flags := visibleFlags(cli.ScanFlags(cmd))
	if len(flags) > 0 {
		fmt.Fprintf(b, "%sflags=(\n", indent)
		for i := range flags {
			f := &flags[i]
			desc := strings.ReplaceAll(f.Help, "'", "'\\''")
			fmt.Fprintf(b, "%s    '--%s[%s]'\n", indent, f.Name, desc)
			if f.Short != "" {
				fmt.Fprintf(b, "%s    '-%s[%s]'\n", indent, f.Short, desc)
			}
		}
		fmt.Fprintf(b, "%s)\n", indent)
	}

	// Subcommands
	if p, ok := cmd.(cli.Parent); ok {
		subs := visibleSubs(p.Subcommands())
		if len(subs) > 0 {
			fmt.Fprintf(b, "%scommands=(\n", indent)
			for _, sub := range subs {
				name := cmdName(sub)
				desc := ""
				if d, ok := sub.(cli.Describer); ok {
					desc = strings.ReplaceAll(d.Description(), "'", "'\\''")
				}
				fmt.Fprintf(b, "%s    '%s:%s'\n", indent, name, desc)
			}
			fmt.Fprintf(b, "%s)\n", indent)
			fmt.Fprintf(b, "%s_describe 'command' commands\n", indent)
		}
	}

	if len(flags) > 0 {
		fmt.Fprintf(b, "%s_arguments -s $flags\n", indent)
	}
}

// --- fish helpers ---

func fishWriteCommand(b *strings.Builder, cmd cli.Runner, appName string, parentCmds []string) {
	condition := fishCondition(appName, parentCmds)

	// Flags
	flags := visibleFlags(cli.ScanFlags(cmd))
	for i := range flags {
		f := &flags[i]
		desc := strings.ReplaceAll(f.Help, "'", "\\'")
		if f.Short != "" {
			fmt.Fprintf(b, "complete -c %s %s -s %s -l %s -d '%s'\n", appName, condition, f.Short, f.Name, desc)
		} else {
			fmt.Fprintf(b, "complete -c %s %s -l %s -d '%s'\n", appName, condition, f.Name, desc)
		}
	}

	// Subcommands
	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			name := cmdName(sub)
			desc := ""
			if d, ok := sub.(cli.Describer); ok {
				desc = strings.ReplaceAll(d.Description(), "'", "\\'")
			}
			fmt.Fprintf(b, "complete -c %s %s -a %s -d '%s'\n", appName, condition, name, desc)
			fishWriteCommand(b, sub, appName, append(parentCmds, name))
		}
	}
}

func fishCondition(_ string, parentCmds []string) string {
	if len(parentCmds) == 0 {
		return "-n '__fish_use_subcommand'"
	}
	cmds := strings.Join(parentCmds, "; and __fish_seen_subcommand_from ")
	return fmt.Sprintf("-n '__fish_seen_subcommand_from %s'", cmds)
}

// --- powershell helpers ---

func psWriteCommands(b *strings.Builder, cmd cli.Runner, parentPath []string) {
	completions := make([]string, 0)

	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			completions = append(completions, cmdName(sub))
		}
	}

	psFlags := visibleFlags(cli.ScanFlags(cmd))
	for i := range psFlags {
		f := &psFlags[i]
		completions = append(completions, "--"+f.Name)
		if f.Short != "" {
			completions = append(completions, "-"+f.Short)
		}
		for _, alt := range f.Alt {
			completions = append(completions, "--"+alt)
		}
	}

	key := strings.Join(parentPath, " ")
	quoted := make([]string, len(completions))
	for i, c := range completions {
		quoted[i] = fmt.Sprintf("'%s'", c)
	}
	fmt.Fprintf(b, "        '%s' = @(%s)\n", key, strings.Join(quoted, ", "))

	// Recurse
	if p, ok := cmd.(cli.Parent); ok {
		for _, sub := range p.Subcommands() {
			if h, ok := sub.(cli.Hider); ok && h.Hidden() {
				continue
			}
			name := cmdName(sub)
			psWriteCommands(b, sub, append(parentPath, name))
		}
	}
}

// --- shared helpers ---

func cmdName(cmd cli.Runner) string {
	if n, ok := cmd.(cli.Namer); ok {
		return n.Name()
	}
	return ""
}

func visibleSubs(subs []cli.Runner) []cli.Runner {
	out := make([]cli.Runner, 0, len(subs))
	for _, s := range subs {
		if h, ok := s.(cli.Hider); ok && h.Hidden() {
			continue
		}
		out = append(out, s)
	}
	return out
}

func visibleFlags(flags []cli.FlagDef) []cli.FlagDef {
	var out []cli.FlagDef
	for i := range flags {
		if !flags[i].Hidden {
			out = append(out, flags[i])
		}
	}
	return out
}

func bashSafe(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
}
