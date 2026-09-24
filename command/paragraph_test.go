package command_test

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
)

func TestParagraphMotion(t *testing.T) {
	f := fileEnv("a.txt", "one", "two", "", "", "three", "four", "", "five")
	f.SetPoint(text.Pos{Line: 0, Col: 1})

	runAll(t, f, "forward-paragraph")
	if got := f.Point(); got != (text.Pos{Line: 2}) {
		t.Errorf("M-} to %v, want the blank line after one/two", got)
	}
	runAll(t, f, "forward-paragraph")
	if got := f.Point(); got != (text.Pos{Line: 6}) {
		t.Errorf("second M-} to %v, want the blank after three/four", got)
	}
	runAll(t, f, "forward-paragraph")
	if got := f.Point(); got != (text.Pos{Line: 7, Col: 4}) {
		t.Errorf("M-} at the last paragraph to %v, want the end of the buffer", got)
	}
	runAll(t, f, "backward-paragraph")
	if got := f.Point(); got != (text.Pos{Line: 6}) {
		t.Errorf("M-{ to %v, want the blank before five", got)
	}
	runAll(t, f, "backward-paragraph")
	if got := f.Point(); got != (text.Pos{Line: 3}) {
		t.Errorf("M-{ to %v, want the blank before three", got)
	}
	f.ArgN, f.ArgExplicit = 5, true
	runAll(t, f, "backward-paragraph")
	if got := f.Point(); got != (text.Pos{}) {
		t.Errorf("M-{ past the first paragraph to %v, want the start", got)
	}
}

// withFill runs fn with FillColumn set to n.
func withFill(t *testing.T, n int) {
	saved := command.FillColumn
	command.FillColumn = n
	t.Cleanup(func() { command.FillColumn = saved })
}

func TestFillParagraph(t *testing.T) {
	withFill(t, 20)
	f := fileEnv("a.txt", "the quick brown fox jumps over", "the lazy dog", "", "next")
	f.SetPoint(text.Pos{Line: 1, Col: 4}) // on "lazy"

	runAll(t, f, "fill-paragraph")
	want := "the quick brown fox\njumps over the lazy\ndog\n\nnext"
	if got := f.Text(); got != want {
		t.Fatalf("filled:\n%s\nwant:\n%s", got, want)
	}
	if got := f.Point(); got != (text.Pos{Line: 1, Col: 15}) {
		t.Errorf("point %v, want still on lazy at {1 15}", got)
	}
}

// A comment block is filled as comments: the prefix stays on every line, and
// the code around it is not part of the paragraph. The tab counts as the
// columns it covers, so "\t// " is eleven wide.
func TestFillComments(t *testing.T) {
	withFill(t, 36)
	f := fileEnv("a.go", "func f() {", "\t// This comment is far too long for the width it", "\t// has, and", "\t// wraps badly.", "\tx := 1", "}")
	f.SetPoint(text.Pos{Line: 2, Col: 5})

	runAll(t, f, "fill-paragraph")
	want := "func f() {\n\t// This comment is far too\n\t// long for the width it\n\t// has, and wraps badly.\n\tx := 1\n}"
	if got := f.Text(); got != want {
		t.Errorf("filled:\n%s\nwant:\n%s", got, want)
	}
}

// Filling what is already filled changes nothing and leaves nothing to undo.
func TestFillIsIdempotent(t *testing.T) {
	withFill(t, 20)
	f := fileEnv("a.txt", "one two three")
	rev := f.Buf().Revision()
	runAll(t, f, "fill-paragraph")
	if got := f.Text(); got != "one two three" {
		t.Errorf("text %q", got)
	}
	if f.Buf().Revision() != rev {
		t.Error("an unchanged fill edited the buffer anyway")
	}
}

// A word wider than the fill column gets a line of its own, whole.
func TestFillKeepsLongWords(t *testing.T) {
	withFill(t, 10)
	url := "https://example.com/" + strings.Repeat("x", 20)
	f := fileEnv("a.txt", "see "+url+" now")
	runAll(t, f, "fill-paragraph")
	if got := f.Text(); got != "see\n"+url+"\nnow" {
		t.Errorf("filled %q", got)
	}
}
