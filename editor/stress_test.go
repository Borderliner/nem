package editor

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Borderliner/nem/backup"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Random keystrokes, many of them, looking for what no test written by hand
// thought of: a panic, a hang, an edit undo cannot take back. Run this way,
// nem was found typing into a file from a prompt, crashing after a search,
// playing a keyboard macro inside itself forever and freezing on a long
// yank into a prompt - each within minutes.
//
// Each run is seeded, and a failure names its seed and the keys before it,
// so it can be run again: NEM_STRESS_SEED picks the first seed.
// NEM_STRESS_SEEDS and NEM_STRESS_STEPS make a run longer than the few
// hundred keystrokes every test run makes; CI's stress job sets them.

// stressSize reads the run's size from the environment, with defaults small
// enough for every test run.
func stressSize(seeds, steps int) (first, n, length int) {
	env := func(name string, def int) int {
		if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
			return v
		}
		return def
	}
	first, _ = strconv.Atoi(os.Getenv("NEM_STRESS_SEED"))
	return first, env("NEM_STRESS_SEEDS", seeds), env("NEM_STRESS_STEPS", steps)
}

// stressKeys are keys that never open a prompt: motion, editing, the region,
// the kill ring, undo, windows, macros and the prefix argument. "|" joins
// the keys of one sequence.
var stressKeys = strings.Fields(`C-f C-b C-n C-p M-f M-b C-a C-e M-< M-> C-v M-v C-l C-d <delete>
<backspace> M-d M-<backspace> C-k C-o C-t M-t RET TAB M-u M-l M-c M-; M-/ M-SPC M-^ M-m M-{ M-}
M-q C-M-f C-M-b C-M-k <backtab> M-<up> M-<down> C-SPC C-SPC C-x|C-x C-x|h C-w M-w C-y C-y M-y
C-/ C-/ M-_ C-g <left> <right> <up> <down> <home> <end> <pgup> <pgdn> C-x|2 C-x|3 C-x|o C-x|0
C-x|1 C-x|C-u C-x|C-l <f3> <f4> C-u C-u|3 M-= C-x|=`)

// stressPromptKeys open prompts and answer them: searches, query-replace,
// goto-line, switching and killing buffers, occur, help, macros that replay
// them. Nothing here reads or writes a file other than the one being edited,
// or runs a command: typed at random, M-x or M-! could do anything.
var stressPromptKeys = strings.Fields(`C-s C-s C-r M-% M-z M-g|M-g M-g|i C-x|b C-x|k C-x|C-b M-s|o
C-x|TAB C-x|( C-x|) C-x|e C-h|k C-h|b C-x|n C-x|x|g C-x|C-s RET RET C-g C-g y n ! q . M-g|n 5 0`)

// stressText is typed between the keys: brackets and quotes for automatic
// pairs, and text from every corner of the layout code - right to left,
// wide, combining, joined emoji, a zero-width non-joiner.
var stressText = []string{"a", "b", "x", " ", " ", "(", ")", "\"", "{", "}", ";", "س", "ل", "ا", "م",
	"日", "é", "́", "👍", "👩‍💻", "‌", "1", "۱", "-", "/", "*", "#"}

// stressFile is what is being edited: a Go file, so it is coloured, with a
// little of everything.
var stressFile = strings.Join([]string{
	"package main", "", "func main() {", "\tfmt.Println(\"سلام دنیا\")", "}", "",
	"// 日本語 comment with é and é and 👩‍💻", "\t\tindented\ttabs", strings.Repeat("long ", 60),
	"(a (b c) [d {e}])", "این یک متن فارسی است با English در میان.", "",
}, "\n")

// stressOpen writes stressFile into dir and opens it.
func stressOpen(t *testing.T, e *Editor, dir string) *text.Buffer {
	t.Helper()
	path := filepath.Join(dir, "stress.go")
	if err := os.WriteFile(path, []byte(stressFile), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := e.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	e.active.Visit(b)
	return b
}

// faultReport is the first report of a bug nem caught in itself, or "". The
// guards turn a panic into a report and carry on; for this test, that is
// still a failure.
func faultReport(e *Editor) string {
	dir := filepath.Join(e.BackupRoot(), backup.Faults)
	ents, _ := os.ReadDir(dir)
	if len(ents) == 0 {
		return ""
	}
	report, _ := os.ReadFile(filepath.Join(dir, ents[0].Name()))
	return string(report)
}

// lastKeys is the end of a run's keys, for a failure to show.
func lastKeys(keys []string) []string { return keys[max(0, len(keys)-30):] }

// Random keys never crash nem, never leave point outside its buffer, and
// never make an edit undo cannot take back: undoing all the way returns the
// file as it was opened.
func TestRandomKeysNeverCrashOrDefeatUndo(t *testing.T) {
	first, seeds, steps := stressSize(6, 300)
	for seed := first; seed < first+seeds; seed++ {
		if msg := stressDirect(t, uint64(seed), steps); msg != "" {
			t.Fatalf("seed %d: %s", seed, msg)
		}
	}
}

// stressDirect runs one seed a key at a time, and reports what went wrong.
func stressDirect(t *testing.T, seed uint64, steps int) (msg string) {
	rng := rand.New(rand.NewPCG(seed, 7))
	e, scr := newTestEditor(t)
	b := stressOpen(t, e, t.TempDir())
	orig := b.String()
	var keys []string
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("panic: %v\nafter %q\n%s", r, lastKeys(keys), debug.Stack())
		}
	}()
	for range steps {
		if rng.IntN(3) == 0 {
			s := stressText[rng.IntN(len(stressText))]
			keys = append(keys, s)
			for _, r := range s {
				e.HandleKey(keymap.Key{Rune: r})
			}
		} else {
			s := strings.ReplaceAll(stressKeys[rng.IntN(len(stressKeys))], "|", " ")
			keys = append(keys, s)
			for _, k := range specKeys(t, s) {
				e.HandleKey(k)
			}
		}
		if rng.IntN(40) == 0 {
			w, h := 1+rng.IntN(100), 1+rng.IntN(40)
			scr.SetSize(w, h)
			keys = append(keys, fmt.Sprintf("<resize %dx%d>", w, h))
		}
		e.Redraw()
		if w := e.Win(); w.Buf.ClampPos(w.Pt) != w.Pt {
			return fmt.Sprintf("point %v is outside its buffer, after %q", w.Pt, lastKeys(keys))
		}
		if report := faultReport(e); report != "" {
			return fmt.Sprintf("%s\nafter %q", report, lastKeys(keys))
		}
	}

	press(t, e, "C-g", "C-g")
	if e.Win().Buf != b {
		return "" // the file is in no window to undo in
	}
	for i := 0; i < 3*steps && b.String() != orig; i++ {
		press(t, e, "C-/")
	}
	if got := b.String(); got != orig {
		return fmt.Sprintf("undoing everything left %q, after %q", got[:min(len(got), 200)], lastKeys(keys))
	}
	return ""
}

// Random keys through the event loop, prompts and all, never crash nem or
// stop it reading keys.
func TestRandomKeysThroughTheLoopNeverCrashOrHang(t *testing.T) {
	first, seeds, steps := stressSize(2, 300)
	// Nothing typed at random may reach anyone's files: a buffer saved
	// under a random name lands here.
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", dir)
	for seed := first; seed < first+seeds; seed++ {
		stressLoop(t, uint64(seed), steps, dir)
	}
}

// stressLoop runs one seed through the event loop.
func stressLoop(t *testing.T, seed uint64, steps int, dir string) {
	rng := rand.New(rand.NewPCG(seed, 11))
	e, scr := newTestEditor(t)
	stressOpen(t, e, dir)
	// The run ends as a closed terminal ends a session: from inside the
	// loop, which finalizing the screen under it would not be.
	sigs := make(chan os.Signal, 1)
	e.SetSignals(sigs)
	// A panic that got past the editor's guards, which would have ended
	// the session; one they caught is a report among the faults.
	caught := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				caught <- fmt.Sprintf("panic: %v\n%s", r, debug.Stack())
			}
			close(done)
		}()
		done <- e.Loop()
	}()
	var keys []string
	fail := func(what string) {
		t.Helper()
		t.Fatalf("seed %d: %s\nafter %q", seed, what, lastKeys(keys))
	}
	for range steps {
		var s string
		var evs []injected
		switch n := rng.IntN(10); {
		case n < 3:
			s = stressText[rng.IntN(len(stressText))]
			evs = txt(s)
		case n < 5:
			s = strings.ReplaceAll(stressPromptKeys[rng.IntN(len(stressPromptKeys))], "|", " ")
			evs = key(t, s)
		default:
			s = strings.ReplaceAll(stressKeys[rng.IntN(len(stressKeys))], "|", " ")
			// C-/ and C-SPC have no Ctrl code of their own to inject; a
			// terminal sends C-_ and NUL for them.
			if s == "C-SPC" {
				evs = []injected{{tcell.KeyNUL, 0, tcell.ModCtrl}}
			} else {
				evs = key(t, strings.ReplaceAll(s, "C-/", "C-_"))
			}
		}
		keys = append(keys, s)
		for _, ev := range evs {
			if !injectWithin(scr, ev, done, 10*time.Second) {
				fail("nem stopped reading keys:\n" + editorStacks())
			}
		}
		select {
		case msg := <-caught:
			fail(msg)
		default:
		}
		if report := faultReport(e); report != "" {
			fail(report)
		}
	}
	sigs <- os.Interrupt
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fail("the loop did not end on a signal:\n" + editorStacks())
	}
	select {
	case msg := <-caught:
		fail(msg)
	default:
	}
}

// injectWithin injects ev, reporting false if nem has not taken it within d.
// The loop ending takes nothing more, and is not a hang.
func injectWithin(scr tcell.SimulationScreen, ev injected, done <-chan error, d time.Duration) bool {
	taken := make(chan struct{})
	go func() {
		scr.InjectKey(ev.key, ev.r, ev.mod)
		close(taken)
	}()
	select {
	case <-taken:
		return true
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// editorStacks is the stacks of the goroutines in the editor, to show where
// a hang is.
func editorStacks() string {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "nem/editor.(*Editor)") {
			out = append(out, g)
		}
	}
	return strings.Join(out, "\n\n")
}
