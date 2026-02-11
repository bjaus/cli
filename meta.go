package cli

// Meta provides default implementations for common metadata interfaces.
// Embed it in your command struct to reduce boilerplate for [Namer],
// [Descriptor], [Aliaser], [Categorizer], [Hider], and [Deprecator].
//
// The zero value is useful — if you don't set a name, the framework's default
// (lowercase struct name) is used. Use the builder methods to set values:
//
//	type ServeCmd struct {
//	    cli.Meta
//	    Port int `flag:"port" default:"8080"`
//	}
//
//	// Rely on struct name default ("servecmd")
//	cmd := &ServeCmd{}
//
//	// Or customize with builder methods
//	cmd := &ServeCmd{
//	    Meta: cli.Meta{}.
//	        WithName("serve").
//	        WithDescription("Start the server").
//	        WithAliases("s"),
//	}
//
// You can override any method by defining it on your command type:
//
//	func (s *ServeCmd) Description() string {
//	    return fmt.Sprintf("Start server on port %d", s.Port)
//	}
//
// See [Interfaces] for a complete list of optional interfaces.
type Meta struct {
	name        string
	description string
	aliases     []string
	category    string
	hidden      bool
	deprecated  string
}

// WithName sets the command name. If not set, the framework uses the
// lowercase struct type name.
func (m Meta) WithName(name string) Meta {
	m.name = name
	return m
}

// WithDescription sets the one-line description for help listings.
func (m Meta) WithDescription(description string) Meta {
	m.description = description
	return m
}

// WithAliases sets alternate names for the command.
func (m Meta) WithAliases(aliases ...string) Meta {
	m.aliases = aliases
	return m
}

// WithCategory sets the category for grouping in help output.
func (m Meta) WithCategory(category string) Meta {
	m.category = category
	return m
}

// WithHidden hides the command from help output.
func (m Meta) WithHidden(hidden bool) Meta {
	m.hidden = hidden
	return m
}

// WithDeprecated marks the command as deprecated with a message.
func (m Meta) WithDeprecated(message string) Meta {
	m.deprecated = message
	return m
}

// Name implements [Namer]. Returns empty string if not set, which signals
// the framework to use the default (lowercase struct name).
func (m Meta) Name() string { return m.name }

// Description implements [Descriptor].
func (m Meta) Description() string { return m.description }

// Aliases implements [Aliaser].
func (m Meta) Aliases() []string { return m.aliases }

// Category implements [Categorizer].
func (m Meta) Category() string { return m.category }

// Hidden implements [Hider].
func (m Meta) Hidden() bool { return m.hidden }

// Deprecated implements [Deprecator].
func (m Meta) Deprecated() string { return m.deprecated }
