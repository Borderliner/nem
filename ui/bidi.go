package ui

import (
	"os"
	"sort"
	"strconv"

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
	rs := l.View()
	dir := bidi.LTR
	if auto {
		dir = bidi.ParagraphDirection(rs)
	}
	levels := bidi.Levels(rs, dir)
	shaped := bidi.Shape(rs)

	clusters := make([]text.Cluster, 0, len(rs))
	for c := range l.Clusters() {
		clusters = append(clusters, c)
	}
	lv := make([]uint8, len(clusters))
	for i, c := range clusters {
		lv[i] = levels[c.Start]
	}

	lay := bidiLayout{rtl: dir == bidi.RTL, cells: make([]bidiCell, 0, len(clusters))}
	for _, i := range bidi.Order(lv) {
		c := clusters[i]
		glyphs := shaped[c.Start : int(c.Start)+len(c.Runes)]
		if lv[i]%2 == 1 {
			if m, ok := bidi.Mirror(glyphs[0]); ok {
				glyphs = append([]rune{m}, glyphs[1:]...)
			}
		}
		lay.cells = append(lay.cells, bidiCell{c: c, x: lay.width, glyphs: glyphs})
		lay.width += c.Width
	}
	return lay
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

	off := lay.origin(avail, left)
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
	if truncated {
		scr.SetContent(x+width-1, y, TruncMarker, nil, th.Trunc)
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
// itself, so nem must not: Konsole, mlterm, and terminals built on VTE since
// its version 0.58, such as GNOME Terminal. Laid out twice, text comes out
// backwards again.
func TerminalDoesBidi() bool {
	if os.Getenv("KONSOLE_VERSION") != "" || os.Getenv("MLTERM") != "" {
		return true
	}
	if v, err := strconv.Atoi(os.Getenv("VTE_VERSION")); err == nil && v >= 5800 {
		return true
	}
	return false
}
