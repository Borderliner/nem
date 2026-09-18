// Package commandtest provides a headless implementation of command.Env.
//
// It exists so that editor commands can be tested with no terminal, no screen
// and no event loop: a test constructs a Fake with some buffer text, runs a
// command against it, and asserts on the resulting text, point, kill ring and
// recorded prompts. That is only possible because command.Env is a narrow
// interface that cannot reach the screen or the layout tree.
//
// The buffer, window and kill ring inside a Fake are the real types, not stubs,
// so editing and kill-run accumulation behave exactly as they will in the
// editor. Only the things a terminal would supply — prompt answers, the window
// height, the file system — are canned.
package commandtest

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/keymap"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// Quit placed in Replies makes that prompt return command.ErrQuit, which is
// how a test exercises a command's C-g path.
const Quit = "\x00commandtest.quit"

// QuitChar placed in Chars makes that ReadChar return command.ErrQuit.
const QuitChar = rune(0)

var (
	// ErrNoReply reports that a command prompted more times than the test
	// supplied answers for. It is a bug in the test, not in the command, so it
	// is deliberately distinct from command.ErrQuit.
	ErrNoReply = errors.New("commandtest: prompt with no canned reply left")

	// ErrLastBuffer reports an attempt to kill the only live buffer.
	ErrLastBuffer = errors.New("commandtest: cannot kill the last buffer")
)

// Fake is an in-memory command.Env.
//
// Fields fall into two groups: knobs a test sets before running a command, and
// recorders a test reads afterwards.
type Fake struct {
	// --- knobs ---

	// ArgN and ArgExplicit are what Arg reports. New sets ArgN to 1.
	ArgN        int
	ArgExplicit bool

	// Height is what TextHeight reports. New sets it to 24.
	Height int

	// Replies are consumed in order by ReadString. The Quit sentinel makes a
	// prompt report command.ErrQuit.
	Replies []string

	// Chars are consumed in order by ReadChar. QuitChar reports ErrQuit.
	Chars []rune

	// Keys are consumed in order by ReadKey.
	Keys []keymap.Key

	// Files seeds OpenFile: opening a path returns a buffer holding
	// Files[path], or an empty buffer when the path is absent, as find-file
	// does for a new file.
	Files map[string]string

	// BindingMap is what Bindings reports and what Where searches.
	BindingMap map[string]string

	// Reg backs Run and CommandNames. New creates an empty registry.
	Reg *command.Registry

	// --- recorders ---

	// Echoes holds every formatted message passed to Echo.
	Echoes []string

	// Prompts, CharPrompts and KeyPrompts hold the prompt strings each read
	// method was called with, in order.
	Prompts     []string
	CharPrompts []string
	KeyPrompts  []string

	// Splits records the vertical flag of each SplitWindow call.
	Splits []bool

	// OtherWindowArgs records the argument of each OtherWindow call.
	OtherWindowArgs []int

	// DeleteWindowCalls and DeleteOtherWindowsCalls count those calls.
	DeleteWindowCalls       int
	DeleteOtherWindowsCalls int

	// QuitArgs records the force flag of each Quit call.
	QuitArgs []bool

	// RunNames records every command name passed to Run.
	RunNames []string

	// SplitErr, DeleteWindowErr, QuitErr and OpenFileErr, when non-nil, are
	// returned by those methods so a test can exercise failure paths.
	SplitErr        error
	DeleteWindowErr error
	QuitErr         error
	OpenFileErr     error

	// --- internals ---

	win     *view.Window
	ring    *command.KillRing
	buffers []*text.Buffer
	names   map[*text.Buffer]string
	lastCmd string
	seq     command.Seq
}

// Compile-time proof that Fake satisfies the whole interface. If Env grows a
// method, this breaks here rather than in sixty command tests.
var _ command.Env = (*Fake)(nil)

// New returns a Fake whose active buffer holds the given lines, with point at
// the start and no mark set.
func New(lines ...string) *Fake {
	b := text.NewBuffer()
	if len(lines) > 0 {
		s := strings.Join(lines, "\n")
		if err := b.Insert(text.Pos{}, []rune(s)); err != nil {
			panic(fmt.Sprintf("commandtest.New: seeding buffer: %v", err))
		}
		b.BreakUndo()
		b.SetModified(false)
	}
	f := &Fake{
		ArgN:       1,
		Height:     24,
		Files:      make(map[string]string),
		BindingMap: make(map[string]string),
		Reg:        command.NewRegistry(),
		win:        view.NewWindow(b),
		ring:       command.NewKillRing(command.DefaultCapacity),
		buffers:    []*text.Buffer{b},
		names:      map[*text.Buffer]string{b: "*scratch*"},
	}
	return f
}

// --- accessors for assertions ---

// Text returns the active buffer's entire contents.
func (f *Fake) Text() string { return f.win.Buf.String() }

// Point returns the active window's point.
func (f *Fake) Point() text.Pos { return f.win.Pt }

// SetPoint moves point, clamped into the buffer.
func (f *Fake) SetPoint(p text.Pos) { f.win.Pt = f.win.Buf.ClampPos(p) }

// Ring exposes the kill ring so a test can assert on it directly.
func (f *Fake) Ring() *command.KillRing { return f.ring }

// SetLastCommand sets what LastCommand reports, for testing commands whose
// behaviour depends on repetition such as yank-pop and recenter-top-bottom.
func (f *Fake) SetLastCommand(name string) { f.lastCmd = name }

// AddBuffer registers an extra live buffer under a display name.
func (f *Fake) AddBuffer(name string, lines ...string) *text.Buffer {
	b := text.NewBuffer()
	if len(lines) > 0 {
		if err := b.Insert(text.Pos{}, []rune(strings.Join(lines, "\n"))); err != nil {
			panic(fmt.Sprintf("commandtest.AddBuffer: seeding buffer: %v", err))
		}
		b.BreakUndo()
		b.SetModified(false)
	}
	f.buffers = append(f.buffers, b)
	f.names[b] = name
	return b
}

// --- command.Env: the active view ---

func (f *Fake) Win() *view.Window { return f.win }
func (f *Fake) Buf() *text.Buffer { return f.win.Buf }
func (f *Fake) TextHeight() int   { return f.Height }

// --- command.Env: the universal argument ---

func (f *Fake) Arg() (int, bool) { return f.ArgN, f.ArgExplicit }

// --- command.Env: the kill ring ---

func (f *Fake) KillForward(s string)     { f.ring.KillForward(s) }
func (f *Fake) KillBackward(s string)    { f.ring.KillBackward(s) }
func (f *Fake) Yank() (string, error)    { return f.ring.Yank() }
func (f *Fake) YankPop() (string, error) { return f.ring.YankPop() }

// --- command.Env: command sequencing ---

func (f *Fake) LastCommand() string { return f.lastCmd }

// Seq returns the fake's sequencing state. The pointer is stable across calls
// and across dispatches, which is the whole point: a fake handing out a fresh
// Seq each time would make every yank-pop test pass for the wrong reason.
func (f *Fake) Seq() *command.Seq { return &f.seq }

// --- command.Env: the minibuffer ---

// ReadString records the prompt and returns the next canned reply.
//
// When opts.OnChange is set it is called once per successive prefix of the
// reply — "f", "fo", "foo" — rather than once with the final value, so an
// incremental-search command is exercised the way real typing would drive it.
func (f *Fake) ReadString(opts command.ReadOpts) (string, error) {
	f.Prompts = append(f.Prompts, opts.Prompt)
	if len(f.Replies) == 0 {
		return "", fmt.Errorf("%w: %q", ErrNoReply, opts.Prompt)
	}
	reply := f.Replies[0]
	f.Replies = f.Replies[1:]

	if reply == Quit {
		if opts.OnChange != nil {
			opts.OnChange(opts.Initial)
		}
		return "", command.ErrQuit
	}

	if opts.OnChange != nil {
		runes := []rune(reply)
		for i := range runes {
			opts.OnChange(string(runes[:i+1]))
		}
		if len(runes) == 0 {
			opts.OnChange("")
		}
	}
	return reply, nil
}

// ReadChar records the prompt and returns the next canned rune, rejecting one
// that is not in valid so a test cannot accidentally assert on an answer the
// real prompt would have refused.
func (f *Fake) ReadChar(prompt string, valid []rune) (rune, error) {
	f.CharPrompts = append(f.CharPrompts, prompt)
	if len(f.Chars) == 0 {
		return 0, fmt.Errorf("%w: %q", ErrNoReply, prompt)
	}
	c := f.Chars[0]
	f.Chars = f.Chars[1:]
	if c == QuitChar {
		return 0, command.ErrQuit
	}
	if len(valid) > 0 {
		ok := false
		for _, v := range valid {
			if v == c {
				ok = true
				break
			}
		}
		if !ok {
			return 0, fmt.Errorf("commandtest: canned char %q is not among the valid answers %q", c, string(valid))
		}
	}
	return c, nil
}

func (f *Fake) ReadKey(prompt string) (keymap.Key, error) {
	f.KeyPrompts = append(f.KeyPrompts, prompt)
	if len(f.Keys) == 0 {
		return keymap.Key{}, fmt.Errorf("%w: %q", ErrNoReply, prompt)
	}
	k := f.Keys[0]
	f.Keys = f.Keys[1:]
	return k, nil
}

func (f *Fake) Echo(format string, a ...any) {
	f.Echoes = append(f.Echoes, fmt.Sprintf(format, a...))
}

// --- command.Env: buffers ---

func (f *Fake) Buffers() []*text.Buffer {
	out := make([]*text.Buffer, len(f.buffers))
	copy(out, f.buffers)
	return out
}

func (f *Fake) BufferName(b *text.Buffer) string {
	if n, ok := f.names[b]; ok {
		return n
	}
	return ""
}

func (f *Fake) BufferByName(name string) (*text.Buffer, bool) {
	for b, n := range f.names {
		if n == name {
			return b, true
		}
	}
	return nil, false
}

func (f *Fake) NewBuffer(name string) *text.Buffer {
	b := text.NewBuffer()
	f.buffers = append(f.buffers, b)
	f.names[b] = name
	return b
}

// OpenFile returns the buffer already visiting path, or creates one from the
// seeded Files content. Nothing touches the real file system.
func (f *Fake) OpenFile(path string) (*text.Buffer, error) {
	if f.OpenFileErr != nil {
		return nil, f.OpenFileErr
	}
	for _, b := range f.buffers {
		if b.Path() == path {
			return b, nil
		}
	}
	b := text.NewBuffer()
	if content, ok := f.Files[path]; ok && content != "" {
		if err := b.Insert(text.Pos{}, []rune(content)); err != nil {
			return nil, err
		}
		b.BreakUndo()
		b.SetModified(false)
	}
	b.SetPath(path)
	f.buffers = append(f.buffers, b)
	f.names[b] = basename(path)
	return b, nil
}

func (f *Fake) KillBuffer(b *text.Buffer) error {
	if len(f.buffers) <= 1 {
		return ErrLastBuffer
	}
	for i, have := range f.buffers {
		if have == b {
			f.buffers = append(f.buffers[:i], f.buffers[i+1:]...)
			delete(f.names, b)
			return nil
		}
	}
	return fmt.Errorf("commandtest: buffer not live")
}

// --- command.Env: windows ---

func (f *Fake) SplitWindow(vertical bool) error {
	f.Splits = append(f.Splits, vertical)
	return f.SplitErr
}

func (f *Fake) OtherWindow(n int) {
	f.OtherWindowArgs = append(f.OtherWindowArgs, n)
}

func (f *Fake) DeleteWindow() error {
	f.DeleteWindowCalls++
	return f.DeleteWindowErr
}

func (f *Fake) DeleteOtherWindows() { f.DeleteOtherWindowsCalls++ }

// --- command.Env: commands and bindings ---

func (f *Fake) Run(name string) error {
	f.RunNames = append(f.RunNames, name)
	return f.Reg.Run(name, f)
}

func (f *Fake) CommandNames() []string { return f.Reg.Names() }

func (f *Fake) Bindings() map[string]string {
	out := make(map[string]string, len(f.BindingMap))
	for k, v := range f.BindingMap {
		out[k] = v
	}
	return out
}

func (f *Fake) Where(command string) []string {
	var out []string
	for spec, name := range f.BindingMap {
		if name == command {
			out = append(out, spec)
		}
	}
	sort.Strings(out)
	return out
}

// --- command.Env: session ---

func (f *Fake) Quit(force bool) error {
	f.QuitArgs = append(f.QuitArgs, force)
	return f.QuitErr
}

func basename(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
