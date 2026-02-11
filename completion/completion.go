// Package completion generates shell completion scripts from a [cli.Commander]
// command tree.
//
// Supported shells: bash, zsh, fish, and PowerShell. Each generator produces
// a script that calls the binary at runtime via the __complete protocol,
// providing dynamic completions based on the current command tree.
//
// # Usage
//
//	script := completion.Bash(root, "myapp")
//	script := completion.Zsh(root, "myapp")
//	script := completion.Fish(root, "myapp")
//	script := completion.PowerShell(root, "myapp")
//
// The root parameter is kept for API compatibility. The generated scripts call
// the binary at runtime, so completions are always up to date with the current
// command tree, including plugins from [cli.Discoverer].
package completion

import (
	"fmt"
	"strings"

	"github.com/bjaus/cli"
)

// Bash generates a bash completion script that calls the binary at runtime.
func Bash(_ cli.Commander, appName string) string {
	var b strings.Builder
	safe := bashSafe(appName)

	fmt.Fprintf(&b, "# bash completion for %s\n\n", appName)
	fmt.Fprintf(&b, "_%s_completions() {\n", safe)
	b.WriteString("    local IFS=$'\\n'\n")
	b.WriteString("    local cur\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n\n")

	// Call the binary with __complete and capture output.
	fmt.Fprintf(&b, "    local out\n")
	fmt.Fprintf(&b, "    out=$(%s __complete \"${COMP_WORDS[@]:1}\" 2>/dev/null)\n", appName)
	b.WriteString("    if [[ $? -ne 0 ]]; then\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	// Parse directive from last line.
	b.WriteString("    local directive\n")
	b.WriteString("    directive=$(echo \"$out\" | tail -n1)\n")
	b.WriteString("    out=$(echo \"$out\" | head -n-1)\n\n")

	// Strip tab-separated descriptions for compgen.
	b.WriteString("    local candidates\n")
	b.WriteString("    candidates=$(echo \"$out\" | while IFS=$'\\t' read -r comp _desc; do echo \"$comp\"; done)\n\n")

	// Generate COMPREPLY.
	b.WriteString("    COMPREPLY=( $(compgen -W \"${candidates}\" -- \"${cur}\") )\n\n")

	// Handle directives (bitfield).
	b.WriteString("    local dir_val=${directive#:}\n\n")

	// FilterDirs (32)
	b.WriteString("    if (( dir_val & 32 )); then\n")
	b.WriteString("        compopt -o dirnames\n")
	b.WriteString("        COMPREPLY=( $(compgen -d -- \"${cur}\") )\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	// FilterFileExt (16) — completions are extensions like .yaml .json
	b.WriteString("    if (( dir_val & 16 )); then\n")
	b.WriteString("        local exts=\"\"\n")
	b.WriteString("        for ext in ${candidates}; do\n")
	b.WriteString("            exts=\"${exts} -o -name \\\"*${ext}\\\"\"\n")
	b.WriteString("        done\n")
	b.WriteString("        compopt -o filenames\n")
	b.WriteString("        COMPREPLY=( $(compgen -f -- \"${cur}\") )\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	// NoFileComp (4)
	b.WriteString("    if (( dir_val & 4 )); then\n")
	b.WriteString("        compopt +o default\n")
	b.WriteString("    fi\n")

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "complete -o default -F _%s_completions %s\n", safe, appName)

	return b.String()
}

// Zsh generates a zsh completion script that calls the binary at runtime.
func Zsh(_ cli.Commander, appName string) string {
	var b strings.Builder
	safe := bashSafe(appName)

	fmt.Fprintf(&b, "#compdef %s\n\n", appName)

	fmt.Fprintf(&b, "_%s() {\n", safe)
	b.WriteString("    local -a completions\n")
	b.WriteString("    local directive\n\n")

	// Call the binary with __complete.
	fmt.Fprintf(&b, "    local out\n")
	fmt.Fprintf(&b, "    out=$(\"${words[1]}\" __complete \"${words[@]:1}\" 2>/dev/null)\n")
	b.WriteString("    if [[ $? -ne 0 ]]; then\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	// Parse directive from last line.
	b.WriteString("    directive=${out##*$'\\n'}\n")
	b.WriteString("    out=${out%$'\\n'*}\n\n")

	// Parse candidates: each line is "candidate\\tdescription".
	b.WriteString("    while IFS=$'\\t' read -r comp desc; do\n")
	b.WriteString("        if [[ -n \"$comp\" ]]; then\n")
	b.WriteString("            if [[ -n \"$desc\" ]]; then\n")
	b.WriteString("                completions+=(\"${comp}:${desc}\")\n")
	b.WriteString("            else\n")
	b.WriteString("                completions+=(\"${comp}\")\n")
	b.WriteString("            fi\n")
	b.WriteString("        fi\n")
	b.WriteString("    done <<< \"$out\"\n\n")

	// Handle directives (bitfield).
	b.WriteString("    local dir_val=${directive#:}\n\n")

	// FilterDirs (32)
	b.WriteString("    if (( dir_val & 32 )); then\n")
	b.WriteString("        _files -/\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	// FilterFileExt (16) — completions are extensions
	b.WriteString("    if (( dir_val & 16 )); then\n")
	b.WriteString("        local -a exts\n")
	b.WriteString("        for comp in ${completions}; do\n")
	b.WriteString("            exts+=(\"*${comp}\")\n")
	b.WriteString("        done\n")
	b.WriteString("        _files -g \"(${(j:|:)exts})\"\n")
	b.WriteString("        return\n")
	b.WriteString("    fi\n\n")

	b.WriteString("    _describe 'completion' completions\n")

	b.WriteString("}\n\n")
	fmt.Fprintf(&b, "_%s\n", safe)

	return b.String()
}

// Fish generates a fish completion script that calls the binary at runtime.
func Fish(_ cli.Commander, appName string) string {
	var b strings.Builder
	safe := bashSafe(appName)

	fmt.Fprintf(&b, "# fish completion for %s\n\n", appName)

	fmt.Fprintf(&b, "function __%s_complete\n", safe)
	fmt.Fprintf(&b, "    set -l out (%s __complete (commandline -cop) 2>/dev/null)\n", appName)
	b.WriteString("    set -l directive $out[-1]\n")
	b.WriteString("    set -e out[-1]\n\n")

	b.WriteString("    set -l dir_val (string replace ':' '' $directive)\n\n")

	// FilterDirs (32)
	b.WriteString("    if test (math \"$dir_val & 32\") -ne 0\n")
	b.WriteString("        __fish_complete_directories\n")
	b.WriteString("        return\n")
	b.WriteString("    end\n\n")

	// FilterFileExt (16) — force file completion
	b.WriteString("    if test (math \"$dir_val & 16\") -ne 0\n")
	b.WriteString("        __fish_complete_suffix $out\n")
	b.WriteString("        return\n")
	b.WriteString("    end\n\n")

	b.WriteString("    for line in $out\n")
	b.WriteString("        echo $line\n")
	b.WriteString("    end\n")
	fmt.Fprintf(&b, "end\n\n")

	fmt.Fprintf(&b, "complete -c %s -f -a '(__%s_complete)'\n", appName, safe)

	return b.String()
}

// PowerShell generates a PowerShell completion script that calls the binary at runtime.
func PowerShell(_ cli.Commander, appName string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# PowerShell completion for %s\n\n", appName)
	fmt.Fprintf(&b, "Register-ArgumentCompleter -CommandName %s -ScriptBlock {\n", appName)
	b.WriteString("    param($commandName, $wordToComplete, $cursorPosition)\n\n")

	// Call the binary with __complete.
	fmt.Fprintf(&b, "    $words = $wordToComplete -split '\\s+' | Select-Object -Skip 1\n")
	fmt.Fprintf(&b, "    $out = & %s __complete @words 2>$null\n", appName)
	b.WriteString("    if ($LASTEXITCODE -ne 0) { return }\n\n")

	// Parse output: last line is directive, rest are candidates.
	b.WriteString("    $lines = $out -split \"`n\"\n")
	b.WriteString("    $directive = $lines[-1]\n")
	b.WriteString("    $candidates = $lines[0..($lines.Count - 2)]\n\n")

	// Parse directive value.
	b.WriteString("    $dirVal = [int]($directive -replace ':', '')\n\n")

	// FilterDirs (32)
	b.WriteString("    if ($dirVal -band 32) {\n")
	b.WriteString("        Get-ChildItem -Directory | ForEach-Object {\n")
	b.WriteString("            [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ProviderContainer', $_.Name)\n")
	b.WriteString("        }\n")
	b.WriteString("        return\n")
	b.WriteString("    }\n\n")

	// FilterFileExt (16)
	b.WriteString("    if ($dirVal -band 16) {\n")
	b.WriteString("        foreach ($ext in $candidates) {\n")
	b.WriteString("            Get-ChildItem -File -Filter \"*$ext\" | ForEach-Object {\n")
	b.WriteString("                [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ProviderItem', $_.Name)\n")
	b.WriteString("            }\n")
	b.WriteString("        }\n")
	b.WriteString("        return\n")
	b.WriteString("    }\n\n")

	// Create CompletionResult for each candidate.
	b.WriteString("    foreach ($line in $candidates) {\n")
	b.WriteString("        $parts = $line -split \"`t\", 2\n")
	b.WriteString("        $comp = $parts[0]\n")
	b.WriteString("        $desc = if ($parts.Count -gt 1) { $parts[1] } else { $comp }\n")
	b.WriteString("        [System.Management.Automation.CompletionResult]::new($comp, $comp, 'ParameterValue', $desc)\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")

	return b.String()
}

func bashSafe(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
}
