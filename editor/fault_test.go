package editor

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Borderliner/nem/backup"
	"github.com/Borderliner/nem/command"
	"github.com/gdamore/tcell/v2"
)

// kept lists the files in the store's directory for kind.
func kept(t *testing.T, store *backup.Store, kind string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(store.Root(), kind))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, filepath.Join(store.Root(), kind, e.Name()))
	}
	return out
}

// wantFileHolds checks the file at path holds want somewhere in it.
func wantFileHolds(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if !strings.Contains(string(got), want) {
		t.Errorf("%s holds %q, want %q in it", path, got, want)
	}
}

// An emergency save writes every unsaved buffer somewhere it is found again:
// a file's to its autosave, a buffer with no file to one of its own. A
// buffer with nothing unsaved is left alone.
func TestEmergencySaveKeepsEveryUnsavedBuffer(t *testing.T) {
	e, store, file := safeEditor(t)
	edit(t, visit(t, e, file), "unsaved in a file")
	scratch, _ := e.BufferByName("*scratch*")
	edit(t, scratch, "unsaved in scratch")
	e.NewBuffer("untouched")

	saved, err := e.EmergencySave()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 {
		t.Fatalf("saved %q, want the file's autosave and scratch", saved)
	}
	wantFileHolds(t, store.AutosavePath(file), "unsaved in a file")
	rescued := kept(t, store, backup.Rescued)
	if len(rescued) != 1 {
		t.Fatalf("rescued %q, want scratch alone", rescued)
	}
	wantFileHolds(t, rescued[0], "unsaved in scratch")
}

// runLoop runs the event loop until it returns, and returns what it did.
func runLoop(t *testing.T, e *Editor) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- e.Loop() }()
	return done
}

// The terminal closing ends the session without a word - there is nobody to
// ask - but with every unsaved buffer written away first. It used to kill
// nem where it stood, with the work in it; so it does from inside a prompt.
func TestASignalEndsTheSessionWithTheWorkSaved(t *testing.T) {
	for _, prompt := range []bool{false, true} {
		e, store, file := safeEditor(t)
		scr := e.scr.(tcell.SimulationScreen)
		edit(t, visit(t, e, file), "typed, never saved")
		sigs := make(chan os.Signal, 1)
		e.SetSignals(sigs)

		// The hook runs as find-file starts, just before its prompt opens:
		// the signal arrives with the prompt waiting for a file name.
		opening := make(chan struct{})
		e.BeforeCommand("find-file", func() { close(opening) })
		done := runLoop(t, e)
		if prompt {
			feed(t, scr, key(t, "C-x", "C-f"))
			<-opening
		}
		sigs <- syscall.SIGHUP
		select {
		case err := <-done:
			se, ok := err.(*SignalError)
			if !ok || se.Signal != syscall.SIGHUP || len(se.Saved) != 1 {
				t.Fatalf("prompt open %v: the loop returned %v, want the hang-up with one buffer saved", prompt, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("prompt open %v: the loop is still running after the hang-up", prompt)
		}
		wantFileHolds(t, store.AutosavePath(file), "typed, never saved")
	}
}

// A bug in a command is caught: the command fails with it, the unsaved work
// is autosaved, a report is written, and the editor carries on. It used to
// end the session.
func TestABugInACommandIsCaughtAndReported(t *testing.T) {
	e, store, file := safeEditor(t)
	b := visit(t, e, file)
	edit(t, b, "work")
	if err := e.reg.Register(command.Command{Name: "explode", Fn: func(command.Env) error {
		var m map[string]int
		m["x"] = 1
		return nil
	}}); err != nil {
		t.Fatal(err)
	}

	e.dispatchReporting("explode")
	wantEcho(t, e, "nem bug in explode: assignment to entry in nil map")
	wantEcho(t, e, "unsaved work autosaved")
	wantFileHolds(t, store.AutosavePath(file), "work")
	reports := kept(t, store, backup.Faults)
	if len(reports) != 1 {
		t.Fatalf("reports %q, want one", reports)
	}
	wantEcho(t, e, reports[0])
	wantFileHolds(t, reports[0], "editor.TestABugInACommandIsCaughtAndReported")

	press(t, e, "M->", "!")
	if !strings.HasSuffix(b.String(), "work!") {
		t.Errorf("after the bug, typing gave %q", b.String())
	}
}

// A bug in a hook run with a command is caught the same way.
func TestABugInAHookIsCaught(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	e.BeforeCommand("forward-char", func() { panic("hook went wrong") })
	press(t, e, "C-f")
	wantEcho(t, e, "nem bug in forward-char: hook went wrong")
	press(t, e, "C-e")
	wantPt(t, e, 0, 3)
}

// Outside any command - drawing, a timer - a bug is caught by the event
// loop's guard, and reports stop after a few, so one in drawing does not
// write a report every frame.
func TestTheLoopGuardCatchesBugsAndCapsReports(t *testing.T) {
	e, store, _ := safeEditor(t)
	for range maxFaultReports + 3 {
		e.guard("drawing", func() { panic("cannot draw") })
	}
	wantEcho(t, e, "nem bug in drawing: cannot draw")
	if got := len(kept(t, store, backup.Faults)); got != maxFaultReports {
		t.Errorf("%d reports written, want %d", got, maxFaultReports)
	}
}
