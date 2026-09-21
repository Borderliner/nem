// Package editor wires nem together and owns the event loop.
//
// It is the only package that knows about every other one: it implements
// command.Env so commands can act, drives keymap lookup over decoded tcell
// events, arranges windows through view, and draws through ui. Nothing imports
// it, which is what lets every other package stay testable without a terminal.
//
// Two responsibilities live here and nowhere else, both because putting them
// anywhere else means someone eventually forgets one:
//
//   - Cross-command bookkeeping at dispatch. Breaking the kill run, resetting
//     the goal column and recording the last command name all happen in one
//     place, so no command has to remember them. See dispatch.
//   - The minibuffer. A prompt is a real text.Buffer in a real view.Window, so
//     C-a, C-e, C-k and the kill ring work inside prompts with no extra code.
//     See minibuffer.go.
package editor

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/highlight"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/lua"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// A drifting Env breaks the build here, loudly, rather than in whichever
// command happens to be compiled first.
var _ command.Env = (*Editor)(nil)

// ErrTooDeep reports that minibuffer recursion hit its limit. It exists so a
// runaway Lua hook that prompts from inside a prompt fails cleanly instead of
// growing the Go stack until the process dies.
var ErrTooDeep = errors.New("minibuffer recursion too deep")

// maxMiniDepth caps nested prompts. Emacs allows recursive minibuffers and so
// does nem; eight is far past any legitimate depth and well short of trouble.
const maxMiniDepth = 8

// Editor is one editing session: the buffers, the windows onto them, and the
// machinery that turns keystrokes into commands.
//
// It is not safe for concurrent use, deliberately. Everything — command
// dispatch, keymap mutation and Lua execution — runs on the goroutine that
// polls for events, which is why keymap.Map and command.KillRing need no locks.
type Editor struct {
	reg  *command.Registry
	keys *keymap.Map

	// buffers is most-recently-visited first, which is the order C-x b offers.
	// names and byName are two directions of the same mapping: display names
	// are an editor concept, because a text.Buffer has only a path.
	buffers []*text.Buffer
	names   map[*text.Buffer]string
	byName  map[string]*text.Buffer

	tree   *view.Tree
	active *view.Window

	ring    *command.KillRing
	seq     command.Seq
	lastCmd string

	echo string

	scr tcell.Screen
	th  ui.Theme

	// arg collects the universal argument between C-u and the command it
	// modifies.
	arg argState

	// pending holds the keys of a partially typed sequence, so C-x waits for
	// its second key.
	pending []keymap.Key

	// wk is prefix-key discovery: the delay, and the panel while it shows. See
	// whichkey.go; it describes whatever pending holds.
	wk whichKeyState

	// hl caches syntax state per buffer, so a keystroke re-lexes from the edit
	// rather than from the top of the file. See highlight.go.
	hl map[*text.Buffer]*highlight.Cache

	// vcs caches each buffer's git branch, because the modeline asks for it on
	// every frame and the answer costs a walk up the directory tree. See vcs.go.
	vcs map[*text.Buffer]branchEntry

	// startup shows the welcome panel. It is set by the caller when nem was
	// started with no file to open, and cleared by the first keystroke - see
	// dismissStartup. Nothing sets it again, which is what makes the panel a
	// once-per-session thing rather than something that reappears whenever the
	// scratch buffer happens to be empty.
	startup bool

	// comp holds completion preferences from the config. They are applied when a
	// panel is built rather than when a session starts, so a reload takes effect
	// on the next frame instead of needing an open prompt to be rebuilt.
	comp completionPrefs

	// mini is the innermost active prompt, or nil when none is. miniDepth
	// counts nesting for the recursion guard.
	mini      *miniState
	miniDepth int

	// miniTransparent makes Win report the text window even while a prompt is
	// open. Only withTextWindow sets it, for search-as-you-type; see the note
	// there.
	miniTransparent bool

	// childDispatched reports that a nested dispatch already did the
	// cross-command bookkeeping, so an outer one must not repeat it. See
	// dispatch.
	childDispatched bool

	// safe holds the data-safety state: the backup store, which files have been
	// backed up this session, the autosave clock, and what each file looked like
	// on disk when nem last touched it. See safety.go.
	safe safety

	// clip mirrors kills to the system clipboard over OSC 52. See clipboard.go.
	clip clipboard

	before map[string][]func()
	after  map[string][]func()

	// host is the Lua interpreter, nil until LoadConfig runs.
	host *lua.Host

	quit bool
}

// New returns an editor with every built-in command registered, the default
// bindings installed, and a single window on an empty *scratch* buffer.
//
// scr may be nil for tests that drive dispatch without drawing; Run requires a
// real screen.
func New(scr tcell.Screen) (*Editor, error) {
	reg, err := command.NewDefaultRegistry()
	if err != nil {
		return nil, fmt.Errorf("registering commands: %w", err)
	}
	km := keymap.New()
	if err := InstallDefaultBindings(km); err != nil {
		return nil, fmt.Errorf("installing bindings: %w", err)
	}

	th := ui.DefaultTheme()
	e := &Editor{
		reg:    reg,
		keys:   km,
		names:  map[*text.Buffer]string{},
		byName: map[string]*text.Buffer{},
		ring:   command.NewKillRing(command.DefaultCapacity),
		th:     th,
		scr:    scr,
		safe:   newSafety(),
		comp:   defaultCompletionPrefs(),
		hl:     map[*text.Buffer]*highlight.Cache{},
		vcs:    map[*text.Buffer]branchEntry{},
		before: map[string][]func(){},
		after:  map[string][]func(){},
	}

	// recover-file closes over the editor rather than going through Env. It is
	// the only command that needs editor-specific state (the autosave store), and
	// adding a 31st Env method to serve one command would widen an interface that
	// sixty other commands share.
	if err := reg.Register(command.Command{
		Name:        "recover-file",
		Doc:         "Replace this buffer with its autosaved contents.",
		Interactive: true,
		Fn:          func(command.Env) error { return e.RecoverFile(e.Buf()) },
	}); err != nil {
		return nil, fmt.Errorf("registering recover-file: %w", err)
	}

	if err := registerDisplayCommands(e, reg); err != nil {
		return nil, fmt.Errorf("registering display commands: %w", err)
	}

	scratch := e.NewBuffer(ui.ScratchName)
	e.active = view.NewWindow(scratch)
	e.tree = view.NewTree(e.active)
	return e, nil
}

// Registry exposes the command table so the Lua layer can register commands
// into the same table the built-ins live in.
func (e *Editor) Registry() *command.Registry { return e.reg }

// Keymap exposes the global keymap so the Lua layer can rebind keys. Mutating
// it is safe only from the input goroutine, which is where config loading runs.
func (e *Editor) Keymap() *keymap.Map { return e.keys }

// --- the active view -----------------------------------------------------

// Win returns the minibuffer's window while a prompt is active, and the active
// text window otherwise.
//
// This one substitution is what makes editing commands work inside a prompt:
// C-a, C-e, C-k and yank all act on whatever Win reports, so a prompt needs no
// parallel implementation of any of them.
func (e *Editor) Win() *view.Window {
	if e.mini != nil && !e.miniTransparent {
		return e.mini.win
	}
	return e.active
}

// Buf returns the active window's buffer.
func (e *Editor) Buf() *text.Buffer { return e.Win().Buf }

// TextHeight reports the rows of buffer text the active window shows. A prompt
// is a single row.
func (e *Editor) TextHeight() int {
	if e.mini != nil && !e.miniTransparent {
		return 1
	}
	if e.scr == nil {
		return 24
	}
	w, h := e.scr.Size()
	rect, ok := e.tree.Layout(w, h-1)[e.active]
	if !ok {
		return 1
	}
	if th := view.TextHeight(rect); th > 0 {
		return th
	}
	return 1
}

// --- the universal argument ----------------------------------------------

// Arg reports the prefix argument for the command being dispatched.
func (e *Editor) Arg() (int, bool) { return e.arg.value() }

// --- the kill ring -------------------------------------------------------

// KillForward and KillBackward push onto the ring and mirror the resulting
// entry to the system clipboard, so C-w and M-w reach other applications. See
// noteKill for why the whole accumulated entry is sent rather than the fragment.
func (e *Editor) KillForward(s string)  { e.noteKill(s, false) }
func (e *Editor) KillBackward(s string) { e.noteKill(s, true) }
func (e *Editor) Yank() (string, error) { return e.ring.Yank() }

func (e *Editor) YankPop() (string, error) { return e.ring.YankPop() }

// Ring exposes the kill ring for tests and for the Lua layer.
func (e *Editor) Ring() *command.KillRing { return e.ring }

// --- command sequencing --------------------------------------------------

func (e *Editor) LastCommand() string { return e.lastCmd }
func (e *Editor) Seq() *command.Seq   { return &e.seq }

// --- the echo area -------------------------------------------------------

// Echo shows a message on the bottom row.
func (e *Editor) Echo(format string, a ...any) {
	e.echo = fmt.Sprintf(format, a...)
}

// Message returns the current echo-area text, for tests.
func (e *Editor) Message() string { return e.echo }

// --- buffers -------------------------------------------------------------

// Buffers lists live buffers, most recently visited first.
func (e *Editor) Buffers() []*text.Buffer {
	out := make([]*text.Buffer, len(e.buffers))
	copy(out, e.buffers)
	return out
}

// BufferName returns b's display name.
func (e *Editor) BufferName(b *text.Buffer) string { return e.names[b] }

// BufferByName finds a buffer by display name.
func (e *Editor) BufferByName(name string) (*text.Buffer, bool) {
	b, ok := e.byName[name]
	return b, ok
}

// NewBuffer creates a file-less buffer under name, or returns the existing one
// if that name is taken — so list-buffers reuses its buffer instead of piling
// up a new one per invocation.
func (e *Editor) NewBuffer(name string) *text.Buffer {
	if b, ok := e.byName[name]; ok {
		return b
	}
	b := text.NewBuffer()
	e.adopt(b, name)
	return b
}

// OpenFile returns the buffer visiting path, reading it if it is not open yet.
// A path that does not exist yields an empty buffer carrying it, which is how
// find-file creates a new file.
func (e *Editor) OpenFile(path string) (*text.Buffer, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, b := range e.buffers {
		if b.Path() == abs {
			e.touch(b)
			return b, nil
		}
	}
	b, err := text.LoadFile(abs)
	if err != nil {
		return nil, err
	}
	b.SetPath(abs)
	e.adopt(b, e.uniqueName(filepath.Base(abs)))

	// Remember what the file looked like, so a later save can tell whether
	// anything else has touched it.
	e.noteOnDisk(abs)

	// An autosave newer than the file means a previous session died with unsaved
	// work. Say so plainly and name the command that gets it back — a message
	// the user cannot act on is no better than silence.
	if newer, at := e.AutosaveAvailable(abs); newer {
		e.Echo("Autosave from %s is newer than %s — M-x recover-file to restore it",
			at.Format("15:04"), filepath.Base(abs))
	}
	return b, nil
}

// KillBuffer removes b from the live list, refusing to remove the last one.
//
// Any window showing b is moved to another buffer first: leaving a window
// pointing at a dead buffer would be a nil-buffer panic on the next redraw.
func (e *Editor) KillBuffer(b *text.Buffer) error {
	if len(e.buffers) <= 1 {
		return errors.New("cannot kill the last buffer")
	}
	var repl *text.Buffer
	for _, c := range e.buffers {
		if c != b {
			repl = c
			break
		}
	}
	for _, w := range e.tree.Windows() {
		if w.Buf == b {
			w.Visit(repl)
		}
	}
	delete(e.byName, e.names[b])
	delete(e.names, b)
	e.forgetHighlight(b)
	e.forgetBranch(b)
	for i, c := range e.buffers {
		if c == b {
			e.buffers = append(e.buffers[:i], e.buffers[i+1:]...)
			break
		}
	}
	return nil
}

// SaveBuffer writes b to disk. An empty path saves to b's own path; a non-empty
// path saves there and adopts it.
//
// On failure the buffer must come out exactly as it went in: still modified,
// and still pointing at its old path. text.Buffer.SaveAs assigns the path
// before attempting the write, so a failed write would otherwise leave the
// buffer claiming a file it was never written to — and the user would then
// believe a later successful C-x C-s had saved somewhere it had not.
// Two data-safety steps happen here, in this order and for these reasons. A
// file something else has changed is not overwritten without asking, because
// silently discarding an external edit is the worst thing this function could
// do. And the file's previous contents are copied to the backup store before the
// write, never after — a backup taken afterwards holds the new contents and
// preserves nothing. See safety.go.
func (e *Editor) SaveBuffer(b *text.Buffer, path string) error {
	target := path
	if target == "" {
		target = b.Path()
	}
	if target == "" {
		return text.ErrNoPath
	}

	if !e.confirmOverwrite(target) {
		// Nothing written, nothing marked clean: the buffer keeps its changes
		// and the file on disk keeps whatever the other writer put there.
		e.Echo("%s left unchanged on disk", filepath.Base(target))
		return nil
	}

	// A failed backup must not stop the save. The user asked to preserve their
	// work; refusing because a copy could not be filed elsewhere would lose more
	// than it protects.
	e.backupBeforeWrite(target)

	if path == "" {
		if err := b.Save(); err != nil {
			return err
		}
		e.afterSave(target)
		return nil
	}

	// No path rollback here: text.SaveAs adopts the new path only after the
	// write succeeds, so a failure leaves the buffer untouched.
	if err := b.SaveAs(path); err != nil {
		return err
	}
	if name := e.uniqueNameFor(b, filepath.Base(path)); name != "" {
		delete(e.byName, e.names[b])
		e.names[b] = name
		e.byName[name] = b
	}
	// The buffer just became a .go file, or a .lua one. Lex it as what it is
	// now rather than as what it was when it had no name.
	e.retuneHighlight(b)
	// The old reading described a different file, and possibly a different
	// repository: saving into one moves the buffer onto its branch.
	e.forgetBranch(b)
	e.afterSave(target)
	return nil
}

// adopt registers b as a live buffer under name, most recently visited first.
func (e *Editor) adopt(b *text.Buffer, name string) {
	e.buffers = append([]*text.Buffer{b}, e.buffers...)
	e.names[b] = name
	e.byName[name] = b
}

// touch moves b to the front of the visited order.
func (e *Editor) touch(b *text.Buffer) {
	for i, c := range e.buffers {
		if c == b {
			e.buffers = append([]*text.Buffer{b}, append(e.buffers[:i:i], e.buffers[i+1:]...)...)
			return
		}
	}
}

// uniqueName disambiguates a display name the way emacs does, so two files
// with the same basename in different directories remain distinguishable.
func (e *Editor) uniqueName(base string) string {
	if base == "" {
		base = ui.ScratchName
	}
	if _, taken := e.byName[base]; !taken {
		return base
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s<%d>", base, n)
		if _, taken := e.byName[cand]; !taken {
			return cand
		}
	}
}

// uniqueNameFor is uniqueName, except that b keeping its current name is not a
// collision with itself. It returns "" when no rename is needed.
func (e *Editor) uniqueNameFor(b *text.Buffer, base string) string {
	if cur, ok := e.names[b]; ok && cur == base {
		return ""
	}
	if holder, taken := e.byName[base]; taken && holder == b {
		return ""
	}
	return e.uniqueName(base)
}

// --- windows -------------------------------------------------------------

// SplitWindow splits the active window and selects the new half.
func (e *Editor) SplitWindow(vertical bool) error {
	w, err := e.tree.Split(e.active, vertical)
	if err != nil {
		return err
	}
	e.active = w
	return nil
}

// OtherWindow moves the selection n windows along the cycle, wrapping.
func (e *Editor) OtherWindow(n int) {
	ws := e.tree.Windows()
	if len(ws) < 2 {
		return
	}
	cur := 0
	for i, w := range ws {
		if w == e.active {
			cur = i
			break
		}
	}
	i := (cur + n) % len(ws)
	if i < 0 {
		i += len(ws)
	}
	e.active = ws[i]
}

// DeleteWindow removes the active window, refusing to remove the sole one.
func (e *Editor) DeleteWindow() error {
	if err := e.tree.Delete(e.active); err != nil {
		return err
	}
	e.active = e.tree.Windows()[0]
	return nil
}

// DeleteOtherWindows makes the active window fill the frame.
func (e *Editor) DeleteOtherWindows() { e.tree.DeleteOthers(e.active) }

// Tree exposes the window tree for tests.
func (e *Editor) Tree() *view.Tree { return e.tree }

// Active returns the selected window.
func (e *Editor) Active() *view.Window { return e.active }

// --- commands and bindings -----------------------------------------------

// Run invokes another command by name. This is M-x's mechanism and Lua's
// nem.run.
//
// It routes through dispatch so the invoked command gets the same bookkeeping
// as a keystroke would — otherwise M-x kill-line would leave the kill run open
// and fuse with the next kill.
func (e *Editor) Run(name string) error {
	if _, ok := e.reg.Lookup(name); !ok {
		return fmt.Errorf("%w: %q", command.ErrUnknownCommand, name)
	}
	return e.dispatch(name)
}

// CommandNames lists interactive command names, sorted, for M-x completion.
func (e *Editor) CommandNames() []string { return e.reg.Names() }

// Bindings maps key sequences to command names, for describe-bindings.
func (e *Editor) Bindings() map[string]string { return e.keys.Bindings() }

// Where lists the sequences bound to a command, for describe-key.
func (e *Editor) Where(cmd string) []string { return e.keys.Where(cmd) }

// --- session -------------------------------------------------------------

// Quit ends the session, refusing without force while any buffer is modified.
func (e *Editor) Quit(force bool) error {
	if !force {
		var dirty []string
		for _, b := range e.buffers {
			if b.Modified() {
				dirty = append(dirty, e.names[b])
			}
		}
		if len(dirty) > 0 {
			sort.Strings(dirty)
			return fmt.Errorf("unsaved changes in %v", dirty)
		}
	}
	e.quit = true
	return nil
}

// Quitting reports whether the session has been asked to end, for tests.
func (e *Editor) Quitting() bool { return e.quit }

// ShowStartup asks for the welcome panel on the next frame.
//
// The caller decides rather than the editor, because only the caller knows
// whether a file was named on the command line - the editor sees an empty
// *scratch* either way, and showing a welcome over a file someone asked for
// would be an interruption rather than a greeting.
func (e *Editor) ShowStartup() { e.startup = true }

// dismissStartup hides the welcome panel.
//
// It never swallows the keystroke that dismisses it: the key goes on to be
// resolved normally, so the panel costs nothing more than the glance it was
// there to be worth.
func (e *Editor) dismissStartup() { e.startup = false }
