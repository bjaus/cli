package cli

import (
	"reflect"
	"strings"
)

type commandInfo struct {
	name        string
	description string
	aliases     []string
	hidden      bool
	examples    []Example
}

func resolveInfo(cmd Runner) commandInfo {
	info := commandInfo{
		name: defaultName(cmd),
	}
	if n, ok := cmd.(Namer); ok {
		info.name = n.Name()
	}
	if d, ok := cmd.(Describer); ok {
		info.description = d.Description()
	}
	if a, ok := cmd.(Aliaser); ok {
		info.aliases = a.Aliases()
	}
	if h, ok := cmd.(Hider); ok {
		info.hidden = h.Hidden()
	}
	if e, ok := cmd.(Exampler); ok {
		info.examples = e.Examples()
	}
	return info
}

func defaultName(cmd Runner) string {
	t := reflect.TypeOf(cmd)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return strings.ToLower(t.Name())
}
