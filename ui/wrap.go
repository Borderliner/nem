package ui

import (
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// Wrapped lines. With Theme.Wrap a line wider than its window is folded into
// rows of the window's width - where they break is text.Line.WrapRows - and
// the window is drawn a row at a time from view.Window's Top and TopRow. The
// line's number goes beside its first row only, and the band under the
// current line runs under all of its rows. Without Wrap a row is a whole
// line, cut off at the edge.

// screenRow is one row of a window's text area: the line drawn on it, and,
// when lines are wrapped, the part of the line - the runes from from to to,
// starting at column col. first and last say whether it is the line's first
// and last row.
type screenRow struct {
	line        int
	index       int // which of the line's rows
	from, to    text.RuneIdx
	col         text.ColIdx
	first, last bool
}

// screenRows is what each row of win's text area shows, height rows of a
// text area width columns wide, from the window's top.
func screenRows(win *view.Window, height, width int, wrap bool) []screenRow {
	b := win.Buf
	out := make([]screenRow, 0, height)
	if !wrap {
		for ln := win.Top; ln < b.NumLines() && len(out) < height; ln++ {
			out = append(out, screenRow{line: ln, to: text.RuneIdx(b.Line(ln).Len()), first: true, last: true})
		}
		return out
	}
	for r := (view.RowPos{Line: win.Top, Row: win.TopRow}); r.Line < b.NumLines() && len(out) < height; r = (view.RowPos{Line: r.Line + 1}) {
		n := len(b.Line(r.Line).WrapRows(text.ColIdx(width)))
		for ; r.Row < n && len(out) < height; r.Row++ {
			from, to, col := view.RowSpan(b, r, width)
			out = append(out, screenRow{line: r.Line, index: r.Row, from: from, to: to, col: col, first: r.Row == 0, last: r.Row == n-1})
		}
	}
	return out
}

// linesShown is how many buffer lines rows show, for forgetting the layout of
// the lines that have scrolled out of them.
func linesShown(rows []screenRow) int {
	if len(rows) == 0 {
		return 0
	}
	return rows[len(rows)-1].line - rows[0].line + 1
}

// drawRow draws one row of a wrapped line: its clusters from the row's start
// column, the band under the rest of the row, and, where the selection runs
// on past the row, the selection too. See drawLine for hl, reg, spans and
// row.
func drawRow(scr tcell.Screen, x, y, width int, l *text.Line, r screenRow, th Theme, hl lineHL, reg regionHL, spans []syntax.Span, row tcell.Style) {
	avail := text.ColIdx(width)
	syn := newLineSyntax(spans, &th)
	for c := range l.ClustersFrom(r.col) {
		if c.Start >= r.to {
			break
		}
		if c.Start < r.from {
			continue
		}
		sx := c.Col - r.col
		if sx >= avail {
			break // a blank hanging past the edge
		}
		style := syn.styleAt(c.Start, th.Text)
		style = parenOver(hl, c.Start, style, th.Text)
		style = overlay(style, row)
		if reg.covers(c.Start, c.Start+text.RuneIdx(len(c.Runes))) {
			style = overlay(style, reg.style)
		}
		switch {
		case c.Runes[0] == '\t':
			for k := text.ColIdx(0); k < c.Width && sx+k < avail; k++ {
				scr.SetContent(x+int(sx+k), y, ' ', nil, style)
			}
		case sx+c.Width > avail:
			// A glyph wider than the whole row: half of it cannot be drawn.
		default:
			scr.SetContent(x+int(sx), y, c.Runes[0], c.Runes[1:], style)
		}
	}
	fillRow(scr, x, y, avail, l.DisplayCol(r.to)-r.col, r, reg, row)
}

// fillRow lays the row's background from column from to the edge: the band
// under the current line, and the selection where it runs on past the row -
// on to the next line from its last row, or on to its next row.
func fillRow(scr tcell.Screen, x, y int, avail, from text.ColIdx, r screenRow, reg regionHL, row tcell.Style) {
	fill := row
	if reg.on && (r.last && reg.toEOL || !r.last && r.to > r.from && reg.covers(r.to-1, r.to)) {
		fill = overlay(row, reg.style)
	}
	if fill == tcell.StyleDefault {
		return
	}
	for sx := max(from, 0); sx < avail; sx++ {
		scr.SetContent(x+int(sx), y, ' ', nil, fill)
	}
}

// drawRowBidi is drawRow for a row of a line holding right-to-left text, laid
// out on its own by layoutBidiRows. A row of a line going right to left sits
// against the right edge, as the whole line does unwrapped.
func drawRowBidi(scr tcell.Screen, x, y, width int, lay bidiLayout, r screenRow, th Theme, hl lineHL, reg regionHL, spans []syntax.Span, row tcell.Style) {
	avail := text.ColIdx(width)
	fillRow(scr, x, y, avail, 0, r, reg, row)
	drawBidiCells(scr, x, y, avail, lay.origin(avail, 0), lay, th, hl, reg, spans, row)
}
