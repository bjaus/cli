package cli

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ScanFlags inspects a command's struct tags and returns flag definitions.
// This is exported so custom [HelpRenderer] and [FlagParser] implementations
// can inspect a command's flags.
func ScanFlags(cmd Runner) []FlagDef {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	var defs []FlagDef //nolint:prealloc // only flag-tagged fields are appended

	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("flag")
		if name == "" {
			continue
		}

		defs = append(defs, FlagDef{
			Name:     name,
			Short:    f.Tag.Get("short"),
			Help:     f.Tag.Get("help"),
			Default:  f.Tag.Get("default"),
			Env:      f.Tag.Get("env"),
			Required: f.Tag.Get("required") == "true",
			TypeName: flagTypeName(f.Type),
			IsBool:   f.Type.Kind() == reflect.Bool,
		})
	}

	return defs
}

func flagTypeName(t reflect.Type) string {
	if t == reflect.TypeOf(time.Duration(0)) {
		return "duration"
	}

	//exhaustive:enforce
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "int"
	case reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	default:
		if reflect.PointerTo(t).Implements(reflect.TypeOf((*FlagUnmarshaler)(nil)).Elem()) ||
			t.Implements(reflect.TypeOf((*FlagUnmarshaler)(nil)).Elem()) {
			return "value"
		}
		return t.String()
	}
}

// defaultParseFlags is the built-in flag parser using struct tag reflection.
func defaultParseFlags(cmd Runner, args []string) ([]string, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil
	}

	t := v.Type()
	type fieldInfo struct {
		index    int
		def      FlagDef
		provided bool
	}

	fields := make(map[string]*fieldInfo) // --name or -short -> field info

	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("flag")
		if name == "" {
			continue
		}

		fi := &fieldInfo{
			index: i,
			def: FlagDef{
				Name:     name,
				Short:    f.Tag.Get("short"),
				Default:  f.Tag.Get("default"),
				Env:      f.Tag.Get("env"),
				Required: f.Tag.Get("required") == "true",
				IsBool:   f.Type.Kind() == reflect.Bool,
			},
		}
		fields["--"+name] = fi
		if fi.def.Short != "" {
			fields["-"+fi.def.Short] = fi
		}
	}

	// Apply defaults first, then env vars.
	for _, fi := range fields {
		field := v.Field(fi.index)
		if fi.def.Default != "" {
			if err := setFieldValue(field, fi.def.Default); err != nil {
				return nil, fmt.Errorf("%w: invalid default for --%s: %w", ErrInvalidFlagValue, fi.def.Name, err)
			}
		}
		if fi.def.Env != "" {
			if envVal, ok := os.LookupEnv(fi.def.Env); ok {
				if err := setFieldValue(field, envVal); err != nil {
					return nil, fmt.Errorf("%w: --%s (from %s): %w", ErrInvalidFlagValue, fi.def.Name, fi.def.Env, err)
				}
				fi.provided = true
			}
		}
	}

	// Parse explicit flags (highest priority).
	var remaining []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			remaining = append(remaining, args[i+1:]...)
			break
		}

		// Handle --flag=value.
		if strings.HasPrefix(arg, "-") {
			if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
				name := arg[:eqIdx]
				value := arg[eqIdx+1:]
				fi, ok := fields[name]
				if !ok {
					return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, name)
				}
				field := v.Field(fi.index)
				if err := setFieldValue(field, value); err != nil {
					return nil, fmt.Errorf("%w: %s: %w", ErrInvalidFlagValue, name, err)
				}
				fi.provided = true
				continue
			}
		}

		fi, ok := fields[arg]
		if !ok {
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, arg)
			}
			remaining = append(remaining, arg)
			continue
		}

		field := v.Field(fi.index)

		if fi.def.IsBool {
			field.SetBool(true)
			fi.provided = true
			continue
		}

		if i+1 >= len(args) {
			return nil, fmt.Errorf("%w: %s", ErrFlagRequiresVal, arg)
		}
		i++
		if err := setFieldValue(field, args[i]); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalidFlagValue, arg, err)
		}
		fi.provided = true
	}

	// Check required flags.
	seen := make(map[string]bool)
	for _, fi := range fields {
		if seen[fi.def.Name] {
			continue
		}
		seen[fi.def.Name] = true
		if fi.def.Required && !fi.provided {
			return nil, fmt.Errorf("%w: --%s", ErrRequiredFlag, fi.def.Name)
		}
	}

	return remaining, nil
}

func setFieldValue(field reflect.Value, value string) error {
	// Check for FlagUnmarshaler interface. Struct fields are always addressable,
	// and the pointer method set includes value receiver methods.
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(FlagUnmarshaler); ok {
			return u.UnmarshalFlag(value)
		}
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.Set(reflect.ValueOf(d))
			return nil
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(n)
	case reflect.Float64:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(n)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, field.Type())
	}
	return nil
}
