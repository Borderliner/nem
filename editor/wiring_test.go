package editor

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/lua"
	"github.com/Borderliner/nem/ui"
	"github.com/gdamore/tcell/v2"
)

// The clipboard read path is dead unless HandleEvent routes the terminal's
// reply. Nothing else surfaces EventClipboard, so without this the reply is
// dropped and a yank silently falls back to the kill ring forever.
func TestHandleEventRoutesTheClipboardReply(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	e.HandleEvent(tcell.NewEventClipboard([]byte("from the terminal")))
	// The reply becomes the newest kill-ring entry, which is what makes it
	// reachable with C-y.
	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank after a clipboard reply: %v", err)
	}
	if got != "from the terminal" {
		t.Errorf("yanked %q, want the clipboard reply - HandleEvent is not routing it", got)
	}
}

// A resize must still be handled after the clipboard case was added above it.
func TestHandleEventStillHandlesResize(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	e.HandleEvent(tcell.NewEventResize(40, 12)) // must not panic
}

// Every test editor must back up into a temporary directory. New() defaults to
// the real $XDG_STATE_HOME/nem, so a test that saves over an existing file would
// otherwise write into the developer's home directory - and the failure would be
// invisible, because the test would still pass.
func TestTestEditorNeverBacksUpIntoTheRealHome(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	root := e.BackupRoot()
	if root == "" {
		t.Fatal("no backup root set on a test editor")
	}
	if strings.Contains(root, ".local/state/nem") {
		t.Errorf("test editor backs up to %q, which is the real state directory", root)
	}
	if !strings.HasPrefix(root, "/tmp") && !strings.Contains(root, "TestTestEditor") {
		t.Errorf("backup root %q does not look like a t.TempDir()", root)
	}
}

// Redraw and a prompt must build the same frame. They were separate
// constructions and had already begun to diverge.
func TestFrameIsBuiltInOnePlace(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	f := e.frame()
	if f.Tree != e.tree || f.Active != e.active {
		t.Error("frame() does not describe the editor's own tree and active window")
	}
	if f.MiniOn {
		t.Error("frame() reports a prompt when none is open")
	}
}

// recover-file is registered by the editor rather than by a command group,
// because it needs the autosave store. It must still be reachable by name, or
// an autosave the user was told about cannot be acted on.
func TestRecoverFileIsReachableByName(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	c, ok := e.reg.Lookup("recover-file")
	if !ok {
		t.Fatal("recover-file is not registered; an offered recovery cannot be acted on")
	}
	if !c.Interactive {
		t.Error("recover-file is not interactive, so M-x cannot find it")
	}
	if c.Doc == "" {
		t.Error("recover-file has no doc string")
	}
}

// Every config setting must reach something. A setting that validates and then
// goes nowhere is the worst kind: the user's config looks correct and does
// nothing.
func TestConfigSettingsReachTheEditor(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	e.applySettings(lua.Settings{
		TabWidth: 4, ScrollMargin: 5, UndoStyle: "linear",
		CompletionStyle: "popup", CompletionRows: 15,
		WhichKeyDelay: 0, AutosaveIdle: 0,
		Backup: false, Clipboard: "off", LineNumbers: false,
		Syntax: false, Theme: "light", DeleteSelection: false,
		OpenBinary: "text",
	})

	if e.th.ScrollMargin != 5 {
		t.Errorf("scroll-margin = %d, want 5", e.th.ScrollMargin)
	}
	if e.ext.mode != binaryText {
		t.Error("open-binary did not reach the editor")
	}
	if e.comp.style != completionPopup {
		t.Error("completion-style did not reach the editor")
	}
	if e.comp.rows != 15 {
		t.Errorf("completion-rows = %d, want 15", e.comp.rows)
	}
	if e.whichKeyDelay() != 0 {
		t.Errorf("which-key-delay = %v, want 0 (disabled)", e.whichKeyDelay())
	}
	if e.safe.idle != 0 {
		t.Errorf("autosave-idle = %v, want 0 (disabled)", e.safe.idle)
	}
	if e.safe.enabled {
		t.Error("backup=false did not reach the editor")
	}
	if e.ClipboardMode() != ClipboardOff {
		t.Error("clipboard=off did not reach the editor")
	}
	if e.th.LineNumbers {
		t.Error("line-numbers=false did not reach the editor")
	}
	if e.th.Syntax {
		t.Error("syntax=false did not reach the editor")
	}
	if e.DeleteSelection() {
		t.Error("delete-selection=false did not reach the editor")
	}
	// theme="light" must actually swap the palette, not just be accepted.
	dark := ui.DefaultTheme()
	dark.UseSyntaxPalette(false)
	if e.th.SyntaxStyle == dark.SyntaxStyle {
		t.Error(`theme="light" left the dark palette in place`)
	}
}

// toggle-line-numbers flips the gutter and says which way it went, so the
// keypress is never a silent no-op.
func TestToggleLineNumbers(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	if !e.th.LineNumbers {
		t.Fatal("line numbers should start on")
	}

	press(t, e, "C-x", "n")
	if e.th.LineNumbers {
		t.Error("C-x n did not turn the gutter off")
	}
	if !strings.Contains(e.echo, "off") {
		t.Errorf("echo = %q, want it to say the gutter went off", e.echo)
	}

	press(t, e, "C-x", "n")
	if !e.th.LineNumbers {
		t.Error("C-x n did not turn the gutter back on")
	}
	if !strings.Contains(e.echo, "on") {
		t.Errorf("echo = %q, want it to say the gutter came back on", e.echo)
	}
}

// The toggle must actually change what is drawn, not just a flag.
func TestToggleLineNumbersChangesTheFrame(t *testing.T) {
	e, scr := newTestEditor(t, "alpha", "beta")
	e.Redraw()
	withNumbers := rowOf(t, scr, 0)

	press(t, e, "C-x", "n")
	e.Redraw()
	without := rowOf(t, scr, 0)

	if withNumbers == without {
		t.Errorf("row unchanged by the toggle: %q", withNumbers)
	}
	if !strings.HasPrefix(strings.TrimLeft(without, " "), "alpha") {
		t.Errorf("with the gutter off the row is %q, want it to start with the text", without)
	}
}

// rowOf reads one rendered row from the simulation screen.
func rowOf(t *testing.T, scr tcell.SimulationScreen, y int) string {
	t.Helper()
	cells, w, _ := scr.GetContents()
	var b strings.Builder
	for x := 0; x < w; x++ {
		rs := cells[y*w+x].Runes
		if len(rs) == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteString(string(rs))
	}
	return strings.TrimRight(b.String(), " ")
}
