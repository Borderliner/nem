package ui

import (
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Region highlighting is computed here, from point and the mark, at draw time -
// the same arrangement as bracket matching in paren.go, and for the same reason:
// Env deliberately cannot reach the screen, so a command has nowhere to report a
// highlight to.
//
// It is drawn for the active window only. The mark lives on the buffer, but
// emacs shows the region in the selected window, and painting a selection into a
// second window onto the same buffer would claim something the user did not do
// there.

// regionSpan is the frame's selection: an ordered pair of buffer positions, or
// none at all.
type regionSpan struct {
	lo, hi text.Pos
	on     bool
	style  tcell.Style
}

// regionHL is the part of the selection falling on one line: a half-open range
// of rune indices, plus whether the selection continues past the line's text.
//
// A range rather than the fixed array paren.go uses, because a selection is a
// span and can cover a whole line - the two highlights are different shapes and
// share only the draw path they meet in.
type regionHL struct {
	on    bool
	from  text.RuneIdx // inclusive
	to    text.RuneIdx // exclusive
	toEOL bool         // selection continues onto the next line
	style tcell.Style
}

// covers reports whether the cluster spanning [start, end) is selected.
//
// Overlap rather than containment of the start index: a grapheme cannot be half
// selected, so a boundary landing inside a cluster selects the whole cluster.
// Point only ever sits on a cluster boundary, but the mark can be left mid
// cluster by an edit, and the alternative is a visibly torn glyph.
func (r regionHL) covers(start, end text.RuneIdx) bool {
	return r.on && start < r.to && end > r.from
}

// regionFor computes the selection for a window whose point is at pt.
//
// An empty region - point and mark at the same place - highlights nothing, which
// is what makes C-SPC followed by no movement look like nothing happened rather
// than like a one-cell selection.
func regionFor(b *text.Buffer, pt text.Pos, active bool, th Theme) regionSpan {
	if !active || b == nil || b.NumLines() == 0 || !b.HasMark() {
		return regionSpan{}
	}
	lo, hi := text.OrderPos(b.ClampPos(pt), b.ClampPos(b.Mark()))
	if lo == hi {
		return regionSpan{}
	}
	return regionSpan{lo: lo, hi: hi, on: true, style: th.Region}
}

// onLine narrows the selection to one buffer line. A line outside the span
// returns a zero regionHL, so drawing needs no special case for a selection
// scrolled off screen.
func (s regionSpan) onLine(line int, l *text.Line) regionHL {
	if !s.on || l == nil || line < s.lo.Line || line > s.hi.Line {
		return regionHL{}
	}

	out := regionHL{on: true, style: s.style, from: 0, to: l.Len()}
	if line == s.lo.Line {
		out.from = s.lo.Col
	}
	if line == s.hi.Line {
		out.to = s.hi.Col
	} else {
		// The selection runs on past this line's text, so it is filled to the
		// window's right edge and the block reads as continuous rather than
		// ragged against the ends of the lines.
		out.toEOL = true
	}
	if out.to < out.from {
		out.to = out.from
	}
	return out
}

// overlay applies over's colours and attributes on top of base.
//
// Two tcell styles cannot be combined through the builder API, so this takes
// them apart. Decompose is deprecated on the grounds that (fg, bg, attrs) does
// not fully describe a style - a URL is excluded - which is true and irrelevant
// to a selection, so the deprecation is accepted deliberately here rather than
// worked around with a theme the render path has to know the contents of.
//
// Attributes are OR-ed, which is what gives the region precedence on the
// background while leaving a bracket match its weight and underline: the two
// highlights compose instead of one winning outright.
func overlay(base, over tcell.Style) tcell.Style {
	ofg, obg, oattr := over.Decompose()
	_, _, battr := base.Decompose()

	out := base
	if ofg != tcell.ColorDefault {
		out = out.Foreground(ofg)
	}
	if obg != tcell.ColorDefault {
		out = out.Background(obg)
	}
	if oattr != 0 {
		out = out.Attributes(battr | oattr)
	}
	return out
}
