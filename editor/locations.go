package editor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// Lists of locations: a buffer whose lines lead to places in files - a
// search's matching lines, a compilation's errors. They share how a location
// is gone to, and M-g n and M-g p, which step through the last such list
// from any buffer, as emacs's next-error does.

// locations is a buffer's list of places.
type locations interface {
	// count is how many there are, and target where the i'th is: a file,
	// and a line and rune column from 0.
	count() int
	target(i int) (path string, line, col int)
	// lineOf is the buffer line showing location i, and pointOn where point
	// rests on a line.
	lineOf(i int) int
	pointOn(line int) text.Pos
	// current is the location last gone to, -1 before the first.
	current() int
	setCurrent(i int)
	// noun names a location, for messages: "match", "error".
	noun() string
}

// chooser is a list some of whose locations name a file that several of the
// project's could be. Which one is asked on the way there, and the answer
// holds for the rest of the list.
type chooser interface {
	choicesFor(i int) []string
	choose(i int, choice string)
}

var errNoLocations = errors.New("nothing to step through; C-x p g searches the project, M-x compile builds it")

// locationsOf is b's list of places, or nil if b has none.
func (e *Editor) locationsOf(b *text.Buffer) locations {
	if st := e.grepOf(b); st != nil {
		return st
	}
	if st := e.compileOf(b); st != nil {
		return st
	}
	return nil
}

// registerLocationCommands adds M-g n and M-g p.
func registerLocationCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{Name: "next-error", Doc: "Go to the next match of the last search, or error of the last compilation, ARG on.",
			Fn: func(command.Env) error {
				n, _ := e.Arg()
				return e.nextLocation(n)
			}},
		{Name: "previous-error", Doc: "Go to the previous match of the last search, or error of the last compilation, ARG back.",
			Fn: func(command.Env) error {
				n, _ := e.Arg()
				return e.nextLocation(-n)
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

// nextLocation is M-g n and M-g p: the location n on from the last one gone
// to, in the window being worked in - or, from the list itself, in the other.
func (e *Editor) nextLocation(n int) error {
	b := e.nextErrorBuf
	ll := e.locationsOf(b)
	if ll == nil {
		return errNoLocations
	}
	i := ll.current() + n
	if ll.current() < 0 && n < 0 {
		i = -1
	}
	if i < 0 || i >= ll.count() {
		return fmt.Errorf("no more %s", plurals(ll.noun()))
	}
	if err := e.visitLocation(b, ll, i, e.active.Buf == b); err != nil {
		return err
	}
	e.Echo("%s %d of %d", capitalise(ll.noun()), i+1, ll.count())
	return nil
}

// plurals is an English noun's plural, for the nouns locations use.
func plurals(noun string) string {
	if strings.HasSuffix(noun, "ch") || strings.HasSuffix(noun, "s") {
		return noun + "es"
	}
	return noun + "s"
}

// capitalise upper-cases the first letter of an ASCII word.
func capitalise(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// visitLocation opens location i of the list in b and puts point on it,
// centred in view.
//
// From the list it is shown in the other window, splitting the frame if there
// is only one, and that window is selected if sel; from anywhere else it is
// shown where you are. The list follows, point moving to the location in
// every window showing it.
func (e *Editor) visitLocation(b *text.Buffer, ll locations, i int, sel bool) error {
	path, line, col := ll.target(i)
	if c, ok := ll.(chooser); ok && path == "" {
		opts := c.choicesFor(i)
		if len(opts) == 0 {
			return errors.New("the file this names cannot be found")
		}
		ans, err := e.ReadString(command.ReadOpts{
			Prompt:       fmt.Sprintf("Which %s? ", filepath.Base(opts[0])),
			Complete:     command.CompleteFrom(opts),
			Icon:         icons.ForCandidate,
			RequireMatch: true,
		})
		if err != nil || ans == "" {
			return err
		}
		c.choose(i, ans)
		path, line, col = ll.target(i)
	}
	fb, err := e.OpenFile(path)
	if err != nil {
		return err
	}
	ll.setCurrent(i)
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			w.Pt = ll.pointOn(ll.lineOf(i))
		}
	}

	from := e.active
	if e.active.Buf == b {
		if len(e.tree.Windows()) < 2 {
			if err := e.SplitWindow(true); err != nil {
				return err
			}
		} else {
			e.OtherWindow(1)
		}
	} else {
		sel = true
	}
	w := e.active
	w.Visit(fb)
	w.Pt = fb.ClampPos(text.Pos{Line: line, Col: text.RuneIdx(col)})
	w.GoalCol = view.GoalColUnset
	w.Top = max(0, w.Pt.Line-e.TextHeight()/2)
	if !sel {
		e.active = from
	}
	return nil
}
