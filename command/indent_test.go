package command_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

// fileFake is a fake editing a file called name, in a directory of its own
// whose .editorconfig is root = true followed by config. The root line is
// what keeps the test to what it wrote: without it the lookup would carry on
// into whatever .editorconfig files the machine running it has above its
// temporary directory.
func fileFake(t *testing.T, name, config string, lines ...string) *commandtest.Fake {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n"+config), 0o644); err != nil {
		t.Fatal(err)
	}
	f := commandtest.New(lines...)
	f.Buf().SetPath(filepath.Join(dir, name))
	return f
}

// spaces4 is an .editorconfig section settling that every file is indented
// four spaces at a time, for tests about what is done with the indentation
// rather than about how it is found out.
const spaces4 = "[*]\nindent_style = space\nindent_size = 4\n"

// tryRun runs name through the default registry, returning what it
// returned.
func tryRun(t *testing.T, f *commandtest.Fake, name string) error {
	t.Helper()
	return allCommands(t).Run(name, f)
}

// activate makes the region from mark to point the active one.
func activate(f *commandtest.Fake, mark, point text.Pos) {
	f.Buf().SetMark(mark)
	f.Buf().ActivateMark()
	f.SetPoint(point)
}

func textIs(t *testing.T, f *commandtest.Fake, want string) {
	t.Helper()
	if got := f.Text(); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func pointIs(t *testing.T, f *commandtest.Fake, want text.Pos) {
	t.Helper()
	if got := f.Point(); got != want {
		t.Errorf("point = %v, want %v", got, want)
	}
}

func markIs(t *testing.T, f *commandtest.Fake, want text.Pos) {
	t.Helper()
	if got := f.Buf().Mark(); got != want {
		t.Errorf("mark = %v, want %v", got, want)
	}
}

// --- which way a buffer indents -------------------------------------------

func TestIndentForAFileWithNothingToGoOnIsItsLanguages(t *testing.T) {
	for _, tc := range []struct {
		name string
		want command.Indent
	}{
		{"main.go", command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"Makefile", command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"rules.mk", command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"app.py", command.Indent{Width: 4}},
		{"lib.rs", command.Indent{Width: 4}},
		{"main.c", command.Indent{Width: 4}},
		{"deploy.sh", command.Indent{Width: 4}},
		{"app.tsx", command.Indent{Width: 2}},
		{"config.YAML", command.Indent{Width: 2}},
		{"init.lua", command.Indent{Width: 2}},
		{"README.md", command.Indent{Width: 2}},
		// What nem does not recognise keeps TAB's old behaviour.
		{"notes.txt", command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"LICENSE", command.Indent{Tabs: true, Width: int(text.TabWidth)}},
	} {
		f := fileFake(t, tc.name, "")
		if got := command.IndentFor(f.Buf()); got != tc.want {
			t.Errorf("%s: IndentFor = %+v, want %+v", tc.name, got, tc.want)
		}
	}

	// And a buffer with no file at all.
	if got, want := command.IndentFor(commandtest.New().Buf()), (command.Indent{Tabs: true, Width: int(text.TabWidth)}); got != want {
		t.Errorf("no file: IndentFor = %+v, want %+v", got, want)
	}
}

// What the text already does beats the language's convention.
func TestIndentForFollowsTheText(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  command.Indent
	}{
		{"old.py", []string{"def f():", "\treturn 1"}, command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"odd.go", []string{"func f() {", "    return", "}"}, command.Indent{Width: 4}},
		{"notes.txt", []string{"a", "  b", "    c", "  d"}, command.Indent{Width: 2}},
		{"notes.txt", []string{"a", "    b", "        c", "    d", "e", "    f"}, command.Indent{Width: 4}},
		// Every indented line at one depth: the depth is the step.
		{"notes.txt", []string{"a", "   b", "   c"}, command.Indent{Width: 3}},
		// Mostly tabs, with a doc comment's " * " and a stray space-indented
		// line.
		{"x.c", []string{"/*", " * doc", " */", "int f() {", "\tif (x) {", "\t\ty();", "\t}", "  z();", "}"}, command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		// Continuation lines in a two-space file do not make it four.
		{"a.txt", []string{"a", "  b", "      c", "  d", "    e", "  f", "    g"}, command.Indent{Width: 2}},
		// Blank lines between do not break the step.
		{"a.txt", []string{"a", "", "    b", "", "", "        c"}, command.Indent{Width: 4}},
	} {
		f := fileFake(t, tc.name, "", tc.lines...)
		if got := command.IndentFor(f.Buf()); got != tc.want {
			t.Errorf("%s %q: IndentFor = %+v, want %+v", tc.name, tc.lines, got, tc.want)
		}
	}
}

// An .editorconfig beats both, and each thing it leaves out comes from where
// it would have.
func TestIndentForFollowsTheEditorconfig(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		lines        []string
		want         command.Indent
	}{
		{"main.go", "[*]\nindent_style = space\nindent_size = 3\n", []string{"f {", "\tx", "}"}, command.Indent{Width: 3}},
		{"app.py", "[*.py]\nindent_style = tab\n", nil, command.Indent{Tabs: true, Width: int(text.TabWidth)}},
		{"app.py", "[*.py]\nindent_style = tab\ntab_width = 4\n", nil, command.Indent{Tabs: true, Width: 4}},
		// A size alone keeps the language's style.
		{"app.js", "[*]\nindent_size = 4\n", nil, command.Indent{Width: 4}},
		// A style alone takes its width from the text.
		{"notes.txt", "[*]\nindent_style = space\n", []string{"a", "   b", "      c"}, command.Indent{Width: 3}},
		// Or, with nothing else, from the language.
		{"app.js", "[*]\nindent_style = space\n", nil, command.Indent{Width: 2}},
		// And spaces known of nowhere else are four.
		{"notes.txt", "[*]\nindent_style = space\n", nil, command.Indent{Width: 4}},
		// A section for other files says nothing.
		{"main.go", "[*.py]\nindent_style = space\n", nil, command.Indent{Tabs: true, Width: int(text.TabWidth)}},
	} {
		f := fileFake(t, tc.name, tc.config, tc.lines...)
		if got := command.IndentFor(f.Buf()); got != tc.want {
			t.Errorf("%s with %q: IndentFor = %+v, want %+v", tc.name, tc.config, got, tc.want)
		}
	}
}

// --- TAB at point -----------------------------------------------------------

func TestTabInsertsTheFilesIndentation(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"main.go", "\tx"},
		{"app.py", "    x"},
		{"app.ts", "  x"},
	} {
		f := fileFake(t, tc.name, "", "x")
		if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
			t.Fatal(err)
		}
		textIs(t, f, tc.want)
		pointIs(t, f, text.Pos{Col: text.RuneIdx(len(tc.want) - 1)})
	}
}

// Spaces go to the next tab stop, not a fixed number further.
func TestTabWithSpacesStopsAtTheNextMultiple(t *testing.T) {
	f := fileFake(t, "app.py", "", "ab")
	f.SetPoint(text.Pos{Col: 2})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "ab  ")
	pointIs(t, f, text.Pos{Col: 4})

	// A tab before point counts as the columns it covers on screen.
	f = fileFake(t, "app.py", spaces4, "\tx")
	f.SetPoint(text.Pos{Col: 1})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "\t"+strings.Repeat(" ", 4)+"x")
}

func TestTabWithAnArgIndentsThatManyLevels(t *testing.T) {
	f := fileFake(t, "app.py", "", "a")
	f.SetPoint(text.Pos{Col: 1})
	edArg(f, 3)
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a"+strings.Repeat(" ", 11)) // 3 to the stop, then two levels

	f = fileFake(t, "main.go", "", "")
	edArg(f, 3)
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "\t\t\t")
}

// C-SPC then TAB is not a region: it indents at point, and the mark it set
// does not become a selection of what was inserted.
func TestTabWithAnEmptyRegionIndentsAtPoint(t *testing.T) {
	f := fileFake(t, "app.py", "", "ab")
	activate(f, text.Pos{Col: 1}, text.Pos{Col: 1})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a   b")
	if f.Buf().MarkActive() {
		t.Error("the region is active after TAB at point")
	}
}

// --- TAB with a region ------------------------------------------------------

// Every line the region touches moves one level right; the region ending at
// the start of a line does not take that line in; and the region stays, on
// the same lines, so TAB can go again.
func TestTabShiftsTheRegionsLines(t *testing.T) {
	f := fileFake(t, "app.py", "", "if x:", "    y()", "z()")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 2})

	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "    if x:\n        y()\nz()")
	markIs(t, f, text.Pos{Line: 0})
	pointIs(t, f, text.Pos{Line: 2})
	if !f.Buf().MarkActive() {
		t.Fatal("the region is not active after TAB")
	}

	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "        if x:\n            y()\nz()")
}

func TestTabOnARegionOfTabs(t *testing.T) {
	f := fileFake(t, "main.go", "", "a", "\tb", "c")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 1, Col: 2})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "\ta\n\t\tb\nc")
}

// Point and mark in the text move with it.
func TestTabKeepsPointAndMarkOnTheirText(t *testing.T) {
	f := fileFake(t, "app.py", "", "ab", "  cd")
	activate(f, text.Pos{Line: 0, Col: 1}, text.Pos{Line: 1, Col: 3})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	// Two at a time, as the text already is.
	textIs(t, f, "  ab\n    cd")
	markIs(t, f, text.Pos{Line: 0, Col: 3})
	pointIs(t, f, text.Pos{Line: 1, Col: 5})
}

// Blank lines stay as they are, whitespace-only ones included, so shifting
// never leaves whitespace at the end of a line.
func TestTabLeavesBlankLinesAlone(t *testing.T) {
	f := fileFake(t, "app.py", "", "a", "", "   ", "b", "")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 4})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "    a\n\n   \n    b\n")
}

func TestTabOnARegionWithAnArg(t *testing.T) {
	f := fileFake(t, "app.js", "", "a", "b")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 1, Col: 1})
	edArg(f, 2)
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "    a\n    b")
}

// Mixed indentation is rewritten the file's way as it shifts: in a file of
// tabs, what adds up to a tab becomes one.
func TestShiftingRewritesIndentationTheFilesWay(t *testing.T) {
	f := fileFake(t, "x.go", "[*]\nindent_style = tab\ntab_width = 4\n", "  a", "      b")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 1, Col: 7})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "\t  a\n\t\t  b")
}

// A shift is one undo step, however many lines it touched.
func TestShiftingUndoesAsOneStep(t *testing.T) {
	f := fileFake(t, "app.py", "", "a", "b", "c")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 2, Col: 1})
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	f.Buf().BreakUndo()
	if _, ok := f.Buf().Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	textIs(t, f, "a\nb\nc")
}

// A buffer that refuses the edit is left exactly as it was - even one that
// would have accepted the first lines, and refuses only a later one.
// A replaced IndentFor that says a level has no width does not bring the
// editor down: it is taken as one column.
func TestAZeroWidthLevelIsOneColumn(t *testing.T) {
	was := command.IndentFor
	t.Cleanup(func() { command.IndentFor = was })
	command.IndentFor = func(*text.Buffer) command.Indent { return command.Indent{} }

	f := commandtest.New("a", "b")
	if err := tryRun(t, f, "indent-for-tab-command"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, " a\nb")
	activate(f, text.Pos{}, text.Pos{Line: 1, Col: 1})
	for _, name := range []string{"indent-for-tab-command", "indent-rigidly-left-to-tab-stop", "indent-rigidly"} {
		if err := tryRun(t, f, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	textIs(t, f, "  a\n b")
}

func TestShiftingIsRefusedWhole(t *testing.T) {
	f := fileFake(t, "app.py", "", "a", "b", "c")
	f.Buf().SetReadOnly(true)
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 2, Col: 1})
	if err := tryRun(t, f, "indent-for-tab-command"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("read-only: err = %v, want ErrReadOnly", err)
	}
	textIs(t, f, "a\nb\nc")

	f = fileFake(t, "app.py", "", "a", "b", "c")
	refused := errors.New("not line 2")
	f.Buf().SetEditGuard(func(from, _ text.Pos, _ []rune) error {
		if from.Line == 2 {
			return refused
		}
		return nil
	})
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 2, Col: 1})
	if err := tryRun(t, f, "indent-for-tab-command"); !errors.Is(err, refused) {
		t.Errorf("guarded: err = %v, want the guard's", err)
	}
	textIs(t, f, "a\nb\nc")
}

// --- S-TAB ------------------------------------------------------------------

func TestBacktabRemovesOneLevel(t *testing.T) {
	for _, tc := range []struct {
		name, config, line, want string
	}{
		{"main.go", "", "\t\ta", "\ta"},
		{"main.go", "", "\ta", "a"},
		{"app.py", spaces4, "      a", "  a"},
		{"app.py", spaces4, "    a", "a"},
		// Less than a level: what there is.
		{"app.py", spaces4, "  a", "a"},
		// A tab is a whole level, whatever the file's style.
		{"app.py", spaces4, "\ta", "a"},
		{"app.py", spaces4, "a", "a"},
		// Alignment after the tabs is kept.
		{"main.go", "", "\t\t  a", "\t  a"},
	} {
		f := fileFake(t, tc.name, tc.config, tc.line)
		if err := tryRun(t, f, "indent-rigidly-left-to-tab-stop"); err != nil {
			t.Fatal(err)
		}
		if got := f.Text(); got != tc.want {
			t.Errorf("%s: S-TAB on %q = %q, want %q", tc.name, tc.line, got, tc.want)
		}
	}
}

func TestBacktabOnARegion(t *testing.T) {
	f := fileFake(t, "app.py", "", "    if x:", "        y()", "z()", "    w()")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 3})
	if err := tryRun(t, f, "indent-rigidly-left-to-tab-stop"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "if x:\n    y()\nz()\n    w()")
	markIs(t, f, text.Pos{Line: 0})
	pointIs(t, f, text.Pos{Line: 3})
	if !f.Buf().MarkActive() {
		t.Error("the region is not active after S-TAB")
	}
}

// Point in the indentation it removes goes to the text; point in the text
// stays on it.
func TestBacktabKeepsPointOnItsText(t *testing.T) {
	f := fileFake(t, "app.py", spaces4, "      abc")
	f.SetPoint(text.Pos{Col: 8})
	if err := tryRun(t, f, "indent-rigidly-left-to-tab-stop"); err != nil {
		t.Fatal(err)
	}
	pointIs(t, f, text.Pos{Col: 4})

	f = fileFake(t, "app.py", spaces4, "      abc")
	f.SetPoint(text.Pos{Col: 3})
	if err := tryRun(t, f, "indent-rigidly-left-to-tab-stop"); err != nil {
		t.Fatal(err)
	}
	pointIs(t, f, text.Pos{Col: 2})
}

// --- C-x TAB ----------------------------------------------------------------

func TestIndentRigidly(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		arg        int // 0: none
		want       string
	}{
		{"app.py", "a", 0, "    a"},
		{"main.go", "a", 0, "\ta"},
		{"app.py", "a", 2, "  a"},
		{"app.py", "    a", -2, "  a"},
		// Never past the first character.
		{"app.py", "  a", -10, "a"},
		// Columns, written the file's way.
		{"main.go", "\ta", 2, "\t  a"},
		{"main.go", "\ta", -2, "      a"},
		{"main.go", "\t  a", 6, "\t\ta"},
	} {
		f := fileFake(t, tc.name, "", tc.line)
		if tc.arg != 0 {
			edArg(f, tc.arg)
		}
		if err := tryRun(t, f, "indent-rigidly"); err != nil {
			t.Fatal(err)
		}
		if got := f.Text(); got != tc.want {
			t.Errorf("%s: C-u %d C-x TAB on %q = %q, want %q", tc.name, tc.arg, tc.line, got, tc.want)
		}
	}
}

func TestIndentRigidlyShiftsTheRegion(t *testing.T) {
	f := fileFake(t, "app.py", "", "a", "  b", "c")
	activate(f, text.Pos{Line: 0}, text.Pos{Line: 1, Col: 3})
	edArg(f, 3)
	if err := tryRun(t, f, "indent-rigidly"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "   a\n     b\nc")
	pointIs(t, f, text.Pos{Line: 1, Col: 6})
}
