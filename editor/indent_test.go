package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// configured visits a file called name holding content, in a directory of its
// own whose .editorconfig is root = true followed by config - root, so what
// the machine running the test has above its temporary directory stays out
// of it.
func configured(t *testing.T, e *Editor, name, config, content string) *text.Buffer {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n"+config), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, path)
	e.Active().Pt = text.Pos{}
	return b
}

// TAB shifts the lines of a selection and keeps it selected, so it can be
// pressed again; S-TAB shifts them back; each press is one undo.
func TestTabAndBacktabShiftASelection(t *testing.T) {
	e, _ := newTestEditor(t)
	configured(t, e, "app.py", "", "if x:\n    y()\nz()\n")

	press(t, e, "C-SPC", "C-n", "C-n") // lines 0 and 1, ending at the start of 2
	press(t, e, "TAB")
	wantText(t, e, "    if x:\n        y()\nz()")
	if !e.Buf().MarkActive() {
		t.Fatal("the selection ended after TAB")
	}
	press(t, e, "TAB")
	wantText(t, e, "        if x:\n            y()\nz()")

	press(t, e, "<backtab>")
	wantText(t, e, "    if x:\n        y()\nz()")
	if !e.Buf().MarkActive() {
		t.Fatal("the selection ended after S-TAB")
	}
	wantPt(t, e, 2, 0)

	press(t, e, "C-/")
	wantText(t, e, "        if x:\n            y()\nz()")
}

// Shift-TAB arrives from the terminal as a key of its own, which must reach
// the <backtab> binding.
func TestTheTerminalsBacktabReachesItsBinding(t *testing.T) {
	e, _ := newTestEditor(t)
	configured(t, e, "main.go", "", "func f() {\n\t\tx()\n}\n")
	e.Active().Pt = text.Pos{Line: 1, Col: 2}

	e.HandleKey(DecodeKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModShift), false))
	wantText(t, e, "func f() {\n\tx()\n}")
	wantPt(t, e, 1, 1)
}

// TAB with no selection indents at point the way the file does.
func TestTabIndentsAsTheFileDoes(t *testing.T) {
	e, _ := newTestEditor(t)
	configured(t, e, "a.yaml", "", "key:\n")
	press(t, e, "C-e", "RET", "TAB")
	wantText(t, e, "key:\n  ")

	// And a file whose .editorconfig says otherwise.
	e, _ = newTestEditor(t)
	configured(t, e, "a.yaml", "[*.yaml]\nindent_size = 4\n", "key:\n")
	press(t, e, "C-e", "RET", "TAB")
	wantText(t, e, "key:\n    ")
}

// C-x TAB shifts by the prefix argument, in columns.
func TestIndentRigidlyByColumns(t *testing.T) {
	e, _ := newTestEditor(t)
	configured(t, e, "app.py", "", "a\nb\n")
	press(t, e, "C-x", "h", "C-u", "3", "C-x", "TAB")
	wantText(t, e, "   a\n   b")
	press(t, e, "C-u", "-", "1", "C-x", "TAB")
	wantText(t, e, "  a\n  b")
}
