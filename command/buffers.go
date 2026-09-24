package command

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Borderliner/nem/text"
)

// bufferListName is the display name of the buffer that list-buffers renders
// into. It is reused across invocations rather than recreated.
const bufferListName = "*Buffer List*"

// RegisterBuffers adds the file, buffer, window and session commands.
func RegisterBuffers(r *Registry) error {
	cmds := []Command{
		// files
		{Name: "find-file", Doc: "Visit a file in the current window, creating it if absent.", Fn: findFile},
		{Name: "save-buffer", Doc: "Save the current buffer, prompting for a name if it has none.", Fn: saveBuffer},
		{Name: "write-file", Doc: "Save the current buffer under a new name.", Fn: writeFile},
		{Name: "save-some-buffers", Doc: "Offer to save each modified buffer.", Fn: saveSomeBuffers},
		// buffers
		{Name: "switch-to-buffer", Doc: "Display another buffer, creating it if the name is new.", Fn: switchToBuffer},
		{Name: "kill-buffer", Doc: "Kill the current buffer, confirming if it is modified.", Fn: killBuffer},
		{Name: "list-buffers", Doc: "Display a listing of every live buffer.", Fn: listBuffers},
		// windows
		{Name: "split-window-below", Doc: "Split the current window, stacking the new one beneath it.", Fn: splitWindowBelow},
		{Name: "split-window-right", Doc: "Split the current window, placing the new one beside it.", Fn: splitWindowRight},
		{Name: "delete-other-windows", Doc: "Make the current window fill the frame.", Fn: deleteOtherWindows},
		{Name: "delete-window", Doc: "Remove the current window.", Fn: deleteWindow},
		{Name: "other-window", Doc: "Select another window, ARG windows forward.", Fn: otherWindow},
		// session
		{Name: "keyboard-quit", Doc: "Abandon the current operation.", Fn: keyboardQuit},
		{Name: "save-buffers-kill-terminal", Doc: "Offer to save modified buffers, then exit.", Fn: saveBuffersKillTerminal},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// --- files ---------------------------------------------------------------

// findFile reads a path and visits it.
//
// This is what the recursive-minibuffer design buys: prompting is a function
// call, so the command reads top to bottom instead of being split across a
// state machine. ErrQuit propagates untouched — the user pressed C-g before
// anything happened, which is an abandoned operation rather than a failure.
func findFile(e Env) error {
	path, err := e.ReadString(ReadOpts{
		Prompt:   "Find file: ",
		Complete: completeFilename,
		Descend:  isDirCandidate,
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	// A path that does not exist yields an empty buffer carrying that path.
	// That is how a new file is created, so it is deliberately not an error.
	buf, err := e.OpenFile(path)
	if err != nil {
		return err
	}
	e.Win().Visit(buf)
	return nil
}

// saveBuffer writes the current buffer, asking for a name if it has none.
func saveBuffer(e Env) error {
	b := e.Buf()
	// A buffer with no file cannot be saved silently; asking for a name is
	// exactly write-file's job, so defer to it rather than failing.
	if b.Path() == "" {
		return writeFile(e)
	}
	if !b.Modified() {
		e.Echo("(No changes need to be saved)")
		return nil
	}
	if err := e.SaveBuffer(b, ""); err != nil {
		return err
	}
	e.Echo("Wrote %s", b.Path())
	return nil
}

// writeFile saves the current buffer under a name the user supplies.
func writeFile(e Env) error {
	b := e.Buf()
	path, err := e.ReadString(ReadOpts{
		Prompt:   "Write file: ",
		Initial:  b.Path(),
		Complete: completeFilename,
		Descend:  isDirCandidate,
	})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if ok, err := confirmReplace(e, b, path); !ok || err != nil {
		return err
	}
	// SaveBuffer adopts the path only on success, so a failed write leaves the
	// buffer still pointing at wherever it came from.
	if err := e.SaveBuffer(b, path); err != nil {
		return err
	}
	e.Echo("Wrote %s", path)
	return nil
}

// confirmReplace asks before write-file writes over a file other than the
// buffer's own, and reports whether to go ahead.
//
// RET at the prompt takes the highlighted candidate, so a new name that
// fuzzy-matches an existing file arrives here as that file. Without the
// question it would be replaced without a word; with it, the mistake costs a
// keystroke. emacs's write-file asks the same thing.
func confirmReplace(e Env, b *text.Buffer, path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if abs == b.Path() {
		return true, nil
	}
	// Nothing there, or a directory the write will fail on anyway: there is
	// nothing to lose, so nothing to ask.
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return true, nil
	}
	c, err := e.ReadChar(
		fmt.Sprintf("File %s exists; overwrite? (y/n) ", path),
		[]rune{'y', 'n', 'Y', 'N'},
	)
	if err != nil {
		return false, err
	}
	if c == 'y' || c == 'Y' {
		return true, nil
	}
	e.Echo("Canceled")
	return false, nil
}

// saveSomeBuffers walks the modified buffers, offering to save each.
//
// Buffers with no file are skipped rather than prompted for a name: the answer
// set here is a single keystroke, and interrupting that with a filename prompt
// mid-walk is worse than leaving *scratch* alone.
func saveSomeBuffers(e Env) error {
	candidates, saved := 0, 0
	all := false

walk:
	for _, b := range e.Buffers() {
		if !b.Modified() || b.Path() == "" {
			continue
		}
		candidates++
		if !all {
			c, err := e.ReadChar(
				fmt.Sprintf("Save file %s? (y, n, !, q) ", b.Path()),
				[]rune{'y', 'n', '!', 'q'},
			)
			if err != nil {
				return err
			}
			switch c {
			case 'q':
				break walk
			case 'n':
				continue
			case '!':
				all = true
			}
		}
		if err := e.SaveBuffer(b, ""); err != nil {
			// Abort rather than carry on. Whatever stopped this write — a full
			// disk, a read-only mount — will almost certainly stop the next
			// one too, and continuing would let the closing summary report
			// successes that never happened. The buffers not yet reached stay
			// modified and untouched, which is the honest outcome.
			return fmt.Errorf("saving %s: %w", b.Path(), err)
		}
		saved++
	}

	switch {
	case candidates == 0:
		e.Echo("(No files need saving)")
	case saved == 0:
		e.Echo("(No files saved)")
	default:
		e.Echo("Saved %d file%s", saved, plural(saved))
	}
	return nil
}

// --- buffers -------------------------------------------------------------

// switchToBuffer displays a buffer chosen by name, creating it when unknown.
func switchToBuffer(e Env) error {
	name, err := e.ReadString(ReadOpts{
		Prompt:   "Switch to buffer: ",
		Complete: completeBufferName(e),
	})
	if err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	b, ok := e.BufferByName(name)
	if !ok {
		// An unfamiliar name creates the buffer, as in emacs: this is the
		// ordinary way to start a scratch buffer that has no file.
		b = e.NewBuffer(name)
	}
	e.Win().Visit(b)
	return nil
}

// killBuffer kills the current buffer, confirming when it has unsaved changes.
func killBuffer(e Env) error {
	b := e.Buf()
	if b.Modified() {
		c, err := e.ReadChar(
			fmt.Sprintf("Buffer %s modified; kill anyway? (y, n) ", e.BufferName(b)),
			[]rune{'y', 'n'},
		)
		if err != nil {
			return err
		}
		if c != 'y' {
			e.Echo("Buffer not killed")
			return nil
		}
	}
	if err := e.KillBuffer(b); err != nil {
		// Refusing to kill the last buffer is a message, not a malfunction.
		e.Echo("%v", err)
		return nil
	}
	if rest := e.Buffers(); len(rest) > 0 {
		e.Win().Visit(rest[0])
	}
	return nil
}

// listBuffers renders a table of live buffers into *Buffer List* and shows it.
func listBuffers(e Env) error {
	lb, ok := e.BufferByName(bufferListName)
	if !ok {
		lb = e.NewBuffer(bufferListName)
	}

	// Snapshot after ensuring the listing buffer exists, so the listing
	// includes itself on the first run as well as on later ones.
	bufs := e.Buffers()

	width := len("Buffer")
	for _, b := range bufs {
		if n := len(e.BufferName(b)); n > width {
			width = n
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, " M %-*s  File\n", width, "Buffer")
	fmt.Fprintf(&sb, " - %s  %s\n", strings.Repeat("-", width), "----")
	for _, b := range bufs {
		mark := " "
		if b.Modified() {
			mark = "*"
		}
		fmt.Fprintf(&sb, " %s %-*s  %s\n", mark, width, e.BufferName(b), b.Path())
	}

	// Replace rather than append: running list-buffers twice must not stack
	// two tables on top of each other.
	if end := lb.End(); !end.Equal(text.Pos{}) {
		if err := lb.Delete(text.Pos{}, end); err != nil {
			return err
		}
	}
	if err := lb.Insert(text.Pos{}, []rune(sb.String())); err != nil {
		return err
	}
	lb.BreakUndo()
	lb.SetModified(false)

	e.Win().Visit(lb)
	e.Win().Pt = text.Pos{}
	return nil
}

// --- windows -------------------------------------------------------------

func splitWindowBelow(e Env) error { return split(e, false) }
func splitWindowRight(e Env) error { return split(e, true) }

// split reports a refusal through the echo area rather than as an error.
//
// A frame too small to divide is a fact about the terminal, not a fault in the
// command, and surfacing it as an error would read to the user as a crash.
func split(e Env, vertical bool) error {
	if err := e.SplitWindow(vertical); err != nil {
		e.Echo("%v", err)
	}
	return nil
}

func deleteOtherWindows(e Env) error {
	e.DeleteOtherWindows()
	return nil
}

// deleteWindow removes the current window, echoing emacs's refusal when it is
// the only one rather than failing.
func deleteWindow(e Env) error {
	if err := e.DeleteWindow(); err != nil {
		e.Echo("%v", err)
	}
	return nil
}

// otherWindow selects the window ARG positions on; a negative ARG goes back.
func otherWindow(e Env) error {
	n, _ := e.Arg()
	e.OtherWindow(n)
	return nil
}

// --- session -------------------------------------------------------------

// keyboardQuit abandons the current operation and deactivates the mark.
//
// C-g must never be silent — a quiet C-g leaves the user unsure whether the
// editor noticed them at all — so this always reports.
//
// Cancelling a half-typed prefix such as C-x, or an open minibuffer prompt,
// happens in the editor's event loop: those states exist only there, and by
// the time any command runs neither is pending. What remains for this command
// is the buffer-level case, which is the mark.
//
// The region is deactivated and the mark kept, as emacs does: the selection
// ends, point stays exactly where the user left it, and C-x C-x can still return
// to the mark. Clearing the mark instead would throw away a position the user
// set deliberately just to stop it being highlighted.
func keyboardQuit(e Env) error {
	e.Buf().DeactivateMark()
	e.Echo("Quit")
	return nil
}

// saveBuffersKillTerminal offers to save each modified buffer, then exits.
func saveBuffersKillTerminal(e Env) error {
	for _, b := range e.Buffers() {
		if !b.Modified() || b.Path() == "" {
			continue
		}
		c, err := e.ReadChar(
			fmt.Sprintf("Save file %s? (y, n, q) ", b.Path()),
			[]rune{'y', 'n', 'q'},
		)
		if err != nil {
			return err
		}
		switch c {
		case 'q':
			e.Echo("Quit")
			return nil
		case 'y':
			if err := e.SaveBuffer(b, ""); err != nil {
				// Abort before reaching Quit. Exiting after a failed write
				// would drop the user out of the editor having just been told
				// their file could not be saved — losing the very work they
				// asked to protect.
				return fmt.Errorf("saving %s: %w", b.Path(), err)
			}
		}
	}

	// Anything still modified — including a pathless buffer that was skipped
	// above — gets one last confirmation. This is the final guard against
	// discarding the user's work.
	if anyModified(e) {
		c, err := e.ReadChar("Modified buffers exist; exit anyway? (y, n) ", []rune{'y', 'n'})
		if err != nil {
			return err
		}
		if c != 'y' {
			e.Echo("Quit")
			return nil
		}
	}
	return e.Quit(true)
}

func anyModified(e Env) bool {
	for _, b := range e.Buffers() {
		if b.Modified() {
			return true
		}
	}
	return false
}

// --- completion ----------------------------------------------------------

// completeBufferName completes over the display names of live buffers.
func completeBufferName(e Env) CompleteFunc {
	return func(string) []string {
		var out []string
		for _, b := range e.Buffers() {
			out = append(out, e.BufferName(b))
		}
		sort.Strings(out)
		return out
	}
}

// completeFilename completes a path against the directory it names.
//
// Directories come back with a trailing separator so that repeated completion
// walks down a tree rather than stalling at the directory itself.
func completeFilename(prefix string) []string {
	dir, base := filepath.Split(prefix)
	lookIn := dir
	if lookIn == "" {
		lookIn = "."
	}
	entries, err := os.ReadDir(lookIn)
	if err != nil {
		return nil
	}
	// base is deliberately unused for filtering: the directory selects which
	// candidates exist, and the minibuffer's fuzzy ranking narrows them. The
	// split is still needed to know which directory to read.
	_ = base
	var out []string
	for _, ent := range entries {
		full := dir + ent.Name()
		if ent.IsDir() {
			full += string(filepath.Separator)
		}
		out = append(out, full)
	}
	sort.Strings(out)
	return out
}

// isDirCandidate reports whether a completeFilename candidate is a directory,
// which completeFilename marks with a trailing separator. It is the Descend
// hook for the filename prompts, so RET on a directory lists it.
func isDirCandidate(s string) bool {
	return s != "" && os.IsPathSeparator(s[len(s)-1])
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
