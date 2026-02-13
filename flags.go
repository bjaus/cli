package cli

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// tagBool checks a boolean struct tag. Returns true if the tag exists with
// any value except "false". This allows concise syntax like `required:""`
// instead of `required:"true"`, while still supporting explicit `required:"false"`
// to override defaults. Boolean tags: required, hidden, counter, negate.
func tagBool(tag reflect.StructTag, key string, defaultVal ...bool) bool {
	val, ok := tag.Lookup(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return false
	}
	return val != "false"
}

// ScanFlags inspects a command's struct tags and returns flag definitions.
// This is exported so custom [HelpRenderer] and [FlagParser] implementations
// can inspect a command's flags.
func ScanFlags(cmd Commander) []FlagDef {
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

		if handleScanPrefixedStruct(f, defs, prefix) {
			continue
		}
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

		*defs = append(*defs, buildFlagDef(f, prefix, name))
	}
}

func handleScanPrefixedStruct(f reflect.StructField, defs *[]FlagDef, prefix string) bool {
	if f.Type.Kind() != reflect.Struct || f.Anonymous {
		return false
	}
	pfx := f.Tag.Get("prefix")
	if pfx == "" {
		return false
	}
	scanFlagsRecurse(f.Type, defs, prefix+pfx)
	return true
}

func buildFlagDef(f reflect.StructField, prefix, name string) FlagDef {
	fullName := prefix + name
	aliases := extractFieldAliases(f, prefix)
	isCounter := tagBool(f.Tag, "counter") && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64 ||
		f.Type.Kind() == reflect.Uint || f.Type.Kind() == reflect.Uint64)

	return FlagDef{
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
		Required:    tagBool(f.Tag, "required"),
		Hidden:      tagBool(f.Tag, "hidden"),
		TypeName:    flagTypeName(f.Type),
		IsBool:      f.Type.Kind() == reflect.Bool,
		IsCounter:   isCounter,
		Negate:      tagBool(f.Tag, "negate") && f.Type.Kind() == reflect.Bool,
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

var (
	timeType = reflect.TypeOf(time.Time{})
	urlType  = reflect.TypeOf(url.URL{})
	ipType   = reflect.TypeOf(net.IP{})
)

func flagTypeName(t reflect.Type) string {
	if t == reflect.TypeOf(time.Duration(0)) {
		return "duration"
	}
	if t == timeType {
		return "time"
	}
	if t == urlType || t == reflect.PointerTo(urlType) {
		return "url"
	}
	if t == ipType {
		return "ip"
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
func hasProcessableFields(cmd Commander) bool {
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
//
// The resolution order is: CLI args first, then env vars, config, and defaults
// as fallbacks for values not explicitly provided. This allows flags like
// --config to be parsed before ConfigProvider.ConfigResolver() is called,
// enabling the config file path to come from the command line.
//
// Priority (highest to lowest): CLI > env > config > default
func defaultParseFlags(cmd Commander, args []string, opts *options) ([]string, map[string]bool, error) {
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

	fields, err := buildFieldMap(v.Type())
	if err != nil {
		return nil, nil, err
	}

	// Phase 1: Parse CLI args first — they have highest priority.
	remaining, err := parseExplicitFlags(v, args, fields, opts)
	if err != nil {
		return nil, nil, err
	}

	// Phase 2: Apply env vars as fallback for fields not set by CLI.
	if err := applyEnv(v, fields, opts.envVarPrefix); err != nil {
		return nil, nil, err
	}

	// Phase 3: Resolve config — ConfigProvider now has access to CLI-parsed
	// values like --config, enabling dynamic config file paths.
	resolver := resolveConfigResolver(cmd, opts)
	if err := applyConfig(v, fields, resolver); err != nil {
		return nil, nil, err
	}

	// Phase 4: Apply defaults as final fallback.
	if err := applyDefaults(v, fields); err != nil {
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

func buildFieldMap(t reflect.Type) (map[string]*fieldInfo, error) {
	fields := make(map[string]*fieldInfo)
	if err := buildFieldMapRecurse(t, fields, nil, "", nil); err != nil {
		return nil, err
	}
	return fields, nil
}

func buildFieldMapRecurse(t reflect.Type, fields map[string]*fieldInfo, indexPath []int, prefix string, parts []string) error {
	for i := range t.NumField() {
		f := t.Field(i)
		currentPath := append(append([]int{}, indexPath...), i)

		if handled, err := handlePrefixedStruct(f, fields, currentPath, prefix, parts); err != nil {
			return err
		} else if handled {
			continue
		}
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := buildFieldMapRecurse(f.Type, fields, currentPath, prefix, parts); err != nil {
				return err
			}
			continue
		}

		name, hasFlag, envOnly := resolveFieldSource(f)
		if !hasFlag && !envOnly {
			continue
		}

		fullName := prefix + name
		aliases := extractFieldAliases(f, prefix)

		if skip, err := checkDuplicateFlag(fields, fullName, envOnly, currentPath); err != nil {
			return err
		} else if skip {
			continue
		}

		// Check short name conflicts.
		if short := f.Tag.Get("short"); short != "" {
			if existing, ok := fields["-"+short]; ok && len(existing.index) == len(currentPath) {
				return fmt.Errorf("%w: -%s (used by --%s and --%s)", ErrDuplicateFlag, short, existing.def.Name, fullName)
			}
		}

		// Check alt name conflicts.
		for _, alias := range aliases {
			if existing, ok := fields["--"+alias]; ok && len(existing.index) == len(currentPath) {
				return fmt.Errorf("%w: --%s (used by --%s and --%s)", ErrDuplicateFlag, alias, existing.def.Name, fullName)
			}
		}

		fi := buildFieldInfo(f, currentPath, parts, name, fullName, aliases, envOnly)
		registerFieldInfo(fields, fi, fullName, envOnly, aliases)
	}
	return nil
}

func handlePrefixedStruct(f reflect.StructField, fields map[string]*fieldInfo, currentPath []int, prefix string, parts []string) (bool, error) {
	if f.Type.Kind() != reflect.Struct || f.Anonymous {
		return false, nil
	}
	pfx := f.Tag.Get("prefix")
	if pfx == "" {
		return false, nil
	}
	part := strings.TrimRight(pfx, "-._/")
	if err := buildFieldMapRecurse(f.Type, fields, currentPath, prefix+pfx, append(parts, part)); err != nil {
		return false, err
	}
	return true, nil
}

func resolveFieldSource(f reflect.StructField) (string, bool, bool) {
	name, hasFlag := f.Tag.Lookup("flag")
	envTag := f.Tag.Get("env")
	_, hasArg := f.Tag.Lookup("arg")
	envOnly := !hasFlag && !hasArg && envTag != ""

	if hasFlag && name == "" {
		name = camelToKebab(f.Name)
	} else if envOnly {
		name = camelToKebab(f.Name)
	}
	return name, hasFlag, envOnly
}

func extractFieldAliases(f reflect.StructField, prefix string) []string {
	raw := f.Tag.Get("alt")
	if raw == "" {
		return nil
	}
	aliases := strings.Split(raw, ",")
	for j := range aliases {
		aliases[j] = prefix + aliases[j]
	}
	return aliases
}

// checkDuplicateFlag returns (skip, error). It returns an error if a duplicate
// flag is found at the same depth (same-struct duplicate). It returns skip=true
// if the existing flag should shadow this one (embedded struct being overridden).
func checkDuplicateFlag(fields map[string]*fieldInfo, fullName string, envOnly bool, currentPath []int) (bool, error) {
	primaryKey := "--" + fullName
	if envOnly {
		primaryKey = ":" + fullName
	}
	existing, ok := fields[primaryKey]
	if !ok {
		return false, nil
	}
	// Same depth means same-struct duplicate — this is an error.
	if len(existing.index) == len(currentPath) {
		return false, fmt.Errorf("%w: --%s", ErrDuplicateFlag, fullName)
	}
	// Shorter existing path means outer field shadows embedded — skip this one.
	if len(existing.index) < len(currentPath) {
		return true, nil
	}
	// Longer existing path means this field shadows embedded — don't skip.
	return false, nil
}

func buildFieldInfo(f reflect.StructField, currentPath []int, parts []string, name, fullName string, aliases []string, envOnly bool) *fieldInfo {
	fieldParts := append(append([]string{}, parts...), name)
	isCounter := tagBool(f.Tag, "counter") && (f.Type.Kind() == reflect.Int || f.Type.Kind() == reflect.Int64 ||
		f.Type.Kind() == reflect.Uint || f.Type.Kind() == reflect.Uint64)

	return &fieldInfo{
		index:   currentPath,
		parts:   fieldParts,
		envOnly: envOnly,
		def: FlagDef{
			Name:        fullName,
			Short:       f.Tag.Get("short"),
			Alt:         aliases,
			Default:     f.Tag.Get("default"),
			Mask:        f.Tag.Get("mask"),
			Env:         f.Tag.Get("env"),
			Enum:        f.Tag.Get("enum"),
			Sep:         f.Tag.Get("sep"),
			Category:    f.Tag.Get("category"),
			Deprecated:  f.Tag.Get("deprecated"),
			Placeholder: f.Tag.Get("placeholder"),
			Required:    tagBool(f.Tag, "required"),
			Hidden:      tagBool(f.Tag, "hidden"),
			IsBool:      f.Type.Kind() == reflect.Bool,
			IsCounter:   isCounter,
			Negate:      tagBool(f.Tag, "negate") && f.Type.Kind() == reflect.Bool,
		},
	}
}

func registerFieldInfo(fields map[string]*fieldInfo, fi *fieldInfo, fullName string, envOnly bool, aliases []string) {
	if envOnly {
		fields[":"+fullName] = fi
		return
	}
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

func applyDefaults(v reflect.Value, fields map[string]*fieldInfo) error {
	for _, fi := range fields {
		if fi.provided || fi.def.Default == "" {
			continue
		}
		field := v.FieldByIndex(fi.index)
		if err := setFieldValue(field, fi.def.Default); err != nil {
			prefix := "--"
			if fi.envOnly {
				prefix = ""
			}
			return fmt.Errorf("%w: invalid default for %s%s: %w", ErrInvalidFlagValue, prefix, fi.def.Name, err)
		}
	}
	return nil
}

func applyConfig(v reflect.Value, fields map[string]*fieldInfo, resolver ConfigResolver) error {
	if resolver == nil {
		return nil
	}
	for _, fi := range fields {
		if fi.provided {
			continue
		}
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
		if fi.provided || fi.def.Env == "" {
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

func resolveConfigResolver(cmd Commander, opts *options) ConfigResolver {
	if cp, ok := cmd.(ConfigProvider); ok {
		return cp.ConfigResolver()
	}
	if opts != nil {
		return opts.configResolver
	}
	return nil
}

func parseExplicitFlags(v reflect.Value, args []string, fields map[string]*fieldInfo, opts *options) ([]string, error) {
	lookup := makeFlagLookup(fields, opts)
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
			if handled, rem, err := parseFlagWithEquals(v, arg, lookup, ignoreUnknown, remaining); handled {
				if err != nil {
					return nil, err
				}
				remaining = rem
				continue
			}
		}

		fi, ok := lookup(arg)
		if !ok {
			rem, err := handleUnknownArg(arg, ignoreUnknown, remaining)
			if err != nil {
				return nil, err
			}
			remaining = rem
			continue
		}

		field := v.FieldByIndex(fi.index)

		if fi.def.IsBool || fi.def.IsCounter {
			setBoolOrCounter(field, fi.def, arg)
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

func makeFlagLookup(fields map[string]*fieldInfo, opts *options) func(string) (*fieldInfo, bool) {
	return func(name string) (*fieldInfo, bool) {
		fi, ok := fields[name]
		if ok || opts == nil || opts.flagNormalizer == nil {
			return fi, ok
		}
		normalized := name
		if strings.HasPrefix(name, "--") {
			normalized = "--" + opts.flagNormalizer(name[2:])
		} else if strings.HasPrefix(name, "-") && len(name) == 2 {
			return nil, false
		}
		fi, ok = fields[normalized]
		return fi, ok
	}
}

func parseFlagWithEquals(v reflect.Value, arg string, lookup func(string) (*fieldInfo, bool), ignoreUnknown bool, remaining []string) (bool, []string, error) {
	eqIdx := strings.Index(arg, "=")
	if eqIdx <= 0 {
		return false, remaining, nil
	}
	name := arg[:eqIdx]
	value := arg[eqIdx+1:]
	fi, ok := lookup(name)
	if !ok {
		if ignoreUnknown {
			return true, append(remaining, arg), nil
		}
		return true, nil, fmt.Errorf("%w: %s", ErrUnknownFlag, name)
	}
	field := v.FieldByIndex(fi.index)
	if err := setFieldValueSep(field, value, fi.def.Sep); err != nil {
		return true, nil, fmt.Errorf("%w: %s: %w", ErrInvalidFlagValue, name, err)
	}
	fi.provided = true
	return true, remaining, nil
}

func handleUnknownArg(arg string, ignoreUnknown bool, remaining []string) ([]string, error) {
	if strings.HasPrefix(arg, "-") {
		if ignoreUnknown {
			return append(remaining, arg), nil
		}
		return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, arg)
	}
	return append(remaining, arg), nil
}

func setBoolOrCounter(field reflect.Value, def FlagDef, arg string) {
	switch {
	case def.IsCounter && (field.Kind() == reflect.Uint || field.Kind() == reflect.Uint64):
		field.SetUint(field.Uint() + 1)
	case def.IsCounter:
		field.SetInt(field.Int() + 1)
	case def.Negate && strings.HasPrefix(arg, "--no-"):
		field.SetBool(false)
	default:
		field.SetBool(true)
	}
}

func validateFlags(v reflect.Value, fields map[string]*fieldInfo) error {
	seen := make(map[string]bool)
	for _, fi := range fields {
		if seen[fi.def.Name] {
			continue
		}
		seen[fi.def.Name] = true

		field := v.FieldByIndex(fi.index)

		if fi.def.Required && !fi.provided {
			if fi.envOnly && fi.def.Env != "" {
				return fmt.Errorf("%w: %s (env: %s)", ErrRequiredFlag, fi.def.Name, fi.def.Env)
			}
			return fmt.Errorf("%w: --%s", ErrRequiredFlag, fi.def.Name)
		}
		if fi.def.Enum != "" && (fi.provided || fi.def.Default != "") {
			val := fmt.Sprint(field.Interface())
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
func ValidateFlags(cmd Commander, provided map[string]bool) error {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	fields, err := buildFieldMap(v.Type())
	if err != nil {
		return err
	}
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
func validateFlagGroups(cmd Commander, provided map[string]bool) error {
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
				return fmt.Errorf("%w: %s", ErrMutuallyExclusive, strings.Join(set, ", "))
			}
		case GroupRequiredTogether:
			if len(set) > 0 && len(set) != len(group.Flags) {
				all := make([]string, len(group.Flags))
				for i, f := range group.Flags {
					all[i] = "--" + f
				}
				return fmt.Errorf("%w: %s", ErrRequiredTogether, strings.Join(all, ", "))
			}
		case GroupOneRequired:
			if len(set) == 0 {
				all := make([]string, len(group.Flags))
				for i, f := range group.Flags {
					all[i] = "--" + f
				}
				return fmt.Errorf("%w (none provided): %s", ErrOneRequired, strings.Join(all, ", "))
			}
			if len(set) > 1 {
				// set already contains --prefixed names
				return fmt.Errorf("%w (multiple provided: %s)", ErrOneRequired, strings.Join(set, ", "))
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
	// Check for FlagUnmarshaler interface.
	if field.CanAddr() {
		if u, ok := field.Addr().Interface().(FlagUnmarshaler); ok {
			return u.UnmarshalFlag(value)
		}
	}

	// Handle special types.
	if handled, err := setSpecialTypeValue(field, value); handled {
		return err
	}

	return setBasicTypeValue(field, value)
}

func setSpecialTypeValue(field reflect.Value, value string) (bool, error) {
	switch field.Type() {
	case timeType:
		t, err := parseTime(value)
		if err != nil {
			return true, err
		}
		field.Set(reflect.ValueOf(t))
		return true, nil
	case reflect.PointerTo(urlType):
		u, err := url.Parse(value)
		if err != nil {
			return true, fmt.Errorf("invalid url: %w", err)
		}
		field.Set(reflect.ValueOf(u))
		return true, nil
	case ipType:
		ip := net.ParseIP(value)
		if ip == nil {
			return true, fmt.Errorf("invalid ip address: %q", value)
		}
		field.Set(reflect.ValueOf(ip))
		return true, nil
	case durationType:
		d, err := time.ParseDuration(value)
		if err != nil {
			return true, err
		}
		field.Set(reflect.ValueOf(d))
		return true, nil
	}
	return false, nil
}

var durationType = reflect.TypeFor[time.Duration]()

func setBasicTypeValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int64:
		return setIntValue(field, value)
	case reflect.Uint, reflect.Uint64:
		return setUintValue(field, value)
	case reflect.Float64:
		return setFloatValue(field, value)
	case reflect.Bool:
		return setBoolValue(field, value)
	case reflect.Slice:
		return appendSliceValue(field, value)
	case reflect.Map:
		return setMapValue(field, value)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, field.Type())
	}
	return nil
}

func setIntValue(field reflect.Value, value string) error {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	field.SetInt(n)
	return nil
}

func setUintValue(field reflect.Value, value string) error {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return err
	}
	field.SetUint(n)
	return nil
}

func setFloatValue(field reflect.Value, value string) error {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	field.SetFloat(n)
	return nil
}

func setBoolValue(field reflect.Value, value string) error {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	field.SetBool(b)
	return nil
}

func appendSliceValue(field reflect.Value, value string) error {
	elemVal, err := parseScalarValue(field.Type().Elem(), value)
	if err != nil {
		return err
	}
	field.Set(reflect.Append(field, elemVal))
	return nil
}

func setMapValue(field reflect.Value, value string) error {
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
	return nil
}

// parseScalarValue parses a string into a reflect.Value of the given type.
// Used for slice element and map key/value parsing.
func parseScalarValue(typ reflect.Type, value string) (reflect.Value, error) {
	if v, ok, err := parseSpecialScalarType(typ, value); ok {
		return v, err
	}
	return parseBasicScalarType(typ, value)
}

func parseSpecialScalarType(typ reflect.Type, value string) (reflect.Value, bool, error) {
	switch typ {
	case durationType:
		d, err := time.ParseDuration(value)
		if err != nil {
			return reflect.Value{}, true, err
		}
		return reflect.ValueOf(d), true, nil
	case timeType:
		t, err := parseTime(value)
		if err != nil {
			return reflect.Value{}, true, err
		}
		return reflect.ValueOf(t), true, nil
	case reflect.PointerTo(urlType):
		u, err := url.Parse(value)
		if err != nil {
			return reflect.Value{}, true, fmt.Errorf("invalid url: %w", err)
		}
		return reflect.ValueOf(u), true, nil
	case ipType:
		ip := net.ParseIP(value)
		if ip == nil {
			return reflect.Value{}, true, fmt.Errorf("invalid ip address: %q", value)
		}
		return reflect.ValueOf(ip), true, nil
	}
	return reflect.Value{}, false, nil
}

func parseBasicScalarType(typ reflect.Type, value string) (reflect.Value, error) {
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
func inheritFlags(chain []Commander, provided []map[string]bool) {
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
	_, hasRequired := f.Tag.Lookup("required")
	isRequired := tagBool(f.Tag, "required")
	_, hasEnum := f.Tag.Lookup("enum")
	_, hasHelp := f.Tag.Lookup("help")
	_, hasMask := f.Tag.Lookup("mask")

	hasSource := hasFlag || hasArg || envTag != ""

	if err := validateRemovedTags(f, hasFlag); err != nil {
		return err
	}
	if err := validateSourceConflicts(f, hasFlag, hasArg, isRequired, hasDefault); err != nil {
		return err
	}
	if err := validateFlagOnlyTags(f, hasFlag); err != nil {
		return err
	}
	if err := validateTypeConstraints(f); err != nil {
		return err
	}
	return validateOrphanTags(f, hasSource, hasDefault, hasRequired, hasEnum, hasHelp, hasMask)
}

func validateRemovedTags(f reflect.StructField, hasFlag bool) error {
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
	return nil
}

func validateSourceConflicts(f reflect.StructField, hasFlag, hasArg, isRequired, hasDefault bool) error {
	if hasFlag && hasArg {
		return fmt.Errorf("%w: field %s: flag and arg are mutually exclusive", ErrInvalidTag, f.Name)
	}
	if isRequired && hasDefault {
		return fmt.Errorf("%w: field %s: required and default are mutually exclusive", ErrInvalidTag, f.Name)
	}
	return nil
}

func validateFlagOnlyTags(f reflect.StructField, hasFlag bool) error {
	valueTags := []string{"short", "alt", "sep", "placeholder", "deprecated", "category"}
	for _, tag := range valueTags {
		if f.Tag.Get(tag) != "" && !hasFlag {
			return fmt.Errorf("%w: field %s: %s requires flag", ErrInvalidTag, f.Name, tag)
		}
	}
	boolTags := []string{"counter", "negate", "hidden"}
	for _, tag := range boolTags {
		if _, ok := f.Tag.Lookup(tag); ok && !hasFlag {
			return fmt.Errorf("%w: field %s: %s requires flag", ErrInvalidTag, f.Name, tag)
		}
	}
	// Empty short tag is a mistake.
	if short, ok := f.Tag.Lookup("short"); ok && short == "" {
		return fmt.Errorf("%w: field %s: short tag cannot be empty", ErrInvalidTag, f.Name)
	}
	return nil
}

func validateTypeConstraints(f reflect.StructField) error {
	kind := f.Type.Kind()
	if tagBool(f.Tag, "counter") {
		if kind != reflect.Int && kind != reflect.Int64 && kind != reflect.Uint && kind != reflect.Uint64 {
			return fmt.Errorf("%w: field %s: counter requires int or uint type", ErrInvalidTag, f.Name)
		}
	}
	if tagBool(f.Tag, "negate") && kind != reflect.Bool {
		return fmt.Errorf("%w: field %s: negate requires bool type", ErrInvalidTag, f.Name)
	}
	if f.Tag.Get("sep") != "" && kind != reflect.Slice {
		return fmt.Errorf("%w: field %s: sep requires slice type", ErrInvalidTag, f.Name)
	}
	if err := validateEnumValues(f); err != nil {
		return err
	}
	return nil
}

// validateEnumValues checks that all enum values can be parsed as the field type.
func validateEnumValues(f reflect.StructField) error {
	enumTag := f.Tag.Get("enum")
	if enumTag == "" {
		return nil
	}

	kind := f.Type.Kind()
	// String types can have any enum values.
	if kind == reflect.String {
		return nil
	}

	vals := strings.Split(enumTag, ",")
	for _, v := range vals {
		if err := checkEnumValueType(v, kind, f.Name); err != nil {
			return err
		}
	}
	return nil
}

func checkEnumValueType(val string, kind reflect.Kind, fieldName string) error {
	var err error
	switch kind {
	case reflect.Int, reflect.Int64:
		_, err = strconv.ParseInt(val, 10, 64)
	case reflect.Uint, reflect.Uint64:
		_, err = strconv.ParseUint(val, 10, 64)
	case reflect.Float64:
		_, err = strconv.ParseFloat(val, 64)
	case reflect.Bool:
		_, err = strconv.ParseBool(val)
	default:
		// Other types: allow any enum values, runtime will validate.
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: field %s: enum value %q is not valid for type %s", ErrInvalidTag, fieldName, val, kind)
	}
	return nil
}

func validateOrphanTags(f reflect.StructField, hasSource, hasDefault, hasRequired, hasEnum, hasHelp, hasMask bool) error {
	orphans := []struct {
		name string
		set  bool
	}{
		{"default", hasDefault},
		{"required", hasRequired},
		{"enum", hasEnum},
		{"help", hasHelp},
		{"mask", hasMask},
	}
	for _, o := range orphans {
		if o.set && !hasSource {
			return fmt.Errorf("%w: field %s: %s requires flag, arg, or env tag", ErrInvalidTag, f.Name, o.name)
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
