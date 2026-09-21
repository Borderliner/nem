package lua_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/keymap"
	nemlua "github.com/hajianpour/nem/lua"
)

// newHost builds a host over a scratch config file. An empty script means no
// config file at all, which is the new-user case.
func newHost(t *testing.T, script string) (*nemlua.Host, *commandtest.Fake, *keymap.Map) {
	t.Helper()
	f := commandtest.New("hello world", "second line")
	km := keymap.New()
	path := ""
	if script != "" {
		path = filepath.Join(t.TempDir(), "init.lua")
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatalf("writing config: %v", err)
		}
	}
	h, err := nemlua.New(nemlua.Options{Registry: f.Reg, Keymap: km, ConfigPath: path})
	if err != nil {
		t.Fatalf("lua.New: %v", err)
	}
	t.Cleanup(h.Close)
	return h, f, km
}

// mustLoad loads a config that is expected to succeed.
func mustLoad(t *testing.T, h *nemlua.Host) {
	t.Helper()
	if err := h.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
}

// lookup resolves a spec the way the event loop will: parsed, then normalized.
func lookup(t *testing.T, km *keymap.Map, spec string) keymap.Result {
	t.Helper()
	seq, err := keymap.ParseSpec(spec)
	if err != nil {
		t.Fatalf("ParseSpec(%q): %v", spec, err)
	}
	for i := range seq {
		seq[i] = keymap.Normalize(seq[i])
	}
	return km.Lookup(seq)
}

// --- construction -----------------------------------------------------------

func TestNewRequiresRegistryAndKeymap(t *testing.T) {
	f := commandtest.New("x")
	if _, err := nemlua.New(nemlua.Options{Keymap: keymap.New()}); err == nil {
		t.Error("New with no Registry succeeded, want an error")
	}
	if _, err := nemlua.New(nemlua.Options{Registry: f.Reg}); err == nil {
		t.Error("New with no Keymap succeeded, want an error")
	}
}

// Running without a config file is normal, not an error.
func TestMissingConfigIsNotAnError(t *testing.T) {
	h, _, _ := newHost(t, "")
	if err := h.LoadConfig(); err != nil {
		t.Errorf("LoadConfig with no config path: %v", err)
	}

	h2, f, km := newHost(t, "-- placeholder\n")
	_ = f
	_ = km
	mustLoad(t, h2)
	// A path that does not exist behaves the same way.
	h3, f3, km3 := newHost(t, "")
	_, _ = f3, km3
	if err := h3.LoadConfig(); err != nil {
		t.Errorf("LoadConfig for absent file: %v", err)
	}
}

func TestAPIVersionIsExposed(t *testing.T) {
	h, _, _ := newHost(t, `
		if nem.api_version ~= 1 then error("unexpected api_version") end
	`)
	mustLoad(t, h)
}

// --- settings ---------------------------------------------------------------

func TestSettingsReachTheirDestination(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.set("tab-width", 4)
		nem.set("scroll-margin", 3)
		nem.set("undo-style", "linear")
	`)
	mustLoad(t, h)
	got := h.Settings()
	// Built from the defaults with only what the script set overridden, so
	// adding a setting does not break this test and a setting that silently
	// fails to apply still does.
	want := nemlua.DefaultSettings()
	want.TabWidth, want.ScrollMargin, want.UndoStyle = 4, 3, "linear"
	if got != want {
		t.Errorf("Settings() = %+v, want %+v", got, want)
	}
}

func TestDefaultSettingsWhenConfigSetsNothing(t *testing.T) {
	h, _, _ := newHost(t, "-- nothing\n")
	mustLoad(t, h)
	if got, want := h.Settings(), nemlua.DefaultSettings(); got != want {
		t.Errorf("Settings() = %+v, want defaults %+v", got, want)
	}
}

func TestBadSettingsAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, script, wantSubstr string }{
		{"unknown key", `nem.set("tab-with", 4)`, "unknown setting"},
		{"wrong type", `nem.set("tab-width", "four")`, "must be a number"},
		{"fractional", `nem.set("tab-width", 4.5)`, "whole number"},
		{"zero tab width", `nem.set("tab-width", 0)`, "between 1 and 64"},
		{"negative tab width", `nem.set("tab-width", -2)`, "between 1 and 64"},
		{"huge tab width", `nem.set("tab-width", 999)`, "between 1 and 64"},
		{"negative margin", `nem.set("scroll-margin", -1)`, "between 0 and 1000"},
		{"bad undo style", `nem.set("undo-style", "tree")`, "must be \"linear\""},
		{"undo style wrong type", `nem.set("undo-style", 1)`, "must be a string"},
		{"missing value", `nem.set("tab-width")`, "missing value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newHost(t, tc.script)
			err := h.LoadConfig()
			if err == nil {
				t.Fatalf("LoadConfig succeeded, want an error mentioning %q", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSubstr)
			}
			// An invalid setting must not corrupt the others.
			if got := h.Settings().UndoStyle; got != "linear" {
				t.Errorf("UndoStyle = %q after a failed set, want the default", got)
			}
		})
	}
}

// --- bindings ---------------------------------------------------------------

// The binding must be findable by the key the event loop actually produces.
// C-SPC arrives as NUL and C-/ as C-_, so a binding stored unnormalized would
// leave set-mark and undo dead — the exact bug this asserts against.
func TestBindingsAreStoredNormalized(t *testing.T) {
	h, _, km := newHost(t, `
		nem.bind("C-x C-f", "find-file")
		nem.bind("C-SPC", "set-mark-command")
		nem.bind("C-/", "undo")
		nem.bind("M-g M-g", "goto-line")
		nem.bind("<f5>", "save-buffer")
		nem.bind("M-%", "query-replace")
	`)
	mustLoad(t, h)
	for _, tc := range []struct{ spec, want string }{
		{"C-x C-f", "find-file"},
		{"C-SPC", "set-mark-command"},
		{"C-/", "undo"},
		{"C-_", "undo"}, // same physical key as C-/
		{"M-g M-g", "goto-line"},
		{"<f5>", "save-buffer"},
		{"M-%", "query-replace"},
	} {
		got := lookup(t, km, tc.spec)
		if got.Kind != keymap.Found || got.Command != tc.want {
			t.Errorf("%s: got kind=%v command=%q, want Found %q", tc.spec, got.Kind, got.Command, tc.want)
		}
	}
}

func TestRebindingAnExactSequenceReplaces(t *testing.T) {
	h, _, km := newHost(t, `
		nem.bind("<f5>", "save-buffer")
		nem.bind("<f5>", "find-file")
	`)
	mustLoad(t, h)
	if got := lookup(t, km, "<f5>"); got.Command != "find-file" {
		t.Errorf("<f5> = %q, want the later binding find-file", got.Command)
	}
}

func TestBadBindingsAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, script, wantSubstr string }{
		{"malformed spec", `nem.bind("C-", "undo")`, "nem.bind"},
		{"nonsense spec", `nem.bind("<nope>", "undo")`, "nem.bind"},
		{"shift on a rune", `nem.bind("S-a", "undo")`, "nem.bind"},
		{"empty command", `nem.bind("<f5>", "")`, "must not be empty"},
		// Lua coerces a number to a string, so this fails when the spec fails to
		// parse rather than as a type error. Either way it is reported.
		{"number as spec", `nem.bind(42, "undo")`, "nem.bind"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newHost(t, tc.script)
			err := h.LoadConfig()
			if err == nil {
				t.Fatalf("LoadConfig succeeded, want an error mentioning %q", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSubstr)
			}
		})
	}
}

// A prefix conflict must name what it collided with, as docs/config.md promises.
func TestPrefixConflictNamesTheConflict(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.bind("C-c", "undo")
		nem.bind("C-c r", "redo")
	`)
	err := h.LoadConfig()
	if err == nil {
		t.Fatal("binding C-c r over a bound C-c succeeded, want a conflict error")
	}
	if !strings.Contains(err.Error(), "C-c") {
		t.Errorf("conflict error = %q, want it to name C-c", err)
	}
}

// A key bound to a command nothing registers is a dead key, and must be
// reportable rather than silent.
func TestUnresolvedBindingsAreReported(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.bind("<f5>", "no-such-command")
		nem.command("real-one", "A real command.", function() end)
		nem.bind("<f6>", "real-one")
	`)
	mustLoad(t, h)
	got := h.UnresolvedBindings()
	if len(got) != 1 || !strings.Contains(got[0], "no-such-command") {
		t.Errorf("UnresolvedBindings() = %v, want just the <f5> -> no-such-command entry", got)
	}
}

// Binding before defining must work, because a script may legitimately do it.
func TestBindBeforeDefineResolves(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.bind("<f7>", "later")
		nem.command("later", "Defined after being bound.", function() end)
	`)
	mustLoad(t, h)
	if got := h.UnresolvedBindings(); len(got) != 0 {
		t.Errorf("UnresolvedBindings() = %v, want none", got)
	}
}

// --- commands ---------------------------------------------------------------

// A Lua command must be a first-class citizen: in M-x completion, invocable
// through the registry, and able to change the buffer.
func TestLuaCommandIsFirstClass(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("shout", "Upcase the current line.", function()
		  nem.buf.replace_line(nem.buf.line():upper())
		end)
	`)
	mustLoad(t, h)

	var found bool
	for _, n := range f.Reg.Names() {
		if n == "shout" {
			found = true
		}
	}
	if !found {
		t.Fatalf("shout missing from Names() = %v", f.Reg.Names())
	}
	if _, ok := f.Reg.Lookup("shout"); !ok {
		t.Fatal("shout not in the registry")
	}
	if err := f.Reg.Run("shout", f); err != nil {
		t.Fatalf("running shout: %v", err)
	}
	if got := f.Text(); !strings.HasPrefix(got, "HELLO WORLD") {
		t.Errorf("buffer = %q, want the first line upcased", got)
	}
}

// docs/config.md's worked example must actually run, verbatim in spirit.
func TestDocumentedReverseLineExample(t *testing.T) {
	h, f, km := newHost(t, `
		nem.command("reverse-line", "Reverse the characters on the current line.",
		  function()
		    local l = nem.buf.line()
		    nem.buf.replace_line(l:reverse())
		  end)

		nem.bind("C-c r", "reverse-line")
	`)
	mustLoad(t, h)
	if err := f.Reg.Run("reverse-line", f); err != nil {
		t.Fatalf("reverse-line: %v", err)
	}
	if got, want := strings.SplitN(f.Text(), "\n", 2)[0], "dlrow olleh"; got != want {
		t.Errorf("first line = %q, want %q", got, want)
	}
	if got := lookup(t, km, "C-c r"); got.Command != "reverse-line" {
		t.Errorf("C-c r = %q, want reverse-line", got.Command)
	}
}

func TestCommandRequiresDocString(t *testing.T) {
	h, _, _ := newHost(t, `nem.command("bare", "", function() end)`)
	err := h.LoadConfig()
	if err == nil {
		t.Fatal("a command with no doc string was accepted")
	}
	if !strings.Contains(err.Error(), "doc string") {
		t.Errorf("error = %q, want it to mention the doc string", err)
	}
}

func TestDuplicateCommandNameIsRejected(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.command("dup", "First.", function() end)
		nem.command("dup", "Second.", function() end)
	`)
	if err := h.LoadConfig(); err == nil {
		t.Fatal("registering the same command name twice succeeded")
	}
}

func TestNemRunInvokesAnotherCommand(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("inner", "Sets the line.", function()
		  nem.buf.replace_line("inner ran")
		end)
		nem.command("outer", "Calls inner.", function()
		  nem.run("inner")
		end)
	`)
	mustLoad(t, h)
	if err := f.Reg.Run("outer", f); err != nil {
		t.Fatalf("outer: %v", err)
	}
	if got := strings.SplitN(f.Text(), "\n", 2)[0]; got != "inner ran" {
		t.Errorf("first line = %q, want %q", got, "inner ran")
	}
}

// --- hooks ------------------------------------------------------------------

func TestHookRegistrationAndFiring(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.hook("before-save", function(buf)
		  if buf.path:match("%.go$") then
		    nem.buf.replace_line("formatted")
		  end
		end)
	`)
	mustLoad(t, h)
	if got := h.HookNames(); len(got) != 1 || got[0] != "before-save" {
		t.Fatalf("HookNames() = %v, want [before-save]", got)
	}

	b := f.Buf()
	b.SetPath("main.go")
	if err := h.FireHook("before-save", f, b); err != nil {
		t.Fatalf("FireHook: %v", err)
	}
	if got := strings.SplitN(f.Text(), "\n", 2)[0]; got != "formatted" {
		t.Errorf("first line = %q, want the hook to have run", got)
	}
}

// The documented idiom relies on buf.path being a Lua string, and the hook must
// not fire its body for a non-matching path.
func TestHookSeesPathAndCanDecline(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.hook("before-save", function(buf)
		  if buf.path:match("%.go$") then nem.buf.replace_line("changed") end
		end)
	`)
	mustLoad(t, h)
	b := f.Buf()
	b.SetPath("notes.md")
	before := f.Text()
	if err := h.FireHook("before-save", f, b); err != nil {
		t.Fatalf("FireHook: %v", err)
	}
	if f.Text() != before {
		t.Errorf("buffer changed for notes.md; the hook should have declined")
	}
}

func TestFiringAnUnregisteredHookIsANoOp(t *testing.T) {
	h, f, _ := newHost(t, "-- none\n")
	mustLoad(t, h)
	if err := h.FireHook("before-save", f, f.Buf()); err != nil {
		t.Errorf("FireHook with no callbacks: %v", err)
	}
}

func TestBadHookNamesAreRejected(t *testing.T) {
	for _, name := range []string{"befor-save", "save", "before_save", "BEFORE-save", "before-"} {
		t.Run(name, func(t *testing.T) {
			h, _, _ := newHost(t, `nem.hook("`+name+`", function() end)`)
			if err := h.LoadConfig(); err == nil {
				t.Errorf("hook name %q was accepted, want a rejection", name)
			}
		})
	}
}

func TestHooksRunInOrderAndAllRunDespiteOneFailing(t *testing.T) {
	h, f, _ := newHost(t, `
		order = ""
		nem.hook("before-save", function() order = order .. "1" end)
		nem.hook("before-save", function() error("second hook is broken") end)
		nem.hook("before-save", function() order = order .. "3" end)
		nem.command("show-order", "Writes the order string.", function()
		  nem.buf.replace_line(order)
		end)
	`)
	mustLoad(t, h)

	err := h.FireHook("before-save", f, f.Buf())
	if err == nil {
		t.Fatal("FireHook succeeded, want the middle hook's error")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error = %q, want it to mention the broken hook", err)
	}
	// The third hook must still have run: one bad hook cannot disable the rest.
	if err := f.Reg.Run("show-order", f); err != nil {
		t.Fatalf("show-order: %v", err)
	}
	if got := strings.SplitN(f.Text(), "\n", 2)[0]; got != "13" {
		t.Errorf("order = %q, want \"13\" — hook 3 must run despite hook 2 failing", got)
	}
}

// --- the error boundary -----------------------------------------------------

// Every way a script can fail must produce a reportable error and leave the
// editor usable. A panic escaping into the event loop would cost unsaved work.
func TestScriptFailuresNeverEscape(t *testing.T) {
	for _, tc := range []struct{ name, script string }{
		{"syntax error", `nem.set("tab-width" 4)`},
		{"throws at top level", `error("deliberate")`},
		{"nil index at top level", `local x = nil; return x.y`},
		{"calls a nil value", `nem.no_such_function()`},
		{"bind with nonsense", `nem.bind("!!bogus!!", "undo")`},
		{"set with nonsense", `nem.set(nil, nil)`},
		{"hook with a non-function", `nem.hook("before-save", 42)`},
		{"command with a non-function", `nem.command("x", "doc", 42)`},
		{"buf access at load time", `nem.buf.line()`},
		{"run at load time", `nem.run("undo")`},
		// 1 + f() is not a tail call, so this grows the stack and overflows.
		// Written as `return f()` it would be a proper tail call, which loops
		// forever instead: a hang, which no protected call can catch. That case
		// belongs to TestTimeoutStopsARunawayScript.
		{"stack overflow", `local function f() return 1 + f() end; f()`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, f, _ := newHost(t, tc.script)
			err := h.LoadConfig() // must not panic
			if err == nil {
				t.Fatal("LoadConfig succeeded, want an error")
			}
			// The host must still be usable afterwards.
			if got := h.Settings(); got.UndoStyle == "" {
				t.Error("Settings() unusable after a failed load")
			}
			if err := h.FireHook("after-save", f, f.Buf()); err != nil {
				t.Errorf("host unusable after a failed load: %v", err)
			}
			_ = h.HookNames()
			_ = h.UnresolvedBindings()
		})
	}
}

// A Lua command that throws when invoked must return an error, not panic.
func TestThrowingCommandReturnsAnError(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("boom", "Throws.", function() error("command exploded") end)
		nem.command("nilboom", "Indexes nil.", function() local x = nil; return x.y end)
	`)
	mustLoad(t, h)
	for _, name := range []string{"boom", "nilboom"} {
		err := f.Reg.Run(name, f)
		if err == nil {
			t.Errorf("%s returned nil, want an error", name)
			continue
		}
		var se *nemlua.ScriptError
		if !errors.As(err, &se) {
			t.Errorf("%s error = %T, want a *ScriptError", name, err)
		}
	}
}

// The traceback is what makes a config error diagnosable at all.
func TestScriptErrorCarriesTraceback(t *testing.T) {
	h, _, _ := newHost(t, "\n\n\nerror(\"find me\")\n")
	err := h.LoadConfig()
	if err == nil {
		t.Fatal("want an error")
	}
	var se *nemlua.ScriptError
	if !errors.As(err, &se) {
		t.Fatalf("error = %T, want *ScriptError", err)
	}
	if !strings.Contains(se.Brief(), "find me") {
		t.Errorf("Brief() = %q, want it to contain the message", se.Brief())
	}
	if se.Traceback == "" {
		t.Error("Traceback is empty; a config error without a location is nearly useless")
	}
	if !strings.Contains(se.Error(), se.Traceback) {
		t.Error("Error() should include the traceback")
	}
}

// Work a script completed before failing must stand, so a typo on line eleven
// does not discard ten good bindings.
func TestPartialConfigEffectsPersist(t *testing.T) {
	h, _, km := newHost(t, `
		nem.set("tab-width", 4)
		nem.bind("<f5>", "save-buffer")
		error("failing after doing useful work")
	`)
	if err := h.LoadConfig(); err == nil {
		t.Fatal("want an error")
	}
	if got := h.Settings().TabWidth; got != 4 {
		t.Errorf("TabWidth = %d, want the 4 set before the failure", got)
	}
	if got := lookup(t, km, "<f5>"); got.Command != "save-buffer" {
		t.Errorf("<f5> = %q, want the binding made before the failure", got.Command)
	}
}

// A runaway script is a hang, which a protected call cannot catch — the timeout
// is the only thing that saves the editor, so the mechanism must work.
func TestTimeoutStopsARunawayScript(t *testing.T) {
	f := commandtest.New("x")
	path := filepath.Join(t.TempDir(), "init.lua")
	if err := os.WriteFile(path, []byte(`while true do end`), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	h, err := nemlua.New(nemlua.Options{
		Registry: f.Reg, Keymap: keymap.New(), ConfigPath: path,
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(h.Close)

	done := make(chan error, 1)
	go func() { done <- h.LoadConfig() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a runaway script returned nil, want a timeout error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LoadConfig did not return; the timeout did not fire")
	}
}

// --- capability boundary ----------------------------------------------------

// docs/config.md promises scripts get the nem table and nothing else. A script
// must not be able to reach the filesystem or spawn a process.
func TestDangerousLibrariesAreAbsent(t *testing.T) {
	for _, global := range []string{"io", "os", "debug", "package", "require", "dofile", "loadfile", "coroutine"} {
		t.Run(global, func(t *testing.T) {
			h, _, _ := newHost(t, `if `+global+` ~= nil then error("`+global+` is reachable") end`)
			if err := h.LoadConfig(); err != nil {
				t.Errorf("%s is reachable from a script: %v", global, err)
			}
		})
	}
}

// The pure-computation libraries a config legitimately needs must be present.
func TestUsefulLibrariesArePresent(t *testing.T) {
	h, _, _ := newHost(t, `
		assert(("abc"):upper() == "ABC", "string lib missing")
		assert(math.max(1, 2) == 2, "math lib missing")
		assert(type(table.concat) == "function", "table lib missing")
		assert(type(tostring) == "function", "base lib missing")
	`)
	mustLoad(t, h)
}

// --- buffer surface ---------------------------------------------------------

func TestBufAccessorsOutsideACommandFail(t *testing.T) {
	for _, call := range []string{
		"nem.buf.line()", "nem.buf.replace_line('x')", "nem.buf.point()",
		"nem.buf.set_point(1, 1)", "nem.buf.path()", "nem.buf.modified()", "nem.buf.text()",
	} {
		t.Run(call, func(t *testing.T) {
			h, _, _ := newHost(t, call)
			err := h.LoadConfig()
			if err == nil {
				t.Fatalf("%s at load time succeeded, want an error", call)
			}
			if !strings.Contains(err.Error(), "no editor context") {
				t.Errorf("error = %q, want it to explain there is no editor context", err)
			}
		})
	}
}

func TestBufPointRoundTrip(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("probe", "Reports and moves point.", function()
		  local l, c = nem.buf.point()
		  nem.buf.set_point(2, 3)
		  local l2, c2 = nem.buf.point()
		  nem.buf.replace_line(l .. "," .. c .. " -> " .. l2 .. "," .. c2)
		end)
	`)
	mustLoad(t, h)
	if err := f.Reg.Run("probe", f); err != nil {
		t.Fatalf("probe: %v", err)
	}
	// Point starts at line 1 col 1 in 1-based terms, then moves to 2,3; the
	// replaced line is the one point ends on.
	if got := f.Text(); !strings.Contains(got, "1,1 -> 2,3") {
		t.Errorf("buffer = %q, want it to contain \"1,1 -> 2,3\"", got)
	}
}

func TestBufPathAndModified(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("probe", "Reports path and modified.", function()
		  nem.buf.replace_line(nem.buf.path() .. "|" .. tostring(nem.buf.modified()))
		end)
	`)
	mustLoad(t, h)
	f.Buf().SetPath("main.go")
	if err := f.Reg.Run("probe", f); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got := strings.SplitN(f.Text(), "\n", 2)[0]; !strings.HasPrefix(got, "main.go|") {
		t.Errorf("first line = %q, want it to start with main.go|", got)
	}
}

func TestReplaceLineRejectsNewlines(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("bad", "Inserts a newline.", function()
		  nem.buf.replace_line("a\nb")
		end)
	`)
	mustLoad(t, h)
	err := f.Reg.Run("bad", f)
	if err == nil {
		t.Fatal("replace_line with a newline succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "newline") {
		t.Errorf("error = %q, want it to mention the newline", err)
	}
}

func TestBufTextReturnsWholeBuffer(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("count", "Counts buffer lines.", function()
		  local n = 0
		  for _ in nem.buf.text():gmatch("[^\n]+") do n = n + 1 end
		  nem.buf.replace_line("lines=" .. n)
		end)
	`)
	mustLoad(t, h)
	if err := f.Reg.Run("count", f); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got := f.Text(); !strings.Contains(got, "lines=2") {
		t.Errorf("buffer = %q, want lines=2", got)
	}
}

// A command invoked through Env.Run from inside Lua must still see a buffer:
// the editor context has to nest.
func TestEditorContextNests(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("inner", "Reads the buffer.", function()
		  if nem.buf.line() == nil then error("no buffer in inner") end
		  nem.buf.replace_line("inner saw the buffer")
		end)
		nem.command("outer", "Runs inner then reads the buffer again.", function()
		  nem.run("inner")
		  local l = nem.buf.line()
		  if l ~= "inner saw the buffer" then error("outer lost the buffer: " .. tostring(l)) end
		end)
	`)
	mustLoad(t, h)
	if err := f.Reg.Run("outer", f); err != nil {
		t.Fatalf("outer: %v", err)
	}
}

var _ = command.ErrQuit // keep the command import honest if assertions change

// --- small surface --------------------------------------------------------

func TestDefaultConfigPath(t *testing.T) {
	got, err := nemlua.DefaultConfigPath()
	if err != nil {
		t.Skipf("no user config dir in this environment: %v", err)
	}
	if !strings.HasSuffix(got, "/nem/init.lua") {
		t.Errorf("DefaultConfigPath() = %q, want it to end in /nem/init.lua", got)
	}
}

func TestHookCount(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.hook("before-save", function() end)
		nem.hook("before-save", function() end)
		nem.hook("after-save", function() end)
	`)
	mustLoad(t, h)
	for _, tc := range []struct {
		name string
		want int
	}{
		{"before-save", 2},
		{"after-save", 1},
		{"before-nothing", 0},
	} {
		if got := h.HookCount(tc.name); got != tc.want {
			t.Errorf("HookCount(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Kind separates a config that does not parse from one that failed while
// running, which is the distinction that matters when reporting to a user.
// It does not go finer than that: an explicit error() call and a VM fault are
// both "runtime", because gopher-lua only reports ApiErrorError when an error
// handler function is installed and this host does not use one.
func TestScriptErrorKinds(t *testing.T) {
	for _, tc := range []struct{ name, script, wantKind string }{
		{"syntax", `nem.set("x" 1)`, "syntax"},
		{"explicit error call", `error("boom")`, "runtime"},
		// Calling a nil value is a VM fault, which gopher-lua classes as
		// "runtime", distinct from an explicit error() call. Worth pinning: the
		// two read identically to a user but differently to a caller.
		{"nil call", `nem.nope()`, "runtime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, _ := newHost(t, tc.script)
			err := h.LoadConfig()
			if err == nil {
				t.Fatal("want an error")
			}
			var se *nemlua.ScriptError
			if !errors.As(err, &se) {
				t.Fatalf("error = %T, want *ScriptError", err)
			}
			if se.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q (message: %s)", se.Kind, tc.wantKind, se.Brief())
			}
		})
	}
}

// A ScriptError with no traceback must still render its message.
func TestScriptErrorWithoutTracebackRenders(t *testing.T) {
	se := &nemlua.ScriptError{Msg: "bare message"}
	if got := se.Error(); got != "bare message" {
		t.Errorf("Error() = %q, want %q", got, "bare message")
	}
}

// nem.run for an unregistered command must report, not panic.
func TestNemRunUnknownCommand(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.command("caller", "Calls a missing command.", function()
		  nem.run("no-such-command")
		end)
	`)
	mustLoad(t, h)
	err := f.Reg.Run("caller", f)
	if err == nil {
		t.Fatal("nem.run of an unknown command succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no-such-command") {
		t.Errorf("error = %q, want it to name the missing command", err)
	}
}

// A hook fired with no buffer must not crash: the editor may fire a hook
// before any buffer exists.
func TestFireHookWithNilBuffer(t *testing.T) {
	h, f, _ := newHost(t, `
		nem.hook("after-save", function(buf)
		  if buf ~= nil then error("expected nil buffer") end
		end)
	`)
	mustLoad(t, h)
	if err := h.FireHook("after-save", f, nil); err != nil {
		t.Errorf("FireHook with a nil buffer: %v", err)
	}
}

// Bindings a script makes must match the keys the decoder actually produces,
// which are the normalized ones: Ctrl+Space arrives as C-@ and C-/ as C-_.
//
// This looks up RAW keys, without calling Normalize, so it tests the guarantee
// rather than restating the mechanism. If keymap ever stopped normalizing on
// Bind or Lookup, set-mark-command and undo would become unreachable from a
// config and this test would catch it here, at the layer a user would notice.
func TestBoundKeysMatchWhatTheDecoderProduces(t *testing.T) {
	h, _, km := newHost(t, `
		nem.bind("C-SPC", "set-mark-command")
		nem.bind("C-/", "undo")
		nem.bind("C-i", "indent-for-tab-command")
	`)
	mustLoad(t, h)

	for _, tc := range []struct {
		name string
		key  keymap.Key
		want string
	}{
		{"Ctrl+Space arrives as C-@", keymap.Key{Rune: '@', Ctrl: true}, "set-mark-command"},
		{"C-/ arrives as C-_", keymap.Key{Rune: '_', Ctrl: true}, "undo"},
		{"C-i arrives as <tab>", keymap.Key{Special: keymap.KeyTab}, "indent-for-tab-command"},
		// The spec-shaped forms must resolve too, since Lookup normalizes.
		{"literal C-SPC also resolves", keymap.Key{Rune: ' ', Ctrl: true}, "set-mark-command"},
		{"literal C-/ also resolves", keymap.Key{Rune: '/', Ctrl: true}, "undo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := km.Lookup([]keymap.Key{tc.key})
			if got.Kind != keymap.Found || got.Command != tc.want {
				t.Errorf("Lookup(%#v) = kind=%v command=%q, want Found %q",
					tc.key, got.Kind, got.Command, tc.want)
			}
		})
	}
}

// Every new setting must validate, apply, and reject bad input. A silently
// ignored setting is the worst outcome: the user reads their config, sees the
// line, and cannot work out why it does nothing.
func TestV2SettingsApplyAndValidate(t *testing.T) {
	h, _, _ := newHost(t, `
		nem.set("completion-style", "bottom")
		nem.set("completion-rows", 15)
		nem.set("which-key-delay", 0)
		nem.set("autosave-idle", 60)
		nem.set("backup", false)
		nem.set("clipboard", "off")
		nem.set("line-numbers", false)
		nem.set("syntax", false)
		nem.set("theme", "light")
	`)
	mustLoad(t, h)
	got := h.Settings()
	for _, c := range []struct {
		name string
		got  any
		want any
	}{
		{"completion-style", got.CompletionStyle, "bottom"},
		{"completion-rows", got.CompletionRows, 15},
		{"which-key-delay", got.WhichKeyDelay, 0},
		{"autosave-idle", got.AutosaveIdle, 60},
		{"backup", got.Backup, false},
		{"clipboard", got.Clipboard, "off"},
		{"line-numbers", got.LineNumbers, false},
		{"syntax", got.Syntax, false},
		{"theme", got.Theme, "light"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestV2SettingsRejectBadValues(t *testing.T) {
	for _, script := range []string{
		`nem.set("completion-style", "floating")`,
		`nem.set("completion-rows", 0)`,
		`nem.set("completion-rows", "ten")`,
		`nem.set("which-key-delay", -1)`,
		`nem.set("autosave-idle", 99999)`,
		`nem.set("backup", "yes")`,
		`nem.set("clipboard", "xclip")`,
		`nem.set("line-numbers", "yes")`,
		`nem.set("syntax", "on")`,
		`nem.set("theme", "solarized")`,
	} {
		t.Run(script, func(t *testing.T) {
			h, _, _ := newHost(t, script)
			if err := h.LoadConfig(); err == nil {
				t.Errorf("%s was accepted; a bad value must be reported, not ignored", script)
			}
		})
	}
}
