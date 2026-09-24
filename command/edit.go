package command

import (
	"errors"
	"strings"
	"unicode"

	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// Editing commands: everything that changes buffer contents.
//
// Two rules govern this file. First, every mutation goes through the buffer's
// Insert and Delete primitives, so undo, mark adjustment and the modified flag
// are maintained in one place rather than remembered here. Second, deletion and
// killing are different operations: emacs deletes small amounts of text without
// disturbing the kill ring, and only the kill commands record anything, so
// delete-char must not be "a kill of one character".

// errEditNoTranspose stays private: it has a single caller, and a boundary the
// dispatcher needs to recognise is a different thing from a local failure.
var (
	errEditNoTranspose = errors.New("nothing to transpose")

	// errEditNoSelfInsertKey reports that self-insert-command ran without the
	// event loop having recorded which key triggered it.
	errEditNoSelfInsertKey = errors.New("no self-inserting key recorded")
)

// RegisterEdit adds the editing commands to r.
func RegisterEdit(r *Registry) error {
	cmds := []Command{
		{Name: "delete-char", Doc: "Delete the character after point.", Fn: deleteChar, Interactive: true},
		{Name: "delete-backward-char", Doc: "Delete the character before point.", Fn: deleteBackwardChar, Interactive: true},
		{Name: "kill-word", Doc: "Kill forward to the end of the next word.", Fn: killWord, Interactive: true},
		{Name: "backward-kill-word", Doc: "Kill backward to the start of the previous word.", Fn: backwardKillWord, Interactive: true},
		{Name: "kill-line", Doc: "Kill to the end of the line, or the newline if already there.", Fn: killLine, Interactive: true},
		{Name: "open-line", Doc: "Insert a newline after point, leaving point in place.", Fn: openLine, Interactive: true},
		{Name: "transpose-chars", Doc: "Transpose the characters around point.", Fn: transposeChars, Interactive: true},
		{Name: "transpose-words", Doc: "Transpose the words around point.", Fn: transposeWords, Interactive: true},
		{Name: "newline", Doc: "Insert a newline, copying the current line's indentation.", Fn: newline, Interactive: true},
		{Name: "indent-for-tab-command", Doc: "Indent by inserting a tab.", Fn: indentForTab, Interactive: true},
		{Name: "upcase-word", Doc: "Convert the following word to upper case.", Fn: upcaseWord, Interactive: true},
		{Name: "downcase-word", Doc: "Convert the following word to lower case.", Fn: downcaseWord, Interactive: true},
		{Name: "capitalize-word", Doc: "Capitalize the following word.", Fn: capitalizeWord, Interactive: true},
		{Name: "self-insert-command", Doc: "Insert the character just typed.", Fn: selfInsert, Interactive: true},
	}
	for _, c := range cmds {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// --- point helpers ---------------------------------------------------------

// edSetPoint moves point, clamping it into the buffer and clearing the goal
// column. Editing is not vertical motion, so the column C-n would aim for is no
// longer meaningful once the text has changed.
func edSetPoint(e Env, p text.Pos) {
	w := e.Win()
	w.Pt = e.Buf().ClampPos(p)
	w.GoalCol = view.GoalColUnset
}

// edAdvance returns the position reached by writing rs starting at p.
func edAdvance(p text.Pos, rs []rune) text.Pos {
	line, col := p.Line, p.Col
	for _, r := range rs {
		if r == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return text.Pos{Line: line, Col: col}
}

// edNextGrapheme returns the position one grapheme after p, crossing into the
// next line when p is at a line end. ok is false at the end of the buffer.
func edNextGrapheme(b *text.Buffer, p text.Pos) (next text.Pos, ok bool) {
	ln := b.Line(p.Line)
	if p.Col < ln.Len() {
		return text.Pos{Line: p.Line, Col: ln.NextGrapheme(p.Col)}, true
	}
	if p.Line+1 < b.NumLines() {
		return text.Pos{Line: p.Line + 1, Col: 0}, true
	}
	return p, false
}

// edPrevGrapheme returns the position one grapheme before p, crossing into the
// previous line when p is at column zero. ok is false at the buffer start.
func edPrevGrapheme(b *text.Buffer, p text.Pos) (prev text.Pos, ok bool) {
	if p.Col > 0 {
		ln := b.Line(p.Line)
		return text.Pos{Line: p.Line, Col: ln.PrevGrapheme(p.Col)}, true
	}
	if p.Line > 0 {
		above := b.Line(p.Line - 1)
		return text.Pos{Line: p.Line - 1, Col: above.Len()}, true
	}
	return p, false
}

// edForwardGraphemes returns the position n graphemes after p, stopping at the
// end of the buffer.
func edForwardGraphemes(b *text.Buffer, p text.Pos, n int) text.Pos {
	for i := 0; i < n; i++ {
		next, ok := edNextGrapheme(b, p)
		if !ok {
			break
		}
		p = next
	}
	return p
}

// edBackwardGraphemes returns the position n graphemes before p, stopping at the
// start of the buffer.
func edBackwardGraphemes(b *text.Buffer, p text.Pos, n int) text.Pos {
	for i := 0; i < n; i++ {
		prev, ok := edPrevGrapheme(b, p)
		if !ok {
			break
		}
		p = prev
	}
	return p
}

// --- word helpers ----------------------------------------------------------

// Word scanning itself lives in words.go and is shared with the motion
// commands, so M-f and M-d can never disagree about where a word ends.
//
// edSkipNonWord is the one thing the shared scanners cannot express: the case
// commands act on the word from point onward, so from mid-word they need point
// itself rather than the start of the whole word. Composing the shared pair as
// backwardWordPos(forwardWordPos(p)) would return the word's start and turn
// "heLlo" into "Hello".
func edSkipNonWord(b *text.Buffer, p text.Pos) text.Pos {
	end := b.End()
	for p.Before(end) && !isWordRune(runeAt(b, p)) {
		p = nextRune(b, p)
	}
	return p
}

// --- deletion --------------------------------------------------------------

func deleteChar(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return edDeleteBackward(e, -n)
	}
	return edDeleteForward(e, n)
}

func deleteBackwardChar(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return edDeleteForward(e, -n)
	}
	if n == 1 {
		if done, err := autoPairDelete(e); done || err != nil {
			return err
		}
	}
	return edDeleteBackward(e, n)
}

func edDeleteForward(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	to := edForwardGraphemes(b, p, n)
	if to.Equal(p) {
		return ErrEndOfBuffer
	}
	if err := b.Delete(p, to); err != nil {
		return err
	}
	edSetPoint(e, p)
	return nil
}

func edDeleteBackward(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	from := edBackwardGraphemes(b, p, n)
	if from.Equal(p) {
		return ErrBeginningOfBuffer
	}
	if err := b.Delete(from, p); err != nil {
		return err
	}
	edSetPoint(e, from)
	return nil
}

// --- killing ---------------------------------------------------------------

// Both kill-word and backward-kill-word delegate to the explicit-count helpers
// rather than to each other. Delegating between the commands would re-read the
// prefix argument, negating it a second time and killing in the wrong
// direction.
func killWord(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return edKillWordsBackward(e, -n)
	}
	return edKillWordsForward(e, n)
}

func backwardKillWord(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return edKillWordsForward(e, -n)
	}
	return edKillWordsBackward(e, n)
}

func edKillWordsForward(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	to := p
	for i := 0; i < n; i++ {
		to = forwardWordPos(b, to)
	}
	if to.Equal(p) {
		return ErrEndOfBuffer
	}
	killed := string(b.Text(p, to))
	if err := b.Delete(p, to); err != nil {
		return err
	}
	e.KillForward(killed)
	edSetPoint(e, p)
	return nil
}

func edKillWordsBackward(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	from := p
	for i := 0; i < n; i++ {
		from = backwardWordPos(b, from)
	}
	if from.Equal(p) {
		return ErrBeginningOfBuffer
	}
	killed := string(b.Text(from, p))
	if err := b.Delete(from, p); err != nil {
		return err
	}
	// Backward, so the ring extends the newest entry on its left and repeated
	// M-DEL yields reading order rather than reversed text.
	e.KillBackward(killed)
	edSetPoint(e, from)
	return nil
}

// killLine implements C-k.
//
// Without an argument it kills to the end of the line but leaves the newline,
// so a second press on the now-empty line kills the newline and joins the
// following line up. With an argument it kills whole lines, newlines included.
func killLine(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	n, explicit := e.Arg()

	if explicit && n <= 0 {
		// Zero kills back to the start of the line; negative kills whole lines
		// backward, as emacs does.
		from := text.Pos{Line: p.Line + n, Col: 0}
		if from.Line < 0 {
			from = text.Pos{Line: 0, Col: 0}
		}
		if from.Equal(p) {
			return ErrBeginningOfBuffer
		}
		killed := string(b.Text(from, p))
		if err := b.Delete(from, p); err != nil {
			return err
		}
		e.KillBackward(killed)
		edSetPoint(e, from)
		return nil
	}

	var to text.Pos
	switch {
	case explicit:
		if p.Line+n >= b.NumLines() {
			to = b.End()
		} else {
			to = text.Pos{Line: p.Line + n, Col: 0}
		}
	default:
		ln := b.Line(p.Line)
		switch {
		case p.Col < ln.Len():
			to = text.Pos{Line: p.Line, Col: ln.Len()}
		case p.Line+1 < b.NumLines():
			to = text.Pos{Line: p.Line + 1, Col: 0}
		default:
			return ErrEndOfBuffer
		}
	}

	if to.Equal(p) {
		return ErrEndOfBuffer
	}
	killed := string(b.Text(p, to))
	if err := b.Delete(p, to); err != nil {
		return err
	}
	e.KillForward(killed)
	edSetPoint(e, p)
	return nil
}

// --- insertion -------------------------------------------------------------

func openLine(e Env) error {
	n, _ := e.Arg()
	if n < 1 {
		n = 1
	}
	b, p := e.Buf(), e.Win().Pt
	if err := b.Insert(p, []rune(strings.Repeat("\n", n))); err != nil {
		return err
	}
	// Point stays put: that is the whole difference between C-o and RET.
	edSetPoint(e, p)
	return nil
}

// edLeadingIndent returns the leading whitespace of line i.
func edLeadingIndent(b *text.Buffer, i int) []rune {
	rs := b.Line(i).View()
	end := 0
	for end < len(rs) && (rs[end] == ' ' || rs[end] == '\t') {
		end++
	}
	// A copy of the indent alone: the caller inserts it, into this same line
	// among others, and must not be handed the line's own array.
	return append([]rune(nil), rs[:end]...)
}

// newline inserts a newline and copies the current line's indentation onto the
// new line.
//
// The indentation copied is the whole leading whitespace of the line, measured
// from its start rather than from point, so splitting a line inside its own
// indentation still reproduces that indentation below. Tabs and spaces are
// copied verbatim rather than normalised to either one.
func newline(e Env) error {
	n, _ := e.Arg()
	if n < 1 {
		n = 1
	}
	b, p := e.Buf(), e.Win().Pt
	indent := edLeadingIndent(b, p.Line)

	ins := make([]rune, 0, n*(1+len(indent)))
	for i := 0; i < n; i++ {
		ins = append(ins, '\n')
		ins = append(ins, indent...)
	}
	if err := b.Insert(p, ins); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(p, ins))
	return nil
}

// indentForTab inserts a tab.
//
// Language-aware indentation is deliberately deferred: it needs the syntax
// knowledge that arrives with highlighting, and guessing without it produces
// indentation that is wrong in a way users cannot correct.
func indentForTab(e Env) error {
	n, _ := e.Arg()
	if n < 1 {
		n = 1
	}
	b, p := e.Buf(), e.Win().Pt
	ins := []rune(strings.Repeat("\t", n))
	if err := b.Insert(p, ins); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(p, ins))
	return nil
}

// selfInsert inserts the key that triggered the command, as many times as the
// prefix argument says.
//
// The rune comes from Seq().LastRune, which the event loop records before
// dispatch. That is what lets this be an ordinary registered command — bindable
// from Lua and reachable from M-x — rather than a stub only the event loop can
// reach.
func selfInsert(e Env) error {
	r := e.Seq().LastRune
	if r == 0 {
		return errEditNoSelfInsertKey
	}
	n, _ := e.Arg()
	if n < 1 {
		n = 1
	}
	if n == 1 {
		if done, err := autoPairInsert(e, r); done || err != nil {
			return err
		}
	}
	b, p := e.Buf(), e.Win().Pt

	// Inserted as one operation so a repeat count is a single undo unit; a lone
	// rune still coalesces with preceding typing inside the undo log.
	ins := make([]rune, n)
	for i := range ins {
		ins[i] = r
	}
	if err := b.Insert(p, ins); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(p, ins))
	return nil
}

// --- transposition ---------------------------------------------------------

// transposeChars swaps the characters around point and moves point past them.
// At the end of a line it swaps the two preceding characters instead of
// erroring, which is what makes C-t useful while typing.
func transposeChars(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	ln := b.Line(p.Line)

	var aStart, aEnd, bEnd text.Pos
	if p.Col >= ln.Len() {
		bEnd = text.Pos{Line: p.Line, Col: ln.Len()}
		aEnd = text.Pos{Line: p.Line, Col: ln.PrevGrapheme(ln.Len())}
		if aEnd.Col == 0 && bEnd.Col == 0 {
			return errEditNoTranspose
		}
		aStart = text.Pos{Line: p.Line, Col: ln.PrevGrapheme(aEnd.Col)}
		if aStart.Equal(aEnd) {
			return errEditNoTranspose
		}
	} else {
		if p.Col == 0 {
			return errEditNoTranspose
		}
		aStart = text.Pos{Line: p.Line, Col: ln.PrevGrapheme(p.Col)}
		aEnd = p
		bEnd = text.Pos{Line: p.Line, Col: ln.NextGrapheme(p.Col)}
	}

	first := b.Text(aStart, aEnd)
	second := b.Text(aEnd, bEnd)
	if len(first) == 0 || len(second) == 0 {
		return errEditNoTranspose
	}

	swapped := append(append([]rune{}, second...), first...)
	if err := b.Delete(aStart, bEnd); err != nil {
		return err
	}
	if err := b.Insert(aStart, swapped); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(aStart, swapped))
	return nil
}

// transposeWords swaps the word before point with the word after it. The words
// may be separated by a newline; whatever lies between them is preserved.
func transposeWords(e Env) error {
	b, p := e.Buf(), e.Win().Pt

	secondEnd := forwardWordPos(b, p)
	secondStart := backwardWordPos(b, secondEnd)
	firstStart := backwardWordPos(b, secondStart)
	firstEnd := forwardWordPos(b, firstStart)

	if firstStart.Equal(secondStart) || firstEnd.After(secondStart) {
		return errEditNoTranspose
	}

	first := b.Text(firstStart, firstEnd)
	middle := b.Text(firstEnd, secondStart)
	second := b.Text(secondStart, secondEnd)
	if len(first) == 0 || len(second) == 0 {
		return errEditNoTranspose
	}

	swapped := make([]rune, 0, len(first)+len(middle)+len(second))
	swapped = append(swapped, second...)
	swapped = append(swapped, middle...)
	swapped = append(swapped, first...)

	if err := b.Delete(firstStart, secondEnd); err != nil {
		return err
	}
	if err := b.Insert(firstStart, swapped); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(firstStart, swapped))
	return nil
}

// --- case ------------------------------------------------------------------

func upcaseWord(e Env) error     { return edCaseWords(e, strings.ToUpper) }
func downcaseWord(e Env) error   { return edCaseWords(e, strings.ToLower) }
func capitalizeWord(e Env) error { return edCaseWords(e, edCapitalize) }

// edCapitalize upper-cases the first rune and lower-cases the rest.
func edCapitalize(s string) string {
	rs := []rune(strings.ToLower(s))
	if len(rs) > 0 {
		rs[0] = unicode.ToTitle(rs[0])
	}
	return string(rs)
}

// edCaseWords applies fn to n words from point, leaving point after the last one.
//
// Point starting inside a word acts on the remainder of that word, as emacs
// does, because the word is taken from point rather than from its start.
//
// A negative argument acts on the preceding words and leaves point at the start
// of the leftmost one. Emacs instead leaves point untouched; tracking it through
// a case mapping that can change length — "ß" upcases to "SS" — is not worth the
// complexity for an argument this rarely used.
func edCaseWords(e Env, fn func(string) string) error {
	n, _ := e.Arg()
	if n == 0 {
		return nil
	}
	b := e.Buf()

	if n < 0 {
		cur := e.Win().Pt
		for i := 0; i < -n; i++ {
			start := backwardWordPos(b, cur)
			end := forwardWordPos(b, start)
			if end.After(cur) {
				end = cur
			}
			if start.Equal(end) {
				break
			}
			if _, err := edReplaceRange(b, start, end, fn); err != nil {
				return err
			}
			cur = start
		}
		edSetPoint(e, cur)
		return nil
	}

	cur := e.Win().Pt
	for i := 0; i < n; i++ {
		start := edSkipNonWord(b, cur)
		end := forwardWordPos(b, cur)
		if start.Equal(end) {
			break
		}
		updated, err := edReplaceRange(b, start, end, fn)
		if err != nil {
			return err
		}
		// Advanced over the replacement rather than to the old end, since a
		// case mapping may change length: "ß" upcases to "SS".
		cur = b.ClampPos(edAdvance(start, updated))
	}
	edSetPoint(e, cur)
	return nil
}

// edReplaceRange rewrites [from, to) as fn of its current contents and returns
// the replacement, doing nothing when the mapping leaves the text unchanged.
func edReplaceRange(b *text.Buffer, from, to text.Pos, fn func(string) string) ([]rune, error) {
	old := string(b.Text(from, to))
	updated := fn(old)
	if updated == old {
		return []rune(old), nil
	}
	if err := b.Delete(from, to); err != nil {
		return nil, err
	}
	if err := b.Insert(from, []rune(updated)); err != nil {
		return nil, err
	}
	return []rune(updated), nil
}
