package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/compile"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/project"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// Compilation: M-x compile runs a build, a test suite, a linter - any
// command - and shows its output as it comes, in *compilation*. Every line
// that names a place in a file - an error, a warning, a failing test's
// assertion - leads there: RET on it, n and p through them, M-g n from
// anywhere. It is emacs's compilation mode, down to g running it again and
// C-c C-k stopping it.
//
// M-& runs a command the same way into *Async Shell Command*, without
// becoming what M-g n steps through.

const (
	compilationName = "*compilation*"
	asyncShellName  = "*Async Shell Command*"
	// outputFrom is the output's first line: the header comes first, then a
	// blank line.
	outputFrom = 2
)

// compileState is what makes a buffer a command's output.
type compileState struct {
	command, dir string
	async        bool

	proc    *process
	start   time.Time
	took    time.Duration
	running bool
	code    int
	killed  bool

	// open is the last line of output, which the command is still writing.
	open string
	// locs are the places the output names, in order; locAt maps a buffer
	// line to the place on it.
	locs  []compileLoc
	locAt map[int]int
	cur   int
	spans map[int][]syntax.Span

	// resolved maps a file as the output names it to the file it is, "" for
	// one that names nothing; choices holds the candidates for a bare name
	// several files of the project answer to.
	resolved map[string]string
	choices  map[string][]string
	root     string
	files    []string
	listed   bool
}

// compileLoc is a place the output names.
type compileLoc struct {
	line int // the buffer line naming it
	// file is as the output gave it, and path what it resolved to - "" when
	// several files answer to it and the user has yet to say which.
	file    string
	path    string
	ln, col int // from 0
	kind    compile.Kind
}

// compileBindings is the output's keymap. The buffer is read-only, so
// printable keys are free; C-n, C-s and the rest move and search as in any
// text.
var compileBindings = []struct{ Spec, Command string }{
	{"RET", "compile-goto-error"},
	{"o", "compilation-display-error"},
	{"C-o", "compilation-display-error"},
	{"n", "compilation-next-error"},
	{"p", "compilation-previous-error"},
	{"g", "recompile"},
	{"C-c C-k", "kill-compilation"},
	{"q", "quit-window"},
}

// newCompileKeymap builds the output's keymap from compileBindings.
func newCompileKeymap() (*keymap.Map, error) {
	m := keymap.New()
	for _, b := range compileBindings {
		if err := bindSpec(m, b.Spec, b.Command); err != nil {
			return nil, err
		}
	}
	return m, nil
}

var (
	errNotCompilation = errors.New("not a compilation's output")
	errNoCompilation  = errors.New("nothing has been compiled yet; M-x compile runs a command")
)

// compileOf returns b's output state, or nil when b holds none.
func (e *Editor) compileOf(b *text.Buffer) *compileState {
	if b == nil {
		return nil
	}
	return e.compile[b]
}

// count, target, lineOf, pointOn, current, setCurrent and noun make the
// output a list of locations; see locations.go.
func (st *compileState) count() int { return len(st.locs) }

func (st *compileState) target(i int) (string, int, int) {
	l := st.locs[i]
	return l.path, l.ln, l.col
}

func (st *compileState) lineOf(i int) int          { return st.locs[i].line }
func (st *compileState) pointOn(line int) text.Pos { return text.Pos{Line: line} }
func (st *compileState) current() int              { return st.cur }
func (st *compileState) setCurrent(i int)          { st.cur = i }
func (st *compileState) noun() string              { return "error" }
func (st *compileState) choicesFor(i int) []string { return st.choices[st.locs[i].file] }
func (st *compileState) choose(i int, rel string)  { st.pick(st.locs[i].file, fromRel(st.root, rel)) }

// pick settles which file a bare name means, for every place that names it.
func (st *compileState) pick(file, path string) {
	st.resolved[file] = path
	for k := range st.locs {
		if st.locs[k].file == file {
			st.locs[k].path = path
		}
	}
}

// registerCompileCommands adds compilation and its keys.
func registerCompileCommands(e *Editor, reg *command.Registry) error {
	inOutput := func(fn func(b *text.Buffer, st *compileState) error) func(command.Env) error {
		return func(command.Env) error {
			b := e.active.Buf
			st := e.compileOf(b)
			if st == nil {
				return errNotCompilation
			}
			return fn(b, st)
		}
	}
	// step moves point to the next place named after it, or before it, and
	// shows that place in the other window.
	step := func(dir int) func(command.Env) error {
		return inOutput(func(b *text.Buffer, st *compileState) error {
			n, _ := e.Arg()
			if n < 0 {
				n, dir = -n, -dir
			}
			line, found := e.active.Pt.Line, -1
			for range n {
				found = -1
				for i := range st.locs {
					k := i
					if dir < 0 {
						k = len(st.locs) - 1 - i
					}
					if (dir > 0 && st.locs[k].line > line) || (dir < 0 && st.locs[k].line < line) {
						found = k
						break
					}
				}
				if found < 0 {
					return fmt.Errorf("no more errors")
				}
				line = st.locs[found].line
			}
			e.active.Pt = st.pointOn(line)
			return e.visitLocation(b, st, found, false)
		})
	}
	// here is the place on point's line.
	here := func(sel bool) func(command.Env) error {
		return inOutput(func(b *text.Buffer, st *compileState) error {
			i, ok := st.locAt[e.active.Pt.Line]
			if !ok {
				e.Echo("No error on this line")
				return nil
			}
			return e.visitLocation(b, st, i, sel)
		})
	}

	cmds := []command.Command{
		{Name: "compile", Doc: "Run a command - a build, the tests - showing its output and leading to the places it names.",
			Fn: func(command.Env) error {
				dir := e.bufferDir(e.active.Buf)
				if dir == "" {
					dir, _ = os.Getwd()
				}
				return e.compilePrompt("Compile command: ", dir)
			}},
		{Name: "project-compile", Doc: "Run a compilation from the top of the current project.",
			Fn: func(command.Env) error {
				root, err := e.projectHere()
				if err != nil {
					return err
				}
				return e.compilePrompt(fmt.Sprintf("Compile %s: ", project.Name(root)), root)
			}},
		{Name: "recompile", Doc: "Run the last compilation again, or the one this buffer shows.",
			Fn: func(command.Env) error {
				st := e.compileOf(e.active.Buf)
				if st == nil {
					st = e.compileOf(e.byName[compilationName])
				}
				if st == nil {
					return errNoCompilation
				}
				return e.runCompile(st.command, st.dir, st.async)
			}},
		{Name: "kill-compilation", Doc: "Stop the running compilation.",
			Fn: func(command.Env) error {
				st := e.compileOf(e.active.Buf)
				if st == nil {
					st = e.compileOf(e.byName[compilationName])
				}
				if st == nil || !st.running {
					e.Echo("No compilation is running")
					return nil
				}
				st.proc.kill()
				return nil
			}},
		{Name: "async-shell-command", Doc: "Run a shell command, showing its output as it comes, in *Async Shell Command*.",
			Fn: func(command.Env) error {
				line, err := e.ReadString(command.ReadOpts{Prompt: "Async shell command: ", History: "shell"})
				if err != nil || strings.TrimSpace(line) == "" {
					return err
				}
				dir := e.bufferDir(e.active.Buf)
				if dir == "" {
					dir, _ = os.Getwd()
				}
				return e.runCompile(line, dir, true)
			}},
		{Name: "compile-goto-error", Doc: "Go to the place the line at point names, in the other window.",
			Fn: here(true)},
		{Name: "compilation-display-error", Doc: "Show the place the line at point names in the other window, staying here.",
			Fn: here(false)},
		{Name: "compilation-next-error", Doc: "Move to the next line naming a place, ARG on, showing it in the other window.",
			Fn: step(1)},
		{Name: "compilation-previous-error", Doc: "Move to the previous line naming a place, ARG back, showing it in the other window.",
			Fn: step(-1)},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// compilePrompt asks for a command to run in dir, offering the one run there
// last, or a guess from the project's build files the first time.
func (e *Editor) compilePrompt(prompt, dir string) error {
	initial, ok := e.compileCommands[dir]
	if !ok {
		initial = guessCompileCommand(dir)
	}
	line, err := e.ReadString(command.ReadOpts{Prompt: prompt, Initial: initial, History: "compile"})
	if err != nil || strings.TrimSpace(line) == "" {
		return err
	}
	return e.runCompile(line, dir, false)
}

// guessCompileCommand is what a project's build files say builds it, and
// emacs's make -k when they say nothing.
func guessCompileCommand(dir string) string {
	root, ok := project.Root(dir)
	if !ok {
		root = dir
	}
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	switch {
	case has("go.mod"):
		return "go build ./..."
	case has("Cargo.toml"):
		return "cargo build"
	case has("xmake.lua"):
		return "xmake"
	case has("Makefile"), has("makefile"), has("GNUmakefile"):
		return "make -k"
	case has("build.zig"):
		return "zig build"
	case has("package.json"):
		return "npm run build"
	case has("CMakeLists.txt"):
		return "cmake --build build"
	case has("mix.exs"):
		return "mix compile"
	case has("pyproject.toml"):
		return "python -m pytest"
	}
	return "make -k"
}

// runCompile runs line in dir into the output buffer - *compilation*, or
// *Async Shell Command* for M-& - and shows it without leaving the window
// being worked in.
func (e *Editor) runCompile(line, dir string, async bool) error {
	name, what := compilationName, "compilation"
	if async {
		name, what = asyncShellName, "command"
	}
	b, ok := e.byName[name]
	if !ok || e.compileOf(b) == nil {
		b = e.NewBuffer(e.uniqueName(name))
		b.SetReadOnly(true)
	}
	if old := e.compileOf(b); old != nil && old.running {
		c, err := e.ReadChar(fmt.Sprintf("A %s is running; kill it? (y/n) ", what), []rune{'y', 'n'})
		if err != nil {
			return err
		}
		if c != 'y' {
			return nil
		}
		old.proc.kill()
	}

	p, err := e.startProcess(line, dir, nil)
	if err != nil {
		return err
	}
	st := &compileState{
		command: line, dir: dir, async: async, proc: p, start: time.Now(), running: true,
		cur: -1, locAt: map[int]int{}, spans: map[int][]syntax.Span{},
		resolved: map[string]string{}, choices: map[string][]string{},
	}
	e.compile[b] = st
	if !async {
		e.nextErrorBuf = b
		e.compileCommands[dir] = line
	}
	head, spans := st.header(homeDir())
	st.spans[0] = spans
	b.Regenerate([]rune(head + "\n\n"))
	e.showBeside(b)
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			w.Pt, w.Top = text.Pos{Line: outputFrom}, 0
		}
	}
	return nil
}

// showBeside puts b in a window without leaving the one being worked in: the
// window already showing it, or the other one, split off if there is only
// this one.
func (e *Editor) showBeside(b *text.Buffer) {
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			return
		}
	}
	from := e.active
	if len(e.tree.Windows()) < 2 {
		if err := e.SplitWindow(true); err != nil {
			e.Echo("%v", err)
			return
		}
	} else {
		e.OtherWindow(1)
	}
	e.active.Visit(b)
	e.active = from
}

// processWoke takes in what a process has for the loop.
func (e *Editor) processWoke(p *process) {
	for b, st := range e.compile {
		if st.proc != p {
			continue
		}
		out, done := p.take()
		if len(out) > 0 {
			e.compileOutput(b, st, out)
		}
		if done && st.running {
			e.compileFinished(b, st)
		}
		return
	}
	// A process nothing shows any more: its buffer was killed, or a newer
	// run replaced it. What it has is dropped.
	p.take()
}

// compileOutput appends output to b, a line at a time as lines complete.
// Windows at the end of the output follow it, as a terminal does; one moved
// up to read stays where it was put.
func (e *Editor) compileOutput(b *text.Buffer, st *compileState, out []byte) {
	last := b.NumLines() - 1
	var follow []int
	for i, w := range e.tree.Windows() {
		if w.Buf == b && w.Pt.Line >= last {
			follow = append(follow, i)
		}
	}

	whole := st.open + string(out)
	parts := strings.Split(whole, "\n")
	st.open = parts[len(parts)-1]
	// One slice for all of it, sized once: the output of a busy build is
	// most of what this does, and converting each line on its own grew the
	// slice a line at a time.
	rs := make([]rune, 0, utf8.RuneCountInString(whole))
	for i, part := range parts[:len(parts)-1] {
		line := compile.Clean(part)
		st.note(last+i, line)
		for _, r := range line {
			rs = append(rs, r)
		}
		rs = append(rs, '\n')
	}
	for _, r := range compile.Clean(st.open) {
		rs = append(rs, r)
	}
	b.RegenerateTail(last, rs)

	ws := e.tree.Windows()
	for _, i := range follow {
		ws[i].Pt = text.Pos{Line: b.NumLines() - 1}
	}
}

// note records the place a complete line of output names, if it names one
// that exists.
func (st *compileState) note(line int, s string) {
	loc, ok := compile.Parse(s)
	if !ok {
		return
	}
	path, known := st.resolve(loc.File)
	if !known {
		return
	}
	st.locAt[line] = len(st.locs)
	st.locs = append(st.locs, compileLoc{
		line: line, file: loc.File, path: path,
		ln: loc.Line - 1, col: max(loc.Col-1, 0), kind: loc.Kind,
	})
	class := syntax.Number
	switch loc.Kind {
	case compile.Warning:
		class = syntax.Type
	case compile.Info:
		class = syntax.String
	}
	st.spans[line] = []syntax.Span{{Start: loc.Start, End: loc.End, Class: class}}
}

// resolve finds the file a name in the output means: as given, from where
// the command ran; or else, for a bare name - go test names a failing test's
// file without its directory - the one file of the project it can be. Where
// several can, it is left for the user to say which on the way there, and
// the path is "". False means the name is not a file at all.
func (st *compileState) resolve(file string) (string, bool) {
	if p, ok := st.resolved[file]; ok {
		return p, p != "" || len(st.choices[file]) > 0
	}
	p := file
	if !filepath.IsAbs(p) {
		p = filepath.Join(st.dir, p)
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
		st.resolved[file] = p
		return p, true
	}
	st.resolved[file] = ""
	if filepath.IsAbs(file) {
		return "", false
	}
	if !st.listed {
		st.listed = true
		if root, ok := project.Root(st.dir); ok {
			st.root = root
			st.files, _ = project.Files(root)
		}
	}
	want := filepath.ToSlash(filepath.Clean(file))
	var found []string
	for _, f := range st.files {
		if f == want || strings.HasSuffix(f, "/"+want) {
			found = append(found, f)
		}
	}
	switch len(found) {
	case 0:
		return "", false
	case 1:
		st.resolved[file] = fromRel(st.root, found[0])
		return st.resolved[file], true
	}
	st.choices[file] = found
	return "", true
}

// compileFinished closes the output with how the command ended, and reads
// again any file it changed.
func (e *Editor) compileFinished(b *text.Buffer, st *compileState) {
	code, killed := st.proc.result()
	st.running, st.code, st.killed, st.took = false, code, killed, time.Since(st.start)

	last := b.NumLines() - 1
	tail := compile.Clean(st.open)
	st.open = ""
	if tail != "" {
		st.note(last, tail)
		tail += "\n"
	}
	status, class := st.status()
	statusLine := "── " + status
	b.RegenerateTail(last, []rune(tail+"\n"+statusLine))
	st.spans[b.NumLines()-1] = []syntax.Span{{Start: 0, End: utf8.RuneCountInString(statusLine), Class: class}}

	head, spans := st.header(homeDir())
	b.RegenerateLine(0, []rune(head))
	st.spans[0] = spans

	what := "Compilation"
	if st.async {
		what = "Command"
	}
	msg := what + " " + status
	if n := len(st.locs); n > 0 && !st.async {
		noun := "errors"
		if n == 1 {
			noun = "error"
		}
		msg += fmt.Sprintf(": %d %s", n, noun)
	}
	if reverted := e.afterCommand(); reverted != "" {
		msg += "; " + strings.ToLower(reverted[:1]) + reverted[1:]
	}
	e.Echo("%s", msg)
}

// status says how the command ended, or that it has not, and how to colour
// saying so.
func (st *compileState) status() (string, syntax.Class) {
	took := st.took.Round(100 * time.Millisecond)
	switch {
	case st.running:
		return "running…", syntax.Comment
	case st.killed:
		return fmt.Sprintf("killed after %v", took), syntax.Number
	case st.code != 0:
		return fmt.Sprintf("exited with code %d after %v", st.code, took), syntax.Number
	}
	return fmt.Sprintf("finished in %v", took), syntax.String
}

// header is the output's first line: the command, where it ran, and how it
// is going.
func (st *compileState) header(home string) (string, []syntax.Span) {
	var spans []syntax.Span
	line := " "
	at := func(s string, c syntax.Class) {
		start := utf8.RuneCountInString(line)
		line += s
		spans = append(spans, syntax.Span{Start: start, End: start + utf8.RuneCountInString(s), Class: c})
	}
	at(st.command, syntax.Keyword)
	line += "  in  "
	dir := tildePath(st.dir, home) + string(filepath.Separator)
	if st.dir == home {
		dir = "~" + string(filepath.Separator)
	}
	at(dir, syntax.Function)
	line += "  "
	status, class := st.status()
	at(status, class)
	return line, spans
}

// lineSpans is the output's colouring for line.
func (st *compileState) lineSpans(line int) []syntax.Span { return st.spans[line] }

// fileType is how the modeline names the output, with how the command is
// going.
func (st *compileState) fileType() string {
	kind := "compile"
	if st.async {
		kind = "shell"
	}
	switch {
	case st.running:
		return kind + " running"
	case st.killed:
		return kind + " killed"
	case st.code != 0:
		return fmt.Sprintf("%s exit %d", kind, st.code)
	}
	return kind
}

// stopProcesses stops every command still running, for when nem exits: a
// build left running with nobody to read its output is only using the
// machine.
func (e *Editor) stopProcesses() {
	for _, st := range e.compile {
		if st.running {
			st.proc.kill()
		}
	}
}
