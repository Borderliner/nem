package text

// Edit tracking exists for one reader: the syntax-highlight cache.
//
// Highlighting a line needs the lexer state the line above left behind, so
// colouring line N means lexing from line 0. On a ten-thousand-line file that
// is milliseconds of work on every keystroke. The cache avoids it by keeping
// one state per line and re-lexing only downward from an edit, which requires
// knowing which line changed - and the buffer is the only thing that knows.
//
// The three values below are what a reader needs to do that safely:
//
//   - dirtyFrom, the lowest line touched, because everything below the lowest
//     edit may have been affected by it and everything above cannot have been.
//   - lineDelta, the net change in line count, so a reader can shift its
//     per-line data to stay aligned instead of throwing it away every time
//     someone presses Enter.
//   - rev, which advances once per mutation. A reader that sees rev advance by
//     exactly one knows dirtyFrom and lineDelta describe a single edit and are
//     therefore exact. Several edits between reads collapse into one dirtyFrom
//     and a summed delta, which cannot be applied line by line - so a reader
//     seeing a jump of more than one falls back to trusting nothing below the
//     edit rather than shifting data by a delta that does not describe it.
//
// The tracking lives in insertRaw and deleteRaw rather than in Insert and
// Delete because undo and redo call the raw forms directly. Tracking the public
// methods would leave a reader believing the buffer was untouched after an
// undo, and the colours would stay wrong until some later edit happened to
// invalidate the same region.

// noDirtyLine is the dirtyFrom value meaning "nothing has been touched". It is
// deliberately larger than any line index rather than -1, so that the usual
// min() against it needs no special case.
const noDirtyLine = int(^uint(0) >> 1) // max int

// Revision returns a counter that advances once per mutation.
//
// It never resets, and a mutation that changes nothing does not advance it.
func (b *Buffer) Revision() uint64 { return b.rev }

// TakeDirty reports what has changed since the last call and clears the record.
//
// It returns the lowest line touched, the net change in line count, and the
// current revision. When nothing has changed, from is >= NumLines and delta is
// zero.
//
// It CONSUMES: the name says so because a second reader calling it would take
// the notification from the first, which would then never learn of the edit.
// nem attaches exactly one highlight cache to a buffer. A second reader is not
// a correctness problem even so - it sees the revision advance without a dirty
// line, which is a signal it cannot account for, and the cache treats that as
// "trust nothing" rather than as "nothing changed". The result is slower, not
// wrong.
func (b *Buffer) TakeDirty() (from, delta int, rev uint64) {
	from, delta, rev = b.dirtyFrom, b.lineDelta, b.rev
	b.dirtyFrom, b.lineDelta = noDirtyLine, 0
	return from, delta, rev
}

// noteEdit records a mutation at line, changing the line count by delta.
//
// Called from insertRaw and deleteRaw, the two functions through which every
// change to the text passes.
func (b *Buffer) noteEdit(line, delta int) {
	b.rev++
	if line < b.dirtyFrom {
		b.dirtyFrom = line
	}
	b.lineDelta += delta
}
