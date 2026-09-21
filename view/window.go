// Package view holds nem's view state: a Window (a buffer plus the cursor and
// viewport belonging to one view of it) and the tree of splits that arranges
// windows on screen.
//
// It imports text and the standard library, and nothing else. A window knows
// where point is and which lines are visible; the split tree knows how to
// divide a rectangle. Neither knows what a terminal is, which is why both stay
// testable without one and why this package is separate from ui.
package view

import "github.com/Borderliner/nem/text"

// GoalColUnset marks a window whose goal column has not been established. The
// goal column is set by horizontal motion and preserved by vertical motion, so
// moving down through a short line and out the other side returns to the
// original column.
const GoalColUnset text.ColIdx = -1

// Window is one view of a buffer: which buffer, where point is in it, and
// which lines are currently on screen.
//
// Point lives here rather than in the buffer because several windows may show
// the same buffer, each with its own cursor. The buffer keeps only the point
// to restore when a window next visits it.
type Window struct {
	Buf     *text.Buffer
	Pt      text.Pos
	Top     int         // first visible buffer line
	LeftCol text.ColIdx // leftmost visible display column
	GoalCol text.ColIdx // GoalColUnset when not established
}

// NewWindow returns a window showing b from its first line, with point at the
// origin and no goal column.
func NewWindow(b *text.Buffer) *Window {
	return &Window{Buf: b, GoalCol: GoalColUnset}
}

// Visit switches this window to b, saving point into the outgoing buffer so
// returning to it later lands where you left. This is switch-to-buffer's
// mechanism.
//
// Visiting the buffer already shown is a no-op, so it cannot lose the current
// position. Otherwise the incoming buffer's saved point is clamped into range,
// since the buffer may have shrunk since it was stored, and Top is set so
// point is visible without needing a ScrollToPoint first.
func (w *Window) Visit(b *text.Buffer) {
	if b == nil || w.Buf == b {
		return
	}
	if w.Buf != nil {
		w.Buf.SetSavePoint(w.Pt)
	}
	w.Buf = b
	w.Pt = b.ClampPos(b.SavePoint())
	w.Top = w.Pt.Line
	w.LeftCol = 0
	w.GoalCol = GoalColUnset
}

// ScrollToPoint adjusts Top so that point is visible in a text area of
// textHeight rows, keeping margin rows of context above and below it where the
// buffer allows.
//
// margin is clamped to (textHeight-1)/2: a margin of half the window or more
// would otherwise demand context on both sides that cannot both be satisfied,
// and the viewport would thrash on every vertical move. Top never goes
// negative and never scrolls past the last screenful of the buffer, so a
// buffer shorter than the viewport always sits at the top.
//
// It is the render path's job to call this before drawing; afterwards point is
// guaranteed to lie within [Top, Top+textHeight).
func (w *Window) ScrollToPoint(textHeight, margin int) {
	if textHeight < 1 {
		textHeight = 1
	}
	if margin < 0 {
		margin = 0
	}
	if max := (textHeight - 1) / 2; margin > max {
		margin = max
	}

	top := w.Top
	if w.Pt.Line < top+margin {
		top = w.Pt.Line - margin
	}
	if w.Pt.Line > top+textHeight-1-margin {
		top = w.Pt.Line - textHeight + 1 + margin
	}

	maxTop := w.Buf.NumLines() - textHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if top > maxTop {
		top = maxTop
	}
	if top < 0 {
		top = 0
	}
	w.Top = top
}

// ScrollToPointHorizontally adjusts LeftCol so point is visible in a text area
// textWidth columns wide. It is the horizontal twin of ScrollToPoint, and lives
// here for the same reason: a viewport is this window's state, not the
// renderer's.
//
// One column is reserved for the truncation marker whenever the line runs past
// the right edge. That reservation is viewport geometry rather than drawing,
// exactly as TextHeight reserving the modeline row is - the renderer is told how
// much room it has, and only decides what to put there.
//
// Afterwards point is guaranteed to lie within [LeftCol, LeftCol+textWidth).
//
// A single pass suffices, which is not obvious: whether the marker is needed
// depends on where we scroll to, and where we scroll to depends on the marker,
// so this looks like it should iterate. It does not, because re-running can only
// widen usable - the marker stops being needed once we have scrolled far enough
// right - and a wider usable makes the scroll-right test strictly weaker, while
// the scroll-left test depends only on left, which is already settled. A sweep
// over two million combinations of width, line length, point column and starting
// offset found no case where a second pass moved the result.
func (w *Window) ScrollToPointHorizontally(textWidth int) {
	if textWidth < 1 {
		textWidth = 1
	}
	pt := w.Buf.ClampPos(w.Pt)
	line := w.Buf.Line(pt.Line)
	ptCol := line.DisplayCol(pt.Col)

	usable := text.ColIdx(textWidth)
	if line.Width()-w.LeftCol > text.ColIdx(textWidth) {
		usable-- // the truncation marker takes the last column
	}
	if usable < 1 {
		usable = 1
	}

	left := w.LeftCol
	if ptCol < left {
		left = ptCol
	}
	if ptCol > left+usable-1 {
		left = ptCol - usable + 1
	}
	if left < 0 {
		left = 0
	}
	w.LeftCol = left
}
