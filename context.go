package cli

import (
	"context"
	"reflect"
)

type (
	argStoreKey  struct{}
	userValueKey struct{ name string }
	leafKey      struct{}
)

// Leaf returns the leaf (target) command from the context. The framework
// stores the leaf before [Beforer] hooks run, so parent commands can
// inspect the leaf to make decisions based on consumer-defined interfaces.
// Returns nil if called on a context that did not originate from [Execute].
//
// A common use case is centralized auth: define marker interfaces in your
// application and check them in a root [Beforer]:
//
//	type Authenticated interface{ Authenticate() }
//	type Authorized interface{ Permissions() []string }
//
//	func (r *Root) Before(ctx context.Context) (context.Context, error) {
//	    leaf := cli.Leaf(ctx)
//	    if _, ok := leaf.(Authenticated); !ok {
//	        return ctx, nil // no auth required
//	    }
//	    token, err := auth.Login(ctx)
//	    if err != nil {
//	        return ctx, err
//	    }
//	    ctx = auth.WithToken(ctx, token)
//	    if az, ok := leaf.(Authorized); ok {
//	        if err := auth.Check(token, az.Permissions()); err != nil {
//	            return ctx, err
//	        }
//	    }
//	    return ctx, nil
//	}
func Leaf(ctx context.Context) Commander {
	r, ok := ctx.Value(leafKey{}).(Commander)
	if !ok {
		return nil
	}
	return r
}

// Set stores a named value in the context. The returned context contains
// the value and should be used for subsequent operations.
//
// Use Set in a [Beforer.Before] hook to share computed values with
// downstream commands:
//
//	func (a *App) Before(ctx context.Context) (context.Context, error) {
//	    db, err := openDB(a.DSN)
//	    if err != nil {
//	        return ctx, err
//	    }
//	    return cli.Set(ctx, "db", db), nil
//	}
//
//	func (s *ServeCmd) Run(ctx context.Context) error {
//	    db := cli.Get[*sql.DB](ctx, "db")
//	    // ...
//	}
//
// Note: Flag values are not stored in context. Access flags via struct
// fields directly, using flag inheritance for child commands that need
// parent flag values.
func Set[T any](ctx context.Context, name string, val T) context.Context {
	return context.WithValue(ctx, userValueKey{name}, val)
}

// Get retrieves a named value from the context. It returns the zero value
// of T if the name is not found or if the stored value's type does not match T.
//
// Use [Lookup] when you need to distinguish between a missing value and a
// stored zero value.
//
//	db := cli.Get[*sql.DB](ctx, "db")
func Get[T any](ctx context.Context, name string) T {
	val, _ := Lookup[T](ctx, name)
	return val
}

// Lookup retrieves a named value from the context. It returns the zero
// value and false if the name is not found or if the stored value's type
// does not match T.
//
//	db, ok := cli.Lookup[*sql.DB](ctx, "db")
func Lookup[T any](ctx context.Context, name string) (T, bool) {
	// Check user-set values first (they can shadow arg values).
	if val := ctx.Value(userValueKey{name}); val != nil {
		if typed, ok := val.(T); ok {
			return typed, true
		}
	}

	// Fall back to arg store (for branching command patterns).
	if store, ok := ctx.Value(argStoreKey{}).(map[string]any); ok {
		if raw, exists := store[name]; exists {
			if typed, ok := raw.(T); ok {
				return typed, true
			}
		}
	}

	var zero T
	return zero, false
}

// storeArgs reads all arg-tagged fields from each command in the chain and
// stores their values in a single immutable map. This enables child commands
// to access parent positional args in branching command patterns like:
//
//	app user 42 delete --force
//
// where DeleteCmd needs access to the user ID from UserCmd.
//
// Called after flag parsing and inheritance, before Before hooks.
func storeArgs(ctx context.Context, chain []Commander) context.Context {
	store := make(map[string]any)

	for _, cmd := range chain {
		v := reflect.ValueOf(cmd)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			continue
		}
		storeArgsRecurse(v, v.Type(), "", store)
	}

	if len(store) == 0 {
		return ctx
	}
	return context.WithValue(ctx, argStoreKey{}, store)
}

func storeArgsRecurse(v reflect.Value, t reflect.Type, prefix string, store map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)

		// Named struct with prefix: recurse.
		if f.Type.Kind() == reflect.Struct && !f.Anonymous {
			if pfx := f.Tag.Get("prefix"); pfx != "" {
				storeArgsRecurse(v.Field(i), f.Type, prefix+pfx, store)
				continue
			}
			// Fall through: may be a custom type with arg tag.
		}

		// Anonymous embedded struct: recurse with promotion.
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			storeArgsRecurse(v.Field(i), f.Type, prefix, store)
			continue
		}

		// Only store arg-tagged fields (not flags or standalone env).
		argName, hasArg := f.Tag.Lookup("arg")
		if !hasArg {
			continue
		}
		if argName == "" {
			argName = camelToKebab(f.Name)
		}
		store[prefix+argName] = v.Field(i).Interface()
	}
}
