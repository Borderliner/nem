package ui

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// wrapTheme is the default theme with wrapping on and the band off, so a
// row reads as its text.
func wrapTheme() Theme {
	th := DefaultTheme()
	th.Wrap, th.HighlightLine = true, false
	return th
}

// A line wider than the window folds into rows, broken after spaces, its
// number beside its first row alone; the lines after it follow.
func TestAWrappedLineFoldsIntoRows(t *testing.T) {
	f, _ := singleFrame(t, "one two three four five", "next")
	scr := drawTh(t, 14, 6, f, wrapTheme()) // a gutter of 2, a text area of 12
	for y, want := range []string{"1 one two", "  three four", "  five", "2 next"} {
		if got := strings.TrimRight(rowText(t, scr, y), " "); got != want {
			t.Errorf("row %d = %q, want %q", y, got, want)
		}
	}
	if strings.Contains(rowText(t, scr, 0), string(TruncMarker)) {
		t.Error("a wrapped line is marked as cut off")
	}
}

// Unwrapped, the same line is cut off at the edge as it always was.
func TestUnwrappedLinesAreCutOffAsBefore(t *testing.T) {
	f, _ := singleFrame(t, "one two three four five", "next")
	th := wrapTheme()
	th.Wrap = false
	scr := drawTh(t, 14, 6, f, th)
	if got := rowText(t, scr, 1); !strings.HasPrefix(got, "2 next") {
		t.Errorf("row 1 = %q, want the second line", got)
	}
	if got := rowText(t, scr, 0); !strings.HasSuffix(got, string(TruncMarker)) {
		t.Errorf("row 0 = %q, want it cut off with the marker", got)
	}
}

// The cursor goes on the row that holds point, at its column in that row.
func TestTheCursorIsOnPointsRow(t *testing.T) {
	f, w := singleFrame(t, "one two three four five")
	w.Pt = text.Pos{Col: 14} // the f of four
	scr := drawTh(t, 14, 6, f, wrapTheme())
	if x, y, _ := scr.GetCursor(); x != 2+6 || y != 1 {
		t.Errorf("cursor at (%d,%d), want (8,1): row 2, after 'three '", x, y)
	}
	w.Pt = text.Pos{Col: 23} // the end of the line, on its last row
	scr = drawTh(t, 14, 6, f, wrapTheme())
	if x, y, _ := scr.GetCursor(); x != 2+4 || y != 2 {
		t.Errorf("cursor at the end at (%d,%d), want (6,2)", x, y)
	}
}

// A line taller than the window scrolls a row at a time: with point on its
// last row, the window starts part way down it, and the rows above point's
// have no number beside them.
func TestALineTallerThanTheWindowScrollsByRows(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 20)) // 99 columns: ten rows at 10
	f, w := singleFrame(t, long)
	w.Pt = text.Pos{Col: 97}
	th := wrapTheme()
	th.LineNumbers = false
	scr := drawTh(t, 10, 5, f, th) // three text rows, a mode line and the echo row
	if w.Top != 0 || w.TopRow == 0 {
		t.Fatalf("top %d row %d; want part way down line 1", w.Top, w.TopRow)
	}
	if _, y, _ := scr.GetCursor(); y != 2 {
		t.Errorf("cursor on row %d, want the last text row", y)
	}
	if got := strings.TrimRight(rowText(t, scr, 2), " "); got != "word word" {
		t.Errorf("last text row = %q", got)
	}
}

// The band under the current line runs under all its rows, and beside them
// in the gutter.
func TestTheBandCoversEveryRowOfTheLine(t *testing.T) {
	f, _ := singleFrame(t, "one two three four five", "next")
	th := wrapTheme()
	th.HighlightLine = true
	scr := drawTh(t, 14, 6, f, th)
	_, band, _ := th.CurrentLine.Decompose()
	for y := 0; y < 3; y++ {
		for _, x := range []int{0, 13} {
			if _, bg, _ := cellAt(t, scr, x, y).Style.Decompose(); bg != band {
				t.Errorf("cell (%d,%d) background %v, want the band", x, y, bg)
			}
		}
	}
	if _, bg, _ := cellAt(t, scr, 13, 3).Style.Decompose(); bg == band {
		t.Error("the band runs on under the next line")
	}
}

// A selection running over a row break fills the rest of the row it leaves,
// so it reads as one block.
func TestASelectionFillsTheRowItRunsOn(t *testing.T) {
	f, w := singleFrame(t, "one two three four five")
	w.Buf.SetMark(text.Pos{Col: 4})
	w.Buf.ActivateMark()
	w.Pt = text.Pos{Col: 18}
	th := wrapTheme()
	th.LineNumbers = false
	scr := drawTh(t, 12, 5, f, th)
	_, _, attr := cellAt(t, scr, 11, 0).Style.Decompose()
	if attr&tcell.AttrReverse == 0 {
		t.Error("the row the selection runs on from is not filled to its edge")
	}
	_, _, attr = cellAt(t, scr, 11, 1).Style.Decompose()
	if attr&tcell.AttrReverse != 0 {
		t.Error("the row the selection ends on is filled past its end")
	}
}

// A paragraph of Persian folds into rows, each read right to left against
// the right edge, the paragraph's first words on the first row.
func TestRightToLeftTextWrapsRowByRow(t *testing.T) {
	f, th := bidiFrame(t, true, text.Pos{}, "سلام دنیا خوب")
	th.Wrap = true
	scr := drawTh(t, 10, 4, f, th)
	if got := strings.TrimLeft(rowText(t, scr, 0), " "); got != visual("سلام دنیا ") && got != visual("سلام دنیا") {
		t.Errorf("row 0 = %q, want %q against the right edge", got, visual("سلام دنیا"))
	}
	if got := strings.TrimSpace(rowText(t, scr, 1)); got != visual("خوب") {
		t.Errorf("row 1 = %q, want %q", got, visual("خوب"))
	}
	if got := rowText(t, scr, 1); !strings.HasSuffix(got, visual("خوب")) {
		t.Errorf("row 1 = %q, want it against the right edge", got)
	}
}

// A row of right-to-left text whose space hangs past the edge still sits
// against the right edge, its first letter in the last column: the hanging
// space is not laid out, where it pushed the row a column too wide and the
// letter off the edge.
func TestARightToLeftRowWithAHangingSpaceKeepsItsFirstLetter(t *testing.T) {
	// At 9 columns: "سلام دنیا " is ten with its space, which hangs.
	f, th := bidiFrame(t, true, text.Pos{}, "سلام دنیا خوب")
	th.Wrap = true
	scr := drawTh(t, 9, 4, f, th)
	if got := strings.TrimLeft(rowText(t, scr, 0), " "); got != visual("سلام دنیا") {
		t.Errorf("row 0 = %q, want %q whole, against the right edge", rowText(t, scr, 0), visual("سلام دنیا"))
	}
}
