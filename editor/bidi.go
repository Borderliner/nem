package editor

import (
	"github.com/Borderliner/nem/bidi"
	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
)

// Right-to-left text, the editor's part: which buffers are prose, whose lines
// each take their direction from their first letter, and arrow keys that go
// the way a right-to-left line reads. Drawing is ui's; see ui/bidi.go.

// isProse is the renderer's DirectionOf: a buffer of text rather than code -
// a file nem has no grammar for, or Markdown - and not a listing. Emacs does
// the same, keeping every line of a programming mode left to right.
func (e *Editor) isProse(b *text.Buffer) bool {
	if b == nil || e.isListing(b) {
		return false
	}
	switch e.FileType(b) {
	case "", "md":
		return true
	}
	return false
}

// rightToLeft reports whether the active window's line reads right to left.
func (e *Editor) rightToLeft() bool {
	w := e.active
	if !e.th.Bidi || !e.isProse(w.Buf) {
		return false
	}
	return bidi.ParagraphDirection(w.Buf.Line(w.Buf.ClampPos(w.Pt).Line).View()) == bidi.RTL
}

// registerBidiCommands adds right-char and left-char, which the arrow keys run.
func registerBidiCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{Name: "right-char", Doc: "Move one character to the right: forward, or backward in a line that reads right to left.",
			Fn: func(env command.Env) error {
				if e.rightToLeft() {
					return env.Run("backward-char")
				}
				return env.Run("forward-char")
			}},
		{Name: "left-char", Doc: "Move one character to the left: backward, or forward in a line that reads right to left.",
			Fn: func(env command.Env) error {
				if e.rightToLeft() {
					return env.Run("forward-char")
				}
				return env.Run("backward-char")
			}},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// takeBidi tells the terminal whether nem lays right-to-left text out itself,
// so that one that would reorder it again does not. See ui.Screen.TakeBidi.
func (e *Editor) takeBidi() {
	if s, ok := e.scr.(interface{ TakeBidi(bool) }); ok {
		s.TakeBidi(e.th.Bidi)
	}
}
