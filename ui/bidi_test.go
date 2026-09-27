package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/Borderliner/nem/bidi"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// bidiFrame is a frame of one window on lines, prose or not, point at pt,
// with right-to-left layout on and the gutter and band off so a column is a
// column of the text.
func bidiFrame(t *testing.T, prose bool, pt text.Pos, lines ...string) (Frame, Theme) {
	t.Helper()
	f, w := singleFrame(t, lines...)
	w.Pt = pt
	f.DirectionOf = func(*text.Buffer) bool { return prose }
	th := DefaultTheme()
	th.LineNumbers, th.HighlightLine, th.Bidi = false, false, true
	return f, th
}

// visual is how Persian in logical order reads on screen, left to right: its
// letters joined, and last first.
func visual(s string) string {
	rs := bidi.Shape([]rune(s))
	slices.Reverse(rs)
	return string(rs)
}

// A line of Persian in prose reads right to left, joined, against the
// window's right edge.
func TestRightToLeftLineIsSetRightToLeft(t *testing.T) {
	f, th := bidiFrame(t, true, text.Pos{}, "سلام دنیا")
	scr := drawTh(t, 20, 4, f, th)

	want := visual("سلام دنیا")
	if got := rowText(t, scr, 0); !strings.HasSuffix(got, want) || !strings.HasPrefix(got, strings.Repeat(" ", 20-len([]rune(want)))) {
		t.Errorf("row = %q, want %q against the right edge", got, want)
	}
	// Point at the start of the line is on its first letter, the rightmost.
	if x, _, _ := scr.GetCursor(); x != 19 {
		t.Errorf("cursor at column %d, want 19", x)
	}
}

// At the end of a right-to-left line the cursor is left of the text.
func TestCursorAtTheEndOfARightToLeftLine(t *testing.T) {
	f, th := bidiFrame(t, true, text.Pos{Col: 4}, "سلام")
	scr := drawTh(t, 20, 4, f, th)
	if x, _, _ := scr.GetCursor(); x != 15 {
		t.Errorf("cursor at column %d, want 15, just left of the text", x)
	}
}

// In code the line stays left to right: the Persian in a comment reads right
// to left where it is.
func TestCodeLinesStayLeftToRight(t *testing.T) {
	f, th := bidiFrame(t, false, text.Pos{}, "// سلام")
	scr := drawTh(t, 20, 4, f, th)
	if got := strings.TrimRight(rowText(t, scr, 0), " "); got != "// "+visual("سلام") {
		t.Errorf("row = %q, want the comment left to right with its Persian reversed", got)
	}
}

// Brackets round right-to-left text are mirrored, so they still face it.
func TestBracketsAreMirrored(t *testing.T) {
	f, th := bidiFrame(t, true, text.Pos{}, "(سلام)")
	scr := drawTh(t, 20, 4, f, th)
	got := strings.TrimSpace(rowText(t, scr, 0))
	if got != "("+visual("سلام")+")" {
		t.Errorf("row = %q, want the brackets facing the word", got)
	}
}

// With the layout off, text is drawn as stored - for a terminal that lays it
// out itself.
func TestBidiOffDrawsTextAsStored(t *testing.T) {
	f, th := bidiFrame(t, true, text.Pos{}, "سلام")
	th.Bidi = false
	scr := drawTh(t, 20, 4, f, th)
	if got := strings.TrimRight(rowText(t, scr, 0), " "); got != "سلام" {
		t.Errorf("row = %q, want it as stored", got)
	}
}

// The prompt row is laid out too, its cursor where its character is drawn.
func TestPromptRowIsLaidOut(t *testing.T) {
	win := view.NewWindow(bufferOf(t, "x"))
	f := Frame{Tree: view.NewTree(win), Active: win, Echo: "Find: سلام", MiniOn: true, MiniPt: 6}
	th := DefaultTheme()
	th.Bidi = true
	scr := sim(t, 30, 4)
	Render(scr, f, th)
	scr.Show()
	if got := strings.TrimRight(rowText(t, scr, 3), " "); got != "Find: "+visual("سلام") {
		t.Errorf("prompt row = %q", got)
	}
	// Point before the first letter, which is drawn rightmost.
	if x, _, _ := scr.GetCursor(); x != 9 {
		t.Errorf("cursor at column %d, want 9", x)
	}
}
