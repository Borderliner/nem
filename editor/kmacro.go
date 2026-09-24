package editor

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Keyboard macros: F3 starts recording keystrokes, F4 stops, and F4 again
// plays them back. C-x ( and C-x ) do the same as F3 and F4, and C-x e plays,
// after which a bare e plays again.
//
// What is recorded is terminal events, not commands, and playback feeds them
// back through the same doors they came in by: HandleEvent for the top level,
// nextEvent for a prompt's nested loop. So a macro that searches, answers a
// query-replace or types into M-x replays exactly, with no command needing to
// know that macros exist.
//
// Playback stops at the first command that fails, as in emacs. That is what
// makes C-u 0 F4 - repeat until it fails - the way to run a macro down the rest
// of a file: C-n at the last line, or a search that finds nothing more, ends
// it.

// kmacroEndless is the most times C-u 0 F4 plays a macro. A macro that can
// never fail - one that only inserts text - would otherwise hang the editor,
// with no way to interrupt it because playback does not read the keyboard.
const kmacroEndless = 10000

var (
	// errKmacroNone is playing with nothing recorded.
	errKmacroNone = errors.New("no keyboard macro defined")
	// errUndefinedKey stops a macro that plays a key bound to nothing.
	errUndefinedKey = errors.New("undefined key")
)

// kmacroState is the whole feature's state.
type kmacroState struct {
	recording bool
	events    []tcell.Event // being recorded
	last      []tcell.Event // the most recently finished macro

	// queue is the playback in progress, which nextEvent drains before
	// reading the terminal.
	queue []tcell.Event
	// playing counts nested playbacks, so events played back are never
	// recorded again - a macro called while recording another is recorded as
	// the call, not as what it did.
	playing int
	// failed reports that a command failed during playback.
	failed bool

	// counter is what F3 inserts while a macro is recorded or played.
	counter int

	// repeatE lets a bare e replay the macro straight after C-x e.
	repeatE bool
}

// recordEvent notes an event the editor is about to act on, while recording.
// Only keystrokes and pastes: a resize or a clipboard reply is not something
// the user did.
func (e *Editor) recordEvent(ev tcell.Event) {
	if !e.km.recording || e.km.playing > 0 {
		return
	}
	switch ev.(type) {
	case *tcell.EventKey, *tcell.EventPaste:
		e.km.events = append(e.km.events, ev)
	}
}

// noteFailure is told of every command's error, and stops a playback on one.
func (e *Editor) noteFailure(err error) {
	if err != nil && e.km.playing > 0 {
		e.km.failed = true
	}
}

// registerKmacroCommands adds the keyboard macro commands.
func registerKmacroCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{Name: "kmacro-start-macro-or-insert-counter",
			Doc: "Start recording a keyboard macro, or while recording, insert the macro counter.",
			Fn:  func(command.Env) error { return e.kmacroStartOrCounter() }},
		{Name: "kmacro-end-or-call-macro",
			Doc: "Stop recording a keyboard macro, or play the last one, ARG times (0: until it fails).",
			Fn:  func(command.Env) error { return e.kmacroEndOrCall() }},
		{Name: "kmacro-start-macro", Doc: "Start recording a keyboard macro.",
			Fn: func(command.Env) error { return e.kmacroStart() }},
		{Name: "kmacro-end-macro", Doc: "Stop recording a keyboard macro.",
			Fn: func(command.Env) error { return e.kmacroEnd() }},
		{Name: "kmacro-end-and-call-macro",
			Doc: "Play the last keyboard macro, ARG times; then e plays it again.",
			Fn: func(command.Env) error {
				if e.km.recording {
					if err := e.kmacroEnd(); err != nil {
						return err
					}
				}
				if err := e.kmacroCall(); err != nil {
					return err
				}
				e.km.repeatE = true
				if e.echo == "" {
					e.Echo("(Type e to repeat macro)")
				}
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

func (e *Editor) kmacroStartOrCounter() error {
	if e.km.recording || e.km.playing > 0 {
		return e.kmacroInsertCounter()
	}
	return e.kmacroStart()
}

func (e *Editor) kmacroStart() error {
	if e.km.recording {
		e.Echo("Already defining a keyboard macro")
		return nil
	}
	e.km.recording, e.km.events, e.km.counter = true, nil, 0
	e.Echo("Defining keyboard macro...")
	return nil
}

func (e *Editor) kmacroEnd() error {
	if !e.km.recording {
		e.Echo("Not defining a keyboard macro")
		return nil
	}
	// The keys that ended the recording were recorded on their way in; they
	// are not part of the macro.
	events := e.km.events
	if n := e.lastSeqLen; n > 0 && n <= len(events) {
		events = events[:len(events)-n]
	}
	e.km.recording, e.km.events = false, nil
	if len(events) == 0 {
		e.Echo("Ignoring an empty keyboard macro")
		return nil
	}
	e.km.last = events
	e.Echo("Keyboard macro defined")
	return nil
}

func (e *Editor) kmacroEndOrCall() error {
	if e.km.recording {
		return e.kmacroEnd()
	}
	return e.kmacroCall()
}

func (e *Editor) kmacroInsertCounter() error {
	s := strconv.Itoa(e.km.counter)
	e.km.counter++
	b, w := e.Buf(), e.Win()
	if err := b.Insert(w.Pt, []rune(s)); err != nil {
		return err
	}
	w.Pt.Col += text.RuneIdx(len(s))
	return nil
}

// kmacroCall plays the last macro ARG times, or until it fails for ARG 0.
func (e *Editor) kmacroCall() error {
	if len(e.km.last) == 0 {
		return errKmacroNone
	}
	n, explicit := e.Arg()
	if !explicit || n < 0 {
		n = 1
	}
	// The argument was for playing, not for the macro's first command.
	e.arg.reset()
	endless := n == 0
	if endless {
		n = kmacroEndless
	}

	macro := e.km.last
	e.km.playing++
	defer func() { e.km.playing-- }()
	saved := e.km.queue
	defer func() { e.km.queue = saved }()

	done := 0
	for ; done < n; done++ {
		e.km.failed = false
		e.km.queue = append([]tcell.Event(nil), macro...)
		for len(e.km.queue) > 0 && !e.km.failed && !e.quit {
			ev := e.km.queue[0]
			e.km.queue = e.km.queue[1:]
			e.handleEvent(ev)
		}
		if e.km.failed || e.quit {
			break
		}
	}
	switch {
	case e.km.failed && endless:
		e.Echo("Keyboard macro ran %s, until it failed", times(done))
	case e.km.failed:
		// The failing command's own message says what went wrong.
	case endless:
		e.Echo("Keyboard macro stopped after %s; it never failed", times(done))
	}
	return nil
}

// times is "once", "twice" or "5 times".
func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return fmt.Sprintf("%d times", n)
}

// kmacroRepeatKey handles the bare e that repeats a macro after C-x e,
// reporting whether k was that.
func (e *Editor) kmacroRepeatKey(k keymap.Key) bool {
	if !e.km.repeatE {
		return false
	}
	e.km.repeatE = false
	if k.Rune != 'e' || k.Ctrl || k.Meta || k.Special != keymap.SpecialNone || len(e.pending) > 0 {
		return false
	}
	if err := e.kmacroCall(); err != nil {
		e.Echo("%v", err)
		return true
	}
	e.km.repeatE = true
	return true
}
