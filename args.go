package cli

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// ScanArgs inspects a command's struct tags and returns positional arg definitions.
// This is exported so custom [HelpRenderer] implementations can inspect a command's args.
func ScanArgs(cmd Commander) []ArgDef {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	var defs []ArgDef
	scanArgsRecurse(v.Type(), &defs)
	return defs
}

func scanArgsRecurse(t reflect.Type, defs *[]ArgDef) {
	for i := range t.NumField() {
		f := t.Field(i)

		// Anonymous embedded struct: promote args.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			scanArgsRecurse(f.Type, defs)
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
}

// populateArgs sets struct fields tagged with `arg` from positional arguments,
// environment variables, and defaults. Also populates any cli.Args field with
// remaining arguments. Returns unconsumed positional arguments.
func populateArgs(cmd Commander, args []string, envPrefix string) ([]string, error) {
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

		// Anonymous embedded struct: recurse into promoted fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := populateArgsRecurse(v.Field(i), f.Type, args, envPrefix, argIdx, argsField); err != nil {
				return err
			}
			continue
		}

		// Check for cli.Args field (no tag needed).
		if f.Type == argsType && f.IsExported() {
			if *argsField != nil {
				return fmt.Errorf("multiple cli.Args fields found; only one is allowed")
			}
			fv := v.Field(i)
			*argsField = &fv
			continue
		}

		name, hasArg := f.Tag.Lookup("arg")
		if !hasArg {
			continue
		}
		if name == "" {
			name = camelToKebab(f.Name)
		}

		field := v.Field(i)
		enumTag := f.Tag.Get("enum")

		// Slice field: consume all remaining args.
		if f.Type.Kind() == reflect.Slice {
			for *argIdx < len(args) {
				elemVal, err := parseScalarValue(f.Type.Elem(), args[*argIdx])
				if err != nil {
					return fmt.Errorf("%w: %s: %w", ErrInvalidArgValue, name, err)
				}
				field.Set(reflect.Append(field, elemVal))
				*argIdx++
			}
			continue
		}

		// Scalar field: consume one positional arg.
		if *argIdx < len(args) {
			if err := setFieldValue(field, args[*argIdx]); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrInvalidArgValue, name, err)
			}
			*argIdx++
			if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
				return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
			}
			continue
		}

		// No positional arg — try env (supports comma-separated names).
		if envTag := f.Tag.Get("env"); envTag != "" {
			found := false
			for _, envVar := range strings.Split(envTag, ",") {
				envVar = strings.TrimSpace(envVar)
				envName := envPrefix + envVar
				envVal, ok := os.LookupEnv(envName)
				if !ok {
					continue
				}
				if err := setFieldValue(field, envVal); err != nil {
					return fmt.Errorf("%w: %s (from %s): %w", ErrInvalidArgValue, name, envName, err)
				}
				if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
					return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
				}
				found = true
				break
			}
			if found {
				continue
			}
		}

		// Try default.
		if def := f.Tag.Get("default"); def != "" {
			if err := setFieldValue(field, def); err != nil {
				return fmt.Errorf("%w: %s: invalid default: %w", ErrInvalidArgValue, name, err)
			}
			if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
				return fmt.Errorf("%w: %s must be one of [%s]", ErrInvalidArgValue, name, enumTag)
			}
			continue
		}

		// Check required.
		if tagBool(f.Tag, "required", true) {
			return fmt.Errorf("%w: %s", ErrRequiredArg, name)
		}
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
