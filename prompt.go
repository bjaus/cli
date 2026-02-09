package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
)

// defaultIsTerminal checks whether os.Stdin is a terminal (character device).
func defaultIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// promptForFlags interactively prompts for missing required flags on the
// leaf command. It only runs when interactive mode is enabled and stdin is
// a terminal. Values entered by the user are set on the command's struct
// fields and marked as provided. Flags are prompted in struct field order.
func promptForFlags(cmd Runner, provided map[string]bool, opts *options) (map[string]bool, error) {
	if !opts.interactive || !opts.isTerminal() {
		return provided, nil
	}

	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return provided, nil
	}

	// Use ScanFlags for deterministic struct-field ordering, then look up
	// the actual reflect field by name+tag match.
	defs := ScanFlags(cmd)
	prompter, hasPrompter := cmd.(Prompter)

	// Use a single scanner for all prompts to avoid buffering issues.
	scanner := bufio.NewScanner(opts.stdin)

	for i := range defs {
		def := &defs[i]
		if !def.Required || provided[def.Name] {
			continue
		}

		var input string
		var err error

		if hasPrompter {
			input, err = prompter.Prompt(*def)
		} else {
			input, err = readPrompt(*def, opts.stderr, scanner)
		}
		if err != nil {
			return provided, err
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue // validation will catch it
		}

		fieldIdx := flagFieldIndex(v.Type(), def.Name)
		if fieldIdx < 0 {
			continue
		}
		if err := setFieldValue(v.Field(fieldIdx), input); err != nil {
			return provided, fmt.Errorf("%w: --%s: %w", ErrInvalidFlagValue, def.Name, err)
		}
		if provided == nil {
			provided = make(map[string]bool)
		}
		provided[def.Name] = true
	}

	return provided, nil
}

// flagFieldIndex returns the struct field index for a flag name, or -1.
func flagFieldIndex(t reflect.Type, flagName string) int {
	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("flag")
		if name == "" {
			name = camelToKebab(f.Name)
		}
		if _, hasFlag := f.Tag.Lookup("flag"); hasFlag && name == flagName {
			return i
		}
	}
	return -1
}

// readPrompt writes a prompt to w and reads a line using the given scanner.
func readPrompt(flag FlagDef, w io.Writer, scanner *bufio.Scanner) (string, error) {
	label := flag.Help
	if label == "" {
		label = flag.Name
	}
	fmt.Fprintf(w, "%s: ", label) //nolint:errcheck
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}
