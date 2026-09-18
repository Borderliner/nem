package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/text"
)

// --- M-x ------------------------------------------------------------------

func TestExecuteExtendedCommand(t *testing.T) {
	e, scr := newTestEditor(t, "abcdef")
	feed(t, scr, txt("forward-char"), key(t, "RET"))
	press(t, e, "M-x")
	wantPt(t, e, 0, 1)
}

// TAB completes the common prefix, so a partial name reaches the command.
func TestExecuteExtendedCommandCompletesOnTab(t *testing.T) {
	e, scr := newTestEditor(t, "abcdef")
	feed(t, scr, txt("forward-ch"), key(t, "TAB", "RET"))
	press(t, e, "M-x")
	wantPt(t, e, 0, 1)
}

// An unknown name reports rather than failing.
func TestExecuteExtendedCommandUnknownName(t *testing.T) {
	e, scr := newTestEditor(t, "abc")
	feed(t, scr, txt("no-such-command-at-all"), key(t, "RET"))
	press(t, e, "M-x")
	if e.Message() == "" {
		t.Error("an unknown M-x name reported nothing")
	}
	wantPt(t, e, 0, 0)
}

// --- editing inside a prompt ---------------------------------------------

// The prompt is a real buffer in a real window, so the ordinary editing
// commands work inside it with no parallel implementation. If Win did not
// report the prompt's window, each of these would edit the text buffer instead —
// which is the bug this design exists to prevent.
func TestOrdinaryEditingCommandsWorkInsideAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		typed string
		want  string
	}{
		{"C-a then insert", []string{"C-a"}, "bcd", "abcd"},
		{"C-e then insert", []string{"C-e"}, "bcd", "bcda"},
		{"C-k truncates", []string{"C-a", "C-k"}, "bcd", "a"},
		{"backspace shortens", []string{"<backspace>"}, "bcd", "bca"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, scr := newTestEditor(t)
			// Type into the prompt, run the editing keys, type "a", accept.
			feed(t, scr, txt(tc.typed), key(t, tc.keys...), txt("a"), key(t, "RET"))

			var got string
			e.Registry().Register(command.Command{
				Name: "test-prompt", Doc: "t", Interactive: true,
				Fn: func(env command.Env) error {
					s, err := env.ReadString(command.ReadOpts{Prompt: "p: "})
					got = s
					return err
				},
			})
			if err := e.Run("test-prompt"); err != nil {
				t.Fatalf("test-prompt: %v", err)
			}
			if got != tc.want {
				t.Errorf("prompt contents = %q, want %q", got, tc.want)
			}
			// The text buffer must be untouched: the prompt edited itself.
			wantText(t, e, "")
		})
	}
}

// C-g abandons a prompt and reports ErrQuit, which commands propagate.
func TestPromptAbortReportsQuit(t *testing.T) {
	e, scr := newTestEditor(t)
	feed(t, scr, txt("abc"), key(t, "C-g"))

	var err error
	e.Registry().Register(command.Command{
		Name: "test-abort", Doc: "t", Interactive: true,
		Fn: func(env command.Env) error {
			_, err = env.ReadString(command.ReadOpts{Prompt: "p: "})
			return nil
		},
	})
	if runErr := e.Run("test-abort"); runErr != nil {
		t.Fatalf("test-abort: %v", runErr)
	}
	if err != command.ErrQuit {
		t.Errorf("ReadString error = %v, want ErrQuit", err)
	}
}

// The recursion guard stops a runaway from growing the Go stack until the
// process dies.
func TestMinibufferRecursionGuard(t *testing.T) {
	e, _ := newTestEditor(t)
	e.miniDepth = maxMiniDepth
	if _, err := e.ReadString(command.ReadOpts{Prompt: "p: "}); err != ErrTooDeep {
		t.Errorf("ReadString at max depth = %v, want ErrTooDeep", err)
	}
}

// OnChange fires on every change, including shortening. A grow-only
// notification cannot express backspace, and incremental search depends on it.
func TestOnChangeSeesShortening(t *testing.T) {
	e, scr := newTestEditor(t)
	feed(t, scr, txt("abc"), key(t, "<backspace>", "<backspace>", "RET"))

	var seen []string
	e.Registry().Register(command.Command{
		Name: "test-onchange", Doc: "t", Interactive: true,
		Fn: func(env command.Env) error {
			_, err := env.ReadString(command.ReadOpts{
				Prompt:   "p: ",
				OnChange: func(s string) { seen = append(seen, s) },
			})
			return err
		},
	})
	if err := e.Run("test-onchange"); err != nil {
		t.Fatalf("test-onchange: %v", err)
	}
	want := []string{"a", "ab", "abc", "ab", "a"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("OnChange saw %v, want %v", seen, want)
	}
}

// --- incremental search ---------------------------------------------------

// Point tracks the match as the pattern grows, and RET leaves it there.
func TestIsearchMovesPointAsYouType(t *testing.T) {
	e, scr := newTestEditor(t, "alpha beta gamma")
	feed(t, scr, txt("beta"), key(t, "RET"))
	press(t, e, "C-s")
	// Forward search leaves point after the match: "alpha beta" is 10 runes.
	wantPt(t, e, 0, 10)
}

// Shortening the pattern walks point back toward the origin rather than
// leaving it stranded at a match the shorter pattern no longer justifies.
func TestIsearchBackspaceWalksPointBack(t *testing.T) {
	e, scr := newTestEditor(t, "alpha beta gamma")
	feed(t, scr, txt("beta"), key(t, "<backspace>", "<backspace>", "<backspace>", "<backspace>", "RET"))
	press(t, e, "C-s")
	// An empty pattern puts point back at the origin.
	wantPt(t, e, 0, 0)
}

// C-g restores the point saved when the search opened. This is the behaviour
// users rely on most and the easiest to lose.
func TestIsearchQuitRestoresOrigin(t *testing.T) {
	e, scr := newTestEditor(t, "alpha beta gamma")
	e.Active().Pt = text.Pos{Line: 0, Col: 3}
	feed(t, scr, txt("gamma"), key(t, "C-g"))
	press(t, e, "C-s")
	wantPt(t, e, 0, 3)
}

// A repeated C-s inside the prompt advances to the next match. This is why the
// Isearch session has to be reachable from the editor at all.
func TestIsearchAdvancesOnRepeatedCtrlS(t *testing.T) {
	e, scr := newTestEditor(t, "xx ab cd ab ef")
	feed(t, scr, txt("ab"), key(t, "C-s", "RET"))
	press(t, e, "C-s")
	// First match ends at 5; C-s advances to the second, which ends at 11.
	wantPt(t, e, 0, 11)
}

// A failing pattern reports and leaves point at the last good match rather
// than jumping somewhere arbitrary.
func TestIsearchFailingPatternReports(t *testing.T) {
	e, scr := newTestEditor(t, "alpha beta")
	feed(t, scr, txt("zzz"), key(t, "RET"))
	press(t, e, "C-s")
	wantEcho(t, e, "Failing I-search")
}

// --- find-file ------------------------------------------------------------

func TestFindFileOpensAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("from disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e, scr := newTestEditor(t)
	feed(t, scr, txt(path), key(t, "RET"))
	press(t, e, "C-x", "C-f")

	if got := e.Buf().String(); !strings.Contains(got, "from disk") {
		t.Errorf("buffer = %q, want it to contain the file's text", got)
	}
	if got := e.BufferName(e.Buf()); got != "hello.txt" {
		t.Errorf("buffer name = %q, want %q", got, "hello.txt")
	}
}

// find-file on a path that does not exist opens an empty buffer carrying it,
// which is how a new file gets created.
func TestFindFileOnMissingPathOpensEmptyBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brand-new.txt")

	e, scr := newTestEditor(t)
	feed(t, scr, txt(path), key(t, "RET"))
	press(t, e, "C-x", "C-f")

	wantText(t, e, "")
	if got := e.Buf().Path(); got != path {
		t.Errorf("buffer path = %q, want %q", got, path)
	}
}

// Abandoning the prompt must leave the editor exactly as it was.
func TestFindFileQuitChangesNothing(t *testing.T) {
	e, scr := newTestEditor(t, "original")
	before := e.Buf()
	feed(t, scr, txt("/tmp/whatever"), key(t, "C-g"))
	press(t, e, "C-x", "C-f")

	if e.Buf() != before {
		t.Error("C-g at the find-file prompt changed the visited buffer")
	}
	wantText(t, e, "original")
}
