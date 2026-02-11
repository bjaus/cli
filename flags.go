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

	var defs []FlagDef
	scanFlagsRecurse(v.Type(), &defs, "")
	return defs
}

func scanFlagsRecurse(t reflect.Type, defs *[]FlagDef, prefix string) {
	for i := range t.NumField() {
		f := t.Field(i)

		// Named struct with prefix tag: recurse.
		if f.Type.Kind() == reflect.Struct && !f.Anonymous {
			if pfx := f.Tag.Get("prefix"); pfx != "" {
				scanFlagsRecurse(f.Type, defs, prefix+pfx)
				continue
			}
			// Fall through: may be a custom type with flag tag (e.g. FlagUnmarshaler).
		}

		// Anonymous embedded struct: promote fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			scanFlagsRecurse(f.Type, defs, prefix)
			continue
		}

		name, hasFlag := f.Tag.Lookup("flag")
		if !hasFlag {
			continue
		}
		if name == "" {
			name = camelToKebab(f.Name)
		}

		fullName := prefix + name

		var aliases []string
		if raw := f.Tag.Get("alt"); raw != "" {
			aliases = strings.Split(raw, ",")
			for j := range aliases {
				aliases[j] = prefix + aliases[j]
			}
		}

		isCounter := f.Tag.Get("counter") == "true" && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64 ||
			f.Type.Kind() == reflect.Uint || f.Type.Kind() == reflect.Uint64)

		*defs = append(*defs, FlagDef{
			Name:        fullName,
			Short:       f.Tag.Get("short"),
			Alt:         aliases,
			Help:        f.Tag.Get("help"),
			Default:     f.Tag.Get("default"),
			Mask:        f.Tag.Get("mask"),
			Env:         f.Tag.Get("env"),
			Enum:        f.Tag.Get("enum"),
			Sep:         f.Tag.Get("sep"),
			Category:    f.Tag.Get("category"),
			Deprecated:  f.Tag.Get("deprecated"),
			Placeholder: f.Tag.Get("placeholder"),
			Required:    f.Tag.Get("required") == "true",
			Hidden:      f.Tag.Get("hidden") == "true",
			TypeName:    flagTypeName(f.Type),
			IsBool:      f.Type.Kind() == reflect.Bool,
			IsCounter:   isCounter,
			Negate:      f.Tag.Get("negate") == "true" && f.Type.Kind() == reflect.Bool,
		})
	}
}

// camelToKebab converts a CamelCase string to kebab-case.
// HTTPHost → http-host, OutputFormat → output-format, ID → id.
func camelToKebab(s string) string {
	var result []byte
	for i := range len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				prev := s[i-1]
				if prev >= 'a' && prev <= 'z' {
					result = append(result, '-')
				} else if i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' {
					result = append(result, '-')
				}
			}
			result = append(result, c+32) // toLower
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

var timeType = reflect.TypeOf(time.Time{})

func flagTypeName(t reflect.Type) string {
	if t == reflect.TypeOf(time.Duration(0)) {
		return "duration"
	}
	if t == timeType {
		return "time"
	}

	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint64:
		return "uint"
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
	index    []int // field path for nested structs (e.g. [0, 2] for embedded.Field)
	def      FlagDef
	parts    []string // decomposed flag name: prefix segments + base (e.g. ["db", "host"])
	provided bool
	envOnly  bool // standalone env field — not a CLI flag, only env/config/default
}

// hasProcessableFields reports whether cmd has any flag-tagged or standalone
// env-tagged struct fields. This is used by parseFlagChain to decide whether
// to call defaultParseFlags.
func hasProcessableFields(cmd Runner) bool {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	return hasProcessableFieldsRecurse(v.Type())
}

func hasProcessableFieldsRecurse(t reflect.Type) bool {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Struct && !f.Anonymous {
			if f.Tag.Get("prefix") != "" {
				if hasProcessableFieldsRecurse(f.Type) {
					return true
				}
				continue
			}
			// Fall through: may be a custom type with flag/env tag.
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if hasProcessableFieldsRecurse(f.Type) {
				return true
			}
			continue
		}
		if _, ok := f.Tag.Lookup("flag"); ok {
			return true
		}
		if f.Tag.Get("env") != "" {
			return true
		}
	}
	return false
}

// defaultParseFlags is the built-in flag parser using struct tag reflection.
// It returns remaining args, a set of flag names that were explicitly provided
// (by CLI args, config resolver, or env vars), and any error. Validation is
// deferred so that inheritance can fill in values before required/enum checks run.
func defaultParseFlags(cmd Runner, args []string, opts *options) ([]string, map[string]bool, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil, nil
	}

	if err := validateStructTags(v.Type()); err != nil {
		return nil, nil, err
	}

	fields := buildFieldMap(v.Type())

	if err := applyDefaults(v, fields); err != nil {
		return nil, nil, err
	}

	resolver := resolveConfigResolver(cmd, opts)
	if err := applyConfig(v, fields, resolver); err != nil {
		return nil, nil, err
	}

	if err := applyEnv(v, fields, opts.envVarPrefix); err != nil {
		return nil, nil, err
	}

	remaining, err := parseExplicitFlags(v, args, fields, opts)
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
	buildFieldMapRecurse(t, fields, nil, "", nil)
	return fields
}

func buildFieldMapRecurse(t reflect.Type, fields map[string]*fieldInfo, indexPath []int, prefix string, parts []string) {
	for i := range t.NumField() {
		f := t.Field(i)
		currentPath := append(append([]int{}, indexPath...), i)

		// Named struct with prefix tag: recurse with prefix.
		if f.Type.Kind() == reflect.Struct && !f.Anonymous {
			if pfx := f.Tag.Get("prefix"); pfx != "" {
				part := strings.TrimRight(pfx, "-._/")
				buildFieldMapRecurse(f.Type, fields, currentPath, prefix+pfx, append(parts, part))
				continue
			}
			// Fall through: may be a custom type with flag tag (e.g. FlagUnmarshaler).
		}

		// Anonymous embedded struct (non-pointer): promote fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			buildFieldMapRecurse(f.Type, fields, currentPath, prefix, parts)
			continue
		}

		name, hasFlag := f.Tag.Lookup("flag")

		// Standalone env: has env tag but no flag or arg tag.
		envTag := f.Tag.Get("env")
		_, hasArg := f.Tag.Lookup("arg")
		envOnly := !hasFlag && !hasArg && envTag != ""

		if !hasFlag && !envOnly {
			continue
		}

		if hasFlag && name == "" {
			name = camelToKebab(f.Name)
		} else if envOnly {
			name = camelToKebab(f.Name)
		}

		fullName := prefix + name
		fieldParts := append(append([]string{}, parts...), name)

		var aliases []string
		if raw := f.Tag.Get("alt"); raw != "" {
			aliases = strings.Split(raw, ",")
			for j := range aliases {
				aliases[j] = prefix + aliases[j]
			}
		}

		// Depth-based shadowing: shallower fields win (Go promotion semantics).
		primaryKey := "--" + fullName
		if envOnly {
			primaryKey = ":" + fullName
		}
		if existing, ok := fields[primaryKey]; ok && len(existing.index) <= len(currentPath) {
			continue
		}

		isCounter := f.Tag.Get("counter") == "true" && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64 ||
			f.Type.Kind() == reflect.Uint || f.Type.Kind() == reflect.Uint64)

		fi := &fieldInfo{
			index:   currentPath,
			parts:   fieldParts,
			envOnly: envOnly,
			def: FlagDef{
				Name:        fullName,
				Short:       f.Tag.Get("short"),
				Alt:         aliases,
				Default:     f.Tag.Get("default"),
				Mask:        f.Tag.Get("mask"),
				Env:         envTag,
				Enum:        f.Tag.Get("enum"),
				Sep:         f.Tag.Get("sep"),
				Category:    f.Tag.Get("category"),
				Deprecated:  f.Tag.Get("deprecated"),
				Placeholder: f.Tag.Get("placeholder"),
				Required:    f.Tag.Get("required") == "true",
				Hidden:      f.Tag.Get("hidden") == "true",
				IsBool:      f.Type.Kind() == reflect.Bool,
				IsCounter:   isCounter,
				Negate:      f.Tag.Get("negate") == "true" && f.Type.Kind() == reflect.Bool,
			},
		}

		if envOnly {
			fields[primaryKey] = fi
		} else {
			fields["--"+fullName] = fi
			if fi.def.Short != "" {
				fields["-"+fi.def.Short] = fi
			}
			if fi.def.Negate {
				fields["--no-"+fullName] = fi
			}
			for _, alias := range aliases {
				fields["--"+alias] = fi
			}
		}
	}
}

func applyDefaults(v reflect.Value, fields map[string]*fieldInfo) error {
	for _, fi := range fields {
		if fi.def.Default != "" {
			field := v.FieldByIndex(fi.index)
			if err := setFieldValue(field, fi.def.Default); err != nil {
				prefix := "--"
				if fi.envOnly {
					prefix = ""
				}
				return fmt.Errorf("%w: invalid default for %s%s: %w", ErrInvalidFlagValue, prefix, fi.def.Name, err)
			}
		}
	}
	return nil
}

func applyConfig(v reflect.Value, fields map[string]*fieldInfo, resolver ConfigResolver) error {
	if resolver == nil {
		return nil
	}
	for _, fi := range fields {
		val, found := resolver(ConfigKey{Name: fi.def.Name, Parts: fi.parts})
		if !found {
			continue
		}
		field := v.FieldByIndex(fi.index)
		if err := setFieldValueSep(field, val, fi.def.Sep); err != nil {
			prefix := "--"
			if fi.envOnly {
				prefix = ""
			}
			return fmt.Errorf("%w: %s%s (from config): %w", ErrInvalidFlagValue, prefix, fi.def.Name, err)
		}
		fi.provided = true
	}
	return nil
}

func applyEnv(v reflect.Value, fields map[string]*fieldInfo, envPrefix string) error {
	for _, fi := range fields {
		if fi.def.Env == "" {
			continue
		}
		// Support comma-separated env var names: env:"A,B" tries A first, then B.
		for _, envVar := range strings.Split(fi.def.Env, ",") {
			envVar = strings.TrimSpace(envVar)
			envName := envPrefix + envVar
			envVal, ok := os.LookupEnv(envName)
			if !ok {
				continue
			}
			field := v.FieldByIndex(fi.index)
			if err := setFieldValueSep(field, envVal, fi.def.Sep); err != nil {
				prefix := "--"
				if fi.envOnly {
					prefix = ""
				}
				return fmt.Errorf("%w: %s%s (from %s): %w", ErrInvalidFlagValue, prefix, fi.def.Name, envName, err)
			}
			fi.provided = true
			break
		}
	}
	return nil
}

func resolveConfigResolver(cmd Runner, opts *options) ConfigResolver {
	if cp, ok := cmd.(ConfigProvider); ok {
		return cp.ConfigResolver()
	}
	if opts != nil {
		return opts.configResolver
	}
	return nil
}

func parseExplicitFlags(v reflect.Value, args []string, fields map[string]*fieldInfo, opts *options) ([]string, error) {
	lookup := func(name string) (*fieldInfo, bool) {
		fi, ok := fields[name]
		if ok || opts == nil || opts.flagNormalizer == nil {
			return fi, ok
		}
		// Try normalized form.
		normalized := name
		if strings.HasPrefix(name, "--") {
			normalized = "--" + opts.flagNormalizer(name[2:])
		} else if strings.HasPrefix(name, "-") && len(name) == 2 {
			// Short flags are not normalized.
			return nil, false
		}
		fi, ok = fields[normalized]
		return fi, ok
	}

	ignoreUnknown := opts != nil && opts.ignoreUnknown

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
				fi, ok := lookup(name)
				if !ok {
					if ignoreUnknown {
						remaining = append(remaining, arg)
						continue
					}
					return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, name)
				}
				field := v.FieldByIndex(fi.index)
				if err := setFieldValueSep(field, value, fi.def.Sep); err != nil {
					return nil, fmt.Errorf("%w: %s: %w", ErrInvalidFlagValue, name, err)
				}
				fi.provided = true
				continue
			}
		}

		fi, ok := lookup(arg)
		if !ok {
			if strings.HasPrefix(arg, "-") {
				if ignoreUnknown {
					remaining = append(remaining, arg)
					continue
				}
				return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, arg)
			}
			remaining = append(remaining, arg)
			continue
		}

		field := v.FieldByIndex(fi.index)

		if fi.def.IsBool || fi.def.IsCounter {
			switch {
			case fi.def.IsCounter && (field.Kind() == reflect.Uint || field.Kind() == reflect.Uint64):
				field.SetUint(field.Uint() + 1)
			case fi.def.IsCounter:
				field.SetInt(field.Int() + 1)
			case fi.def.Negate && strings.HasPrefix(arg, "--no-"):
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
		if err := setFieldValueSep(field, args[i], fi.def.Sep); err != nil {
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
			if fi.envOnly && fi.def.Env != "" {
				return fmt.Errorf("%w: %s (env: %s)", ErrRequiredFlag, fi.def.Name, fi.def.Env)
			}
			return fmt.Errorf("%w: --%s", ErrRequiredFlag, fi.def.Name)
		}
		if fi.def.Enum != "" && (fi.provided || fi.def.Default != "") {
			val := fmt.Sprint(v.FieldByIndex(fi.index).Interface())
			if !enumContains(fi.def.Enum, val) {
				if fi.envOnly {
					return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidFlagValue, fi.def.Name, fi.def.Enum)
				}
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

// MutuallyExclusive creates a flag group where at most one flag may be set.
func MutuallyExclusive(flags ...string) FlagGroup {
	return FlagGroup{Kind: GroupMutuallyExclusive, Flags: flags}
}

// RequiredTogether creates a flag group where if any flag is set, all must be set.
func RequiredTogether(flags ...string) FlagGroup {
	return FlagGroup{Kind: GroupRequiredTogether, Flags: flags}
}

// OneRequired creates a flag group where exactly one flag must be set.
func OneRequired(flags ...string) FlagGroup {
	return FlagGroup{Kind: GroupOneRequired, Flags: flags}
}

// validateFlagGroups checks FlagGrouper constraints using the provided flag set.
func validateFlagGroups(cmd Runner, provided map[string]bool) error {
	g, ok := cmd.(FlagGrouper)
	if !ok {
		return nil
	}
	for _, group := range g.FlagGroups() {
		var set []string
		for _, f := range group.Flags {
			if provided[f] {
				set = append(set, "--"+f)
			}
		}

		switch group.Kind {
		case GroupMutuallyExclusive:
			if len(set) > 1 {
				return fmt.Errorf("flags %s are mutually exclusive", strings.Join(set, ", "))
			}
		case GroupRequiredTogether:
			if len(set) > 0 && len(set) != len(group.Flags) {
				all := make([]string, len(group.Flags))
				for i, f := range group.Flags {
					all[i] = "--" + f
				}
				return fmt.Errorf("flags %s must be set together", strings.Join(all, ", "))
			}
		case GroupOneRequired:
			if len(set) != 1 {
				all := make([]string, len(group.Flags))
				for i, f := range group.Flags {
					all[i] = "--" + f
				}
				return fmt.Errorf("exactly one of %s is required", strings.Join(all, ", "))
			}
		}
	}
	return nil
}

func enumContains(enum, val string) bool {
	for _, e := range strings.Split(enum, ",") {
		if strings.TrimSpace(e) == val {
			return true
		}
	}
	return false
}

// setFieldValueSep sets a field value, splitting on sep for slice fields.
// If sep is empty, behaves like setFieldValue.
func setFieldValueSep(field reflect.Value, value, sep string) error {
	if sep != "" && field.Kind() == reflect.Slice {
		for _, part := range strings.Split(value, sep) {
			part = strings.TrimSpace(part)
			if err := setFieldValue(field, part); err != nil {
				return err
			}
		}
		return nil
	}
	return setFieldValue(field, value)
}

func setFieldValue(field reflect.Value, value string) error {
	// Check for FlagUnmarshaler interface. Struct fields are always addressable,
	// and the pointer method set includes value receiver methods.
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(FlagUnmarshaler); ok {
			return u.UnmarshalFlag(value)
		}
	}

	if field.Type() == timeType {
		t, err := parseTime(value)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(t))
		return nil
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
	case reflect.Uint, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(n)
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
	if typ == timeType {
		t, err := parseTime(value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(t), nil
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
	case reflect.Uint:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(uint(n)), nil
	case reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
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

// inheritableField describes a flag field with its resolved name, type, and index path.
type inheritableField struct {
	name string
	typ  reflect.Type
	path []int
}

// collectInheritableFields returns all flag-tagged fields with their resolved names and paths.
func collectInheritableFields(t reflect.Type, indexPath []int, prefix string) []inheritableField {
	fields := make([]inheritableField, 0, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		currentPath := append(append([]int{}, indexPath...), i)

		if f.Type.Kind() == reflect.Struct && !f.Anonymous {
			if pfx := f.Tag.Get("prefix"); pfx != "" {
				fields = append(fields, collectInheritableFields(f.Type, currentPath, prefix+pfx)...)
				continue
			}
			// Fall through: may be a custom type with flag tag.
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			fields = append(fields, collectInheritableFields(f.Type, currentPath, prefix)...)
			continue
		}

		name, hasFlag := f.Tag.Lookup("flag")
		if !hasFlag {
			continue
		}
		if name == "" {
			name = camelToKebab(f.Name)
		}
		fields = append(fields, inheritableField{name: prefix + name, typ: f.Type, path: currentPath})
	}
	return fields
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

		childFields := collectInheritableFields(cv.Type(), nil, "")
		for _, cf := range childFields {
			if provided[i][cf.name] {
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

				parentFields := collectInheritableFields(pv.Type(), nil, "")
				for _, pf := range parentFields {
					if pf.name != cf.name || pf.typ != cf.typ {
						continue
					}
					cv.FieldByIndex(cf.path).Set(pv.FieldByIndex(pf.path))
					if provided[i] == nil {
						provided[i] = make(map[string]bool)
					}
					provided[i][cf.name] = true
					goto nextField
				}
			}
		nextField:
		}
	}
}

// validateStructTags checks for invalid or conflicting struct tag combinations.
func validateStructTags(t reflect.Type) error {
	return validateStructTagsRecurse(t)
}

func validateStructTagsRecurse(t reflect.Type) error {
	for i := range t.NumField() {
		f := t.Field(i)

		// prefix tag validation and recursion.
		if pfx := f.Tag.Get("prefix"); pfx != "" {
			if f.Anonymous {
				return fmt.Errorf("%w: field %s: prefix cannot be used on anonymous (embedded) fields", ErrInvalidTag, f.Name)
			}
			if f.Type.Kind() != reflect.Struct {
				return fmt.Errorf("%w: field %s: prefix requires struct type", ErrInvalidTag, f.Name)
			}
			if err := validateStructTagsRecurse(f.Type); err != nil {
				return err
			}
			continue
		}

		// Anonymous embedded struct: recurse into promoted fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := validateStructTagsRecurse(f.Type); err != nil {
				return err
			}
			continue
		}

		if err := validateFieldTags(f); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldTags(f reflect.StructField) error {
	_, hasFlag := f.Tag.Lookup("flag")
	_, hasArg := f.Tag.Lookup("arg")
	envTag := f.Tag.Get("env")
	_, hasDefault := f.Tag.Lookup("default")
	hasRequired := f.Tag.Get("required") == "true"
	_, hasEnum := f.Tag.Lookup("enum")
	_, hasHelp := f.Tag.Lookup("help")
	_, hasMask := f.Tag.Lookup("mask")

	hasSource := hasFlag || hasArg || envTag != ""

	// Removed tag migration errors.
	if f.Tag.Get("inherit") != "" {
		name := f.Tag.Get("inherit")
		return fmt.Errorf("%w: field %s: inherit tag removed; use flag:%q hidden:\"true\" for automatic inheritance", ErrInvalidTag, f.Name, name)
	}
	if f.Tag.Get("default-mask") != "" {
		return fmt.Errorf("%w: field %s: default-mask renamed to mask", ErrInvalidTag, f.Name)
	}
	if hasFlag && f.Tag.Get("flag") == "-" {
		return fmt.Errorf(`%w: field %s: flag:"-" removed; use env:"VAR" without flag tag`, ErrInvalidTag, f.Name)
	}
	if f.Tag.Get("negatable") != "" {
		return fmt.Errorf("%w: field %s: negatable renamed to negate", ErrInvalidTag, f.Name)
	}

	// Mutually exclusive source tags.
	if hasFlag && hasArg {
		return fmt.Errorf("%w: field %s: flag and arg are mutually exclusive", ErrInvalidTag, f.Name)
	}

	// Contradictory constraints.
	if hasRequired && hasDefault {
		return fmt.Errorf("%w: field %s: required and default are mutually exclusive", ErrInvalidTag, f.Name)
	}

	// Tags that require flag.
	flagOnlyTags := map[string]string{
		"short":       f.Tag.Get("short"),
		"counter":     f.Tag.Get("counter"),
		"negate":      f.Tag.Get("negate"),
		"alt":         f.Tag.Get("alt"),
		"sep":         f.Tag.Get("sep"),
		"placeholder": f.Tag.Get("placeholder"),
		"hidden":      f.Tag.Get("hidden"),
		"deprecated":  f.Tag.Get("deprecated"),
		"category":    f.Tag.Get("category"),
	}
	for tag, val := range flagOnlyTags {
		if val != "" && !hasFlag {
			return fmt.Errorf("%w: field %s: %s requires flag", ErrInvalidTag, f.Name, tag)
		}
	}

	// Type-specific constraints.
	if f.Tag.Get("counter") == "true" && f.Type.Kind() != reflect.Int && f.Type.Kind() != reflect.Int64 &&
		f.Type.Kind() != reflect.Uint && f.Type.Kind() != reflect.Uint64 {
		return fmt.Errorf("%w: field %s: counter requires int or uint type", ErrInvalidTag, f.Name)
	}
	if f.Tag.Get("negate") == "true" && f.Type.Kind() != reflect.Bool {
		return fmt.Errorf("%w: field %s: negate requires bool type", ErrInvalidTag, f.Name)
	}
	if f.Tag.Get("sep") != "" && f.Type.Kind() != reflect.Slice {
		return fmt.Errorf("%w: field %s: sep requires slice type", ErrInvalidTag, f.Name)
	}

	// Orphan detection: CLI-related tags without a source.
	orphanTags := []struct {
		name string
		set  bool
	}{
		{"default", hasDefault},
		{"required", hasRequired},
		{"enum", hasEnum},
		{"help", hasHelp},
		{"mask", hasMask},
	}
	for _, ot := range orphanTags {
		if ot.set && !hasSource {
			return fmt.Errorf("%w: field %s: %s requires flag, arg, or env tag", ErrInvalidTag, f.Name, ot.name)
		}
	}
	return nil
}

// timeLayouts are the formats tried when parsing time.Time flag values.
var timeLayouts = []string{
	time.RFC3339,
	time.DateOnly,
	time.DateTime,
}

func parseTime(value string) (time.Time, error) {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as time (expected RFC3339, date, or datetime)", value)
}
