package editor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/dired"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// Dired is nem's directory editor: a buffer listing a directory, with keys of
// its own for walking the tree and acting on files.
//
// Three pieces meet here. The dired package reads and formats a listing and
// performs the file operations, and knows nothing of buffers. The buffer holds
// the listing, read-only, so a stray keystroke cannot leave it describing files
// that are not there. This file keeps the two in step and supplies the
// commands.
//
// The listing is never parsed back out of the buffer. Which file a line names
// is answered by the Listing the text was generated from, so a file called
// "a -> b" or one with a newline in its name cannot be misread.

// diredState is what makes a buffer a directory listing.
type diredState struct {
	dir  string
	opts dired.Options
	// entries is the directory as last read. Toggling hidden files or the
	// details reformats these without reading the disk again; g and every file
	// operation read it afresh.
	entries []dired.Entry
	// list is what the buffer currently shows, line for line.
	list *dired.Listing
	// marks maps entry names to their mark, '*' or 'D'.
	marks map[string]rune
	// now is the clock the listing was formatted against, so a line redrawn
	// later is dated the same way as its neighbours.
	now time.Time
}

// errNotDired is returned by a dired command run anywhere else, say from M-x.
var errNotDired = errors.New("not a dired buffer")

// diredBindings is the dired keymap, consulted before the global one while a
// dired buffer is current. Printable keys are free to take here because the
// buffer is read-only: there is no text to type into.
var diredBindings = []struct{ Spec, Command string }{
	{"RET", "dired-find-file"},
	{"f", "dired-find-file"},
	{"e", "dired-find-file"},
	{"o", "dired-find-file-other-window"},
	{"^", "dired-up-directory"},
	{"n", "dired-next-line"},
	{"SPC", "dired-next-line"},
	{"C-n", "dired-next-line"},
	{"<down>", "dired-next-line"},
	{"p", "dired-previous-line"},
	{"C-p", "dired-previous-line"},
	{"<up>", "dired-previous-line"},
	{"M-<", "dired-first-file"},
	{"M->", "dired-last-file"},
	{"m", "dired-mark"},
	{"u", "dired-unmark"},
	{"DEL", "dired-unmark-backward"},
	{"U", "dired-unmark-all"},
	{"t", "dired-toggle-marks"},
	{"d", "dired-flag-file-deletion"},
	{"x", "dired-do-flagged-delete"},
	{"D", "dired-do-delete"},
	{"R", "dired-do-rename"},
	{"C", "dired-do-copy"},
	{"+", "dired-create-directory"},
	{"g", "dired-revert"},
	{"(", "dired-toggle-details"},
	{".", "dired-toggle-hidden"},
	{"s", "dired-sort-toggle"},
	{"w", "dired-copy-filename"},
	{"E", "dired-do-open"},
	{"q", "quit-window"},
}

// newDiredKeymap builds the dired keymap from diredBindings.
func newDiredKeymap() (*keymap.Map, error) {
	m := keymap.New()
	for _, b := range diredBindings {
		if err := bindSpec(m, b.Spec, b.Command); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// DiredKeymap exposes the dired keymap so the Lua layer can rebind keys in it.
func (e *Editor) DiredKeymap() *keymap.Map { return e.diredKeys }

// modeKeys is the keymap of b's mode, or nil for a buffer of plain text.
func (e *Editor) modeKeys(b *text.Buffer) *keymap.Map {
	if e.diredOf(b) != nil {
		return e.diredKeys
	}
	return nil
}

// diredOf returns b's listing state, or nil when b is not a dired buffer.
func (e *Editor) diredOf(b *text.Buffer) *diredState {
	if b == nil {
		return nil
	}
	return e.dired[b]
}

// isListing is the renderer's ListingFunc.
func (e *Editor) isListing(b *text.Buffer) bool { return e.diredOf(b) != nil }

// diredSpans colours a line of a listing from the Listing that produced it.
func (st *diredState) spans(line int) []syntax.Span {
	if st.list == nil || line < 0 || line >= len(st.list.Spans) {
		return nil
	}
	return st.list.Spans[line]
}

// Dired returns the buffer listing dir, reading it afresh. A directory already
// listed reuses its buffer, so asking for it twice does not pile up copies.
func (e *Editor) Dired(dir string) (*text.Buffer, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for _, b := range e.buffers {
		if st := e.dired[b]; st != nil && st.dir == abs {
			if err := e.diredReread(b, st); err != nil {
				return nil, err
			}
			return b, nil
		}
	}

	entries, err := dired.Read(abs)
	if err != nil {
		return nil, err
	}
	b := text.NewBuffer()
	b.SetReadOnly(true)
	st := &diredState{dir: abs, entries: entries, marks: map[string]rune{}}
	st.opts.Icons = e.icons
	e.adopt(b, e.uniqueName(diredName(abs)))
	e.dired[b] = st
	e.diredRender(b, st, "")
	return b, nil
}

// diredName is the buffer name for a listing of dir: its last component with a
// trailing separator, so a directory reads differently from a file of the same
// name. The root has no last component and is named in full.
func diredName(dir string) string {
	base := filepath.Base(dir)
	if base == dir || base == "." || base == string(filepath.Separator) {
		return dir
	}
	return base + string(filepath.Separator)
}

// diredReread reads st's directory again and redraws. Marks on files that have
// gone are dropped with them.
func (e *Editor) diredReread(b *text.Buffer, st *diredState) error {
	entries, err := dired.Read(st.dir)
	if err != nil {
		return err
	}
	st.entries = entries
	present := make(map[string]bool, len(entries))
	for _, en := range entries {
		present[en.Name] = true
	}
	for name := range st.marks {
		if !present[name] {
			delete(st.marks, name)
		}
	}
	e.diredRender(b, st, "")
	return nil
}

// diredRender regenerates b from st, keeping every window's point on the entry
// it was on.
//
// focus, when not empty, names the entry every window showing b should land
// on instead: the file just created, or the directory just come up out of.
// An entry that has gone - deleted, or now hidden - leaves point on the same
// row, so a deletion lands on the next file rather than jumping to the top.
func (e *Editor) diredRender(b *text.Buffer, st *diredState, focus string) {
	type spot struct {
		name string
		line int
	}
	before := func(pt text.Pos) spot {
		if st.list != nil {
			if en, ok := st.list.EntryAt(pt.Line); ok {
				return spot{en.Name, pt.Line}
			}
		}
		return spot{"", pt.Line}
	}
	var wins []*view.Window
	var spots []spot
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			wins = append(wins, w)
			spots = append(spots, before(w.Pt))
		}
	}
	saved := before(b.SavePoint())
	fresh := st.list == nil

	st.now = time.Now()
	st.list = dired.Format(st.dir, st.entries, st.marks, st.opts, st.now, homeDir())
	b.Regenerate([]rune(strings.Join(st.list.Lines, "\n")))

	after := func(s spot) text.Pos {
		switch {
		case focus != "":
			if line, ok := st.list.LineOf(focus); ok {
				return st.entryPos(line)
			}
		case s.name != "":
			if line, ok := st.list.LineOf(s.name); ok {
				return st.entryPos(line)
			}
		}
		if fresh {
			return st.entryPos(st.firstFileLine())
		}
		return st.entryPos(s.line)
	}
	for i, w := range wins {
		w.Pt = after(spots[i])
		w.GoalCol = view.GoalColUnset
		if fresh {
			// A directory just entered starts at the top, header in view,
			// whatever the last one was scrolled to.
			w.Top = 0
		}
	}
	b.SetSavePoint(after(saved))
	if fresh {
		b.SetSaveTop(0)
	}
}

// entryPos is where point sits on line: at the start of the entry's name,
// clamped to the entries so it never rests on the header.
func (st *diredState) entryPos(line int) text.Pos {
	n := len(st.list.Entries)
	if n == 0 {
		return text.Pos{Line: min(dired.FirstEntry, len(st.list.Lines)-1)}
	}
	line = max(dired.FirstEntry, min(line, dired.FirstEntry+n-1))
	return text.Pos{Line: line, Col: text.RuneIdx(dired.NameColumn(st.opts))}
}

// firstFileLine is the line of the first entry other than the parent, where a
// new listing puts point: the way out is one p away, and m or D pressed
// straight away should act on a file.
func (st *diredState) firstFileLine() int {
	if len(st.list.Entries) > 1 && st.list.Entries[0].IsParent() {
		return dired.FirstEntry + 1
	}
	return dired.FirstEntry
}

// homeDir abbreviates listing headers to ~. Without one nothing is abbreviated.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// here returns the dired buffer the active window shows.
func (e *Editor) here() (*text.Buffer, *diredState, error) {
	b := e.active.Buf
	st := e.diredOf(b)
	if st == nil {
		return nil, nil, errNotDired
	}
	return b, st, nil
}

// entryAtPoint returns the entry on the active window's line.
func (e *Editor) entryAtPoint(st *diredState) (dired.Entry, bool) {
	return st.list.EntryAt(e.active.Pt.Line)
}

// pathOf is the full path of an entry in st's directory.
func (st *diredState) pathOf(en dired.Entry) string { return filepath.Join(st.dir, en.Name) }

// targets is what an operation acts on: the entries marked with mark if there
// are any, otherwise the entry at point. It is the emacs rule, and what makes
// one key serve both a single file and a selection.
//
// The parent entry is never a target. It is how to leave the directory, and
// D on it would otherwise offer to delete the directory being looked at.
func (e *Editor) targets(st *diredState, mark rune) []dired.Entry {
	var out []dired.Entry
	for _, en := range st.list.Entries {
		if st.marks[en.Name] == mark && !en.IsParent() {
			out = append(out, en)
		}
	}
	if len(out) == 0 {
		if en, ok := e.entryAtPoint(st); ok && !en.IsParent() {
			out = append(out, en)
		}
	}
	return out
}

// noTarget says why an operation found nothing to act on.
func (e *Editor) noTarget(st *diredState) {
	if en, ok := e.entryAtPoint(st); ok && en.IsParent() {
		e.Echo("Cannot operate on %s", dired.ParentName)
		return
	}
	e.Echo("No file on this line")
}

// diredUp lists the parent of st's directory, with point on the directory just
// left, or says there is nowhere to go.
func (e *Editor) diredUp(b *text.Buffer, st *diredState) error {
	parent := filepath.Dir(st.dir)
	if parent == st.dir {
		e.Echo("Already at the top")
		return nil
	}
	return e.diredGo(b, st, parent, filepath.Base(st.dir))
}

// registerDiredCommands adds dired and its commands. Like the display commands
// they close over the Editor: a listing is editor state that Env has no reason
// to expose to sixty other commands.
func registerDiredCommands(e *Editor, reg *command.Registry) error {
	// inDired wraps a command that only makes sense in a listing.
	inDired := func(fn func(b *text.Buffer, st *diredState) error) func(command.Env) error {
		return func(command.Env) error {
			b, st, err := e.here()
			if err != nil {
				return err
			}
			return fn(b, st)
		}
	}
	// onEntry wraps a command that acts on the entry at point.
	onEntry := func(fn func(b *text.Buffer, st *diredState, en dired.Entry) error) func(command.Env) error {
		return inDired(func(b *text.Buffer, st *diredState) error {
			en, ok := e.entryAtPoint(st)
			if !ok {
				e.Echo("No file on this line")
				return nil
			}
			return fn(b, st, en)
		})
	}

	cmds := []command.Command{
		{Name: "dired", Doc: "List a directory, to browse it and act on its files.",
			Fn: func(command.Env) error { return e.diredPrompt() }},
		{Name: "dired-jump", Doc: "List the directory of the current file, with point on the file.",
			Fn: func(command.Env) error { return e.diredJump() }},
		{Name: "dired-find-file", Doc: "Visit the file at point, or list the directory at point.",
			Fn: onEntry(func(b *text.Buffer, st *diredState, en dired.Entry) error {
				if en.IsParent() {
					return e.diredUp(b, st)
				}
				if en.IsDir {
					return e.diredGo(b, st, st.pathOf(en), "")
				}
				return e.visitFile(st.pathOf(en))
			})},
		{Name: "dired-find-file-other-window", Doc: "Visit the file at point in another window.",
			Fn: onEntry(func(b *text.Buffer, st *diredState, en dired.Entry) error {
				return e.diredOtherWindow(st.pathOf(en), en.IsDir)
			})},
		{Name: "dired-up-directory", Doc: "List the parent directory, with point on this one.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredUp(b, st) })},
		{Name: "dired-next-line", Doc: "Move to the next file, ARG files down.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				n, _ := e.Arg()
				e.diredMove(st, n)
				return nil
			})},
		{Name: "dired-previous-line", Doc: "Move to the previous file, ARG files up.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				n, _ := e.Arg()
				e.diredMove(st, -n)
				return nil
			})},
		{Name: "dired-first-file", Doc: "Move to the first file in the listing, below the parent entry.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				e.active.Pt = st.entryPos(st.firstFileLine())
				return nil
			})},
		{Name: "dired-last-file", Doc: "Move to the last file in the listing.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				e.active.Pt = st.entryPos(dired.FirstEntry + len(st.list.Entries) - 1)
				return nil
			})},
		{Name: "dired-mark", Doc: "Mark the file at point for the next operation, and move down.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredSetMark(b, st, '*', 1) })},
		{Name: "dired-unmark", Doc: "Remove the mark on the file at point, and move down.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredSetMark(b, st, ' ', 1) })},
		{Name: "dired-unmark-backward", Doc: "Move up a file and remove its mark.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				e.diredMove(st, -1)
				return e.diredSetMark(b, st, ' ', 0)
			})},
		{Name: "dired-unmark-all", Doc: "Remove every mark and deletion flag.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				n := len(st.marks)
				clear(st.marks)
				e.diredRender(b, st, "")
				e.Echo("Removed %d mark%s", n, plural(n))
				return nil
			})},
		{Name: "dired-toggle-marks", Doc: "Mark every unmarked file and unmark every marked one.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				for _, en := range st.list.Entries {
					if en.IsParent() {
						continue
					}
					switch st.marks[en.Name] {
					case '*':
						delete(st.marks, en.Name)
					case 0:
						st.marks[en.Name] = '*'
					}
				}
				e.diredRender(b, st, "")
				return nil
			})},
		{Name: "dired-flag-file-deletion", Doc: "Flag the file at point for deletion by x, and move down.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredSetMark(b, st, 'D', 1) })},
		{Name: "dired-do-flagged-delete", Doc: "Delete the files flagged with d, after asking.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				var flagged []dired.Entry
				for _, en := range st.list.Entries {
					if st.marks[en.Name] == 'D' {
						flagged = append(flagged, en)
					}
				}
				if len(flagged) == 0 {
					e.Echo("No deletions requested (flag files with d)")
					return nil
				}
				return e.diredDelete(b, st, flagged)
			})},
		{Name: "dired-do-delete", Doc: "Delete the marked files, or the file at point, after asking.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				return e.diredDelete(b, st, e.targets(st, '*'))
			})},
		{Name: "dired-do-rename", Doc: "Rename the file at point, or move the marked files into a directory.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredTransfer(b, st, false) })},
		{Name: "dired-do-copy", Doc: "Copy the file at point, or the marked files into a directory.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredTransfer(b, st, true) })},
		{Name: "dired-create-directory", Doc: "Create a directory, and any missing parents.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error { return e.diredMkdir(b, st) })},
		{Name: "dired-revert", Doc: "Read the directory again.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				if err := e.diredReread(b, st); err != nil {
					return err
				}
				e.Echo("Listed %s", e.diredShort(st.dir))
				return nil
			})},
		{Name: "dired-toggle-details", Doc: "Show or hide permissions, sizes and dates.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				st.opts.HideDetails = !st.opts.HideDetails
				e.diredRender(b, st, "")
				return nil
			})},
		{Name: "dired-toggle-hidden", Doc: "Show or hide dotfiles.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				st.opts.ShowHidden = !st.opts.ShowHidden
				e.diredRender(b, st, "")
				if st.opts.ShowHidden {
					e.Echo("Showing hidden files")
				} else {
					e.Echo("Hiding hidden files")
				}
				return nil
			})},
		{Name: "dired-sort-toggle", Doc: "Sort by name, then by time, then by size.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				st.opts.Sort = st.opts.Sort.Next()
				e.diredRender(b, st, "")
				e.Echo("Sorted by %s", st.opts.Sort)
				return nil
			})},
		{Name: "dired-copy-filename", Doc: "Copy the names of the marked files, or of the file at point.",
			Fn: inDired(func(b *text.Buffer, st *diredState) error {
				ts := e.targets(st, '*')
				if len(ts) == 0 {
					e.noTarget(st)
					return nil
				}
				names := make([]string, len(ts))
				for i, en := range ts {
					names[i] = en.Name
				}
				s := strings.Join(names, " ")
				// A fresh entry, never appended to a kill that came before.
				e.ring.BreakRun()
				e.KillForward(s)
				e.Echo("Copied %s", s)
				return nil
			})},
		{Name: "quit-window", Doc: "Show another buffer here, putting this one at the back of the list.",
			Fn: func(command.Env) error { return e.quitWindow() }},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// diredPrompt is C-x d: ask for a directory and list it.
//
// The prompt starts at the current buffer's directory, and that directory is
// the first candidate, so C-x d RET lists where you already are.
func (e *Editor) diredPrompt() error {
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   "Dired: ",
		History:  "directory",
		Initial:  promptDir(e.bufferDir(e.active.Buf)),
		Complete: command.DirectoryCompleter(),
		Descend:  command.IsDirCandidate,
		Icon:     icons.ForCandidate,
	})
	if err != nil {
		return err
	}
	if ans == "" {
		ans = "."
	}
	b, err := e.Dired(ans)
	if err != nil {
		return err
	}
	e.active.Visit(b)
	return nil
}

// diredJump is C-x C-j: list the directory of the file being edited, with
// point on that file. From a listing it goes up a level.
func (e *Editor) diredJump() error {
	cur := e.active.Buf
	if st := e.diredOf(cur); st != nil {
		return e.diredUp(cur, st)
	}
	dir, focus := e.bufferDir(cur), ""
	if p := cur.Path(); p != "" {
		focus = filepath.Base(p)
	}
	if dir == "" {
		dir = "."
	}
	b, err := e.Dired(dir)
	if err != nil {
		return err
	}
	e.active.Visit(b)
	if focus != "" {
		st := e.dired[b]
		if line, ok := st.list.LineOf(focus); ok {
			e.active.Pt = st.entryPos(line)
		}
	}
	return nil
}

// bufferDir is the directory b belongs to: the one it lists, or its file's.
// Empty means neither, which callers read as the working directory.
func (e *Editor) bufferDir(b *text.Buffer) string {
	if st := e.diredOf(b); st != nil {
		return st.dir
	}
	if b != nil && b.Path() != "" {
		return filepath.Dir(b.Path())
	}
	return ""
}

// promptDir is how a prompt shows dir: nothing for the working directory,
// since a relative name already starts there; a relative path for a directory
// beneath it; the full path otherwise. Always with a trailing separator, so
// the prompt lists the directory's contents rather than its siblings.
func promptDir(dir string) string {
	if dir == "" {
		return ""
	}
	sep := string(filepath.Separator)
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, dir); err == nil {
			switch {
			case rel == ".":
				return ""
			case rel != ".." && !strings.HasPrefix(rel, ".."+sep):
				return rel + sep
			}
		}
	}
	if strings.HasSuffix(dir, sep) {
		return dir
	}
	return dir + sep
}

// diredGo points the listing in b at dir, putting point on focus.
//
// The same buffer follows you around the tree rather than one buffer being
// left behind per directory visited. Emacs does the latter by default, and
// the pile of stale listings it leaves is one of the first things people turn
// off. Marks belong to a directory, so they do not come along.
func (e *Editor) diredGo(b *text.Buffer, st *diredState, dir, focus string) error {
	entries, err := dired.Read(dir)
	if err != nil {
		return err
	}
	st.dir, st.entries, st.list = dir, entries, nil
	clear(st.marks)
	if name := e.uniqueNameFor(b, diredName(dir)); name != "" {
		delete(e.byName, e.names[b])
		e.names[b] = name
		e.byName[name] = b
	}
	e.forgetBranch(b)
	e.diredRender(b, st, focus)
	return nil
}

// visitFile opens path in the active window.
func (e *Editor) visitFile(path string) error {
	b, err := e.OpenFile(path)
	if err != nil {
		return err
	}
	e.active.Visit(b)
	return nil
}

// diredOtherWindow opens path in the next window, splitting the frame when
// there is only one, and selects that window.
func (e *Editor) diredOtherWindow(path string, isDir bool) error {
	var b *text.Buffer
	var err error
	if isDir {
		b, err = e.Dired(path)
	} else {
		b, err = e.OpenFile(path)
	}
	if err != nil {
		return err
	}
	if len(e.tree.Windows()) < 2 {
		if err := e.SplitWindow(true); err != nil {
			e.Echo("%v", err)
			return nil
		}
	} else {
		e.OtherWindow(1)
	}
	e.active.Visit(b)
	return nil
}

// diredMove moves point n entries, stopping at the first and last.
func (e *Editor) diredMove(st *diredState, n int) {
	line := e.active.Pt.Line
	if line < dired.FirstEntry {
		line = dired.FirstEntry - 1
	}
	e.active.Pt = st.entryPos(line + n)
}

// diredSetMark sets the mark on the entry at point and moves down by advance.
// A blank mark removes it.
func (e *Editor) diredSetMark(b *text.Buffer, st *diredState, mark rune, advance int) error {
	en, ok := e.entryAtPoint(st)
	if !ok {
		e.Echo("No file on this line")
		return nil
	}
	if en.IsParent() {
		e.noTarget(st)
		return nil
	}
	if mark == ' ' {
		delete(st.marks, en.Name)
	} else {
		st.marks[en.Name] = mark
	}
	e.diredRedrawEntry(b, st, en)
	if advance != 0 {
		e.diredMove(st, advance)
	}
	return nil
}

// diredRedrawEntry redraws the one line a changed mark affects. A mark changes
// nothing else - not the order, not the header - so re-formatting the whole
// directory for it made holding m down across a large one visibly slow.
func (e *Editor) diredRedrawEntry(b *text.Buffer, st *diredState, en dired.Entry) {
	line, ok := st.list.LineOf(en.Name)
	if !ok {
		return
	}
	s, spans := dired.FormatEntry(en, st.marks[en.Name], st.opts, st.now)
	st.list.Lines[line], st.list.Spans[line] = s, spans
	b.RegenerateLine(line, []rune(s))
}

// describeEntries names what an operation is about to touch, for a prompt:
// the file itself when there is one, a count and the first few names when
// there are several.
func describeEntries(ens []dired.Entry) string {
	name := func(en dired.Entry) string {
		if en.IsDir && en.Target == "" {
			return en.Name + string(filepath.Separator)
		}
		return en.Name
	}
	if len(ens) == 1 {
		return name(ens[0])
	}
	const shown = 3
	names := make([]string, 0, shown+1)
	for i, en := range ens {
		if i == shown {
			names = append(names, "…")
			break
		}
		names = append(names, name(en))
	}
	return fmt.Sprintf("%d files (%s)", len(ens), strings.Join(names, ", "))
}

// yesOrNo asks a question that must be answered by typing yes.
//
// Deleting files cannot be undone, so a single keystroke - which could be one
// meant for the listing and typed a moment too late - is not enough. Anything
// but "yes" is a no.
func (e *Editor) yesOrNo(question string) (bool, error) {
	ans, err := e.ReadString(command.ReadOpts{Prompt: question + "(yes or no) "})
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(ans), "yes"), nil
}

// diredDelete deletes ens after asking, then lists the directory again.
func (e *Editor) diredDelete(b *text.Buffer, st *diredState, ens []dired.Entry) error {
	if len(ens) == 0 {
		e.noTarget(st)
		return nil
	}
	inside := ""
	for _, en := range ens {
		if en.IsDir && en.Target == "" {
			inside = " and everything in it"
			if len(ens) > 1 {
				inside = ", with everything in the directories,"
			}
			break
		}
	}
	ok, err := e.yesOrNo(fmt.Sprintf("Delete %s%s permanently? ", describeEntries(ens), inside))
	if err != nil {
		return err
	}
	if !ok {
		e.Echo("Nothing deleted")
		return nil
	}

	var failed []string
	var firstErr error
	for _, en := range ens {
		if err := dired.Remove(st.pathOf(en)); err != nil {
			failed = append(failed, en.Name)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		delete(st.marks, en.Name)
	}
	if err := e.diredReread(b, st); err != nil {
		return err
	}
	done := len(ens) - len(failed)
	if len(failed) > 0 {
		e.Echo("Deleted %d of %d; %v", done, len(ens), firstErr)
		return nil
	}
	e.Echo("Deleted %s", describeCount(done))
	return nil
}

// describeCount is "1 file" or "3 files".
func describeCount(n int) string { return fmt.Sprintf("%d file%s", n, plural(n)) }

// diredTransfer is R and C: rename or copy the file at point to a new name or
// into a directory, or the marked files into a directory.
//
// Nothing is replaced without asking, and a directory is never replaced at
// all - overwriting one would delete a tree to make room for a file.
func (e *Editor) diredTransfer(b *text.Buffer, st *diredState, copying bool) error {
	ens := e.targets(st, '*')
	if len(ens) == 0 {
		e.noTarget(st)
		return nil
	}
	verb, did := "Rename", "Renamed"
	if copying {
		verb, did = "Copy", "Copied"
	} else if len(ens) > 1 {
		verb, did = "Move", "Moved"
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   fmt.Sprintf("%s %s to: ", verb, describeEntries(ens)),
		History:  "file",
		Initial:  promptDir(st.dir),
		Complete: command.DirectoryCompleter(),
		Descend:  command.IsDirCandidate,
		Icon:     icons.ForCandidate,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(ans) == "" {
		return nil
	}
	dest, err := filepath.Abs(ans)
	if err != nil {
		return err
	}
	destIsDir := false
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		destIsDir = true
	}
	if len(ens) > 1 && !destIsDir {
		return fmt.Errorf("%s is not a directory", dest)
	}

	n, focus := 0, ""
	for _, en := range ens {
		src := st.pathOf(en)
		dst := dest
		if destIsDir {
			dst = filepath.Join(dest, en.Name)
		}
		if dst == src {
			continue
		}
		if fi, err := os.Lstat(dst); err == nil {
			if fi.IsDir() {
				e.Echo("%s is a directory; not replaced", e.diredShort(dst))
				continue
			}
			c, err := e.ReadChar(fmt.Sprintf("%s exists; replace it? (y/n) ", e.diredShort(dst)), []rune{'y', 'n'})
			if err != nil {
				return err
			}
			if c != 'y' {
				continue
			}
			if err := dired.Remove(dst); err != nil {
				return err
			}
		}
		if copying {
			err = dired.Copy(src, dst)
		} else {
			err = dired.Move(src, dst)
		}
		if err != nil {
			_ = e.diredReread(b, st)
			return err
		}
		if !copying {
			e.followRename(src, dst)
			delete(st.marks, en.Name)
		}
		n++
		if filepath.Dir(dst) == st.dir {
			focus = filepath.Base(dst)
		}
	}
	if err := e.diredReread(b, st); err != nil {
		return err
	}
	if focus != "" {
		if line, ok := st.list.LineOf(focus); ok {
			e.active.Pt = st.entryPos(line)
		}
	}
	switch {
	case n == 0:
		e.Echo("Nothing %s", strings.ToLower(did))
	case len(ens) == 1:
		e.Echo("%s %s to %s", did, ens[0].Name, e.diredShort(dest))
	default:
		e.Echo("%s %s to %s", did, describeCount(n), e.diredShort(dest))
	}
	return nil
}

// followRename points buffers at their files' new home after a rename, the
// file itself or anything under a renamed directory. Left alone, the next save
// would quietly recreate the file at its old path.
func (e *Editor) followRename(from, to string) {
	sep := string(filepath.Separator)
	for _, b := range e.buffers {
		p := b.Path()
		var np string
		switch {
		case p == "":
			continue
		case p == from:
			np = to
		case strings.HasPrefix(p, from+sep):
			np = to + p[len(from):]
		default:
			continue
		}
		b.SetPath(np)
		if name := e.uniqueNameFor(b, filepath.Base(np)); name != "" {
			delete(e.byName, e.names[b])
			e.names[b] = name
			e.byName[name] = b
		}
		// The file on disk is the same file, so what nem knew about it still
		// holds; only its name changed.
		if stamp, ok := e.safe.stamps[p]; ok {
			delete(e.safe.stamps, p)
			e.safe.stamps[np] = stamp
		}
		e.forgetBranch(b)
		e.retuneHighlight(b)
	}
}

// diredMkdir is +: create a directory, with any missing parents.
func (e *Editor) diredMkdir(b *text.Buffer, st *diredState) error {
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:  "Create directory: ",
		History: "file",
		Initial: promptDir(st.dir),
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(ans) == "" {
		return nil
	}
	dir, err := filepath.Abs(ans)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dir); err == nil {
		return fmt.Errorf("%s already exists: %w", e.diredShort(dir), fs.ErrExist)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := e.diredReread(b, st); err != nil {
		return err
	}
	// Land on what was made, or on the first directory of a nested path.
	if rel, err := filepath.Rel(st.dir, dir); err == nil && !strings.HasPrefix(rel, "..") {
		first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if line, ok := st.list.LineOf(first); ok {
			e.active.Pt = st.entryPos(line)
		}
	}
	e.Echo("Created %s", e.diredShort(dir))
	return nil
}

// diredShort shortens a path for a message: relative to the working directory
// when it is beneath it, abbreviated to ~ when it is under home.
func (e *Editor) diredShort(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	if h := homeDir(); h != "" && strings.HasPrefix(p, h+string(filepath.Separator)) {
		return "~" + p[len(h):]
	}
	return p
}

// quitWindow shows the most recently visited other buffer in this window and
// moves this one to the back of the list, so C-x b RET does not bring it
// straight back.
func (e *Editor) quitWindow() error {
	cur := e.active.Buf
	for _, b := range e.buffers {
		if b != cur {
			e.active.Visit(b)
			for i, c := range e.buffers {
				if c == cur {
					e.buffers = append(append(e.buffers[:i:i], e.buffers[i+1:]...), cur)
					break
				}
			}
			return nil
		}
	}
	e.Echo("No other buffer")
	return nil
}

// detectIcons decides the "auto" icons setting. A variable so the test binary
// can make the answer the same on every machine; see TestMain.
var detectIcons = icons.Detect

// SetIcons turns file icons on or off, in the prompts and in every listing,
// open ones included. They need a Nerd Font, or a terminal that carries its
// symbols; without one each icon is an empty box, which is what this is for.
func (e *Editor) SetIcons(on bool) {
	e.icons = on
	for _, b := range e.buffers {
		if st := e.dired[b]; st != nil && st.opts.Icons != on {
			st.opts.Icons = on
			e.diredRender(b, st, "")
		}
	}
}

// plural is "s" unless n is one.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
