package editor

import (
	"sort"
	"testing"

	"github.com/hajianpour/nem/command"
)

// Every command named in the default binding table must actually exist in the
// registry. The two halves are written independently — the table by hand from
// the inventory, the commands by their implementers — so a typo on either side
// is invisible until a user presses the key and nothing happens. This is the
// only test that catches it.
func TestEveryBoundCommandExists(t *testing.T) {
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
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
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
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
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
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
