package editor

import (
	"fmt"
	"strconv"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/imenu"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// registerImenuCommands adds M-g i.
func registerImenuCommands(e *Editor, reg *command.Registry) error {
	return reg.Register(command.Command{
		Name:        "imenu",
		Doc:         "Go to a definition in this buffer - a function, a type, a heading - by name.",
		Interactive: true,
		Fn:          func(command.Env) error { return e.imenu() },
	})
}

// imenu is M-g i: the buffer's definitions, in order, each with its line
// beside it, and point taken to the one chosen. The mark is left where point
// was, so C-u C-SPC comes back.
func (e *Editor) imenu() error {
	b, w := e.active.Buf, e.active
	lines := make([]string, b.NumLines())
	for i := range lines {
		lines[i] = b.Line(i).String()
	}
	entries := imenu.For(b.Path(), lines)
	if len(entries) == 0 {
		e.Echo("No definitions found in this buffer")
		return nil
	}

	// Two definitions of one name - overloads, a method on two types - are
	// told apart by where they are.
	lineOf := make(map[string]int, len(entries))
	cands := make([]string, 0, len(entries))
	for _, en := range entries {
		name := en.Name
		if _, dup := lineOf[name]; dup {
			name = fmt.Sprintf("%s <%d>", name, en.Line+1)
		}
		lineOf[name] = en.Line
		cands = append(cands, name)
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:       "Go to: ",
		Complete:     command.CompleteFrom(cands),
		RequireMatch: true,
		Annotate:     func(c string) string { return strconv.Itoa(lineOf[c] + 1) },
	})
	if err != nil || ans == "" {
		return err
	}
	line, ok := lineOf[ans]
	if !ok {
		return nil
	}
	b.SetMark(w.Pt)
	b.DeactivateMark()
	rs := b.Line(line).View()
	col := 0
	for col < len(rs) && (rs[col] == ' ' || rs[col] == '\t') {
		col++
	}
	w.Pt = text.Pos{Line: line, Col: text.RuneIdx(col)}
	w.GoalCol = view.GoalColUnset
	w.Top = max(0, line-e.TextHeight()/3)
	return nil
}
