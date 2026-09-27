//go:build unix

package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// pump hands the events posted to scr to the editor until done says so, as
// the event loop would: how a test waits for a command running in the
// background.
func pump(t *testing.T, e *Editor, scr tcell.SimulationScreen, done func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !done() {
		evs := make(chan tcell.Event, 1)
		go func() { evs <- scr.PollEvent() }()
		select {
		case ev := <-evs:
			if ev == nil {
				t.Fatal("the screen closed while waiting")
			}
			e.HandleEvent(ev)
		case <-deadline:
			t.Fatal("timed out waiting for the command")
		}
	}
}

// compiled runs a compilation of line in dir and waits for it to end.
func compiled(t *testing.T, e *Editor, scr tcell.SimulationScreen, line, dir string) (*text.Buffer, *compileState) {
	t.Helper()
	if err := e.runCompile(line, dir, false); err != nil {
		t.Fatal(err)
	}
	b := e.byName[compilationName]
	pump(t, e, scr, func() bool { return !e.compileOf(b).running })
	return b, e.compileOf(b)
}

// A compilation's output appears beside the window being worked in, which
// stays selected; the lines naming files lead there, and how it ended is
// said at the end and in the echo area.
func TestCompileShowsOutputAndErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\tx := 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, scr := newTestEditor(t)
	start := e.active

	b, st := compiled(t, e, scr, `printf '\033[31mmain.go:2:3: undefined: x\033[0m\nall done\n'; exit 2`, dir)
	body := b.String()
	for _, want := range []string{"main.go:2:3: undefined: x", "all done", "── exited with code 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("output lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "\x1b") {
		t.Error("colour escapes reached the buffer")
	}
	if e.active != start || len(e.tree.Windows()) != 2 {
		t.Error("the output did not open beside the window being worked in")
	}
	if len(st.locs) != 1 || st.locs[0].path != filepath.Join(dir, "main.go") || st.locs[0].ln != 1 || st.locs[0].col != 2 {
		t.Fatalf("locations %+v", st.locs)
	}
	wantEcho(t, e, "Compilation exited with code 2")
	wantEcho(t, e, "1 error")

	press(t, e, "M-g", "n")
	wantVisiting(t, e, filepath.Join(dir, "main.go"))
	wantPt(t, e, 1, 2)
}

// A test's failure names its file without the directory; the one file of
// the project it can be is found, and where several can, the user is asked
// on the way there.
func TestCompileFindsBareNames(t *testing.T) {
	root := aProject(t, map[string]string{
		"pkg/a_test.go": "x\ny\nz\n", "one/b_test.go": "1\n2\n", "two/b_test.go": "1\n2\n",
	})
	e, scr := newTestEditor(t)
	_, st := compiled(t, e, scr, `printf '    a_test.go:3: bad\n    b_test.go:2: worse\n'`, root)
	if len(st.locs) != 2 {
		t.Fatalf("locations %+v", st.locs)
	}
	if st.locs[0].path != filepath.Join(root, "pkg", "a_test.go") {
		t.Errorf("a_test.go resolved to %q", st.locs[0].path)
	}
	if st.locs[1].path != "" {
		t.Errorf("b_test.go, which two files could be, resolved to %q", st.locs[1].path)
	}

	press(t, e, "M-g", "n")
	wantVisiting(t, e, filepath.Join(root, "pkg", "a_test.go"))
	feed(t, scr, txt("two"), key(t, "RET"))
	press(t, e, "M-g", "n")
	wantVisiting(t, e, filepath.Join(root, "two", "b_test.go"))
	wantPt(t, e, 1, 0)
}

// In the output, RET goes to the error on its line, n and p step through
// them, g runs it again and C-c C-k stops it.
func TestCompileOutputKeys(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.c", "b.c")
	e, scr := newTestEditor(t)
	b, st := compiled(t, e, scr, `printf 'a.c:1:1: error: one\nnoise\nb.c:1:1: warning: two\n'`, dir)
	e.OtherWindow(1)
	if e.active.Buf != b {
		t.Fatal("the other window is not the output")
	}

	e.active.Pt = text.Pos{Line: st.locs[1].line}
	press(t, e, "RET")
	wantVisiting(t, e, filepath.Join(dir, "b.c"))

	e.OtherWindow(1)
	e.active.Pt = text.Pos{}
	press(t, e, "n")
	if e.active.Buf != b || st.cur != 0 {
		t.Errorf("n: in the output %v, at error %d; want to stay, showing the first", e.active.Buf == b, st.cur)
	}
	press(t, e, "n")
	if st.cur != 1 {
		t.Errorf("second n: at error %d, want the second", st.cur)
	}

	press(t, e, "g")
	again := e.compileOf(b)
	if again == st || !again.running {
		t.Fatal("g did not run the command again")
	}
	pump(t, e, scr, func() bool { return !again.running })

	if err := e.runCompile("sleep 30", dir, false); err != nil {
		t.Fatal(err)
	}
	slow := e.compileOf(b)
	e.active.Visit(b)
	press(t, e, "C-c", "C-k")
	pump(t, e, scr, func() bool { return !slow.running })
	if !slow.killed || !strings.Contains(b.String(), "── killed") {
		t.Errorf("C-c C-k: killed %v, output:\n%s", slow.killed, b.String())
	}
}

// The prompt offers what the project's build files say builds it, and then
// the command last run there.
func TestCompilePromptOffersACommand(t *testing.T) {
	root := aProject(t, map[string]string{"go.mod": "module x\n", "main.go": ""})
	if got := guessCompileCommand(root); got != "go build ./..." {
		t.Errorf("guess for a Go module = %q", got)
	}
	e, scr := newTestEditor(t)
	visiting(t, e, filepath.Join(root, "main.go"))

	feed(t, scr, key(t, "C-a", "C-k"), txt("echo built"), key(t, "RET"))
	press(t, e, "C-x", "p", "c")
	b := e.byName[compilationName]
	pump(t, e, scr, func() bool { return !e.compileOf(b).running })
	if got := e.compileCommands[root]; got != "echo built" {
		t.Errorf("remembered %q for the project", got)
	}
	if !strings.Contains(b.String(), "built") {
		t.Errorf("output:\n%s", b.String())
	}
}

// M-! shows a line of output in the echo area and more in a buffer; with C-u
// it goes in at point.
func TestShellCommand(t *testing.T) {
	e, scr := newTestEditor(t, "text")

	feed(t, scr, txt("echo hello"), key(t, "RET"))
	press(t, e, "M-!")
	wantEcho(t, e, "hello")

	feed(t, scr, txt("printf 'a\\nb\\nc\\n'"), key(t, "RET"))
	press(t, e, "M-!")
	out, ok := e.byName[shellOutputName]
	if !ok || out.String() != "a\nb\nc" {
		t.Fatalf("output buffer: %v, %q", ok, out.String())
	}

	feed(t, scr, txt("printf ' more'"), key(t, "RET"))
	e.active.Pt = text.Pos{Line: 0, Col: 4}
	press(t, e, "C-u", "M-!")
	wantText(t, e, "text more")

	feed(t, scr, txt("echo oops; exit 3"), key(t, "RET"))
	press(t, e, "M-!")
	wantEcho(t, e, "code 3")
}

// C-u M-| puts the region through a command in place; a command that fails
// leaves the region alone.
func TestShellCommandOnRegion(t *testing.T) {
	e, scr := newTestEditor(t, "pear", "apple", "fig")
	press(t, e, "C-x", "h")

	feed(t, scr, txt("sort"), key(t, "RET"))
	press(t, e, "C-u", "M-|")
	wantText(t, e, "apple\nfig\npear")

	press(t, e, "C-x", "h")
	feed(t, scr, txt("false"), key(t, "RET"))
	press(t, e, "C-u", "M-|")
	wantText(t, e, "apple\nfig\npear")
	wantEcho(t, e, "unchanged")
}

// M-& runs in the background into its own buffer, not the compilation M-g n
// steps through.
func TestAsyncShellCommand(t *testing.T) {
	e, scr := newTestEditor(t)
	feed(t, scr, txt("echo in the background"), key(t, "RET"))
	press(t, e, "M-&")
	b := e.byName[asyncShellName]
	st := e.compileOf(b)
	if st == nil {
		t.Fatal("no output buffer")
	}
	pump(t, e, scr, func() bool { return !st.running })
	if !strings.Contains(b.String(), "in the background") {
		t.Errorf("output:\n%s", b.String())
	}
	if e.nextErrorBuf == b {
		t.Error("M-& became what M-g n steps through")
	}
}

// A shell command that changes files leaves the buffers visiting them - those
// without edits of their own - reading as the files now do.
func TestShellCommandRevertsWhatItChanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, scr := newTestEditor(t)
	b := visiting(t, e, p)

	feed(t, scr, txt("printf 'new text\\n' > f.txt"), key(t, "RET"))
	press(t, e, "M-!")
	if b.String() != "new text" || b.Modified() {
		t.Errorf("the buffer reads %q, modified %v", b.String(), b.Modified())
	}
	wantEcho(t, e, "Reverted f.txt")
}
