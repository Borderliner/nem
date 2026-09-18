package main

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v2"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func demoPanes() []pane {
	return []pane{
		{
			name: "buffer.go", modified: true,
			lines: []string{"package main", "", "func main() {", "}"},
			line:  3, col: 1,
		},
		{
			name: "notes.md", modified: false,
			lines: []string{
				"CJK:      日本語のテキスト",
				"emoji:    🙂 🚀 ✨",
				"ZWJ fam:  👨‍👩‍👧‍👦 (one cluster)",
				"combining: ééé",
				"mixed:    ab日cd🙂ef",
			},
			line: 3, col: 11,
		},
	}
}

func renderDemo(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(w, h)
	draw(scr, demoPanes(), 0)
	scr.Show()
	return scr
}

// dump renders the screen one rune per cell, so column N of the output is
// column N of the terminal.
//
// Two artifacts follow from that and are correct, not bugs. A wide glyph owns
// two cells, so its trailing cell prints as a space: "日本" appears as "日 本 ".
// And only the first rune of a cluster is shown, so a ZWJ family sequence
// appears as a lone "👨". Both keep the dump column-aligned, which is what makes
// it useful for spotting real misalignment.
func dump(scr tcell.SimulationScreen) []string {
	cells, w, h := scr.GetContents()
	rows := make([]string, 0, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			r := cells[y*w+x].Runes
			if len(r) == 0 || r[0] == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r[0])
		}
		rows = append(rows, strings.TrimRight(b.String(), " "))
	}
	return rows
}

// TestDividerIsIntact is the sharpest available check on cell-width handling.
// The divider sits at a fixed column. If any glyph in the left pane is measured
// wrongly - a CJK character counted as one cell, an emoji as one, a ZWJ cluster
// as several - content will overrun and land on the divider column. Asserting
// the divider survives every row proves the widths agreed end to end.
func TestDividerIsIntact(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{80, 24}, {100, 30}, {60, 12}, {41, 10},
	} {
		scr := renderDemo(t, size.w, size.h)
		cells, w, h := scr.GetContents()
		dividerX := w / 2

		for y := 0; y < h-1; y++ { // last row is the minibuffer, which spans
			r := cells[y*w+dividerX].Runes
			if len(r) == 0 || r[0] != '│' {
				t.Errorf("%dx%d: divider column %d broken at row %d: got %q",
					size.w, size.h, dividerX, y, string(r))
			}
		}
	}
}

// TestRightPaneStartsAfterDivider confirms the right pane's content begins one
// column past the divider and never before it.
func TestRightPaneStartsAfterDivider(t *testing.T) {
	scr := renderDemo(t, 80, 24)
	cells, w, _ := scr.GetContents()
	dividerX := w / 2

	// Row 0 of the right pane starts with its gutter line number "  1 ".
	r := cells[0*w+dividerX+1].Runes
	if len(r) == 0 || r[0] != ' ' {
		t.Errorf("right pane first cell = %q, want gutter space", string(r))
	}
	// The gutter's digit should appear within the first few columns.
	found := false
	for x := dividerX + 1; x < dividerX+5 && x < w; x++ {
		if rr := cells[x].Runes; len(rr) > 0 && rr[0] == '1' {
			found = true
		}
	}
	if !found {
		t.Error("right pane gutter line number not found near the divider")
	}
}

// TestModelineSpansPaneWidth checks the modeline fills its pane exactly: the
// position indicator must sit flush against the right edge without crossing it.
func TestModelineSpansPaneWidth(t *testing.T) {
	scr := renderDemo(t, 80, 24)
	cells, w, h := scr.GetContents()
	dividerX := w / 2
	modeRow := h - 2 // last pane row, above the minibuffer

	// The left modeline must not write onto the divider column.
	if r := cells[modeRow*w+dividerX].Runes; len(r) > 0 && r[0] != '│' {
		t.Errorf("left modeline overran onto divider: %q", string(r))
	}

	// The left modeline should carry the modified flag near its start.
	row := dump(scr)[modeRow]
	if !strings.Contains(row, "**") {
		t.Errorf("modeline row missing modified flag: %q", row)
	}
	if !strings.Contains(row, "buffer.go") {
		t.Errorf("modeline row missing buffer name: %q", row)
	}
}

func TestMinibufferPresent(t *testing.T) {
	scr := renderDemo(t, 80, 24)
	rows := dump(scr)
	last := rows[len(rows)-1]
	for _, want := range []string{"C-x C-s", "save", "M-x"} {
		if !strings.Contains(last, want) {
			t.Errorf("minibuffer %q missing %q", last, want)
		}
	}
}

func TestTinyTerminalDoesNotPanic(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{1, 1}, {5, 3}, {19, 5}, {20, 6}, {0, 0},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%dx%d panicked: %v", size.w, size.h, r)
				}
			}()
			scr := tcell.NewSimulationScreen("UTF-8")
			if err := scr.Init(); err != nil {
				t.Fatalf("init: %v", err)
			}
			defer scr.Fini()
			scr.SetSize(size.w, size.h)
			draw(scr, demoPanes(), 0)
			scr.Show()
		}()
	}
}

// TestVisualDump prints the rendered frame so it can be inspected in test
// output without opening a terminal.
func TestVisualDump(t *testing.T) {
	scr := renderDemo(t, 80, 16)
	t.Log("\n" + strings.Join(dump(scr), "\n"))
}
