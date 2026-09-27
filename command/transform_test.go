package command_test

import (
	"errors"
	"testing"

	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

// --- sort-lines and reverse-region --------------------------------------------

func TestSortLines(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lines   []string
		reverse bool
		want    string
	}{
		{"plain", []string{"cherry", "apple", "banana"}, false, "apple\nbanana\ncherry"},
		{"reversed with an argument", []string{"cherry", "apple", "banana"}, true, "cherry\nbanana\napple"},
		// By bytes, as emacs sorts: capitals first, and no locale.
		{"by bytes", []string{"b", "a", "B", "A", "é", "e"}, false, "A\nB\na\nb\ne\né"},
		{"empty lines first", []string{"b", "", "a"}, false, "\na\nb"},
		{"duplicates kept", []string{"b", "a", "b", "a"}, false, "a\na\nb\nb"},
		{"indentation counts", []string{"b", "  z", "a"}, false, "  z\na\nb"},
	} {
		f := commandtest.New(tc.lines...)
		activate(f, text.Pos{}, f.Buf().End())
		if tc.reverse {
			edArg(f, 4)
		}
		if err := tryRun(t, f, "sort-lines"); err != nil {
			t.Fatal(err)
		}
		if got := f.Text(); got != tc.want {
			t.Errorf("%s: sorted %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Whole lines, however much of the first and last the region covers, and
// not the line it ends at the start of. The region's ends stay on their
// lines: at the start of one, where they were; inside one that was rewritten,
// at its end.
func TestSortLinesTakesTheRegionsWholeLines(t *testing.T) {
	f := commandtest.New("top", "delta", "alpha", "charlie", "bravo", "zulu")
	activate(f, text.Pos{Line: 1, Col: 3}, text.Pos{Line: 5})
	if err := tryRun(t, f, "sort-lines"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "top\nalpha\nbravo\ncharlie\ndelta\nzulu")
	pointIs(t, f, text.Pos{Line: 5})
	markIs(t, f, text.Pos{Line: 1, Col: 5})
}

func TestSortLinesIsOneUndo(t *testing.T) {
	f := commandtest.New("c", "b", "a")
	activate(f, text.Pos{}, f.Buf().End())
	if err := tryRun(t, f, "sort-lines"); err != nil {
		t.Fatal(err)
	}
	f.Buf().BreakUndo()
	f.Buf().Undo()
	textIs(t, f, "c\nb\na")
}

func TestReverseRegion(t *testing.T) {
	f := commandtest.New("1", "2", "3", "4")
	activate(f, text.Pos{}, text.Pos{Line: 3})
	if err := tryRun(t, f, "reverse-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "3\n2\n1\n4")
}

func TestReorderingLinesWantsARegion(t *testing.T) {
	for _, name := range []string{"sort-lines", "reverse-region"} {
		f := commandtest.New("b", "a")
		if err := tryRun(t, f, name); err != nil {
			t.Fatal(err)
		}
		textIs(t, f, "b\na")
		if got := lastEcho(t, f.Echoes); got != "The mark is not set now, so there is no region" {
			t.Errorf("%s: echo %q", name, got)
		}
	}
}

func TestReorderingLinesInAReadOnlyBuffer(t *testing.T) {
	f := commandtest.New("b", "a")
	f.Buf().SetReadOnly(true)
	activate(f, text.Pos{}, f.Buf().End())
	if err := tryRun(t, f, "sort-lines"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	textIs(t, f, "b\na")
}

// --- upcase-region and downcase-region ----------------------------------------

func TestUpcaseRegion(t *testing.T) {
	f := commandtest.New("hello world", "and more", "end")
	activate(f, text.Pos{Line: 0, Col: 6}, text.Pos{Line: 2, Col: 1})
	if err := tryRun(t, f, "upcase-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "hello WORLD\nAND MORE\nEnd")
	markIs(t, f, text.Pos{Line: 0, Col: 6})
	pointIs(t, f, text.Pos{Line: 2, Col: 1})
}

// Point before the mark is the same region.
func TestDowncaseRegionBackwards(t *testing.T) {
	f := commandtest.New("ONE TWO THREE")
	activate(f, text.Pos{Col: 7}, text.Pos{Col: 4})
	if err := tryRun(t, f, "downcase-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "ONE two THREE")
	markIs(t, f, text.Pos{Col: 7})
	pointIs(t, f, text.Pos{Col: 4})
}

func TestCaseRegionIsOneUndo(t *testing.T) {
	f := commandtest.New("ab", "cd")
	activate(f, text.Pos{}, text.Pos{Line: 1, Col: 2})
	if err := tryRun(t, f, "upcase-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "AB\nCD")
	f.Buf().BreakUndo()
	f.Buf().Undo()
	textIs(t, f, "ab\ncd")
}

// With no region it says so, as emacs does, and changes nothing - whether
// there is no mark at all or only one a yank left behind.
func TestCaseRegionWantsARegion(t *testing.T) {
	f := commandtest.New("abc")
	if err := tryRun(t, f, "upcase-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "abc")
	if got := lastEcho(t, f.Echoes); got != "The mark is not set now, so there is no region" {
		t.Errorf("echo %q", got)
	}

	f.Buf().SetMark(text.Pos{Col: 3}) // set, not active
	if err := tryRun(t, f, "upcase-region"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "abc")
	if got := lastEcho(t, f.Echoes); got != "The mark is not active now" {
		t.Errorf("echo %q", got)
	}
}

func TestCaseRegionInAReadOnlyBuffer(t *testing.T) {
	f := commandtest.New("abc")
	f.Buf().SetReadOnly(true)
	activate(f, text.Pos{}, text.Pos{Col: 3})
	if err := tryRun(t, f, "upcase-region"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	textIs(t, f, "abc")

	// Already in the case asked for, there is nothing to refuse.
	f = commandtest.New("ABC")
	f.Buf().SetReadOnly(true)
	activate(f, text.Pos{}, text.Pos{Col: 3})
	if err := tryRun(t, f, "upcase-region"); err != nil {
		t.Errorf("err = %v, want none", err)
	}
}
