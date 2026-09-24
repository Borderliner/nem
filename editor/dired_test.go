package editor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/dired"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/lua"
	"github.com/Borderliner/nem/text"
)

// Dired driven through HandleKey, as a user drives it. Prompts are answered by
// feeding their keys before the key that opens them; see press.

// listed opens a dired buffer on dir in the active window.
func listed(t *testing.T, e *Editor, dir string) (*text.Buffer, *diredState) {
	t.Helper()
	b, err := e.Dired(dir)
	if err != nil {
		t.Fatalf("Dired(%s): %v", dir, err)
	}
	e.active.Visit(b)
	return b, e.dired[b]
}

// atEntry reports the entry point is on, failing when it is on none.
func atEntry(t *testing.T, e *Editor) string {
	t.Helper()
	st := e.diredOf(e.active.Buf)
	if st == nil {
		t.Fatalf("buffer %q is not a listing", e.BufferName(e.active.Buf))
	}
	en, ok := st.list.EntryAt(e.active.Pt.Line)
	if !ok {
		t.Fatalf("point %v is on no entry", e.active.Pt)
	}
	return en.Name
}

// goTo puts point on the entry called name.
func goTo(t *testing.T, e *Editor, name string) {
	t.Helper()
	st := e.diredOf(e.active.Buf)
	line, ok := st.list.LineOf(name)
	if !ok {
		t.Fatalf("%s is not listed", name)
	}
	e.active.Pt = st.entryPos(line)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// A directory handed to OpenFile - by find-file or on the command line - is
// listed rather than refused, read-only, with point on the first file's name.
func TestOpenFileOnADirectoryListsIt(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "b.txt", "a.txt", filepath.Join("sub", "x"))
	e, _ := newTestEditor(t)

	b, err := e.OpenFile(dir)
	if err != nil {
		t.Fatalf("OpenFile(dir): %v", err)
	}
	e.active.Visit(b)

	if !b.ReadOnly() {
		t.Error("the listing is editable")
	}
	if got, want := e.BufferName(b), filepath.Base(dir)+string(filepath.Separator); got != want {
		t.Errorf("buffer is named %q, want %q", got, want)
	}
	// Directories first.
	if got := atEntry(t, e); got != "sub" {
		t.Errorf("point is on %q, want the first entry, sub", got)
	}
	if got := int(e.active.Pt.Col); got != dired.NameColumn(e.dired[b].opts) {
		t.Errorf("point is in column %d, want the name column", got)
	}
	if got := e.FileType(b); got != "dired" {
		t.Errorf("modeline type %q, want dired", got)
	}
	// Listed again, the same buffer comes back rather than a second copy.
	if again, _ := e.OpenFile(dir); again != b {
		t.Error("listing the same directory twice made two buffers")
	}
}

// Keys that would type into a listing are refused, and the listing is left
// describing the directory.
func TestDiredRefusesTyping(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt")
	e, _ := newTestEditor(t)
	b, _ := listed(t, e, dir)
	before := b.String()

	press(t, e, "z")

	if b.String() != before {
		t.Errorf("typing changed the listing to %q", b.String())
	}
	wantEcho(t, e, "read-only")
}

// n and p move a file at a time and stop at the ends rather than wandering onto
// the header. The top is the parent entry.
func TestDiredMovesByFile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b", "c")
	e, _ := newTestEditor(t)
	listed(t, e, dir)

	press(t, e, "n", "n", "n", "n")
	if got := atEntry(t, e); got != "c" {
		t.Errorf("after four n, on %q, want the last file, c", got)
	}
	press(t, e, "p", "C-p", "<up>", "p")
	if got := atEntry(t, e); got != ".." {
		t.Errorf("after four p, on %q, want the top entry, ..", got)
	}
	press(t, e, "M-<")
	if got := atEntry(t, e); got != "a" {
		t.Errorf("after M-<, on %q, want the first file, a", got)
	}
	press(t, e, "M->")
	if got := atEntry(t, e); got != "c" {
		t.Errorf("after M->, on %q, want c", got)
	}
}

// RET on a file visits it; RET on a directory lists it in the same buffer, and
// ^ comes back out with point on the directory just left.
func TestDiredWalksTheTree(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, filepath.Join("sub", "inner.txt"), "top.txt")
	e, _ := newTestEditor(t)
	b, st := listed(t, e, dir)
	count := len(e.Buffers())

	goTo(t, e, "sub")
	press(t, e, "RET")
	if e.active.Buf != b || st.dir != filepath.Join(dir, "sub") {
		t.Fatalf("RET on sub: showing %q, listing %s", e.BufferName(e.active.Buf), st.dir)
	}
	if len(e.Buffers()) != count {
		t.Errorf("walking into sub made a new buffer; the listing should follow")
	}
	if got := e.BufferName(b); got != "sub"+string(filepath.Separator) {
		t.Errorf("listing is named %q after walking into sub", got)
	}

	press(t, e, "^")
	if st.dir != dir {
		t.Fatalf("^ lists %s, want %s", st.dir, dir)
	}
	if got := atEntry(t, e); got != "sub" {
		t.Errorf("after ^, point is on %q, want sub", got)
	}

	goTo(t, e, "top.txt")
	press(t, e, "RET")
	if got, want := e.Buf().Path(), filepath.Join(dir, "top.txt"); got != want {
		t.Errorf("RET on top.txt visits %q", got)
	}
}

// m marks and moves on, u unmarks, t inverts and U clears. A mark is drawn in
// the mark column, from the same Listing the colours come from.
func TestDiredMarks(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b", "c")
	e, _ := newTestEditor(t)
	b, st := listed(t, e, dir)
	markOn := func(name string) rune {
		line, _ := st.list.LineOf(name)
		return b.Line(line).Runes()[dired.MarkColumn]
	}

	press(t, e, "m", "m")
	if markOn("a") != '*' || markOn("b") != '*' || markOn("c") != ' ' {
		t.Errorf("after m m, marks are %q %q %q, want * * blank", markOn("a"), markOn("b"), markOn("c"))
	}
	if got := atEntry(t, e); got != "c" {
		t.Errorf("m should move down; on %q", got)
	}

	press(t, e, "DEL")
	if markOn("b") != ' ' {
		t.Error("DEL did not unmark the file above")
	}

	press(t, e, "t")
	if markOn("a") != ' ' || markOn("b") != '*' || markOn("c") != '*' {
		t.Errorf("t gave %q %q %q, want the marks inverted", markOn("a"), markOn("b"), markOn("c"))
	}

	press(t, e, "U")
	if len(st.marks) != 0 || markOn("b") != ' ' {
		t.Error("U left marks behind")
	}
}

// d flags, x deletes what is flagged once "yes" is typed, and anything else
// deletes nothing. Point stays on the row it was on.
func TestDiredDeletesFlaggedFilesAfterAsking(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b", "c", filepath.Join("d", "deep"))
	e, scr := newTestEditor(t)
	listed(t, e, dir)
	goTo(t, e, "d")
	press(t, e, "d", "d") // flags d, then a

	feed(t, scr, txt("no"), key(t, "RET"))
	press(t, e, "x")
	if !exists(filepath.Join(dir, "a")) || !exists(filepath.Join(dir, "d")) {
		t.Fatal("answering no deleted something")
	}
	wantEcho(t, e, "Nothing deleted")

	feed(t, scr, txt("yes"), key(t, "RET"))
	press(t, e, "x")
	if exists(filepath.Join(dir, "a")) || exists(filepath.Join(dir, "d")) {
		t.Error("x with yes left a flagged file behind")
	}
	if !exists(filepath.Join(dir, "b")) || !exists(filepath.Join(dir, "c")) {
		t.Error("x deleted a file that was not flagged")
	}
	wantEcho(t, e, "Deleted 2 files")
}

// D acts on the marked files, or on the file at point when none are marked.
func TestDiredDeleteAtPoint(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b")
	e, scr := newTestEditor(t)
	listed(t, e, dir)
	goTo(t, e, "b")

	feed(t, scr, txt("yes"), key(t, "RET"))
	press(t, e, "D")

	if exists(filepath.Join(dir, "b")) || !exists(filepath.Join(dir, "a")) {
		t.Error("D should delete exactly the file at point")
	}
	if got := atEntry(t, e); got != "a" {
		t.Errorf("after deleting the last file, point is on %q, want a", got)
	}
}

// R renames in place, and a buffer visiting the file follows it: left on the
// old path, its next save would quietly recreate the file there.
func TestDiredRenameCarriesOpenBuffers(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "old.txt")
	t.Chdir(dir)
	e, scr := newTestEditor(t)
	visiting, err := e.OpenFile(filepath.Join(dir, "old.txt"))
	if err != nil {
		t.Fatal(err)
	}
	listed(t, e, dir)
	goTo(t, e, "old.txt")

	// The prompt starts empty here: the listing is the working directory.
	feed(t, scr, txt("new.txt"), key(t, "M-RET"))
	press(t, e, "R")

	if exists(filepath.Join(dir, "old.txt")) || !exists(filepath.Join(dir, "new.txt")) {
		t.Fatal("R did not rename old.txt to new.txt")
	}
	if got, want := visiting.Path(), filepath.Join(dir, "new.txt"); got != want {
		t.Errorf("the open buffer still points at %q, want %q", got, want)
	}
	if got := atEntry(t, e); got != "new.txt" {
		t.Errorf("point is on %q after the rename, want new.txt", got)
	}
}

// C copies the marked files into a directory, and replaces nothing without
// asking: a file already there is kept when the answer is n.
func TestDiredCopiesMarkedFilesIntoADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b", filepath.Join("dest", "b"))
	e, scr := newTestEditor(t)
	listed(t, e, dir)
	goTo(t, e, "a")
	press(t, e, "m", "m")

	dest := filepath.Join(dir, "dest") + string(filepath.Separator)
	feed(t, scr, key(t, "C-a", "C-k"), txt(dest), key(t, "RET"), txt("n"))
	press(t, e, "C")

	got, err := os.ReadFile(filepath.Join(dir, "dest", "a"))
	if err != nil || string(got) != "contents of a\n" {
		t.Errorf("dest/a = %q, %v; want a copy of a", got, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "dest", "b")); string(got) != "contents of "+filepath.Join("dest", "b")+"\n" {
		t.Errorf("dest/b was replaced after answering n: %q", got)
	}
	if !exists(filepath.Join(dir, "a")) || !exists(filepath.Join(dir, "b")) {
		t.Error("copying removed a source")
	}
}

// + creates a directory, parents included, and lands on it.
func TestDiredCreatesADirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a")
	t.Chdir(dir)
	e, scr := newTestEditor(t)
	listed(t, e, dir)

	feed(t, scr, txt(filepath.Join("made", "deeper")), key(t, "RET"))
	press(t, e, "+")

	if fi, err := os.Stat(filepath.Join(dir, "made", "deeper")); err != nil || !fi.IsDir() {
		t.Fatalf("made/deeper was not created: %v", err)
	}
	if got := atEntry(t, e); got != "made" {
		t.Errorf("point is on %q, want the new directory", got)
	}
}

// g reads the directory again, keeping point on the file it was on.
func TestDiredRevertSeesChangesOnDisk(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "b", "c")
	e, _ := newTestEditor(t)
	listed(t, e, dir)
	goTo(t, e, "c")

	writeFiles(t, dir, "a")
	press(t, e, "g")

	if got := atEntry(t, e); got != "c" {
		t.Errorf("after g, point is on %q, want c still", got)
	}
	goTo(t, e, "a") // fails the test if a is not listed
}

// . shows dotfiles and ( hides the details. Point stays on its file through
// both, though the name moves column when the details go.
func TestDiredToggles(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, ".hidden", "shown")
	e, _ := newTestEditor(t)
	_, st := listed(t, e, dir)
	goTo(t, e, "shown")

	if _, ok := st.list.LineOf(".hidden"); ok {
		t.Error("a dotfile is listed by default")
	}
	press(t, e, ".")
	if _, ok := st.list.LineOf(".hidden"); !ok {
		t.Error(". did not show the dotfile")
	}
	if got := atEntry(t, e); got != "shown" {
		t.Errorf("after ., point is on %q, want shown", got)
	}

	press(t, e, "(")
	if got, want := int(e.active.Pt.Col), dired.NameColumn(dired.Options{HideDetails: true}); got != want {
		t.Errorf("after (, point is in column %d, want the new name column %d", got, want)
	}
	if got := atEntry(t, e); got != "shown" {
		t.Errorf("after (, point is on %q, want shown", got)
	}

	press(t, e, "s")
	wantEcho(t, e, "Sorted by time")
}

// C-x d starts at the current buffer's directory, which is also the first
// candidate, so RET alone lists it.
func TestDiredPromptListsTheCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", filepath.Join("sub", "b"))
	t.Chdir(dir)
	e, scr := newTestEditor(t)

	feed(t, scr, key(t, "RET"))
	press(t, e, "C-x", "d")

	st := e.diredOf(e.active.Buf)
	if st == nil {
		t.Fatalf("C-x d RET left %q showing, want a listing", e.BufferName(e.active.Buf))
	}
	if real, _ := filepath.EvalSymlinks(dir); st.dir != dir && st.dir != real {
		t.Errorf("C-x d RET lists %s, want %s", st.dir, dir)
	}
}

// C-x C-j lists the directory of the file being edited, with point on the file.
func TestDiredJumpLandsOnTheFile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a", "b", "c")
	e, _ := newTestEditor(t)
	if err := e.visitFile(filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}

	press(t, e, "C-x", "C-j")

	if st := e.diredOf(e.active.Buf); st == nil || st.dir != dir {
		t.Fatalf("C-x C-j shows %q, want a listing of %s", e.BufferName(e.active.Buf), dir)
	}
	if got := atEntry(t, e); got != "b" {
		t.Errorf("point is on %q, want b", got)
	}
}

// q puts the previous buffer back, and C-x b RET does not bring the listing
// straight back: it has gone to the end of the list.
func TestDiredQuitWindow(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a")
	e, _ := newTestEditor(t)
	scratch := e.active.Buf
	b, _ := listed(t, e, dir)
	press(t, e, "n") // a command, so the listing counts as visited

	press(t, e, "q")

	if e.active.Buf != scratch {
		t.Errorf("q shows %q, want *scratch*", e.BufferName(e.active.Buf))
	}
	if bufs := e.Buffers(); bufs[len(bufs)-1] != b {
		t.Error("q did not move the listing to the back of the buffer list")
	}
}

// In a listing, help reports dired's keys, and a global key dired has taken
// over is not claimed for its global command.
func TestDiredKeysAppearInHelp(t *testing.T) {
	dir := t.TempDir()
	e, _ := newTestEditor(t)
	if got := e.Bindings()["d"]; got != "" {
		t.Fatalf("outside dired, d is bound to %q", got)
	}
	listed(t, e, dir)

	if got := e.Bindings()["d"]; got != "dired-flag-file-deletion" {
		t.Errorf("in dired, Bindings says d runs %q", got)
	}
	if slices.Contains(e.Where("next-line"), "C-n") {
		t.Errorf("Where(next-line) = %q; C-n runs dired-next-line here", e.Where("next-line"))
	}
}

// w copies the name at point, as a fresh kill.
func TestDiredCopiesTheFileName(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "name.txt")
	e, _ := newTestEditor(t)
	listed(t, e, dir)

	press(t, e, "w")

	if got, err := e.ring.Yank(); err != nil || got != "name.txt" {
		t.Errorf("kill ring holds %q (%v), want name.txt", got, err)
	}
}

// A listing is drawn as one: coloured from the Listing, with no line numbers,
// and the modeline says what it is.
func TestDiredRenders(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, filepath.Join("sub", "x"), "file.txt")
	e, scr := newTestEditor(t)
	listed(t, e, dir)

	e.Redraw()

	row := func(y int) string {
		cells, w, _ := scr.GetContents()
		var sb strings.Builder
		for x := range w {
			if rs := cells[y*w+x].Runes; len(rs) > 0 {
				sb.WriteString(string(rs))
			} else {
				sb.WriteByte(' ')
			}
		}
		return sb.String()
	}
	if got := row(0); !strings.HasPrefix(got, " ") || !strings.Contains(got, filepath.Base(dir)) {
		t.Errorf("row 0 = %q, want the header with no line number", got)
	}
	if got := row(dired.FirstEntry); !strings.Contains(got, "../") {
		t.Errorf("row %d = %q, want the parent entry", dired.FirstEntry, got)
	}
	if got := row(dired.FirstEntry + 1); !strings.Contains(got, "sub/") {
		t.Errorf("row %d = %q, want the sub/ entry", dired.FirstEntry+1, got)
	}
	if !strings.Contains(row(22), "dired") {
		t.Errorf("modeline = %q, want it to say dired", row(22))
	}
}

// The parent entry sits at the top: RET on it goes up, with point on the
// directory just left, as ^ does. Nothing acts on it - D there would otherwise
// offer to delete the directory being looked at.
func TestDiredParentEntry(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, filepath.Join("sub", "x"))
	e, _ := newTestEditor(t)
	_, st := listed(t, e, filepath.Join(dir, "sub"))

	if got := atEntry(t, e); got != "x" {
		t.Errorf("a new listing puts point on %q, want the first file, not ..", got)
	}
	press(t, e, "p")
	if got := atEntry(t, e); got != ".." {
		t.Fatalf("p from the first file lands on %q, want ..", got)
	}

	for _, k := range []string{"m", "d", "D", "R", "w"} {
		press(t, e, k)
		wantEcho(t, e, "Cannot operate on ..")
		if len(st.marks) != 0 {
			t.Errorf("%s marked the parent entry", k)
		}
	}
	if !exists(dir) {
		t.Fatal("the parent directory is gone")
	}

	press(t, e, "RET")
	if st.dir != dir {
		t.Fatalf("RET on .. lists %s, want %s", st.dir, dir)
	}
	if got := atEntry(t, e); got != "sub" {
		t.Errorf("after RET on .., point is on %q, want sub", got)
	}
}

// Icons show in listings and beside file and buffer candidates, and turning
// them off takes them out of open listings too.
func TestIcons(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "main.go")
	e, _ := newTestEditor(t)
	e.SetIcons(true)
	b, st := listed(t, e, dir)
	line, _ := st.list.LineOf("main.go")
	if got := b.Line(line).String(); !strings.Contains(got, " main.go") {
		t.Errorf("listing line = %q, want the Go icon before the name", got)
	}

	e.SetIcons(false)
	if got := b.Line(line).String(); strings.Contains(got, "") {
		t.Errorf("after turning icons off the listing still has one: %q", got)
	}
	if got := atEntry(t, e); got != "main.go" {
		t.Errorf("point moved off main.go to %q when the icons went", got)
	}
}

// A file prompt's candidates carry their icons; the prompt's answer does not.
func TestPromptCandidatesCarryIcons(t *testing.T) {
	names := []string{"src/", "main.go"}
	e, _ := promptOn(t, 60, 20, command.ReadOpts{
		Prompt:   "Find file: ",
		Complete: command.CompleteFrom(names),
		Icon:     icons.ForCandidate,
	}, "")
	e.SetIcons(true)
	rows := e.frame().MiniRows
	if len(rows) != 2 || rows[0].Icon != '' || rows[1].Icon != '' {
		t.Fatalf("rows = %+v, want a folder and the Go mark", rows)
	}
	if rows[1].Text != "main.go" {
		t.Errorf("candidate text = %q; the icon must not be part of it", rows[1].Text)
	}

	e.SetIcons(false)
	if rows := e.frame().MiniRows; rows[0].Icon != 0 {
		t.Error("icons switched off, but candidates still carry them")
	}
}

// A listing longer than the window still opens with its header in view, and
// walking into another directory starts at the top again.
func TestDiredOpensWithTheHeaderInView(t *testing.T) {
	dir := t.TempDir()
	var names []string
	for i := range 40 {
		names = append(names, filepath.Join("sub", string(rune('a'+i%26))+string(rune('a'+i/26))))
		names = append(names, string(rune('a'+i%26))+string(rune('a'+i/26))+".txt")
	}
	writeFiles(t, dir, names...)
	e, scr := newTestEditor(t)

	b, err := e.OpenFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	e.active.Visit(b)
	e.Redraw()
	if got := screenRow(t, scr, 0); !strings.Contains(got, filepath.Base(dir)) {
		t.Fatalf("row 0 = %q, want the header", got)
	}

	press(t, e, "M->")
	goTo(t, e, "sub")
	press(t, e, "RET")
	e.Redraw()
	if got := screenRow(t, scr, 0); !strings.Contains(got, "sub/") {
		t.Errorf("after walking into sub, row 0 = %q, want its header", got)
	}
}

// "auto" asks the guess; true and false do not. The guess is stubbed out for
// the whole test binary, so it is swapped here to see that it is consulted.
func TestIconsSettingAuto(t *testing.T) {
	saved := detectIcons
	t.Cleanup(func() { detectIcons = saved })
	asked := false
	detectIcons = func() bool { asked = true; return true }

	e, _ := newTestEditor(t)
	s := lua.DefaultSettings()
	e.applySettings(s)
	if !asked || !e.icons {
		t.Errorf("auto: asked %v, icons %v; want the guess asked and followed", asked, e.icons)
	}

	asked = false
	s.Icons = "off"
	e.applySettings(s)
	if asked || e.icons {
		t.Errorf("off: asked %v, icons %v; want neither", asked, e.icons)
	}
}
