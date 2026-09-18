package blit

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v2"
	"github.com/muesli/termenv"
)

// TestMain pins the Lip Gloss colour profile to truecolor.
//
// Lip Gloss decides how much colour to emit by probing os.Stdout. Under `go
// test` stdout is a pipe, so it degrades to the Ascii profile and silently
// renders every style with no colour at all - which would make the colour
// assertions below vacuously pass against a blitter that dropped colour
// entirely. The editor has the same problem for a different reason: once tcell
// owns the terminal, Lip Gloss's probe is meaningless, because tcell is the
// output, not stdout. Both cases need the profile set explicitly.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// render draws s into a fresh w by h simulation screen and returns it. Show is
// called so the front buffer - what a terminal would actually display - is
// populated for assertions.
func render(t *testing.T, w, h int, s string) tcell.SimulationScreen {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(w, h)
	scr.Clear()
	Draw(scr, 0, 0, w, h, s)
	scr.Show()
	return scr
}

// cellAt returns the front-buffer cell at (x, y).
func cellAt(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.SimCell {
	t.Helper()
	cells, w, h := scr.GetContents()
	if x < 0 || y < 0 || x >= w || y >= h {
		t.Fatalf("cell (%d,%d) out of bounds %dx%d", x, y, w, h)
	}
	return cells[y*w+x]
}

// runesAt returns the rune slice of the cell at (x, y).
func runesAt(t *testing.T, scr tcell.SimulationScreen, x, y int) []rune {
	t.Helper()
	return cellAt(t, scr, x, y).Runes
}

// rowText reads row y as a string, treating empty cells as spaces.
func rowText(t *testing.T, scr tcell.SimulationScreen, y int) string {
	t.Helper()
	cells, w, _ := scr.GetContents()
	var b strings.Builder
	for x := 0; x < w; x++ {
		r := cells[y*w+x].Runes
		if len(r) == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r[0])
	}
	return strings.TrimRight(b.String(), " ")
}

func TestDrawPlainText(t *testing.T) {
	scr := render(t, 20, 3, "hello")
	if got := rowText(t, scr, 0); got != "hello" {
		t.Errorf("row 0 = %q, want %q", got, "hello")
	}
}

func TestDrawOffset(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(20, 5)
	scr.Clear()
	Draw(scr, 3, 2, 10, 2, "hi")
	scr.Show()

	if got := runesAt(t, scr, 3, 2); len(got) == 0 || got[0] != 'h' {
		t.Errorf("cell (3,2) = %q, want 'h'", string(got))
	}
	if got := runesAt(t, scr, 4, 2); len(got) == 0 || got[0] != 'i' {
		t.Errorf("cell (4,2) = %q, want 'i'", string(got))
	}
	// Nothing should have landed at the origin.
	if got := runesAt(t, scr, 0, 0); len(got) > 0 && got[0] != ' ' && got[0] != 0 {
		t.Errorf("cell (0,0) = %q, want empty", string(got))
	}
}

func TestDrawMultiline(t *testing.T) {
	scr := render(t, 20, 4, "one\ntwo\nthree")
	for i, want := range []string{"one", "two", "three"} {
		if got := rowText(t, scr, i); got != want {
			t.Errorf("row %d = %q, want %q", i, got, want)
		}
	}
}

func TestDrawCarriageReturn(t *testing.T) {
	// A carriage return returns to the left edge; "XY" overwrites "ab".
	scr := render(t, 20, 2, "abc\rXY")
	if got := rowText(t, scr, 0); got != "XYc" {
		t.Errorf("row 0 = %q, want %q", got, "XYc")
	}
}

func TestColors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		x    int
		want tcell.Style
	}{
		{
			name: "basic foreground red",
			in:   "\x1b[31mR",
			want: tcell.StyleDefault.Foreground(tcell.PaletteColor(1)),
		},
		{
			name: "basic background green",
			in:   "\x1b[42mG",
			want: tcell.StyleDefault.Background(tcell.PaletteColor(2)),
		},
		{
			name: "bright foreground",
			in:   "\x1b[94mB",
			want: tcell.StyleDefault.Foreground(tcell.PaletteColor(12)),
		},
		{
			name: "256 color foreground",
			in:   "\x1b[38;5;208mO",
			want: tcell.StyleDefault.Foreground(tcell.PaletteColor(208)),
		},
		{
			name: "256 color background",
			in:   "\x1b[48;5;27mB",
			want: tcell.StyleDefault.Background(tcell.PaletteColor(27)),
		},
		{
			name: "truecolor foreground",
			in:   "\x1b[38;2;255;128;0mT",
			want: tcell.StyleDefault.Foreground(tcell.NewRGBColor(255, 128, 0)),
		},
		{
			name: "truecolor foreground and background",
			in:   "\x1b[38;2;10;20;30m\x1b[48;2;200;100;50mT",
			want: tcell.StyleDefault.
				Foreground(tcell.NewRGBColor(10, 20, 30)).
				Background(tcell.NewRGBColor(200, 100, 50)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := render(t, 10, 2, tt.in)
			if got := cellAt(t, scr, tt.x, 0).Style; got != tt.want {
				t.Errorf("style = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttributes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want tcell.Style
	}{
		{"bold", "\x1b[1mX", tcell.StyleDefault.Bold(true)},
		{"dim", "\x1b[2mX", tcell.StyleDefault.Dim(true)},
		{"italic", "\x1b[3mX", tcell.StyleDefault.Italic(true)},
		{"underline", "\x1b[4mX", tcell.StyleDefault.Underline(tcell.UnderlineStyleSolid)},
		{"blink", "\x1b[5mX", tcell.StyleDefault.Blink(true)},
		{"reverse", "\x1b[7mX", tcell.StyleDefault.Reverse(true)},
		{"strikethrough", "\x1b[9mX", tcell.StyleDefault.StrikeThrough(true)},
		{"bold and italic", "\x1b[1;3mX", tcell.StyleDefault.Bold(true).Italic(true)},
		{"curly underline", "\x1b[4:3mX", tcell.StyleDefault.Underline(tcell.UnderlineStyleCurly)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := render(t, 10, 2, tt.in)
			if got := cellAt(t, scr, 0, 0).Style; got != tt.want {
				t.Errorf("style = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResetReturnsToDefault(t *testing.T) {
	scr := render(t, 10, 2, "\x1b[1;31mA\x1b[0mB")
	if got, want := cellAt(t, scr, 0, 0).Style,
		tcell.StyleDefault.Bold(true).Foreground(tcell.PaletteColor(1)); got != want {
		t.Errorf("cell A style = %v, want %v", got, want)
	}
	if got := cellAt(t, scr, 1, 0).Style; got != tcell.StyleDefault {
		t.Errorf("cell B style = %v, want default", got)
	}
}

func TestNestedStyleUnwinds(t *testing.T) {
	// Red, then bold-on for B, then bold-off (22) for C. C must stay red.
	scr := render(t, 10, 2, "\x1b[31mA\x1b[1mB\x1b[22mC")
	red := tcell.StyleDefault.Foreground(tcell.PaletteColor(1))

	if got := cellAt(t, scr, 0, 0).Style; got != red {
		t.Errorf("A = %v, want red", got)
	}
	if got, want := cellAt(t, scr, 1, 0).Style, red.Bold(true); got != want {
		t.Errorf("B = %v, want red+bold", got)
	}
	if got := cellAt(t, scr, 2, 0).Style; got != red {
		t.Errorf("C = %v, want red without bold", got)
	}
}

func TestLipglossRoundedBorder(t *testing.T) {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Foreground(lipgloss.Color("205")).
		Render("hi")

	scr := render(t, 20, 6, box)

	// Corners of a rounded border.
	corners := []struct {
		x, y int
		want rune
	}{
		{0, 0, '╭'},
		{3, 0, '╮'},
		{0, 2, '╰'},
		{3, 2, '╯'},
	}
	for _, c := range corners {
		got := runesAt(t, scr, c.x, c.y)
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("cell (%d,%d) = %q, want %q", c.x, c.y, string(got), string(c.want))
		}
	}

	// Content sits inside the border.
	if got := runesAt(t, scr, 1, 1); len(got) == 0 || got[0] != 'h' {
		t.Errorf("content cell = %q, want 'h'", string(got))
	}
	// And carries the foreground colour through the border machinery.
	if got, want := cellAt(t, scr, 1, 1).Style,
		tcell.StyleDefault.Foreground(tcell.PaletteColor(205)); got != want {
		t.Errorf("content style = %v, want %v", got, want)
	}
}

func TestWideCJK(t *testing.T) {
	scr := render(t, 20, 2, "日本語")

	// Each glyph occupies two cells: present at even columns.
	for i, want := range []rune{'日', '本', '語'} {
		got := runesAt(t, scr, i*2, 0)
		if len(got) == 0 || got[0] != want {
			t.Fatalf("cell (%d,0) = %q, want %q", i*2, string(got), string(want))
		}
	}

	// The trailing half of a wide glyph must not repeat it.
	for _, x := range []int{1, 3, 5} {
		got := runesAt(t, scr, x, 0)
		if len(got) > 0 && (got[0] == '日' || got[0] == '本' || got[0] == '語') {
			t.Errorf("cell (%d,0) = %q, wide glyph double-drawn", x, string(got))
		}
	}

	// Text after the wide run lands at the right column.
	scr2 := render(t, 20, 2, "日本X")
	if got := runesAt(t, scr2, 4, 0); len(got) == 0 || got[0] != 'X' {
		t.Errorf("cell (4,0) = %q, want 'X' after two wide glyphs", string(got))
	}
}

func TestEmoji(t *testing.T) {
	scr := render(t, 20, 2, "🙂X")
	got := runesAt(t, scr, 0, 0)
	if len(got) == 0 || got[0] != '🙂' {
		t.Fatalf("cell (0,0) = %q, want emoji", string(got))
	}
	// Emoji is double-width, so X follows at column 2.
	if got := runesAt(t, scr, 2, 0); len(got) == 0 || got[0] != 'X' {
		t.Errorf("cell (2,0) = %q, want 'X' at column 2", string(got))
	}
}

func TestEmojiZWJSequence(t *testing.T) {
	// A ZWJ family sequence is many runes but one grapheme cluster.
	const family = "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466"
	scr := render(t, 20, 2, family+"X")

	got := runesAt(t, scr, 0, 0)
	if len(got) < 2 {
		t.Fatalf("cell (0,0) has %d runes, want the whole ZWJ cluster", len(got))
	}
	if string(got) != family {
		t.Errorf("cell (0,0) = %q, want the full cluster %q", string(got), family)
	}
	// The cluster occupies two cells, so X follows at column 2.
	if got := runesAt(t, scr, 2, 0); len(got) == 0 || got[0] != 'X' {
		t.Errorf("cell (2,0) = %q, want 'X'", string(got))
	}
}

func TestCombiningMark(t *testing.T) {
	// "e" + combining acute is two runes, one cluster, one cell.
	scr := render(t, 20, 2, "éX")

	got := runesAt(t, scr, 0, 0)
	if len(got) != 2 || got[0] != 'e' || got[1] != '́' {
		t.Errorf("cell (0,0) = %v, want ['e','\\u0301'] in one cell", got)
	}
	// The mark must not consume a cell of its own.
	if got := runesAt(t, scr, 1, 0); len(got) == 0 || got[0] != 'X' {
		t.Errorf("cell (1,0) = %q, want 'X' immediately after", string(got))
	}
}

func TestClipping(t *testing.T) {
	t.Run("horizontal", func(t *testing.T) {
		// A 20-cell screen but a 4-cell drawing rectangle: the string must be
		// clipped to the rectangle, not to the screen.
		scr := tcell.NewSimulationScreen("UTF-8")
		if err := scr.Init(); err != nil {
			t.Fatalf("init: %v", err)
		}
		t.Cleanup(scr.Fini)
		scr.SetSize(20, 3)
		scr.Clear()
		Draw(scr, 0, 0, 4, 3, "abcdefghij")
		scr.Show()

		if got := rowText(t, scr, 0); got != "abcd" {
			t.Errorf("clipped row = %q, want %q", got, "abcd")
		}
	})

	t.Run("vertical", func(t *testing.T) {
		scr := tcell.NewSimulationScreen("UTF-8")
		if err := scr.Init(); err != nil {
			t.Fatalf("init: %v", err)
		}
		t.Cleanup(scr.Fini)
		scr.SetSize(20, 5)
		scr.Clear()
		Draw(scr, 0, 0, 20, 2, "one\ntwo\nthree")
		scr.Show()

		if got := rowText(t, scr, 0); got != "one" {
			t.Errorf("row 0 = %q", got)
		}
		if got := rowText(t, scr, 1); got != "two" {
			t.Errorf("row 1 = %q", got)
		}
		if got := rowText(t, scr, 2); got != "" {
			t.Errorf("row 2 = %q, want empty (clipped)", got)
		}
	})

	t.Run("wide glyph straddling right edge is dropped", func(t *testing.T) {
		scr := tcell.NewSimulationScreen("UTF-8")
		if err := scr.Init(); err != nil {
			t.Fatalf("init: %v", err)
		}
		t.Cleanup(scr.Fini)
		scr.SetSize(20, 3)
		scr.Clear()
		// Width 3: "a" then a wide glyph needing columns 1-2 fits; the next
		// wide glyph would straddle and must be dropped.
		Draw(scr, 0, 0, 3, 3, "a日本")
		scr.Show()

		if got := runesAt(t, scr, 0, 0); len(got) == 0 || got[0] != 'a' {
			t.Errorf("cell (0,0) = %q, want 'a'", string(got))
		}
		if got := runesAt(t, scr, 1, 0); len(got) == 0 || got[0] != '日' {
			t.Errorf("cell (1,0) = %q, want 日", string(got))
		}
		if got := runesAt(t, scr, 3, 0); len(got) > 0 && got[0] == '本' {
			t.Error("glyph past the right edge was drawn")
		}
	})
}

func TestUnknownSequencesAreSkipped(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"cursor move", "a\x1b[5Cb", "ab"},
		{"osc title", "a\x1b]0;window title\x07b", "ab"},
		{"unknown csi", "a\x1b[?25lb", "ab"},
		{"unknown escape", "a\x1b(Bb", "ab"},
		{"bare escape at end", "ab\x1b", "ab"},
		{"dcs", "a\x1bP1$r\x1b\\b", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := render(t, 20, 2, tt.in)
			if got := rowText(t, scr, 0); got != tt.want {
				t.Errorf("row = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDrawDoesNotPanic(t *testing.T) {
	// Malformed and hostile input must not panic or hang.
	inputs := []string{
		"",
		"\x1b",
		"\x1b[",
		"\x1b[38;2;",
		"\x1b[999999999999999999m",
		"\x1b]",
		"\xff\xfe invalid utf8",
		strings.Repeat("\x1b[31m", 1000) + "x",
		"\x00\x01\x02",
	}
	for _, in := range inputs {
		scr := tcell.NewSimulationScreen("UTF-8")
		if err := scr.Init(); err != nil {
			t.Fatalf("init: %v", err)
		}
		scr.SetSize(20, 5)
		Draw(scr, 0, 0, 20, 5, in)
		scr.Show()
		scr.Fini()
	}
}

func TestDrawDegenerateRects(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(20, 5)

	// None of these should panic or draw anything.
	Draw(scr, 0, 0, 0, 5, "x")
	Draw(scr, 0, 0, 5, 0, "x")
	Draw(scr, 0, 0, -1, -1, "x")
	Draw(nil, 0, 0, 5, 5, "x")
	scr.Show()

	if got := rowText(t, scr, 0); got != "" {
		t.Errorf("row 0 = %q, want empty", got)
	}
}

func TestSize(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		wantW int
		wantH int
	}{
		{"empty", "", 0, 0},
		{"plain", "hello", 5, 1},
		{"multiline ragged", "one\nthree\nxx", 5, 3},
		{"styled text ignores escapes", "\x1b[31mred\x1b[0m", 3, 1},
		{"cjk counts double", "日本", 4, 1},
		{"emoji counts double", "🙂", 2, 1},
		{"combining counts once", "é", 1, 1},
		{"trailing newline", "a\n", 1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := Size(tt.in)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("Size() = (%d,%d), want (%d,%d)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestSizeAgreesWithLipgloss(t *testing.T) {
	cases := []string{
		"plain",
		lipgloss.NewStyle().Bold(true).Render("bold text"),
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render("boxed"),
		lipgloss.NewStyle().Padding(1, 2).Render("padded"),
		lipgloss.JoinHorizontal(lipgloss.Top, "left", "right"),
		lipgloss.NewStyle().Width(20).Render("fixed width"),
	}

	for _, c := range cases {
		gotW, gotH := Size(c)
		wantW, wantH := lipgloss.Width(c), lipgloss.Height(c)
		if gotW != wantW || gotH != wantH {
			t.Errorf("Size(%q) = (%d,%d), lipgloss says (%d,%d)",
				c, gotW, gotH, wantW, wantH)
		}
	}
}

func TestSizeMatchesDrawnExtent(t *testing.T) {
	// Size must not disagree with what Draw actually puts on screen.
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		Render("content")

	w, h := Size(box)
	scr := render(t, w+10, h+5, box)

	// The rightmost column of the box must be occupied on its top row.
	if got := runesAt(t, scr, w-1, 0); len(got) == 0 || got[0] == ' ' {
		t.Errorf("column w-1=%d of row 0 is empty; Size overstates width", w-1)
	}
	// One past it must be empty.
	if got := runesAt(t, scr, w, 0); len(got) > 0 && got[0] != ' ' && got[0] != 0 {
		t.Errorf("column w=%d of row 0 = %q; Size understates width", w, string(got))
	}
	// The bottom row of the box must be occupied.
	if got := runesAt(t, scr, 0, h-1); len(got) == 0 || got[0] == ' ' {
		t.Errorf("row h-1=%d is empty; Size overstates height", h-1)
	}
}

func TestJoinHorizontalPanes(t *testing.T) {
	// The composition the real chrome depends on: two bordered panes joined.
	left := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(6).Render("L")
	right := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(6).Render("R")
	joined := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	w, h := Size(joined)
	scr := render(t, w+4, h+2, joined)

	if got := runesAt(t, scr, 0, 0); len(got) == 0 || got[0] != '╭' {
		t.Errorf("left pane corner = %q, want ╭", string(got))
	}
	// The right pane starts immediately after the left pane's width.
	lw, _ := Size(left)
	if got := runesAt(t, scr, lw, 0); len(got) == 0 || got[0] != '╭' {
		t.Errorf("right pane corner at x=%d = %q, want ╭", lw, string(got))
	}
}

func TestSyncLipglossProfile(t *testing.T) {
	// Restore whatever TestMain set, so ordering cannot leak between tests.
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.TrueColor) })

	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)

	SyncLipglossProfile(scr)

	// The simulation screen reports a real colour count; whatever it is, the
	// profile must end up non-Ascii so styling survives, and styled output must
	// actually carry escape sequences.
	out := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("x")
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("after sync, styled render = %q, want ANSI escapes (screen reports %d colors)",
			out, scr.Colors())
	}

	// A nil screen must be a no-op rather than a panic.
	SyncLipglossProfile(nil)
}
