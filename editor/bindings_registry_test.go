package editor

import (
	"sort"
	"testing"

	"github.com/Borderliner/nem/command"
)

// Every command named in the default binding table must actually exist in the
// registry. The two halves are written independently — the table by hand from
// the inventory, the commands by their implementers — so a typo on either side
// is invisible until a user presses the key and nothing happens. This is the
// only test that catches it.
func TestEveryBoundCommandExists(t *testing.T) {
	r := editorRegistry(t)
	var missing []string
	for _, b := range defaultBindings {
		if _, ok := r.Lookup(b.Command); !ok {
			missing = append(missing, b.Spec+" -> "+b.Command)
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("bound key names a command no group registers: %s", m)
	}
}

// A bound key must invoke something a user can also reach by name, so anything
// bound should be interactive. A non-interactive command behind a key is either
// a mistake in the table or a command that should have been marked interactive.
func TestBoundCommandsAreInteractive(t *testing.T) {
	r := editorRegistry(t)
	for _, b := range defaultBindings {
		c, ok := r.Lookup(b.Command)
		if !ok {
			continue // reported by TestEveryBoundCommandExists
		}
		if !c.Interactive {
			t.Errorf("%s is bound to %q, which is not interactive", b.Spec, b.Command)
		}
	}
}

// Report interactive commands reachable only through M-x. Not a failure — M-x is
// a legitimate way to reach a command — but a command the inventory intended to
// bind and didn't shows up here.
func TestReportUnboundInteractiveCommands(t *testing.T) {
	r := editorRegistry(t)
	bound := map[string]bool{}
	for _, b := range defaultBindings {
		bound[b.Command] = true
	}
	var unbound []string
	for _, c := range r.All() {
		if c.Interactive && !bound[c.Name] {
			unbound = append(unbound, c.Name)
		}
	}
	sort.Strings(unbound)
	if len(unbound) > 0 {
		t.Logf("%d interactive commands reachable only via M-x: %v", len(unbound), unbound)
	}
}

// editorRegistry returns the registry a running editor resolves keys against.
//
// It is NOT command.NewDefaultRegistry: the editor registers commands of its own
// on top of the built-in groups - recover-file and toggle-line-numbers need
// editor state that Env cannot reach - and a binding to one of those is real.
// Checking the built-in registry alone reported them as dangling.
func editorRegistry(t *testing.T) *command.Registry {
	t.Helper()
	e, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e.reg
}
