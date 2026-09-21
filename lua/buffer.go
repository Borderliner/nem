package lua

import (
	"strings"

	glua "github.com/yuin/gopher-lua"

	"github.com/Borderliner/nem/text"
)

// Line-addressable buffer access for scripts.
//
// # Indexing
//
// Every line number crossing into Lua is 1-based. That matches Lua's own
// convention, what nem.buf.point() already reports, what the modeline shows, and
// what M-x goto-line takes. The internal indices are 0-based, so every function
// here converts at the boundary and nowhere else.
//
// # Minimal edits
//
// Nothing here rewrites more of the buffer than it has to. A transform that
// changes one line records an edit covering that line, not the whole buffer.
// This is not only about undo units: an edit that spans the buffer moves the
// mark and every other window's point, so a before-save hook written to strip
// one trailing space would silently disturb state the user never touched.
//
// # Mutation path
//
// Every change goes through text.Buffer.Insert and Delete, which is what keeps
// the undo log and mark adjustment correct. Nothing here reaches into line
// storage, and no *text.Line is held across a mutation — Buffer.Line returns a
// pointer into the line slice, and any edit invalidates it.

// checkLine converts a 1-based line number from Lua into a 0-based index,
// raising a Lua error when it is out of range. Out of range is an error rather
// than a clamp: a script asking for line 900 of a 12-line buffer has a bug, and
// silently handing back line 12 hides it.
func (h *Host) checkLine(L *glua.LState, b *text.Buffer, n int, what string) int {
	if n < 1 || n > b.NumLines() {
		L.RaiseError("%s: line %d is out of range (buffer has %d lines)", what, n, b.NumLines())
	}
	return n - 1
}

// checkNoNewline rejects embedded newlines in text destined for a single line.
// Accepting them would let set_line change the line count, which is what
// insert_line and set_text are for.
func checkNoNewline(L *glua.LState, s, what string) {
	if strings.ContainsRune(s, '\n') {
		L.RaiseError("%s: text must not contain a newline", what)
	}
}

// clampPoint pulls the active window's point back into the buffer after an edit
// that may have shortened it. Without this a script that deletes the lines below
// point leaves the cursor past the end, and the next redraw reads out of range.
func (h *Host) clampPoint(b *text.Buffer) {
	if h.env == nil {
		return
	}
	w := h.env.Win()
	w.Pt = b.ClampPos(w.Pt)
}

// replaceLineContent rewrites line i to s, recording only the run of runes that
// actually differs.
//
// The common case this exists for is trimming: rewriting "foo   " to "foo" is a
// delete of three runes rather than a delete of the line plus an insert of it
// again, so it is one undo unit and it leaves a mark earlier in the line alone.
func replaceLineContent(b *text.Buffer, i int, s string) error {
	old := b.Line(i).Runes() // a copy, safe to hold across the edits below
	next := []rune(s)

	// Trim the common prefix, then the common suffix of what remains.
	lo := 0
	for lo < len(old) && lo < len(next) && old[lo] == next[lo] {
		lo++
	}
	endOld, endNew := len(old), len(next)
	for endOld > lo && endNew > lo && old[endOld-1] == next[endNew-1] {
		endOld--
		endNew--
	}
	if lo == endOld && lo == endNew {
		return nil // identical
	}

	at := text.Pos{Line: i, Col: text.RuneIdx(lo)}
	if err := b.Delete(at, text.Pos{Line: i, Col: text.RuneIdx(endOld)}); err != nil {
		return err
	}
	return b.Insert(at, next[lo:endNew])
}

// replaceLines swaps the lines in [lo, hiOld) for next, as one delete and one
// insert.
//
// Ranges are half-open and 0-based. lo == hiOld inserts without removing;
// next == nil removes without inserting. The awkwardness is the buffer's line
// separators: a line range's text includes the newline that ends it, except at
// the end of the buffer where there is none, so replacing the final lines has to
// consume the newline *before* lo instead.
func replaceLines(b *text.Buffer, lo, hiOld int, next []string) error {
	numLines := b.NumLines()

	var from, to text.Pos
	var ins string

	switch {
	case hiOld < numLines:
		// An interior range: take the lines and their trailing newlines.
		from = text.Pos{Line: lo}
		to = text.Pos{Line: hiOld}
		if len(next) > 0 {
			ins = strings.Join(next, "\n") + "\n"
		}

	case lo > 0:
		// Through the last line, with lines remaining above. Start at the end of
		// the previous line so the newline joining it to lo is consumed, and put
		// a leading newline back for each replacement line.
		from = text.Pos{Line: lo - 1, Col: b.Line(lo - 1).Len()}
		to = b.End()
		if len(next) > 0 {
			ins = "\n" + strings.Join(next, "\n")
		}

	default:
		// The whole buffer.
		from = text.Pos{}
		to = b.End()
		ins = strings.Join(next, "\n")
	}

	if err := b.Delete(from, to); err != nil {
		return err
	}
	return b.Insert(from, []rune(ins))
}

// bufferLines snapshots the buffer as strings. Snapshotting rather than reading
// lazily is deliberate: Buffer.Line hands back a pointer into the line slice,
// and the caller is about to edit.
func bufferLines(b *text.Buffer) []string {
	out := make([]string, b.NumLines())
	for i := range out {
		out[i] = b.Line(i).String()
	}
	return out
}

// --- the Lua-facing functions -----------------------------------------------

func (h *Host) bufLineCount(L *glua.LState) int {
	L.Push(glua.LNumber(h.mustEnv(L).Buf().NumLines()))
	return 1
}

func (h *Host) bufGetLine(L *glua.LState) int {
	n := L.CheckInt(1)
	b := h.mustEnv(L).Buf()
	i := h.checkLine(L, b, n, "nem.buf.get_line")
	L.Push(glua.LString(b.Line(i).String()))
	return 1
}

func (h *Host) bufSetLine(L *glua.LState) int {
	n := L.CheckInt(1)
	s := L.CheckString(2)
	checkNoNewline(L, s, "nem.buf.set_line")

	b := h.mustEnv(L).Buf()
	i := h.checkLine(L, b, n, "nem.buf.set_line")
	if err := replaceLineContent(b, i, s); err != nil {
		L.RaiseError("nem.buf.set_line: %v", err)
	}
	h.clampPoint(b)
	return 0
}

// bufInsertLine inserts a new line so that it becomes line n, shifting the rest
// down. n may be line_count+1, which appends — the same range table.insert
// accepts, so the convention is one a Lua author already knows.
func (h *Host) bufInsertLine(L *glua.LState) int {
	n := L.CheckInt(1)
	s := L.CheckString(2)
	checkNoNewline(L, s, "nem.buf.insert_line")

	b := h.mustEnv(L).Buf()
	if n < 1 || n > b.NumLines()+1 {
		L.RaiseError("nem.buf.insert_line: line %d is out of range (buffer has %d lines; 1 to %d inserts)",
			n, b.NumLines(), b.NumLines()+1)
	}
	i := n - 1
	if err := replaceLines(b, i, i, []string{s}); err != nil {
		L.RaiseError("nem.buf.insert_line: %v", err)
	}
	h.clampPoint(b)
	return 0
}

// bufRemoveLine deletes line n.
//
// Removing the only line leaves one empty line rather than a buffer with none,
// which is what the rest of the editor requires — Buffer.End() indexes the last
// line unconditionally. That falls out of replaceLines' whole-buffer case with no
// special handling here; an earlier version guarded it explicitly and mutation
// testing showed the guard was unreachable.
func (h *Host) bufRemoveLine(L *glua.LState) int {
	n := L.CheckInt(1)
	b := h.mustEnv(L).Buf()
	i := h.checkLine(L, b, n, "nem.buf.remove_line")

	if err := replaceLines(b, i, i+1, nil); err != nil {
		L.RaiseError("nem.buf.remove_line: %v", err)
	}
	h.clampPoint(b)
	return 0
}

// bufSetText replaces the whole buffer, recording only the lines that differ.
//
// It compares old and new line by line from both ends first, so a transform that
// rewrites the text but changes one line records an edit covering that line. A
// script can therefore read nem.buf.text(), transform the string however it
// likes, and write it back without the cost of a whole-buffer rewrite.
func (h *Host) bufSetText(L *glua.LState) int {
	s := L.CheckString(1)
	b := h.mustEnv(L).Buf()

	old := bufferLines(b)
	next := strings.Split(s, "\n")

	lo := 0
	for lo < len(old) && lo < len(next) && old[lo] == next[lo] {
		lo++
	}
	hiOld, hiNew := len(old), len(next)
	for hiOld > lo && hiNew > lo && old[hiOld-1] == next[hiNew-1] {
		hiOld--
		hiNew--
	}

	switch {
	case lo == hiOld && lo == hiNew:
		// Identical: record nothing at all, so a no-op hook does not mark the
		// buffer modified or consume an undo step.
		return 0

	case hiOld-lo == 1 && hiNew-lo == 1:
		// Exactly one line changed. Go through the rune-level path so a trim
		// stays a single small delete.
		if err := replaceLineContent(b, lo, next[lo]); err != nil {
			L.RaiseError("nem.buf.set_text: %v", err)
		}

	default:
		if err := replaceLines(b, lo, hiOld, next[lo:hiNew]); err != nil {
			L.RaiseError("nem.buf.set_text: %v", err)
		}
	}

	h.clampPoint(b)
	return 0
}
