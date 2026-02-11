package cli

import (
	"reflect"
	"strings"
)

type commandInfo struct {
	name            string
	description     string
	longDescription string
	aliases         []string
	hidden          bool
	examples        []Example
}

func resolveInfo(cmd Commander) commandInfo {
	info := commandInfo{
		name: defaultName(cmd),
	}
	if n, ok := cmd.(Namer); ok {
		if name := n.Name(); name != "" {
			info.name = name
		}
	}
	if d, ok := cmd.(Descriptor); ok {
		if desc := d.Description(); desc != "" {
			info.description = desc
		}
	}
	if ld, ok := cmd.(LongDescriptor); ok {
		if desc := ld.LongDescription(); desc != "" {
			info.longDescription = desc
		}
	}
	if a, ok := cmd.(Aliaser); ok {
		if aliases := a.Aliases(); len(aliases) > 0 {
			info.aliases = aliases
		}
	}
	if h, ok := cmd.(Hider); ok {
		// Note: Hidden() returning false is equivalent to not implementing Hider
		info.hidden = h.Hidden()
	}
	if e, ok := cmd.(Exampler); ok {
		if examples := e.Examples(); len(examples) > 0 {
			info.examples = examples
		}
	}
	return info
}

func defaultName(cmd Commander) string {
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return strings.ToLower(t.Name())
}
