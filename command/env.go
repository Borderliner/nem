// Package command defines nem's command layer: the named commands the editor
// can run, the registry that holds them, and the Env through which a command
// touches editor state.
//
// Env is an interface declared here and implemented by the editor package. A
// command may move point, edit the buffer, kill and yank, prompt in the
// minibuffer, and manage windows and buffers. It may never reach the tcell
// screen or the layout tree: there is deliberately no method that exposes
// either. That boundary is what lets every command be tested headlessly
// against commandtest.Fake, and it is why this package imports text, keymap
// and view but neither ui nor editor.
//
// Env is an interface rather than a struct because a struct holding editor
// state would force this package to import editor, which imports this package.
//
// Three things are deliberately absent from Env. They are recorded here so
// that their absence is not later mistaken for an oversight:
//
//   - Lua hooks. The editor fires hooks around dispatch, keyed on command
//     name, so save-buffer need not know that before-save exists. A RunHook
//     method here would leak the scripting layer into every command; do not
//     add one.
//   - Paren highlighting. show-paren is computed by ui from point at render
//     time. Nothing highlight-related belongs on Env, because Env cannot reach
//     the screen — that is this boundary working, not a gap in it.
//   - Buffer naming in text. A text.Buffer has only a Path; display names live
//     in the editor's name-to-buffer map. That is why *scratch* and
//     *Buffer List* are editor concepts, and why BufferName and BufferByName
//     are methods here rather than on the buffer itself.
package command

import (
	"errors"

	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
)

// Func is the signature every command implementation has. A returned error is
// reported to the user in the echo area; returning ErrQuit is not an error
// condition but an abandoned operation.
type Func func(Env) error

var (
	// ErrQuit reports that the user pressed C-g. ReadString, ReadChar and
	// ReadKey return it when a prompt is abandoned, and commands should
	// propagate it rather than treating it as a failure.
	ErrQuit = errors.New("quit")

	// ErrNoMark is returned by commands needing a region when the buffer has
	// no mark set.
	ErrNoMark = errors.New("no mark set in this buffer")

	// ErrUnknownCommand is returned by Env.Run and Registry.Run for a name
	// that is not registered.
	ErrUnknownCommand = errors.New("no such command")

	// ErrBeginningOfBuffer and ErrEndOfBuffer report that point could not move
	// because it already sits at a boundary.
	//
	// These are conditions rather than faults: a caller normally reports one
	// through Echo and carries on rather than treating it as a failure. They
	// are declared here, exported, rather than privately in whichever file
	// raises them, so that the dispatcher can tell a harmless boundary from a
	// genuine error with errors.Is across package boundaries.
	ErrBeginningOfBuffer = errors.New("beginning of buffer")
	ErrEndOfBuffer       = errors.New("end of buffer")
)

// Seq holds state that spans consecutive commands.
//
// Commands read and mutate it in place; the editor resets whichever fields
// need resetting at dispatch. It is a concrete struct rather than an untyped
// scratch slot so that every field is type-checked — a failed type assertion
// inside a command is a panic, and a panic costs the user unsaved work.
//
// A future sequencing need is a new field here, not a new method on Env.
type Seq struct {
	// LastYankFrom and LastYankTo bound the text the most recent yank
	// inserted; HasLastYank reports whether they are meaningful.
	//
	// yank-pop cannot work without them: it must delete what the preceding
	// yank inserted before putting the rotated entry in its place.
	LastYankFrom, LastYankTo text.Pos
	HasLastYank              bool

	// RecenterCycle is recenter-top-bottom's position in its centre, top,
	// bottom cycle across successive C-l presses.
	RecenterCycle int

	// LastRune is the rune of the key that triggered the current command. The
	// event loop sets it immediately before dispatch.
	//
	// It is meaningful only for a command invoked by a self-inserting key, and
	// self-insert-command is the one command that needs it: it must insert the
	// character that was typed, and nothing else in Env reports which key ran
	// the command. ReadKey would prompt the user, which is not the same thing
	// at all.
	//
	// Carrying this here rather than as an Env method is what lets
	// self-insert-command be an ordinary registered command, reachable from
	// M-x and bindable from Lua, instead of a stub that only the event loop
	// can call.
	LastRune rune
}

// CompleteFunc returns the candidates available for what has been typed so far.
//
// It supplies the candidate UNIVERSE, not a filtered result: the minibuffer
// filters and ranks with fuzzy matching, so a CompleteFunc that pre-filtered by
// prefix would defeat it. Typing "fwc" to reach forward-char returns no
// prefix matches at all, and the list handed back would be empty.
//
// The argument is still the current contents, because for some completions it
// selects which universe applies rather than narrowing one: a filename
// completion reads the directory the input names, and then offers everything in
// it rather than only the entries whose base matches.
type CompleteFunc func(input string) []string

// ReadOpts configures a minibuffer prompt.
type ReadOpts struct {
	// Prompt is shown at the start of the minibuffer line, e.g. "Find file: ".
	Prompt string

	// Initial pre-fills the minibuffer with editable text.
	Initial string

	// Complete supplies completion candidates. Nil means no completion at all:
	// no candidate panel is built and TAB does nothing, which is what an
	// incremental search prompt wants.
	Complete CompleteFunc

	// RequireMatch governs what RET accepts.
	//
	// When true, RET accepts only a candidate — the selected one, or an exact
	// match for what was typed. Typed text that matches nothing is refused with
	// a message rather than returned. execute-extended-command sets this,
	// because inventing a command name is meaningless.
	//
	// When false, RET accepts exactly what was typed whenever it is not an exact
	// candidate, so find-file and switch-to-buffer can still name something that
	// does not exist yet. This is why the flag exists instead of Vertico's
	// always-take-the-selection rule: without it, C-x C-f newfile.go could never
	// create a file, because the selection would win and open an existing one.
	//
	// M-RET forces literal input where RequireMatch is false.
	RequireMatch bool

	// OnChange, when non-nil, is called with the full contents after every
	// edit of the minibuffer — typed, backspaced, killed or yanked alike.
	//
	// This is the hook that makes search-as-you-type fall out of the ordinary
	// prompt mechanism rather than needing one of its own.
	//
	// It runs with Win reporting the PRE-PROMPT TEXT WINDOW, not the
	// minibuffer's, because a hook that reacts to the pattern needs to move
	// point in the buffer being searched. Write an OnChange that assumes the
	// minibuffer window and it will move point in the prompt instead, which
	// reads as a rendering bug rather than as the mistake it is.
	OnChange func(string)

	// Session, when non-nil, is the incremental-search session this prompt
	// drives. The minibuffer calls its Update after every edit and its Advance
	// when the search key is pressed again inside the prompt, so the prompt and
	// the search share one session.
	//
	// It exists because Advance cannot be expressed as a callback: a repeated
	// C-s must step to the next match, which is a property of the session
	// rather than of the pattern, and is not recoverable from an OnChange
	// closure. Passing the session explicitly is what keeps the editor from
	// having to guess which prompts are searches from their command names.
	//
	// Like OnChange, it runs against the pre-prompt text window. Session and
	// OnChange are independent: set both and both are called.
	Session *Isearch
}

// Env is the whole of the editor a command may touch.
type Env interface {
	// --- the active view -------------------------------------------------

	// Win returns the active window: the buffer being edited, where point is,
	// and which lines are on screen.
	//
	// While a prompt is open, Win reports the MINIBUFFER's window, not the
	// text window. That is deliberate and is what makes prompts editable for
	// free: C-a, C-e, C-k, M-b and the kill ring are ordinary commands, and
	// they must act on the prompt the user is typing into.
	//
	// The exception is ReadOpts.OnChange and ReadOpts.Session, which run
	// against the pre-prompt text window — see the note on OnChange. Anything
	// else that needs the text window while a prompt is open is asking for
	// trouble; there is no accessor for it, by design.
	Win() *view.Window

	// Buf returns the active window's buffer. Shorthand for Win().Buf.
	Buf() *text.Buffer

	// TextHeight reports how many rows of buffer text the active window shows,
	// excluding its modeline. Needed by every command that moves by screenfuls
	// or repositions the viewport: scroll-up-command, scroll-down-command and
	// recenter-top-bottom cannot be written without it.
	TextHeight() int

	// --- the universal argument ------------------------------------------

	// Arg reports the prefix argument. n is 1 when none was given, and
	// explicit distinguishes a bare command from C-u 1, which some commands
	// treat differently.
	Arg() (n int, explicit bool)

	// --- the kill ring ---------------------------------------------------

	// KillForward records text killed forward of point; KillBackward records
	// text killed backward of it. The ring handles kill-run accumulation, so a
	// command states only the direction.
	KillForward(s string)
	KillBackward(s string)

	// Yank returns the current kill-ring entry without consuming it.
	Yank() (string, error)

	// YankPop rotates to the next-older entry and returns the full replacement
	// text. It is valid only immediately after a yank.
	YankPop() (string, error)

	// --- command sequencing ----------------------------------------------

	// LastCommand names the command that ran immediately before this one, or
	// "" at the start of a session. This is emacs's last-command.
	LastCommand() string

	// Seq returns the sequencing state shared across consecutive commands.
	//
	// The pointer is stable for the life of the session, so commands mutate
	// the struct in place: e.Seq().RecenterCycle++ is the intended idiom.
	Seq() *Seq

	// --- the minibuffer --------------------------------------------------

	// ReadString prompts in the minibuffer and returns what the user typed,
	// or ErrQuit if they pressed C-g.
	ReadString(opts ReadOpts) (string, error)

	// ReadChar prompts for a single keystroke, accepting only the runes in
	// valid (all runes when valid is empty), and returns it. This is what
	// query-replace's y/n/!/q loop and save-some-buffers are built from; a
	// ReadString would force the user to press RET after every answer.
	ReadChar(prompt string, valid []rune) (rune, error)

	// ReadKey prompts for one raw keystroke and returns it undecoded, for
	// describe-key.
	ReadKey(prompt string) (keymap.Key, error)

	// Echo shows a message in the echo area.
	Echo(format string, a ...any)

	// --- buffers ---------------------------------------------------------

	// Buffers lists every live buffer, most recently visited first.
	Buffers() []*text.Buffer

	// BufferName is the buffer's display name: the basename of its file, or a
	// generated name such as "*scratch*" for one with no file.
	BufferName(b *text.Buffer) string

	// BufferByName finds a buffer by display name, for switch-to-buffer.
	BufferByName(name string) (*text.Buffer, bool)

	// NewBuffer creates a live, file-less buffer under the given display name.
	// list-buffers renders into one of these, as does the startup *scratch*.
	NewBuffer(name string) *text.Buffer

	// OpenFile returns the buffer visiting path, reading it from disk if it is
	// not already open. A path that does not exist yields an empty buffer, as
	// find-file does.
	OpenFile(path string) (*text.Buffer, error)

	// KillBuffer removes a buffer from the live list. It is an error to kill
	// the last remaining buffer.
	KillBuffer(b *text.Buffer) error

	// SaveBuffer writes b to disk. An empty path saves to b's own path; a
	// non-empty path saves there and adopts it, which is write-file.
	//
	// Commands go through Env rather than calling text.Buffer.Save directly so
	// that tests can intercept saves and inject failures. That matters more
	// here than anywhere else in this interface: saving is the one operation
	// where a bug costs the user the work they were trying to protect.
	SaveBuffer(b *text.Buffer, path string) error

	// --- windows ---------------------------------------------------------

	// SplitWindow splits the active window, side by side when vertical is
	// true and stacked otherwise, and makes the new window active.
	SplitWindow(vertical bool) error

	// OtherWindow moves the selection n windows forward in cycle order,
	// wrapping; negative n moves backward.
	OtherWindow(n int)

	// DeleteWindow removes the active window. It is an error to remove the
	// sole window.
	DeleteWindow() error

	// DeleteOtherWindows makes the active window fill the frame.
	DeleteOtherWindows()

	// --- commands and bindings -------------------------------------------

	// Run executes another command by name, which is how M-x and the Lua
	// nem.run bridge invoke commands.
	Run(name string) error

	// CommandNames lists every interactive command name, sorted, for M-x
	// completion.
	CommandNames() []string

	// Bindings maps every bound key sequence to its command name, for
	// describe-bindings.
	Bindings() map[string]string

	// Where lists the key sequences bound to a command, for describe-key and
	// for showing a command's binding in M-x.
	Where(command string) []string

	// --- session ---------------------------------------------------------

	// Quit ends the editing session. Without force it fails if any buffer has
	// unsaved changes.
	Quit(force bool) error
}
