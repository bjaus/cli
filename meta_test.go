package cli

import "testing"

func TestMeta(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		m := Meta{}
		if m.Name() != "" {
			t.Errorf("Name() = %q, want empty string", m.Name())
		}
		if m.Description() != "" {
			t.Errorf("Description() = %q, want empty string", m.Description())
		}
	})

	t.Run("builder methods", func(t *testing.T) {
		m := Meta{}.
			WithName("serve").
			WithDescription("Start the server").
			WithAliases("s", "start").
			WithCategory("Server").
			WithHidden(true).
			WithDeprecated("use 'run' instead")

		if m.Name() != "serve" {
			t.Errorf("Name() = %q, want %q", m.Name(), "serve")
		}
		if m.Description() != "Start the server" {
			t.Errorf("Description() = %q, want %q", m.Description(), "Start the server")
		}
		if got := m.Aliases(); len(got) != 2 || got[0] != "s" || got[1] != "start" {
			t.Errorf("Aliases() = %v, want [s start]", got)
		}
		if m.Category() != "Server" {
			t.Errorf("Category() = %q, want %q", m.Category(), "Server")
		}
		if !m.Hidden() {
			t.Error("Hidden() = false, want true")
		}
		if m.Deprecated() != "use 'run' instead" {
			t.Errorf("Deprecated() = %q, want %q", m.Deprecated(), "use 'run' instead")
		}
	})

	t.Run("builder methods are immutable", func(t *testing.T) {
		m1 := Meta{}.WithName("serve")
		m2 := m1.WithAliases("s")

		if len(m1.Aliases()) != 0 {
			t.Error("original Meta was mutated")
		}
		if len(m2.Aliases()) != 1 {
			t.Error("new Meta should have alias")
		}
	})
}

func TestMetaEmbed(t *testing.T) {
	type ServeCmd struct {
		Meta
		Port int
	}

	cmd := &ServeCmd{
		Meta: Meta{}.
			WithName("serve").
			WithDescription("Start the server").
			WithAliases("s"),
		Port: 8080,
	}

	// Verify interface satisfaction via embedding
	var _ Namer = cmd
	var _ Descriptor = cmd
	var _ Aliaser = cmd
	var _ Categorizer = cmd
	var _ Hider = cmd
	var _ Deprecator = cmd

	if cmd.Name() != "serve" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "serve")
	}
	if cmd.Port != 8080 {
		t.Errorf("Port = %d, want %d", cmd.Port, 8080)
	}
}

func TestMetaEmbed_ZeroValue(t *testing.T) {
	type ServeCmd struct {
		Meta
	}

	cmd := &ServeCmd{} // No Meta initialization

	// Empty name should signal framework to use default
	if cmd.Name() != "" {
		t.Errorf("Name() = %q, want empty string (default signal)", cmd.Name())
	}
}
