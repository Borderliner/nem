package command_test

import (
	"errors"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
)

// With lines wrapped, C-n and C-p go a row on screen at a time, holding the
// column within the row, as emacs's do.
func TestLinesWrappedMoveByRows(t *testing.T) {
	f := newMotionFake(t, "top", "abcd abcd efgh efgh ijkl", "bottom")
	f.Wrap = 10
	f.SetPoint(text.Pos{Line: 0, Col: 2})
	for _, want := range []text.Pos{
		{Line: 1, Col: 2},  // the long line's first row
		{Line: 1, Col: 12}, // its second, the same column in the row
		{Line: 1, Col: 22}, // its third
		{Line: 2, Col: 2},  // the next line
	} {
		run(t, f, "next-line")
		wantPoint(t, f, want.Line, want.Col)
	}
	run(t, f, "previous-line")
	wantPoint(t, f, 1, 22)
	run(t, f, "previous-line")
	wantPoint(t, f, 1, 12)
}

// A row shorter than the goal column takes point to its last character,
// not on to the next row; the goal comes back on a row long enough.
func TestAShortRowKeepsPointOnIt(t *testing.T) {
	f := newMotionFake(t, "abcd abcd efgh efgh ijkl", "0123456789")
	f.Wrap = 10
	f.SetPoint(text.Pos{Line: 0, Col: 18}) // the second row, column 8
	run(t, f, "next-line")
	wantPoint(t, f, 0, 24) // "ijkl" is the line's last row: its end
	run(t, f, "next-line")
	wantPoint(t, f, 1, 8) // the goal column again
}

// At the last row C-n says so, as it does at the last line unwrapped.
func TestTheLastRowIsTheEnd(t *testing.T) {
	f := newMotionFake(t, "abcd abcd efgh")
	f.Wrap = 10
	f.SetPoint(text.Pos{Col: 11})
	if err := f.Run("next-line"); !errors.Is(err, command.ErrEndOfBuffer) {
		t.Errorf("C-n on the last row: %v, want the end of the buffer", err)
	}
}

// C-v goes a screenful of rows, and C-l centres point's row.
func TestScrollingAndRecentringCountRows(t *testing.T) {
	f := newMotionFake(t, "abcd abcd efgh efgh ijkl mnop qrst uvwx yz", "end")
	f.Wrap = 10
	f.Height = 4 // a screenful less two rows of overlap: two rows
	f.SetPoint(text.Pos{Col: 1})
	run(t, f, "scroll-up-command")
	wantPoint(t, f, 0, 21) // two rows down, the same column
	if w := f.Win(); w.Top != 0 || w.TopRow != 2 {
		t.Errorf("after C-v the window starts at line %d row %d, want line 0 row 2", w.Top, w.TopRow)
	}
	run(t, f, "recenter-top-bottom")
	if w := f.Win(); w.Top != 0 || w.TopRow != 1 {
		t.Errorf("after C-l the window starts at line %d row %d, want row 1: point's row, with one above", w.Top, w.TopRow)
	}
}
