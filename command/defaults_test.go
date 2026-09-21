package command_test

import (
	"testing"

	"github.com/Borderliner/nem/command"
)

// Every group must register without conflict. A duplicate command name across
// two groups fails here rather than at editor startup.
func TestNewDefaultRegistry(t *testing.T) {
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	if got := len(r.All()); got < 50 {
		t.Errorf("registered %d commands, want at least 50", got)
	}
}

// Registering twice into one registry must fail loudly. Silent replacement
// would let a Lua config shadow a built-in without anyone noticing.
func TestRegisterAllTwiceConflicts(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterAll(r); err != nil {
		t.Fatalf("first RegisterAll: %v", err)
	}
	if err := command.RegisterAll(r); err == nil {
		t.Error("second RegisterAll succeeded, want a duplicate-name error")
	}
}

// Every interactive command needs a doc string: M-x lists them and describe-key
// reports them, so a blank doc is a user-visible hole.
func TestEveryInteractiveCommandIsDocumented(t *testing.T) {
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, c := range r.All() {
		if !c.Interactive {
			continue
		}
		if c.Doc == "" {
			t.Errorf("command %q is interactive but has no doc string", c.Name)
		}
	}
}
