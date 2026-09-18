package text

type opKind int

const (
	opInsert opKind = iota
	opDelete
)

// undoEntry is one reversible edit. Because every mutation funnels through
// Insert and Delete, recording those two is enough to invert anything.
type undoEntry struct {
	kind opKind
	at   Pos    // where the change began
	text []rune // text inserted (opInsert) or removed (opDelete)
	open bool   // still absorbing consecutive single-rune inserts
}

// UndoLog is a linear undo history. Undoing and then making a fresh edit
// discards the redo branch, which is the behaviour most editors have and
// emacs conspicuously does not.
type UndoLog struct {
	entries []undoEntry
	pos     int // entries[:pos] are currently applied
	savedAt int // pos corresponding to the last saved state, -1 if unreachable
}

func newUndoLog() *UndoLog { return &UndoLog{savedAt: 0} }

func (u *UndoLog) modified() bool { return u.pos != u.savedAt }

func (u *UndoLog) setModified(m bool) {
	if m {
		u.savedAt = -1
	} else {
		u.savedAt = u.pos
	}
}

// breakUnit closes the open entry so later typing starts a new undo unit.
func (u *UndoLog) breakUnit() {
	if u.pos > 0 {
		u.entries[u.pos-1].open = false
	}
}

// truncate discards the redo branch before recording a new edit.
func (u *UndoLog) truncate() {
	u.entries = u.entries[:u.pos]
	if u.savedAt > u.pos {
		u.savedAt = -1 // the saved state is no longer reachable
	}
}

func (u *UndoLog) recordInsert(at Pos, rs []rune) {
	u.truncate()

	// Coalesce ordinary typing: a single non-newline rune inserted exactly
	// where the previous insert left off joins that unit.
	if u.pos > 0 && len(rs) == 1 && rs[0] != '\n' {
		prev := &u.entries[u.pos-1]
		if prev.kind == opInsert && prev.open && advance(prev.at, prev.text).Equal(at) {
			prev.text = append(prev.text, rs[0])
			return
		}
	}

	u.breakUnit()
	u.entries = append(u.entries, undoEntry{
		kind: opInsert,
		at:   at,
		text: append([]rune(nil), rs...),
		open: len(rs) == 1 && rs[0] != '\n',
	})
	u.pos++
}

func (u *UndoLog) recordDelete(from Pos, removed []rune) {
	u.truncate()
	u.breakUnit()
	u.entries = append(u.entries, undoEntry{
		kind: opDelete,
		at:   from,
		text: append([]rune(nil), removed...),
	})
	u.pos++
}

func (u *UndoLog) undo(b *Buffer) (Pos, bool) {
	if u.pos == 0 {
		return Pos{}, false
	}
	e := &u.entries[u.pos-1]
	e.open = false // never resume coalescing into an undone unit

	var land Pos
	switch e.kind {
	case opInsert:
		land = e.at
		if _, err := b.deleteRaw(e.at, advance(e.at, e.text)); err != nil {
			return Pos{}, false
		}
	case opDelete:
		land = e.at
		if err := b.insertRaw(e.at, e.text); err != nil {
			return Pos{}, false
		}
	}
	u.pos--
	return land, true
}

func (u *UndoLog) redo(b *Buffer) (Pos, bool) {
	if u.pos >= len(u.entries) {
		return Pos{}, false
	}
	e := &u.entries[u.pos]

	var land Pos
	switch e.kind {
	case opInsert:
		land = advance(e.at, e.text)
		if err := b.insertRaw(e.at, e.text); err != nil {
			return Pos{}, false
		}
	case opDelete:
		land = e.at
		if _, err := b.deleteRaw(e.at, advance(e.at, e.text)); err != nil {
			return Pos{}, false
		}
	}
	u.pos++
	return land, true
}
