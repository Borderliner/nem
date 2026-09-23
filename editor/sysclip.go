package editor

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"time"
	"unicode/utf8"
)

// Reading the system clipboard.
//
// OSC 52 carries kills out to the clipboard, but almost no terminal will answer
// an OSC 52 read, so text copied in another application never came back in:
// C-y reported the kill ring empty with a browser's selection sitting right
// there. Emacs solves this with interprogram-paste-function, which C-y consults
// before the ring, and nem does the same with the clipboard tools the desktop
// already provides.
//
// Those tools are strict about what counts as text, and so is this. Only stdout
// is read, and only when the tool exits 0 with valid UTF-8. That rule was learned
// the hard way: with an image on the clipboard, wl-paste --type text exits 1 and
// prints its explanation on stderr, and merging the two streams would paste
// "Clipboard content is not available as requested type" into the buffer.

// clipboardTimeout bounds how long C-y waits for a clipboard tool. A healthy one
// answers in milliseconds; one stuck on an unresponsive selection owner must not
// freeze the editor.
const clipboardTimeout = 500 * time.Millisecond

// clipboardReader returns the system clipboard's text, and false when there is
// no text to be had: no tool, no display, an image, or a tool that failed.
type clipboardReader func() (string, bool)

// defaultClipboardReader is what New installs. The test binary replaces it in
// TestMain, so no test can read the developer's real clipboard even by building
// an editor some other way than newTestEditor.
var defaultClipboardReader clipboardReader = readSystemClipboard

// readSystemClipboard asks this session's clipboard tools for text.
func readSystemClipboard() (string, bool) {
	return readClipboardFrom(clipboardCommands(runtime.GOOS, os.Getenv), clipboardTimeout)
}

// clipboardCommands lists the tools to try, in order, for this session.
//
// The choice follows the session rather than what happens to be installed: an
// X11 tool answering inside a Wayland session reads XWayland's clipboard, which
// is not always the one the user just copied to. XWayland sessions set both
// variables, and there the native Wayland tool goes first. Over SSH with no
// forwarded display nothing is listed, and C-y is the kill ring alone.
//
// Every tool is asked for text specifically, so an image is refused by the tool
// rather than handed over as bytes.
func clipboardCommands(goos string, getenv func(string) string) [][]string {
	if goos == "darwin" {
		return [][]string{{"pbpaste"}}
	}
	var cmds [][]string
	if getenv("WAYLAND_DISPLAY") != "" {
		cmds = append(cmds, []string{"wl-paste", "--no-newline", "--type", "text"})
	}
	if getenv("DISPLAY") != "" {
		cmds = append(cmds,
			[]string{"xclip", "-selection", "clipboard", "-out", "-target", "UTF8_STRING"},
			[]string{"xsel", "--clipboard", "--output"},
		)
	}
	return cmds
}

// readClipboardFrom returns the text of the first tool that yields any.
func readClipboardFrom(cmds [][]string, timeout time.Duration) (string, bool) {
	for _, argv := range cmds {
		if s, ok := runClipboardCommand(argv, timeout); ok {
			return s, true
		}
	}
	return "", false
}

// runClipboardCommand runs one tool and returns its stdout as text.
//
// A tool that is not installed, exits non-zero, prints nothing, prints bytes that
// are not UTF-8, or overruns the timeout yields nothing. Stderr is never read as
// text.
func runClipboardCommand(argv []string, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	// The context kills the tool, but Wait also waits for stdout to close, and a
	// grandchild the tool spawned can hold it open after the kill. WaitDelay
	// bounds that wait, so the timeout is a real bound.
	cmd.WaitDelay = timeout

	out, err := cmd.Output()
	if err != nil || len(out) == 0 || !utf8.Valid(out) {
		return "", false
	}
	return string(out), true
}
