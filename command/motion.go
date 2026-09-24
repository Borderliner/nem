package command

import (
	"strconv"
	"strings"

	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// RegisterMotion adds nem's motion commands to r.
//
// Two rules govern this file and are easy to break by accident:
//
// Character motion moves by grapheme cluster, never by rune. A combining
// sequence or a ZWJ emoji is one cursor stop however many runes it holds, so
// forward-char and backward-char go through text.Line's grapheme helpers rather
// than incrementing a rune index.
//
// Vertical motion preserves the goal column and every other command clears it.
// The goal column is the display column the cursor is trying to keep, so that
// descending through a short line and out the other side returns to the
// original column rather than to the short line's end. It is established on the
// first vertical move of a run, preserved by later ones, and cleared by
// everything else — which is why a new command added here must call clearGoal
// unless it is itself vertical motion.
func RegisterMotion(r *Registry) error {
	for _, c := range []Command{
		{Name: "forward-char", Doc: "Move point one grapheme forward.", Fn: forwardChar, Interactive: true},
		{Name: "backward-char", Doc: "Move point one grapheme backward.", Fn: backwardChar, Interactive: true},
		{Name: "next-line", Doc: "Move point down one line, keeping the goal column.", Fn: nextLine, Interactive: true},
		{Name: "previous-line", Doc: "Move point up one line, keeping the goal column.", Fn: previousLine, Interactive: true},
		{Name: "forward-word", Doc: "Move point to the end of the next word.", Fn: forwardWord, Interactive: true},
		{Name: "backward-word", Doc: "Move point to the start of the previous word.", Fn: backwardWord, Interactive: true},
		{Name: "move-beginning-of-line", Doc: "Move point to the start of the line.", Fn: moveBeginningOfLine, Interactive: true},
		{Name: "move-end-of-line", Doc: "Move point to the end of the line.", Fn: moveEndOfLine, Interactive: true},
		{Name: "beginning-of-buffer", Doc: "Move point to the start of the buffer.", Fn: beginningOfBuffer, Interactive: true},
		{Name: "end-of-buffer", Doc: "Move point to the end of the buffer.", Fn: endOfBuffer, Interactive: true},
		{Name: "scroll-up-command", Doc: "Move forward one screenful.", Fn: scrollUpCommand, Interactive: true},
		{Name: "scroll-down-command", Doc: "Move backward one screenful.", Fn: scrollDownCommand, Interactive: true},
		{Name: "goto-line", Doc: "Move point to the start of a numbered line.", Fn: gotoLine, Interactive: true},
		{Name: "recenter-top-bottom", Doc: "Scroll point to the centre, then the top, then the bottom.", Fn: recenterTopBottom, Interactive: true},
	} {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// clearGoal drops the goal column so that the next vertical motion establishes
// a fresh one from wherever point now is.
func clearGoal(w *view.Window) { w.GoalCol = view.GoalColUnset }

// --- character motion -------------------------------------------------------

func forwardChar(e Env) error {
	n, _ := e.Arg()
	w := e.Win()
	if n < 0 {
		charBackward(w, -n)
	} else {
		charForward(w, n)
	}
	clearGoal(w)
	return nil
}

func backwardChar(e Env) error {
	n, _ := e.Arg()
	w := e.Win()
	if n < 0 {
		charForward(w, -n)
	} else {
		charBackward(w, n)
	}
	clearGoal(w)
	return nil
}

// charForward advances point n grapheme clusters, crossing line boundaries and
// stopping silently at the end of the buffer. Running off the end is ordinary
// use, not an error.
func charForward(w *view.Window, n int) {
	b := w.Buf
	for ; n > 0; n-- {
		ln := b.Line(w.Pt.Line)
		if w.Pt.Col < ln.Len() {
			w.Pt.Col = ln.NextGrapheme(w.Pt.Col)
			continue
		}
		if w.Pt.Line >= b.NumLines()-1 {
			return
		}
		w.Pt.Line++
		w.Pt.Col = 0
	}
}

// charBackward retreats point n grapheme clusters, stopping at the origin.
func charBackward(w *view.Window, n int) {
	b := w.Buf
	for ; n > 0; n-- {
		if w.Pt.Col > 0 {
			w.Pt.Col = b.Line(w.Pt.Line).PrevGrapheme(w.Pt.Col)
			continue
		}
		if w.Pt.Line == 0 {
			return
		}
		w.Pt.Line--
		w.Pt.Col = b.Line(w.Pt.Line).Len()
	}
}

// --- vertical motion --------------------------------------------------------

// nextLine and previousLine report running into the end or the start of the
// buffer, as emacs does, though point still goes as far as it can. The
// sentinels are silent at the keyboard; what they are for is a keyboard macro,
// which stops at them - C-u 0 F4 running a C-n macro to the last line.
func nextLine(e Env) error {
	n, _ := e.Arg()
	if !lineDelta(e.Win(), n) {
		return ErrEndOfBuffer
	}
	return nil
}

func previousLine(e Env) error {
	n, _ := e.Arg()
	if !lineDelta(e.Win(), -n) {
		return ErrBeginningOfBuffer
	}
	return nil
}

// lineDelta moves point n lines while holding the goal column, establishing it
// first if this is the start of a vertical run. RuneAt clamps a goal past the
// end of a short line to that line's end without losing the goal itself, which
// is what makes the descend-and-return case work.
//
// It reports whether point went all n lines, rather than stopping at an end
// of the buffer.
func lineDelta(w *view.Window, n int) bool {
	b := w.Buf
	if w.GoalCol == view.GoalColUnset {
		w.GoalCol = b.Line(w.Pt.Line).DisplayCol(w.Pt.Col)
	}
	target := w.Pt.Line + n
	full := true
	if target < 0 {
		target, full = 0, false
	}
	if last := b.NumLines() - 1; target > last {
		target, full = last, false
	}
	w.Pt.Line = target
	w.Pt.Col = b.Line(target).RuneAt(w.GoalCol)
	return full
}

// --- word motion ------------------------------------------------------------

func forwardWord(e Env) error {
	n, _ := e.Arg()
	w := e.Win()
	if n < 0 {
		wordBackward(w, -n)
	} else {
		wordForward(w, n)
	}
	clearGoal(w)
	return nil
}

func backwardWord(e Env) error {
	n, _ := e.Arg()
	w := e.Win()
	if n < 0 {
		wordForward(w, -n)
	} else {
		wordBackward(w, n)
	}
	clearGoal(w)
	return nil
}

// wordForward advances to the end of the nth word ahead, stopping early if it
// runs out of buffer.
//
// The scan itself lives in words.go, shared with the kill-word commands, so that
// forward-word and kill-word cannot disagree about where a word ends.
func wordForward(w *view.Window, n int) {
	for ; n > 0; n-- {
		next := forwardWordPos(w.Buf, w.Pt)
		if next.Equal(w.Pt) {
			return
		}
		w.Pt = next
	}
}

// wordBackward retreats to the start of the nth word behind.
func wordBackward(w *view.Window, n int) {
	for ; n > 0; n-- {
		prev := backwardWordPos(w.Buf, w.Pt)
		if prev.Equal(w.Pt) {
			return
		}
		w.Pt = prev
	}
}

// --- line and buffer ends ---------------------------------------------------

func moveBeginningOfLine(e Env) error {
	w := e.Win()
	n, _ := e.Arg()
	lineOffset(w, n)
	w.Pt.Col = 0
	clearGoal(w)
	return nil
}

func moveEndOfLine(e Env) error {
	w := e.Win()
	n, _ := e.Arg()
	lineOffset(w, n)
	w.Pt.Col = w.Buf.Line(w.Pt.Line).Len()
	clearGoal(w)
	return nil
}

// lineOffset moves point n-1 lines, so that an argument of 1 — or none at all —
// leaves it on the current line. This is how emacs's C-a and C-e read an
// argument: C-u 3 C-a goes to the start of the line two below.
func lineOffset(w *view.Window, n int) {
	target := w.Pt.Line + n - 1
	if target < 0 {
		target = 0
	}
	if last := w.Buf.NumLines() - 1; target > last {
		target = last
	}
	w.Pt.Line = target
}

// beginningOfBuffer and endOfBuffer push the mark before jumping, as emacs
// does, so that C-x C-x takes the reader back to where they were. A jump across
// a whole buffer is exactly the motion worth being able to undo, and losing your
// place to it is immediately noticeable.
//
// The mark is pushed without being activated - a jump is navigation, not a
// selection. When these set an active mark, M-> M-< followed by one typed
// character deleted the entire file.
//
// With a region already active the mark is left alone and the jump extends the
// selection instead, which is how emacs does "select from here to the end":
// C-SPC then M->. Re-anchoring the mark would silently discard where the user
// started selecting.

func beginningOfBuffer(e Env) error {
	w := e.Win()
	if !w.Buf.MarkActive() {
		w.Buf.SetMark(w.Pt)
	}
	w.Pt = text.Pos{}
	clearGoal(w)
	return nil
}

func endOfBuffer(e Env) error {
	w := e.Win()
	if !w.Buf.MarkActive() {
		w.Buf.SetMark(w.Pt)
	}
	w.Pt = w.Buf.End()
	clearGoal(w)
	return nil
}

// --- scrolling --------------------------------------------------------------

func scrollUpCommand(e Env) error   { return scrollBy(e, 1) }
func scrollDownCommand(e Env) error { return scrollBy(e, -1) }

// scrollBy moves point and the viewport one screenful in direction dir, keeping
// two lines of overlap so the reader has context across the jump. An explicit
// argument scrolls that many lines instead of a screenful.
func scrollBy(e Env, dir int) error {
	n, explicit := e.Arg()
	lines := e.TextHeight() - 2
	if lines < 1 {
		lines = 1
	}
	if explicit {
		lines = n
	}
	lines *= dir

	w := e.Win()
	b := w.Buf
	last := b.NumLines() - 1

	target := w.Pt.Line + lines
	if target < 0 {
		target = 0
	}
	if target > last {
		target = last
	}
	w.Pt.Line = target
	w.Pt = b.ClampPos(w.Pt)

	// Move the viewport with point so its position on screen is roughly
	// preserved; the render pass reconciles anything left inconsistent.
	top := w.Top + lines
	if top < 0 {
		top = 0
	}
	if top > last {
		top = last
	}
	w.Top = top

	clearGoal(w)
	return nil
}

// --- goto-line --------------------------------------------------------------

// gotoLine jumps to a line by its one-based number, prompting unless an
// argument supplied it. Out-of-range numbers clamp into the buffer, and
// unparseable input is reported in the echo area rather than returned as an
// error: a typo is not a failure worth unwinding.
func gotoLine(e Env) error {
	n, explicit := e.Arg()
	if !explicit {
		s, err := e.ReadString(ReadOpts{Prompt: "Goto line: ", History: "line"})
		if err != nil {
			return err
		}
		v, convErr := strconv.Atoi(strings.TrimSpace(s))
		if convErr != nil {
			e.Echo("Not a number: %s", s)
			return nil
		}
		n = v
	}

	w := e.Win()
	target := n - 1
	if target < 0 {
		target = 0
	}
	if last := w.Buf.NumLines() - 1; target > last {
		target = last
	}
	// Where it jumped from is kept, for C-u C-SPC to come back to.
	w.Buf.SetMark(w.Pt)
	w.Pt = text.Pos{Line: target}
	clearGoal(w)
	return nil
}

// --- recentring -------------------------------------------------------------

// recenterTopBottom scrolls the window so point sits at the centre, then the
// top, then the bottom on successive presses, as emacs's C-l does. Any other
// command in between restarts the cycle at the centre, which is why it consults
// LastCommand rather than only its own stored position.
func recenterTopBottom(e Env) error {
	w := e.Win()
	seq := e.Seq()
	if e.LastCommand() == "recenter-top-bottom" {
		seq.RecenterCycle = (seq.RecenterCycle + 1) % 3
	} else {
		seq.RecenterCycle = 0
	}

	h := e.TextHeight()
	if h < 1 {
		h = 1
	}

	var top int
	switch seq.RecenterCycle {
	case 0:
		top = w.Pt.Line - (h-1)/2
	case 1:
		top = w.Pt.Line
	default:
		top = w.Pt.Line - h + 1
	}
	if top < 0 {
		top = 0
	}
	w.Top = top

	clearGoal(w)
	return nil
}
