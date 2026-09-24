package command_test

import (
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

// runAll dispatches name through the full default registry, as the editor would.
func runAll(t *testing.T, f *commandtest.Fake, name string) {
	t.Helper()
	if err := allCommands(t).Run(name, f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	f.SetLastCommand(name)
}

// allCommands is the full default registry.
func allCommands(t *testing.T) *command.Registry {
	t.Helper()
	r, err := command.NewDefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// fileEnv is a fake editing a file called name, holding lines.
func fileEnv(name string, lines ...string) *commandtest.Fake {
	f := commandtest.New(lines...)
	f.Buf().SetPath("/src/" + name)
	return f
}

// selectLines activates a region from the start of line a to the start of
// line b.
func selectLines(f *commandtest.Fake, a, b int) {
	f.Buf().SetMark(text.Pos{Line: a})
	f.Buf().ActivateMark()
	f.SetPoint(text.Pos{Line: b})
}

func TestCommentTogglesTheCurrentLine(t *testing.T) {
	f := fileEnv("main.go", "\tx := 1", "y")
	f.SetPoint(text.Pos{Line: 0, Col: 3})

	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "\t// x := 1\ny" {
		t.Fatalf("commented: %q", got)
	}
	if got := f.Point(); got != (text.Pos{Line: 0, Col: 6}) {
		t.Errorf("point %v, want it to stay on the x", got)
	}

	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "\tx := 1\ny" {
		t.Errorf("uncommented: %q", got)
	}
	if got := f.Point(); got != (text.Pos{Line: 0, Col: 3}) {
		t.Errorf("point %v after uncommenting, want it back on the x", got)
	}
}

// A region's lines are commented in one column - the least indentation - and
// a region ending at the start of a line does not take that line in.
func TestCommentARegionInOneColumn(t *testing.T) {
	f := fileEnv("a.py", "if x:", "    y()", "", "z()")
	selectLines(f, 0, 3)

	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "# if x:\n#     y()\n\nz()" {
		t.Fatalf("commented region: %q", got)
	}

	selectLines(f, 0, 3)
	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "if x:\n    y()\n\nz()" {
		t.Errorf("uncommented region: %q", got)
	}
}

// Half-commented regions are commented, not uncommented: the toggle only goes
// back when every line is already a comment.
func TestCommentAMixedRegionComments(t *testing.T) {
	f := fileEnv("a.lua", "-- x", "y")
	selectLines(f, 0, 1)
	f.SetPoint(text.Pos{Line: 1, Col: 1})

	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "-- -- x\n-- y" {
		t.Errorf("mixed region: %q", got)
	}
}

func TestCommentBlockSyntax(t *testing.T) {
	f := fileEnv("index.html", "<p>hi</p>")
	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "<!-- <p>hi</p> -->" {
		t.Fatalf("commented: %q", got)
	}
	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "<p>hi</p>" {
		t.Errorf("uncommented: %q", got)
	}
}

// On an empty line, a comment is started, with point where its text goes.
func TestCommentStartsOnAnEmptyLine(t *testing.T) {
	f := fileEnv("x.rs", "    ")
	f.SetPoint(text.Pos{Line: 0, Col: 4})
	runAll(t, f, "comment-dwim")
	if got := f.Text(); got != "    // " {
		t.Errorf("text %q", got)
	}
	if got := f.Point(); got != (text.Pos{Line: 0, Col: 7}) {
		t.Errorf("point %v, want after the marker", got)
	}
}

// Files known by name, and a buffer with no file at all, still get a marker.
func TestCommentSyntaxByName(t *testing.T) {
	for name, want := range map[string]string{"Makefile": "# all:", "Dockerfile": "# all:", "notes": "# all:"} {
		f := fileEnv(name, "all:")
		runAll(t, f, "comment-dwim")
		if got := f.Text(); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}
