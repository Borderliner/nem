package editor

import (
	"errors"
	"fmt"
	"time"

	"github.com/Borderliner/nem/command"
)

// Reverting: reading a buffer's file again when something else changed it.
// C-x x g does it on request. And a buffer with no edits of its own is read
// again by itself once its file changes - after a shell command or a build
// that formats or generates code, a git checkout in another terminal - so
// what is on screen is what is on disk, as emacs's global-auto-revert-mode
// has it. A buffer with edits is never touched: that is what saving's
// overwrite check is for.

// revertEvery is how often files are looked at for changes while nem idles.
const revertEvery = 2 * time.Second

var errNothingToRevert = errors.New("this buffer has no file to read again")

// registerRevertCommands adds revert-buffer.
func registerRevertCommands(e *Editor, reg *command.Registry) error {
	return reg.Register(command.Command{
		Name:        "revert-buffer",
		Doc:         "Read the buffer's file again, throwing away any edits after asking; a listing is read again.",
		Interactive: true,
		Fn:          func(command.Env) error { return e.revertBuffer() },
	})
}

// revertBuffer is C-x x g: the buffer's file read again. What a listing
// shows, or a search, is got again the way each gets it.
func (e *Editor) revertBuffer() error {
	b := e.active.Buf
	switch {
	case e.diredOf(b) != nil:
		return e.Run("dired-revert")
	case e.grepOf(b) != nil:
		return e.Run("grep-revert")
	case b.Path() == "":
		return errNothingToRevert
	}
	name := e.BufferName(b)
	if b.Modified() {
		c, err := e.ReadChar(fmt.Sprintf("Throw away the edits to %s and read it again? (y/n) ", name), []rune{'y', 'n'})
		if err != nil {
			return err
		}
		if c != 'y' {
			return nil
		}
	}
	if err := b.Revert(); err != nil {
		return err
	}
	e.noteOnDisk(b.Path())
	e.clampWindowPoints()
	e.Echo("Reverted %s", name)
	return nil
}

// revertChanged reads again every buffer without edits whose file something
// else has changed, and says which in the echo area.
func (e *Editor) revertChanged() {
	if msg := e.revertQuietly(); msg != "" {
		e.Echo("%s", msg)
	}
}

// revertQuietly is revertChanged leaving the saying to its caller, which may
// have something of its own to say as well: the message, or "" when nothing
// changed.
func (e *Editor) revertQuietly() string {
	if !e.autoRevert {
		return ""
	}
	var names []string
	for _, b := range e.buffers {
		p := b.Path()
		if p == "" || b.Modified() || !e.changedOnDisk(p) || !stampOf(p).exists {
			continue
		}
		if err := b.Revert(); err != nil {
			continue
		}
		e.noteOnDisk(p)
		names = append(names, e.BufferName(b))
	}
	if len(names) == 0 {
		return ""
	}
	e.clampWindowPoints()
	if len(names) == 1 {
		return fmt.Sprintf("Reverted %s, changed on disk", names[0])
	}
	return fmt.Sprintf("Reverted %d buffers changed on disk", len(names))
}

// afterCommand catches up with what a shell command or a build may have done
// to the files: buffers without edits are read again, and the directories on
// screen listed again. It returns what revertQuietly says about it.
func (e *Editor) afterCommand() string {
	msg := e.revertQuietly()
	if !e.autoRevert {
		return msg
	}
	seen := map[*diredState]bool{}
	for _, w := range e.tree.Windows() {
		if st := e.diredOf(w.Buf); st != nil && st.wd == nil && !seen[st] {
			seen[st] = true
			_ = e.diredReread(w.Buf, st)
		}
	}
	return msg
}
