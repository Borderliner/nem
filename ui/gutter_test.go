package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// gutterTheme returns the default theme with the gutter forced on or off, so a
// test states which layout it is asserting about rather than depending on what
// the default happens to be.
func gutterTheme(on bool) Theme {
	th := DefaultTheme()
	th.LineNumbers = on
	return th
}

// drawTh is draw with an explicit theme.
func drawTh(t *testing.T, w, h int, f Frame, th Theme) tcell.SimulationScreen {
	t.Helper()
	scr := sim(t, w, h)
	Render(scr, f, th)
	scr.Show()
	return scr
}

func TestGutterWidthGrowsWithTheBuffer(t *testing.T) {
	// The width comes from the buffer's line count, not from what is on screen:
	// scrolling a 200-line buffer to line 150 must not make the numbers shift.
	for _, tc := range []struct{ lines, want int }{
		{1, 2}, {9, 2}, {10, 3}, {99, 3}, {100, 4}, {999, 4}, {1000, 5}, {10000, 6},
	} {
		w := view.NewWindow(bufferOf(t, linesOf(tc.lines)...))
		if got := gutterWidth(w, gutterTheme(true)); got != tc.want {
			t.Errorf("%d lines: gutterWidth = %d, want %d", tc.lines, got, tc.want)
		}
	}
}

func TestGutterWidthIsZeroWhenDisabled(t *testing.T) {
	w := view.NewWindow(bufferOf(t, linesOf(500)...))
	if got := gutterWidth(w, gutterTheme(false)); got != 0 {
		t.Errorf("gutterWidth with numbers off = %d, want 0", got)
	}
}

// Scrolling past line 99 must not widen the gutter. Computing the width from the
// visible rows instead of the buffer would do exactly that, and the numbers would
// jump sideways as you scrolled.
func TestGutterWidthDoesNotDependOnScrollPosition(t *testing.T) {
	w := view.NewWindow(bufferOf(t, linesOf(200)...))
	at0 := gutterWidth(w, gutterTheme(true))
	w.Top = 150
	if at150 := gutterWidth(w, gutterTheme(true)); at150 != at0 {
		t.Errorf("gutterWidth = %d at Top=0 but %d at Top=150; the numbers would shift while scrolling", at0, at150)
	}
}

func TestGutterNumbersAreRightAlignedAndTextFollows(t *testing.T) {
	f, _ := singleFrame(t, linesOf(12)...)
	// 14 rows: one echo row, one modeline, leaving 12 text rows - enough to show
	// line 10 and prove a two-digit number needs no leading blank.
	scr := drawTh(t, 40, 14, f, gutterTheme(true))

	// 12 lines -> 2 digits + 1 separator = 3 columns.
	for i, want := range []string{" 1 line0", " 2 line1", " 3 line2"} {
		if got := rowText(t, scr, i); !strings.HasPrefix(got, want) {
			t.Errorf("row %d = %q, want prefix %q", i, got, want)
		}
	}
	// Row 9 is line index 9, numbered 10: two digits, so no leading blank.
	if got := rowText(t, scr, 9); !strings.HasPrefix(got, "10 line9") {
		t.Errorf("row 9 = %q, want prefix %q", got, "10 line9")
	}
}

func TestGutterLeavesRowsPastTheBufferBlank(t *testing.T) {
	f, _ := singleFrame(t, "only")
	scr := drawTh(t, 30, 8, f, gutterTheme(true))
	for y := 1; y < 6; y++ {
		if got := strings.TrimSpace(rowText(t, scr, y)); got != "" {
			t.Errorf("row %d = %q, want blank past the end of the buffer", y, got)
		}
	}
}

// THE requirement: a selection must never reach the gutter. Line numbers are not
// buffer content, so C-w and M-w cannot copy them - but they must not even look
// selected, or the user cannot tell what the kill will take.
func TestRegionNeverStylesTheGutter(t *testing.T) {
	f, w := singleFrame(t, "alpha", "beta", "gamma")
	w.Buf.SetMark(text.Pos{Line: 0, Col: 0})
	w.Pt = text.Pos{Line: 2, Col: 5} // whole buffer selected
	th := gutterTheme(true)
	scr := drawTh(t, 30, 8, f, th)

	gw := gutterWidth(w, th)
	if gw == 0 {
		t.Fatal("no gutter drawn; this test asserts nothing")
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < gw; x++ {
			c := cellAt(t, scr, x, y)
			if c.Style == th.Region {
				t.Errorf("gutter cell (%d,%d) carries the region style; line numbers look selected", x, y)
			}
		}
		// And the text beside it genuinely is selected, so the test is not
		// passing because nothing was highlighted at all.
		if c := cellAt(t, scr, gw, y); c.Style != th.Region {
			t.Errorf("text cell (%d,%d) is not selected; the fixture is wrong", gw, y)
		}
	}
}

func TestBracketMatchNeverStylesTheGutter(t *testing.T) {
	f, w := singleFrame(t, "(abc)")
	w.Pt = text.Pos{Line: 0, Col: 0} // on the open bracket
	th := gutterTheme(true)
	scr := drawTh(t, 30, 6, f, th)

	gw := gutterWidth(w, th)
	for x := 0; x < gw; x++ {
		c := cellAt(t, scr, x, 0)
		if c.Style == th.ParenMatch || c.Style == th.ParenMismatch {
			t.Errorf("gutter cell (%d,0) carries a bracket style", x)
		}
	}
	// The brackets themselves must be marked, or the fixture proves nothing.
	if got := cellAt(t, scr, gw, 0); got.Style != th.ParenMatch {
		t.Errorf("open bracket at column %d is not highlighted; the fixture is wrong", gw)
	}
}

// The most visible possible bug: a cursor sitting a few columns left of the
// character it is on. drawWindow and placeCursor must agree about the width.
func TestCursorLandsOnTheCharacterAtPoint(t *testing.T) {
	f, w := singleFrame(t, "abcdef", "ghijkl")
	th := gutterTheme(true)

	for _, tc := range []struct {
		line int
		col  text.RuneIdx
		want rune
	}{
		{0, 0, 'a'}, {0, 3, 'd'}, {1, 0, 'g'}, {1, 5, 'l'},
	} {
		w.Pt = text.Pos{Line: tc.line, Col: tc.col}
		scr := drawTh(t, 30, 6, f, th)
		cx, cy, vis := scr.GetCursor()
		if !vis {
			t.Fatalf("point %v: cursor hidden", w.Pt)
		}
		got := cellAt(t, scr, cx, cy)
		if len(got.Runes) == 0 || got.Runes[0] != tc.want {
			t.Errorf("point %v: cursor at (%d,%d) sits on %q, want %q",
				w.Pt, cx, cy, string(got.Runes), string(tc.want))
		}
	}
}

func TestCurrentLineIsEmphasisedInTheActiveWindowOnly(t *testing.T) {
	th := gutterTheme(true)

	f, w := singleFrame(t, linesOf(5)...)
	w.Pt = text.Pos{Line: 2}
	scr := drawTh(t, 30, 8, f, th)
	if got := cellAt(t, scr, 0, 2); got.Style != th.LineNumberCurrent {
		t.Error("the current line's number is not emphasised in the active window")
	}
	if got := cellAt(t, scr, 0, 1); got.Style != th.LineNumber {
		t.Error("a line that is not current is emphasised")
	}

	// Same window, but the frame says something else has focus.
	f.Active = nil
	scr = drawTh(t, 30, 8, f, th)
	if got := cellAt(t, scr, 0, 2); got.Style == th.LineNumberCurrent {
		t.Error("an inactive window emphasises its current line")
	}
}

// Two windows onto one buffer keep independent viewports, so each must number
// the lines it is actually showing.
func TestSplitWindowsNumberTheirOwnVisibleLines(t *testing.T) {
	th := gutterTheme(true)
	b := bufferOf(t, linesOf(40)...)
	top := view.NewWindow(b)
	tree := view.NewTree(top)
	bottom, err := tree.Split(top, false) // stacked
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	top.Top, bottom.Top = 0, 20
	top.Pt, bottom.Pt = text.Pos{Line: 0}, text.Pos{Line: 25}

	scr := drawTh(t, 40, 14, Frame{Tree: tree, Active: top}, th)
	rects := tree.Layout(40, 13)

	// Read each pane's Top back after rendering rather than assuming it:
	// ScrollToPoint may legitimately have moved it to keep scroll margin around
	// point. The invariant under test is that a pane numbers from its own Top,
	// whatever that turned out to be.
	for _, pane := range []struct {
		name string
		win  *view.Window
	}{{"upper", top}, {"lower", bottom}} {
		r := rects[pane.win]
		first := pane.win.Top
		want := strconv.Itoa(first+1) + " line" + strconv.Itoa(first)
		if got := rowText(t, scr, r.Y); !strings.HasPrefix(strings.TrimLeft(got, " "), want) {
			t.Errorf("%s pane first row = %q, want it to start at %q", pane.name, got, want)
		}
	}
	if top.Top == bottom.Top {
		t.Fatal("both panes scrolled to the same line; the fixture proves nothing")
	}
}

// The gutter narrows the text area, so horizontal scrolling has to be told the
// narrowed width. Given the full width it would think point was visible when it
// was one column off the right edge.
func TestHorizontalScrollUsesTheNarrowedWidth(t *testing.T) {
	th := gutterTheme(true)
	long := strings.Repeat("x", 100) + "END"
	f, w := singleFrame(t, long)
	w.Pt = text.Pos{Line: 0, Col: text.RuneIdx(len(long))}

	scr := drawTh(t, 20, 6, f, th)
	cx, cy, vis := scr.GetCursor()
	if !vis {
		t.Fatal("cursor hidden")
	}
	gw := gutterWidth(w, th)
	if cx < gw {
		t.Errorf("cursor at column %d is inside the %d-column gutter", cx, gw)
	}
	if cx > 19 {
		t.Errorf("cursor at column %d is off screen", cx)
	}
	// Point is at end of line, so the cell before the cursor is the last 'D'.
	if got := cellAt(t, scr, cx-1, cy); len(got.Runes) == 0 || got.Runes[0] != 'D' {
		t.Errorf("cell left of the cursor is %q, want the end of the line to be visible", string(got.Runes))
	}
}

// The truncation marker belongs in the last column of the text area, never in
// the gutter and never past the pane.
func TestTruncationMarkerSitsInTheLastTextColumn(t *testing.T) {
	th := gutterTheme(true)
	f, w := singleFrame(t, strings.Repeat("y", 200))
	w.Pt = text.Pos{}
	scr := drawTh(t, 24, 6, f, th)

	if got := cellAt(t, scr, 23, 0); len(got.Runes) == 0 || got.Runes[0] != TruncMarker {
		t.Errorf("last column holds %q, want the truncation marker", string(got.Runes))
	}
	gw := gutterWidth(w, th)
	for x := 0; x < gw; x++ {
		if got := cellAt(t, scr, x, 0); len(got.Runes) > 0 && got.Runes[0] == TruncMarker {
			t.Errorf("truncation marker drawn in the gutter at column %d", x)
		}
	}
}

// A wide glyph must still land on the right column once the gutter has shifted
// the text area.
func TestWideGlyphsAlignAfterTheGutter(t *testing.T) {
	th := gutterTheme(true)
	f, w := singleFrame(t, "日本語")
	gw := gutterWidth(w, th)
	scr := drawTh(t, 30, 6, f, th)

	for i, want := range []rune{'日', '本', '語'} {
		x := gw + i*2 // each occupies two columns
		if got := cellAt(t, scr, x, 0); len(got.Runes) == 0 || got.Runes[0] != want {
			t.Errorf("column %d holds %q, want %q", x, string(got.Runes), string(want))
		}
	}
}

// A pane with no room for both a gutter and readable text drops the gutter: text
// is the point of the window, numbers are an aid.
func TestNarrowPaneDropsTheGutter(t *testing.T) {
	th := gutterTheme(true)
	w := view.NewWindow(bufferOf(t, linesOf(1000)...)) // 5-column gutter
	for _, tc := range []struct {
		paneW    int
		wantDrop bool
	}{
		{6, true}, {8, true}, {9, false}, {40, false},
	} {
		got := gutterFor(view.Rect{W: tc.paneW, H: 5}, w, th)
		if dropped := got == 0; dropped != tc.wantDrop {
			t.Errorf("pane width %d: gutter = %d (dropped=%v), want dropped=%v",
				tc.paneW, got, dropped, tc.wantDrop)
		}
	}
}

func TestGutterSurvivesDegenerateRects(t *testing.T) {
	th := gutterTheme(true)
	w := view.NewWindow(bufferOf(t, "x"))
	for _, r := range []view.Rect{
		{}, {W: -5, H: -5}, {W: 0, H: 10}, {W: 10, H: 0}, {W: 1, H: 1},
	} {
		gutterFor(r, w, th) // must not panic
		scr := sim(t, 30, 8)
		drawGutter(scr, r, w, view.TextHeight(r), true, th) // must not panic
	}
}

func TestGutterHandlesNilWindow(t *testing.T) {
	th := gutterTheme(true)
	if got := gutterWidth(nil, th); got != 0 {
		t.Errorf("gutterWidth(nil) = %d, want 0", got)
	}
	if got := gutterWidth(&view.Window{}, th); got != 0 {
		t.Errorf("gutterWidth with no buffer = %d, want 0", got)
	}
}

// With numbers off the layout must be exactly what it was before the gutter
// existed: text at the pane's own first column.
func TestDisabledGutterRestoresTheOldLayout(t *testing.T) {
	f, w := singleFrame(t, "alpha", "beta")
	w.Pt = text.Pos{Line: 1, Col: 2}
	scr := drawTh(t, 30, 6, f, gutterTheme(false))

	if got := rowText(t, scr, 0); !strings.HasPrefix(got, "alpha") {
		t.Errorf("row 0 = %q, want text at column 0", got)
	}
	cx, cy, _ := scr.GetCursor()
	if got := cellAt(t, scr, cx, cy); len(got.Runes) == 0 || got.Runes[0] != 't' {
		t.Errorf("cursor sits on %q, want 't' of beta", string(got.Runes))
	}
}

// The default must be on: it is the sane default the feature exists to provide.
func TestLineNumbersAreOnByDefault(t *testing.T) {
	if !DefaultTheme().LineNumbers {
		t.Error("DefaultTheme().LineNumbers is false, want line numbers on by default")
	}
	f, _ := singleFrame(t, "alpha")
	scr := draw(t, 30, 6, f)
	if got := rowText(t, scr, 0); !strings.HasPrefix(got, strconv.Itoa(1)+" alpha") {
		t.Errorf("row 0 = %q, want a line number before the text by default", got)
	}
}
