package editor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Borderliner/nem/text"
)

// M-s l finds a line by a few letters of it, in the buffer's order; RET
// leaves point on what matched, and the mark where it came from. C-g puts
// point back where it was, however far the preview took it.
func TestFzfLines(t *testing.T) {
	e, scr := newTestEditor(t, "alpha", "beta", "gamma ray", "delta")
	e.active.Pt = text.Pos{Line: 3, Col: 2}
	feed(t, scr, txt("ray"), key(t, "RET"))
	press(t, e, "M-s", "l")
	wantPt(t, e, 2, 6)
	if m := e.Buf().Mark(); m != (text.Pos{Line: 3, Col: 2}) {
		t.Errorf("mark at %v, want where the search began", m)
	}

	feed(t, scr, txt("alp"), key(t, "C-g"))
	press(t, e, "M-s", "l")
	wantPt(t, e, 2, 6)
}

// M-s r searches the project as the pattern is typed, ripgrep's options and
// all, and RET goes to the line highlighted; M-RET lists every match in a
// results buffer instead.
func TestRgLive(t *testing.T) {
	root := aProject(t, map[string]string{
		"a.go":     "package a\n\nvar needle = 1\n",
		"b.md":     "a needle in a doc\n",
		"sub/c.go": "// nothing\n",
	})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "sub", "c.go"))

	feedAfter(t, scr, 300*time.Millisecond, txt("-t go needle"), key(t, "RET"))
	press(t, e, "M-s", "r")
	wantVisiting(t, e, filepath.Join(root, "a.go"))
	wantPt(t, e, 2, 4)

	feedAfter(t, scr, 300*time.Millisecond, txt("needle"), key(t, "M-RET"))
	press(t, e, "M-s", "r")
	st := e.grepOf(e.active.Buf)
	if st == nil || len(st.matches) != 2 {
		t.Fatalf("M-RET listed %+v; want both needles in a results buffer", st)
	}
}

// M-s f finds a file anywhere under the project by a few letters of its
// path.
func TestFzfFiles(t *testing.T) {
	root := aProject(t, map[string]string{
		"README.md":                "x\n",
		"deep/down/below/note.txt": "x\n",
	})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "README.md"))
	feed(t, scr, txt("dbnote"), key(t, "RET"))
	press(t, e, "M-s", "f")
	wantVisiting(t, e, filepath.Join(root, "deep", "down", "below", "note.txt"))
}
