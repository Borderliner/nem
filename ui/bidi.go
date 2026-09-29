package ui

import (
	"os"
	"sort"

	"github.com/Borderliner/nem/bidi"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Right-to-left text. A line holding Persian, Arabic or Hebrew is drawn in
// the order a reader reads it, not the order it is stored: the Unicode
// Bidirectional Algorithm decides which runs go right to left, and the
// clusters are laid out left to right in that visual order. Arabic-script
// letters are drawn in their joined forms, since most terminals draw each
// cell alone and never join them.
//
// In prose each line takes its direction from its first letter, and a line
// that goes right to left sits against the window's right edge, as emacs
// sets it. In code, and in listings and prompts, every line goes left to
// right, as emacs keeps it in programming modes: a Persian comment is still
// written right to left, but the line stays where the code is.
//
// Only the drawing changes. The buffer, point, the region and every command
// work on the text in its stored order, as they do in emacs; a line with no
// right-to-left letter in it takes none of this path.

// bidiCell is one cluster of a laid-out line: where it is drawn, counted from
// the start of the line's visual extent, and what is drawn.
type bidiCell struct {
	c      text.Cluster
	x      text.ColIdx
	glyphs []rune // the cluster's runes as shown: joined, and mirrored at an odd level
}

// bidiLayout is a line laid out in visual order.
type bidiLayout struct {
	cells []bidiCell // left to right
	width text.ColIdx
	rtl   bool // the line's own direction is right to left
}

// usesBidi reports whether text needs laying out as right-to-left text.
func (th Theme) usesBidi(rs []rune) bool { return th.Bidi && bidi.HasRTL(rs) }

// lineUsesBidi is usesBidi for a buffer's line, which knows without looking
// at every rune when they are all ASCII - as a line of code nearly always is.
//
// A line longer than maxBidiLine is drawn as stored, as emacs gives up the
// same work on long lines: laying out a line means every one of its
// clusters, however few fit the window, and a line that long is data, not a
// paragraph anyone reads.
func (th Theme) lineUsesBidi(l *text.Line) bool {
	return th.Bidi && len(l.View()) <= maxBidiLine && !l.ASCII() && bidi.HasRTL(l.View())
}

// maxBidiLine is the longest line laid out right to left.
const maxBidiLine = 10000

// layoutBidi lays l out in visual order: its direction its own when auto,
// left to right otherwise.
func layoutBidi(l *text.Line, auto bool) bidiLayout {
	return layoutBidiRows(l, []text.RuneIdx{0}, auto)[0]
}

// layoutBidiRows lays l out a row at a time, for a line wrapped into rows
// beginning at starts. The levels are the whole line's - a paragraph is one
// paragraph however it is folded - and each row is put in visual order on
// its own, as the Unicode Bidirectional Algorithm orders a paragraph broken
// into lines.
func layoutBidiRows(l *text.Line, starts []text.RuneIdx, auto bool) []bidiLayout {
	rs := l.View()
	dir := bidi.LTR
	if auto {
		dir = bidi.ParagraphDirection(rs)
	}
	levels := bidi.Levels(rs, dir)
	shaped := bidi.Shape(rs)

	lays := make([]bidiLayout, len(starts))
	row := make([]text.Cluster, 0, len(rs)/len(starts)+8)
	k := 0
	order := func() {
		// A row that wraps ends in the blanks it broke after, which hang
		// past the edge. Laid out, they would widen the row past the window,
		// and in a row going right to left push its first letter off the
		// right edge; they are left out, as the algorithm lets trailing
		// whitespace be.
		if k < len(starts)-1 {
			for len(row) > 0 && isBlank(row[len(row)-1]) {
				row = row[:len(row)-1]
			}
		}
		lv := make([]uint8, len(row))
		for i, c := range row {
			lv[i] = levels[c.Start]
		}
		lay := bidiLayout{rtl: dir == bidi.RTL, cells: make([]bidiCell, 0, len(row))}
		for _, i := range bidi.Order(lv) {
			c := row[i]
			glyphs := shaped[c.Start : int(c.Start)+len(c.Runes)]
			if lv[i]%2 == 1 {
				if m, ok := bidi.Mirror(glyphs[0]); ok {
					glyphs = append([]rune{m}, glyphs[1:]...)
				}
			}
			lay.cells = append(lay.cells, bidiCell{c: c, x: lay.width, glyphs: glyphs})
			lay.width += c.Width
		}
		lays[k] = lay
		row = row[:0]
	}
	for c := range l.Clusters() {
		for k+1 < len(starts) && c.Start >= starts[k+1] {
			order()
			k++
		}
		row = append(row, c)
	}
	for ; k < len(starts); k++ {
		order()
	}
	return lays
}

// origin is the screen column, within a text area avail wide and scrolled
// left columns, where the layout's first cell goes: a right-to-left line
// that fits is set against the right edge.
func (lay bidiLayout) origin(avail, left text.ColIdx) text.ColIdx {
	if lay.rtl && left == 0 && lay.width <= avail {
		return avail - lay.width
	}
	return -left
}

// cursorAt is the visual column of the cursor at rune index col, from the
// layout's origin: on the cluster holding col, or past the text's end - which
// for a right-to-left line is at its left.
func (lay bidiLayout) cursorAt(col text.RuneIdx) text.ColIdx {
	for _, cell := range lay.cells {
		if col >= cell.c.Start && col < cell.c.Start+text.RuneIdx(len(cell.c.Runes)) {
			return cell.x
		}
	}
	if lay.rtl {
		return -1
	}
	return lay.width
}

// spanClassAt is the class of the span covering rune r, for a walk that does
// not go through the line in order.
func spanClassAt(spans []syntax.Span, r int) (syntax.Class, bool) {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].End > r })
	if i < len(spans) && spans[i].Start <= r {
		return spans[i].Class, true
	}
	return 0, false
}

// drawLineBidi is drawLine for a line holding right-to-left text. See
// drawLine for what hl, reg, spans and row are.
func drawLineBidi(scr tcell.Screen, x, y, width int, l *text.Line, left text.ColIdx, th Theme, hl lineHL, reg regionHL, spans []syntax.Span, row tcell.Style, auto bool) {
	lay := layoutBidi(l, auto)
	avail := text.ColIdx(width)
	truncated := lay.width-left > avail
	if truncated {
		avail--
	}
	if avail <= 0 {
		return
	}

	// The row's background first, all of it: a right-to-left line leaves its
	// empty part on the left, and a selection running on to the next line
	// fills what the text does not.
	fill := row
	if reg.on && reg.toEOL {
		fill = overlay(row, reg.style)
	}
	if fill != tcell.StyleDefault {
		for sx := text.ColIdx(0); sx < avail; sx++ {
			scr.SetContent(x+int(sx), y, ' ', nil, fill)
		}
	}

	drawBidiCells(scr, x, y, avail, lay.origin(avail, left), lay, th, hl, reg, spans, row)
	if truncated {
		scr.SetContent(x+width-1, y, TruncMarker, nil, th.Trunc)
	}
}

// drawBidiCells draws a layout's cells from column off of a text area avail
// wide at x, leaving out any that would cross its edges.
func drawBidiCells(scr tcell.Screen, x, y int, avail, off text.ColIdx, lay bidiLayout, th Theme, hl lineHL, reg regionHL, spans []syntax.Span, row tcell.Style) {
	for _, cell := range lay.cells {
		c := cell.c
		sx := off + cell.x
		if sx < 0 || sx+c.Width > avail {
			continue
		}
		style := th.Text
		if cl, ok := spanClassAt(spans, int(c.Start)); ok && int(cl) < numSyntaxClasses {
			style = th.SyntaxStyle[cl]
		}
		style = parenOver(hl, c.Start, style, th.Text)
		style = overlay(style, row)
		if reg.covers(c.Start, c.Start+text.RuneIdx(len(c.Runes))) {
			style = overlay(style, reg.style)
		}
		if c.Runes[0] == '\t' {
			for k := text.ColIdx(0); k < c.Width; k++ {
				scr.SetContent(x+int(sx+k), y, ' ', nil, style)
			}
			continue
		}
		scr.SetContent(x+int(sx), y, cell.glyphs[0], cell.glyphs[1:], style)
	}
}

// bidiCursorCol is where the cursor at rune index col of l goes, as a column
// of a text area avail wide scrolled left columns.
func bidiCursorCol(l *text.Line, col text.RuneIdx, avail, left text.ColIdx, auto bool) int {
	lay := layoutBidi(l, auto)
	if lay.width-left > avail {
		avail--
	}
	return int(lay.origin(avail, left) + lay.cursorAt(col))
}

// bidiString is s as it is drawn - joined and in visual order - for text
// drawn whole, such as the prompt row, whose own direction is left to right.
// pos maps a rune index of s to the display column it is drawn at.
func bidiString(s string) (string, func(text.RuneIdx) int) {
	l := text.NewLine([]rune(s))
	lay := layoutBidi(&l, false)
	var out []rune
	for _, cell := range lay.cells {
		out = append(out, cell.glyphs...)
	}
	return string(out), func(r text.RuneIdx) int { return int(lay.cursorAt(r)) }
}

// TerminalDoesBidi reports whether the terminal lays out right-to-left text
// itself whatever it is told, so nem must not: laid out twice, text comes out
// backwards again. These are Konsole, mlterm and macOS's Terminal: each
// reorders a line as a left-to-right paragraph, so a right-to-left line
// stays against the left edge, and none can be asked to stop.
//
// A terminal that follows the BiDi recommendation for terminal emulators -
// VTE's, as in GNOME Terminal, Tilix and Xfce's, and mintty - reorders too,
// but is told not to while nem runs: see Screen.TakeBidi. nem then lays the
// text out itself, set against the right edge, as it does in a terminal that
// never reorders. Handing it to them instead left every right-to-left line
// against the left edge; and where VTE's variable did not reach nem - over
// ssh, under sudo - nem laid the text out and VTE reversed it again.
func TerminalDoesBidi() bool {
	return os.Getenv("KONSOLE_VERSION") != "" || os.Getenv("MLTERM") != "" ||
		os.Getenv("TERM_PROGRAM") == "Apple_Terminal"
}

// isBlank reports whether c is a space or a tab.
func isBlank(c text.Cluster) bool { return c.Runes[0] == ' ' || c.Runes[0] == '\t' }
