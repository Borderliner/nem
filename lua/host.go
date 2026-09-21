// Package lua hosts nem's configuration script.
//
// The config file is a real Lua program, not a declarative format, so the same
// mechanism serves rebinding a key and defining a new command. A command
// defined in Lua registers into the same command.Registry as the built-ins and
// is indistinguishable from one at the call site: M-x finds it, a key can be
// bound to it, and describe-key reports it.
//
// # The error boundary
//
// A script error must never take the editor down, because a panic escaping into
// the event loop costs the user whatever they had not saved. Every entry into
// Lua — loading the config, invoking a Lua-defined command, firing a hook —
// goes through callProtected, which runs under gopher-lua's protected call.
// That converts both Lua errors and arbitrary Go panics into error values
// carrying a Lua traceback, and the editor continues with the built-in default
// for whatever failed.
//
// # Concurrency
//
// A Host is not safe for concurrent use and deliberately carries no locks. The
// editor runs all Lua on the input goroutine, which is the same goroutine that
// dispatches commands, so an LState is never touched from two places. Adding a
// mutex here would imply a guarantee the design does not make; if Lua ever
// needs to run elsewhere, the fix is to marshal it back onto the input
// goroutine rather than to lock the interpreter.
//
// # Capability boundary
//
// Scripts get the nem table and the pure-computation half of the Lua standard
// library: base, string, table and math. They do not get io, os, debug or
// package, and dofile and loadfile are removed from base. A script therefore
// cannot open a file, spawn a process, or load more Lua from disk, and cannot
// reach the screen or a raw command.Env under any name. Starting strict is
// deliberate: relaxing a capability boundary later is harmless, while tightening
// one breaks scripts people have already written.
package lua

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	glua "github.com/yuin/gopher-lua"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
)

// APIVersion is the version of the nem table exposed to scripts, readable as
// nem.api_version. It exists so that a script written against a future,
// incompatible surface can detect the mismatch and say so, rather than failing
// in pieces.
const APIVersion = 1

// ErrNoEnv reports that a script reached for editor state outside a command or
// hook, where there is no active buffer to act on.
var ErrNoEnv = errors.New("no editor context: nem.buf and nem.run are only available inside a command or hook")

// hookNamePattern constrains hook names to before-<something> or
// after-<something>. The host does not interpret the rest of the name — the
// editor decides which names it fires — but validating the shape turns a typo
// like "befor-save" into an error at load time instead of a hook that silently
// never runs, which is the single most confusing way for a config to be wrong.
var hookNamePattern = regexp.MustCompile(`^(before|after)-[a-z0-9]+(-[a-z0-9]+)*$`)

// Options configures a Host.
type Options struct {
	// Registry receives commands defined with nem.command. Required.
	Registry *command.Registry

	// Keymap receives bindings made with nem.bind. Required.
	Keymap *keymap.Map

	// ConfigPath is the script to load. Injectable so tests never touch a real
	// ~/.config.
	ConfigPath string

	// Timeout bounds a single entry into Lua. Zero means unlimited.
	//
	// It defaults to unlimited because a legitimate hook may be slow, and an
	// editor that abandons a user's formatting hook halfway is worse than one
	// that waits. Set it if you run scripts you do not trust: without it, a
	// `while true do end` in a config hangs the editor, which the protected
	// call cannot help with because a spin is not an error.
	Timeout time.Duration
}

// DefaultConfigPath returns ~/.config/nem/init.lua.
func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config dir: %w", err)
	}
	return dir + "/nem/init.lua", nil
}

// Host owns the Lua interpreter and the nem table.
type Host struct {
	l        *glua.LState
	reg      *command.Registry
	keys     *keymap.Map
	cfgPath  string
	timeout  time.Duration
	settings Settings

	// hooks maps a hook name to its callbacks, in registration order.
	hooks map[string][]*glua.LFunction

	// binds records what nem.bind bound, so UnresolvedBindings can report any
	// key left pointing at a command nothing registered.
	binds []bindRecord

	// depth counts nested entries into Lua, so that a Lua command calling
	// nem.run into another Lua command does not have the inner call cancel the
	// outer call's deadline.
	depth int

	// env is the editor context for the script currently running, and is nil
	// whenever no command or hook is executing. Scripts reach editor state only
	// through this, which is what keeps a raw Env out of Lua's hands.
	env command.Env
}

// New creates a Host with the nem table installed. It does not load the config;
// call LoadConfig for that, so a caller can distinguish a broken host from a
// broken script.
func New(opts Options) (*Host, error) {
	if opts.Registry == nil {
		return nil, errors.New("lua.New: Registry is required")
	}
	if opts.Keymap == nil {
		return nil, errors.New("lua.New: Keymap is required")
	}
	h := &Host{
		l:        glua.NewState(glua.Options{SkipOpenLibs: true}),
		reg:      opts.Registry,
		keys:     opts.Keymap,
		cfgPath:  opts.ConfigPath,
		timeout:  opts.Timeout,
		settings: DefaultSettings(),
		hooks:    map[string][]*glua.LFunction{},
	}
	if err := h.openLibs(); err != nil {
		h.Close()
		return nil, err
	}
	h.installAPI()
	return h, nil
}

// openLibs opens only the libraries a config legitimately needs, then removes
// the two base functions that can read the filesystem.
func (h *Host) openLibs() error {
	for _, lib := range []struct {
		name string
		fn   glua.LGFunction
	}{
		{glua.BaseLibName, glua.OpenBase},
		{glua.StringLibName, glua.OpenString},
		{glua.TabLibName, glua.OpenTable},
		{glua.MathLibName, glua.OpenMath},
	} {
		h.l.Push(h.l.NewFunction(lib.fn))
		h.l.Push(glua.LString(lib.name))
		if err := h.l.PCall(1, 0, nil); err != nil {
			return fmt.Errorf("opening lua library %q: %w", lib.name, err)
		}
	}
	// dofile, loadfile and require all read arbitrary files. require is
	// registered by the BASE library in gopher-lua, not by package, so opening
	// only base/string/table/math is not enough to keep it out — it has to be
	// removed explicitly. load and loadstring only compile strings the script
	// already holds, so they stay.
	for _, name := range []string{"dofile", "loadfile", "require"} {
		h.l.SetGlobal(name, glua.LNil)
	}
	return nil
}

// Close releases the interpreter.
func (h *Host) Close() {
	if h.l != nil {
		h.l.Close()
	}
}

// Settings returns the settings after any nem.set calls. The editor reads this
// once the config has loaded and applies the values itself.
func (h *Host) Settings() Settings { return h.settings }

// LoadConfig runs the config file.
//
// A missing file is not an error: running without a config is the normal case
// for a new user. A file that fails to parse or throws at top level returns an
// error carrying the Lua traceback, and whatever the script managed to do
// before failing stands — a config that binds ten keys and then has a typo on
// line eleven keeps its ten bindings.
func (h *Host) LoadConfig() error {
	if h.cfgPath == "" {
		return nil
	}
	if _, err := os.Stat(h.cfgPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return h.protect(func() error { return h.l.DoFile(h.cfgPath) })
}

// HookNames lists every hook a script registered, sorted.
func (h *Host) HookNames() []string {
	names := make([]string, 0, len(h.hooks))
	for name := range h.hooks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HookCount reports how many callbacks are registered under name.
func (h *Host) HookCount(name string) int { return len(h.hooks[name]) }

// FireHook runs every callback registered under name, passing a table
// describing b. The editor calls this around dispatch, keyed on command name;
// the host does not interpret hook names, so which names exist is the editor's
// decision rather than this package's.
//
// Every callback runs even if an earlier one fails, because one broken hook
// should not silently disable the others. The returned error joins whatever
// failed.
func (h *Host) FireHook(name string, e command.Env, b *text.Buffer) error {
	fns := h.hooks[name]
	if len(fns) == 0 {
		return nil
	}
	arg := h.bufferTable(e, b)
	var errs []error
	h.withEnv(e, func() {
		for i, fn := range fns {
			if err := h.call(fn, arg); err != nil {
				errs = append(errs, fmt.Errorf("hook %s[%d]: %w", name, i, err))
			}
		}
	})
	return errors.Join(errs...)
}

// bufferTable describes a buffer to a hook. path is a Lua string so that the
// documented idiom buf.path:match("%.go$") works.
func (h *Host) bufferTable(e command.Env, b *text.Buffer) glua.LValue {
	if b == nil {
		return glua.LNil
	}
	t := h.l.NewTable()
	t.RawSetString("path", glua.LString(b.Path()))
	t.RawSetString("modified", glua.LBool(b.Modified()))
	if e != nil {
		t.RawSetString("name", glua.LString(e.BufferName(b)))
	}
	return t
}

// withEnv makes e the active editor context for the duration of fn. It nests,
// so a Lua command that calls nem.run into another Lua command works.
func (h *Host) withEnv(e command.Env, fn func()) {
	prev := h.env
	h.env = e
	defer func() { h.env = prev }()
	fn()
}

// call invokes a Lua function under protection.
func (h *Host) call(fn *glua.LFunction, args ...glua.LValue) error {
	return h.protect(func() error {
		return h.l.CallByParam(glua.P{Fn: fn, NRet: 0, Protect: true}, args...)
	})
}

// protect runs one entry into Lua with the timeout applied, converts the result
// into a scriptError carrying any traceback, and recovers anything that somehow
// escaped the protected call.
//
// gopher-lua's protected call already converts arbitrary Go panics into errors,
// so the recover here is for the paths that do not go through it. Between the
// two, nothing from a script reaches the event loop as a panic.
func (h *Host) protect(fn func() error) (err error) {
	defer func() {
		if rcv := recover(); rcv != nil {
			err = fmt.Errorf("lua panic: %v", rcv)
		}
	}()

	// Only the outermost entry owns the deadline. A nested call installing its
	// own context would reset the clock, and removing it on the way out would
	// leave the outer call running unbounded.
	h.depth++
	defer func() { h.depth-- }()
	if h.timeout > 0 && h.depth == 1 {
		ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
		defer cancel()
		h.l.SetContext(ctx)
		defer h.l.RemoveContext()
	}
	return asScriptError(fn())
}
