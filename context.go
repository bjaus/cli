package cli

import (
	"context"
	"reflect"
)

type contextStoreKey struct{}
type leafKey struct{}

type contextStore struct {
	values map[string]any
}

// Leaf returns the leaf (target) command from the context. The framework
// stores the leaf before [BeforeRunner] hooks run, so parent commands can
// inspect the leaf to make decisions based on consumer-defined interfaces.
// Returns nil if called on a context that did not originate from [Execute].
//
// A common use case is centralized auth: define marker interfaces in your
// application and check them in a root [BeforeRunner]:
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
func Leaf(ctx context.Context) Runner {
	r, ok := ctx.Value(leafKey{}).(Runner)
	if !ok {
		return nil
	}
	return r
}

// Set stores a named value in the context. The returned context contains
// the value and should be used for subsequent operations.
//
// The framework automatically calls Set for every parsed flag value before
// [BeforeRunner] hooks run, so flag values are available to all commands in
// the chain via [Get] or [Lookup]. User code can also call Set in a
// [BeforeRunner.Before] hook to share arbitrary values with downstream commands:
//
//	func (a *App) Before(ctx context.Context) (context.Context, error) {
//	    db, err := openDB(a.DSN)
//	    if err != nil {
//	        return ctx, err
//	    }
//	    return cli.Set(ctx, "db", db), nil
//	}
//
//	func (s *ServeCmd) Run(ctx context.Context, args []string) error {
//	    db := cli.Get[*sql.DB](ctx, "db")
//	    // ...
//	}
func Set[T any](ctx context.Context, name string, val T) context.Context {
	return setContextValue(ctx, name, val)
}

// Get retrieves a named value from the context. It returns the zero value
// of T if the name is not found or if the stored value's type does not match T.
//
// Use [Lookup] when you need to distinguish between a missing value and a
// stored zero value.
//
//	env := cli.Get[string](ctx, "env")
func Get[T any](ctx context.Context, name string) T {
	val, _ := Lookup[T](ctx, name)
	return val
}

// Lookup retrieves a named value from the context. It returns the zero
// value and false if the name is not found or if the stored value's type
// does not match T.
//
//	env, ok := cli.Lookup[string](ctx, "env")
func Lookup[T any](ctx context.Context, name string) (T, bool) {
	store, ok := ctx.Value(contextStoreKey{}).(*contextStore)
	if !ok {
		var zero T
		return zero, false
	}
	raw, exists := store.values[name]
	if !exists {
		var zero T
		return zero, false
	}
	typed, ok := raw.(T)
	if !ok {
		var zero T
		return zero, false
	}
	return typed, true
}

func setContextValue(ctx context.Context, name string, val any) context.Context {
	store, ok := ctx.Value(contextStoreKey{}).(*contextStore)
	if !ok {
		store = &contextStore{values: make(map[string]any)}
		ctx = context.WithValue(ctx, contextStoreKey{}, store)
	}
	store.values[name] = val
	return ctx
}

// storeFlags reads all flag-tagged and standalone env-tagged fields from each
// command in the chain and stores their current values in the context. Called
// after flag parsing and inheritance, before Before hooks.
func storeFlags(ctx context.Context, chain []Runner) context.Context {
	for _, cmd := range chain {
		v := reflect.ValueOf(cmd)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			continue
		}
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			name, hasFlag := f.Tag.Lookup("flag")
			if hasFlag {
				if name == "" {
					name = camelToKebab(f.Name)
				}
				ctx = setContextValue(ctx, name, v.Field(i).Interface())
				continue
			}
			// Standalone env field: has env tag but no flag or arg tag.
			if f.Tag.Get("env") != "" {
				if _, hasArg := f.Tag.Lookup("arg"); !hasArg {
					name = camelToKebab(f.Name)
					ctx = setContextValue(ctx, name, v.Field(i).Interface())
				}
			}
		}
	}
	return ctx
}
