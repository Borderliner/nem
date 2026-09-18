package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/ui"
)

// Every name in the classification sets must be a real command. A name that no
// group registers is a silent no-op: the kill run would never break, or the goal
// column would never be preserved, and nothing else would complain. This is the
// same guard the default bindings get.
func TestClassifiedCommandsExist(t *testing.T) {
	reg, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	for _, set := range []struct {
		what  string
		names map[string]bool
	}{
		{"killCommands", killCommands},
		{"yankCommands", yankCommands},
		{"goalColumnCommands", goalColumnCommands},
	} {
		for name := range set.names {
			if _, ok := reg.Lookup(name); !ok {
				t.Errorf("%s names %q, which no command group registers", set.what, name)
			}
		}
	}
}

func TestNewStartsInScratch(t *testing.T) {
	e, _ := newTestEditor(t)
	if got := e.BufferName(e.Buf()); got != ui.ScratchName {
		t.Errorf("initial buffer name = %q, want %q", got, ui.ScratchName)
	}
	if got := len(e.Buffers()); got != 1 {
		t.Errorf("started with %d buffers, want 1", got)
	}
}

// --- saving ---------------------------------------------------------------

func TestSaveBufferWritesToOwnPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	e, _ := newTestEditor(t, "content")
	e.Buf().SetPath(path)

	if err := e.SaveBuffer(e.Buf(), ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "content") {
		t.Errorf("file = %q, want it to contain %q", got, "content")
	}
	if e.Buf().Modified() {
		t.Error("buffer still modified after a successful save")
	}
}

// A non-empty path saves there and adopts it, which is write-file.
func TestSaveBufferAdoptsANewPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adopted.txt")
	e, _ := newTestEditor(t, "content")

	if err := e.SaveBuffer(e.Buf(), path); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if got := e.Buf().Path(); got != path {
		t.Errorf("path = %q, want %q", got, path)
	}
	if got := e.BufferName(e.Buf()); got != "adopted.txt" {
		t.Errorf("buffer name = %q, want it renamed to the file's basename", got)
	}
}

// A failed save must leave the buffer exactly as it was: still modified, still
// pointing at its old path.
//
// text.Buffer.SaveAs adopts a new path only after the write succeeds, so
// SaveBuffer needs no rollback of its own. This test guards the behaviour at the
// editor level regardless of where the guarantee lives: if a failed write ever
// left the buffer claiming a file it was never written to, a later successful
// C-x C-s would silently save somewhere the user never asked for.
func TestFailedSaveLeavesBufferUntouched(t *testing.T) {
	e, _ := newTestEditor(t, "precious")
	e.Buf().SetPath("/original/path.txt")
	if err := e.Buf().Insert(text.Pos{Line: 0, Col: 0}, []rune("!")); err != nil {
		t.Fatal(err)
	}
	if !e.Buf().Modified() {
		t.Fatal("test setup: buffer should be modified")
	}

	bad := filepath.Join(t.TempDir(), "no-such-dir", "nested", "file.txt")
	if err := e.SaveBuffer(e.Buf(), bad); err == nil {
		t.Fatal("SaveBuffer to an unwritable path succeeded, want an error")
	}
	if !e.Buf().Modified() {
		t.Error("buffer was marked clean after a FAILED save — the user would lose work believing it saved")
	}
	if got := e.Buf().Path(); got != "/original/path.txt" {
		t.Errorf("path = %q after a failed save, want the original %q", got, "/original/path.txt")
	}
}

// --- buffers --------------------------------------------------------------

func TestOpenFileReusesAnOpenBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, _ := newTestEditor(t)
	a, err := e.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("OpenFile returned a second buffer for the same path")
	}
}

// Two files with the same basename stay distinguishable, as in emacs.
func TestDuplicateBasenamesAreDisambiguated(t *testing.T) {
	d1, d2 := t.TempDir(), t.TempDir()
	for _, d := range []string{d1, d2} {
		if err := os.WriteFile(filepath.Join(d, "dup.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, _ := newTestEditor(t)
	a, _ := e.OpenFile(filepath.Join(d1, "dup.txt"))
	b, _ := e.OpenFile(filepath.Join(d2, "dup.txt"))

	if e.BufferName(a) == e.BufferName(b) {
		t.Errorf("both buffers are named %q, want distinct names", e.BufferName(a))
	}
}

// NewBuffer reuses an existing name, so list-buffers does not pile up a new
// buffer per invocation.
func TestNewBufferReusesByName(t *testing.T) {
	e, _ := newTestEditor(t)
	a := e.NewBuffer("*Buffer List*")
	b := e.NewBuffer("*Buffer List*")
	if a != b {
		t.Error("NewBuffer created a second buffer under the same name")
	}
}

// Killing a buffer must move any window showing it, or the next redraw would
// dereference a dead buffer.
func TestKillBufferMovesWindowsOffIt(t *testing.T) {
	e, _ := newTestEditor(t, "doomed")
	victim := e.Buf()
	keep := e.NewBuffer("keeper")
	_ = keep

	if err := e.KillBuffer(victim); err != nil {
		t.Fatalf("KillBuffer: %v", err)
	}
	for _, w := range e.Tree().Windows() {
		if w.Buf == victim {
			t.Error("a window still shows the killed buffer")
		}
	}
}

func TestKillLastBufferIsRefused(t *testing.T) {
	e, _ := newTestEditor(t)
	if err := e.KillBuffer(e.Buf()); err == nil {
		t.Error("killing the last buffer succeeded, want a refusal")
	}
}

// --- quitting -------------------------------------------------------------

func TestQuitRefusesWithUnsavedChanges(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "x") // dirty the buffer
	if err := e.Quit(false); err == nil {
		t.Fatal("Quit without force succeeded with unsaved changes")
	}
	if e.Quitting() {
		t.Error("the session is quitting despite the refusal")
	}
	if err := e.Quit(true); err != nil {
		t.Fatalf("forced Quit: %v", err)
	}
	if !e.Quitting() {
		t.Error("forced Quit did not end the session")
	}
}

// --- hooks ----------------------------------------------------------------

// Hooks fire around dispatch, keyed on command name. This is the seam the Lua
// layer hangs before-save from, without command ever learning that scripting
// exists.
func TestHooksFireAroundDispatch(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	var order []string
	e.BeforeCommand("forward-char", func() { order = append(order, "before") })
	e.AfterCommand("forward-char", func() { order = append(order, "after") })
	e.BeforeCommand("kill-line", func() { order = append(order, "other") })

	press(t, e, "C-f")

	if got := strings.Join(order, ","); got != "before,after" {
		t.Errorf("hook order = %q, want %q", got, "before,after")
	}
}

func TestHooksRunInRegistrationOrder(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	var order []string
	e.BeforeCommand("forward-char", func() { order = append(order, "1") })
	e.BeforeCommand("forward-char", func() { order = append(order, "2") })
	press(t, e, "C-f")
	if got := strings.Join(order, ","); got != "1,2" {
		t.Errorf("hook order = %q, want %q", got, "1,2")
	}
}

// --- resize ---------------------------------------------------------------

// A resize must not panic, including down to a degenerate size where the layout
// has no room for a window at all.
func TestResizeDoesNotPanic(t *testing.T) {
	e, scr := newTestEditor(t, "some text here")
	press(t, e, "C-x", "2")
	for _, sz := range [][2]int{{80, 24}, {20, 5}, {1, 1}, {200, 60}} {
		scr.SetSize(sz[0], sz[1])
		e.HandleEvent(tcell.NewEventResize(sz[0], sz[1]))
		e.Redraw()
	}
}

// Redraw must survive a frame with no room, which is what a one-row terminal
// gives the split tree.
func TestRedrawAtDegenerateSize(t *testing.T) {
	e, scr := newTestEditor(t, "x")
	scr.SetSize(1, 1)
	e.Redraw()
}
