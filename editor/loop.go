package editor

import (
	"errors"
	"time"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/ui"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// Loop reads events and dispatches commands until the session ends.
//
// It is not called Run because Env.Run invokes a command by name; this is the
// event loop.
//
// It selects over a channel of events and a timer rather than blocking in
// PollEvent, because prefix-key discovery needs to notice that a prefix has sat
// pending for a while - a blocking read cannot express "or nothing happened for
// 300ms". tcell fills the channel from its own goroutine, but there is still
// exactly ONE consumer, which is what lets keymap.Map, command.KillRing and the
// Lua interpreter stay lock-free. Nothing in here may be moved onto another
// goroutine without revisiting that.
//
// The timer is rebuilt each iteration and stopped as soon as an event wins the
// select, so a fluent user who never pauses arms and discards a timer per
// keystroke and never sees a panel.
func (e *Editor) Loop() error {
	if e.scr == nil {
		return errors.New("editor: no screen")
	}

	events := make(chan tcell.Event)
	quit := make(chan struct{})
	go e.scr.ChannelEvents(events, quit)
	// Nested prompt loops read from this same channel. Without it they call
	// PollEvent while this goroutine is also reading, and two consumers race
	// for one keyboard.
	e.events = events
	defer func() { e.events = nil }()
	// Closing quit stops tcell's producer, so Loop cannot leave a goroutine
	// behind however it returns.
	defer close(quit)

	// Autosave is driven from here rather than from a goroutine: the editor's
	// state is touched by exactly one consumer and that invariant is what keeps
	// keymap, the kill ring and Lua lock-free.
	var tick <-chan time.Time
	if e.safe.idle > 0 {
		t := time.NewTicker(e.safe.idle)
		defer t.Stop()
		tick = t.C
	}

	e.Redraw()
	for !e.quit {
		var fire <-chan time.Time
		var timer *time.Timer
		if e.whichKeyArmed() {
			timer = time.NewTimer(e.whichKeyDelay())
			fire = timer.C
		}

		select {
		case ev, ok := <-events:
			if timer != nil {
				timer.Stop()
			}
			if !ok {
				return nil // the screen finalized underneath us
			}
			e.NoteInput(time.Now())
			e.HandleEvent(ev)
		case <-fire:
			e.fireWhichKey()
		case now := <-tick:
			// Reported through the echo area by RunAutosave itself; a failure
			// must not stop the loop.
			if e.AutosaveDue(now) {
				_ = e.RunAutosave(now)
			}
		}
		e.Redraw()
	}
	return nil
}

// nextEvent returns the next terminal event from whichever source is active.
//
// While Loop runs, tcell feeds events to a channel from its own goroutine, and
// a nested prompt loop MUST take them from that channel. Calling PollEvent
// directly instead makes two consumers race for one keyboard: the channel's
// goroutine is already blocked in a read and wins, so the first keystroke after
// a prompt opens is swallowed and surfaces only once the prompt closes. That is
// exactly what "C-s then the first letter does nothing" looked like.
//
// Outside Loop - tests that drive HandleEvent directly - there is no channel
// and PollEvent is the only source, so it stays the fallback.
func (e *Editor) nextEvent() tcell.Event {
	if e.events == nil {
		return e.scr.PollEvent()
	}
	ev, ok := <-e.events
	if !ok {
		return nil // the screen finalized underneath us
	}
	return ev
}

// HandleEvent processes one terminal event. Exported so tests can drive the
// editor a keystroke at a time without a real loop.
func (e *Editor) HandleEvent(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		k := DecodeKey(ev, e.keys.TreatCtrlHAsBackspace)
		if k == (keymap.Key{}) {
			return // nothing could name this key; ignore it
		}
		e.HandleKey(k)
	case *tcell.EventClipboard:
		// The terminal's answer to GetClipboard. Routed here rather than in the
		// loop because ReadChar and ReadKey forward non-key events to
		// HandleEvent, so a reply arriving during a prompt is not lost.
		e.SetClipboardReply(ev.Data())
	case *tcell.EventResize:
		// Nothing to recompute: layout is derived from the screen size on every
		// frame, so a resize is just a redraw. A degenerate size is handled by
		// the renderer rather than guarded here.
		e.scr.Sync()
	}
}

// HandleKey resolves one decoded key and dispatches whatever it names.
func (e *Editor) HandleKey(k keymap.Key) {
	// Any key hides a which-key panel, and is then resolved normally. Dismissing
	// here rather than in each branch below is what guarantees the keystroke that
	// dismisses the panel is not also consumed by it - the feature must cost a
	// fluent user nothing.
	e.dismissWhichKey()
	// The welcome panel goes on the first keystroke, whatever it was, and does
	// not come back. Dismissing here rather than per branch is what keeps the
	// key itself from being consumed.
	e.dismissStartup()

	// C-g is handled before anything else, because in emacs it is never a
	// no-op. Cancelling a half-typed C-x prefix or a half-typed argument must
	// happen here: appending C-g to the pending sequence would look up "C-x
	// C-g", find nothing, and report it undefined instead of cancelling.
	//
	// A prompt is not handled here — its own keymap binds C-g to abort, and
	// that map is consulted first.
	if quitKey(k) && e.mini == nil && (len(e.pending) > 0 || e.arg.active) {
		e.pending = nil
		e.arg.reset()
		e.Echo("Quit")
		return
	}

	// The universal argument is resolved before keymap lookup, so C-u 4 C-f
	// reaches forward-char with an argument rather than being looked up as a
	// four-key sequence.
	if len(e.pending) == 0 && e.argKey(k) {
		return
	}

	e.pending = append(e.pending, k)
	res, from := e.lookup(e.pending)

	switch res.Kind {
	case keymap.Pending:
		// Show the prefix so C-x announces that it is waiting.
		e.Echo("%s-", keymap.SpecString(e.pending))

	case keymap.Found:
		// Clear the echo area only when it was showing a prefix indicator we
		// put there. Clearing it for every command would discard messages a
		// command legitimately left behind — a failing incremental search
		// reports through Echo and then the accepting RET would erase it.
		hadPrefix := len(e.pending) > 1
		e.pending = nil
		if hadPrefix {
			e.echo = ""
		}
		e.runKeyCommand(res.Command, k, from)

	default:
		spec := keymap.SpecString(e.pending)
		e.pending = nil
		// A lone printable key with no binding inserts itself. This is the
		// fallback that makes an editor an editor, and it is why
		// self-insert-command needs the triggering rune: nothing else in Env
		// reports which key ran a command.
		if selfInserting(k) {
			e.seq.LastRune = k.Rune
			e.dispatchReporting("self-insert-command")
			e.afterMiniEdit()
			return
		}
		e.arg.reset()
		e.Echo("%s is undefined", spec)
	}
}

// quitKey reports whether k is C-g.
func quitKey(k keymap.Key) bool {
	return k.Ctrl && !k.Meta && k.Rune == 'g'
}

// selfInserting reports whether an unbound key should insert itself: a
// printable rune with no Ctrl or Meta.
func selfInserting(k keymap.Key) bool {
	return k.Special == keymap.SpecialNone && !k.Ctrl && !k.Meta && k.Rune >= ' ' && k.Rune != 0x7F
}

// keymapStack is the ordered list of maps a key is resolved against:
// innermost first.
//
// A prompt's map comes first so RET accepts and C-g aborts, but it deliberately
// binds nothing else — everything it does not name falls through to the global
// map, which is how C-a, C-e, C-k and yank end up editing the prompt with no
// duplicate implementations.
func (e *Editor) keymapStack() []*keymap.Map {
	if e.mini != nil {
		return []*keymap.Map{e.mini.keys, e.keys}
	}
	return []*keymap.Map{e.keys}
}

// lookup resolves seq against the keymap stack, returning the first decisive
// result and the map that gave it.
//
// A Pending in an outer map still counts: C-x is not bound in the prompt map,
// so without honouring the global map's Pending a prefix would be reported
// undefined the moment a prompt was open.
func (e *Editor) lookup(seq []keymap.Key) (keymap.Result, *keymap.Map) {
	var pending *keymap.Map
	for _, m := range e.keymapStack() {
		switch res := m.Lookup(seq); res.Kind {
		case keymap.Found:
			return res, m
		case keymap.Pending:
			if pending == nil {
				pending = m
			}
		}
	}
	if pending != nil {
		return keymap.Result{Kind: keymap.Pending}, pending
	}
	return keymap.Result{Kind: keymap.Undefined}, nil
}

// runKeyCommand runs the command a key sequence named.
//
// A prompt-control name is handled by the minibuffer directly rather than
// through the registry, because accepting or aborting a prompt is a property of
// the prompt rather than an editor-wide command.
func (e *Editor) runKeyCommand(name string, k keymap.Key, from *keymap.Map) {
	if e.mini != nil && from == e.mini.keys {
		e.mini.control(e, name)
		return
	}
	e.seq.LastRune = k.Rune
	e.dispatchReporting(name)
	e.afterMiniEdit()
}

// dispatchReporting dispatches and reports any failure in the echo area.
//
// ErrQuit is an abandoned operation rather than a failure, and the boundary
// sentinels are conditions rather than faults — a user who presses C-f at the
// end of the buffer wants silence, not an error.
func (e *Editor) dispatchReporting(name string) {
	err := e.dispatch(name)
	switch {
	case err == nil,
		errors.Is(err, command.ErrQuit),
		errors.Is(err, command.ErrBeginningOfBuffer),
		errors.Is(err, command.ErrEndOfBuffer):
	default:
		e.Echo("%s", err.Error())
	}
}

// dispatch runs one command and performs every piece of cross-command
// bookkeeping.
//
// This is the load-bearing part of the editor. Three separate concerns are
// centralised here because each of them, left to individual commands, would be
// forgotten in exactly one of sixty places and the resulting bug would look
// like a fault in the kill ring or in motion rather than an omission:
//
//   - The kill run. Every command that is not a kill or a yank breaks it, which
//     is what makes "kill, move, kill" produce two ring entries.
//   - The goal column. Every command that is not a vertical motion clears it,
//     so C-n only preserves a column across a run of C-n and C-p.
//   - last-command. Recorded after the command runs, so a command sees the
//     previous one, which is what yank-pop and recenter-top-bottom need.
//   - Every window's point, clamped back inside its buffer. See
//     clampWindowPoints: this one prevents a crash, not a misbehaviour.
//   - An active region, consumed by the handful of commands that replace it.
//     See delsel.go.
//
// It is re-entrant: M-x and Lua's nem.run come through here too. Only the
// innermost dispatch does the bookkeeping, so M-x kill-line leaves the kill run
// open exactly as C-k would, and last-command names kill-line rather than
// execute-extended-command.
func (e *Editor) dispatch(name string) error {
	if _, ok := e.reg.Lookup(name); !ok {
		e.arg.reset()
		return command.ErrUnknownCommand
	}

	e.childDispatched = false

	// A region the command is about to replace is deleted first, inside an undo
	// group that stays open across the command so the two undo together. skip
	// is true when the deletion was the whole operation.
	skip, closeGroup := e.consumeSelection(name)
	if closeGroup != nil {
		defer closeGroup()
	}

	var err error
	e.runHooks(e.before, name)
	if !skip {
		err = e.reg.Run(name, e)
	}
	e.runHooks(e.after, name)

	// Outside the childDispatched guard on purpose: clamping is idempotent and
	// costs one comparison per window, and a nested dispatch that shortens a
	// buffer must not be able to leave a window stranded either.
	e.clampWindowPoints()

	if !e.childDispatched {
		e.bookkeep(name)
	}
	e.arg.reset()

	// Tell our caller that a nested dispatch handled the bookkeeping.
	e.childDispatched = true
	return err
}

// bookkeep applies the three cross-command rules. See dispatch.
func (e *Editor) bookkeep(name string) {
	if breaksKillRun(name) {
		e.ring.BreakRun()
	}
	if !keepsGoalColumn(name) {
		// Cleared on the window the command acted on, which is the prompt's
		// window while a prompt is open.
		if w := e.Win(); w != nil {
			w.GoalCol = view.GoalColUnset
		}
	}
	e.lastCmd = name
}

// clampWindowPoints brings every window's point back inside its buffer.
//
// Point lives in the window, and a buffer has no idea which windows are showing
// it - so an edit made through one window can leave another window's point past
// the end of the text. text.Buffer.Line is an unguarded slice index, so the next
// command run in that window indexes out of range and takes the whole editor
// down, losing unsaved work in every other buffer with it.
//
// Two windows onto one buffer is the case the architecture exists for, so this
// is reachable by ordinary editing and not only by a script: kill more lines
// than the other window's point sits above, switch to it, press any motion key.
//
// Centralised here for the same reason as the three rules above - sixty commands
// cannot each be relied on to remember it.
func (e *Editor) clampWindowPoints() {
	for _, w := range e.tree.Windows() {
		w.Pt = w.Buf.ClampPos(w.Pt)
	}
}

// Redraw paints one frame. Exported so tests can assert on rendered output.
// frame builds the frame for the current state. It is the single place a frame
// is constructed, so what a test asserts on and what Redraw paints cannot drift
// apart - a prompt used to build its own frame separately and that duplication
// was already diverging.
func (e *Editor) frame() ui.Frame {
	// The editor owns the name map, so it is the only thing that can tell the
	// renderer what a path-less buffer is called.
	f := ui.Frame{
		Tree: e.tree, Active: e.active, Echo: e.echo,
		NameOf:   e.BufferName,
		SpansOf:  e.spansOf,
		TypeOf:   e.FileType,
		BranchOf: e.BranchOf,
	}
	if e.mini != nil {
		f.Echo = e.mini.line()
		f.MiniPt = e.mini.cursorCol()
		f.MiniOn = true
	}
	// Panel sources append here. A prompt rendering its completion list is one;
	// which-key is another. They are mutually exclusive in practice, since
	// which-key does not arm while a prompt is open.
	e.decorateWithCompletion(&f)
	if p := e.whichKeyPanel(); p != nil {
		f.Panels = append(f.Panels, *p)
	}
	if p, ok := e.startupPanel(); ok {
		f.Panels = append(f.Panels, p)
	}
	return f
}

func (e *Editor) Redraw() {
	if e.scr == nil {
		return
	}
	ui.Render(e.scr, e.frame(), e.th)
	e.scr.Show()
}

// startupPanel builds the welcome panel when one is wanted and there is room.
//
// The frame handed to the placer is the screen minus the echo row, matching the
// area the split tree is laid out in, so the panel can never cover the row a
// message or a prompt would appear on.
func (e *Editor) startupPanel() (ui.Panel, bool) {
	if !e.startup || e.scr == nil {
		return ui.Panel{}, false
	}
	w, h := e.scr.Size()
	if w <= 0 || h <= 1 {
		return ui.Panel{}, false
	}
	return ui.StartupPanel(view.Rect{W: w, H: h - 1}, e.th)
}
