package view

import "github.com/Borderliner/nem/text"

// Wrapped windows. With line-wrap on, a line wider than its window is folded
// into rows of the window's width, and a window's unit is the row on screen
// rather than the buffer line: it scrolls by rows, keeps point's row in view,
// and C-n goes down a row, as emacs's does. These are the row arithmetic all
// of that shares. A width here is the text area's, in columns.

// RowPos is a row on screen of a wrapped buffer: a line, and which of the
// rows it folds into.
type RowPos struct{ Line, Row int }

// Before reports whether r comes before o.
func (r RowPos) Before(o RowPos) bool {
	return r.Line < o.Line || r.Line == o.Line && r.Row < o.Row
}

// rows is how many rows line folds into at width.
func rows(b *text.Buffer, line, width int) int {
	return len(b.Line(line).WrapRows(text.ColIdx(width)))
}

// RowOf is the row p is on.
func RowOf(b *text.Buffer, p text.Pos, width int) RowPos {
	p = b.ClampPos(p)
	return RowPos{p.Line, text.RowOf(b.Line(p.Line).WrapRows(text.ColIdx(width)), p.Col)}
}

// clampRow keeps r within the buffer's rows.
func clampRow(b *text.Buffer, r RowPos, width int) RowPos {
	r.Line = min(max(r.Line, 0), b.NumLines()-1)
	r.Row = min(max(r.Row, 0), rows(b, r.Line, width)-1)
	return r
}

// StepRows moves r n rows down, or up for a negative n, stopping at the
// buffer's first and last rows, and reports how many rows it moved, as a
// count whichever way it went.
func StepRows(b *text.Buffer, r RowPos, n, width int) (RowPos, int) {
	r = clampRow(b, r, width)
	moved := 0
	for ; n > 0; n-- {
		switch {
		case r.Row+1 < rows(b, r.Line, width):
			r.Row++
		case r.Line+1 < b.NumLines():
			r = RowPos{r.Line + 1, 0}
		default:
			return r, moved
		}
		moved++
	}
	for ; n < 0; n++ {
		switch {
		case r.Row > 0:
			r.Row--
		case r.Line > 0:
			r.Line--
			r.Row = rows(b, r.Line, width) - 1
		default:
			return r, moved
		}
		moved++
	}
	return r, moved
}

// RowSpan is where row r starts and ends, as rune indices of its line, and
// the column it starts at.
func RowSpan(b *text.Buffer, r RowPos, width int) (from, to text.RuneIdx, col text.ColIdx) {
	l := b.Line(r.Line)
	starts := l.WrapRows(text.ColIdx(width))
	from, to = starts[r.Row], text.RuneIdx(l.Len())
	if r.Row+1 < len(starts) {
		to = starts[r.Row+1]
	}
	return from, to, l.DisplayCol(from)
}

// PosInRow is the position col columns into row r: on the cluster at that
// column, or the row's last one when it is shorter. The last row of a line
// can take its end, as a line can; any other row ends before the next
// begins, since that position is the next row's.
func PosInRow(b *text.Buffer, r RowPos, width int, col text.ColIdx) text.Pos {
	r = clampRow(b, r, width)
	l := b.Line(r.Line)
	from, to, rowCol := RowSpan(b, r, width)
	i := l.RuneAt(rowCol + max(col, 0))
	if i < from {
		i = from
	}
	if to < text.RuneIdx(l.Len()) && i >= to {
		i = l.PrevGrapheme(to)
	}
	return text.Pos{Line: r.Line, Col: i}
}

// rowsBetween counts the rows from a down to b, stopping once past limit.
func rowsBetween(buf *text.Buffer, a, b RowPos, width, limit int) int {
	if a.Line == b.Line {
		return b.Row - a.Row
	}
	n := rows(buf, a.Line, width) - a.Row
	for ln := a.Line + 1; ln < b.Line && n <= limit; ln++ {
		n += rows(buf, ln, width)
	}
	return n + b.Row
}

// ScrollToPointWrapped is ScrollToPoint for a window whose lines are
// wrapped width columns wide: it moves Top and TopRow, a row at a time, so
// point's row is in view with margin rows around it, and never scrolls past
// the last screenful.
func (w *Window) ScrollToPointWrapped(textHeight, margin, width int) {
	textHeight = max(textHeight, 1)
	margin = min(max(margin, 0), (textHeight-1)/2)
	b := w.Buf
	top := clampRow(b, RowPos{w.Top, w.TopRow}, width)
	pt := RowOf(b, w.Pt, width)

	if pt.Before(top) || rowsBetween(b, top, pt, width, margin) < margin {
		top, _ = StepRows(b, pt, -margin, width)
	} else if rowsBetween(b, top, pt, width, textHeight) > textHeight-1-margin {
		top, _ = StepRows(b, pt, -(textHeight - 1 - margin), width)
	}

	last := b.NumLines() - 1
	maxTop, _ := StepRows(b, RowPos{last, rows(b, last, width) - 1}, -(textHeight - 1), width)
	if maxTop.Before(top) {
		top = maxTop
	}
	w.Top, w.TopRow = top.Line, top.Row
}
