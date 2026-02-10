package cli

import (
	"fmt"
	"os"
	"reflect"
)

// ScanArgs inspects a command's struct tags and returns positional arg definitions.
// This is exported so custom [HelpRenderer] implementations can inspect a command's args.
func ScanArgs(cmd Runner) []ArgDef {
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
		required := !isSlice // non-slice args required by default
		if req, ok := f.Tag.Lookup("required"); ok {
			required = req == "true"
		}

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
// environment variables, and defaults. Returns unconsumed positional arguments.
func populateArgs(cmd Runner, args []string, envPrefix string) ([]string, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil
	}

	argIdx := 0
	if err := populateArgsRecurse(v, v.Type(), args, envPrefix, &argIdx); err != nil {
		return nil, err
	}

	return args[argIdx:], nil
}

func populateArgsRecurse(v reflect.Value, t reflect.Type, args []string, envPrefix string, argIdx *int) error {
	for i := range t.NumField() {
		f := t.Field(i)

		// Anonymous embedded struct: recurse into promoted fields.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := populateArgsRecurse(v.Field(i), f.Type, args, envPrefix, argIdx); err != nil {
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

		field := v.Field(i)
		enumTag := f.Tag.Get("enum")

		// Slice field: consume all remaining args.
		if f.Type.Kind() == reflect.Slice {
			for *argIdx < len(args) {
				elemVal, err := parseScalarValue(f.Type.Elem(), args[*argIdx])
				if err != nil {
					return fmt.Errorf("invalid value for argument %q: %w", name, err)
				}
				field.Set(reflect.Append(field, elemVal))
				*argIdx++
			}
			continue
		}

		// Scalar field: consume one positional arg.
		if *argIdx < len(args) {
			if err := setFieldValue(field, args[*argIdx]); err != nil {
				return fmt.Errorf("invalid value for argument %q: %w", name, err)
			}
			*argIdx++
			if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
				return fmt.Errorf("%w: argument %s must be one of [%s]", ErrInvalidFlagValue, name, enumTag)
			}
			continue
		}

		// No positional arg — try env.
		if envName := f.Tag.Get("env"); envName != "" {
			if envVal, ok := os.LookupEnv(envPrefix + envName); ok {
				if err := setFieldValue(field, envVal); err != nil {
					return fmt.Errorf("invalid value for argument %q (from %s): %w", name, envPrefix+envName, err)
				}
				if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
					return fmt.Errorf("%w: argument %s must be one of [%s]", ErrInvalidFlagValue, name, enumTag)
				}
				continue
			}
		}

		// Try default.
		if def := f.Tag.Get("default"); def != "" {
			if err := setFieldValue(field, def); err != nil {
				return fmt.Errorf("invalid default for argument %q: %w", name, err)
			}
			if enumTag != "" && !enumContains(enumTag, fmt.Sprint(field.Interface())) {
				return fmt.Errorf("%w: argument %s must be one of [%s]", ErrInvalidFlagValue, name, enumTag)
			}
			continue
		}

		// Check required.
		required := true
		if req, ok := f.Tag.Lookup("required"); ok {
			required = req == "true"
		}
		if required {
			return fmt.Errorf("missing required argument: %s", name)
		}
	}

	return nil
}

// ExactArgs returns an arg validator that requires exactly n arguments.
func ExactArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) != n {
			return fmt.Errorf("expected exactly %d argument(s), got %d", n, len(args))
		}
		return nil
	}
}

// MinArgs returns an arg validator that requires at least n arguments.
func MinArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) < n {
			return fmt.Errorf("expected at least %d argument(s), got %d", n, len(args))
		}
		return nil
	}
}

// MaxArgs returns an arg validator that requires at most n arguments.
func MaxArgs(n int) func([]string) error {
	return func(args []string) error {
		if len(args) > n {
			return fmt.Errorf("expected at most %d argument(s), got %d", n, len(args))
		}
		return nil
	}
}

// RangeArgs returns an arg validator that requires between lo and hi arguments inclusive.
func RangeArgs(lo, hi int) func([]string) error {
	return func(args []string) error {
		if len(args) < lo || len(args) > hi {
			return fmt.Errorf("expected between %d and %d argument(s), got %d", lo, hi, len(args))
		}
		return nil
	}
}

// NoArgs is an arg validator that rejects any arguments.
func NoArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("expected no arguments, got %d", len(args))
	}
	return nil
}
