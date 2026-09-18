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
	// group ties entries that undo together. Zero means the entry is a unit of
	// its own. Non-zero values are identities rather than a flag on purpose: two
	// groups can be adjacent in the log, and a boolean would fuse them into one
	// unit the first time that happened.
	group int
}

// UndoLog is a linear undo history. Undoing and then making a fresh edit
// discards the redo branch, which is the behaviour most editors have and
// emacs conspicuously does not.
type UndoLog struct {
	entries []undoEntry
	pos     int // entries[:pos] are currently applied
	savedAt int // pos corresponding to the last saved state, -1 if unreachable

	// depth counts open BeginUndoGroup calls, so a helper that groups its own
	// edits cannot split its caller's group by closing early.
	depth     int
	curGroup  int // identity being stamped on new entries, 0 when ungrouped
	nextGroup int // monotonic counter
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

// breakUnit closes the open entry so later typing starts a new undo unit. It
// deliberately does not touch group state: recordInsert and recordDelete call it
// while a group is being built.
func (u *UndoLog) breakUnit() {
	if u.pos > 0 {
		u.entries[u.pos-1].open = false
	}
}

// beginGroup opens a group, or nests inside one already open.
//
// It deliberately does not close the open entry. Keeping typing from merging
// across a group boundary is the group check in recordInsert's coalescing test,
// and that is the only mechanism for it — a breakUnit here as well would be a
// second defence that no test could fail on, which is how an invariant quietly
// stops being guarded.
func (u *UndoLog) beginGroup() {
	u.depth++
	if u.depth == 1 {
		u.nextGroup++
		u.curGroup = u.nextGroup
	}
}

// endGroup closes the outermost open group. An End with nothing open is a no-op
// rather than an error, so a stray call cannot corrupt the log.
func (u *UndoLog) endGroup() {
	if u.depth == 0 {
		return
	}
	u.depth--
	if u.depth == 0 {
		u.curGroup = 0
	}
}

// abandonGroup force-closes an open group at whatever it has recorded.
//
// The realistic case is a command that opens a group and returns an error before
// closing it. The editor calls BreakUndo between commands, which lands here, so
// an abandoned group can never absorb a later edit — otherwise a single C-/
// would eventually undo the whole session. undo and redo also call it, so even
// if BreakUndo were never reached the group is complete before it is traversed.
func (u *UndoLog) abandonGroup() {
	u.depth = 0
	u.curGroup = 0
}

// endUnit is what Buffer.BreakUndo calls: abandon any open group, then close the
// open entry.
func (u *UndoLog) endUnit() {
	u.abandonGroup()
	u.breakUnit()
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
	// where the previous insert left off joins that unit. The group identities
	// must agree, so typing never merges across a group boundary.
	if u.pos > 0 && len(rs) == 1 && rs[0] != '\n' {
		prev := &u.entries[u.pos-1]
		if prev.kind == opInsert && prev.open && prev.group == u.curGroup &&
			advance(prev.at, prev.text).Equal(at) {
			prev.text = append(prev.text, rs[0])
			return
		}
	}

	u.breakUnit()
	u.entries = append(u.entries, undoEntry{
		kind:  opInsert,
		at:    at,
		text:  append([]rune(nil), rs...),
		open:  len(rs) == 1 && rs[0] != '\n',
		group: u.curGroup,
	})
	u.pos++
}

func (u *UndoLog) recordDelete(from Pos, removed []rune) {
	u.truncate()
	u.breakUnit()
	u.entries = append(u.entries, undoEntry{
		kind:  opDelete,
		at:    from,
		text:  append([]rune(nil), removed...),
		group: u.curGroup,
	})
	u.pos++
}

// undo reverts one unit: a lone entry, or every entry of a group.
//
// Point lands where the user's edit began — the position of the earliest
// primitive in the unit. Entries are reverted newest first, so the earliest is
// reverted last and its landing position is the one that survives.
func (u *UndoLog) undo(b *Buffer) (Pos, bool) {
	u.abandonGroup()
	if u.pos == 0 {
		return Pos{}, false
	}

	group := u.entries[u.pos-1].group
	land, ok := u.undoOne(b)
	if !ok {
		return Pos{}, false
	}
	for group != 0 && u.pos > 0 && u.entries[u.pos-1].group == group {
		next, ok := u.undoOne(b)
		if !ok {
			// A primitive failed to invert. Report the progress made rather
			// than claiming the whole unit reverted.
			return land, true
		}
		land = next
	}
	return land, true
}

// undoOne reverts the single entry below pos.
func (u *UndoLog) undoOne(b *Buffer) (Pos, bool) {
	e := &u.entries[u.pos-1]
	e.open = false // never resume coalescing into an undone unit

	switch e.kind {
	case opInsert:
		if _, err := b.deleteRaw(e.at, advance(e.at, e.text)); err != nil {
			return Pos{}, false
		}
	case opDelete:
		if err := b.insertRaw(e.at, e.text); err != nil {
			return Pos{}, false
		}
	}
	u.pos--
	return e.at, true
}

// redo reapplies one unit: a lone entry, or every entry of a group.
//
// Point lands where the user's edit began, matching undo. Entries are reapplied
// oldest first, so the position from the first one applied is kept and the rest
// are discarded.
func (u *UndoLog) redo(b *Buffer) (Pos, bool) {
	u.abandonGroup()
	if u.pos >= len(u.entries) {
		return Pos{}, false
	}

	group := u.entries[u.pos].group
	land, ok := u.redoOne(b)
	if !ok {
		return Pos{}, false
	}
	for group != 0 && u.pos < len(u.entries) && u.entries[u.pos].group == group {
		if _, ok := u.redoOne(b); !ok {
			return land, true
		}
	}
	return land, true
}

// redoOne reapplies the single entry at pos.
func (u *UndoLog) redoOne(b *Buffer) (Pos, bool) {
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
