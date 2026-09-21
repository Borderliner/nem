package editor

import "github.com/Borderliner/nem/command"

// registerDisplayCommands adds the commands that change how the editor looks
// rather than what a buffer contains.
//
// They close over the Editor instead of going through Env for the same reason
// recover-file does: display state lives in the theme, which Env cannot reach by
// design, and adding an interface method to serve one command would widen a
// surface sixty other commands share.
func registerDisplayCommands(e *Editor, reg *command.Registry) error {
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
