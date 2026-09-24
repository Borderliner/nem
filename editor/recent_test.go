package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
)

// writeLines writes n numbered lines to a file in dir and returns its path.
func writeLines(t *testing.T, dir, name string, n int) string {
	t.Helper()
	var b strings.Builder
	for i := range n {
		b.WriteString("line ")
		b.WriteString(string(rune('a' + i%26)))
		b.WriteByte('\n')
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// C-x C-r offers the files opened lately, the one on screen last, and skips
// any that no longer exist.
func TestRecentFiles(t *testing.T) {
	dir := t.TempDir()
	a, b, c := writeLines(t, dir, "a.txt", 3), writeLines(t, dir, "b.txt", 3), writeLines(t, dir, "c.txt", 3)
	e, scr := newTestEditor(t)
	for _, p := range []string{a, b, c} {
		if err := e.visitFile(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}

	// Most recent first, current (c) last, deleted (b) gone: RET opens a.
	feed(t, scr, key(t, "RET"))
	press(t, e, "C-x", "C-r")
	if got := e.Buf().Path(); got != a {
		t.Errorf("C-x C-r RET opened %q, want %q", got, a)
	}
}

// A file opened again - in this session after C-x k, or in the next - shows
// where it was left.
func TestFilesReopenWherePointWas(t *testing.T) {
	dir, state := t.TempDir(), t.TempDir()
	p := writeLines(t, dir, "long.txt", 50)

	e, _ := newTestEditor(t)
	if err := e.UseStateDir(state); err != nil {
		t.Fatal(err)
	}
	if err := e.visitFile(p); err != nil {
		t.Fatal(err)
	}
	e.active.Pt = text.Pos{Line: 30, Col: 3}
	e.Redraw() // scrolls, so the view has a top worth remembering
	top := e.active.Top

	press(t, e, "C-x", "k")
	if err := e.visitFile(p); err != nil {
		t.Fatal(err)
	}
	wantPt(t, e, 30, 3)

	e.active.Pt = text.Pos{Line: 40, Col: 1}
	e.SaveMemory()

	e2, _ := newTestEditor(t)
	if err := e2.UseStateDir(state); err != nil {
		t.Fatal(err)
	}
	if err := e2.visitFile(p); err != nil {
		t.Fatal(err)
	}
	wantPt(t, e2, 40, 1)
	if top <= 0 {
		t.Fatalf("setup: the window never scrolled (top %d)", top)
	}
}

// A file that has shrunk since is opened at its end, not past it.
func TestRememberedPlacePastTheEndIsClamped(t *testing.T) {
	dir, state := t.TempDir(), t.TempDir()
	p := writeLines(t, dir, "shrinks.txt", 50)
	e, _ := newTestEditor(t)
	e.UseStateDir(state)
	e.visitFile(p)
	e.active.Pt = text.Pos{Line: 45, Col: 2}
	e.SaveMemory()

	writeLines(t, dir, "shrinks.txt", 5)
	e2, _ := newTestEditor(t)
	e2.UseStateDir(state)
	e2.visitFile(p)
	if got := e2.active.Pt; got.Line > 5 {
		t.Errorf("point at %v in a 5-line file", got)
	}
}
