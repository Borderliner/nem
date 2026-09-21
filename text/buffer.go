package text

import "errors"

// ErrOutOfRange is returned when a Pos does not name a location in the buffer.
var ErrOutOfRange = errors.New("text: position out of range")

// Buffer is a sequence of lines plus the editing state that belongs to the
// text itself rather than to any view of it.
//
// Point deliberately does not live here: several windows may show one buffer,
// each with its own cursor. Buffer keeps only savePt, the point restored when
// a window next visits it.
type Buffer struct {
	lines   []Line
	mark    Pos
	hasMark bool
	savePt  Pos
	undo    *UndoLog

	path    string
	crlf    bool // file used \r\n line endings
	finalNL bool // file ended with a newline

	// Edit tracking for the highlight cache. See dirty.go.
	rev       uint64
	dirtyFrom int
	lineDelta int
}

// NewBuffer returns an empty buffer holding a single empty line.
func NewBuffer() *Buffer {
	return &Buffer{
		lines:     []Line{NewLine(nil)},
		undo:      newUndoLog(),
		dirtyFrom: noDirtyLine,
	}
}

// NumLines returns the number of lines. It is always at least 1.
func (b *Buffer) NumLines() int { return len(b.lines) }

// Line returns the line at index i.
func (b *Buffer) Line(i int) *Line { return &b.lines[i] }

// Path returns the file this buffer is associated with, empty if none.
func (b *Buffer) Path() string { return b.path }

// SetPath associates the buffer with a file path.
func (b *Buffer) SetPath(p string) { b.path = p }

// Mark returns the buffer's mark, the far end of the region.
func (b *Buffer) Mark() Pos { return b.mark }

// SetMark sets the buffer's mark and records that a mark now exists.
func (b *Buffer) SetMark(p Pos) { b.mark, b.hasMark = p, true }

// HasMark reports whether a mark has been set in this buffer.
//
// The zero Pos is a legitimate mark position, so a flag is the only way to tell
// "mark at the buffer start" from "no mark at all". Region commands must check
// this: emacs refuses to act on a region in a buffer with no mark, and without
// the distinction C-w would silently kill from the buffer start to point.
func (b *Buffer) HasMark() bool { return b.hasMark }

// ClearMark forgets the mark, as C-g does when it deactivates the region.
func (b *Buffer) ClearMark() { b.mark, b.hasMark = Pos{}, false }

// SavePoint returns the point stored for when a window next visits this buffer.
func (b *Buffer) SavePoint() Pos { return b.savePt }

// SetSavePoint stores the point for when a window next visits this buffer.
func (b *Buffer) SetSavePoint(p Pos) { b.savePt = p }

// Modified reports whether the buffer differs from its last saved state.
func (b *Buffer) Modified() bool { return b.undo.modified() }

// SetModified marks the buffer clean or dirty. Marking it clean records the
// current undo position, so undoing back to it clears the flag again.
func (b *Buffer) SetModified(m bool) { b.undo.setModified(m) }

// End returns the position just past the last rune in the buffer.
func (b *Buffer) End() Pos {
	last := len(b.lines) - 1
	return Pos{last, b.lines[last].Len()}
}

// ClampPos returns the position in p clamped into the buffer.
func (b *Buffer) ClampPos(p Pos) Pos {
	if p.Line < 0 {
		p.Line = 0
	}
	if p.Line >= len(b.lines) {
		p.Line = len(b.lines) - 1
	}
	if p.Col < 0 {
		p.Col = 0
	}
	if n := b.lines[p.Line].Len(); p.Col > n {
		p.Col = n
	}
	return p
}

func (b *Buffer) checkPos(p Pos) error {
	if p.Line < 0 || p.Line >= len(b.lines) {
		return ErrOutOfRange
	}
	if p.Col < 0 || p.Col > b.lines[p.Line].Len() {
		return ErrOutOfRange
	}
	return nil
}

// Text returns a copy of the runes between from and to, with '\n' at line
// boundaries. The endpoints may be given in either order.
func (b *Buffer) Text(from, to Pos) []rune {
	from, to = OrderPos(from, to)
	from, to = b.ClampPos(from), b.ClampPos(to)
	if from.Equal(to) {
		return nil
	}
	if from.Line == to.Line {
		return append([]rune(nil), b.lines[from.Line].runes[from.Col:to.Col]...)
	}
	out := append([]rune(nil), b.lines[from.Line].runes[from.Col:]...)
	out = append(out, '\n')
	for i := from.Line + 1; i < to.Line; i++ {
		out = append(out, b.lines[i].runes...)
		out = append(out, '\n')
	}
	return append(out, b.lines[to.Line].runes[:to.Col]...)
}

// String returns the whole buffer as text, without a trailing newline.
func (b *Buffer) String() string { return string(b.Text(Pos{0, 0}, b.End())) }

// Insert inserts rs at at, splitting the line at any newline in rs.
// It is one of the two primitives every edit composes from.
func (b *Buffer) Insert(at Pos, rs []rune) error {
	if err := b.checkPos(at); err != nil {
		return err
	}
	if len(rs) == 0 {
		return nil
	}
	b.undo.recordInsert(at, rs)
	return b.insertRaw(at, rs)
}

// Delete removes the runes between from and to, joining lines as needed.
// The endpoints may be given in either order. It is the other primitive.
func (b *Buffer) Delete(from, to Pos) error {
	from, to = OrderPos(from, to)
	if err := b.checkPos(from); err != nil {
		return err
	}
	if err := b.checkPos(to); err != nil {
		return err
	}
	if from.Equal(to) {
		return nil
	}
	b.undo.recordDelete(from, b.Text(from, to))
	_, err := b.deleteRaw(from, to)
	return err
}

// insertRaw performs an insertion without recording undo history.
func (b *Buffer) insertRaw(at Pos, rs []rune) error {
	if err := b.checkPos(at); err != nil {
		return err
	}
	if len(rs) == 0 {
		return nil
	}
	cur := b.lines[at.Line].runes
	head := append([]rune(nil), cur[:at.Col]...)
	tail := append([]rune(nil), cur[at.Col:]...)
	parts := splitRuneLines(rs)

	if len(parts) == 1 {
		nw := make([]rune, 0, len(head)+len(parts[0])+len(tail))
		nw = append(nw, head...)
		nw = append(nw, parts[0]...)
		nw = append(nw, tail...)
		b.lines[at.Line].setRunes(nw)
	} else {
		built := make([]Line, 0, len(parts))
		built = append(built, NewLine(append(head, parts[0]...)))
		for _, p := range parts[1 : len(parts)-1] {
			built = append(built, NewLine(p))
		}
		built = append(built, NewLine(append(append([]rune(nil), parts[len(parts)-1]...), tail...)))

		nl := make([]Line, 0, len(b.lines)+len(built)-1)
		nl = append(nl, b.lines[:at.Line]...)
		nl = append(nl, built...)
		nl = append(nl, b.lines[at.Line+1:]...)
		b.lines = nl
	}

	b.noteEdit(at.Line, len(parts)-1)
	b.mark = adjustForInsert(b.mark, at, rs)
	b.savePt = adjustForInsert(b.savePt, at, rs)
	return nil
}

// deleteRaw performs a deletion without recording undo history, returning the
// removed text.
func (b *Buffer) deleteRaw(from, to Pos) ([]rune, error) {
	from, to = OrderPos(from, to)
	if err := b.checkPos(from); err != nil {
		return nil, err
	}
	if err := b.checkPos(to); err != nil {
		return nil, err
	}
	if from.Equal(to) {
		return nil, nil
	}
	removed := b.Text(from, to)

	head := append([]rune(nil), b.lines[from.Line].runes[:from.Col]...)
	tail := append([]rune(nil), b.lines[to.Line].runes[to.Col:]...)
	b.lines[from.Line].setRunes(append(head, tail...))

	if to.Line > from.Line {
		nl := make([]Line, 0, len(b.lines)-(to.Line-from.Line))
		nl = append(nl, b.lines[:from.Line+1]...)
		nl = append(nl, b.lines[to.Line+1:]...)
		b.lines = nl
	}

	b.noteEdit(from.Line, -(to.Line - from.Line))
	b.mark = adjustForDelete(b.mark, from, to)
	b.savePt = adjustForDelete(b.savePt, from, to)
	return removed, nil
}

// Undo reverts the most recent edit, returning where point should land.
func (b *Buffer) Undo() (Pos, bool) { return b.undo.undo(b) }

// Redo reapplies the most recently undone edit, returning where point lands.
func (b *Buffer) Redo() (Pos, bool) { return b.undo.redo(b) }

// BreakUndo ends the current undo unit, so subsequent typing starts a new one.
// The editor calls it on movement, on any non-inserting command, and on save.
//
// It also abandons an undo group left open by a command that bailed out. That
// bounds the damage to the one command: without it an unclosed group would keep
// absorbing later edits until a single undo reverted the whole session.
func (b *Buffer) BreakUndo() { b.undo.endUnit() }

// BeginUndoGroup starts a group: every Insert and Delete until the matching
// EndUndoGroup undoes and redoes as one unit.
//
// A command that composes several primitives needs this. Moving a line is a
// delete plus an insert, so without grouping five presses of the move key would
// cost ten presses of undo.
//
// Groups nest by depth, so a helper may group its own edits without splitting
// its caller's group. Pair it with defer, or with EndUndoGroup on every path.
func (b *Buffer) BeginUndoGroup() { b.undo.beginGroup() }

// EndUndoGroup closes the outermost open group. Calling it with no group open is
// a no-op, so a stray call cannot corrupt the history.
func (b *Buffer) EndUndoGroup() { b.undo.endGroup() }

// splitRuneLines splits rs on '\n'. The result always has at least one element,
// and one more than the number of newlines present.
func splitRuneLines(rs []rune) [][]rune {
	var out [][]rune
	start := 0
	for i, r := range rs {
		if r == '\n' {
			out = append(out, rs[start:i])
			start = i + 1
		}
	}
	return append(out, rs[start:])
}

// advance returns the position just past text inserted at at.
func advance(at Pos, text []rune) Pos {
	nl, lastNL := 0, -1
	for i, r := range text {
		if r == '\n' {
			nl++
			lastNL = i
		}
	}
	if nl == 0 {
		return Pos{at.Line, at.Col + RuneIdx(len(text))}
	}
	return Pos{at.Line + nl, RuneIdx(len(text) - lastNL - 1)}
}

// adjustForInsert shifts a stored position to account for an insertion.
// A position exactly at the insertion point does not move.
func adjustForInsert(p, at Pos, rs []rune) Pos {
	if p.Before(at) || p.Equal(at) {
		return p
	}
	end := advance(at, rs)
	nl := end.Line - at.Line
	if p.Line == at.Line {
		if nl == 0 {
			return Pos{p.Line, p.Col + RuneIdx(len(rs))}
		}
		return Pos{end.Line, end.Col + (p.Col - at.Col)}
	}
	return Pos{p.Line + nl, p.Col}
}

// adjustForDelete shifts a stored position to account for a deletion.
// A position inside the deleted range collapses to its start.
func adjustForDelete(p, from, to Pos) Pos {
	if p.Before(from) || p.Equal(from) {
		return p
	}
	if p.Before(to) {
		return from
	}
	if p.Line == to.Line {
		return Pos{from.Line, from.Col + (p.Col - to.Col)}
	}
	return Pos{p.Line - (to.Line - from.Line), p.Col}
}
