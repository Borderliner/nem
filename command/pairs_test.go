package command_test

import (
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

// typeRune runs self-insert-command for r, as the event loop would.
func typeRune(t *testing.T, f *commandtest.Fake, r rune) {
	t.Helper()
	f.Seq().LastRune = r
	runAll(t, f, "self-insert-command")
}

// typeAt types rs into a one-line buffer at col and returns the line and
// point's column.
func typeAt(t *testing.T, line string, col int, rs string) (string, int) {
	t.Helper()
	f := fileEnv("a.go", line)
	f.SetPoint(text.Pos{Col: text.RuneIdx(col)})
	for _, r := range rs {
		typeRune(t, f, r)
	}
	return f.Text(), int(f.Point().Col)
}

func TestAutoPair(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		col        int
		typed      string
		want       string
		pt         int
	}{
		{"opener pairs at line end", "f", 1, "(", "f()", 2},
		{"typing through the closer", "f", 1, "(x)", "f(x)", 4},
		{"nested", "", 0, "([", "([])", 2},
		{"not before a word", "word", 0, "(", "(word", 1},
		{"quote pairs", "x = ", 4, `"`, `x = ""`, 5},
		{"quote typed through", "x = ", 4, `"hi"`, `x = "hi"`, 8},
		{"apostrophe after a word", "don", 3, "'", "don'", 4},
		{"before a comma", "f(a, b)", 3, "[", "f(a[], b)", 4},
	} {
		got, pt := typeAt(t, tc.line, tc.col, tc.typed)
		if got != tc.want || pt != tc.pt {
			t.Errorf("%s: %q point %d, want %q point %d", tc.name, got, pt, tc.want, tc.pt)
		}
	}
}

// Backspace between an empty pair takes both halves, and only then.
func TestAutoPairBackspace(t *testing.T) {
	f := fileEnv("a.go", "f()")
	f.SetPoint(text.Pos{Col: 2})
	runAll(t, f, "delete-backward-char")
	if got := f.Text(); got != "f" {
		t.Errorf("between (): %q", got)
	}
	f = fileEnv("a.go", "f(x)")
	f.SetPoint(text.Pos{Col: 3})
	runAll(t, f, "delete-backward-char")
	if got := f.Text(); got != "f()" {
		t.Errorf("after x: %q", got)
	}
}

// Off, typing is plain typing.
func TestAutoPairOff(t *testing.T) {
	command.AutoPair = false
	t.Cleanup(func() { command.AutoPair = true })
	if got, _ := typeAt(t, "f", 1, "("); got != "f(" {
		t.Errorf("with auto-pair off: %q", got)
	}
}
