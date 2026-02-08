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
			Name:      name,
			Short:     f.Tag.Get("short"),
			Help:      f.Tag.Get("help"),
			Default:   f.Tag.Get("default"),
			Env:       f.Tag.Get("env"),
			Enum:      f.Tag.Get("enum"),
			Required:  f.Tag.Get("required") == "true",
			TypeName:  flagTypeName(f.Type),
			IsBool:    f.Type.Kind() == reflect.Bool,
			IsCounter: f.Tag.Get("counter") == "true" && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64),
			Negatable: f.Tag.Get("negatable") == "true" && f.Type.Kind() == reflect.Bool,
		})
	}

	return defs
}

func flagTypeName(t reflect.Type) string {
	if t == reflect.TypeOf(time.Duration(0)) {
		return "duration"
	}

	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "int"
	case reflect.Float64:
		return "float"
	case reflect.Bool:
		return "bool"
	case reflect.Slice:
		return flagTypeName(t.Elem()) + "s"
	case reflect.Map:
		return "key=value"
	default:
		if reflect.PointerTo(t).Implements(reflect.TypeOf((*FlagUnmarshaler)(nil)).Elem()) ||
			t.Implements(reflect.TypeOf((*FlagUnmarshaler)(nil)).Elem()) {
			return "value"
		}
		return t.String()
	}
}

type fieldInfo struct {
	index    int
	def      FlagDef
	provided bool
}

// defaultParseFlags is the built-in flag parser using struct tag reflection.
// It returns remaining args, a set of flag names that were explicitly provided
// (by CLI args or env vars), and any error. Validation is deferred so that
// inheritance can fill in values before required/enum checks run.
func defaultParseFlags(cmd Runner, args []string) ([]string, map[string]bool, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil, nil
	}

	fields := buildFieldMap(v.Type())

	if err := applyDefaultsAndEnv(v, fields); err != nil {
		return nil, nil, err
	}

	remaining, err := parseExplicitFlags(v, args, fields)
	if err != nil {
		return nil, nil, err
	}

	provided := collectProvided(fields)
	return remaining, provided, nil
}

// collectProvided extracts the set of flag names that were explicitly set.
func collectProvided(fields map[string]*fieldInfo) map[string]bool {
	provided := make(map[string]bool)
	for _, fi := range fields {
		if fi.provided {
			provided[fi.def.Name] = true
		}
	}
	return provided
}

func buildFieldMap(t reflect.Type) map[string]*fieldInfo {
	fields := make(map[string]*fieldInfo)

	for i := range t.NumField() {
		f := t.Field(i)
		name := f.Tag.Get("flag")
		if name == "" {
			continue
		}

		fi := &fieldInfo{
			index: i,
			def: FlagDef{
				Name:      name,
				Short:     f.Tag.Get("short"),
				Default:   f.Tag.Get("default"),
				Env:       f.Tag.Get("env"),
				Enum:      f.Tag.Get("enum"),
				Required:  f.Tag.Get("required") == "true",
				IsBool:    f.Type.Kind() == reflect.Bool,
				IsCounter: f.Tag.Get("counter") == "true" && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64),
				Negatable: f.Tag.Get("negatable") == "true" && f.Type.Kind() == reflect.Bool,
			},
		}
		fields["--"+name] = fi
		if fi.def.Short != "" {
			fields["-"+fi.def.Short] = fi
		}
		if fi.def.Negatable {
			fields["--no-"+name] = fi
		}
	}

	return fields
}

func applyDefaultsAndEnv(v reflect.Value, fields map[string]*fieldInfo) error {
	for _, fi := range fields {
		field := v.Field(fi.index)
		if fi.def.Default != "" {
			if err := setFieldValue(field, fi.def.Default); err != nil {
				return fmt.Errorf("%w: invalid default for --%s: %w", ErrInvalidFlagValue, fi.def.Name, err)
			}
		}
		if fi.def.Env != "" {
			if envVal, ok := os.LookupEnv(fi.def.Env); ok {
				if err := setFieldValue(field, envVal); err != nil {
					return fmt.Errorf("%w: --%s (from %s): %w", ErrInvalidFlagValue, fi.def.Name, fi.def.Env, err)
				}
				fi.provided = true
			}
		}
	}
	return nil
}

func parseExplicitFlags(v reflect.Value, args []string, fields map[string]*fieldInfo) ([]string, error) {
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

		if fi.def.IsBool || fi.def.IsCounter {
			switch {
			case fi.def.IsCounter:
				field.SetInt(field.Int() + 1)
			case fi.def.Negatable && strings.HasPrefix(arg, "--no-"):
				field.SetBool(false)
			default:
				field.SetBool(true)
			}
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

	return remaining, nil
}

func validateFlags(v reflect.Value, fields map[string]*fieldInfo) error {
	seen := make(map[string]bool)
	for _, fi := range fields {
		if seen[fi.def.Name] {
			continue
		}
		seen[fi.def.Name] = true
		if fi.def.Required && !fi.provided {
			return fmt.Errorf("%w: --%s", ErrRequiredFlag, fi.def.Name)
		}
		if fi.def.Enum != "" && (fi.provided || fi.def.Default != "") {
			val := fmt.Sprint(v.Field(fi.index).Interface())
			if !enumContains(fi.def.Enum, val) {
				return fmt.Errorf("%w: --%s must be one of [%s]", ErrInvalidFlagValue, fi.def.Name, fi.def.Enum)
			}
		}
	}
	return nil
}

// ValidateFlags runs required and enum checks on a command using the given
// provided set. This is called after flag inheritance so that inherited values
// satisfy required constraints and are checked against enum lists.
func ValidateFlags(cmd Runner, provided map[string]bool) error {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	fields := buildFieldMap(v.Type())
	for key, fi := range fields {
		if provided[fi.def.Name] {
			fi.provided = true
		}
		fields[key] = fi
	}
	return validateFlags(v, fields)
}

func enumContains(enum, val string) bool {
	for _, e := range strings.Split(enum, ",") {
		if strings.TrimSpace(e) == val {
			return true
		}
	}
	return false
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
	case reflect.Slice:
		elemVal, err := parseScalarValue(field.Type().Elem(), value)
		if err != nil {
			return err
		}
		field.Set(reflect.Append(field, elemVal))
	case reflect.Map:
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("expected key=value, got %q", value)
		}
		keyVal, err := parseScalarValue(field.Type().Key(), parts[0])
		if err != nil {
			return err
		}
		valVal, err := parseScalarValue(field.Type().Elem(), parts[1])
		if err != nil {
			return err
		}
		if field.IsNil() {
			field.Set(reflect.MakeMap(field.Type()))
		}
		field.SetMapIndex(keyVal, valVal)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, field.Type())
	}
	return nil
}

// parseScalarValue parses a string into a reflect.Value of the given type.
// Used for slice element and map key/value parsing.
func parseScalarValue(typ reflect.Type, value string) (reflect.Value, error) {
	if typ == reflect.TypeOf(time.Duration(0)) {
		d, err := time.ParseDuration(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(d), nil
	}

	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(value), nil
	case reflect.Int:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(int(n)), nil
	case reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(n), nil
	case reflect.Float64:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(n), nil
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(b), nil
	default:
		return reflect.Value{}, fmt.Errorf("%w: %s", ErrUnsupportedType, typ)
	}
}

// inheritFlags copies matching flag values from parent commands to child
// commands when the child's flag was not explicitly provided. It walks
// parent→child and for each child flag not in its provided set, finds the
// nearest ancestor with the same flag name and compatible type.
func inheritFlags(chain []Runner, provided []map[string]bool) {
	for i := 1; i < len(chain); i++ {
		cv := reflect.ValueOf(chain[i])
		if cv.Kind() == reflect.Ptr {
			cv = cv.Elem()
		}
		if cv.Kind() != reflect.Struct {
			continue
		}

		ct := cv.Type()
		for j := range ct.NumField() {
			cf := ct.Field(j)
			name := cf.Tag.Get("flag")
			if name == "" {
				continue
			}
			if provided[i][name] {
				continue
			}

			// Walk ancestors from nearest to farthest.
			for a := i - 1; a >= 0; a-- {
				pv := reflect.ValueOf(chain[a])
				if pv.Kind() == reflect.Ptr {
					pv = pv.Elem()
				}
				if pv.Kind() != reflect.Struct {
					continue
				}
				pt := pv.Type()
				for k := range pt.NumField() {
					pf := pt.Field(k)
					if pf.Tag.Get("flag") != name {
						continue
					}
					if pf.Type != cf.Type {
						continue
					}
					cv.Field(j).Set(pv.Field(k))
					if provided[i] == nil {
						provided[i] = make(map[string]bool)
					}
					provided[i][name] = true
					goto nextField
				}
			}
		nextField:
		}
	}
}

// inheritTagFields copies values from ancestor flag fields into child fields
// tagged with `inherit:"flagname"`. The child field does not register a CLI
// flag — it silently receives the nearest ancestor's matching flag value.
func inheritTagFields(chain []Runner) {
	for i := 1; i < len(chain); i++ {
		cv := reflect.ValueOf(chain[i])
		if cv.Kind() == reflect.Ptr {
			cv = cv.Elem()
		}
		if cv.Kind() != reflect.Struct {
			continue
		}

		ct := cv.Type()
		for j := range ct.NumField() {
			cf := ct.Field(j)
			inheritName := cf.Tag.Get("inherit")
			if inheritName == "" {
				continue
			}

			// Walk ancestors from nearest to farthest.
			for a := i - 1; a >= 0; a-- {
				pv := reflect.ValueOf(chain[a])
				if pv.Kind() == reflect.Ptr {
					pv = pv.Elem()
				}
				if pv.Kind() != reflect.Struct {
					continue
				}
				pt := pv.Type()
				for k := range pt.NumField() {
					pf := pt.Field(k)
					if pf.Tag.Get("flag") != inheritName {
						continue
					}
					if pf.Type != cf.Type {
						continue
					}
					cv.Field(j).Set(pv.Field(k))
					goto nextInheritField
				}
			}
		nextInheritField:
		}
	}
}
