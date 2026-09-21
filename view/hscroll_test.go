package view

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
)

// bufOneLine returns a buffer holding a single line of s.
func bufOneLine(t *testing.T, s string) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	if err := b.Insert(text.Pos{}, []rune(s)); err != nil {
		t.Fatalf("seeding buffer: %v", err)
	}
	b.SetModified(false)
	return b
}

// winAtCol returns a window over s with point at rune index col.
func winAtCol(t *testing.T, s string, col text.RuneIdx) *Window {
	t.Helper()
	w := NewWindow(bufOneLine(t, s))
	w.Pt = text.Pos{Line: 0, Col: col}
	return w
}

func TestNewWindowStartsUnscrolledHorizontally(t *testing.T) {
	if got := NewWindow(bufLines(t, 3)).LeftCol; got != 0 {
		t.Errorf("LeftCol = %d on a new window, want 0", got)
	}
}

func TestShortLineNeedsNoHorizontalScroll(t *testing.T) {
	w := winAtCol(t, "short", 5)
	w.ScrollToPointHorizontally(40)
	if w.LeftCol != 0 {
		t.Errorf("LeftCol = %d for a line that fits, want 0", w.LeftCol)
	}
}

// Point past the right edge must bring itself into view, leaving room for the
// truncation marker that the overflow implies.
func TestScrollFollowsPointPastRightEdge(t *testing.T) {
	line := strings.Repeat("x", 100)
	w := winAtCol(t, line, 50)
	w.ScrollToPointHorizontally(20)

	ptCol := w.Buf.Line(0).DisplayCol(w.Pt.Col)
	if ptCol < w.LeftCol {
		t.Fatalf("point column %d is left of LeftCol %d", ptCol, w.LeftCol)
	}
	// 20 columns wide with a marker in the last leaves 19 for text.
	if got := ptCol - w.LeftCol; got > 18 {
		t.Errorf("point sits %d columns into a 20-wide pane, want within 18", got)
	}
}

// Scrolling back left is the case a one-directional implementation gets wrong.
func TestScrollReturnsLeftwardWhenPointMovesBack(t *testing.T) {
	line := strings.Repeat("x", 100)
	w := winAtCol(t, line, 90)
	w.ScrollToPointHorizontally(20)
	if w.LeftCol == 0 {
		t.Fatal("expected to be scrolled right before testing the return")
	}

	w.Pt.Col = 0
	w.ScrollToPointHorizontally(20)
	if w.LeftCol != 0 {
		t.Errorf("LeftCol = %d after point returned to column 0, want 0", w.LeftCol)
	}
}

func TestHorizontalScrollNeverGoesNegative(t *testing.T) {
	w := winAtCol(t, "abc", 0)
	w.LeftCol = 5 // as if the line had been longer before an edit
	w.ScrollToPointHorizontally(20)
	if w.LeftCol < 0 {
		t.Errorf("LeftCol = %d, want >= 0", w.LeftCol)
	}
}

// Scrolling is measured in display columns, so a tab or a wide glyph counts for
// what it occupies rather than for one rune.
func TestHorizontalScrollUsesDisplayColumns(t *testing.T) {
	// Ten CJK glyphs occupy twenty columns; point after the sixth is at column
	// twelve, which does not fit in a ten-column pane.
	w := winAtCol(t, strings.Repeat("日", 10), 6)
	w.ScrollToPointHorizontally(10)
	if w.LeftCol == 0 {
		t.Error("LeftCol = 0, but point at display column 12 cannot fit in 10 columns")
	}
	ptCol := w.Buf.Line(0).DisplayCol(w.Pt.Col)
	if ptCol < w.LeftCol {
		t.Errorf("point column %d is left of LeftCol %d", ptCol, w.LeftCol)
	}
}

func TestDegenerateWidthDoesNotLoopOrPanic(t *testing.T) {
	for _, width := range []int{-5, 0, 1} {
		w := winAtCol(t, strings.Repeat("x", 100), 50)
		w.ScrollToPointHorizontally(width)
		if w.LeftCol < 0 {
			t.Errorf("width %d: LeftCol = %d, want >= 0", width, w.LeftCol)
		}
	}
}

// Point must end up visible for every position along a line at every pane
// width - the horizontal counterpart of the vertical property test.
func TestScrollAlwaysLeavesPointVisibleHorizontally(t *testing.T) {
	line := "a\t世é\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466b" +
		strings.Repeat("z", 40)
	b := bufOneLine(t, line)
	n := b.Line(0).Len()

	for _, width := range []int{1, 2, 5, 13, 40, 200} {
		w := NewWindow(b)
		for col := text.RuneIdx(0); col <= n; col++ {
			w.Pt = text.Pos{Line: 0, Col: col}
			w.ScrollToPointHorizontally(width)

			ptCol := b.Line(0).DisplayCol(col)
			if w.LeftCol < 0 {
				t.Fatalf("width %d col %d: LeftCol = %d", width, col, w.LeftCol)
			}
			if ptCol < w.LeftCol {
				t.Fatalf("width %d col %d: point column %d left of LeftCol %d",
					width, col, ptCol, w.LeftCol)
			}
			if got := int(ptCol - w.LeftCol); got > width-1 {
				t.Fatalf("width %d col %d: point %d columns in, pane is %d wide",
					width, col, got, width)
			}
		}
	}
}

// Visiting another buffer resets horizontal scroll along with the rest of the
// viewport; otherwise a narrow buffer would open scrolled off its own text.
func TestVisitResetsHorizontalScroll(t *testing.T) {
	w := winAtCol(t, strings.Repeat("x", 100), 90)
	w.ScrollToPointHorizontally(20)
	if w.LeftCol == 0 {
		t.Fatal("expected to be scrolled right before visiting")
	}
	w.Visit(bufOneLine(t, "short"))
	if w.LeftCol != 0 {
		t.Errorf("LeftCol = %d after Visit, want 0", w.LeftCol)
	}
}
