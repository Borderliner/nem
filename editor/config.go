package editor

import (
	"errors"
	"time"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/lua"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui"
)

// configTimeout bounds a single entry into Lua.
//
// lua.Options defaults to unlimited, on the reasoning that abandoning a user's
// formatting hook halfway is worse than waiting for it. nem sets a bound anyway,
// because the failure that argument does not cover is a spin: `while true do end`
// in a config hangs the event loop forever, the protected call cannot help
// because a spin is not an error, and the user has no way to quit and no way to
// save. An editor that gives up on a hook after five seconds is recoverable; one
// that is frozen is not. Five seconds is far past any legitimate config or
// format-on-save hook.
const configTimeout = 5 * time.Second

// hookCommands maps a hook name to the commands whose dispatch fires it.
//
// lua.Host validates only the shape of a hook name — before-… or after-… — and
// leaves which names actually exist to the editor, so this table is the whole
// definition of nem's hook vocabulary. A name absent here is accepted by the
// host and then never fires, so adding a documented hook means adding it here.
var hookCommands = map[string][]string{
	"before-save": {"save-buffer", "write-file"},
	"after-save":  {"save-buffer", "write-file"},
}

// LoadConfig loads the Lua config at path, applies its settings, and wires its
// hooks into dispatch. An empty path uses the default location.
//
// A broken config never takes the editor down. A script error is reported in the
// echo area and the editor carries on with built-in defaults for whatever
// failed, because losing a session to a typo in init.lua is a far worse outcome
// than starting unconfigured. The returned error is informational: callers
// normally ignore it, having already seen it echoed.
func (e *Editor) LoadConfig(path string) error {
	if path == "" {
		p, err := lua.DefaultConfigPath()
		if err != nil {
			e.Echo("config: %v", err)
			return err
		}
		path = p
	}

	h, err := lua.New(lua.Options{
		Registry:    e.reg,
		Keymap:      e.keys,
		ModeKeymaps: map[string]*keymap.Map{"dired": e.diredKeys, "wdired": e.wdiredKeys},
		ConfigPath:  path,
		Timeout:     configTimeout,
	})
	if err != nil {
		e.Echo("config: %v", err)
		return err
	}
	e.host = h

	loadErr := h.LoadConfig()
	if loadErr != nil {
		// A missing config file is not an error, so anything reaching here is
		// worth showing. Brief() is the message without the traceback, which is
		// what fits on one row.
		var se *lua.ScriptError
		if errors.As(loadErr, &se) {
			e.Echo("init.lua: %s", se.Brief())
		} else {
			e.Echo("init.lua: %v", loadErr)
		}
	}

	// Settings are applied here rather than by the host, because text.TabWidth
	// is package-level state and because a changed setting has to invalidate
	// what the renderer cached.
	e.applySettings(h.Settings())

	e.wireHooks(h)

	// A key bound to a command that does not exist is a dead key. The host
	// permits it — a script may bind before defining — so the editor reports
	// whatever is still dangling once the whole config has run.
	if dangling := h.UnresolvedBindings(); len(dangling) > 0 {
		e.Echo("init.lua: %d binding(s) name no command: %v", len(dangling), dangling)
	}
	return loadErr
}

// applySettings pushes validated settings into the places that hold them.
func (e *Editor) applySettings(s lua.Settings) {
	if s.TabWidth > 0 {
		text.TabWidth = text.ColIdx(s.TabWidth)
	}
	if s.ScrollMargin >= 0 {
		e.th.ScrollMargin = s.ScrollMargin
	}
	// UndoStyle has one legal value, already validated by the host, so there is
	// nothing to apply until a second model exists.

	e.th.LineNumbers = s.LineNumbers
	e.SetDeleteSelection(s.DeleteSelection)
	e.th.Syntax = s.Syntax
	// "auto" guesses from the terminal; an explicit choice always wins, because
	// the guess relies on COLORFGBG which many terminals never set.
	switch s.Theme {
	case "light":
		e.th.UseSyntaxPalette(true)
	case "dark":
		e.th.UseSyntaxPalette(false)
	default:
		e.th.UseSyntaxPalette(ui.TerminalIsLight())
	}
	e.SetOpenBinary(s.OpenBinary)
	if s.FillColumn > 0 {
		command.FillColumn = s.FillColumn
	}
	command.AutoPair = s.AutoPair
	switch s.Icons {
	case "on":
		e.SetIcons(true)
	case "off":
		e.SetIcons(false)
	default:
		e.SetIcons(detectIcons())
	}
	e.SetCompletionStyle(s.CompletionStyle)
	e.SetCompletionRows(s.CompletionRows)
	// Zero disables each of these, which is why they are passed through
	// unconditionally rather than guarded by a > 0 check.
	e.SetWhichKeyDelay(time.Duration(s.WhichKeyDelay) * time.Millisecond)
	e.SetAutosaveIdle(time.Duration(s.AutosaveIdle) * time.Second)
	e.SetBackupEnabled(s.Backup)
	if s.Clipboard == "off" {
		e.SetClipboardMode(ClipboardOff)
	} else {
		e.SetClipboardMode(ClipboardOSC52)
	}
}

// wireHooks connects the script's hooks to command dispatch.
//
// The hook fires through the editor's own Before/AfterCommand seams, so the
// command layer still knows nothing about Lua: save-buffer remains a function of
// Env, and a hook failure is reported rather than propagated, because a broken
// format-on-save hook must not prevent the save.
func (e *Editor) wireHooks(h *lua.Host) {
	for hook, cmds := range hookCommands {
		if h.HookCount(hook) == 0 {
			continue
		}
		hook := hook
		for _, cmd := range cmds {
			fire := func() {
				if err := h.FireHook(hook, e, e.Buf()); err != nil {
					e.Echo("%s: %v", hook, err)
				}
			}
			if len(hook) >= 7 && hook[:7] == "before-" {
				e.BeforeCommand(cmd, fire)
			} else {
				e.AfterCommand(cmd, fire)
			}
		}
	}
}

// CloseConfig releases the Lua interpreter.
func (e *Editor) CloseConfig() {
	if e.host != nil {
		e.host.Close()
		e.host = nil
	}
}
