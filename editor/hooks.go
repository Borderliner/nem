package editor

// Hooks fire around command dispatch, keyed on command name.
//
// This is where the Lua layer hangs before-save without the command package
// ever learning that scripting exists: save-buffer stays a function of Env, and
// Env has no RunHook. That boundary is deliberate — a hook mechanism on Env
// would put the scripting layer in front of every command in the editor.

// BeforeCommand registers fn to run immediately before name is dispatched.
// Several hooks on one name run in registration order.
func (e *Editor) BeforeCommand(name string, fn func()) {
	if fn == nil {
		return
	}
	e.before[name] = append(e.before[name], fn)
}

// AfterCommand registers fn to run immediately after name is dispatched,
// whether or not the command returned an error — an after-save hook still wants
// to know a save was attempted.
func (e *Editor) AfterCommand(name string, fn func()) {
	if fn == nil {
		return
	}
	e.after[name] = append(e.after[name], fn)
}

func (e *Editor) runHooks(hooks map[string][]func(), name string) {
	for _, fn := range hooks[name] {
		fn()
	}
}
