package cli

import (
	"path/filepath"
	"reflect"
	"strings"
)

// Args is a slice of positional arguments. Commands can declare an Args field
// to receive positional arguments via dependency injection:
//
//	type ServeCmd struct {
//	    Args cli.Args
//	    Port int `flag:"port"`
//	}
//
//	func (s *ServeCmd) Run(ctx context.Context) error {
//	    for _, file := range s.Args {
//	        // process file
//	    }
//	}
type Args []string

// binding holds a registered dependency and its target type.
type binding struct {
	value      any
	targetType reflect.Type // interface type for BindTo, nil for Bind
}

// Bind registers a dependency for injection into command structs.
// The value is matched by its concrete type against struct fields.
//
//	db := openDB()
//	cli.Execute(ctx, root, args, cli.Bind(db))
//
// Commands declare the dependency as a struct field (no tag needed):
//
//	type ServeCmd struct {
//	    DB   *sql.DB
//	    Port int `flag:"port"`
//	}
//
//	func (s *ServeCmd) Run(ctx context.Context) error {
//	    s.DB.Query("SELECT ...") // ready to use
//	}
//
// Fields with flag:, arg:, or env: tags are not eligible for injection.
func Bind(v any) Option {
	return func(o *options) {
		o.bindings = append(o.bindings, binding{value: v})
	}
}

// BindTo registers a dependency as a specific interface type. Use this when
// you want commands to depend on an interface rather than a concrete type.
//
//	cli.Execute(ctx, root, args,
//	    cli.BindTo(redisCache, (*Cache)(nil)),
//	)
//
// Commands declare the interface type:
//
//	type ServeCmd struct {
//	    Store Cache
//	}
func BindTo(v any, iface any) Option {
	ifaceType := reflect.TypeOf(iface)
	if ifaceType.Kind() == reflect.Ptr {
		ifaceType = ifaceType.Elem()
	}
	return func(o *options) {
		o.bindings = append(o.bindings, binding{
			value:      v,
			targetType: ifaceType,
		})
	}
}

// injectBindings populates injectable fields on all commands in the chain.
// A field is injectable if it has no flag:, arg:, or env: tag.
func injectBindings(chain []Runner, bindings []binding) error {
	if len(bindings) == 0 {
		return nil
	}

	// Build type -> value index for fast lookup.
	byType := make(map[reflect.Type]any, len(bindings))
	for _, b := range bindings {
		key := b.targetType
		if key == nil {
			key = reflect.TypeOf(b.value)
		}
		byType[key] = b.value
	}

	for _, cmd := range chain {
		injectIntoStruct(cmd, byType)
	}
	return nil
}

func injectIntoStruct(cmd any, byType map[reflect.Type]any) {
	v := reflect.ValueOf(cmd)
	if v.Kind() != reflect.Ptr {
		return
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Recurse into exported embedded structs.
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			injectIntoStruct(v.Field(i).Addr().Interface(), byType)
			continue
		}

		// Skip fields with CLI tags — they're populated by flag/arg/env parsing.
		if hasCliTag(field) {
			continue
		}

		// Try to find a matching binding.
		val, found := findBinding(field.Type, byType)
		if !found {
			continue
		}

		fv := v.Field(i)
		fv.Set(reflect.ValueOf(val))
	}
}

// hasCliTag returns true if the field has any tag that makes it a CLI input.
func hasCliTag(field reflect.StructField) bool {
	_, hasFlag := field.Tag.Lookup("flag")
	_, hasArg := field.Tag.Lookup("arg")
	_, hasEnv := field.Tag.Lookup("env")
	return hasFlag || hasArg || hasEnv
}

// findBinding looks up a value for the target type. It first tries an exact
// type match, then checks if any bound value implements the target interface.
func findBinding(target reflect.Type, byType map[reflect.Type]any) (any, bool) {
	// Exact type match.
	if val, ok := byType[target]; ok {
		return val, true
	}

	// Interface matching: check if any bound value implements target.
	if target.Kind() == reflect.Interface {
		for _, val := range byType {
			if reflect.TypeOf(val).Implements(target) {
				return val, true
			}
		}
	}

	return nil, false
}

// stripProgramName removes the first argument if it looks like a program path.
// This allows callers to pass os.Args directly instead of os.Args[1:].
func stripProgramName(args []string, cmdName string) []string {
	if len(args) == 0 {
		return args
	}

	first := args[0]

	// Check if first arg looks like a path (contains / or \).
	if strings.ContainsAny(first, `/\`) {
		base := filepath.Base(first)
		// Strip extension for Windows .exe comparison.
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if strings.EqualFold(base, cmdName) {
			return args[1:]
		}
		// Looks like a path but doesn't match — probably still the binary.
		// Be conservative: if it starts with common path prefixes, strip it.
		if strings.HasPrefix(first, "/") || strings.HasPrefix(first, "./") || strings.HasPrefix(first, "../") {
			return args[1:]
		}
	}

	return args
}
