package editor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/memory"
	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// Recent files, and each file's place: the files opened lately are on C-x C-r,
// and a file opened again shows where it was left - point on the same line,
// the same line at the top of the window - as emacs's recentf and save-place
// do.

// registerRecentCommands adds recentf-open.
func registerRecentCommands(e *Editor, reg *command.Registry) error {
	return reg.Register(command.Command{
		Name:        "recentf-open",
		Doc:         "Open one of the files opened recently.",
		Interactive: true,
		Fn:          func(command.Env) error { return e.recentOpen() },
	})
}

// recentOpen is C-x C-r: a prompt over the recent files that still exist,
// the most recent first - except the file on screen, which goes last, since
// opening it again would do nothing.
func (e *Editor) recentOpen() error {
	home := homeDir()
	cur := e.active.Buf.Path()
	var cands, last []string
	for _, p := range e.mem.Recent {
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue // deleted or moved since; offering it would only fail
		}
		if p == cur {
			last = append(last, tildePath(p, home))
		} else {
			cands = append(cands, tildePath(p, home))
		}
	}
	cands = append(cands, last...)
	if len(cands) == 0 {
		e.Echo("No recent files")
		return nil
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   "Recent file: ",
		Complete: command.CompleteFrom(cands),
		Icon:     icons.ForCandidate,
		History:  "file",
	})
	if err != nil || ans == "" {
		return err
	}
	return e.visitFile(untildePath(ans, home))
}

// tildePath shortens a path under home to start with ~.
func tildePath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// untildePath undoes tildePath.
func untildePath(p, home string) string {
	if home != "" && strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		return home + p[1:]
	}
	return p
}

// rememberFile notes that path was opened, for C-x C-r, and the project it
// is in, for C-x p p: working in a project is what makes it a known one, as
// projectile has it.
func (e *Editor) rememberFile(path string) {
	e.mem.AddRecent(path)
	if root, ok := project.Root(filepath.Dir(path)); ok {
		e.mem.AddProject(root)
	}
	e.saveMemory()
}

// rememberPlaces records where point is in every buffer visiting a file:
// from the active window when it shows the buffer, another window when one
// does, or where the buffer was last left.
func (e *Editor) rememberPlaces() {
	for _, b := range e.buffers {
		e.rememberPlace(b)
	}
}

func (e *Editor) rememberPlace(b *text.Buffer) {
	path := b.Path()
	if path == "" {
		return
	}
	pt, top := b.SavePoint(), b.SaveTop()
	if w := e.windowShowing(b); w != nil {
		pt, top = w.Pt, w.Top
	}
	e.mem.SetPlace(path, memory.Place{Line: pt.Line, Col: int(pt.Col), Top: top})
}

// restorePlace puts a buffer just read from path back where it was left, for
// the window that visits it next.
func (e *Editor) restorePlace(b *text.Buffer, path string) {
	pl, ok := e.mem.PlaceOf(path)
	if !ok {
		return
	}
	p := b.ClampPos(text.Pos{Line: pl.Line, Col: text.RuneIdx(pl.Col)})
	b.SetSavePoint(p)
	b.SetSaveTop(min(max(pl.Top, 0), p.Line))
}

// windowShowing is the window showing b: the active one if it does, otherwise
// the first that does, or nil.
func (e *Editor) windowShowing(b *text.Buffer) *view.Window {
	if e.active != nil && e.active.Buf == b {
		return e.active
	}
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			return w
		}
	}
	return nil
}
