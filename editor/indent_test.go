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

// --- saving ---------------------------------------------------------------

// onDisk is the file b was saved to, as bytes.
func onDisk(t *testing.T, b *text.Buffer) string {
	t.Helper()
	got, err := os.ReadFile(b.Path())
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// An .editorconfig's trim_trailing_whitespace and insert_final_newline are
// done to the buffer as it is saved, so the screen shows what the file holds,
// and the whole of it undoes in one step.
func TestSavingTidiesAsTheEditorconfigAsks(t *testing.T) {
	e, _ := newTestEditor(t)
	b := configured(t, e, "notes.txt", "[*]\ntrim_trailing_whitespace = true\ninsert_final_newline = true\n", "a  \n\t\nb c \t")
	e.Active().Pt = text.Pos{Line: 2, Col: 5} // in the whitespace that goes
	press(t, e, "C-a", "x", "C-e")            // modified, point back at the end
	press(t, e, "C-x", "C-s")

	if got, want := onDisk(t, b), "a\n\nxb c\n"; got != want {
		t.Errorf("saved %q, want %q", got, want)
	}
	wantText(t, e, "a\n\nxb c")
	wantPt(t, e, 2, 4)
	if b.Modified() {
		t.Error("the buffer is modified after saving")
	}

	press(t, e, "C-/")
	wantText(t, e, "a  \n\t\nxb c \t")
}

// insert_final_newline = false leaves no newline at the end: neither the one
// a file is read with, nor the empty lines it would end with.
func TestSavingDropsTheFinalNewlineWhenAsked(t *testing.T) {
	e, _ := newTestEditor(t)
	b := configured(t, e, "a.txt", "[*]\ninsert_final_newline = false\n", "a\n\n\n")
	press(t, e, "M->", "x", "<backspace>")
	press(t, e, "C-x", "C-s")

	if got, want := onDisk(t, b), "a"; got != want {
		t.Errorf("saved %q, want %q", got, want)
	}
	wantText(t, e, "a")
	wantPt(t, e, 0, 1)
}

// Without the rules, a file is saved exactly as it is.
func TestSavingWithoutTheRulesChangesNothing(t *testing.T) {
	e, _ := newTestEditor(t)
	b := configured(t, e, "a.txt", "[*]\nindent_style = space\n", "a  \nb")
	press(t, e, "C-e", "x")
	press(t, e, "C-x", "C-s")
	if got, want := onDisk(t, b), "a  x\nb"; got != want {
		t.Errorf("saved %q, want %q", got, want)
	}
}

// The rules are the ones for where the file is going: write-file into a
// directory whose .editorconfig asks for them applies them.
func TestWritingElsewhereTidiesForTheNewPlace(t *testing.T) {
	e, _ := newTestEditor(t, "a  ", "b")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n[*.md]\ntrim_trailing_whitespace = true\ninsert_final_newline = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "out.md")
	if err := e.SaveBuffer(e.Buf(), path); err != nil {
		t.Fatal(err)
	}
	if got, want := onDisk(t, e.Buf()), "a\nb\n"; got != want {
		t.Errorf("saved %q, want %q", got, want)
	}
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
