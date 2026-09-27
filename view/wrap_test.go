package view

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
)

// wrapped is a buffer of lines, for wrapping at width 10: "short", then a
// line of 45 letters in words of 4 - five rows - then "end".
func wrapped(t *testing.T) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	long := strings.TrimSpace(strings.Repeat("abcd ", 9)) // 44 columns
	if err := b.Insert(text.Pos{}, []rune("short\n"+long+"\nend")); err != nil {
		t.Fatal(err)
	}
	return b
}

// Stepping goes row by row through a wrapped line and on to the next, and
// stops at the ends, saying how far it got.
func TestStepRows(t *testing.T) {
	b := wrapped(t)
	if n := rows(b, 1, 10); n != 5 {
		t.Fatalf("the long line is %d rows at width 10, want 5", n)
	}
	for _, tc := range []struct {
		from     RowPos
		n        int
		to       RowPos
		howFar   int
		scenario string
	}{
		{RowPos{0, 0}, 1, RowPos{1, 0}, 1, "down onto the long line"},
		{RowPos{0, 0}, 3, RowPos{1, 2}, 3, "down into it"},
		{RowPos{1, 4}, 1, RowPos{2, 0}, 1, "off its last row"},
		{RowPos{2, 0}, -1, RowPos{1, 4}, 1, "up onto its last row"},
		{RowPos{2, 0}, 10, RowPos{2, 0}, 0, "past the end"},
		{RowPos{1, 1}, -10, RowPos{0, 0}, 2, "past the start"},
	} {
		to, n := StepRows(b, tc.from, tc.n, 10)
		if to != tc.to || n != tc.howFar {
			t.Errorf("%s: %v by %d went to %v, %d rows; want %v, %d", tc.scenario, tc.from, tc.n, to, n, tc.to, tc.howFar)
		}
	}
}

// A column in a row lands in that row: a row shorter than the column puts
// point on its last character, not at the start of the next row.
func TestPosInRow(t *testing.T) {
	b := wrapped(t)
	// Rows of line 1 at width 10: "abcd abcd " x4, then "abcd".
	for _, tc := range []struct {
		r    RowPos
		col  text.ColIdx
		want text.RuneIdx
	}{
		{RowPos{1, 0}, 0, 0},
		{RowPos{1, 1}, 3, 13},
		{RowPos{1, 1}, 40, 19}, // the row's last character, not 20
		{RowPos{1, 4}, 40, 44}, // the last row reaches the line's end
	} {
		if got := PosInRow(b, tc.r, 10, tc.col); got != (text.Pos{Line: 1, Col: tc.want}) {
			t.Errorf("row %v col %d: %v, want col %d", tc.r, tc.col, got, tc.want)
		}
		if tc.want < 44 {
			if got := RowOf(b, text.Pos{Line: 1, Col: tc.want}, 10); got != tc.r {
				t.Errorf("RowOf col %d = %v, want %v", tc.want, got, tc.r)
			}
		}
	}
}

// Scrolled by rows, a wrapped window keeps point's row in view, even inside
// a line taller than the window, and stops at the last screenful.
func TestScrollToPointWrapped(t *testing.T) {
	b := wrapped(t)
	w := NewWindow(b)
	for _, tc := range []struct {
		pt  text.Pos
		top RowPos
	}{
		{text.Pos{Line: 0}, RowPos{0, 0}},
		{text.Pos{Line: 1, Col: 25}, RowPos{1, 0}}, // row 2 of line 1: the window's last
		{text.Pos{Line: 1, Col: 35}, RowPos{1, 1}}, // row 3: one row on
		{text.Pos{Line: 2, Col: 2}, RowPos{1, 3}},  // the last screenful
		{text.Pos{Line: 0}, RowPos{0, 0}},          // back to the top
	} {
		w.Pt = tc.pt
		w.ScrollToPointWrapped(3, 0, 10)
		if got := (RowPos{w.Top, w.TopRow}); got != tc.top {
			t.Errorf("point at %v: top %v, want %v", tc.pt, got, tc.top)
		}
	}

	// With a margin, point keeps a row of room above and below.
	w.Pt = text.Pos{Line: 1, Col: 15}
	w.Top, w.TopRow = 0, 0
	w.ScrollToPointWrapped(3, 1, 10)
	if got := (RowPos{w.Top, w.TopRow}); got != (RowPos{1, 0}) {
		t.Errorf("with a margin: top %v, want {1 0}, point's row in the middle", got)
	}
}
