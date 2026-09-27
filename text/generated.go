package text

// RegenerateTail replaces everything from the start of line from to the end
// of the buffer with rs, as Regenerate replaces the whole: past read-only and
// an edit guard, with no undo record, leaving the buffer unmodified.
//
// It is for a generated buffer that grows - a program's output, arriving a
// piece at a time - which rewrites only its last, unfinished line and what
// comes after it, rather than all of itself on every piece.
func (b *Buffer) RegenerateTail(from int, rs []rune) {
	from = max(0, min(from, len(b.lines)-1))
	// Neither can fail: both positions come from the buffer itself.
	_, _ = b.deleteRaw(Pos{Line: from}, b.End())
	_ = b.insertRaw(Pos{Line: from}, rs)
}
