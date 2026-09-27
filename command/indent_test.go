package command_test

import (
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
