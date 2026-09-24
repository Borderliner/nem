package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"unicode"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/icons"
	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui"
)

// Projects, as projectile and emacs's project.el have them: the repository a
// file is in is its project, and C-x p works on the whole of it - finding any
// file in it by a few letters of its name, searching it, replacing across it,
// and switching between it and the other projects nem has seen.
//
// The keys are project.el's, under C-x p, rather than projectile's C-c p: C-c
// is left for the user's own bindings, as emacs leaves it. The commands are
// projectile's, or near enough.

var errNoProjects = errors.New("not in a project, and no other project is known yet")

// registerProjectCommands adds the project commands.
func registerProjectCommands(e *Editor, reg *command.Registry) error {
	// inProject wraps a command that works on the current project.
	inProject := func(fn func(root string) error) func(command.Env) error {
		return func(command.Env) error {
			root, err := e.projectHere()
			if err != nil {
				return err
			}
			return fn(root)
		}
	}
	cmds := []command.Command{
		{Name: "project-find-file", Doc: "Open a file in the current project, found by name.",
			Fn: inProject(e.projectFindFile)},
		{Name: "project-switch-project", Doc: "Open a file in another project nem knows.",
			Fn: func(command.Env) error {
				root, err := e.askProject("Switch to project: ")
				if err != nil {
					return err
				}
				return e.projectFindFile(root)
			}},
		{Name: "project-switch-to-buffer", Doc: "Switch to a buffer of the current project.",
			Fn: inProject(e.projectSwitchToBuffer)},
		{Name: "project-find-dir", Doc: "List a directory of the current project, found by name.",
			Fn: inProject(e.projectFindDir)},
		{Name: "project-dired", Doc: "List the current project's top directory.",
			Fn: inProject(func(root string) error {
				b, err := e.Dired(root)
				if err != nil {
					return err
				}
				e.active.Visit(b)
				return nil
			})},
		{Name: "project-find-regexp", Doc: "Search the current project's files for a regexp, listing the matching lines.",
			Fn: inProject(e.projectGrep)},
		{Name: "project-query-replace", Doc: "Replace text across the current project's files, asking at each match.",
			Fn: inProject(e.projectQueryReplace)},
		{Name: "project-recentf", Doc: "Open one of the current project's recently opened files.",
			Fn: inProject(e.projectRecent)},
		{Name: "project-toggle-test", Doc: "Go from a file to its test, or from a test to the file it tests.",
			Fn: inProject(e.projectToggleTest)},
		{Name: "project-kill-buffers", Doc: "Kill every buffer of the current project, after asking.",
			Fn: inProject(e.projectKillBuffers)},
		{Name: "project-save-buffers", Doc: "Save every modified file of the current project.",
			Fn: inProject(e.projectSaveBuffers)},
		{Name: "project-forget-project", Doc: "Take a project off the known projects.",
			Fn: func(command.Env) error {
				known := e.knownProjects("")
				if len(known) == 0 {
					e.Echo("No projects are known")
					return nil
				}
				ans, err := e.ReadString(command.ReadOpts{
					Prompt:       "Forget project: ",
					Complete:     command.CompleteFrom(known),
					RequireMatch: true,
				})
				if err != nil || ans == "" {
					return err
				}
				root := projectFromCandidate(ans)
				e.mem.ForgetProject(root)
				e.saveMemory()
				e.Echo("Forgot %s", ans)
				return nil
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

// projectHere is the project the active buffer belongs to, which becomes the
// most recent known one. Outside every project it asks for one of those, as
// projectile does, rather than refusing.
func (e *Editor) projectHere() (string, error) {
	dir := e.bufferDir(e.active.Buf)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if root, ok := project.Root(dir); ok {
		e.noteProject(root)
		return root, nil
	}
	return e.askProject("Not in a project; switch to: ")
}

// noteProject records root as the project most recently worked in.
func (e *Editor) noteProject(root string) {
	if len(e.mem.Projects) > 0 && e.mem.Projects[0] == root {
		return
	}
	e.mem.AddProject(root)
	e.saveMemory()
}

// knownProjects are the known projects that still exist, as the prompt shows
// them - under ~, with a separator after - most recent first and current
// last, since switching to it would do nothing.
func (e *Editor) knownProjects(current string) []string {
	home := homeDir()
	var out, last []string
	for _, root := range e.mem.Projects {
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		c := tildePath(root, home) + string(filepath.Separator)
		if root == current {
			last = append(last, c)
		} else {
			out = append(out, c)
		}
	}
	return append(out, last...)
}

// projectFromCandidate turns a project prompt's answer back into a root.
func projectFromCandidate(c string) string {
	p := untildePath(c, homeDir())
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// askProject asks for a known project. A directory that is not one yet can be
// typed too: it becomes known, as the project it is in, or as a project of
// its own.
func (e *Editor) askProject(prompt string) (string, error) {
	current, _ := project.Root(e.bufferDir(e.active.Buf))
	known := e.knownProjects(current)
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   prompt,
		History:  "project",
		Complete: command.CompleteFrom(known),
		Icon:     icons.ForCandidate,
	})
	if err != nil {
		return "", err
	}
	if ans == "" {
		if len(known) == 0 {
			return "", errNoProjects
		}
		return "", command.ErrQuit
	}
	dir := projectFromCandidate(ans)
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", ans)
	}
	root, ok := project.Root(dir)
	if !ok {
		root = dir
	}
	e.noteProject(root)
	return root, nil
}

// projectFiles lists root's files, saying so when the list had to stop short.
func (e *Editor) projectFiles(root string) ([]string, error) {
	files, err := project.Files(root)
	if errors.Is(err, project.ErrTooManyFiles) {
		e.Echo("%v", err)
		err = nil
	}
	return files, err
}

// relTo is path relative to root, with "/" between its parts, and false if it
// is not inside root.
func relTo(root, path string) (string, bool) {
	if path == "" || !project.Contains(root, path) {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// fromRel is a project-relative path made whole again.
func fromRel(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// projectFindFile is C-x p f: any file in the project, by name. The files
// opened most recently come first, so the one wanted is usually a RET away,
// and the file on screen goes last. A name that matches nothing opens a new
// file of that name, from the project's top.
func (e *Editor) projectFindFile(root string) error {
	files, err := e.projectFiles(root)
	if err != nil {
		return err
	}
	cur, _ := relTo(root, e.active.Buf.Path())
	var recent []string
	seen := map[string]bool{}
	for _, p := range e.mem.Recent {
		if rel, ok := relTo(root, p); ok && rel != cur && !seen[rel] {
			seen[rel] = true
			recent = append(recent, rel)
		}
	}
	known := make(map[string]bool, len(files))
	for _, f := range files {
		known[f] = true
	}
	recent = slices.DeleteFunc(recent, func(r string) bool { return !known[r] })
	for _, r := range recent {
		seen[r] = true
	}
	cands := append([]string(nil), recent...)
	for _, f := range files {
		if !seen[f] && f != cur {
			cands = append(cands, f)
		}
	}
	if known[cur] {
		cands = append(cands, cur)
	}

	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   fmt.Sprintf("Find file in %s: ", project.Name(root)),
		History:  "project-file",
		Complete: command.CompleteFrom(cands),
		Icon:     icons.ForCandidate,
	})
	if err != nil || ans == "" {
		return err
	}
	return e.visitFile(fromRel(root, ans))
}

// projectBuffers are the buffers belonging to the project at root: visiting
// its files, listing its directories, or holding a search of it.
func (e *Editor) projectBuffers(root string) []*text.Buffer {
	var out []*text.Buffer
	for _, b := range e.buffers {
		switch {
		case b.Path() != "" && project.Contains(root, b.Path()):
		case e.diredOf(b) != nil && project.Contains(root, e.diredOf(b).dir):
		case e.grepOf(b) != nil && e.grepOf(b).root == root:
		default:
			continue
		}
		out = append(out, b)
	}
	return out
}

// projectSwitchToBuffer is C-x p b: C-x b over the project's buffers.
func (e *Editor) projectSwitchToBuffer(root string) error {
	cur := e.active.Buf
	var names, last []string
	for _, b := range e.projectBuffers(root) {
		if b == cur {
			last = append(last, e.BufferName(b))
		} else {
			names = append(names, e.BufferName(b))
		}
	}
	names = append(names, last...)
	if len(names) == 0 {
		e.Echo("No buffers in %s", project.Name(root))
		return nil
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:       fmt.Sprintf("Switch to buffer in %s: ", project.Name(root)),
		History:      "buffer",
		Complete:     command.CompleteFrom(names),
		Icon:         icons.ForBuffer,
		RequireMatch: true,
	})
	if err != nil || ans == "" {
		return err
	}
	if b, ok := e.byName[ans]; ok {
		e.active.Visit(b)
	}
	return nil
}

// projectFindDir is C-x p d: a directory of the project, by name, listed.
func (e *Editor) projectFindDir(root string) error {
	files, err := e.projectFiles(root)
	if err != nil {
		return err
	}
	cands := append([]string{"./"}, project.Dirs(files)...)
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:       fmt.Sprintf("Directory in %s: ", project.Name(root)),
		History:      "project-dir",
		Complete:     command.CompleteFrom(cands),
		Icon:         icons.ForCandidate,
		RequireMatch: true,
	})
	if err != nil || ans == "" {
		return err
	}
	b, err := e.Dired(fromRel(root, ans))
	if err != nil {
		return err
	}
	e.active.Visit(b)
	return nil
}

// projectRecent is C-x p e: C-x C-r for the project's files alone.
func (e *Editor) projectRecent(root string) error {
	cur, _ := relTo(root, e.active.Buf.Path())
	var cands, last []string
	for _, p := range e.mem.Recent {
		rel, ok := relTo(root, p)
		if !ok {
			continue
		}
		if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if rel == cur {
			last = append(last, rel)
		} else {
			cands = append(cands, rel)
		}
	}
	cands = append(cands, last...)
	if len(cands) == 0 {
		e.Echo("No recent files in %s", project.Name(root))
		return nil
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:   fmt.Sprintf("Recent file in %s: ", project.Name(root)),
		History:  "project-file",
		Complete: command.CompleteFrom(cands),
		Icon:     icons.ForCandidate,
	})
	if err != nil || ans == "" {
		return err
	}
	return e.visitFile(fromRel(root, ans))
}

// projectToggleTest is C-x p t: from a file to its test, or back. When there
// are several, the nearest is offered first.
func (e *Editor) projectToggleTest(root string) error {
	rel, ok := relTo(root, e.active.Buf.Path())
	if !ok {
		e.Echo("This buffer is not a file of %s", project.Name(root))
		return nil
	}
	files, err := e.projectFiles(root)
	if err != nil {
		return err
	}
	other := project.Counterparts(rel, files)
	switch len(other) {
	case 0:
		if project.IsTest(filepath.Base(rel)) {
			e.Echo("No file found that %s tests", filepath.Base(rel))
		} else {
			e.Echo("No test found for %s", filepath.Base(rel))
		}
		return nil
	case 1:
		return e.visitFile(fromRel(root, other[0]))
	}
	ans, err := e.ReadString(command.ReadOpts{
		Prompt:       "Go to: ",
		Complete:     command.CompleteFrom(other),
		Icon:         icons.ForCandidate,
		RequireMatch: true,
	})
	if err != nil || ans == "" {
		return err
	}
	return e.visitFile(fromRel(root, ans))
}

// projectKillBuffers is C-x p k: every buffer of the project killed, after
// asking once - and again for each with unsaved changes.
func (e *Editor) projectKillBuffers(root string) error {
	bufs := e.projectBuffers(root)
	if len(bufs) == 0 {
		e.Echo("No buffers in %s", project.Name(root))
		return nil
	}
	c, err := e.ReadChar(fmt.Sprintf("Kill %d buffer%s in %s? (y/n) ", len(bufs), plural(len(bufs)), project.Name(root)), []rune{'y', 'n'})
	if err != nil {
		return err
	}
	if c != 'y' {
		return nil
	}
	killed := 0
	for _, b := range bufs {
		if b.Modified() && b.Path() != "" {
			c, err := e.ReadChar(fmt.Sprintf("Buffer %s modified; kill anyway? (y/n) ", e.BufferName(b)), []rune{'y', 'n'})
			if err != nil {
				return err
			}
			if c != 'y' {
				continue
			}
		}
		// The last buffer cannot go, so a scratch buffer takes its place.
		if len(e.buffers) == 1 {
			e.NewBuffer(ui.ScratchName)
		}
		if err := e.KillBuffer(b); err != nil {
			return err
		}
		killed++
	}
	e.Echo("Killed %d buffer%s", killed, plural(killed))
	return nil
}

// projectSaveBuffers is C-x p S: every modified file of the project saved,
// without asking about each.
func (e *Editor) projectSaveBuffers(root string) error {
	saved := 0
	for _, b := range e.projectBuffers(root) {
		if !b.Modified() || b.Path() == "" {
			continue
		}
		if err := e.SaveBuffer(b, ""); err != nil {
			return fmt.Errorf("saving %s: %w", b.Path(), err)
		}
		saved++
	}
	if saved == 0 {
		e.Echo("(No files in %s need saving)", project.Name(root))
		return nil
	}
	e.Echo("Saved %d file%s in %s", saved, plural(saved), project.Name(root))
	return nil
}

// searchDefault is what a project search offers to look for when nothing is
// typed: the selection, if it is on one line, or the word at point.
func (e *Editor) searchDefault() string {
	b, w := e.active.Buf, e.active
	if b.MarkActive() {
		lo, hi := text.OrderPos(w.Pt, b.Mark())
		if lo.Line == hi.Line && lo != hi {
			return string(b.Text(lo, hi))
		}
	}
	rs := b.Line(w.Pt.Line).View()
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	lo := min(int(w.Pt.Col), len(rs))
	hi := lo
	for lo > 0 && isWord(rs[lo-1]) {
		lo--
	}
	for hi < len(rs) && isWord(rs[hi]) {
		hi++
	}
	return string(rs[lo:hi])
}

// searchRegexp compiles what was typed at a project search: a regexp, in
// Go's syntax, ignoring case unless it has a capital letter - as isearch
// does. Typed text that is not a valid regexp is searched for as it is, so
// looking for "f(" finds f( rather than complaining.
func searchRegexp(pat string) *regexp.Regexp {
	flags := ""
	if command.FoldCase(pat) {
		flags = "(?i)"
	}
	if re, err := regexp.Compile(flags + pat); err == nil {
		return re
	}
	return regexp.MustCompile(flags + regexp.QuoteMeta(pat))
}

// projectGrep is C-x p g: the project searched for a regexp.
func (e *Editor) projectGrep(root string) error {
	def := e.searchDefault()
	prompt := fmt.Sprintf("Search %s for: ", project.Name(root))
	if def != "" {
		prompt = fmt.Sprintf("Search %s for (default %s): ", project.Name(root), def)
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
	return e.grepSearch(root, ans, searchRegexp(ans))
}

// projectQueryReplace is C-x p r: M-% across every file of the project that
// has a match, each opened in turn. Besides query-replace's answers, N
// leaves the rest of a file alone and Y replaces everything left, in every
// file. The files are left modified, for C-x p S to save.
func (e *Editor) projectQueryReplace(root string) error {
	name := project.Name(root)
	from, err := e.ReadString(command.ReadOpts{Prompt: fmt.Sprintf("Query replace in %s: ", name), History: "replace"})
	if err != nil || from == "" {
		return err
	}
	to, err := e.ReadString(command.ReadOpts{Prompt: fmt.Sprintf("Query replace %s with: ", from), History: "replace"})
	if err != nil {
		return err
	}
	fold := command.FoldCase(from)
	flags := ""
	if fold {
		flags = "(?i)"
	}
	files, err := e.projectFiles(root)
	if err != nil {
		return err
	}
	e.Echo("Searching %s…", name)
	e.Redraw()
	found, _ := project.Search(root, files, regexp.MustCompile(flags+regexp.QuoteMeta(from)), e.openLines(root))
	var todo []string
	for i, m := range found {
		if i == 0 || m.File != found[i-1].File {
			todo = append(todo, m.File)
		}
	}
	if len(todo) == 0 {
		e.Echo("No matches for %s in %s", from, name)
		return nil
	}

	n, touched, skipped := 0, 0, 0
	all := false
	answers := []rune{'y', 'n', '!', 'N', 'Y', 'q'}
	report := func() {
		msg := fmt.Sprintf("Replaced %d occurrence%s in %d file%s", n, plural(n), touched, plural(touched))
		if skipped > 0 {
			msg += fmt.Sprintf(" (skipped %d that cannot be edited)", skipped)
		}
		if n > 0 {
			msg += "; C-x p S saves them"
		}
		e.Echo("%s", msg)
	}
files:
	for _, rel := range todo {
		b, err := e.OpenFile(fromRel(root, rel))
		if err != nil {
			return err
		}
		e.active.Visit(b)
		fileAll, changed := all, false
		at := text.Pos{}
		for {
			start, end, ok := command.SearchForward(b, from, at, fold)
			if !ok {
				break
			}
			if !command.CanReplace(b, start, end, to) {
				skipped++
				at = end
				continue
			}
			e.active.Pt = start
			if !fileAll {
				c, err := e.ReadChar(fmt.Sprintf("Query replacing %s with %s (y/n/!/N/Y/q): ", from, to), answers)
				if err != nil {
					return err
				}
				switch c {
				case 'n':
					at = end
					continue
				case '!':
					fileAll = true
				case 'Y':
					fileAll, all = true, true
				case 'N':
					continue files
				case 'q':
					report()
					return nil
				}
			}
			if at, err = command.ReplaceMatch(b, start, end, to); err != nil {
				return err
			}
			e.active.Pt = at
			n++
			if !changed {
				changed = true
				touched++
			}
		}
	}
	report()
	return nil
}
