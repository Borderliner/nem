package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// The frame reserves the bottom row for the echo area, so a screen of height h
// lays the tree out over h-1 rows and each window's modeline sits on the last
// row of its own rectangle. For a single window on a 6-row screen that puts
// text on rows 0-3, the modeline on row 4 and the echo area on row 5.

func TestSingleWindowDrawsTextAndModeline(t *testing.T) {
	f, _ := singleFrame(t, "hello", "world")
	scr := drawPlain(t, 20, 6, f)

	if got := strings.TrimRight(rowText(t, scr, 0), " "); got != "hello" {
		t.Errorf("row 0 = %q, want %q", got, "hello")
	}
	if got := strings.TrimRight(rowText(t, scr, 1), " "); got != "world" {
		t.Errorf("row 1 = %q, want %q", got, "world")
	}
	// Rows past the end of the buffer stay blank, as in emacs.
	if got := strings.TrimSpace(rowText(t, scr, 2)); got != "" {
		t.Errorf("row 2 = %q, want blank", got)
	}
	if ml := rowText(t, scr, 4); !strings.Contains(ml, ScratchName) {
		t.Errorf("modeline row = %q, want it to name the buffer %q", ml, ScratchName)
	}
}

func TestCursorSitsAtPointInActiveWindow(t *testing.T) {
	f, w := singleFrame(t, "hello", "world")
	w.Pt = text.Pos{Line: 1, Col: 3}
	scr := drawPlain(t, 20, 6, f)

	x, y, vis := scr.GetCursor()
	if !vis {
		t.Fatal("cursor hidden; the real hardware cursor is the point of using tcell")
	}
	if x != 3 || y != 1 {
		t.Errorf("cursor at (%d,%d), want (3,1)", x, y)
	}
}

func TestCursorUsesDisplayColumnNotRuneIndex(t *testing.T) {
	// Point after two CJK glyphs is rune index 2 but display column 4.
	f, w := singleFrame(t, "日本語")
	w.Pt = text.Pos{Line: 0, Col: 2}
	scr := drawPlain(t, 20, 6, f)

	x, _, _ := scr.GetCursor()
	if x != 4 {
		t.Errorf("cursor column = %d, want 4 (two wide glyphs)", x)
	}
}

func TestVerticalSplitDrawsBothPanesAndDivider(t *testing.T) {
	left := view.NewWindow(bufferOf(t, "LEFT"))
	tree := view.NewTree(left)
	right, err := tree.Split(left, true)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	right.Visit(bufferOf(t, "RIGHT"))

	scr := drawPlain(t, 40, 8, Frame{Tree: tree, Active: left})

	dividers := tree.Dividers(40, 7)
	if len(dividers) != 1 {
		t.Fatalf("got %d dividers, want 1", len(dividers))
	}
	dx := dividers[0].X

	row := rowText(t, scr, 0)
	if !strings.Contains(row[:dx], "LEFT") {
		t.Errorf("left of divider = %q, want it to contain LEFT", row[:dx])
	}
	if !strings.Contains(row[dx+1:], "RIGHT") {
		t.Errorf("right of divider = %q, want it to contain RIGHT", row[dx+1:])
	}

	// The divider column must be intact for the full height of the panes: any
	// width miscount in the left pane bleeds onto it.
	col := colText(t, scr, dx)
	for i, r := range col[:view.TextHeight(dividers[0])] {
		if r != '│' {
			t.Errorf("divider column row %d = %q, want '│' (full column %q)", i, r, col)
			break
		}
	}
}

func TestHorizontalSplitHasNoDivider(t *testing.T) {
	top := view.NewWindow(bufferOf(t, "TOP"))
	tree := view.NewTree(top)
	bottom, err := tree.Split(top, false)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	bottom.Visit(bufferOf(t, "BOTTOM"))

	scr := drawPlain(t, 40, 12, Frame{Tree: tree, Active: top})

	// view draws no divider for a horizontal split: the upper window's modeline
	// already separates the panes.
	cells, w, h := scr.GetContents()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for _, r := range cells[y*w+x].Runes {
				if r == '│' {
					t.Fatalf("found a divider rune at (%d,%d); horizontal splits have none", x, y)
				}
			}
		}
	}
	if !strings.Contains(rowText(t, scr, 0), "TOP") {
		t.Error("upper pane did not draw")
	}
	found := false
	for y := 0; y < h; y++ {
		if strings.Contains(rowText(t, scr, y), "BOTTOM") {
			found = true
		}
	}
	if !found {
		t.Error("lower pane did not draw")
	}
}

func TestScrollFollowsPointBelowViewport(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line" + string(rune('A'+i%26))
	}
	f, w := singleFrame(t, lines...)
	w.Pt = text.Pos{Line: 30}

	scr := drawPlain(t, 20, 8, f)

	if w.Top == 0 {
		t.Fatal("Top still 0; the render path must scroll point into view")
	}
	_, y, vis := scr.GetCursor()
	if !vis {
		t.Fatal("cursor hidden")
	}
	textH := 8 - 1 - 1 // screen height less echo row less modeline
	if y < 0 || y >= textH {
		t.Errorf("cursor row = %d, want within the text area [0,%d)", y, textH)
	}
	if got := strings.TrimRight(rowText(t, scr, y), " "); got != lines[30] {
		t.Errorf("cursor row shows %q, want the line point is on, %q", got, lines[30])
	}
}

func TestLongLineTruncatesWithMarker(t *testing.T) {
	f, _ := singleFrame(t, strings.Repeat("a", 100))
	scr := drawPlain(t, 20, 6, f)

	row := rowText(t, scr, 0)
	if want := strings.Repeat("a", 19) + "$"; row != want {
		t.Errorf("row 0 = %q, want %q", row, want)
	}
}

func TestShortLineHasNoTruncationMarker(t *testing.T) {
	f, _ := singleFrame(t, strings.Repeat("a", 20))
	scr := drawPlain(t, 20, 6, f)

	if row := rowText(t, scr, 0); strings.Contains(row, "$") {
		t.Errorf("row 0 = %q, want no truncation marker for a line that fits exactly", row)
	}
}

func TestWideGlyphNeverDoubleDrawn(t *testing.T) {
	f, _ := singleFrame(t, "日本語")
	scr := drawPlain(t, 20, 6, f)

	for i, want := range []string{"日", "", "本", "", "語", ""} {
		if got := string(cellAt(t, scr, i, 0).Runes); got != want {
			t.Errorf("cell %d = %q, want %q (trailing cell of a wide glyph must stay empty)", i, got, want)
		}
	}
}

func TestEmojiOccupiesTwoCells(t *testing.T) {
	f, _ := singleFrame(t, "🚀x")
	scr := drawPlain(t, 20, 6, f)

	if got := string(cellAt(t, scr, 0, 0).Runes); got != "🚀" {
		t.Errorf("cell 0 = %q, want the emoji", got)
	}
	if got := string(cellAt(t, scr, 2, 0).Runes); got != "x" {
		t.Errorf("cell 2 = %q, want x after a two-cell emoji", got)
	}
}

func TestCombiningMarkStaysInOneCell(t *testing.T) {
	f, _ := singleFrame(t, "éz")
	scr := drawPlain(t, 20, 6, f)

	if got := string(cellAt(t, scr, 0, 0).Runes); got != "é" {
		t.Errorf("cell 0 = %q, want the combining cluster", got)
	}
	if got := string(cellAt(t, scr, 1, 0).Runes); got != "z" {
		t.Errorf("cell 1 = %q, want z; a combining mark consumes no column", got)
	}
}

func TestTabAdvancesToTabStop(t *testing.T) {
	f, _ := singleFrame(t, "\tx")
	scr := drawPlain(t, 20, 6, f)

	row := rowText(t, scr, 0)
	if want := strings.Repeat(" ", 8) + "x"; !strings.HasPrefix(row, want) {
		t.Errorf("row 0 = %q, want a tab expanded to column 8 then x", row)
	}
}

func TestWideGlyphStraddlingRightEdgeIsClipped(t *testing.T) {
	// "aa日" is 4 columns wide in a 3-column window, so it truncates: the
	// marker takes the last column and the wide glyph cannot fit in the two
	// that remain. It must be dropped whole, never half-drawn.
	f, _ := singleFrame(t, "aa日")
	scr := drawPlain(t, 3, 6, f)

	if row := rowText(t, scr, 0); row != "aa$" {
		t.Errorf("row 0 = %q, want %q", row, "aa$")
	}
}

func TestHorizontalScrollFollowsPoint(t *testing.T) {
	f, w := singleFrame(t, strings.Repeat("abcde", 20)) // 100 columns
	w.Pt = text.Pos{Line: 0, Col: 90}

	scr := drawPlain(t, 20, 6, f)

	x, y, vis := scr.GetCursor()
	if !vis {
		t.Fatal("cursor hidden")
	}
	if y != 0 {
		t.Errorf("cursor row = %d, want 0", y)
	}
	if x < 0 || x >= 20 {
		t.Errorf("cursor column = %d, want it scrolled into [0,20)", x)
	}
	// The glyph under point must be the one point is on.
	if got := string(cellAt(t, scr, x, 0).Runes); got != "a" {
		t.Errorf("cell under cursor = %q, want %q (column 90 of the repeat)", got, "a")
	}
}

func TestEchoAreaDrawsOnBottomRow(t *testing.T) {
	f, _ := singleFrame(t, "hi")
	f.Echo = "C-x C-z is undefined"
	scr := drawPlain(t, 40, 6, f)

	if got := rowText(t, scr, 5); !strings.Contains(got, "C-x C-z is undefined") {
		t.Errorf("echo row = %q, want the message", got)
	}
}

func TestMinibufferInactiveLeavesCursorInWindow(t *testing.T) {
	f, w := singleFrame(t, "hello")
	w.Pt = text.Pos{Line: 0, Col: 2}
	f.Echo = "a message"
	scr := drawPlain(t, 40, 6, f)

	x, y, _ := scr.GetCursor()
	if x != 2 || y != 0 {
		t.Errorf("cursor at (%d,%d), want (2,0) in the window", x, y)
	}
}

func TestMinibufferActiveTakesCursor(t *testing.T) {
	f, w := singleFrame(t, "hello")
	w.Pt = text.Pos{Line: 0, Col: 2}
	f.Echo = "Find file: /home/reza/"
	f.MiniOn = true
	f.MiniPt = 22

	scr := drawPlain(t, 40, 6, f)

	x, y, vis := scr.GetCursor()
	if !vis {
		t.Fatal("cursor hidden while the minibuffer is active")
	}
	if y != 5 {
		t.Errorf("cursor row = %d, want 5 (the echo row)", y)
	}
	if x != 22 {
		t.Errorf("cursor column = %d, want 22", x)
	}
	if got := rowText(t, scr, 5); !strings.Contains(got, "Find file:") {
		t.Errorf("echo row = %q, want the prompt", got)
	}
}

func TestDegenerateFramesDoNotPanic(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 1}, {1, 2}, {3, 2}, {8, 2}, {2, 8}} {
		w, h := size[0], size[1]
		t.Run("", func(t *testing.T) {
			f, _ := singleFrame(t, "hello", "world")
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked at %dx%d: %v", w, h, r)
				}
			}()
			scr := sim(t, w, h)
			Render(scr, f, DefaultTheme())
			scr.Show()
		})
	}
}

func TestDegenerateSplitFrameDoesNotPanic(t *testing.T) {
	a := view.NewWindow(bufferOf(t, "a"))
	tree := view.NewTree(a)
	if _, err := tree.Split(a, true); err != nil {
		t.Skipf("split refused at startup size: %v", err)
	}
	for _, size := range [][2]int{{4, 3}, {10, 3}, {1, 4}} {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked at %dx%d: %v", size[0], size[1], r)
			}
		}()
		scr := sim(t, size[0], size[1])
		Render(scr, Frame{Tree: tree, Active: a}, DefaultTheme())
		scr.Show()
	}
}

func TestRenderIsIdempotent(t *testing.T) {
	f, w := singleFrame(t, "hello", "world")
	w.Pt = text.Pos{Line: 1, Col: 2}

	first := drawPlain(t, 20, 6, f)
	want := rowText(t, first, 0) + "|" + rowText(t, first, 1)

	second := drawPlain(t, 20, 6, f)
	got := rowText(t, second, 0) + "|" + rowText(t, second, 1)

	if got != want {
		t.Errorf("second render differs:\n got %q\nwant %q", got, want)
	}
}

// Two windows onto the same buffer scroll independently on both axes.
//
// This used to require a map in the renderer keyed by window, which leaked an
// entry per closed window. Horizontal scroll now lives on view.Window beside
// Top, so independence comes for free and a closed window's state is collected
// with it - there is no longer anywhere for a leak to happen.
func TestWindowsOntoOneBufferScrollIndependently(t *testing.T) {
	buf := bufferOf(t, strings.Repeat("x", 200))
	a := view.NewWindow(buf)
	tree := view.NewTree(a)
	scr := sim(t, 40, 12)
	Render(scr, Frame{Tree: tree, Active: a}, DefaultTheme())

	b, err := tree.Split(a, true)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	// Scroll one window far to the right, leave the other at the origin.
	a.Pt = text.Pos{Line: 0, Col: 150}
	b.Pt = text.Pos{Line: 0, Col: 0}
	Render(scr, Frame{Tree: tree, Active: a}, DefaultTheme())

	if a.LeftCol == 0 {
		t.Error("window a did not scroll right to follow its point")
	}
	if b.LeftCol != 0 {
		t.Errorf("window b LeftCol = %d, want 0: it shares a buffer, not a viewport", b.LeftCol)
	}

	if err := tree.Delete(b); err != nil {
		t.Fatalf("delete: %v", err)
	}
	Render(scr, Frame{Tree: tree, Active: a}, DefaultTheme())
}

func TestHorizontalScrollReturnsLeftward(t *testing.T) {
	f, w := singleFrame(t, strings.Repeat("abcde", 20))
	scr := sim(t, 20, 6)

	w.Pt = text.Pos{Line: 0, Col: 90}
	plain := DefaultTheme()
	plain.LineNumbers = false // measures text-area columns; see drawPlain
	Render(scr, f, plain)
	if w.LeftCol == 0 {
		t.Fatal("expected to have scrolled right")
	}

	w.Pt = text.Pos{Line: 0, Col: 0}
	Render(scr, f, plain)
	if got := w.LeftCol; got != 0 {
		t.Errorf("LeftCol = %d after point returned to column 0, want 0", got)
	}
	scr.Show()
	if got := string(cellAt(t, scr, 0, 0).Runes); got != "a" {
		t.Errorf("cell 0 = %q, want the first glyph of the line", got)
	}
}

func TestCursorHiddenWhenActiveWindowIsNotOnScreen(t *testing.T) {
	f, _ := singleFrame(t, "hello")
	f.Active = view.NewWindow(bufferOf(t, "elsewhere")) // not in the tree
	scr := drawPlain(t, 20, 6, f)

	if _, _, vis := scr.GetCursor(); vis {
		t.Error("cursor shown for a window that is not in the tree")
	}
}

// A negative scroll margin is nonsense a config file can easily produce. It
// must behave as zero rather than scrolling backwards or panicking, and since
// the clamp now lives in Render the test has to be behavioural.
func TestNegativeScrollMarginIsClamped(t *testing.T) {
	f, w := singleFrame(t, linesOf(40)...)
	w.Pt = text.Pos{Line: 30, Col: 0}

	th := DefaultTheme()
	th.ScrollMargin = -5
	scr := sim(t, 20, 8)
	Render(scr, f, th)

	textH := 6 // 8 rows less the echo row less the modeline
	if w.Top > 30 || 30 >= w.Top+textH {
		t.Errorf("Top = %d leaves point at line 30 outside [%d,%d)", w.Top, w.Top, w.Top+textH)
	}
}

func TestWideGlyphStartingInsideEdgeButOverflowingIsDropped(t *testing.T) {
	// "a日b" is 4 columns in a 3-column window, so it truncates and the marker
	// takes the last column, leaving 2 usable. The wide glyph STARTS inside
	// those 2 columns but needs 3, so it must be dropped whole. A bounds check
	// that only asks where a glyph starts, rather than where it ends, draws
	// half of it here.
	f, _ := singleFrame(t, "a日b")
	scr := drawPlain(t, 3, 6, f)

	if row := rowText(t, scr, 0); row != "a $" {
		t.Errorf("row 0 = %q, want %q", row, "a $")
	}
}

func TestTabRegionCarriesTheTextStyle(t *testing.T) {
	// A tab spans several columns and every one of them must be painted with
	// the text style, not merely left blank. It looks identical under a default
	// style, and wrong the moment the text area has a background colour.
	th := DefaultTheme()
	th.LineNumbers = false // measures text-area columns; see drawPlain
	th.Text = tcell.StyleDefault.Background(tcell.ColorDarkBlue)

	scr := sim(t, 20, 6)
	f, _ := singleFrame(t, "\tx")
	Render(scr, f, th)
	scr.Show()

	wantFg, wantBg, _ := th.Text.Decompose()
	for x := 0; x < 8; x++ {
		c := cellAt(t, scr, x, 0)
		if len(c.Runes) != 1 || c.Runes[0] != ' ' {
			t.Errorf("cell %d holds %q, want a single space painted across the tab", x, string(c.Runes))
			continue
		}
		fg, bg, _ := c.Style.Decompose()
		if fg != wantFg || bg != wantBg {
			t.Errorf("cell %d style = (fg %v, bg %v), want (fg %v, bg %v)", x, fg, bg, wantFg, wantBg)
		}
	}
}

func TestGlyphStraddlingLeftEdgeDoesNotCorruptTheDivider(t *testing.T) {
	// Horizontal scroll is computed from the line point is on, but every
	// visible line is drawn with that same offset. A line whose glyph
	// boundaries fall differently can therefore have a wide glyph straddling
	// the left edge of the pane. In the RIGHT pane of a split, drawing its
	// left half means writing at paneX-1, which is the divider column.
	//
	// Line 0 is prefixed with an ASCII rune so its CJK boundaries land on odd
	// columns; line 1 is pure CJK with boundaries on even ones. Point sits on
	// line 0, so the offset is odd and line 1 straddles.
	lines := []string{"x" + strings.Repeat("日", 30), strings.Repeat("日", 30)}

	leftWin := view.NewWindow(bufferOf(t, "left pane"))
	tree := view.NewTree(leftWin)
	rightWin, err := tree.Split(leftWin, true)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	rightWin.Visit(bufferOf(t, lines...))
	rightWin.Pt = text.Pos{Line: 0, Col: 20}

	scr := drawPlain(t, 40, 8, Frame{Tree: tree, Active: rightWin})

	dividers := tree.Dividers(40, 7)
	if len(dividers) != 1 {
		t.Fatalf("got %d dividers, want 1", len(dividers))
	}
	dx := dividers[0].X

	col := colText(t, scr, dx)
	for y := 0; y < view.TextHeight(dividers[0]); y++ {
		if r := []rune(col)[y]; r != '│' {
			t.Errorf("divider column row %d = %q, want '│': a pane bled into it", y, r)
		}
	}
}

func TestScrollKeepsMarginOfContextBelowPoint(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "line" + string(rune('A'+i%26))
	}
	f, w := singleFrame(t, lines...)
	w.Pt = text.Pos{Line: 30}

	scr := drawPlain(t, 20, 8, f)

	_, y, _ := scr.GetCursor()
	textH := 6 // 8 rows less the echo row less the modeline
	if want := textH - 1 - DefaultScrollMargin; y > want {
		t.Errorf("cursor row = %d, want at most %d so %d rows of context stay below point",
			y, want, DefaultScrollMargin)
	}
}

func TestDrawLineDoesNotWriteLeftOfItsRectangle(t *testing.T) {
	// Tested on drawLine directly rather than through Render, because Render
	// paints dividers after windows and would repaint over the damage. The
	// guard is what keeps a half-scrolled wide glyph from writing at x-1, which
	// in a split is the divider column and in general is not this pane's cell.
	l := text.NewLine([]rune(strings.Repeat("日", 3))) // 6 columns
	scr := sim(t, 8, 1)
	drawLine(scr, 1, 0, 4, &l, 1, DefaultTheme(), lineHL{}, regionHL{}, nil)
	scr.Show()

	if got := cellAt(t, scr, 0, 0); len(got.Runes) != 0 && string(got.Runes) != " " {
		t.Errorf("cell left of the rectangle holds %q, want it untouched", string(got.Runes))
	}
}
