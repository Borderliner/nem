package command

import (
	"errors"

	"github.com/Borderliner/nem/text"
)

// RegisterRegion adds the mark, region, kill-ring and undo commands.
func RegisterRegion(r *Registry) error {
	for _, c := range []Command{
		{
			Name:        "set-mark-command",
			Doc:         "Set the mark where point is.",
			Fn:          setMarkCommand,
			Interactive: true,
		},
		{
			Name:        "exchange-point-and-mark",
			Doc:         "Put point where the mark is, and the mark where point was.",
			Fn:          exchangePointAndMark,
			Interactive: true,
		},
		{
			Name:        "kill-region",
			Doc:         "Kill the text between point and mark, saving it on the kill ring.",
			Fn:          killRegion,
			Interactive: true,
		},
		{
			Name:        "kill-ring-save",
			Doc:         "Save the text between point and mark on the kill ring without removing it.",
			Fn:          killRingSave,
			Interactive: true,
		},
		{
			Name:        "yank",
			Doc:         "Insert the current kill-ring entry at point.",
			Fn:          yank,
			Interactive: true,
		},
		{
			Name:        "yank-pop",
			Doc:         "Replace the text just yanked with the next-older kill-ring entry.",
			Fn:          yankPop,
			Interactive: true,
		},
		{
			Name:        "undo",
			Doc:         "Undo the most recent change.",
			Fn:          undo,
			Interactive: true,
		},
		{
			Name:        "redo",
			Doc:         "Redo the change most recently undone.",
			Fn:          redo,
			Interactive: true,
		},
	} {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// Messages emacs reports for the two out-of-sequence mistakes in this file.
const (
	notAYank  = "Previous command was not a yank"
	noMarkSet = "No mark set in this buffer"
)

// requireMark reports whether the buffer has a mark, telling the user if it does
// not.
//
// Refusals here go through Echo rather than returning ErrNoMark, matching the
// other user-facing refusal in this file: reaching for a region before setting a
// mark is a mistake, not a failure, and one mechanism is better than two.
// ErrNoMark stays declared for a caller that needs to distinguish this case
// programmatically.
//
// Every command that reads the region must call this FIRST and make no
// modification when it returns false. Without a mark, kill-region would
// otherwise treat the buffer origin as one endpoint and silently kill everything
// up to point — destructive by default, which is the bug this guard exists for.
func requireMark(e Env) bool {
	if e.Buf().HasMark() {
		return true
	}
	e.Echo(noMarkSet)
	return false
}

// region returns the endpoints of the region between point and mark, ordered,
// so that a command need not care which side of point the user set the mark on.
//
// Callers must have checked requireMark; a mark deliberately set at the buffer
// origin is a real mark, which is why the check consults HasMark rather than
// comparing Mark against the zero position.
func region(e Env) (lo, hi text.Pos) {
	return text.OrderPos(e.Win().Pt, e.Buf().Mark())
}

// advance returns the position just past rs when inserted at at.
//
// text.Buffer.Insert reports only an error, so the end of an insertion has to be
// computed. yank and yank-pop both need it to record the extent they inserted.
func advance(at text.Pos, rs []rune) text.Pos {
	lastNL := -1
	newlines := 0
	for i, r := range rs {
		if r == '\n' {
			newlines++
			lastNL = i
		}
	}
	if newlines == 0 {
		return text.Pos{Line: at.Line, Col: at.Col + text.RuneIdx(len(rs))}
	}
	return text.Pos{
		Line: at.Line + newlines,
		Col:  text.RuneIdx(len(rs) - lastNL - 1),
	}
}

// set-mark-command.
//
// Its binding C-SPC reaches the terminal as NUL and is folded to C-@ by
// keymap.Normalize, so nothing here has to know about that encoding.
func setMarkCommand(e Env) error {
	e.Buf().SetMark(e.Win().Pt)
	e.Echo("Mark set")
	return nil
}

func exchangePointAndMark(e Env) error {
	if !requireMark(e) {
		return nil
	}
	b, w := e.Buf(), e.Win()
	mark := b.Mark()
	b.SetMark(w.Pt)
	w.Pt = b.ClampPos(mark)
	return nil
}

func killRegion(e Env) error {
	if !requireMark(e) {
		return nil
	}
	lo, hi := region(e)
	b := e.Buf()

	// The ring decides whether this extends a kill run or starts a new entry,
	// and an empty region kills nothing at all.
	e.KillForward(string(b.Text(lo, hi)))

	if err := b.Delete(lo, hi); err != nil {
		return err
	}
	e.Win().Pt = lo
	return nil
}

// killRingSave is M-w: it copies the region and must leave the buffer and point
// exactly as they were.
func killRingSave(e Env) error {
	if !requireMark(e) {
		return nil
	}
	lo, hi := region(e)
	e.KillForward(string(e.Buf().Text(lo, hi)))
	return nil
}

func yank(e Env) error {
	s, err := e.Yank()
	if err != nil {
		return err
	}
	return insertYanked(e, e.Win().Pt, s)
}

// yankPop replaces the text the preceding yank inserted with the ring's
// next-older entry.
//
// It cannot work without knowing what the previous yank put in the buffer,
// which is what Seq.LastYankFrom/LastYankTo carry. Three separate conditions
// have to hold, and each reports through the echo area rather than returning an
// error, because using M-y out of sequence is a mistake rather than a failure:
// the previous command must have been a yank, that yank's extent must still be
// recorded, and the ring's own yank state must still be live — dispatch may
// have broken the run even when the command history looks right.
func yankPop(e Env) error {
	if last := e.LastCommand(); last != "yank" && last != "yank-pop" {
		e.Echo(notAYank)
		return nil
	}

	seq := e.Seq()
	if !seq.HasLastYank {
		e.Echo(notAYank)
		return nil
	}

	s, err := e.YankPop()
	if err != nil {
		if errors.Is(err, ErrNotAfterYank) {
			e.Echo(notAYank)
			return nil
		}
		return err
	}

	from := seq.LastYankFrom
	if err := e.Buf().Delete(from, seq.LastYankTo); err != nil {
		return err
	}
	return insertYanked(e, from, s)
}

// insertYanked puts s at at, leaves point after it, and records the extent so a
// following yank-pop can replace exactly this text.
//
// The mark is left at the start of the insertion, as emacs does, so C-x C-x
// selects what was just yanked.
func insertYanked(e Env, at text.Pos, s string) error {
	rs := []rune(s)
	b := e.Buf()
	if err := b.Insert(at, rs); err != nil {
		return err
	}
	end := advance(at, rs)

	b.SetMark(at)
	e.Win().Pt = end

	seq := e.Seq()
	seq.LastYankFrom = at
	seq.LastYankTo = end
	seq.HasLastYank = true
	return nil
}

func undo(e Env) error { return undoOrRedo(e, true) }
func redo(e Env) error { return undoOrRedo(e, false) }

// undoOrRedo steps the undo log, honouring the universal argument as a repeat
// count, and reports exhaustion through the echo area rather than as an error.
//
// Undo is linear by design: a fresh edit after an undo discards the redo
// branch. That decision lives in text's undo log, not here.
//
// Nothing here breaks the kill run or clears the window's goal column. Both
// belong to the central dispatcher, which does them for every command, so that
// no single command can forget.
func undoOrRedo(e Env, backward bool) error {
	n, _ := e.Arg()
	if n < 1 {
		// A negative or zero repeat count has no sensible meaning for undo, so
		// it degrades to a single step rather than silently doing nothing.
		n = 1
	}

	b, w := e.Buf(), e.Win()
	for range n {
		var (
			pos text.Pos
			ok  bool
		)
		if backward {
			pos, ok = b.Undo()
		} else {
			pos, ok = b.Redo()
		}
		if !ok {
			if backward {
				e.Echo("No further undo information")
			} else {
				e.Echo("No further redo information")
			}
			return nil
		}
		w.Pt = b.ClampPos(pos)
	}
	return nil
}
