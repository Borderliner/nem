package editor

import (
	"path/filepath"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"

	"github.com/Borderliner/nem/syntax"
	"os"
)

// fgOnScreen is the foreground colour of one rendered cell. Syntax styles set
// only a foreground, so this is what the whole feature comes down to.
func fgOnScreen(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.Color {
	t.Helper()
	cells, w, h := scr.GetContents()
	if x < 0 || y < 0 || x >= w || y >= h {
		t.Fatalf("cell (%d,%d) out of bounds %dx%d", x, y, w, h)
	}
	fg, _, _ := cells[y*w+x].Style.Decompose()
	return fg
}

// anyColourOnRow reports whether a row holds any cell with a non-default
// foreground, which is the coarse "is this highlighted at all" question.
func anyColourOnRow(t *testing.T, scr tcell.SimulationScreen, y int) bool {
	t.Helper()
	cells, w, _ := scr.GetContents()
	for x := 0; x < w; x++ {
		if fg, _, _ := cells[y*w+x].Style.Decompose(); fg != tcell.ColorDefault {
			return true
		}
	}
	return false
}

// A Go file must actually arrive on screen coloured. Every piece of this works
// in isolation - the lexer, the cache, the renderer - so only an end-to-end
// assertion can catch them being wired together wrongly.
func TestGoBufferIsHighlightedOnScreen(t *testing.T) {
	e, scr := newTestEditor(t)
	b := e.NewBuffer("main.go")
	b.SetPath(filepath.Join(t.TempDir(), "main.go"))
	if err := b.Insert(text.Pos{}, []rune("package main")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	e.Active().Visit(b)
	e.Active().Pt = text.Pos{}
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Fatal("a Go buffer rendered with no colour at all; the wiring is dead")
	}
	// "package" is a keyword and must not be the terminal default.
	if got := fgOnScreen(t, scr, gutterOffset(t, e), 0); got == tcell.ColorDefault {
		t.Error("the keyword is uncoloured")
	}
}

// gutterOffset is the first text column, so a test can address the start of a
// line without hard-coding the gutter's width.
func gutterOffset(t *testing.T, e *Editor) int {
	t.Helper()
	if !e.th.LineNumbers {
		return 0
	}
	// Buffers in these tests are short, so the gutter is one digit plus its
	// separating column.
	return 2
}

// A buffer with no file has no language, so it must render plain. *scratch* and
// *Buffer List* are the everyday cases.
func TestScratchBufferIsNotHighlighted(t *testing.T) {
	e, scr := newTestEditor(t, "package main")
	e.Redraw()
	if anyColourOnRow(t, scr, 0) {
		t.Error("*scratch* is coloured; a buffer with no file has no language to lex")
	}
}

// Saving a path-less buffer as main.go must switch it to the Go lexer. Without
// this the file stays plain until the editor restarts, which reads as
// highlighting being broken rather than as a stale lexer.
func TestWriteFileRetunesTheLexer(t *testing.T) {
	e, scr := newTestEditor(t, "package main")
	b := e.Buf()

	e.Redraw()
	if anyColourOnRow(t, scr, 0) {
		t.Fatal("setup: the buffer is coloured before it has a path")
	}

	if err := e.SaveBuffer(b, filepath.Join(t.TempDir(), "main.go")); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Error("after saving as main.go the buffer is still unhighlighted")
	}
}

// A killed buffer's cache holds one lexer state per line it had. Nothing else
// would ever remove it, so a long session would leak one per buffer opened.
func TestKillingABufferDropsItsHighlightCache(t *testing.T) {
	e, _ := newTestEditor(t, "alpha")
	b := e.NewBuffer("other.go")
	b.SetPath(filepath.Join(t.TempDir(), "other.go"))

	// Touch it so a cache exists.
	e.spansOf(b, 0)
	if _, ok := e.hl[b]; !ok {
		t.Fatal("setup: no cache was created")
	}

	if err := e.KillBuffer(b); err != nil {
		t.Fatalf("KillBuffer: %v", err)
	}
	if _, ok := e.hl[b]; ok {
		t.Error("the cache outlived the buffer it describes")
	}
}

// Editing must not leave stale colours behind. This is the whole point of the
// cache's invalidation, seen from the outside.
func TestEditingUpdatesTheColours(t *testing.T) {
	e, scr := newTestEditor(t)
	b := e.NewBuffer("edit.go")
	b.SetPath(filepath.Join(t.TempDir(), "edit.go"))
	if err := b.Insert(text.Pos{}, []rune("x")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	e.Active().Visit(b)
	e.Active().Pt = b.End()
	e.Redraw()

	if anyColourOnRow(t, scr, 0) {
		t.Fatal("setup: a bare identifier should not be coloured")
	}

	// Turn the line into a comment; every character should now be one colour.
	e.Active().Pt = text.Pos{}
	press(t, e, "/", "/")
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Error("typing // did not colour the line as a comment")
	}
}

// A language nem has no hand-written lexer for must still be coloured, from
// nano's definitions. Skipped where nano is not installed, which is normal.
func TestNanorcLanguagesAreColoured(t *testing.T) {
	if _, err := os.Stat("/usr/share/nano"); err != nil {
		t.Skip("nano is not installed here")
	}
	e, _ := newTestEditor(t, `def handler(self):`, `    return "hi"  # done`)
	e.Buf().SetPath(filepath.Join(t.TempDir(), "script.py"))
	e.retuneHighlight(e.Buf())

	if name := e.cacheFor(e.Buf()).Lexer().Name(); name != "python" {
		t.Fatalf("lexer = %q, want python", name)
	}

	var classes []syntax.Class
	for ln := 0; ln < e.Buf().NumLines(); ln++ {
		for _, s := range e.spansOf(e.Buf(), ln) {
			classes = append(classes, s.Class)
		}
	}
	if len(classes) == 0 {
		t.Fatal("no spans for a Python file; highlighting is not reaching nanorc")
	}
	var sawString, sawComment bool
	for _, c := range classes {
		switch c {
		case syntax.String:
			sawString = true
		case syntax.Comment:
			sawComment = true
		}
	}
	if !sawString {
		t.Error(`no String span for "hi"`)
	}
	if !sawComment {
		t.Error("no Comment span for the trailing #")
	}
}

// nem's own lexers must not be displaced by nano's, which ship definitions for
// the same four languages and are less precise.
func TestNativeLexersWinOverNanorc(t *testing.T) {
	for _, tc := range []struct{ file, want string }{
		{"a.go", "go"}, {"b.lua", "lua"}, {"c.json", "json"}, {"d.md", "markdown"},
	} {
		e, _ := newTestEditor(t, "x")
		e.Buf().SetPath(filepath.Join(t.TempDir(), tc.file))
		e.retuneHighlight(e.Buf())
		if got := e.cacheFor(e.Buf()).Lexer().Name(); got != tc.want {
			t.Errorf("%s: lexer = %q, want nem's own %q", tc.file, got, tc.want)
		}
	}
}
