package editor

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// A project search's results: a listing of the matching lines grouped under
// their files, read-only like a directory listing and with keys of its own.
// RET goes to a match in the other window, n and p step through them showing
// each there, and M-g n and M-g p step through them from any buffer, as
// emacs's next-error does.
//
// Occur, M-s o, is the same listing for the lines of one buffer.

// grepName and occurName are the results buffers' names. One of each is kept
// and reused, as emacs keeps one *grep* and one *Occur*: a search replaces
// the last one's results.
const (
	grepName  = "*grep*"
	occurName = "*Occur*"
)

// grepState is what makes a buffer a search's results.
type grepState struct {
	root string
	// buf is the buffer an occur searched, which its matches lead to; nil
	// for a search of a project's files.
	buf     *text.Buffer
	pattern string
	re      *regexp.Regexp
	// query is a project search's options - its file types and globs, and
	// -u - for searching again with g.
	query   project.Query
	matches []project.Match
	more    bool

	// match maps each buffer line to the match it shows, or -1 for the
	// header, a file's heading and the blank lines.
	match []int
	// heading maps each file's heading line to its first match; -1 elsewhere.
	heading []int
	// textCol is where a match line's text starts.
	textCol int
	spans   [][]syntax.Span

	// cur is the match last gone to, -1 before the first.
	cur int
}

var (
	errNotGrep       = errors.New("not a search's results")
	errOccurGone     = errors.New("the buffer these lines were found in has been killed")
	errNoMoreMatches = errors.New("no more matches")
)

// grepBindings is the results' keymap. Printable keys are free to take, the
// buffer being read-only.
var grepBindings = []struct{ Spec, Command string }{
	{"RET", "grep-goto-match"},
	{"o", "grep-display-match"},
	{"n", "grep-next-match"},
	{"p", "grep-previous-match"},
	{"C-n", "grep-next-line"},
	{"<down>", "grep-next-line"},
	{"C-p", "grep-previous-line"},
	{"<up>", "grep-previous-line"},
	{"}", "grep-next-file"},
	{"{", "grep-previous-file"},
	{"g", "grep-revert"},
	{"q", "quit-window"},
}

// newGrepKeymap builds the results' keymap from grepBindings.
func newGrepKeymap() (*keymap.Map, error) {
	m := keymap.New()
	for _, b := range grepBindings {
		if err := bindSpec(m, b.Spec, b.Command); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// grepOf returns b's results, or nil when b holds none.
func (e *Editor) grepOf(b *text.Buffer) *grepState {
	if b == nil {
		return nil
	}
	return e.grep[b]
}

// grepSearch searches the files at root for query, typed as a ripgrep
// command line - see project.ParseQuery - and shows the results in the other
// window, selecting it. Nothing found shows nothing: an empty list would
// only have to be closed again.
func (e *Editor) grepSearch(root, query string) error {
	q, err := project.ParseQuery(query)
	if err != nil {
		return err
	}
	return e.showResults(grepName, &grepState{root: root, pattern: query, re: q.Regexp(), query: q, cur: -1})
}

// occur lists the lines of b that re matches, as grepSearch lists a
// project's.
func (e *Editor) occur(b *text.Buffer, pattern string, re *regexp.Regexp) error {
	return e.showResults(occurName, &grepState{buf: b, root: e.bufferDir(b), pattern: pattern, re: re, cur: -1})
}

// showResults runs st's search and shows its results in the buffer called
// name.
func (e *Editor) showResults(name string, st *grepState) error {
	if err := e.grepRun(st); err != nil {
		return err
	}
	if len(st.matches) == 0 {
		e.Echo("No matches for %s in %s", st.pattern, st.where())
		return nil
	}

	b, ok := e.byName[name]
	if !ok || e.grepOf(b) == nil {
		b = e.NewBuffer(e.uniqueName(name))
		b.SetReadOnly(true)
	}
	e.grep[b] = st
	e.nextErrorBuf = b
	e.grepRender(b, st)
	first := st.firstMatchLine()

	if e.active.Buf != b {
		if len(e.tree.Windows()) < 2 {
			if err := e.SplitWindow(true); err != nil {
				return err
			}
		} else {
			e.OtherWindow(1)
		}
		e.active.Visit(b)
	}
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			w.Pt = st.pointOn(first)
			w.SetTop(0)
		}
	}
	e.Echo("%s", st.summary())
	return nil
}

// grepRun searches, filling st's matches. Open buffers are searched as they
// are, unsaved edits and all, so the lines found are the lines a match takes
// you to.
func (e *Editor) grepRun(st *grepState) error {
	if st.buf != nil {
		if !slices.Contains(e.buffers, st.buf) {
			return errOccurGone
		}
		lines := make([]string, st.buf.NumLines())
		for i := range lines {
			lines[i] = st.buf.Line(i).String()
		}
		st.matches = project.MatchLines(e.BufferName(st.buf), lines, st.re)
		if st.more = len(st.matches) > project.MaxMatches; st.more {
			st.matches = st.matches[:project.MaxMatches]
		}
		return nil
	}
	files, err := project.FilesWith(st.root, st.query.NoIgnore)
	if err != nil && !errors.Is(err, project.ErrTooManyFiles) {
		return err
	}
	e.Echo("Searching %s…", project.Name(st.root))
	e.Redraw()
	st.matches, st.more = project.Search(st.root, st.query.Filter(files), st.re, e.openLines(st.root))
	return nil
}

// openLines reads the text of every buffer visiting a file in the project at
// root, for a search to use instead of the disk. They are read here, before
// the search starts, because the search reads in parallel and a buffer is not
// something to share between goroutines.
func (e *Editor) openLines(root string) project.Lines {
	open := map[string][]string{}
	for _, b := range e.buffers {
		p := b.Path()
		if p == "" || !project.Contains(root, p) {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			continue
		}
		lines := make([]string, b.NumLines())
		for i := range lines {
			lines[i] = b.Line(i).String()
		}
		open[filepath.ToSlash(rel)] = lines
	}
	return func(rel string) ([]string, bool) {
		lines, ok := open[rel]
		return lines, ok
	}
}

// summary is the one line saying what a search found.
func (st *grepState) summary() string {
	files := 0
	for i, m := range st.matches {
		if i == 0 || m.File != st.matches[i-1].File {
			files++
		}
	}
	matches := "matches"
	if len(st.matches) == 1 {
		matches = "match"
	}
	s := fmt.Sprintf("%d %s in %d file%s", len(st.matches), matches, files, plural(files))
	if st.more {
		s += ", stopped there; narrow the search"
	}
	return s
}

// grepRender writes st's results into b.
func (e *Editor) grepRender(b *text.Buffer, st *grepState) {
	width := 1
	for _, m := range st.matches {
		width = max(width, len(strconv.Itoa(m.Line+1)))
	}
	st.textCol = 3 + width + 2

	var lines []string
	st.match, st.heading, st.spans = nil, nil, nil
	add := func(line string, match, heading int, spans []syntax.Span) {
		lines = append(lines, line)
		st.match = append(st.match, match)
		st.heading = append(st.heading, heading)
		st.spans = append(st.spans, spans)
	}

	head, headSpans := st.header(homeDir())
	add(head, -1, -1, headSpans)
	for i, m := range st.matches {
		if i == 0 || m.File != st.matches[i-1].File {
			add("", -1, -1, nil)
			line, spans := " ", []syntax.Span(nil)
			if e.icons {
				ic := icons.For(m.File, icons.File)
				line += string(ic.Glyph) + " "
				spans = append(spans, syntax.Span{Start: 1, End: 2, Class: ic.Class})
			}
			start := utf8.RuneCountInString(line)
			line += m.File
			spans = append(spans, syntax.Span{Start: start, End: start + utf8.RuneCountInString(m.File), Class: syntax.Function})
			add(line, -1, i, spans)
		}
		num := strconv.Itoa(m.Line + 1)
		line := "   " + strings.Repeat(" ", width-len(num)) + num + "  " + m.Text
		spans := []syntax.Span{{Start: 3, End: 3 + width, Class: syntax.Comment}}
		for _, s := range m.Spans {
			spans = append(spans, syntax.Span{Start: st.textCol + s[0], End: st.textCol + s[1], Class: syntax.Type})
		}
		add(line, i, -1, spans)
	}
	if st.more {
		add("", -1, -1, nil)
		note := fmt.Sprintf("   (stopped at %d matches; narrow the search)", project.MaxMatches)
		add(note, -1, -1, []syntax.Span{{Start: 3, End: utf8.RuneCountInString(note), Class: syntax.Comment}})
	}
	b.Regenerate([]rune(strings.Join(lines, "\n")))
}

// header is the results' first line: what was searched for, where, and what
// was found.
func (st *grepState) header(home string) (string, []syntax.Span) {
	var spans []syntax.Span
	at := func(s string, c syntax.Class, line *string) {
		start := utf8.RuneCountInString(*line)
		*line += s
		spans = append(spans, syntax.Span{Start: start, End: start + utf8.RuneCountInString(s), Class: c})
	}
	line := " "
	at(st.pattern, syntax.Type, &line)
	line += "  in  "
	dir := tildePath(st.root, home) + string(filepath.Separator)
	if st.root == home {
		dir = "~" + string(filepath.Separator)
	}
	if st.buf != nil {
		dir = st.where()
	}
	at(dir, syntax.Function, &line)
	line += "  "
	at(st.summary(), syntax.Comment, &line)
	return line, spans
}

// where names what was searched, for messages: the project, or the buffer.
func (st *grepState) where() string {
	if st.buf != nil {
		return "this buffer"
	}
	return project.Name(st.root)
}

// lineSpans is the results' colouring for line.
func (st *grepState) lineSpans(line int) []syntax.Span {
	if line < 0 || line >= len(st.spans) {
		return nil
	}
	return st.spans[line]
}

// count, target, lineOf, current and setCurrent make the results a list of
// locations, for M-g n and the keys they share with a compilation's errors.
func (st *grepState) count() int { return len(st.matches) }

func (st *grepState) target(i int) (string, int, int) {
	m := st.matches[i]
	if st.buf != nil {
		return st.buf.Path(), m.Line, m.Col
	}
	return fromRel(st.root, m.File), m.Line, m.Col
}

// targetBuffer is the buffer an occur's matches lead to, which need not have
// a file: *scratch* can be searched too.
func (st *grepState) targetBuffer() *text.Buffer { return st.buf }

func (st *grepState) lineOf(i int) int { return st.lineOfMatch(i) }
func (st *grepState) current() int     { return st.cur }
func (st *grepState) setCurrent(i int) { st.cur = i }
func (st *grepState) noun() string     { return "match" }

// pointOn is where point rests on line: at a match's text, or a heading's
// file name.
func (st *grepState) pointOn(line int) text.Pos {
	if line >= 0 && line < len(st.match) && st.match[line] < 0 {
		return text.Pos{Line: line, Col: 1}
	}
	return text.Pos{Line: line, Col: text.RuneIdx(st.textCol)}
}

// firstMatchLine is the buffer line of the first match.
func (st *grepState) firstMatchLine() int {
	for l, m := range st.match {
		if m >= 0 {
			return l
		}
	}
	return 0
}

// lineOfMatch is the buffer line showing match i.
func (st *grepState) lineOfMatch(i int) int {
	for l, m := range st.match {
		if m == i {
			return l
		}
	}
	return 0
}

// matchNear is the match on line, or on the heading's file for a heading.
func (st *grepState) matchNear(line int) (int, bool) {
	if line < 0 || line >= len(st.match) {
		return 0, false
	}
	if m := st.match[line]; m >= 0 {
		return m, true
	}
	if h := st.heading[line]; h >= 0 {
		return h, true
	}
	return 0, false
}

// step finds the next line from line, forward or back, that satisfies ok.
func (st *grepState) step(line, dir int, ok func(int) bool) (int, bool) {
	for l := line + dir; l >= 0 && l < len(st.match); l += dir {
		if ok(l) {
			return l, true
		}
	}
	return line, false
}

// registerGrepCommands adds the results' commands.
func registerGrepCommands(e *Editor, reg *command.Registry) error {
	inGrep := func(fn func(b *text.Buffer, st *grepState) error) func(command.Env) error {
		return func(command.Env) error {
			b := e.active.Buf
			st := e.grepOf(b)
			if st == nil {
				return errNotGrep
			}
			return fn(b, st)
		}
	}
	// moveTo steps point to the next line that ok accepts, ARG times.
	moveTo := func(dir int, ok func(st *grepState, line int) bool, show bool) func(command.Env) error {
		return inGrep(func(b *text.Buffer, st *grepState) error {
			n, _ := e.Arg()
			if n < 0 {
				n, dir = -n, -dir
			}
			line := e.active.Pt.Line
			// Point starts on the first match, so the first n shows that one
			// rather than skipping it, as emacs's first n shows the first.
			if show && st.cur < 0 && line < len(st.match) && st.match[line] >= 0 {
				n--
			}
			for range n {
				next, found := st.step(line, dir, func(l int) bool { return ok(st, l) })
				if !found {
					e.active.Pt = st.pointOn(line)
					return errNoMoreMatches
				}
				line = next
			}
			e.active.Pt = st.pointOn(line)
			if show {
				if i, ok := st.matchNear(line); ok {
					return e.visitLocation(b, st, i, false)
				}
			}
			return nil
		})
	}
	isMatch := func(st *grepState, l int) bool { return st.match[l] >= 0 }
	isHeading := func(st *grepState, l int) bool { return st.heading[l] >= 0 }

	cmds := []command.Command{
		{Name: "grep-goto-match", Doc: "Go to the match at point, in the other window.",
			Fn: inGrep(func(b *text.Buffer, st *grepState) error {
				i, ok := st.matchNear(e.active.Pt.Line)
				if !ok {
					e.Echo("No match on this line")
					return nil
				}
				return e.visitLocation(b, st, i, true)
			})},
		{Name: "grep-display-match", Doc: "Show the match at point in the other window, staying here.",
			Fn: inGrep(func(b *text.Buffer, st *grepState) error {
				i, ok := st.matchNear(e.active.Pt.Line)
				if !ok {
					e.Echo("No match on this line")
					return nil
				}
				return e.visitLocation(b, st, i, false)
			})},
		{Name: "grep-next-match", Doc: "Move to the next match, ARG matches on, showing it in the other window.",
			Fn: moveTo(1, isMatch, true)},
		{Name: "grep-previous-match", Doc: "Move to the previous match, ARG matches back, showing it in the other window.",
			Fn: moveTo(-1, isMatch, true)},
		{Name: "grep-next-line", Doc: "Move to the next match, ARG matches on.",
			Fn: moveTo(1, isMatch, false)},
		{Name: "grep-previous-line", Doc: "Move to the previous match, ARG matches back.",
			Fn: moveTo(-1, isMatch, false)},
		{Name: "grep-next-file", Doc: "Move to the next file's matches, ARG files on.",
			Fn: moveTo(1, isHeading, false)},
		{Name: "grep-previous-file", Doc: "Move to the previous file's matches, ARG files back.",
			Fn: moveTo(-1, isHeading, false)},
		{Name: "occur", Doc: "List the lines of this buffer that match a regexp, each leading to its line.",
			Fn: func(command.Env) error {
				b := e.active.Buf
				def := e.searchDefault()
				prompt := "List lines matching: "
				if def != "" {
					prompt = fmt.Sprintf("List lines matching (default %s): ", def)
				}
				ans, err := e.ReadString(command.ReadOpts{Prompt: prompt, History: "search"})
				if err != nil {
					return err
				}
				if ans == "" {
					ans = def
				}
				if ans == "" {
					return nil
				}
				return e.occur(b, ans, searchRegexp(ans))
			}},
		{Name: "grep-revert", Doc: "Search again, for the same thing in the same project or buffer.",
			Fn: inGrep(func(b *text.Buffer, st *grepState) error {
				line := e.active.Pt.Line
				if err := e.grepRun(st); err != nil {
					return err
				}
				st.cur = -1
				e.grepRender(b, st)
				e.active.Pt = st.pointOn(min(line, b.NumLines()-1))
				e.Echo("%s", st.summary())
				return nil
			})},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}
