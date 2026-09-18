package lua

import (
	"sort"
	"strings"

	glua "github.com/yuin/gopher-lua"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/keymap"
	"github.com/hajianpour/nem/text"
)

// installAPI builds the nem table. This is the entire surface a script can
// reach: there is no path from here to the screen, the filesystem, or a raw
// command.Env, which is what lets the internals move without breaking scripts.
func (h *Host) installAPI() {
	nem := h.l.NewTable()
	nem.RawSetString("api_version", glua.LNumber(APIVersion))

	for name, fn := range map[string]glua.LGFunction{
		"set":     h.apiSet,
		"bind":    h.apiBind,
		"command": h.apiCommand,
		"run":     h.apiRun,
		"hook":    h.apiHook,
	} {
		nem.RawSetString(name, h.l.NewFunction(fn))
	}

	buf := h.l.NewTable()
	for name, fn := range map[string]glua.LGFunction{
		// Where point is.
		"line":         h.bufLine,
		"replace_line": h.bufReplaceLine,
		"point":        h.bufPoint,
		"set_point":    h.bufSetPoint,
		// Addressed by line number, all 1-based. See buffer.go.
		"line_count":  h.bufLineCount,
		"get_line":    h.bufGetLine,
		"set_line":    h.bufSetLine,
		"insert_line": h.bufInsertLine,
		"remove_line": h.bufRemoveLine,
		// The whole buffer.
		"text":     h.bufText,
		"set_text": h.bufSetText,
		// About the buffer.
		"path":     h.bufPath,
		"modified": h.bufModified,
	} {
		buf.RawSetString(name, h.l.NewFunction(fn))
	}
	nem.RawSetString("buf", buf)

	h.l.SetGlobal("nem", nem)
}

// --- settings ---------------------------------------------------------------

func (h *Host) apiSet(L *glua.LState) int {
	key := L.CheckString(1)
	if L.GetTop() < 2 {
		L.RaiseError("nem.set(%q): missing value", key)
	}
	if err := h.settings.set(key, L.Get(2)); err != nil {
		L.RaiseError("nem.set: %v", err)
	}
	return 0
}

// --- bindings ---------------------------------------------------------------

// bindRecord remembers a binding a script made, so the editor can report any
// that name a command nothing registered.
type bindRecord struct {
	Spec    string
	Command string
}

func (h *Host) apiBind(L *glua.LState) int {
	spec := L.CheckString(1)
	cmd := L.CheckString(2)
	if cmd == "" {
		L.RaiseError("nem.bind(%q): command name must not be empty", spec)
	}

	seq, err := keymap.ParseSpec(spec)
	if err != nil {
		L.RaiseError("nem.bind(%q): %v", spec, err)
	}
	// No normalization here on purpose: keymap.Map normalizes on both Bind and
	// Lookup, so it owns the guarantee that C-SPC (which reaches the editor as
	// NUL) and C-/ (which reaches it as C-_) resolve to the key the user
	// actually pressed. Normalizing again here would be a second source of
	// truth for the same rule. The end-to-end behaviour is pinned by
	// TestBoundKeysMatchWhatTheDecoderProduces.
	if err := h.keys.Bind(seq, cmd); err != nil {
		L.RaiseError("nem.bind(%q): %v", spec, err)
	}
	h.binds = append(h.binds, bindRecord{Spec: spec, Command: cmd})
	return 0
}

// UnresolvedBindings lists bindings whose command is not registered, as
// "spec -> command" strings, sorted.
//
// Binding a command that does not exist yet is not rejected at bind time: a
// script may legitimately bind a key before defining the command, and emacs
// allows it too. But a key bound to nothing is a dead key, so the editor calls
// this after loading the config and reports whatever is left dangling. Silence
// would be the worst outcome — the user presses the key, nothing happens, and
// the config looks correct.
func (h *Host) UnresolvedBindings() []string {
	var out []string
	for _, b := range h.binds {
		if _, ok := h.reg.Lookup(b.Command); !ok {
			out = append(out, b.Spec+" -> "+b.Command)
		}
	}
	sort.Strings(out)
	return out
}

// --- commands ---------------------------------------------------------------

func (h *Host) apiCommand(L *glua.LState) int {
	name := L.CheckString(1)
	doc := L.CheckString(2)
	fn := L.CheckFunction(3)
	if strings.TrimSpace(doc) == "" {
		L.RaiseError("nem.command(%q): a doc string is required; M-x and describe-key show it", name)
	}

	// The Lua function becomes an ordinary command.Command, so it lands in the
	// same registry as the built-ins and is indistinguishable from one: M-x
	// completes it, a key can be bound to it, describe-key reports its doc.
	err := h.reg.Register(command.Command{
		Name:        name,
		Doc:         doc,
		Interactive: true,
		Fn: func(e command.Env) error {
			var rerr error
			h.withEnv(e, func() { rerr = h.call(fn) })
			return rerr
		},
	})
	if err != nil {
		L.RaiseError("nem.command: %v", err)
	}
	return 0
}

func (h *Host) apiRun(L *glua.LState) int {
	name := L.CheckString(1)
	e := h.mustEnv(L)
	if err := e.Run(name); err != nil {
		L.RaiseError("nem.run(%q): %v", name, err)
	}
	return 0
}

// --- hooks ------------------------------------------------------------------

func (h *Host) apiHook(L *glua.LState) int {
	name := L.CheckString(1)
	fn := L.CheckFunction(2)
	if !hookNamePattern.MatchString(name) {
		L.RaiseError("nem.hook(%q): a hook name must look like before-<something> or after-<something>", name)
	}
	h.hooks[name] = append(h.hooks[name], fn)
	return 0
}

// --- buffer access ----------------------------------------------------------

// mustEnv returns the active editor context, raising a Lua error when a script
// reaches for editor state at load time, where there is no buffer to act on.
func (h *Host) mustEnv(L *glua.LState) command.Env {
	if h.env == nil {
		L.RaiseError("%s", ErrNoEnv.Error())
	}
	return h.env
}

// curLine returns the buffer and the index of the line point is on.
func (h *Host) curLine(L *glua.LState) (*text.Buffer, int) {
	e := h.mustEnv(L)
	b, n := e.Buf(), e.Win().Pt.Line
	if n < 0 || n >= b.NumLines() {
		L.RaiseError("no current line")
	}
	return b, n
}

func (h *Host) bufLine(L *glua.LState) int {
	b, n := h.curLine(L)
	L.Push(glua.LString(b.Line(n).String()))
	return 1
}

func (h *Host) bufReplaceLine(L *glua.LState) int {
	s := L.CheckString(1)
	checkNoNewline(L, s, "nem.buf.replace_line")
	b, n := h.curLine(L)
	if err := replaceLineContent(b, n, s); err != nil {
		L.RaiseError("nem.buf.replace_line: %v", err)
	}
	// The old point may now sit past the end of a shorter line.
	h.clampPoint(b)
	return 0
}

// bufPoint returns line and column, both 1-based, matching what the modeline
// shows rather than the internal 0-based indices.
func (h *Host) bufPoint(L *glua.LState) int {
	e := h.mustEnv(L)
	p := e.Win().Pt
	L.Push(glua.LNumber(p.Line + 1))
	L.Push(glua.LNumber(int(p.Col) + 1))
	return 2
}

func (h *Host) bufSetPoint(L *glua.LState) int {
	line := L.CheckInt(1)
	col := L.CheckInt(2)
	e := h.mustEnv(L)
	w := e.Win()
	w.Pt = e.Buf().ClampPos(text.Pos{Line: line - 1, Col: text.RuneIdx(col - 1)})
	return 0
}

func (h *Host) bufPath(L *glua.LState) int {
	L.Push(glua.LString(h.mustEnv(L).Buf().Path()))
	return 1
}

func (h *Host) bufModified(L *glua.LState) int {
	L.Push(glua.LBool(h.mustEnv(L).Buf().Modified()))
	return 1
}

func (h *Host) bufText(L *glua.LState) int {
	L.Push(glua.LString(h.mustEnv(L).Buf().String()))
	return 1
}
