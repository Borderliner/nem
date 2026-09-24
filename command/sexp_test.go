package command_test

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

// sexpAt runs name from col on a one-line buffer and reports where point ends.
func sexpAt(t *testing.T, line string, col int, name string) (int, error) {
	t.Helper()
	f := fileEnv("a.go", line)
	f.SetPoint(text.Pos{Col: text.RuneIdx(col)})
	err := allCommands(t).Run(name, f)
	return int(f.Point().Col), err
}

func TestForwardSexp(t *testing.T) {
	for _, tc := range []struct {
		line string
		from int
		to   int
	}{
		{"foo_bar baz", 0, 7},         // a word, underscores included
		{"  (a (b) c) d", 0, 11},      // a nested group
		{`f("(" + x) y`, 1, 10},       // a bracket inside a string is skipped
		{`s := "a\"b" + c`, 5, 11},    // an escaped quote does not end it
		{"a + b", 1, 5},               // punctuation is passed over
		{"x := []int{1, 2}", 4, 7},    // [] then int are separate
		{"`raw (` rest", 0, 7},        // a raw string
		{"{ a: [1, {b: 2}] }", 0, 18}, // mixed brackets
	} {
		got, err := sexpAt(t, tc.line, tc.from, "forward-sexp")
		if err != nil || got != tc.to {
			t.Errorf("C-M-f in %q from %d: %d (%v), want %d", tc.line, tc.from, got, err, tc.to)
		}
	}
}

func TestBackwardSexp(t *testing.T) {
	for _, tc := range []struct {
		line string
		from int
		to   int
	}{
		{"foo bar_baz", 11, 4},
		{"x (a (b) c)", 11, 2},
		{`f("(" + x)`, 10, 1},
		{`y := "a\"b"`, 11, 5},
		{"a + b", 4, 0},
	} {
		got, err := sexpAt(t, tc.line, tc.from, "backward-sexp")
		if err != nil || got != tc.to {
			t.Errorf("C-M-b in %q from %d: %d (%v), want %d", tc.line, tc.from, got, err, tc.to)
		}
	}
}

// Across lines, and the errors emacs gives at a group's edge or with no end.
func TestSexpEdges(t *testing.T) {
	f := fileEnv("a.go", "f(a,", "  b)", "next")
	allCommands(t).Run("forward-sexp", f)
	allCommands(t).Run("forward-sexp", f)
	if got := f.Point(); got != (text.Pos{Line: 1, Col: 4}) {
		t.Errorf("over a group spanning lines: %v", got)
	}

	if got, err := sexpAt(t, "(a b  )", 4, "forward-sexp"); err == nil || got != 4 {
		t.Errorf("C-M-f before a closing bracket: moved to %d, err %v; want an error and no move", got, err)
	}
	if got, err := sexpAt(t, "(  a b)", 3, "backward-sexp"); err == nil || got != 3 {
		t.Errorf("C-M-b after an opening bracket: moved to %d, err %v; want an error and no move", got, err)
	}
	for _, tc := range []struct {
		line, name string
		from       int
	}{
		{"  (a b", "forward-sexp", 0},
		{`  "open`, "forward-sexp", 0},
		{"a b)  ", "backward-sexp", 6},
		{`close"  `, "backward-sexp", 8},
	} {
		if got, err := sexpAt(t, tc.line, tc.from, tc.name); err == nil || got != tc.from {
			t.Errorf("%s in unbalanced %q: moved to %d, err %v; want an error and no move", tc.name, tc.line, got, err)
		}
	}
}

// C-M-k kills exactly what C-M-f would move over, onto the kill ring - from
// point, so blanks between point and the expression go too, as in emacs.
func TestKillSexp(t *testing.T) {
	f := fileEnv("a.go", "x = call(a, (b)) + 1")
	f.SetPoint(text.Pos{Col: 4})
	runAll(t, f, "kill-sexp")
	if got := f.Text(); got != "x = (a, (b)) + 1" {
		t.Errorf("after C-M-k: %q", got)
	}
	runAll(t, f, "kill-sexp")
	if got := f.Text(); got != "x =  + 1" {
		t.Errorf("after a second C-M-k: %q", got)
	}
}
