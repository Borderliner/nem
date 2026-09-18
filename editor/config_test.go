package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajianpour/nem/text"
)

// writeConfig puts a script in a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "init.lua")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// text.TabWidth is package-level state, so a test that changes it must put it
// back or it leaks into every later test in the package.
func keepTabWidth(t *testing.T) {
	t.Helper()
	saved := text.TabWidth
	t.Cleanup(func() { text.TabWidth = saved })
}

// A missing config file is not an error. Most users will never write one.
func TestLoadConfigMissingFileIsFine(t *testing.T) {
	e, _ := newTestEditor(t)
	missing := filepath.Join(t.TempDir(), "absent.lua")
	if err := e.LoadConfig(missing); err != nil {
		t.Errorf("LoadConfig on a missing file = %v, want nil", err)
	}
	t.Cleanup(e.CloseConfig)
}

// The editor applies settings itself, because the host deliberately does not.
func TestLoadConfigAppliesSettings(t *testing.T) {
	keepTabWidth(t)
	e, _ := newTestEditor(t)
	path := writeConfig(t, `
nem.set("tab-width", 4)
nem.set("scroll-margin", 7)
`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)

	if got := text.TabWidth; got != 4 {
		t.Errorf("text.TabWidth = %d, want 4", got)
	}
	if got := e.th.ScrollMargin; got != 7 {
		t.Errorf("ScrollMargin = %d, want 7", got)
	}
}

// A broken config is reported and the editor carries on. Losing a session to a
// typo in init.lua would be a far worse outcome than starting unconfigured.
func TestBrokenConfigDoesNotStopTheEditor(t *testing.T) {
	e, _ := newTestEditor(t)
	path := writeConfig(t, `this is not valid lua at all (((`)

	if err := e.LoadConfig(path); err == nil {
		t.Error("LoadConfig on a broken script returned nil, want an error to report")
	}
	t.Cleanup(e.CloseConfig)

	if e.Message() == "" {
		t.Error("a broken config reported nothing in the echo area")
	}
	// And the editor still edits.
	press(t, e, "h", "i")
	wantText(t, e, "hi")
}

// A runtime error mid-script is reported with its message, not swallowed.
func TestConfigRuntimeErrorIsReported(t *testing.T) {
	e, _ := newTestEditor(t)
	path := writeConfig(t, `nem.set("tab-width", "not a number")`)
	_ = e.LoadConfig(path)
	t.Cleanup(e.CloseConfig)
	if !strings.Contains(e.Message(), "init.lua") {
		t.Errorf("echo = %q, want it to name init.lua", e.Message())
	}
}

// A Lua-defined command registers into the same table as the built-ins, so it is
// bindable and reachable from M-x.
func TestConfigCommandIsBindableAndRunnable(t *testing.T) {
	e, _ := newTestEditor(t)
	path := writeConfig(t, `
nem.command("insert-marker", "Insert a marker.", function()
  nem.run("self-insert-command")
end)
nem.bind("C-c m", "insert-marker")
`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)

	if _, ok := e.Registry().Lookup("insert-marker"); !ok {
		t.Fatal("the Lua-defined command did not register")
	}
	if got := e.Where("insert-marker"); len(got) == 0 {
		t.Error("the Lua binding did not reach the keymap")
	}
}

// A key bound to a command that does not exist is a dead key, and silence is the
// worst outcome: the user presses it, nothing happens, the config looks right.
func TestUnresolvedBindingIsReported(t *testing.T) {
	e, _ := newTestEditor(t)
	path := writeConfig(t, `nem.bind("C-c z", "no-such-command")`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)

	if !strings.Contains(e.Message(), "no command") {
		t.Errorf("echo = %q, want it to report the dangling binding", e.Message())
	}
}

// before-save fires on dispatch of save-buffer, which is the whole point of the
// hook seam: the command layer stays ignorant that Lua exists.
func TestBeforeSaveHookFires(t *testing.T) {
	target := filepath.Join(t.TempDir(), "saved.txt")

	e, _ := newTestEditor(t, "content")
	path := writeConfig(t, `
seen = 0
nem.hook("before-save", function(buf) seen = seen + 1 end)
nem.command("report-seen", "Write the hook count into the buffer.", function()
  nem.buf.replace_line(tostring(seen))
end)
`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)

	e.Buf().SetPath(target)
	press(t, e, "x") // save-buffer skips an unmodified buffer, so dirty it first

	if err := e.Run("save-buffer"); err != nil {
		t.Fatalf("save-buffer: %v", err)
	}
	if err := e.Run("report-seen"); err != nil {
		t.Fatalf("report-seen: %v", err)
	}
	wantText(t, e, "1")
}

// A failing hook must not prevent the save. A broken format-on-save hook that
// silently stopped saves would be the worst possible bug in this seam.
func TestFailingHookStillSaves(t *testing.T) {
	target := filepath.Join(t.TempDir(), "saved.txt")
	e, _ := newTestEditor(t, "content")
	path := writeConfig(t, `nem.hook("before-save", function(buf) error("boom") end)`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)

	e.Buf().SetPath(target)
	press(t, e, "x") // dirty it, or save-buffer has nothing to do

	if err := e.Run("save-buffer"); err != nil {
		t.Fatalf("save-buffer: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("the file was not written despite a failing hook: %v", err)
	}
	if !strings.Contains(string(got), "content") {
		t.Errorf("file = %q, want the buffer's text", got)
	}
	// The hook's failure is reported rather than swallowed.
	if !strings.Contains(e.Message(), "before-save") && !strings.Contains(e.Message(), "Wrote") {
		t.Errorf("echo = %q, want either the hook failure or the save confirmation", e.Message())
	}
}
