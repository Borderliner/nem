package command_test

import (
	"errors"
	"testing"

	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

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
