package cli

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/bjaus/bind"
)

// Args captures remaining positional arguments after named args are consumed.
// Declare an Args field (no tag needed) to receive unconsumed arguments:
//
//	type CopyCmd struct {
//	    Src  string   `arg:"src"`   // named arg, gets args[0]
//	    Dst  string   `arg:"dst"`   // named arg, gets args[1]
//	    Args cli.Args               // captures args[2:]
//	}
//
//	func (c *CopyCmd) Run(ctx context.Context) error {
//	    if c.Args.Empty() {
//	        return errors.New("no extra files")
//	    }
//	    for _, file := range c.Args {
//	        // process file
//	    }
//	}
//
// Only one Args field is allowed per command. The field must be of type cli.Args.
type Args []string

// Len returns the number of arguments.
func (a Args) Len() int { return len(a) }

// Empty returns true if there are no arguments.
func (a Args) Empty() bool { return len(a) == 0 }

// First returns the first argument or empty string if none.
func (a Args) First() string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

// Last returns the last argument or empty string if none.
func (a Args) Last() string {
	if len(a) == 0 {
		return ""
	}
	return a[len(a)-1]
}

// Get returns the argument at index i or empty string if out of bounds.
func (a Args) Get(i int) string {
	if i < 0 || i >= len(a) {
		return ""
	}
	return a[i]
}

// Contains returns true if the argument list contains s.
func (a Args) Contains(s string) bool { return slices.Contains(a, s) }

// Index returns the index of s or -1 if not found.
func (a Args) Index(s string) int { return slices.Index(a, s) }

// Tail returns all arguments after the first, or nil if there are 0-1 arguments.
func (a Args) Tail() Args {
	if len(a) <= 1 {
		return nil
	}
	return a[1:]
}

// WithBindings registers dependencies for injection into command structs.
// Uses [bind.Option] from github.com/bjaus/bind.
//
// # Binding Modes
//
// Four binding modes are available:
//
//   - bind.Value(v) — register a value, matched by its concrete type
//   - bind.Interface(v, (*Iface)(nil)) — register a value as an interface type
//   - bind.Provider(func() (T, error)) — lazy factory, called on each injection
//   - bind.Singleton(func() (T, error)) — lazy factory, called once and cached
//
// # Example
//
//	db, _ := sql.Open("postgres", connStr)
//	cli.Execute(ctx, root, args,
//	    cli.WithBindings(
//	        bind.Value(db),                        // inject *sql.DB
//	        bind.Interface(cache, (*Cache)(nil)),  // inject as Cache interface
//	        bind.Singleton(newLogger),             // create once, share across commands
//	        bind.Provider(newRequestID),           // create fresh each time
//	    ),
//	)
//
// # Declaring Dependencies
//
// Commands declare dependencies as struct fields without tags:
//
//	type ServeCmd struct {
//	    DB     *sql.DB   // injected by type
//	    Cache  Cache     // injected by interface
//	    Port   int       `flag:"port"` // NOT injected (has flag tag)
//	}
//
// Fields with flag:, arg:, or env: tags are not eligible for injection.
// The injector matches by exact type first, then by interface compatibility.
//
// # Auto-Bound Types
//
// [Args] (positional arguments) is auto-bound. Commands can declare an Args
// field to receive remaining positional arguments:
//
//	type GrepCmd struct {
//	    Pattern string   `arg:"pattern"`
//	    Args    cli.Args // automatically populated
//	}
//
// # Context Lookup
//
// Bindings are also accessible in Run via [bind.Get]:
//
//	func (s *ServeCmd) Run(ctx context.Context) error {
//	    db := bind.Get[*sql.DB](ctx)
//	    // ...
//	}
//
// Use [bind.Lookup] for optional dependencies (returns value, bool).
func WithBindings(opts ...bind.Option) Option {
	return func(o *options) {
		o.bindOpts = append(o.bindOpts, opts...)
	}
}

// stripProgramName removes the first argument if it looks like a program path.
// This allows callers to pass os.Args directly instead of os.Args[1:].
func stripProgramName(args []string, cmdName string) []string {
	if len(args) == 0 {
		return args
	}

	first := args[0]

	// Check if first arg looks like a path (contains / or \).
	if strings.ContainsAny(first, `/\`) {
		base := filepath.Base(first)
		// Strip extension for Windows .exe comparison.
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if strings.EqualFold(base, cmdName) {
			return args[1:]
		}
		// Looks like a path but doesn't match — probably still the binary.
		// Be conservative: if it starts with common path prefixes, strip it.
		if strings.HasPrefix(first, "/") || strings.HasPrefix(first, "./") || strings.HasPrefix(first, "../") {
			return args[1:]
		}
	}

	return args
}
