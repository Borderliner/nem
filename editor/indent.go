package editor

import (
	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/editorconfig"
	"github.com/Borderliner/nem/text"
)

// The editor's half of .editorconfig: the rules that apply when a file is
// saved. The indentation rules are the command package's, since TAB is where
// they are used; see command.IndentFor.

// tidyForSave does to b what the .editorconfig for the file it is about to be
// written to asks of a save: trim_trailing_whitespace takes the spaces and
// tabs off the ends of its lines, and insert_final_newline makes the file end
// with a newline, or, when false, not.
//
// It is done to the buffer, as edits - one undo step - rather than only to
// the bytes written, so that what is on screen is what the file holds, and
// undo gives the whitespace back if it was wanted after all. A final newline
// is not text in the buffer, so adding one is only the buffer's note to write
// it; taking one away also deletes any empty lines the file would have ended
// with, as those are newlines at its end too.
//
// A read-only buffer is written as it is: it cannot be edited, and a save is
// no time to report that.
func (e *Editor) tidyForSave(b *text.Buffer, target string) {
	p := editorconfig.Lookup(target)
	if b.ReadOnly() || (p.TrimTrailingWhitespace != editorconfig.True && p.InsertFinalNewline == editorconfig.Unset) {
		return
	}
	b.BeginUndoGroup()
	defer b.EndUndoGroup()

	if p.TrimTrailingWhitespace == editorconfig.True {
		// A part of the buffer it may not change is left, and saved, as it is.
		_, _ = command.TrimTrailingWhitespace(b, 0, b.NumLines()-1)
	}

	last := b.NumLines() - 1
	switch p.InsertFinalNewline {
	case editorconfig.True:
		// An empty last line already ends the file with the newline before
		// it; and an empty file is left empty, as emacs leaves it.
		if b.Line(last).Len() > 0 {
			b.SetFinalNewline(true)
		}
	case editorconfig.False:
		b.SetFinalNewline(false)
		keep := last
		for keep > 0 && b.Line(keep).Len() == 0 {
			keep--
		}
		if keep < last {
			_ = b.Delete(text.Pos{Line: keep, Col: b.Line(keep).Len()}, text.Pos{Line: last})
		}
	}

	// Point stays on its text. On whitespace that is gone it goes to the end
	// of the line, and on an empty line that is gone, to the end of the text
	// it came after.
	for _, w := range e.tree.Windows() {
		switch {
		case w.Buf != b:
		case w.Pt.Line >= b.NumLines():
			w.Pt = b.End()
		default:
			w.Pt = b.ClampPos(w.Pt)
		}
	}
}
