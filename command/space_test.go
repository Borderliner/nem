package command_test

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

func TestJustOneSpaceAndDeleteHorizontalSpace(t *testing.T) {
	f := fileEnv("a.txt", "foo  \t  bar")
	f.SetPoint(text.Pos{Line: 0, Col: 5})
	runAll(t, f, "just-one-space")
	if got := f.Text(); got != "foo bar" {
		t.Errorf("just-one-space: %q", got)
	}
	if got := f.Point(); got.Col != 4 {
		t.Errorf("point %v, want after the space", got)
	}

	f = fileEnv("a.txt", "foo   bar")
	f.SetPoint(text.Pos{Line: 0, Col: 4})
	runAll(t, f, "delete-horizontal-space")
	if got := f.Text(); got != "foobar" {
		t.Errorf("delete-horizontal-space: %q", got)
	}
}

// M-^ joins onto the previous line with one space, dropping the indentation,
// and no space inside brackets or at the start of a line.
func TestDeleteIndentation(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"foo  \n    bar", "foo bar"},
		{"call(\n\targ", "call(arg"},
		{"x = [1, 2\n  ]", "x = [1, 2]"},
		{"\n   bar", "bar"},
	} {
		f := fileEnv("a.go", tc.in)
		f.SetPoint(text.Pos{Line: 1, Col: 1})
		runAll(t, f, "delete-indentation")
		if got := f.Text(); got != tc.want {
			t.Errorf("%q joined to %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBackToIndentation(t *testing.T) {
	f := fileEnv("a.go", "\t  x := 1")
	f.SetPoint(text.Pos{Line: 0, Col: 8})
	runAll(t, f, "back-to-indentation")
	if got := f.Point(); got.Col != 3 {
		t.Errorf("point %v, want at the x", got)
	}
}

// M-z kills through the character, onto the kill ring; with an argument, the
// nth one; backwards with a negative one; and says so when there is none.
func TestZapToChar(t *testing.T) {
	f := fileEnv("a.txt", "one, two, three")
	f.Chars = []rune{','}
	runAll(t, f, "zap-to-char")
	if got := f.Text(); got != " two, three" {
		t.Errorf("zap: %q", got)
	}
	if got, _ := f.Ring().Yank(); got != "one," {
		t.Errorf("killed %q", got)
	}

	f = fileEnv("a.txt", "a.b.c.d")
	f.Chars = []rune{'.'}
	f.ArgN, f.ArgExplicit = 2, true
	runAll(t, f, "zap-to-char")
	if got := f.Text(); got != "c.d" {
		t.Errorf("zap 2: %q", got)
	}

	f = fileEnv("a.txt", "a.b.c")
	f.SetPoint(text.Pos{Line: 0, Col: 5})
	f.Chars = []rune{'.'}
	f.ArgN, f.ArgExplicit = -1, true
	runAll(t, f, "zap-to-char")
	if got := f.Text(); got != "a.b" {
		t.Errorf("zap -1: %q", got)
	}

	f = fileEnv("a.txt", "abc")
	f.Chars = []rune{'z'}
	r := allCommands(t)
	if err := r.Run("zap-to-char", f); err == nil {
		t.Error("zapping to a missing character succeeded")
	}
	if got := f.Text(); got != "abc" {
		t.Errorf("a failed zap changed the text to %q", got)
	}
}
