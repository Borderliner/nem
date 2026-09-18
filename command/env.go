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
package command

import (
	"errors"

	"github.com/hajianpour/nem/keymap"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
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
)

// CompleteFunc returns the candidate completions for a minibuffer prefix.
type CompleteFunc func(prefix string) []string

// ReadOpts configures a minibuffer prompt.
type ReadOpts struct {
	// Prompt is shown at the start of the minibuffer line, e.g. "Find file: ".
	Prompt string

	// Initial pre-fills the minibuffer with editable text.
	Initial string

	// Complete supplies TAB completion candidates. Nil means no completion.
	Complete CompleteFunc

	// OnChange, when non-nil, is called with the full contents after every
	// edit of the minibuffer.
	//
	// This is the hook that makes incremental search fall out of the ordinary
	// prompt mechanism rather than needing one of its own: isearch is a
	// ReadString whose OnChange searches from the saved start position and
	// moves point to the match, so the display updates as the user types.
	OnChange func(string)
}

// Env is the whole of the editor a command may touch.
type Env interface {
	// --- the active view -------------------------------------------------

	// Win returns the active window: the buffer being edited, where point is,
	// and which lines are on screen.
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

	// Scratch and SetScratch hold state that survives only between
	// consecutive runs of the same command; the dispatcher clears it as soon
	// as a different command runs.
	//
	// Two v1 commands genuinely need this. yank-pop must delete the text the
	// preceding yank inserted, so yank records that extent here. And
	// recenter-top-bottom cycles centre, top, bottom across successive C-l
	// presses, so it records its position in the cycle.
	Scratch() any
	SetScratch(v any)

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
