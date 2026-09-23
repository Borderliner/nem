package editor

import (
	"time"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Bracketed paste.
//
// With bracketed paste on, the terminal wraps whatever is pasted in a pair of
// markers, which tcell delivers as an EventPaste start, one EventKey per pasted
// byte, and an EventPaste end. Without the markers a paste is indistinguishable
// from very fast typing, and nem used to treat it that way. That was both halves
// of the bug: every character went through the keymap, so each newline ran
// newline - which copies the previous line's indentation - and the pasted text's
// own indentation then landed on top, compounding line by line; and every
// character was a full dispatch and a full redraw, so a paste of any size
// crawled.
//
// Between the markers keys are now collected as literal text and never looked
// up. At the end marker the block is inserted by one command, bracketed-paste:
// verbatim, as one undo step, followed by one redraw.

// pasteStall is how long a paste may go quiet before it is taken as finished.
//
// A real paste arrives as fast as the terminal can write it. A gap this long
// means the end marker is not coming - a terminal that mangled it, or a start
// marker typed by hand - and waiting for it would swallow every key the user
// presses from then on.
const pasteStall = 2 * time.Second

// pasteCommand inserts the text a paste collected. It goes through dispatch like
// any other command, which is what gives a paste delete-selection, hooks and
// the kill-run and goal-column bookkeeping without any of it repeated here.
const pasteCommand = "bracketed-paste"

// pasteState collects one paste between its markers.
type pasteState struct {
	active bool
	buf    []rune

	// last is when the paste last showed signs of life. See pasteStall.
	last time.Time

	// cr records that the previous pasted key was a carriage return, so the LF
	// of a CRLF pair does not become a second newline.
	cr bool
}

// add appends the character one pasted key stands for.
//
// tcell has already turned the bytes into keys, and that is lossy in a way that
// matters here: CR arrives as Enter and LF as C-j. Both are newlines in a paste,
// and CRLF is one newline, not two. Every other control byte is dropped: it is
// invisible junk in a text file, and as a key it could do anything at all.
func (p *pasteState) add(ev *tcell.EventKey) {
	afterCR := p.cr
	p.cr = false
	switch ev.Key() {
	case tcell.KeyRune:
		p.buf = append(p.buf, ev.Rune())
	case tcell.KeyEnter:
		p.buf = append(p.buf, '\n')
		p.cr = true
	case tcell.KeyCtrlJ:
		if !afterCR {
			p.buf = append(p.buf, '\n')
		}
	case tcell.KeyTab:
		p.buf = append(p.buf, '\t')
	}
}

// registerPasteCommand adds bracketed-paste to the registry. It is not
// interactive: it inserts whatever the last paste collected, and there is
// nothing for M-x to offer.
func registerPasteCommand(e *Editor, reg *command.Registry) error {
	return reg.Register(command.Command{
		Name: pasteCommand,
		Doc:  "Insert text pasted through the terminal, verbatim.",
		Fn:   func(command.Env) error { return e.insertPasted() },
	})
}

// pasteEvent routes ev through the paste machinery and reports whether it was
// consumed there.
func (e *Editor) pasteEvent(ev tcell.Event, now time.Time) bool {
	switch ev := ev.(type) {
	case *tcell.EventPaste:
		if ev.Start() {
			e.beginPaste(now)
		} else {
			e.endPaste()
		}
		return true
	case *tcell.EventKey:
		if !e.paste.active {
			return false
		}
		if e.pasteStalled(now) {
			// The end marker is not coming. Land what did arrive, and let this
			// key be the ordinary keystroke it almost certainly is.
			e.endPaste()
			return false
		}
		e.paste.add(ev)
		e.paste.last = now
		return true
	}
	return false
}

// pasteStalled reports whether an open paste has gone quiet for too long.
func (e *Editor) pasteStalled(now time.Time) bool {
	return e.paste.active && now.Sub(e.paste.last) > pasteStall
}

// beginPaste opens a paste.
//
// Pasted text is not the second key of a half-typed C-x, nor something a C-u
// should repeat, so both are dropped - the same as the first ordinary key after
// them would drop the panels.
func (e *Editor) beginPaste(now time.Time) {
	if !e.paste.active {
		e.paste = pasteState{active: true}
	}
	e.paste.last = now

	e.dismissWhichKey()
	e.dismissStartup()
	if len(e.pending) > 0 {
		e.pending = nil
		e.echo = ""
	}
	e.arg.reset()
}

// endPaste closes a paste and inserts what it collected. An end marker with no
// paste open is ignored.
func (e *Editor) endPaste() {
	if !e.paste.active {
		return
	}
	e.paste.active = false
	if e.mini != nil {
		e.paste.buf = flattenForPrompt(e.paste.buf)
	}
	if len(e.paste.buf) > 0 {
		e.dispatchReporting(pasteCommand)
		// Once for the whole block, so incremental search and completion
		// filtering see the pasted text rather than a character at a time.
		e.afterMiniEdit()
	}
	e.paste = pasteState{}
}

// insertPasted is bracketed-paste: the collected text at point, verbatim.
//
// It inserts the block in one Buffer.Insert rather than character by character,
// so there is no newline command to copy indentation and nothing to coalesce.
// The group is what keeps a one-character paste from merging into the typing
// before it: the undo log joins consecutive single-rune inserts, and a paste is
// not typing.
func (e *Editor) insertPasted() error {
	rs := e.paste.buf
	if len(rs) == 0 {
		return nil
	}
	w := e.Win()
	b := w.Buf
	b.BeginUndoGroup()
	defer b.EndUndoGroup()

	at := w.Pt
	if err := b.Insert(at, rs); err != nil {
		return err
	}
	w.Pt = endOfInsert(at, rs)
	return nil
}

// endOfInsert returns the position just past rs inserted at at.
func endOfInsert(at text.Pos, rs []rune) text.Pos {
	for _, r := range rs {
		if r == '\n' {
			at.Line++
			at.Col = 0
		} else {
			at.Col++
		}
	}
	return at
}

// flattenForPrompt makes pasted text fit a one-line prompt.
//
// A trailing newline comes along whenever a whole line is copied, and in a file
// name it is never wanted, so it goes. A newline inside the text becomes a
// space rather than vanishing, so two pasted words do not fuse into one.
func flattenForPrompt(rs []rune) []rune {
	for len(rs) > 0 && rs[len(rs)-1] == '\n' {
		rs = rs[:len(rs)-1]
	}
	out := make([]rune, len(rs))
	for i, r := range rs {
		if r == '\n' {
			r = ' '
		}
		out[i] = r
	}
	return out
}

// pasteGate keeps a paste out of a prompt that reads single keys.
//
// query-replace's y/n/!/q prompt is not a text field. Taken as answers, pasting
// "yyyy" there would replace four matches, so everything between the markers is
// swallowed whole. The stall rule applies here too, or a lost end marker would
// leave the prompt deaf to every key.
type pasteGate struct {
	on   bool
	last time.Time
}

// swallow reports whether ev belongs to a paste and must be ignored.
func (g *pasteGate) swallow(ev tcell.Event, now time.Time) bool {
	switch ev := ev.(type) {
	case *tcell.EventPaste:
		g.on, g.last = ev.Start(), now
		return true
	case *tcell.EventKey:
		if !g.on {
			return false
		}
		if now.Sub(g.last) > pasteStall {
			g.on = false
			return false
		}
		g.last = now
		return true
	}
	return false
}
