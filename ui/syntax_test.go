package ui

import (
	"testing"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// spansOn builds a SpansFunc from a per-line table, so a test states exactly
// what the lexer would have said without depending on a real grammar.
func spansOn(table map[int][]syntax.Span) SpansFunc {
	return func(_ *text.Buffer, line int) []syntax.Span { return table[line] }
}

// drawSyntax renders one window with highlighting and the gutter off, so a
// column in the assertion is a column in the line.
func drawSyntax(t *testing.T, w, h int, b *text.Buffer, spans SpansFunc) tcell.SimulationScreen {
	t.Helper()
	win := view.NewWindow(b)
	th := DefaultTheme()
	th.LineNumbers = false
	scr := sim(t, w, h)
	Render(scr, Frame{
		Tree:    view.NewTree(win),
		Active:  win,
		SpansOf: spans,
	}, th)
	scr.Show()
	return scr
}

// fgAt is the foreground colour of one cell, which is what a syntax style sets.
func fgAt(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.Color {
	t.Helper()
	fg, _, _ := cellAt(t, scr, x, y).Style.Decompose()
	return fg
}

// Each class must reach the screen as its own colour. Without this the whole
// feature is invisible: the lexer can be perfect and the code still renders
// flat.
func TestEachClassGetsItsColour(t *testing.T) {
	th := DefaultTheme()
	b := bufferOf(t, "func x() {}")

	for _, tc := range []struct {
		name  string
		class syntax.Class
	}{
		{"keyword", syntax.Keyword},
		{"string", syntax.String},
		{"comment", syntax.Comment},
		{"number", syntax.Number},
		{"function", syntax.Function},
		{"type", syntax.Type},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scr := drawSyntax(t, 20, 4, b, spansOn(map[int][]syntax.Span{
				0: {{Start: 0, End: 4, Class: tc.class}},
			}))
			want, _, _ := th.SyntaxStyle[tc.class].Decompose()
			if got := fgAt(t, scr, 0, 0); got != want {
				t.Errorf("foreground = %v, want %v for %s", got, want, tc.class)
			}
			if want == tcell.ColorDefault {
				t.Errorf("%s has no colour in the default theme", tc.class)
			}
		})
	}
}

// Text no span covers keeps the terminal's own foreground. A theme that
// coloured plain text would override whatever the user chose.
func TestPlainTextKeepsTheTerminalForeground(t *testing.T) {
	b := bufferOf(t, "func x")
	scr := drawSyntax(t, 20, 4, b, spansOn(map[int][]syntax.Span{
		0: {{Start: 0, End: 4, Class: syntax.Keyword}},
	}))
	// Column 5 is "x", outside the span.
	if got := fgAt(t, scr, 5, 0); got != tcell.ColorDefault {
		t.Errorf("plain text foreground = %v, want the terminal default", got)
	}
}

// The user chose a look that never paints a background, so nem sits inside
// their terminal theme instead of replacing it. Syntax colour must not be the
// thing that breaks that promise.
func TestSyntaxNeverPaintsABackground(t *testing.T) {
	b := bufferOf(t, `x := "hi" // 42`)
	scr := drawSyntax(t, 30, 4, b, spansOn(map[int][]syntax.Span{
		0: {
			{Start: 0, End: 1, Class: syntax.Function},
			{Start: 5, End: 9, Class: syntax.String},
			{Start: 10, End: 15, Class: syntax.Comment},
		},
	}))
	for x := 0; x < 15; x++ {
		if _, bg, _ := cellAt(t, scr, x, 0).Style.Decompose(); bg != tcell.ColorDefault {
			t.Errorf("cell %d has background %v; syntax must set foreground only", x, bg)
		}
	}
}

// A block comment spans lines, and every line it covers must be coloured. This
// is the case that separates a real highlighter from one that only works on
// single-line tokens.
func TestMultiLineCommentColoursEveryLine(t *testing.T) {
	th := DefaultTheme()
	b := bufferOf(t, "/* one", "   two", "   three */")
	scr := drawSyntax(t, 20, 6, b, spansOn(map[int][]syntax.Span{
		0: {{Start: 0, End: 6, Class: syntax.Comment}},
		1: {{Start: 0, End: 6, Class: syntax.Comment}},
		2: {{Start: 0, End: 11, Class: syntax.Comment}},
	}))
	want, _, _ := th.SyntaxStyle[syntax.Comment].Decompose()
	for y := 0; y < 3; y++ {
		if got := fgAt(t, scr, 0, y); got != want {
			t.Errorf("line %d foreground = %v, want the comment colour %v", y, got, want)
		}
	}
}

// Spans are rune indices; drawing is in display columns. A wide glyph makes
// those disagree, and applying a span at its rune index paints the wrong
// characters from there on.
func TestSpansLandOnTheRightColumnsAfterAWideGlyph(t *testing.T) {
	th := DefaultTheme()
	// Runes:   0=日 1=本 2=space 3..6=func
	// Columns: 0,1=日  2,3=本   4=space  5..8=func
	b := bufferOf(t, "日本 func")
	scr := drawSyntax(t, 20, 4, b, spansOn(map[int][]syntax.Span{
		0: {{Start: 3, End: 7, Class: syntax.Keyword}},
	}))
	want, _, _ := th.SyntaxStyle[syntax.Keyword].Decompose()

	// Every column of the word, not just its first. Looking spans up by display
	// column instead of rune index still colours the start correctly here - the
	// error only shows at the far end, where the four-rune span has run out two
	// columns early, and on the space it wrongly swallows.
	for _, col := range []int{5, 6, 7, 8} {
		if got := fgAt(t, scr, col, 0); got != want {
			t.Errorf("column %d of func = %v, want the keyword colour %v", col, got, want)
		}
	}
	for _, tc := range []struct {
		col  int
		what string
	}{
		{0, "日"},
		{2, "本"},
		{4, "the space before func"},
	} {
		if got := fgAt(t, scr, tc.col, 0); got != tcell.ColorDefault {
			t.Errorf("column %d (%s) = %v, want plain - the span covers runes 3..6",
				tc.col, tc.what, got)
		}
	}
}

// Scrolled right, a span must still land on its own characters.
func TestSpansFollowHorizontalScroll(t *testing.T) {
	th := DefaultTheme()
	b := bufferOf(t, "0123456789keyword")
	win := view.NewWindow(b)
	win.LeftCol = 10
	win.Pt = text.Pos{Line: 0, Col: 16}

	tth := DefaultTheme()
	tth.LineNumbers = false
	scr := sim(t, 12, 4)
	Render(scr, Frame{
		Tree:   view.NewTree(win),
		Active: win,
		SpansOf: spansOn(map[int][]syntax.Span{
			0: {{Start: 10, End: 17, Class: syntax.Keyword}},
		}),
	}, tth)
	scr.Show()

	want, _, _ := th.SyntaxStyle[syntax.Keyword].Decompose()
	if got := fgAt(t, scr, 0, 0); got != want {
		t.Errorf("first visible column = %v, want the keyword colour %v", got, want)
	}
}

// A cell can be syntax-coloured, selected and on a matched bracket at once.
// Each must contribute: colour from syntax, inversion from the region, weight
// from the bracket.
func TestSyntaxRegionAndBracketCompose(t *testing.T) {
	b := bufferOf(t, "f(x)")
	b.SetMark(text.Pos{Line: 0, Col: 0})
	win := view.NewWindow(b)
	win.Pt = text.Pos{Line: 0, Col: 4} // region covers the whole line

	th := DefaultTheme()
	th.LineNumbers = false
	scr := sim(t, 20, 4)
	Render(scr, Frame{
		Tree:   view.NewTree(win),
		Active: win,
		SpansOf: spansOn(map[int][]syntax.Span{
			0: {{Start: 0, End: 1, Class: syntax.Function}},
		}),
	}, th)
	scr.Show()

	// Column 1 is "(", a matched bracket inside the region.
	_, _, attr := cellAt(t, scr, 1, 0).Style.Decompose()
	if attr&tcell.AttrReverse == 0 {
		t.Error("bracket cell is not reversed; the region did not compose")
	}
	if attr&tcell.AttrBold == 0 {
		t.Error("bracket cell is not bold; the bracket highlight did not compose")
	}

	// Column 0 is "f": syntax-coloured and selected.
	fg, _, attr0 := cellAt(t, scr, 0, 0).Style.Decompose()
	wantFg, _, _ := th.SyntaxStyle[syntax.Function].Decompose()
	if fg != wantFg {
		t.Errorf("function cell foreground = %v, want %v kept under the region", fg, wantFg)
	}
	if attr0&tcell.AttrReverse == 0 {
		t.Error("function cell is not reversed; the region did not compose over syntax")
	}
}

// Turning highlighting off must restore the previous rendering exactly, so the
// setting is a real escape hatch rather than a different kind of colour.
func TestSyntaxOffRendersLikeBefore(t *testing.T) {
	b := bufferOf(t, "func x() {}")
	spans := spansOn(map[int][]syntax.Span{
		0: {{Start: 0, End: 4, Class: syntax.Keyword}},
	})

	win := view.NewWindow(b)
	th := DefaultTheme()
	th.LineNumbers = false
	th.Syntax = false
	scr := sim(t, 20, 4)
	Render(scr, Frame{Tree: view.NewTree(win), Active: win, SpansOf: spans}, th)
	scr.Show()

	if got := fgAt(t, scr, 0, 0); got != tcell.ColorDefault {
		t.Errorf("foreground = %v with Syntax off, want the terminal default", got)
	}
}

// A nil SpansOf is the no-highlighting case, and ui must not require the field.
func TestNilSpansFuncDrawsPlainText(t *testing.T) {
	b := bufferOf(t, "func x() {}")
	scr := drawSyntax(t, 20, 4, b, nil)
	if got := fgAt(t, scr, 0, 0); got != tcell.ColorDefault {
		t.Errorf("foreground = %v with no SpansOf, want the terminal default", got)
	}
}

// A span reaching past the end of a line must not panic or colour past it.
// Spans arrive from another package, so ui cannot assume they are in range.
func TestOutOfRangeSpansAreHarmless(t *testing.T) {
	b := bufferOf(t, "ab")
	scr := drawSyntax(t, 20, 4, b, spansOn(map[int][]syntax.Span{
		0: {{Start: 0, End: 99, Class: syntax.Keyword}},
		1: {{Start: 5, End: 9, Class: syntax.String}},
	}))
	if got := cellAt(t, scr, 0, 0).Runes[0]; got != 'a' {
		t.Errorf("first cell = %q, want the text drawn", got)
	}
}
