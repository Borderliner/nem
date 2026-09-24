package editor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/dired"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// wdired makes a listing's file names editable text, and renames the files to
// match when the edit is done. C-x C-q in a listing starts it; C-c C-c applies
// the edits and C-c C-k throws them away.
//
// The point is every tool the editor has for text. Renaming a hundred photos
// is a query-replace, or a keyboard macro run down the listing; giving files
// numbered names is a macro with its counter. Emptying a name flags the file
// for deletion, which x in the listing then carries out, and a name with a
// slash in it moves the file into that directory.
//
// Only the names can change. The buffer turns down any other edit - to the
// permissions, a symlink's target, the header, or a line break - so what is
// read back when the edit is done can only be one name per entry, on the line
// that entry had.

// wdiredState is a listing while its names are edited.
type wdiredState struct {
	// shown is each entry's name as the listing showed it, and so what it
	// still is if the line says the same.
	shown []string
	// tail is how many runes follow each entry's name on its line: a
	// symlink's arrow and target, which stay as they are.
	tail []int
	// width is each entry line's length before editing, which is what keeps
	// the colours lined up as names grow and shrink.
	width []int
}

var (
	errWdiredNames = errors.New("only file names can be edited here; C-c C-c applies, C-c C-k cancels")
	errWdiredBusy  = errors.New("file names are being edited; C-c C-c applies, C-c C-k cancels")
	errNotWdired   = errors.New("not editing file names")
)

// wdiredBindings is the keymap of a listing whose names are being edited. It
// is a text buffer then, so every key not named here types or edits as usual.
var wdiredBindings = []struct{ Spec, Command string }{
	{"C-c C-c", "wdired-finish-edit"},
	{"C-x C-s", "wdired-finish-edit"},
	{"C-c C-k", "wdired-abort-changes"},
	{"C-x C-q", "wdired-exit"},
	{"C-a", "wdired-beginning-of-line"},
	{"<home>", "wdired-beginning-of-line"},
	{"C-e", "wdired-end-of-line"},
	{"<end>", "wdired-end-of-line"},
}

// newWdiredKeymap builds the wdired keymap from wdiredBindings.
func newWdiredKeymap() (*keymap.Map, error) {
	m := keymap.New()
	for _, b := range wdiredBindings {
		if err := bindSpec(m, b.Spec, b.Command); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// wdiredHere returns the listing the active window shows, if its names are
// being edited.
func (e *Editor) wdiredHere() (*text.Buffer, *diredState, error) {
	b := e.active.Buf
	st := e.diredOf(b)
	if st == nil || st.wd == nil {
		return nil, nil, errNotWdired
	}
	return b, st, nil
}

// registerWdiredCommands adds the commands for editing a listing's names.
func registerWdiredCommands(e *Editor, reg *command.Registry) error {
	editing := func(fn func(b *text.Buffer, st *diredState) error) func(command.Env) error {
		return func(command.Env) error {
			b, st, err := e.wdiredHere()
			if err != nil {
				return err
			}
			return fn(b, st)
		}
	}
	cmds := []command.Command{
		{Name: "wdired-change-to-wdired-mode", Doc: "Edit the file names in this listing as text, to rename the files.",
			Fn: func(command.Env) error {
				b, st, err := e.here()
				if err != nil {
					return err
				}
				e.wdiredStart(b, st)
				return nil
			}},
		{Name: "wdired-finish-edit", Doc: "Rename the files to the names as edited, and return to the listing.",
			Fn: editing(e.wdiredFinish)},
		{Name: "wdired-abort-changes", Doc: "Throw the edited names away, and return to the listing.",
			Fn: editing(func(b *text.Buffer, st *diredState) error {
				e.wdiredLeave(b, st)
				e.Echo("Changes aborted")
				return nil
			})},
		{Name: "wdired-exit", Doc: "Return to the listing, asking whether to apply any edited names.",
			Fn: editing(func(b *text.Buffer, st *diredState) error {
				if !st.wdiredChanged(b) {
					e.wdiredLeave(b, st)
					e.Echo("(No changes need to be saved)")
					return nil
				}
				c, err := e.ReadChar("Apply the edited names? (y/n) ", []rune{'y', 'n'})
				if err != nil {
					return err
				}
				if c == 'y' {
					return e.wdiredFinish(b, st)
				}
				e.wdiredLeave(b, st)
				e.Echo("Changes aborted")
				return nil
			})},
		{Name: "wdired-beginning-of-line", Doc: "Move to the start of the file name, or of the line off an entry.",
			Fn: func(env command.Env) error { return e.wdiredLineEdge(env, "move-beginning-of-line", true) }},
		{Name: "wdired-end-of-line", Doc: "Move to the end of the file name, or of the line off an entry.",
			Fn: func(env command.Env) error { return e.wdiredLineEdge(env, "move-end-of-line", false) }},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// wdiredStart makes b's names editable.
func (e *Editor) wdiredStart(b *text.Buffer, st *diredState) {
	n := len(st.list.Entries)
	wd := &wdiredState{shown: make([]string, n), tail: make([]int, n), width: make([]int, n)}
	for i, en := range st.list.Entries {
		width := len([]rune(st.list.Lines[dired.FirstEntry+i]))
		wd.shown[i] = dired.ShownName(en)
		wd.tail[i] = width - dired.NameEnd(en, st.opts)
		wd.width[i] = width
	}
	st.wd = wd
	// The text is the listing's as generated, with no history: the guard
	// vets edits, not undo, so there must be nothing from before it to undo.
	b.Regenerate([]rune(strings.Join(st.list.Lines, "\n")))
	b.SetReadOnly(false)
	b.SetEditGuard(st.wdiredGuard(b))
	b.DeactivateMark()
	e.Echo("Editing file names: C-c C-c applies, C-c C-k cancels")
}

// wdiredLeave returns b to a plain listing, drawn afresh from its entries.
func (e *Editor) wdiredLeave(b *text.Buffer, st *diredState) {
	st.wd = nil
	b.SetEditGuard(nil)
	b.SetReadOnly(true)
	// SetIcons passes a listing by while its names are edited.
	st.opts.Icons = e.icons
	e.diredRender(b, st, "")
}

// nameSpan is where the name on line sits, from its first rune to just past
// its last, and false for a line whose text is not open to editing: the
// header, the parent entry, the placeholder of an empty directory.
func (st *diredState) nameSpan(b *text.Buffer, line int) (lo, hi text.RuneIdx, ok bool) {
	i := line - dired.FirstEntry
	if st.wd == nil || i < 0 || i >= len(st.list.Entries) || line >= b.NumLines() ||
		st.list.Entries[i].IsParent() {
		return 0, 0, false
	}
	lo = text.RuneIdx(dired.NameColumn(st.opts))
	hi = b.Line(line).Len() - text.RuneIdx(st.wd.tail[i])
	return lo, hi, hi >= lo
}

// wdiredGuard allows an edit only inside one entry's name, and never a line
// break or other control character, which no name should hold.
func (st *diredState) wdiredGuard(b *text.Buffer) text.EditGuard {
	return func(from, to text.Pos, ins []rune) error {
		lo, hi, ok := st.nameSpan(b, from.Line)
		if !ok || to.Line != from.Line || from.Col < lo || to.Col > hi {
			return errWdiredNames
		}
		for _, r := range ins {
			if unicode.IsControl(r) {
				return errWdiredNames
			}
		}
		return nil
	}
}

// wdiredChanged reports whether any name has been edited.
func (st *diredState) wdiredChanged(b *text.Buffer) bool {
	for i := range st.list.Entries {
		if lo, hi, ok := st.nameSpan(b, dired.FirstEntry+i); ok &&
			string(b.Line(dired.FirstEntry + i).View()[lo:hi]) != st.wd.shown[i] {
			return true
		}
	}
	return false
}

// wdiredFinish renames the files whose names were edited and returns to the
// listing. Should anything be refused, nothing is renamed and the edit goes
// on, so the names can be put right or the edit abandoned.
func (e *Editor) wdiredFinish(b *text.Buffer, st *diredState) error {
	var renames []dired.Rename
	var flagged []string
	// newName maps an entry renamed within this directory to its new name, and
	// one moved out of it to "".
	newName := map[string]string{}
	for i, en := range st.list.Entries {
		lo, hi, ok := st.nameSpan(b, dired.FirstEntry+i)
		if !ok {
			continue
		}
		typed := string(b.Line(dired.FirstEntry + i).View()[lo:hi])
		if typed == st.wd.shown[i] {
			continue
		}
		if strings.TrimSpace(typed) == "" {
			flagged = append(flagged, en.Name)
			continue
		}
		to := typed
		if !filepath.IsAbs(to) {
			to = filepath.Join(st.dir, to)
		}
		to = filepath.Clean(to)
		if to == st.pathOf(en) {
			continue // "name/" for a file, "./name": the same name written otherwise
		}
		renames = append(renames, dired.Rename{From: en.Name, To: strings.TrimSuffix(typed, "/")})
		newName[en.Name] = ""
		if filepath.Dir(to) == st.dir {
			newName[en.Name] = filepath.Base(to)
		}
	}
	if len(renames) == 0 && len(flagged) == 0 {
		e.wdiredLeave(b, st)
		e.Echo("(No changes to be performed)")
		return nil
	}
	if err := dired.RenameAll(st.dir, renames); err != nil {
		return err
	}

	moves := make([]fileMove, len(renames))
	for i, r := range renames {
		to := r.To
		if !filepath.IsAbs(to) {
			to = filepath.Join(st.dir, to)
		}
		moves[i] = fileMove{filepath.Join(st.dir, r.From), filepath.Clean(to)}
	}
	e.followRenames(moves)

	// Marks go with their files, and emptied names are flagged for x.
	marks := make(map[string]rune, len(st.marks)+len(flagged))
	for name, m := range st.marks {
		if nn, renamed := newName[name]; !renamed {
			marks[name] = m
		} else if nn != "" {
			marks[nn] = m
		}
	}
	for _, name := range flagged {
		marks[name] = 'D'
	}
	st.marks = marks

	// Point stays with the file it was on, under its new name.
	focus := ""
	if en, ok := e.entryAtPoint(st); ok {
		focus = en.Name
		if nn, renamed := newName[en.Name]; renamed {
			focus = nn
		}
	}
	st.wd = nil
	b.SetEditGuard(nil)
	b.SetReadOnly(true)
	st.opts.Icons = e.icons
	if err := e.diredReread(b, st); err != nil {
		return err
	}
	if line, ok := st.list.LineOf(focus); ok && focus != "" {
		e.active.Pt = st.entryPos(line)
	}
	e.Echo("%s", wdiredReport(renames, flagged))
	return nil
}

// wdiredReport says what finishing an edit did.
func wdiredReport(renames []dired.Rename, flagged []string) string {
	var parts []string
	switch len(renames) {
	case 0:
	case 1:
		parts = append(parts, fmt.Sprintf("Renamed %s to %s", renames[0].From, renames[0].To))
	default:
		parts = append(parts, fmt.Sprintf("Renamed %s", describeCount(len(renames))))
	}
	if n := len(flagged); n > 0 {
		f := fmt.Sprintf("flagged %s for deletion (x deletes)", describeCount(n))
		if len(parts) == 0 {
			f = strings.ToUpper(f[:1]) + f[1:]
		}
		parts = append(parts, f)
	}
	return strings.Join(parts, "; ")
}

// wdiredLineEdge is C-a or C-e while names are edited: the name's start or
// end on an entry's line, where the editing happens, and the line's anywhere
// else.
func (e *Editor) wdiredLineEdge(env command.Env, fallback string, start bool) error {
	b, st := e.active.Buf, e.diredOf(e.active.Buf)
	if st != nil {
		if lo, hi, ok := st.nameSpan(b, e.active.Pt.Line); ok {
			col := hi
			if start {
				col = lo
			}
			e.active.Pt.Col = col
			return nil
		}
	}
	return env.Run(fallback)
}

// wdiredSpans colours an entry's line while its name is edited: the listing's
// colours, with those after the name moved along by however much it grew or
// shrank.
func (st *diredState) wdiredSpans(b *text.Buffer, line int, spans []syntax.Span) []syntax.Span {
	i := line - dired.FirstEntry
	if i < 0 || i >= len(st.wd.width) || line >= b.NumLines() {
		return spans
	}
	delta := int(b.Line(line).Len()) - st.wd.width[i]
	if delta == 0 {
		return spans
	}
	end := st.wd.width[i] - st.wd.tail[i]
	out := make([]syntax.Span, len(spans))
	for k, s := range spans {
		switch {
		case s.Start >= end:
			s.Start += delta
			s.End += delta
		case s.End >= end:
			s.End += delta // the name's own colour
		}
		out[k] = s
	}
	return out
}
