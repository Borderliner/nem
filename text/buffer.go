package text

import (
	"errors"
	"slices"
)

// ErrOutOfRange is returned when a Pos does not name a location in the buffer.
var ErrOutOfRange = errors.New("text: position out of range")

// ErrReadOnly is returned by an edit to a read-only buffer.
var ErrReadOnly = errors.New("buffer is read-only")

// Buffer is a sequence of lines plus the editing state that belongs to the
// text itself rather than to any view of it.
//
// Point deliberately does not live here: several windows may show one buffer,
// each with its own cursor. Buffer keeps only savePt, the point restored when
// a window next visits it.
type Buffer struct {
	// lines holds pointers, not Lines, for two reasons. A newline shifts every
	// line after it, and moving eight bytes a line instead of seventy-two is
	// what keeps RET cheap in a large file. And a *Line from Line(i) then stays
	// the line it was, rather than whichever line an edit above moved into
	// its slot.
	lines   []*Line
	mark    Pos
	hasMark bool
	// markActive says the region is live - highlighted, and replaced by typing.
	// It is separate from hasMark on purpose; see MarkActive.
	markActive bool
	savePt     Pos
	// saveTop is the first line on screen when a window last left this buffer,
	// restored with savePt so coming back shows the same view.
	saveTop int
	undo    *UndoLog

	path    string
	crlf    bool // file used \r\n line endings
	finalNL bool // file ended with a newline

	// Edit tracking for the highlight cache. See dirty.go.
	rev       uint64
	dirtyFrom int
	lineDelta int

	// readOnly refuses every edit. It is enforced here, in the two primitives,
	// rather than by commands: sixty commands would each have to remember it,
	// and the one that forgot would quietly corrupt a directory listing.
	readOnly bool
}

// NewBuffer returns an empty buffer holding a single empty line.
func NewBuffer() *Buffer {
	return &Buffer{
		lines:     []*Line{{}},
		undo:      newUndoLog(),
		dirtyFrom: noDirtyLine,
	}
}

// NumLines returns the number of lines. It is always at least 1.
func (b *Buffer) NumLines() int { return len(b.lines) }

// Line returns the line at index i.
func (b *Buffer) Line(i int) *Line { return b.lines[i] }

// Path returns the file this buffer is associated with, empty if none.
func (b *Buffer) Path() string { return b.path }

// SetPath associates the buffer with a file path.
func (b *Buffer) SetPath(p string) { b.path = p }

// Mark returns the buffer's mark, the far end of the region.
func (b *Buffer) Mark() Pos { return b.mark }

// SetMark sets the buffer's mark and records that a mark now exists.
//
// It does NOT activate the region. Setting a mark and selecting text are
// different acts: yank sets the mark so C-x C-x can select what was yanked, and
// the buffer-edge jumps set it so C-x C-x can return, but in neither case is the
// text between mark and point a selection. Commands that mean to select call
// ActivateMark as well.
func (b *Buffer) SetMark(p Pos) { b.mark, b.hasMark = p, true }

// HasMark reports whether a mark has been set in this buffer.
//
// The zero Pos is a legitimate mark position, so a flag is the only way to tell
// "mark at the buffer start" from "no mark at all". Region commands must check
// this: emacs refuses to act on a region in a buffer with no mark, and without
// the distinction C-w would silently kill from the buffer start to point.
func (b *Buffer) HasMark() bool { return b.hasMark }

// ClearMark forgets the mark entirely, which also deactivates the region.
func (b *Buffer) ClearMark() { b.mark, b.hasMark, b.markActive = Pos{}, false, false }

// MarkActive reports whether the region is live: drawn as a selection, and
// replaced by typing, Backspace or a yank.
//
// A mark can exist without the region being active, and the difference matters.
// When the two were conflated, every command that set a mark for navigation
// silently created a selection: M-> M-< then typing a character deleted the
// whole file, and typing after C-y deleted what had just been pasted. This is
// emacs's transient-mark-mode distinction between the mark and mark-active.
//
// Region commands that consume the region themselves - C-w, M-w, C-x C-x - use
// HasMark instead, so C-y then C-w still kills what was yanked, as emacs does by
// default.
func (b *Buffer) MarkActive() bool { return b.hasMark && b.markActive }

// ActivateMark makes the region live. With no mark set there is no region, so
// it does nothing.
func (b *Buffer) ActivateMark() {
	if b.hasMark {
		b.markActive = true
	}
}

// DeactivateMark ends the selection but keeps the mark, so C-x C-x can still
// return to it. This is what C-g does, and what any buffer-changing command does
// afterwards.
func (b *Buffer) DeactivateMark() { b.markActive = false }

// SavePoint returns the point stored for when a window next visits this buffer.
func (b *Buffer) SavePoint() Pos { return b.savePt }

// SetSavePoint stores the point for when a window next visits this buffer.
func (b *Buffer) SetSavePoint(p Pos) { b.savePt = p }

// SaveTop returns the first line shown when a window last left this buffer.
func (b *Buffer) SaveTop() int { return b.saveTop }

// SetSaveTop stores the first line shown, for when a window next visits.
func (b *Buffer) SetSaveTop(line int) { b.saveTop = line }

// Modified reports whether the buffer differs from its last saved state.
func (b *Buffer) Modified() bool { return b.undo.modified() }

// SetModified marks the buffer clean or dirty. Marking it clean records the
// current undo position, so undoing back to it clears the flag again.
func (b *Buffer) SetModified(m bool) { b.undo.setModified(m) }

// ReadOnly reports whether the buffer refuses edits.
func (b *Buffer) ReadOnly() bool { return b.readOnly }

// SetReadOnly makes the buffer refuse or accept edits. Undo is refused too while
// it is set, since undoing is editing.
func (b *Buffer) SetReadOnly(ro bool) { b.readOnly = ro }

// Regenerate replaces the whole text of a buffer whose contents are generated,
// such as a directory listing. It works on a read-only buffer, leaves it
// unmodified, and discards the undo history: undoing back to an old listing
// would show files that are no longer there.
func (b *Buffer) Regenerate(rs []rune) {
	ro := b.readOnly
	b.readOnly = false
	// Neither can fail: both positions come from the buffer itself.
	_ = b.Delete(Pos{}, b.End())
	_ = b.Insert(Pos{}, rs)
	b.readOnly = ro
	b.undo = newUndoLog()
}

// RegenerateLine replaces the text of one line of a generated buffer, as
// Regenerate does the whole: past read-only, with no undo record, leaving the
// buffer unmodified. A listing uses it when one entry changes, so marking a
// file costs one line rather than the whole directory.
func (b *Buffer) RegenerateLine(i int, rs []rune) {
	if i < 0 || i >= len(b.lines) {
		return
	}
	b.lines[i].setRunes(append([]rune(nil), rs...))
	b.noteEdit(i, 0)
}

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
func (b *Buffer) String() string { return string(b.encode("\n", false)) }

// Insert inserts rs at at, splitting the line at any newline in rs.
// It is one of the two primitives every edit composes from.
func (b *Buffer) Insert(at Pos, rs []rune) error {
	if b.readOnly {
		return ErrReadOnly
	}
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
	if b.readOnly {
		return ErrReadOnly
	}
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
	// In place, not rebuilt. Rebuilding copied the line three times per
	// keystroke, and - on a newline - the whole slice of lines, which in a
	// file of a million lines is tens of megabytes for one RET. A line's runes
	// belong to that line alone, so its array can be grown or reused here.
	cur := b.lines[at.Line].runes
	parts := splitRuneLines(rs)

	if len(parts) == 1 {
		b.lines[at.Line].setRunes(slices.Insert(cur, int(at.Col), parts[0]...))
	} else {
		// The line splits. The text after the cut moves to the last new line,
		// and is copied out before the head reuses its array.
		last := parts[len(parts)-1]
		tail := make([]rune, 0, len(last)+len(cur)-int(at.Col))
		tail = append(append(tail, last...), cur[at.Col:]...)

		built := make([]*Line, 0, len(parts))
		head := b.lines[at.Line]
		head.setRunes(append(cur[:at.Col], parts[0]...))
		built = append(built, head)
		for _, p := range parts[1 : len(parts)-1] {
			l := NewLine(p)
			built = append(built, &l)
		}
		built = append(built, &Line{runes: tail})
		b.lines = slices.Replace(b.lines, at.Line, at.Line+1, built...)
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

	// In place, for the reasons insertRaw gives.
	first := b.lines[from.Line]
	if to.Line == from.Line {
		first.setRunes(slices.Delete(first.runes, int(from.Col), int(to.Col)))
	} else {
		// The last line's remainder joins the first; that line is about to be
		// dropped, so its runes can be read while the first line's array is
		// overwritten from the cut onwards.
		first.setRunes(append(first.runes[:from.Col], b.lines[to.Line].runes[to.Col:]...))
		b.lines = slices.Delete(b.lines, from.Line+1, to.Line+1)
	}

	b.noteEdit(from.Line, -(to.Line - from.Line))
	b.mark = adjustForDelete(b.mark, from, to)
	b.savePt = adjustForDelete(b.savePt, from, to)
	return removed, nil
}

// Undo reverts the most recent edit, returning where point should land. A
// read-only buffer reports nothing to undo; callers that want to say why check
// ReadOnly first.
func (b *Buffer) Undo() (Pos, bool) {
	if b.readOnly {
		return Pos{}, false
	}
	return b.undo.undo(b)
}

// Redo reapplies the most recently undone edit, returning where point lands.
func (b *Buffer) Redo() (Pos, bool) {
	if b.readOnly {
		return Pos{}, false
	}
	return b.undo.redo(b)
}

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
