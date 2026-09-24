package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
)

// editing opens a listing of dir, goes to entry and starts editing names.
func editing(t *testing.T, e *Editor, dir, entry string) (*text.Buffer, *diredState) {
	t.Helper()
	b, st := listed(t, e, dir)
	if entry != "" {
		goTo(t, e, entry)
	}
	press(t, e, "C-x", "C-q")
	if st.wd == nil {
		t.Fatalf("C-x C-q did not start editing names; echo %q", e.Message())
	}
	return b, st
}

// nameOnLine is the name as the buffer now shows it on entry's line.
func nameOnLine(t *testing.T, b *text.Buffer, st *diredState, entry string) string {
	t.Helper()
	line, ok := st.list.LineOf(entry)
	if !ok {
		t.Fatalf("%s is not listed", entry)
	}
	lo, hi, ok := st.nameSpan(b, line)
	if !ok {
		t.Fatalf("line %d has no editable name", line)
	}
	return string(b.Line(line).View()[lo:hi])
}

func contentsOf(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// C-x C-q makes the names text; editing one and C-c C-c renames the file, and
// a buffer visiting it follows.
func TestWdiredRenamesByEditingNames(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	visiting, err := e.OpenFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, st := editing(t, e, dir, "a.txt")
	if b.ReadOnly() || e.FileType(b) != "wdired" || !e.isEditingListing(b) {
		t.Errorf("editing: read-only %v, type %q, drawn as edited %v", b.ReadOnly(), e.FileType(b), e.isEditingListing(b))
	}

	press(t, e, "C-e")
	press(t, e, "C-a")
	for _, r := range "new-" {
		press(t, e, string(r))
	}
	if got := nameOnLine(t, b, st, "a.txt"); got != "new-a.txt" {
		t.Fatalf("name reads %q after typing", got)
	}
	press(t, e, "C-c", "C-c")

	if exists(filepath.Join(dir, "a.txt")) || contentsOf(t, filepath.Join(dir, "new-a.txt")) != "contents of a.txt\n" {
		t.Fatal("a.txt was not renamed to new-a.txt")
	}
	if !exists(filepath.Join(dir, "b.txt")) {
		t.Error("b.txt went too")
	}
	if got, want := visiting.Path(), filepath.Join(dir, "new-a.txt"); got != want {
		t.Errorf("the buffer visiting a.txt points at %q, want %q", got, want)
	}
	if st.wd != nil || !b.ReadOnly() || e.FileType(b) != "dired" {
		t.Error("the listing did not go back to being a listing")
	}
	if got := atEntry(t, e); got != "new-a.txt" {
		t.Errorf("point is on %q, want the renamed file", got)
	}
	wantEcho(t, e, "Renamed a.txt to new-a.txt")
}

// Everything but the names stays as it is: typing in the details, a line
// break, joining lines, and the header are all turned down, the text
// untouched.
func TestWdiredOnlyNamesAreEditable(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	b, st := editing(t, e, dir, "a.txt")
	before := b.String()
	line := e.active.Pt.Line

	for _, try := range []struct {
		name string
		at   text.Pos
		keys []string
	}{
		{"the details", text.Pos{Line: line, Col: 5}, []string{"z"}},
		{"a line break", text.Pos{Line: line, Col: 38}, []string{"RET"}},
		{"joining lines", text.Pos{Line: line}, []string{"C-e", "C-d"}},
		{"backing into the date", text.Pos{Line: line}, []string{"C-a", "DEL"}},
		{"the header", text.Pos{}, []string{"z"}},
	} {
		e.active.Pt = try.at
		press(t, e, try.keys...)
		if b.String() != before {
			t.Fatalf("%s changed the listing to %q", try.name, b.String())
		}
		wantEcho(t, e, "only file names can be edited")
	}
	if st.wd == nil {
		t.Error("a refused edit ended the editing")
	}
}

// Names can be swapped, which renaming one at a time could not do.
func TestWdiredSwapsNames(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	editing(t, e, dir, "a.txt")
	press(t, e, "C-a", "C-k", "b", ".", "t", "x", "t")
	goTo(t, e, "b.txt")
	press(t, e, "C-a", "C-k", "a", ".", "t", "x", "t")
	press(t, e, "C-c", "C-c")

	if got := contentsOf(t, filepath.Join(dir, "a.txt")); got != "contents of b.txt\n" {
		t.Errorf("a.txt holds %q after the swap", got)
	}
	if got := contentsOf(t, filepath.Join(dir, "b.txt")); got != "contents of a.txt\n" {
		t.Errorf("b.txt holds %q after the swap", got)
	}
	wantEcho(t, e, "Renamed 2 files")
}

// C-c C-k throws the edits away, and C-/ undoes them one at a time as in any
// buffer.
func TestWdiredAbortAndUndo(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt")
	e, _ := newTestEditor(t)
	b, st := editing(t, e, dir, "a.txt")
	listing := b.String()

	press(t, e, "x", "C-/")
	if b.String() != listing {
		t.Errorf("undo left %q", b.String())
	}
	press(t, e, "C-c", "C-c")
	wantEcho(t, e, "No changes")

	editing(t, e, dir, "a.txt")
	press(t, e, "x", "y")
	press(t, e, "C-c", "C-k")
	if st.wd != nil || b.String() != listing || !b.ReadOnly() {
		t.Errorf("after C-c C-k: editing %v, read-only %v, text %q", st.wd != nil, b.ReadOnly(), b.String())
	}
	if !exists(filepath.Join(dir, "a.txt")) {
		t.Error("aborting renamed the file")
	}
	wantEcho(t, e, "aborted")
}

// A name emptied flags its file for deletion rather than deleting it.
func TestWdiredEmptyNameFlagsForDeletion(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	_, st := editing(t, e, dir, "a.txt")
	press(t, e, "C-a", "C-k", "C-c", "C-c")

	if !exists(filepath.Join(dir, "a.txt")) {
		t.Fatal("emptying the name deleted the file")
	}
	if st.marks["a.txt"] != 'D' {
		t.Errorf("a.txt's mark is %q, want the deletion flag", st.marks["a.txt"])
	}
	wantEcho(t, e, "Flagged 1 file for deletion")
}

// A rename that would overwrite is refused, nothing is renamed, and the
// editing goes on so the name can be put right.
func TestWdiredRefusalKeepsEditing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt", "c.txt")
	e, _ := newTestEditor(t)
	b, st := editing(t, e, dir, "a.txt")
	press(t, e, "C-a", "C-k", "b", ".", "t", "x", "t")
	goTo(t, e, "c.txt")
	press(t, e, "C-a", "d")
	press(t, e, "C-c", "C-c")

	wantEcho(t, e, "b.txt already exists")
	if st.wd == nil || b.ReadOnly() {
		t.Fatal("a refused rename ended the editing")
	}
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if got := contentsOf(t, filepath.Join(dir, n)); got != "contents of "+n+"\n" {
			t.Errorf("%s holds %q; nothing should have moved", n, got)
		}
	}

	// Put right, it goes through.
	goTo(t, e, "a.txt")
	press(t, e, "C-e", "2", "C-c", "C-c")
	if !exists(filepath.Join(dir, "b.txt2")) || !exists(filepath.Join(dir, "dc.txt")) {
		t.Errorf("after the fix: %v", readDir(t, dir))
	}
}

// A selection reaching past a name is not replaced, nor wrapped in brackets,
// and nothing is left half done: not the bracket the other end would take.
func TestWdiredSelectionPastTheName(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	b, _ := editing(t, e, dir, "a.txt")
	before := b.String()
	line := e.active.Pt.Line

	for _, sel := range []struct{ mark, pt text.Pos }{
		{text.Pos{Line: line, Col: 5}, text.Pos{Line: line, Col: 37}},     // from the details
		{text.Pos{Line: line, Col: 36}, text.Pos{Line: line + 1, Col: 0}}, // on to the next line
	} {
		for _, typed := range []string{"x", "("} {
			b.SetMark(sel.mark)
			b.ActivateMark()
			e.active.Pt = sel.pt
			press(t, e, typed)
			if b.String() != before {
				t.Fatalf("typing %s over %v-%v changed the listing to %q", typed, sel.mark, sel.pt, b.String())
			}
		}
	}

	// Inside the name, wrapping works as anywhere.
	b.SetMark(text.Pos{Line: line, Col: 35})
	b.ActivateMark()
	e.active.Pt = text.Pos{Line: line, Col: 36}
	press(t, e, "(")
	if got := string(b.Line(line).View()[35:]); got != "(a).txt" {
		t.Errorf("wrapping gave %q", got)
	}
}

// A slash in a name moves the file into that directory.
func TestWdiredMovesIntoADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", filepath.Join("sub", "x"))
	e, _ := newTestEditor(t)
	editing(t, e, dir, "a.txt")
	press(t, e, "C-a", "s", "u", "b", "/", "C-c", "C-c")

	if exists(filepath.Join(dir, "a.txt")) || !exists(filepath.Join(dir, "sub", "a.txt")) {
		t.Errorf("a.txt was not moved into sub: %v", readDir(t, dir))
	}
}

// C-x C-q with edits asks whether to apply them.
func TestWdiredExitAsks(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt")
	e, scr := newTestEditor(t)
	_, st := editing(t, e, dir, "a.txt")
	press(t, e, "C-e", "2")
	feed(t, scr, txt("y"))
	press(t, e, "C-x", "C-q")

	if st.wd != nil || !exists(filepath.Join(dir, "a.txt2")) {
		t.Errorf("answering y did not apply the edit: %v", readDir(t, dir))
	}
}

// The listing's own commands wait until the editing is done: one that read
// the directory again would throw the edited names away.
func TestWdiredHoldsOffDiredCommands(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt")
	e, _ := newTestEditor(t)
	b, _ := editing(t, e, dir, "a.txt")
	press(t, e, "C-e", "2")
	edited := b.String()

	e.dispatchReporting("dired-revert")
	wantEcho(t, e, "being edited")
	if again, err := e.Dired(dir); err != nil || again != b || b.String() != edited {
		t.Errorf("listing the directory again lost the edits: %q", b.String())
	}
}

// A keyboard macro runs down the listing as down any text: here, putting a
// prefix on every name.
func TestWdiredWithAKeyboardMacro(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "1.txt", "2.txt", "3.txt")
	e, _ := newTestEditor(t)
	editing(t, e, dir, "1.txt")
	hit(t, e, "<f3>", "C-a")
	hitText(e, "x-")
	hit(t, e, "C-n", "<f4>")
	hit(t, e, "C-u", "0", "<f4>")
	hit(t, e, "C-c", "C-c")

	for _, n := range []string{"x-1.txt", "x-2.txt", "x-3.txt"} {
		if !exists(filepath.Join(dir, n)) {
			t.Errorf("%s is missing: %v", n, readDir(t, dir))
		}
	}
}

// A symlink's target is shown after its name and cannot be edited; C-e stops
// at the end of the name, before it, and the colours after the name move
// along with its end.
func TestWdiredSymlink(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt")
	if err := os.Symlink("a.txt", filepath.Join(dir, "link")); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	e, _ := newTestEditor(t)
	b, st := editing(t, e, dir, "link")
	line := e.active.Pt.Line
	arrow := strings.Index(string(b.Line(line).View()), "→")
	arrowCol := len([]rune(string(b.Line(line).View())[:arrow]))

	press(t, e, "C-e")
	if got := int(e.active.Pt.Col); got != arrowCol-1 {
		t.Errorf("C-e went to column %d, want %d, the end of the name", got, arrowCol-1)
	}
	press(t, e, "C-f", "z")
	wantEcho(t, e, "only file names")

	before := st.spans(line)
	press(t, e, "C-b", "2", "2")
	after := st.wdiredSpans(b, line, st.spans(line))
	if last := len(after) - 1; after[last].Start != before[last].Start+2 {
		t.Errorf("the target's colour starts at %d, want %d", after[last].Start, before[last].Start+2)
	}

	press(t, e, "C-c", "C-c")
	if target, err := os.Readlink(filepath.Join(dir, "link22")); err != nil || target != "a.txt" {
		t.Errorf("link22 → %q, %v; want the link renamed with its target", target, err)
	}
}

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range des {
		names = append(names, d.Name())
	}
	return names
}
