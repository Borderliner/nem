package ui

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// reversedAt reports whether the cell at (x, y) carries the selection's inverted
// attribute.
func reversedAt(t *testing.T, scr tcell.SimulationScreen, x, y int) bool {
	t.Helper()
	_, _, attr := cellAt(t, scr, x, y).Style.Decompose()
	return attr&tcell.AttrReverse != 0
}

// selectionRow renders which cells of a row are selected, so a failure reads as
// a picture of the row rather than as a single disagreeing coordinate.
// '#' selected, '.' not.
func selectionRow(t *testing.T, scr tcell.SimulationScreen, y, width int) string {
	t.Helper()
	var b strings.Builder
	for x := 0; x < width; x++ {
		if reversedAt(t, scr, x, y) {
			b.WriteByte('#')
		} else {
			b.WriteByte('.')
		}
	}
	return b.String()
}

// marked returns a frame whose buffer has an active region from markPos to pt,
// as C-SPC and a motion would leave it.
func marked(t *testing.T, markPos, pt text.Pos, lines ...string) (Frame, *view.Window) {
	t.Helper()
	f, w := singleFrame(t, lines...)
	w.Buf.SetMark(markPos)
	w.Buf.ActivateMark()
	w.Pt = pt
	return f, w
}

func TestRegionHighlightsASingleLineSpan(t *testing.T) {
	// "alpha beta" - select "alpha" by marking column 0 and leaving point at 5.
	f, _ := marked(t, text.Pos{Line: 0, Col: 0}, text.Pos{Line: 0, Col: 5}, "alpha beta")
	scr := drawPlain(t, 12, 3, f)

	if got, want := selectionRow(t, scr, 0, 12), "#####......."; got != want {
		t.Errorf("row 0 selection\n got %q\nwant %q", got, want)
	}
}

// The user may set the mark after point as readily as before it. Ordering the
// endpoints is what makes both directions work, and a version that assumed mark
// follows point passes every test where it happens to.
func TestRegionWorksWithTheMarkAfterPoint(t *testing.T) {
	f, _ := marked(t, text.Pos{Line: 0, Col: 5}, text.Pos{Line: 0, Col: 0}, "alpha beta")
	scr := drawPlain(t, 12, 3, f)

	if got, want := selectionRow(t, scr, 0, 12), "#####......."; got != want {
		t.Errorf("row 0 selection\n got %q\nwant %q", got, want)
	}
}

func TestRegionSpansMultipleLines(t *testing.T) {
	// From line 0 col 2 to line 2 col 3.
	f, _ := marked(t, text.Pos{Line: 0, Col: 2}, text.Pos{Line: 2, Col: 3},
		"abcde", "fg", "hijkl")
	scr := drawPlain(t, 8, 5, f)

	for _, tc := range []struct {
		row  int
		want string
	}{
		// First line: from column 2, then filled to the right edge because the
		// selection continues.
		{0, "..######"},
		// Middle line: entirely selected, including past its two characters.
		{1, "########"},
		// Last line: up to column 3 only, and no fill past it.
		{2, "###....."},
		// Past the selection.
		{3, "........"},
	} {
		if got := selectionRow(t, scr, tc.row, 8); got != tc.want {
			t.Errorf("row %d selection\n got %q\nwant %q", tc.row, got, tc.want)
		}
	}
}

func TestRegionSpanningTheWholeBuffer(t *testing.T) {
	f, w := marked(t, text.Pos{}, text.Pos{}, "one", "two", "three")
	w.Buf.SetMark(text.Pos{})
	w.Buf.ActivateMark()
	w.Pt = w.Buf.End()
	scr := drawPlain(t, 7, 5, f)

	for _, tc := range []struct {
		row  int
		want string
	}{
		{0, "#######"},
		{1, "#######"},
		{2, "#####.."}, // last line ends at point, no fill past it
	} {
		if got := selectionRow(t, scr, tc.row, 7); got != tc.want {
			t.Errorf("row %d selection\n got %q\nwant %q", tc.row, got, tc.want)
		}
	}
}

// C-SPC followed by no movement must look like nothing happened, not like a
// one-cell selection.
func TestEmptyRegionHighlightsNothing(t *testing.T) {
	f, _ := marked(t, text.Pos{Line: 0, Col: 3}, text.Pos{Line: 0, Col: 3}, "alpha beta")
	scr := drawPlain(t, 12, 3, f)

	if got, want := selectionRow(t, scr, 0, 12), "............"; got != want {
		t.Errorf("row 0 selection\n got %q\nwant %q", got, want)
	}
}

func TestNoMarkHighlightsNothing(t *testing.T) {
	f, w := singleFrame(t, "alpha beta")
	w.Pt = text.Pos{Line: 0, Col: 5}
	if w.Buf.HasMark() {
		t.Fatal("fixture buffer already has a mark")
	}
	scr := drawPlain(t, 12, 3, f)

	if got, want := selectionRow(t, scr, 0, 12), "............"; got != want {
		t.Errorf("row 0 selection\n got %q\nwant %q", got, want)
	}
}

// The mark is per-buffer, but the selection belongs to the window you are in.
// Painting it into a second window onto the same buffer would claim a selection
// the user never made there.
func TestInactiveWindowShowsNoRegion(t *testing.T) {
	b := bufferOf(t, "alpha beta", "second line")
	b.SetMark(text.Pos{Line: 0, Col: 0})
	b.ActivateMark()

	w1 := view.NewWindow(b)
	w1.Pt = text.Pos{Line: 0, Col: 5}
	tree := view.NewTree(w1)
	w2, err := tree.Split(w1, true) // side by side
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	w2.Pt = text.Pos{Line: 0, Col: 5}

	scr := drawPlain(t, 24, 4, Frame{Tree: tree, Active: w1})

	rects := tree.Layout(24, 3)
	r1, r2 := rects[w1], rects[w2]

	if !reversedAt(t, scr, r1.X, r1.Y) {
		t.Errorf("active window's first cell is not selected; the region is missing where it belongs")
	}
	for x := r2.X; x < r2.X+r2.W; x++ {
		if reversedAt(t, scr, x, r2.Y) {
			t.Errorf("inactive window has a selected cell at x=%d; the region leaked across windows", x)
			break
		}
	}
}

// A wide glyph must not end up half selected.
//
// tcell stores the style on the glyph's base cell only: CellBuffer.Put sets
// currStyle on the base and merely marks the trailing cell dirty (cell.go:~100).
// The draw loop then advances past the continuation - tscreen.go:780 does
// `x += width - 1` on top of the loop's own x++ - so the trailing column is never
// emitted, and the terminal paints both columns from the base cell's single SGR.
//
// So the base cell carrying the region style IS both columns highlighted, and a
// simulation-screen dump showing the continuation cell unstyled is the model
// working, not a tear. Asserted on the base cells, with the continuation cells
// checked to be empty continuations rather than stray content.
func TestRegionHighlightsWideGlyphsWhole(t *testing.T) {
	f, w := marked(t, text.Pos{Line: 0, Col: 0}, text.Pos{Line: 0, Col: 3}, "日本語x")
	_ = w
	scr := drawPlain(t, 10, 3, f)

	for _, x := range []int{0, 2, 4} { // base cell of each CJK glyph
		if !reversedAt(t, scr, x, 0) {
			t.Errorf("base cell of the wide glyph at x=%d is not selected (row %q)",
				x, selectionRow(t, scr, 0, 10))
		}
	}
	for _, x := range []int{1, 3, 5} { // continuation cells
		if got := cellAt(t, scr, x, 0); len(got.Runes) != 0 {
			t.Errorf("x=%d holds %q, want an empty continuation cell", x, string(got.Runes))
		}
	}
	// "x" at column 6 is outside the region (point is at rune 3).
	if reversedAt(t, scr, 6, 0) {
		t.Error("the character past point is selected")
	}
}

func TestRegionHighlightsEveryColumnOfATab(t *testing.T) {
	// One tab then "ab". The tab expands to the next multiple of 8.
	f, _ := marked(t, text.Pos{Line: 0, Col: 0}, text.Pos{Line: 0, Col: 1}, "\tab")
	scr := drawPlain(t, 12, 3, f)

	if got, want := selectionRow(t, scr, 0, 12), "########...."; got != want {
		t.Errorf("a selected tab must invert every column it expands across\n got %q\nwant %q", got, want)
	}
}

// Point is always one end of the region and ScrollToPoint keeps point on screen,
// so a region can never be entirely off screen - only the mark end can be. This
// covers the mark end scrolled off the top.
//
// Expectations are derived from the window's actual Top and text height rather
// than hardcoded: the modeline owns a row and the scroll margin is clamped on a
// short window, so a fixed row list ends up asserting on the wrong rows.
func TestRegionContinuesOffTheTop(t *testing.T) {
	const scrW, scrH = 10, 8
	f, w := singleFrame(t, linesOf(40)...)
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Buf.ActivateMark()
	w.Pt = text.Pos{Line: 30, Col: 0}
	scr := drawPlain(t, scrW, scrH, f)

	if w.Top == 0 {
		t.Fatalf("fixture did not scroll: Top = 0")
	}
	textH := view.TextHeight(f.Tree.Layout(scrW, scrH-1)[w])
	if textH <= 1 {
		t.Fatalf("fixture window too short to test: textH = %d", textH)
	}

	sawSelected := false
	for row := 0; row < textH; row++ {
		line := w.Top + row
		got := selectionRow(t, scr, row, scrW)
		// A line above point is inside the region and continues onto the next,
		// so it is filled to the right edge. Point's own line and anything below
		// selects nothing, because point sits at column 0 and the region is a
		// half-open range.
		want := strings.Repeat(".", scrW)
		if line < w.Pt.Line {
			want = strings.Repeat("#", scrW)
			sawSelected = true
		}
		if got != want {
			t.Errorf("row %d (line %d)\n got %q\nwant %q", row, line, got, want)
		}
	}
	if !sawSelected {
		t.Fatal("fixture selected nothing; it cannot detect a dropped region")
	}
}

// And the mark end scrolled off the bottom: every visible line lies inside the
// region, so every one is filled to the right edge.
func TestRegionContinuesOffTheBottom(t *testing.T) {
	const scrW, scrH = 10, 8
	f, w := singleFrame(t, linesOf(40)...)
	w.Pt = text.Pos{Line: 0, Col: 0}
	w.Buf.SetMark(text.Pos{Line: 39, Col: 0})
	w.Buf.ActivateMark()
	scr := drawPlain(t, scrW, scrH, f)

	textH := view.TextHeight(f.Tree.Layout(scrW, scrH-1)[w])
	for row := 0; row < textH; row++ {
		if got, want := selectionRow(t, scr, row, scrW), strings.Repeat("#", scrW); got != want {
			t.Errorf("row %d (line %d)\n got %q\nwant %q", row, w.Top+row, got, want)
		}
	}
}

// The mark end can also be off screen horizontally. Everything from the left
// edge up to point is selected; point's own cell and anything past it is not,
// because the region is a half-open range - emacs does not highlight the cell at
// point either.
func TestRegionBeyondTheHorizontalScrollOffset(t *testing.T) {
	const scrW = 20
	long := strings.Repeat("abcdefghij", 8) // 80 columns
	f, w := singleFrame(t, long)
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Buf.ActivateMark()
	w.Pt = text.Pos{Line: 0, Col: 70}
	scr := drawPlain(t, scrW, 3, f)

	if w.LeftCol == 0 {
		t.Fatalf("fixture did not scroll horizontally: LeftCol = 0")
	}

	ptCol := int(text.ColIdx(w.Pt.Col) - w.LeftCol) // point's screen column
	got := selectionRow(t, scr, 0, scrW)
	want := strings.Repeat("#", ptCol) + strings.Repeat(".", scrW-ptCol)
	if got != want {
		t.Errorf("selection should run to point at screen column %d\n got %q\nwant %q", ptCol, got, want)
	}
	if !strings.Contains(got, "#") {
		t.Fatal("nothing selected; the mark end being off screen dropped the region entirely")
	}
}

// Reachable only as a unit: the integrated path cannot put both ends off screen,
// because point is always one of them. onLine must still be correct for it, so a
// future change to scrolling cannot turn it into a bug.
func TestRegionSpanOnLineWithBothEndsOffScreen(t *testing.T) {
	b := bufferOf(t, "one", "two", "three", "four", "five")
	span := regionSpan{
		lo:    text.Pos{Line: 0, Col: 1},
		hi:    text.Pos{Line: 4, Col: 2},
		on:    true,
		style: DefaultTheme().Region,
	}
	mid := span.onLine(2, b.Line(2))
	if !mid.on || mid.from != 0 || mid.to != b.Line(2).Len() || !mid.toEOL {
		t.Errorf("a line inside the span = %+v, want fully selected and continuing", mid)
	}
	if out := span.onLine(5, b.Line(4)); out.on {
		t.Errorf("a line past the span = %+v, want nothing selected", out)
	}
}

func TestRegionInADegenerateWindowDoesNotPanic(t *testing.T) {
	for _, size := range [][2]int{{1, 2}, {2, 2}, {1, 1}, {3, 2}} {
		f, _ := marked(t, text.Pos{Line: 0, Col: 0}, text.Pos{Line: 1, Col: 2}, "alpha", "beta")
		drawPlain(t, size[0], size[1], f) // must not panic
	}
}

// Where a bracket match and the selection fall on the same cell they compose:
// the region supplies the inverted background, the bracket keeps its weight and
// underline. Either one winning outright loses information the user needs.
func TestRegionAndBracketMatchCompose(t *testing.T) {
	f, w := singleFrame(t, "(ab)")
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Buf.ActivateMark()
	w.Pt = text.Pos{Line: 0, Col: 4} // point just after ')', so both brackets match
	scr := drawPlain(t, 8, 3, f)

	_, _, attr := cellAt(t, scr, 0, 0).Style.Decompose()
	if attr&tcell.AttrReverse == 0 {
		t.Error("the open bracket is inside the region but not inverted")
	}
	if attr&tcell.AttrBold == 0 || attr&tcell.AttrUnderline == 0 {
		t.Errorf("the bracket match lost its weight or underline inside the region (attr=%b)", attr)
	}
}

// The fill past a continuing line's text must start at the line's display width,
// not its rune count. With wide glyphs the two differ, and starting at the rune
// count overwrites the trailing cells of the glyphs at the end of the line -
// leaving them torn. A single-line region never exercises the fill at all, so
// this needs a region that continues onto a following line.
func TestRegionFillDoesNotEatWideGlyphsAtTheEndOfALine(t *testing.T) {
	// 日本語 occupies columns 0-5, x column 6: width 7, but only 4 runes.
	f, w := singleFrame(t, "日本語x", "second")
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Buf.ActivateMark()
	w.Pt = text.Pos{Line: 1, Col: 2}
	scr := drawPlain(t, 10, 4, f)

	for _, want := range []struct {
		x int
		r rune
	}{{0, '日'}, {2, '本'}, {4, '語'}, {6, 'x'}} {
		got := cellAt(t, scr, want.x, 0)
		if len(got.Runes) == 0 || got.Runes[0] != want.r {
			t.Errorf("x=%d holds %q, want %q - the selection fill overwrote the line's own text",
				want.x, string(got.Runes), string(want.r))
		}
		if !reversedAt(t, scr, want.x, 0) {
			t.Errorf("x=%d is not selected", want.x)
		}
	}
}

// A grapheme cannot be half selected. Point always sits on a cluster boundary,
// but an edit can leave the mark inside one, and the visible result of getting
// this wrong is a torn glyph. Tested on covers directly because the integrated
// path reaches it only through that narrow case.
func TestRegionCoversAClusterStraddlingItsBoundary(t *testing.T) {
	r := regionHL{on: true, from: 1, to: 5, style: DefaultTheme().Region}

	if !r.covers(0, 4) {
		t.Error("a cluster straddling the region start is not covered; the glyph would be torn")
	}
	if !r.covers(4, 8) {
		t.Error("a cluster straddling the region end is not covered")
	}
	if r.covers(5, 9) {
		t.Error("a cluster starting at the exclusive end is covered")
	}
	if r.covers(0, 1) {
		t.Error("a cluster entirely before the region is covered")
	}
}

// regionFor reports no selection at all when point is at the mark, rather than a
// zero-width one. Asserted on regionFor directly: downstream the two happen to
// draw identically, so a test on the rendered output cannot tell them apart and
// would let the distinction rot.
// A mark that is set but not active - the one yank or M-< leaves - is a
// bookmark, not a selection, so nothing is highlighted. Otherwise the reader
// would see text marked for replacement that is not.
func TestAnInactiveMarkHighlightsNothing(t *testing.T) {
	f, w := singleFrame(t, "alpha beta")
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Pt = text.Pos{Line: 0, Col: 5}

	if got := regionFor(w.Buf, w.Pt, true, DefaultTheme()); got.on {
		t.Errorf("regionFor with an inactive mark = %+v, want no selection", got)
	}
	scr := drawPlain(t, 12, 3, f)
	for x := 0; x < 12; x++ {
		if reversedAt(t, scr, x, 0) {
			t.Errorf("cell x=%d is highlighted though the region is inactive", x)
		}
	}
}

func TestRegionForReportsNothingWhenPointIsAtTheMark(t *testing.T) {
	b := bufferOf(t, "alpha")
	b.SetMark(text.Pos{Line: 0, Col: 2})
	b.ActivateMark()

	if got := regionFor(b, text.Pos{Line: 0, Col: 2}, true, DefaultTheme()); got.on {
		t.Errorf("regionFor with point at the mark = %+v, want no selection", got)
	}
	if got := regionFor(b, text.Pos{Line: 0, Col: 3}, true, DefaultTheme()); !got.on {
		t.Error("regionFor with point one past the mark reports no selection")
	}
}
