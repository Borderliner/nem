package lua_test

import (
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/keymap"
	nemlua "github.com/hajianpour/nem/lua"
	"github.com/hajianpour/nem/text"
)

// examples/init.lua is committed and users will copy it verbatim, so it has to
// load cleanly, bind only commands that exist, and have every command it defines
// actually work. A published example that errors on first run is worse than no
// example.
func TestCommittedExampleConfigWorks(t *testing.T) {
	f := commandtest.New("hello  ", "world")
	if err := command.RegisterAll(f.Reg); err != nil {
		t.Fatalf("registering built-ins: %v", err)
	}
	km := keymap.New()
	h, err := nemlua.New(nemlua.Options{
		Registry:   f.Reg,
		Keymap:     km,
		ConfigPath: "../examples/init.lua",
	})
	if err != nil {
		t.Fatalf("lua.New: %v", err)
	}
	t.Cleanup(h.Close)

	if err := h.LoadConfig(); err != nil {
		t.Fatalf("examples/init.lua failed to load: %v", err)
	}

	// No key may point at a command nothing registers: that is a dead key in a
	// config we ship.
	if u := h.UnresolvedBindings(); len(u) > 0 {
		t.Errorf("unresolved bindings in the example config: %v", u)
	}

	// Its settings must reach the Settings struct the editor applies.
	if got := h.Settings().TabWidth; got != 4 {
		t.Errorf("TabWidth = %d, want 4 from the example", got)
	}
	if got := h.Settings().ScrollMargin; got != 3 {
		t.Errorf("ScrollMargin = %d, want 3 from the example", got)
	}

	// Every command it defines must run.
	for _, name := range []string{"reverse-line", "strip-trailing-space", "number-lines"} {
		if _, ok := f.Reg.Lookup(name); !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if err := f.Reg.Run(name, f); err != nil {
			t.Errorf("running %s: %v", name, err)
		}
	}

	// And its hook must fire and actually strip. The example gates on a .go
	// path, so give it one, then put trailing whitespace back for it to find.
	f.Buf().SetPath("/tmp/x.go")
	if err := f.Reg.Run("strip-trailing-space", f); err != nil {
		t.Fatalf("resetting: %v", err)
	}
	seedTrailingWhitespace(t, f)
	if err := h.FireHook("before-save", f, f.Buf()); err != nil {
		t.Fatalf("before-save hook: %v", err)
	}
	if got := f.Text(); strings.Contains(got, " \n") || strings.HasSuffix(got, " ") {
		t.Errorf("buffer = %q, want the before-save hook to have stripped trailing whitespace", got)
	}
}

// seedTrailingWhitespace appends spaces to every line, so a stripping hook has
// something to remove.
func seedTrailingWhitespace(t *testing.T, f *commandtest.Fake) {
	t.Helper()
	b := f.Buf()
	for i := 0; i < b.NumLines(); i++ {
		at := text.Pos{Line: i, Col: b.Line(i).Len()}
		if err := b.Insert(at, []rune("   ")); err != nil {
			t.Fatalf("seeding line %d: %v", i, err)
		}
	}
}

// The bindings the example makes must resolve the way the decoder produces them.
func TestExampleBindingsResolve(t *testing.T) {
	f := commandtest.New("x")
	if err := command.RegisterAll(f.Reg); err != nil {
		t.Fatalf("registering built-ins: %v", err)
	}
	km := keymap.New()
	h, err := nemlua.New(nemlua.Options{Registry: f.Reg, Keymap: km, ConfigPath: "../examples/init.lua"})
	if err != nil {
		t.Fatalf("lua.New: %v", err)
	}
	t.Cleanup(h.Close)
	if err := h.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	for spec, want := range map[string]string{
		"<f5>":  "save-buffer",
		"C-c r": "reverse-line",
		"C-c w": "strip-trailing-space",
	} {
		got := lookup(t, km, spec)
		if got.Kind != keymap.Found || got.Command != want {
			t.Errorf("%s: got kind=%v command=%q, want Found %q", spec, got.Kind, got.Command, want)
		}
	}
}
