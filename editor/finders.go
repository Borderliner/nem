package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/fuzzy"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/text"
)

// Finders, as ripgrep and fzf have made people expect them, built in: no
// program to install beside nem.
//
//   - rg, M-s r, searches a directory's files as the pattern is typed, the
//     matching lines listed as they are found; RET goes to one, and M-RET
//     lists them all in a results buffer, as C-x p g does. The pattern is
//     typed as ripgrep's command line has it: -t go -w count.
//   - fzf, M-s f, finds a file anywhere under a directory by a few letters of
//     its path.
//   - fzf-lines, M-s l, finds a line of the buffer, the cursor following the
//     highlighted one.
//
// Every prompt reads fzf's search syntax: words that must all match, 'exact,
// ^prefix, suffix$, !not, and a | b.

// liveMatches is how many lines a search as you type lists: enough to scroll
// through, and few enough that a search for "e" stops at once.
const liveMatches = 2000

// registerFinderCommands adds rg, fzf and fzf-lines.
func registerFinderCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{Name: "rg", Doc: "Search a directory's files as the pattern is typed, ripgrep's options and all; C-u asks where.",
			Fn: func(command.Env) error {
				dir, err := e.finderDir()
				if err != nil {
					return err
				}
				return e.rgLive(dir)
			}},
		{Name: "fzf", Doc: "Open a file anywhere under a directory, found by a few letters of its path; C-u asks where.",
			Fn: func(command.Env) error {
				dir, err := e.finderDir()
				if err != nil {
					return err
				}
				return e.fzfFiles(dir)
			}},
		{Name: "fzf-lines", Doc: "Go to a line of the buffer, found by a few letters of it.",
			Fn: func(command.Env) error { return e.fzfLines() }},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// finderDir is where rg and fzf look: the project of the buffer on screen, or
// its directory outside every project - or, with C-u, a directory asked for.
func (e *Editor) finderDir() (string, error) {
	if _, explicit := e.Arg(); explicit {
		e.arg.reset()
		ans, err := e.ReadString(command.ReadOpts{
			Prompt:   "In directory: ",
			History:  "directory",
			Initial:  promptDir(e.bufferDir(e.active.Buf)),
			Complete: command.DirectoryCompleter(),
			Descend:  command.IsDirCandidate,
			Icon:     icons.ForCandidate,
			Rewrite:  command.RestartPath,
		})
		if err != nil {
			return "", err
		}
		dir, err := filepath.Abs(command.ExpandPath(ans))
		if err != nil {
			return "", err
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			return "", fmt.Errorf("%s is not a directory", ans)
		}
		return dir, nil
	}
	dir := e.bufferDir(e.active.Buf)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if root, ok := project.Root(dir); ok {
		return root, nil
	}
	return dir, nil
}

// rgLive is M-s r: dir's files searched as the pattern is typed.
func (e *Editor) rgLive(dir string) error {
	// The files are listed once, by the first search, off the event loop;
	// the open buffers are read now, since they are the loop's to read.
	open := e.openLines(dir)
	var listed, listedAll sync.Once
	var files, all []string
	var listErr error
	list := func(noIgnore bool) []string {
		if noIgnore {
			listedAll.Do(func() { all, _ = project.FilesWith(dir, true) })
			return all
		}
		listed.Do(func() { files, listErr = project.FilesWith(dir, false) })
		return files
	}

	// found is what the latest search found, by the text each is shown as,
	// for RET to know where to go. Only a search not overtaken sets it.
	var mu sync.Mutex
	found := map[string]project.Match{}
	search := func(input string, stop *atomic.Bool) []command.Found {
		q, err := project.ParseQuery(input)
		if err != nil || q.Pattern == "" {
			return nil
		}
		ms, _ := project.SearchWith(dir, q.Filter(list(q.NoIgnore)), q.Regexp(), open,
			project.Options{Max: liveMatches, Stop: stop})
		out := make([]command.Found, len(ms))
		byText := make(map[string]project.Match, len(ms))
		for i, m := range ms {
			head := fmt.Sprintf("%s:%d: ", m.File, m.Line+1)
			shift := utf8.RuneCountInString(head)
			f := command.Found{Text: head + m.Text}
			for _, sp := range m.Spans {
				f.Spans = append(f.Spans, [2]int{sp[0] + shift, sp[1] + shift})
			}
			out[i] = f
			byText[f.Text] = m
		}
		mu.Lock()
		if !stop.Load() {
			found = byText
		}
		mu.Unlock()
		return out
	}

	ans, err := e.ReadString(command.ReadOpts{
		Prompt:  fmt.Sprintf("rg in %s: ", project.Name(dir)),
		History: "search",
		Search:  search,
		Icon: func(c string) icons.Icon {
			file, _, _ := strings.Cut(c, ":")
			return icons.ForCandidate(file)
		},
	})
	if err != nil {
		return err
	}
	// A search may still be listing the files; its error is read once it
	// has.
	listed.Do(func() {})
	if listErr != nil && !errors.Is(listErr, project.ErrTooManyFiles) {
		return listErr
	}
	mu.Lock()
	m, ok := found[ans]
	mu.Unlock()
	if !ok {
		// Not a line found: the pattern, as typed with M-RET, every match
		// of which goes in a results buffer.
		if ans == "" {
			return nil
		}
		return e.grepSearch(dir, ans)
	}
	return e.goToMatch(dir, m)
}

// goToMatch visits the file of m, from dir, with point on the match; where
// point was is kept, for C-u C-SPC to come back to.
func (e *Editor) goToMatch(dir string, m project.Match) error {
	from := e.active.Buf
	pt := e.active.Pt
	if err := e.visitFile(fromRel(dir, m.File)); err != nil {
		return err
	}
	if b := e.active.Buf; b != from {
		from.SetMark(pt) // the mark stays with the buffer that had it
	} else {
		b.SetMark(pt)
	}
	e.active.Pt = e.active.Buf.ClampPos(text.Pos{Line: m.Line, Col: text.RuneIdx(m.Col)})
	return nil
}

// fzfFiles is M-s f: a file anywhere under dir, found by a few letters of
// its path, as fzf finds one - .gitignore honoured in a repository.
func (e *Editor) fzfFiles(dir string) error {
	files, err := project.Files(dir)
	if err != nil && !errors.Is(err, project.ErrTooManyFiles) {
		return err
	}
	if errors.Is(err, project.ErrTooManyFiles) {
		e.Echo("%v", err)
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   fmt.Sprintf("fzf in %s: ", tildePath(dir, homeDir())),
		History:  "file",
		Complete: command.CompleteFrom(files),
		Icon:     icons.ForCandidate,
	})
	if err != nil || ans == "" {
		return err
	}
	return e.visitFile(fromRel(dir, ans))
}

// fzfLines is M-s l: a line of the buffer, found by a few letters of it, in
// the buffer's own order, the cursor following the highlighted line. C-g
// puts the cursor back where it was; RET leaves it on the line, at what was
// matched, and the mark where it came from.
func (e *Editor) fzfLines() error {
	w := e.active
	b := w.Buf
	width := len(strconv.Itoa(b.NumLines()))
	lines := make([]string, b.NumLines())
	for i := range lines {
		// Tabs are shown as a space each, so a line's runes are where its
		// text's are.
		lines[i] = fmt.Sprintf("%*d  %s", width, i+1, strings.ReplaceAll(b.Line(i).String(), "\t", " "))
	}
	lineOf := func(cand string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(cand[:min(width, len(cand))]))
		return n - 1
	}
	pt, top, topRow := w.Pt, w.Top, w.TopRow
	var typed string
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:       "Line: ",
		History:      "lines",
		Complete:     command.CompleteFrom(lines),
		KeepOrder:    true,
		RequireMatch: true,
		OnChange:     func(s string) { typed = s },
		Preview: func(cand string) {
			if cand == "" {
				return
			}
			w.Pt = text.Pos{Line: lineOf(cand)}
			w.SetTop(max(w.Pt.Line-e.TextHeight()/2, 0))
		},
	})
	if err != nil || ans == "" {
		w.Pt, w.Top, w.TopRow = pt, top, topRow
		return err
	}
	ln := lineOf(ans)
	col := 0
	// Point goes to the first thing matched in the line itself, not in its
	// number.
	if m, ok := fuzzy.Score(typed, strings.ReplaceAll(b.Line(ln).String(), "\t", " ")); ok && len(m.Indices) > 0 {
		col = m.Indices[0]
	}
	b.SetMark(pt)
	w.Pt = b.ClampPos(text.Pos{Line: ln, Col: text.RuneIdx(col)})
	return nil
}
