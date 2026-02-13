package cli

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// ScanArgs inspects a command's struct tags and returns positional arg definitions.
// This is exported so custom [HelpRenderer] implementations can inspect a command's args.
// If the arg definitions are invalid (e.g., variadic arg not last), returns nil.
func ScanArgs(cmd Commander) []ArgDef {
	defs, _ := scanArgsValidated(cmd)
	return defs
}

// scanArgsValidated inspects a command's struct tags and returns positional arg definitions.
// Returns an error if the arg definitions are invalid (e.g., variadic arg not last).
func scanArgsValidated(cmd Commander) ([]ArgDef, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, nil
	}

	var defs []ArgDef
	var sawVariadic bool
	if err := scanArgsRecurse(v.Type(), &defs, &sawVariadic); err != nil {
		return nil, err
	}
	return defs, nil
}

func scanArgsRecurse(t reflect.Type, defs *[]ArgDef, sawVariadic *bool) error {
	for i := range t.NumField() {
		f := t.Field(i)

		// Anonymous embedded struct: promote args.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := scanArgsRecurse(f.Type, defs, sawVariadic); err != nil {
				return err
			}
			continue
		}

		name, hasArg := f.Tag.Lookup("arg")
		if !hasArg {
			continue
		}
		if name == "" {
			name = camelToKebab(f.Name)
		}

		isSlice := f.Type.Kind() == reflect.Slice

		// Variadic args must come last since they consume all remaining args.
		if *sawVariadic {
			return fmt.Errorf("%w: variadic argument must be last", ErrArgOrder)
		}
		if isSlice {
			*sawVariadic = true
		}

		required := tagBool(f.Tag, "required", !isSlice) // non-slice args required by default

		*defs = append(*defs, ArgDef{
			Name:     name,
			Help:     f.Tag.Get("help"),
			Default:  f.Tag.Get("default"),
			Mask:     f.Tag.Get("mask"),
			Env:      f.Tag.Get("env"),
			Enum:     f.Tag.Get("enum"),
			Required: required,
			TypeName: flagTypeName(f.Type),
			IsSlice:  isSlice,
		})
	}
	return nil
}

// populateArgs sets struct fields tagged with `arg` from positional arguments,
// environment variables, and defaults. Also populates any cli.Args field with
// remaining arguments. Returns unconsumed positional arguments.
func populateArgs(cmd Commander, args []string, envPrefix string) ([]string, error) {
	// Validate arg order before populating (variadic args must come last).
	if _, err := scanArgsValidated(cmd); err != nil {
		return nil, err
	}

	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil
	}

	argIdx := 0
	var argsField *reflect.Value
	if err := populateArgsRecurse(v, v.Type(), args, envPrefix, &argIdx, &argsField); err != nil {
		return nil, err
	}

	remaining := args[argIdx:]

	// If a cli.Args field exists, populate it with remaining args.
	if argsField != nil {
		argsField.Set(reflect.ValueOf(Args(remaining)))
		return nil, nil // all args consumed
	}

	return remaining, nil
}

var argsType = reflect.TypeFor[Args]()

func populateArgsRecurse(v reflect.Value, t reflect.Type, args []string, envPrefix string, argIdx *int, argsField **reflect.Value) error {
	for i := range t.NumField() {
		f := t.Field(i)

		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := populateArgsRecurse(v.Field(i), f.Type, args, envPrefix, argIdx, argsField); err != nil {
				return err
			}
			continue
		}

		if f.Type == argsType && f.IsExported() {
			if *argsField != nil {
				return fmt.Errorf("multiple cli.Args fields found; only one is allowed")
			}
			fv := v.Field(i)
			*argsField = &fv
			continue
		}

		if err := populateArgField(v.Field(i), f, args, envPrefix, argIdx); err != nil {
			return err
		}
	}
	return nil
}

func populateArgField(field reflect.Value, f reflect.StructField, args []string, envPrefix string, argIdx *int) error {
	name, hasArg := f.Tag.Lookup("arg")
	if !hasArg {
		return nil
	}
	if name == "" {
		name = camelToKebab(f.Name)
	}

	enumTag := f.Tag.Get("enum")

	// Slice field: consume all remaining args.
	if f.Type.Kind() == reflect.Slice {
		return populateSliceArg(field, f.Type.Elem(), args, argIdx, name)
	}

	// Scalar field: try positional arg, then env, then default.
	if *argIdx < len(args) {
		return populateScalarFromArg(field, args[*argIdx], argIdx, name, enumTag)
	}
	if envTag := f.Tag.Get("env"); envTag != "" {
		if found, err := populateArgFromEnv(field, envTag, envPrefix, name, enumTag); err != nil || found {
			return err
		}
	}
	if def := f.Tag.Get("default"); def != "" {
		return populateArgFromDefault(field, def, name, enumTag)
	}
	if tagBool(f.Tag, "required", true) {
		return fmt.Errorf("%w: %s", ErrRequiredArg, name)
	}
	return nil
}

func populateSliceArg(field reflect.Value, elemType reflect.Type, args []string, argIdx *int, name string) error {
	for *argIdx < len(args) {
		elemVal, err := parseScalarValue(elemType, args[*argIdx])
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidArgValue, name, err)
		}
		field.Set(reflect.Append(field, elemVal))
		*argIdx++
	}
	return nil
}

func populateScalarFromArg(field reflect.Value, arg string, argIdx *int, name, enumTag string) error {
	if err := setFieldValue(field, arg); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidArgValue, name, err)
	}
	*argIdx++
	if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
		return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
	}
	return nil
}

func populateArgFromEnv(field reflect.Value, envTag, envPrefix, name, enumTag string) (bool, error) {
	for _, envVar := range strings.Split(envTag, ",") {
		envVar = strings.TrimSpace(envVar)
		envName := envPrefix + envVar
		envVal, ok := os.LookupEnv(envName)
		if !ok {
			continue
		}
		if err := setFieldValue(field, envVal); err != nil {
			return false, fmt.Errorf("%w: %s (from %s): %w", ErrInvalidArgValue, name, envName, err)
		}
		if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
			return false, fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
		}
		return true, nil
	}
	return false, nil
}

func populateArgFromDefault(field reflect.Value, def, name, enumTag string) error {
	if err := setFieldValue(field, def); err != nil {
		return fmt.Errorf("%w: %s: invalid default: %w", ErrInvalidArgValue, name, err)
	}
	if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
		return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
	}
	return nil
}

// ExactArgs returns an arg validator that requires exactly n arguments.
func ExactArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) != n {
			return fmt.Errorf("%w: expected exactly %d, got %d", ErrArgCount, n, len(args))
		}
		return nil
	}
}

// MinArgs returns an arg validator that requires at least n arguments.
func MinArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) < n {
			return fmt.Errorf("%w: expected at least %d, got %d", ErrArgCount, n, len(args))
		}
		return nil
	}
}

// MaxArgs returns an arg validator that requires at most n arguments.
func MaxArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) > n {
			return fmt.Errorf("%w: expected at most %d, got %d", ErrArgCount, n, len(args))
		}
		return nil
	}
}

// RangeArgs returns an arg validator that requires between lo and hi arguments inclusive.
func RangeArgs(lo, hi int) func([]string) error {
	return func(args []string) error {
		if len(args) < lo || len(args) > hi {
			return fmt.Errorf("%w: expected between %d and %d, got %d", ErrArgCount, lo, hi, len(args))
		}
		return nil
	}
}

// NoArgs is an arg validator that rejects any arguments.
func NoArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("%w: expected none, got %d", ErrArgCount, len(args))
	}
	return nil
}
