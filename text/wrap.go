package text

import "sort"

// Wrapping. A line longer than its window is folded into rows of the window's
// width, rather than cut off at the edge, when the line-wrap setting is on.
//
// A row breaks where a reader expects: after a space or a tab, so words are
// kept whole. Only a word longer than a whole row is broken inside it. A space
// that reaches the edge stays at the end of its row, past the edge if need be,
// rather than starting the next row with a gap - as emacs's visual-line-mode
// lets it hang in the fringe.

// WrapRows is where each row of the line starts when it is folded into rows
// width columns wide: the rune index of each row's first cluster, the first
// always 0. A line that fits is one row.
//
// Every row holds at least one cluster, so a glyph wider than the whole row
// has a row of its own and runs past its edge. The rows are kept until the
// line changes, so a line drawn frame after frame is folded once.
func (l *Line) WrapRows(width ColIdx) []RuneIdx {
	l.build()
	if width < 1 {
		width = 1
	}
	if l.wraps != nil && l.wrapW == width {
		return l.wraps
	}
	starts := []RuneIdx{0}
	if l.width > width {
		rowStart, rowCol := RuneIdx(0), ColIdx(0)
		// brk is the latest place the row could break, just after a space,
		// and brkCol the column there; -1 when the row has none yet.
		brk, brkCol := RuneIdx(-1), ColIdx(0)
		for c := range l.Clusters() {
			end := c.Col + c.Width
			blank := c.Runes[0] == ' ' || c.Runes[0] == '\t'
			if end-rowCol > width && c.Start > rowStart {
				switch {
				case blank:
					// A space at the edge hangs past it, and the row breaks
					// after it.
					rowStart, rowCol = c.Start+RuneIdx(len(c.Runes)), end
					starts = append(starts, rowStart)
					brk = -1
					continue
				case brk > rowStart:
					rowStart, rowCol = brk, brkCol
				default:
					rowStart, rowCol = c.Start, c.Col
				}
				starts = append(starts, rowStart)
				brk = -1
				// Broken at the last space, the word after it may still be
				// longer than a row: then it breaks here, inside itself.
				if end-rowCol > width && c.Start > rowStart {
					rowStart, rowCol = c.Start, c.Col
					starts = append(starts, rowStart)
				}
			}
			if blank {
				brk, brkCol = c.Start+RuneIdx(len(c.Runes)), end
			}
		}
	}
	l.wraps, l.wrapW = starts, width
	return starts
}

// RowOf is which of the rows WrapRows gives the rune index i falls in. The
// end of the line is on the last row.
func RowOf(starts []RuneIdx, i RuneIdx) int {
	return max(sort.Search(len(starts), func(k int) bool { return starts[k] > i })-1, 0)
}
