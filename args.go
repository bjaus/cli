package cli

import (
	"fmt"
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

	t := v.Type()
	defs := make([]ArgDef, 0, t.NumField())

	for i := range t.NumField() {
		f := t.Field(i)
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

		defs = append(defs, ArgDef{
			Name:     name,
			Help:     f.Tag.Get("help"),
			Required: required,
			TypeName: flagTypeName(f.Type),
			IsSlice:  isSlice,
		})
	}

	return defs
}

// populateArgs sets struct fields tagged with `arg` from positional arguments.
// Returns unconsumed positional arguments.
func populateArgs(cmd Runner, args []string) ([]string, error) {
	v := reflect.ValueOf(cmd)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return args, nil
	}

	t := v.Type()
	argIdx := 0

	for i := range t.NumField() {
		f := t.Field(i)
		name, hasArg := f.Tag.Lookup("arg")
		if !hasArg {
			continue
		}
		if name == "" {
			name = camelToKebab(f.Name)
		}

		field := v.Field(i)

		// Slice field: consume all remaining args.
		if f.Type.Kind() == reflect.Slice {
			for argIdx < len(args) {
				elemVal, err := parseScalarValue(f.Type.Elem(), args[argIdx])
				if err != nil {
					return nil, fmt.Errorf("invalid value for argument %q: %w", name, err)
				}
				field.Set(reflect.Append(field, elemVal))
				argIdx++
			}
			continue
		}

		// Scalar field: consume one arg.
		if argIdx < len(args) {
			if err := setFieldValue(field, args[argIdx]); err != nil {
				return nil, fmt.Errorf("invalid value for argument %q: %w", name, err)
			}
			argIdx++
			continue
		}

		// No more args — check if required.
		required := true
		if req, ok := f.Tag.Lookup("required"); ok {
			required = req == "true"
		}
		if required {
			return nil, fmt.Errorf("missing required argument: %s", name)
		}
	}

	return args[argIdx:], nil
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
