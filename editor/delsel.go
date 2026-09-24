package editor

import (
	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
)

// Delete-selection: typing or deleting with a region active replaces it.
//
// Emacs calls this delete-selection-mode and ships it disabled; every other
// modern editor has it on, and without it the region is a thing you can see and
// cut but not type over. After C-x h, Backspace did nothing at all — point sat
// at the buffer start with no character before it — so the selection was simply
// ignored.
//
// It lives in dispatch with the kill run, the goal column and point clamping,
// for the same reason those do: five commands need it, sixty must not be
// affected, and a rule spread across sixty call sites is forgotten in one of
// them.

// delSelKind says what an active region means for a command.
type delSelKind int

const (
	// delSelNone: the command is unaffected by a selection.
	delSelNone delSelKind = iota
	// delSelReplace: delete the region, then run the command. Typing, RET and
	// yank all put something in the selection's place.
	delSelReplace
	// delSelSupersede: deleting the region IS the command, which must then not
	// run. Otherwise Backspace on a selection would take the selection and a
	// further character with it.
	delSelSupersede
)

// delSelCommands is the whole rule. Emacs expresses this as a `delete-selection`
// property on each command symbol; a table here keeps it in one readable place.
//
// Deliberately absent:
//
//   - kill-region and kill-ring-save consume the region themselves. Deleting it
//     first would leave the kill with nothing to take, silently losing what the
//     user asked to cut.
//   - indent-for-tab-command. In a modern editor TAB with a block selected
//     indents it; deleting the block instead would be destructive and is the
//     opposite of what the keystroke means there.
//   - Motion. Moving point with a region active is how you adjust a selection.
var delSelCommands = map[string]delSelKind{
	"delete-backward-char": delSelSupersede,
	"delete-char":          delSelSupersede,
	"self-insert-command":  delSelReplace,
	"newline":              delSelReplace,
	"yank":                 delSelReplace,
	"bracketed-paste":      delSelReplace,
}

// SetDeleteSelection turns delete-selection behaviour on or off. It is on by
// default; off restores emacs's own behaviour, where a selection is inert.
func (e *Editor) SetDeleteSelection(on bool) { e.delSel = on }

// DeleteSelection reports whether typing replaces an active region.
func (e *Editor) DeleteSelection() bool { return e.delSel }

// consumeSelection deletes the active region when name is delete-selection
// aware, and reports whether the command itself must now be skipped.
//
// The returned function closes the undo group and is nil when nothing was
// deleted. The group has to stay open across the command that follows, so that
// the deletion and the replacement undo together: a user who types over a
// selection and presses C-/ expects their text back, not an intermediate state
// they never saw.
func (e *Editor) consumeSelection(name string) (skip bool, done func()) {
	if !e.delSel {
		return false, nil
	}
	kind := delSelCommands[name]
	if kind == delSelNone {
		return false, nil
	}

	w := e.Win()
	if w == nil || w.Buf == nil {
		return false, nil
	}
	b := w.Buf
	// Only an ACTIVE region is a selection. Using HasMark here is what made
	// typing after C-y delete the paste, and M-> M-< x delete the whole file:
	// both leave a mark behind without selecting anything.
	if !b.MarkActive() {
		return false, nil
	}

	lo, hi := text.OrderPos(w.Pt, b.Mark())
	if lo.Equal(hi) {
		// An empty region is not a selection. The command runs as it always
		// does, and the mark — which was set deliberately — stays put.
		return false, nil
	}

	// With automatic pairs, a bracket or quote typed over a selection goes
	// around it rather than replacing it: select a word, type ", and it is
	// quoted. Only in a text buffer - in a prompt the selection is replaced as
	// ever.
	if closer, ok := command.PairFor(e.seq.LastRune); ok && name == "self-insert-command" &&
		command.AutoPair && e.mini == nil {
		b.BeginUndoGroup()
		defer b.EndUndoGroup()
		// The opener first, so a buffer that refuses the edit - one read-only
		// in parts - refuses before anything is inserted rather than after
		// the closer is.
		if err := b.Insert(lo, []rune{e.seq.LastRune}); err != nil {
			e.Echo("%v", err)
			return true, nil
		}
		if hi.Line == lo.Line {
			hi.Col++
		}
		if err := b.Insert(hi, []rune{closer}); err != nil {
			_ = b.Delete(lo, text.Pos{Line: lo.Line, Col: lo.Col + 1})
			e.Echo("%v", err)
			return true, nil
		}
		// Point after the closer.
		w.Pt = text.Pos{Line: hi.Line, Col: hi.Col + 1}
		b.DeactivateMark()
		return true, nil
	}

	b.BeginUndoGroup()
	if err := b.Delete(lo, hi); err != nil {
		// The buffer refused: it is read-only, or read-only in parts and the
		// selection reaches into them. The keystroke goes no further - typed
		// beside a selection it could not replace, it would do something the
		// user did not ask for.
		b.EndUndoGroup()
		e.Echo("%v", err)
		return true, nil
	}
	w.Pt = lo
	// The selection is consumed, so it ends. The mark itself is kept, as emacs
	// keeps it: deactivating is what stops the next keystroke seeing a region.
	b.DeactivateMark()

	return kind == delSelSupersede, b.EndUndoGroup
}
