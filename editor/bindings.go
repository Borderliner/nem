package editor

import (
	"fmt"

	"github.com/Borderliner/nem/keymap"
)

// defaultBindings is the built-in keymap, as documented in
// docs/design/2026-09-18-nem-commands.md. Each entry maps an emacs
// key sequence to a command name; the names are resolved against the command
// registry at dispatch, so this table has no compile-time dependency on the
// command package.
//
// C-u is deliberately absent: the universal argument is handled by the event
// loop before keymap lookup, not as an ordinary command.
var defaultBindings = []struct{ Spec, Command string }{
	// Motion.
	{"C-f", "forward-char"}, {"<right>", "forward-char"},
	{"C-b", "backward-char"}, {"<left>", "backward-char"},
	{"C-n", "next-line"}, {"<down>", "next-line"},
	{"C-p", "previous-line"}, {"<up>", "previous-line"},
	{"M-f", "forward-word"},
	{"M-b", "backward-word"},
	{"C-a", "move-beginning-of-line"}, {"<home>", "move-beginning-of-line"},
	{"C-e", "move-end-of-line"}, {"<end>", "move-end-of-line"},
	{"M-<", "beginning-of-buffer"},
	{"M->", "end-of-buffer"},
	{"C-v", "scroll-up-command"}, {"<pgdn>", "scroll-up-command"},
	{"M-v", "scroll-down-command"}, {"<pgup>", "scroll-down-command"},
	{"M-g M-g", "goto-line"},
	{"C-l", "recenter-top-bottom"},

	// Editing.
	{"C-d", "delete-char"}, {"<delete>", "delete-char"},
	{"<backspace>", "delete-backward-char"},
	{"M-d", "kill-word"},
	{"M-<backspace>", "backward-kill-word"},
	{"C-k", "kill-line"},
	{"C-o", "open-line"},
	{"C-t", "transpose-chars"},
	{"M-t", "transpose-words"},
	{"RET", "newline"},
	{"TAB", "indent-for-tab-command"},
	{"M-u", "upcase-word"},
	{"M-l", "downcase-word"},
	{"M-c", "capitalize-word"},
	{"M-;", "comment-dwim"},
	{"M-/", "dabbrev-expand"},
	{"M-SPC", "just-one-space"},
	{`M-\`, "delete-horizontal-space"},
	{"M-^", "delete-indentation"},
	{"M-m", "back-to-indentation"},
	{"M-z", "zap-to-char"},
	{"M-{", "backward-paragraph"},
	{"M-}", "forward-paragraph"},
	{"M-q", "fill-paragraph"},
	{"C-M-f", "forward-sexp"},
	{"C-M-b", "backward-sexp"},
	{"C-M-k", "kill-sexp"},
	{"M-=", "count-words"},
	{"C-x =", "what-cursor-position"},

	// Keyboard macros.
	{"<f3>", "kmacro-start-macro-or-insert-counter"},
	{"<f4>", "kmacro-end-or-call-macro"},
	{"C-x (", "kmacro-start-macro"},
	{"C-x )", "kmacro-end-macro"},
	{"C-x e", "kmacro-end-and-call-macro"},

	// Line movement. Not an emacs binding: emacs has no native line move, and
	// M-<up>/M-<down> is the convention every other editor uses.
	{"M-<up>", "move-lines-up"},
	{"M-<down>", "move-lines-down"},

	// Mark, region, kill ring.
	{"C-SPC", "set-mark-command"},
	{"C-x C-x", "exchange-point-and-mark"},
	{"C-x h", "mark-whole-buffer"},
	{"C-w", "kill-region"},
	{"M-w", "kill-ring-save"},
	{"C-y", "yank"},
	{"M-y", "yank-pop"},

	// Undo. C-/ arrives as C-_ on most terminals; both are listed because the
	// binding is stored normalized, so they collapse to the same entry.
	{"C-_", "undo"},
	{"C-/", "undo"},
	{"C-x u", "undo"},
	{"M-_", "redo"},

	// Search and replace.
	{"C-s", "isearch-forward"},
	{"C-r", "isearch-backward"},
	{"M-%", "query-replace"},

	// Files.
	{"C-x C-f", "find-file"},
	{"C-x C-r", "recentf-open"},
	{"C-x C-s", "save-buffer"},
	{"C-x C-w", "write-file"},
	{"C-x s", "save-some-buffers"},

	// Buffers.
	{"C-x b", "switch-to-buffer"},
	{"C-x k", "kill-buffer"},
	{"C-x C-b", "list-buffers"},
	{"C-x d", "dired"},
	{"C-x C-j", "dired-jump"},

	// Projects: project.el's keys, with projectile's commands. See project.go.
	{"C-x p f", "project-find-file"},
	{"C-x p p", "project-switch-project"},
	{"C-x p b", "project-switch-to-buffer"},
	{"C-x p d", "project-find-dir"},
	{"C-x p D", "project-dired"},
	{"C-x p g", "project-find-regexp"},
	{"C-x p r", "project-query-replace"},
	{"C-x p e", "project-recentf"},
	{"C-x p t", "project-toggle-test"},
	{"C-x p k", "project-kill-buffers"},
	{"C-x p S", "project-save-buffers"},
	{"M-g n", "next-error"},
	{"M-g p", "previous-error"},
	{"C-x `", "next-error"},

	// Windows.
	{"C-x 2", "split-window-below"},
	{"C-x 3", "split-window-right"},
	{"C-x 1", "delete-other-windows"},
	{"C-x 0", "delete-window"},
	{"C-x o", "other-window"},

	// Display. C-x n is free in nem: emacs uses it for narrowing, which nem does
	// not have. C-c is deliberately not used - that prefix belongs to the user,
	// and examples/init.lua already hands it out.
	{"C-x n", "toggle-line-numbers"},

	// Session.
	{"C-g", "keyboard-quit"},
	{"C-x C-c", "save-buffers-kill-terminal"},
	{"M-x", "execute-extended-command"},

	// Help. <f1> is the primary prefix, not C-h: tcell's legacy input path
	// cannot distinguish C-h from Backspace, so C-h works only on terminals
	// that negotiate CSI-u. See the spec's "C-h is not reliably available".
	{"<f1> b", "describe-bindings"},
	{"<f1> k", "describe-key"},
	{"C-h b", "describe-bindings"},
	{"C-h k", "describe-key"},
}

// bindSpec parses an emacs key spec and binds it.
//
// It does NOT normalize the sequence first: keymap.Map normalizes on both Bind
// and Lookup, so a binding matches however the terminal encodes the keystroke —
// C-SPC arriving as NUL and C-/ arriving as C-_ both resolve. Normalizing here
// as well would be a second source of truth for the same rule, and dead code
// that no test could fail on.
func bindSpec(m *keymap.Map, spec, command string) error {
	seq, err := keymap.ParseSpec(spec)
	if err != nil {
		return fmt.Errorf("binding %q to %s: %w", spec, command, err)
	}
	if err := m.Bind(seq, command); err != nil {
		return fmt.Errorf("binding %q to %s: %w", spec, command, err)
	}
	return nil
}

// InstallDefaultBindings populates m with nem's built-in keymap. It is called
// before the Lua config loads, so user bindings override these.
func InstallDefaultBindings(m *keymap.Map) error {
	for _, b := range defaultBindings {
		if err := bindSpec(m, b.Spec, b.Command); err != nil {
			return err
		}
	}
	return nil
}
