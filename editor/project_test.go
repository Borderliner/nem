package editor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/text"
)

// aProject makes a project - a directory with a projectile marker, so the
// tests do not need git - holding files with the given contents.
func aProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, project.Marker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// visiting opens path in the active window.
func visiting(t *testing.T, e *Editor, path string) *text.Buffer {
	t.Helper()
	if err := e.visitFile(path); err != nil {
		t.Fatalf("visiting %s: %v", path, err)
	}
	return e.active.Buf
}

func wantVisiting(t *testing.T, e *Editor, path string) {
	t.Helper()
	if got := e.active.Buf.Path(); got != path {
		t.Fatalf("visiting %q, want %q (echo %q)", got, path, e.Message())
	}
}

// C-x p f finds any file of the project by a few letters of its name, and
// the project becomes a known one.
func TestProjectFindFile(t *testing.T) {
	root := aProject(t, map[string]string{"main.go": "", "pkg/util.go": "", "docs/guide.md": ""})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "main.go"))

	feed(t, scr, txt("utl"), key(t, "RET"))
	press(t, e, "C-x", "p", "f")
	wantVisiting(t, e, filepath.Join(root, "pkg", "util.go"))
	if len(e.mem.Projects) == 0 || e.mem.Projects[0] != root {
		t.Errorf("known projects %q, want %s first", e.mem.Projects, root)
	}

	// The files opened lately come first, so RET alone goes back.
	feed(t, scr, key(t, "RET"))
	press(t, e, "C-x", "p", "f")
	wantVisiting(t, e, filepath.Join(root, "main.go"))
}

// Opening a file in a project makes the project a known one.
func TestOpeningAFileNotesItsProject(t *testing.T) {
	root := aProject(t, map[string]string{"a.go": ""})
	e, _ := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "a.go"))
	if !slices.Contains(e.mem.Projects, root) {
		t.Errorf("known projects %q, want %s", e.mem.Projects, root)
	}
}

// Outside every project, the project commands ask for a known one rather
// than refusing; C-x p p does the same from anywhere.
func TestProjectSwitch(t *testing.T) {
	root := aProject(t, map[string]string{"main.go": "", "pkg/util.go": ""})
	e, scr := newTestEditor(t)
	t.Chdir(t.TempDir())
	e.mem.AddProject(root)

	feed(t, scr, key(t, "RET"), txt("util"), key(t, "RET"))
	press(t, e, "C-x", "p", "f")
	wantVisiting(t, e, filepath.Join(root, "pkg", "util.go"))

	other := aProject(t, map[string]string{"notes.txt": ""})
	e.mem.AddProject(other)
	feed(t, scr, txt(filepath.Base(other)), key(t, "RET"), txt("notes"), key(t, "RET"))
	press(t, e, "C-x", "p", "p")
	wantVisiting(t, e, filepath.Join(other, "notes.txt"))
}

// C-x p b offers only the project's buffers.
func TestProjectBuffers(t *testing.T) {
	root := aProject(t, map[string]string{"a.go": "", "b.go": ""})
	elsewhere := filepath.Join(t.TempDir(), "x.txt")
	e, _ := newTestEditor(t)
	a := visiting(t, e, filepath.Join(root, "a.go"))
	b := visiting(t, e, filepath.Join(root, "b.go"))
	visiting(t, e, elsewhere)
	listing, err := e.Dired(root)
	if err != nil {
		t.Fatal(err)
	}

	got := e.projectBuffers(root)
	for _, want := range []*text.Buffer{a, b, listing} {
		if !slices.Contains(got, want) {
			t.Errorf("project buffers leave out %s", e.BufferName(want))
		}
	}
	if len(got) != 3 {
		t.Errorf("project buffers are %d, want 3: not the file elsewhere or *scratch*", len(got))
	}
}

// C-x p g lists the matching lines of every file, unsaved edits included,
// and RET and M-g n go to them.
func TestProjectGrep(t *testing.T) {
	root := aProject(t, map[string]string{
		"a.go":     "package a\n\nfunc Needle() {}\n",
		"sub/b.go": "// no match here\nvar needle = 1\n",
		"c.txt":    "nothing\n",
	})
	e, scr := newTestEditor(t)
	c := visiting(t, e, filepath.Join(root, "c.txt"))
	if err := c.Insert(text.Pos{Line: 0}, []rune("unsaved needle ")); err != nil {
		t.Fatal(err)
	}

	feed(t, scr, txt("needle"), key(t, "RET"))
	press(t, e, "C-x", "p", "g")
	st := e.grepOf(e.active.Buf)
	if st == nil {
		t.Fatalf("no results shown; echo %q", e.Message())
	}
	if len(st.matches) != 3 {
		t.Fatalf("found %+v, want a line in each of three files", st.matches)
	}
	if len(e.tree.Windows()) != 2 {
		t.Error("the results are not beside the file")
	}
	wantEcho(t, e, "3 matches in 3 files")
	body := e.active.Buf.String()
	for _, want := range []string{"needle  in", "a.go", "sub/b.go", "3  func Needle() {}"} {
		if !strings.Contains(body, want) {
			t.Errorf("results lack %q:\n%s", want, body)
		}
	}

	// RET on the first match goes there, in the other window.
	results := e.active
	press(t, e, "RET")
	if e.active == results {
		t.Fatal("RET stayed in the results")
	}
	wantVisiting(t, e, filepath.Join(root, "a.go"))
	wantPt(t, e, 2, 5)

	// M-g n steps on from anywhere.
	press(t, e, "M-g", "n")
	wantVisiting(t, e, filepath.Join(root, "c.txt"))
	wantPt(t, e, 0, 8)
	press(t, e, "M-g", "n", "M-g", "n")
	wantEcho(t, e, "no more matches")
	press(t, e, "M-g", "p")
	wantVisiting(t, e, filepath.Join(root, "c.txt"))
}

// With nothing typed, C-x p g searches for the word at point.
func TestProjectGrepDefault(t *testing.T) {
	root := aProject(t, map[string]string{"a.go": "call(widget)\n", "b.go": "widget := 1\n"})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "a.go"))
	e.active.Pt = text.Pos{Line: 0, Col: 7}

	feed(t, scr, key(t, "RET"))
	press(t, e, "C-x", "p", "g")
	if st := e.grepOf(e.active.Buf); st == nil || st.pattern != "widget" || len(st.matches) != 2 {
		t.Fatalf("searched for %+v; want widget, twice", st)
	}
}

// In the results, n and p step between matches showing each in the other
// window, and } goes to the next file.
func TestGrepResultsKeys(t *testing.T) {
	root := aProject(t, map[string]string{"a.txt": "hit\nhit\n", "b.txt": "hit\n"})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "a.txt"))
	feed(t, scr, txt("hit"), key(t, "RET"))
	press(t, e, "C-x", "p", "g")
	results := e.active
	st := e.grepOf(results.Buf)

	press(t, e, "n")
	if e.active != results || st.cur != 0 {
		t.Errorf("first n: active is the results %v, current match %d; want to stay, showing match 1", e.active == results, st.cur)
	}
	if other := e.tree.Windows(); len(other) != 2 || other[0].Buf.Path() != filepath.Join(root, "a.txt") {
		t.Error("n did not show the match in the other window")
	}
	press(t, e, "n")
	if st.cur != 1 {
		t.Errorf("second n: current match %d, want 2", st.cur)
	}
	press(t, e, "}")
	if m, ok := st.matchNear(results.Pt.Line); !ok || st.matches[m].File != "b.txt" {
		t.Errorf("} went to line %d, not b.txt's heading", results.Pt.Line)
	}
	press(t, e, "p")
	if st.cur != 1 {
		t.Errorf("p: current match %d, want 2 again", st.cur)
	}
}

// C-x p r replaces across the files that match, asking at each: here yes,
// no, then Y for everything left. The files are left modified for C-x p S.
func TestProjectQueryReplace(t *testing.T) {
	root := aProject(t, map[string]string{"a.txt": "foo foo\n", "b.txt": "foo\nfoo\n", "c.txt": "none\n"})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "c.txt"))

	feed(t, scr, txt("foo"), key(t, "RET"), txt("bar"), key(t, "RET"), txt("ynY"))
	press(t, e, "C-x", "p", "r")
	wantEcho(t, e, "Replaced 3 occurrences in 2 files")

	for name, want := range map[string]string{"a.txt": "bar foo", "b.txt": "bar\nbar"} {
		b, _ := e.OpenFile(filepath.Join(root, name))
		if got := b.String(); got != want {
			t.Errorf("%s reads %q, want %q", name, got, want)
		}
		if disk, _ := os.ReadFile(filepath.Join(root, name)); strings.Contains(string(disk), "bar") {
			t.Errorf("%s was saved before being asked to be", name)
		}
	}

	press(t, e, "C-x", "p", "S")
	wantEcho(t, e, "Saved 2 files")
	if disk, _ := os.ReadFile(filepath.Join(root, "b.txt")); string(disk) != "bar\nbar\n" {
		t.Errorf("b.txt on disk is %q after C-x p S", disk)
	}
}

// C-x p t goes from a file to its test and back.
func TestProjectToggleTest(t *testing.T) {
	root := aProject(t, map[string]string{"calc.go": "", "calc_test.go": "", "other.go": ""})
	e, _ := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "calc.go"))
	press(t, e, "C-x", "p", "t")
	wantVisiting(t, e, filepath.Join(root, "calc_test.go"))
	press(t, e, "C-x", "p", "t")
	wantVisiting(t, e, filepath.Join(root, "calc.go"))

	visiting(t, e, filepath.Join(root, "other.go"))
	press(t, e, "C-x", "p", "t")
	wantEcho(t, e, "No test found for other.go")
}

// C-x p k kills the project's buffers after asking, and leaves the rest.
func TestProjectKillBuffers(t *testing.T) {
	root := aProject(t, map[string]string{"a.go": "", "b.go": ""})
	e, scr := newTestEditor(t)
	scratch := e.active.Buf
	a := visiting(t, e, filepath.Join(root, "a.go"))
	b := visiting(t, e, filepath.Join(root, "b.go"))

	feed(t, scr, txt("y"))
	press(t, e, "C-x", "p", "k")
	for _, gone := range []*text.Buffer{a, b} {
		if slices.Contains(e.buffers, gone) {
			t.Errorf("%s was not killed", gone.Path())
		}
	}
	if !slices.Contains(e.buffers, scratch) {
		t.Error("*scratch* was killed too")
	}
	wantEcho(t, e, "Killed 2 buffers")
}

// C-x p d lists a directory of the project found by name.
func TestProjectFindDir(t *testing.T) {
	root := aProject(t, map[string]string{"a.go": "", "internal/deep/x.go": ""})
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "a.go"))
	feed(t, scr, txt("deep"), key(t, "RET"))
	press(t, e, "C-x", "p", "d")
	if st := e.diredOf(e.active.Buf); st == nil || st.dir != filepath.Join(root, "internal", "deep") {
		t.Fatalf("C-x p d did not list internal/deep; echo %q", e.Message())
	}
}

// M-g n with no search says how to make one.
func TestNextErrorWithoutASearch(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "M-g", "n")
	wantEcho(t, e, "C-x p g")
}

// The bindings docs/config.md suggests load: the results' mode is bindable
// by name, and projectile's C-c p can be had.
func TestProjectBindingsFromConfig(t *testing.T) {
	e, _ := newTestEditor(t)
	path := writeConfig(t, `
nem.bind("TAB", "grep-display-match", "grep")
nem.bind("C-c p f", "project-find-file")
`)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	t.Cleanup(e.CloseConfig)
	if got := e.keys.Bindings()["C-c p f"]; got != "project-find-file" {
		t.Errorf("C-c p f runs %q", got)
	}
	if got := e.grepKeys.Bindings()["<tab>"]; got != "grep-display-match" {
		t.Errorf("TAB in the results runs %q", got)
	}
}
