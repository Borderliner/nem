package ui

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// panelOn draws p into a fresh screen and flushes it, so assertions read the
// front buffer - what a terminal would actually display.
func panelOn(t *testing.T, w, h int, p Panel) tcell.SimulationScreen {
	t.Helper()
	scr := sim(t, w, h)
	drawPanel(scr, p, DefaultTheme())
	scr.Show()
	return scr
}

// styleAt is the front-buffer style at (x, y).
func styleAt(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.Style {
	t.Helper()
	return cellAt(t, scr, x, y).Style
}

func TestPanelDrawsBorder(t *testing.T) {
	scr := panelOn(t, 20, 6, Panel{
		Rect:  view.Rect{X: 1, Y: 1, W: 8, H: 4},
		Lines: []PanelLine{{Text: "ab"}, {Text: "cd"}},
	})

	for _, tc := range []struct {
		name string
		x, y int
		want rune
	}{
		{"top left", 1, 1, panelTopLeft},
		{"top right", 8, 1, panelTopRight},
		{"bottom left", 1, 4, panelBottomLeft},
		{"bottom right", 8, 4, panelBottomRight},
		{"top edge", 4, 1, panelHorizontal},
		{"bottom edge", 4, 4, panelHorizontal},
		{"left edge", 1, 2, panelVertical},
		{"right edge", 8, 2, panelVertical},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cellAt(t, scr, tc.x, tc.y).Runes; len(got) == 0 || got[0] != tc.want {
				t.Errorf("cell (%d,%d) = %q, want %q", tc.x, tc.y, string(got), string(tc.want))
			}
		})
	}
}

func TestPanelDrawsContentInsideTheBorder(t *testing.T) {
	scr := panelOn(t, 20, 6, Panel{
		Rect:  view.Rect{X: 1, Y: 1, W: 8, H: 4},
		Lines: []PanelLine{{Text: "ab"}, {Text: "cd"}},
	})
	if got := rowText(t, scr, 2); !strings.Contains(got, "ab") {
		t.Errorf("row 2 = %q, want it to contain the first line", got)
	}
	if got := rowText(t, scr, 3); !strings.Contains(got, "cd") {
		t.Errorf("row 3 = %q, want it to contain the second line", got)
	}
	// The interior starts one cell in from the border.
	if got := cellAt(t, scr, 2, 2).Runes; len(got) == 0 || got[0] != 'a' {
		t.Errorf("cell (2,2) = %q, want the line to start one cell inside the border", string(got))
	}
}

// More lines than the interior can hold are dropped, not drawn over the border.
func TestPanelDropsLinesPastTheInterior(t *testing.T) {
	scr := panelOn(t, 20, 8, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 10, H: 4}, // interior is 2 rows
		Lines: []PanelLine{{Text: "one"}, {Text: "two"}, {Text: "three"}},
	})
	if got := rowText(t, scr, 3); strings.Contains(got, "three") {
		t.Errorf("row 3 = %q, want the bottom border, not a third line", got)
	}
	if got := cellAt(t, scr, 1, 3).Runes; len(got) == 0 || got[0] != panelHorizontal {
		t.Errorf("cell (1,3) = %q, want the bottom border intact", string(got))
	}
}

func TestPanelTitleSitsInTheTopBorder(t *testing.T) {
	scr := panelOn(t, 30, 6, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 20, H: 4},
		Title: "3/57",
		Lines: []PanelLine{{Text: "x"}},
	})
	if got := rowText(t, scr, 0); !strings.Contains(got, "3/57") {
		t.Errorf("top border = %q, want it to carry the title", got)
	}
	// Corners survive the title.
	if got := cellAt(t, scr, 0, 0).Runes; got[0] != panelTopLeft {
		t.Errorf("top-left corner = %q, want it intact", string(got))
	}
	if got := cellAt(t, scr, 19, 0).Runes; got[0] != panelTopRight {
		t.Errorf("top-right corner = %q, want it intact", string(got))
	}
}

// A title longer than the border truncates rather than overwriting the corner.
func TestPanelTitleTruncates(t *testing.T) {
	scr := panelOn(t, 30, 6, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 10, H: 4},
		Title: "a-very-long-title-indeed",
		Lines: []PanelLine{{Text: "x"}},
	})
	if got := cellAt(t, scr, 9, 0).Runes; len(got) == 0 || got[0] != panelTopRight {
		t.Errorf("top-right corner = %q, want it intact under a long title", string(got))
	}
}

func TestPanelSelectedRowIsStyled(t *testing.T) {
	th := DefaultTheme()
	scr := sim(t, 20, 6)
	drawPanel(scr, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 12, H: 5},
		Lines: []PanelLine{{Text: "one"}, {Text: "two", Selected: true}, {Text: "three"}},
	}, th)
	scr.Show()

	plain := styleAt(t, scr, 1, 1)  // "one"
	chosen := styleAt(t, scr, 1, 2) // "two", selected
	if plain == chosen {
		t.Error("the selected row renders identically to an unselected one")
	}
	if want := overlay(th.Text, th.PanelSelected); chosen != want {
		t.Errorf("selected cell style = %v, want %v", chosen, want)
	}
	// The whole interior width is filled, so a selected row reads as a bar
	// rather than stopping at the end of its text.
	if got := styleAt(t, scr, 9, 2); got != overlay(th.Text, th.PanelSelected) {
		t.Errorf("selected row not filled to the interior edge: style at x=9 is %v", got)
	}
}

func TestPanelMatchedRunesAreEmphasised(t *testing.T) {
	th := DefaultTheme()
	scr := sim(t, 30, 5)
	drawPanel(scr, Panel{
		Rect: view.Rect{X: 0, Y: 0, W: 20, H: 3},
		// "forward-char" with f, w, c emphasised, as fuzzy would report for "fwc".
		Lines: []PanelLine{{Text: "forward-char", Match: []int{0, 3, 8}}},
	}, th)
	scr.Show()

	match := overlay(th.Text, th.PanelMatch)
	for _, x := range []int{1, 4, 9} { // interior starts at x=1
		if got := styleAt(t, scr, x, 1); got != match {
			t.Errorf("cell x=%d should be emphasised: style %v, want %v", x, got, match)
		}
	}
	for _, x := range []int{2, 3, 5} { // o, r, a - not matched
		if got := styleAt(t, scr, x, 1); got == match {
			t.Errorf("cell x=%d is emphasised but should not be", x)
		}
	}
}

// Match indices are RUNE indices. After a wide glyph the rune index and the
// display column diverge, and emphasising by index would light the wrong cell.
func TestPanelMatchColumnsAccountForWideGlyphs(t *testing.T) {
	th := DefaultTheme()
	scr := sim(t, 30, 5)
	// "日本X": runes 0,1,2 but columns 0,2,4. Emphasise rune 2 ("X").
	drawPanel(scr, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 20, H: 3},
		Lines: []PanelLine{{Text: "日本X", Match: []int{2}}},
	}, th)
	scr.Show()

	match := overlay(th.Text, th.PanelMatch)
	// Interior starts at x=1, so rune 2 sits at column 4 -> screen x=5.
	if got := cellAt(t, scr, 5, 1).Runes; len(got) == 0 || got[0] != 'X' {
		t.Fatalf("expected 'X' at x=5, found %q", string(got))
	}
	if got := styleAt(t, scr, 5, 1); got != match {
		t.Errorf("'X' not emphasised: style %v, want %v", got, match)
	}
	// Had indices been treated as columns, x=3 would have been lit instead.
	if got := styleAt(t, scr, 3, 1); got == match {
		t.Error("column 2 was emphasised, so match indices were treated as columns not runes")
	}
}

// Indices arrive from another package and must never panic the renderer.
func TestPanelOutOfRangeMatchIndicesAreIgnored(t *testing.T) {
	scr := panelOn(t, 20, 5, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 12, H: 3},
		Lines: []PanelLine{{Text: "abc", Match: []int{-3, 1, 99}}},
	})
	if got := rowText(t, scr, 1); !strings.Contains(got, "abc") {
		t.Errorf("row = %q, want the text drawn despite bogus indices", got)
	}
}

// Truncation is by display width, not rune count: three CJK glyphs are six
// columns, so an interior of four columns holds two of them.
func TestPanelTruncatesByDisplayWidth(t *testing.T) {
	scr := panelOn(t, 20, 5, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 6, H: 3}, // interior 4 columns
		Lines: []PanelLine{{Text: "日本語"}},
	})
	// Assert per cell rather than by substring: rowText renders the trailing
	// half of a wide glyph as a blank, so the row reads "日 本 " and a
	// "contains 日本" check would fail on correct output.
	if got := cellAt(t, scr, 1, 1).Runes; len(got) == 0 || got[0] != '日' {
		t.Errorf("cell (1,1) = %q, want the first glyph", string(got))
	}
	if got := cellAt(t, scr, 3, 1).Runes; len(got) == 0 || got[0] != '本' {
		t.Errorf("cell (3,1) = %q, want the second glyph at column 2 of the interior", string(got))
	}
	if row := rowText(t, scr, 1); strings.ContainsRune(row, '語') {
		t.Errorf("row = %q, want the third glyph dropped - four columns hold two wide glyphs", row)
	}
	// The border must survive: a half-drawn glyph would overwrite it.
	if got := cellAt(t, scr, 5, 1).Runes; len(got) == 0 || got[0] != panelVertical {
		t.Errorf("right border = %q, want it intact", string(got))
	}
}

// A wide glyph that would straddle the border is dropped whole.
func TestPanelNeverSplitsAWideGlyphAtTheBorder(t *testing.T) {
	scr := panelOn(t, 20, 5, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 5, H: 3}, // interior 3 columns
		Lines: []PanelLine{{Text: "a日"}},         // a=1 col, 日=2 cols -> exactly 3
	})
	if got := cellAt(t, scr, 4, 1).Runes; len(got) == 0 || got[0] != panelVertical {
		t.Errorf("right border = %q, want it intact", string(got))
	}

	scr2 := panelOn(t, 20, 5, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 4, H: 3}, // interior 2 columns
		Lines: []PanelLine{{Text: "a日"}},         // 日 would need columns 1-2, one past
	})
	if got := rowText(t, scr2, 1); strings.Contains(got, "日") {
		t.Errorf("row = %q, want the wide glyph dropped rather than half-drawn", got)
	}
	if got := cellAt(t, scr2, 3, 1).Runes; len(got) == 0 || got[0] != panelVertical {
		t.Errorf("right border = %q, want it intact", string(got))
	}
}

// The whole point of a panel: it hides what is under it. Without the clearing
// pass the buffer text shows through the gaps between glyphs.
func TestPanelOccludesTheTextBeneathIt(t *testing.T) {
	f, _ := singleFrame(t, strings.Repeat("A", 20), strings.Repeat("B", 20))
	f.Panels = []Panel{{
		Rect:  view.Rect{X: 2, Y: 0, W: 6, H: 2},
		Lines: []PanelLine{},
	}}
	scr := drawPlain(t, 20, 6, f)

	// Index by rune, not byte: the border glyphs are multi-byte, so a byte offset
	// lands mid-sequence and compares against nothing meaningful.
	row0 := []rune(rowText(t, scr, 0))
	for x := 2; x < 8; x++ {
		if row0[x] == 'A' {
			t.Errorf("column %d holds buffer text: the panel is not opaque (row %q)", x, string(row0))
		}
	}
	// Outside the panel the text is untouched.
	if row0[0] != 'A' || row0[8] != 'A' {
		t.Errorf("row 0 = %q, want buffer text either side of the panel", string(row0))
	}
}

// The occlusion that matters is of INTERIOR cells no content writes to. A border
// draws a glyph in every column it spans, so a test that only covers a border row
// proves the border is opaque and says nothing about the panel - it passes even
// with the clearing pass deleted.
func TestPanelInteriorOccludesTextEvenWhereItHasNoContent(t *testing.T) {
	f, _ := singleFrame(t, strings.Repeat("A", 30), strings.Repeat("A", 30), strings.Repeat("A", 30))
	f.Panels = []Panel{{
		Rect:  view.Rect{X: 2, Y: 0, W: 10, H: 4}, // interior columns 3..10, rows 1..2
		Lines: []PanelLine{{Text: "ok"}},          // covers only columns 3 and 4
	}}
	scr := draw(t, 30, 8, f)

	row1 := []rune(rowText(t, scr, 1))
	for x := 5; x <= 10; x++ {
		if row1[x] == 'A' {
			t.Errorf("interior column %d shows buffer text through the panel (row %q)", x, string(row1))
		}
	}
	// Row 2 is interior with no content line at all - entirely blank cells.
	row2 := []rune(rowText(t, scr, 2))
	for x := 3; x <= 10; x++ {
		if row2[x] == 'A' {
			t.Errorf("empty interior column %d shows buffer text (row %q)", x, string(row2))
		}
	}
}

// A rect hanging off the left of the screen is clipped to a closed box rather
// than drawn with its left edge missing. view.PlacePanel never produces one, so
// this is the degradation path for a stale rect captured before a resize.
func TestPanelPartlyOffscreenDrawsAClosedBox(t *testing.T) {
	scr := panelOn(t, 20, 6, Panel{
		Rect:  view.Rect{X: -4, Y: 1, W: 10, H: 4},
		Lines: []PanelLine{{Text: "x"}},
	})
	if got := cellAt(t, scr, 0, 1).Runes; len(got) == 0 || got[0] != panelTopLeft {
		t.Errorf("cell (0,1) = %q, want the box closed at the screen edge", string(got))
	}
	if got := cellAt(t, scr, 5, 1).Runes; len(got) == 0 || got[0] != panelTopRight {
		t.Errorf("cell (5,1) = %q, want the right corner where the rect ends", string(got))
	}
}

func TestPanelsOverlapInSliceOrder(t *testing.T) {
	f, _ := singleFrame(t, strings.Repeat("A", 30))
	f.Panels = []Panel{
		{Rect: view.Rect{X: 0, Y: 0, W: 10, H: 3}, Lines: []PanelLine{{Text: "first"}}},
		{Rect: view.Rect{X: 3, Y: 0, W: 10, H: 3}, Lines: []PanelLine{{Text: "second"}}},
	}
	scr := draw(t, 30, 6, f)
	if got := rowText(t, scr, 1); !strings.Contains(got, "second") {
		t.Errorf("row 1 = %q, want the later panel drawn over the earlier one", got)
	}
}

func TestPanelDegenerateAndOffscreenRectsDoNotPanic(t *testing.T) {
	for _, tc := range []struct {
		name string
		rect view.Rect
	}{
		{"zero", view.Rect{}},
		{"negative width", view.Rect{X: 2, Y: 2, W: -5, H: 3}},
		{"negative height", view.Rect{X: 2, Y: 2, W: 5, H: -3}},
		{"one cell", view.Rect{X: 1, Y: 1, W: 1, H: 1}},
		{"entirely off the right", view.Rect{X: 50, Y: 1, W: 10, H: 3}},
		{"entirely below", view.Rect{X: 1, Y: 50, W: 10, H: 3}},
		{"negative origin", view.Rect{X: -4, Y: -2, W: 10, H: 5}},
		{"wider than the screen", view.Rect{X: 0, Y: 0, W: 500, H: 3}},
		{"taller than the screen", view.Rect{X: 0, Y: 0, W: 10, H: 500}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panelOn(t, 20, 6, Panel{
				Rect:  tc.rect,
				Title: "t",
				Lines: []PanelLine{{Text: "abc", Match: []int{0}}, {Text: "def", Selected: true}},
			})
		})
	}
}

// A rect too small to hold an interior draws the border alone, which is what
// view.MinPanelWidth x MinPanelHeight produces.
func TestPanelAtTheMinimumSizeShowsOneLine(t *testing.T) {
	// view's minimums include the border a panel always draws, so the smallest
	// placeable panel has a real interior: one content row, four columns. This
	// is the invariant that keeps PlacePanel and drawPanel agreeing about what
	// "usable" means - it previously drew a bare box at the minimum.
	scr := panelOn(t, 20, 6, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: view.MinPanelWidth, H: view.MinPanelHeight},
		Lines: []PanelLine{{Text: "xyz"}},
	})
	if got := rowText(t, scr, 0); !strings.Contains(got, string(panelTopLeft)) {
		t.Errorf("row 0 = %q, want a border", got)
	}
	if got := rowText(t, scr, 1); !strings.Contains(got, "xyz") {
		t.Errorf("row 1 = %q, want the content row to show %q", got, "xyz")
	}
	if got := rowText(t, scr, view.MinPanelHeight-1); !strings.Contains(got, string(panelBottomLeft)) {
		t.Errorf("last row = %q, want the bottom border", got)
	}
}

// When the prompt renders inside a panel the hardware cursor has to go there,
// which neither the active window nor the echo row can express.
func TestFrameCursorOverrideWins(t *testing.T) {
	f, _ := singleFrame(t, "hello", "world")
	f.CursorSet, f.CursorX, f.CursorY = true, 7, 3
	scr := draw(t, 20, 6, f)
	if x, y, vis := scr.GetCursor(); !vis || x != 7 || y != 3 {
		t.Errorf("cursor at (%d,%d) visible=%v, want (7,3) visible", x, y, vis)
	}
}

// The override beats an active minibuffer prompt, so a completion panel holding
// the prompt keeps the cursor even though MiniOn is set.
func TestCursorOverrideBeatsTheMinibuffer(t *testing.T) {
	f, _ := singleFrame(t, "hello")
	f.MiniOn, f.MiniPt, f.Echo = true, 4, "M-x pr"
	f.CursorSet, f.CursorX, f.CursorY = true, 11, 2
	scr := draw(t, 20, 6, f)
	if x, y, _ := scr.GetCursor(); x != 11 || y != 2 {
		t.Errorf("cursor at (%d,%d), want the override at (11,2)", x, y)
	}
}

// An override outside the screen is clamped rather than passed through.
func TestCursorOverrideIsClamped(t *testing.T) {
	f, _ := singleFrame(t, "hello")
	f.CursorSet, f.CursorX, f.CursorY = true, 999, 999
	scr := draw(t, 20, 6, f)
	x, y, vis := scr.GetCursor()
	if !vis || x > 19 || y > 5 || x < 0 || y < 0 {
		t.Errorf("cursor at (%d,%d) visible=%v, want it clamped inside 20x6", x, y, vis)
	}
}

// A candidate can contain a tab - a filename may, and a caller can put any
// string in a line. It must expand to spaces rather than be written literally,
// or the terminal reinterprets it and the row's columns stop matching what the
// panel measured.
func TestPanelExpandsATabToSpaces(t *testing.T) {
	scr := panelOn(t, 24, 5, Panel{
		Rect:  view.Rect{X: 0, Y: 0, W: 20, H: 3},
		Lines: []PanelLine{{Text: "a\tb"}},
	})
	row := []rune(rowText(t, scr, 1))
	if strings.ContainsRune(string(row), '\t') {
		t.Errorf("row %q holds a literal tab", string(row))
	}
	if row[1] != 'a' {
		t.Errorf("cell x=1 = %q, want 'a'", string(row[1]))
	}
	// A tab from column 1 reaches the next multiple of 8, so 'b' lands at
	// interior column 8, i.e. screen x=9.
	if row[9] != 'b' {
		t.Errorf("row = %q, want 'b' at x=9 after the tab stop", string(row))
	}
	for x := 2; x < 9; x++ {
		if row[x] != ' ' {
			t.Errorf("cell x=%d = %q, want the tab expanded to a space", x, string(row[x]))
		}
	}
}

// clampInt's inverted range cannot be reached through Render, which bails out on
// a zero-size screen before placing a cursor. It is a guard against a future
// caller, and is tested directly rather than left to look like dead code.
func TestClampIntHandlesAnInvertedRange(t *testing.T) {
	for _, tc := range []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5},
		{-3, 0, 10, 0},
		{99, 0, 10, 10},
		{5, 0, -1, 0}, // a zero-width screen yields hi = w-1 = -1
	} {
		if got := clampInt(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}
