// Package view holds nem's view state: a Window (a buffer plus the cursor and
// viewport belonging to one view of it) and the tree of splits that arranges
// windows on screen.
//
// It imports text and the standard library, and nothing else. A window knows
// where point is and which lines are visible; the split tree knows how to
// divide a rectangle. Neither knows what a terminal is, which is why both stay
// testable without one and why this package is separate from ui.
package view

import "github.com/hajianpour/nem/text"

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
