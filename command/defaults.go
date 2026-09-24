package command

import "fmt"

// RegisterAll registers every built-in command group into r.
//
// This is the one place the groups are assembled. A group missing from here does
// not exist as far as the rest of the editor is concerned: M-x will not find its
// commands, the Lua config cannot bind them, and the default keymap will point
// at names that resolve to nothing. So adding a group means adding it here, and
// the cross-check test in the editor package fails if a bound key names a
// command no group registers.
func RegisterAll(r *Registry) error {
	groups := []struct {
		name string
		fn   func(*Registry) error
	}{
		{"motion", RegisterMotion},
		{"edit", RegisterEdit},
		{"region", RegisterRegion},
		{"buffers", RegisterBuffers},
		{"search", RegisterSearch},
		{"lines", RegisterLines},
		{"comment", RegisterComment},
	}
	for _, g := range groups {
		if err := g.fn(r); err != nil {
			return fmt.Errorf("registering %s commands: %w", g.name, err)
		}
	}
	return nil
}

// NewDefaultRegistry returns a Registry holding every built-in command.
func NewDefaultRegistry() (*Registry, error) {
	r := NewRegistry()
	if err := RegisterAll(r); err != nil {
		return nil, err
	}
	return r, nil
}
