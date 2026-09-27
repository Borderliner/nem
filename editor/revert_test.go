package editor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// rewrite changes a file behind the editor's back, making sure its time
// moves on even on a filesystem that keeps it coarsely.
func rewrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

// A buffer without edits follows its file when something else changes it -
// here on coming back to the terminal; one with edits is left for its owner
// to decide about.
func TestRevertFollowsTheDisk(t *testing.T) {
	dir := t.TempDir()
	clean, edited := filepath.Join(dir, "clean.txt"), filepath.Join(dir, "edited.txt")
	for _, p := range []string{clean, edited} {
		if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, scr := newTestEditor(t)
	cb := visiting(t, e, clean)
	eb := visiting(t, e, edited)
	if err := eb.Insert(text.Pos{}, []rune("mine ")); err != nil {
		t.Fatal(err)
	}

	rewrite(t, clean, "new text\n")
	rewrite(t, edited, "theirs\n")
	e.HandleEvent(tcell.NewEventFocus(true))
	if cb.String() != "new text" || cb.Modified() {
		t.Errorf("the buffer without edits reads %q, modified %v", cb.String(), cb.Modified())
	}
	if eb.String() != "mine old" {
		t.Errorf("the buffer with edits was changed to %q", eb.String())
	}
	wantEcho(t, e, "Reverted clean.txt, changed on disk")

	// C-x x g reads it again on request, asking first since it has edits.
	feed(t, scr, txt("y"))
	press(t, e, "C-x", "x", "g")
	if eb.String() != "theirs" || eb.Modified() {
		t.Errorf("after C-x x g: %q, modified %v", eb.String(), eb.Modified())
	}
}

// With auto-revert off, only C-x x g reads a file again.
func TestAutoRevertCanBeTurnedOff(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, _ := newTestEditor(t)
	e.autoRevert = false
	b := visiting(t, e, p)
	rewrite(t, p, "new\n")
	e.revertChanged()
	if b.String() != "old" {
		t.Errorf("reverted to %q with auto-revert off", b.String())
	}
}
