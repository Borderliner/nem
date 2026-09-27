package editor

import (
	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/view"
)

// registerDisplayCommands adds the commands that change how the editor looks
// rather than what a buffer contains.
//
// They close over the Editor instead of going through Env for the same reason
// recover-file does: display state lives in the theme, which Env cannot reach by
// design, and adding an interface method to serve one command would widen a
// surface sixty other commands share.
func registerDisplayCommands(e *Editor, reg *command.Registry) error {
	if err := reg.Register(command.Command{
		Name:        "toggle-line-wrap",
		Doc:         "Wrap lines wider than the window, or cut them off at its edge.",
		Interactive: true,
		Fn: func(command.Env) error {
			e.SetLineWrap(!e.th.Wrap)
			if e.th.Wrap {
				e.Echo("Line wrap on")
			} else {
				e.Echo("Line wrap off")
			}
			return nil
		},
	}); err != nil {
		return err
	}
	return reg.Register(command.Command{
		Name:        "toggle-line-numbers",
		Doc:         "Show or hide the line-number gutter.",
		Interactive: true,
		Fn: func(command.Env) error {
			e.th.LineNumbers = !e.th.LineNumbers
			if e.th.LineNumbers {
				e.Echo("Line numbers on")
			} else {
				e.Echo("Line numbers off")
			}
			return nil
		},
	})
}

// SetLineWrap turns line wrapping on or off. The goal column each window
// keeps means a column of the line unwrapped and of the row wrapped, so it
// is let go of; and a window scrolled sideways or part way down a line starts
// its view afresh.
func (e *Editor) SetLineWrap(on bool) {
	e.th.Wrap = on
	for _, w := range e.tree.Windows() {
		w.GoalCol, w.LeftCol, w.TopRow = view.GoalColUnset, 0, 0
	}
}
