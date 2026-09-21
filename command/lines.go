package command

import "github.com/Borderliner/nem/text"

// RegisterLines adds the line-moving commands.
//
// These are nem going past emacs rather than following it: emacs has no native
// line move, so there is no traditional behaviour to be faithful to and these
// work the way the equivalent does in any modern editor.
func RegisterLines(r *Registry) error {
	for _, c := range []Command{
		{
			Name:        "move-lines-up",
			Doc:         "Move the region's lines, or the current line, one line up.",
			Fn:          moveLinesUp,
			Interactive: true,
		},
		{
			Name:        "move-lines-down",
			Doc:         "Move the region's lines, or the current line, one line down.",
			Fn:          moveLinesDown,
			Interactive: true,
		},
	} {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// Messages reported when a block is already against a buffer boundary. These are
// facts about where you are rather than faults, so they go through Echo.
const (
	atBufferTop    = "Beginning of buffer"
	atBufferBottom = "End of buffer"
)

func moveLinesUp(e Env) error   { return moveLines(e, -1) }
func moveLinesDown(e Env) error { return moveLines(e, +1) }

// lineBlock returns the inclusive range of buffer lines the command acts on.
//
// Whole lines, always: the region may cover only part of its first and last
// lines, but a partial line move would be meaningless. With no mark it is the
// line point is on.
//
// A region that ends at column 0 does not include that line. C-a C-SPC C-n is a
// very common way to select exactly one line, and it leaves point at column 0 of
// the next one; treating that as two lines would move a line the user never saw
// highlighted. This is the convention every editor with this feature follows.
func lineBlock(e Env) (first, last int) {
	b := e.Buf()
	pt := b.ClampPos(e.Win().Pt)
	if !b.HasMark() {
		return pt.Line, pt.Line
	}
	lo, hi := text.OrderPos(pt, b.ClampPos(b.Mark()))
	first, last = lo.Line, hi.Line
	if last > first && hi.Col == 0 {
		last--
	}
	return first, last
}

// moveLines shifts the block by dir (-1 up, +1 down), honouring the prefix
// argument.
//
// A negative argument reverses direction, as it does for the motion commands. An
// argument that overshoots the buffer moves as far as it can and then reports,
// rather than refusing outright: C-u 20 M-<down> should land the line at the
// bottom, not leave it where it started.
func moveLines(e Env, dir int) error {
	n, _ := e.Arg()
	if n < 0 {
		dir, n = -dir, -n
	}
	if n == 0 {
		return nil
	}

	b := e.Buf()
	first, last := lineBlock(e)

	// How many single-line steps this direction allows before the block is
	// against the boundary.
	avail := first
	if dir > 0 {
		avail = b.NumLines() - 1 - last
	}
	steps := min(n, avail)

	if steps == 0 {
		e.Echo(edgeMessage(dir))
		return nil
	}

	// One group for the whole command, so C-u 3 M-<down> is a single C-/ and a
	// refused move records nothing at all.
	b.BeginUndoGroup()
	defer b.EndUndoGroup()

	for i := 0; i < steps; i++ {
		if err := moveLinesOnce(e, dir); err != nil {
			return err
		}
	}
	if steps < n {
		e.Echo(edgeMessage(dir))
	}
	return nil
}

func edgeMessage(dir int) string {
	if dir < 0 {
		return atBufferTop
	}
	return atBufferBottom
}

// moveLinesOnce shifts the block one line, by moving the single adjacent line
// across it rather than moving the block itself.
//
// Rotating the neighbour is the cheaper half of the same operation: moving a
// three-line block up by one is identical to moving the one line above it down
// past the block, and it touches one line's worth of text however large the
// block is.
//
// The block is recomputed from point and mark on each call, so a multi-step move
// follows the block as it travels.
func moveLinesOnce(e Env, dir int) error {
	b, w := e.Buf(), e.Win()
	first, last := lineBlock(e)

	donor := first - 1
	target := last // after the donor above is removed, the block sits one higher
	if dir > 0 {
		donor = last + 1
		target = first
	}

	// Copied before the edit: Buffer.Line returns a pointer into the line slice
	// and any edit invalidates it.
	content := b.Line(donor).String()

	// Both ends are captured before the edits and rewritten afterwards rather
	// than left to the primitives' own adjustment.
	//
	// Insert and Delete do shift the mark, but inserting at exactly the mark's
	// position leaves the mark BEFORE the inserted text - so moving a block down
	// re-anchored the region onto the line that had just been moved above it, and
	// a second press then moved the wrong lines. Setting both ends explicitly is
	// predictable and does not depend on that boundary rule.
	ptBefore, markBefore := w.Pt, b.Mark()
	hadMark := b.HasMark()

	if err := deleteLine(b, donor); err != nil {
		return err
	}
	if err := insertLine(b, target, content); err != nil {
		return err
	}

	w.Pt = b.ClampPos(text.Pos{Line: ptBefore.Line + dir, Col: ptBefore.Col})
	if hadMark {
		b.SetMark(b.ClampPos(text.Pos{Line: markBefore.Line + dir, Col: markBefore.Col}))
	}
	return nil
}

// deleteLine removes line i along with the newline separating it from its
// neighbour, so the line count drops by one.
func deleteLine(b *text.Buffer, i int) error {
	if i < b.NumLines()-1 {
		return b.Delete(text.Pos{Line: i}, text.Pos{Line: i + 1})
	}
	// The last line has no newline after it, so take the one before it.
	prev := i - 1
	return b.Delete(
		text.Pos{Line: prev, Col: b.Line(prev).Len()},
		text.Pos{Line: i, Col: b.Line(i).Len()},
	)
}

// insertLine inserts s so that it becomes line i, raising the line count by one.
func insertLine(b *text.Buffer, i int, s string) error {
	if i < b.NumLines() {
		return b.Insert(text.Pos{Line: i}, []rune(s+"\n"))
	}
	// Past the end: append after the current last line instead.
	last := b.NumLines() - 1
	return b.Insert(text.Pos{Line: last, Col: b.Line(last).Len()}, []rune("\n"+s))
}
