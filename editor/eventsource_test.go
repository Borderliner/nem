package editor

import (
	"testing"
	"time"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// These tests drive the REAL Loop instead of injecting into HandleKey, because
// that is the only way this class of bug is visible. Loop reads events from a
// tcell goroutine; a nested prompt loop calling PollEvent instead would compete
// with it for the same keyboard, and the goroutine - already blocked in a read -
// wins. The first keystroke after a prompt opened was therefore swallowed and
// surfaced only once the prompt closed.
//
// Nothing is observed while the loop runs. The editor is single-consumer by
// design, and tcell's SimulationScreen.GetContents returns the live cell array
// after releasing its lock, so reading either from a second goroutine is a race
// in the test. Every assertion happens after Loop has returned, which is the
// synchronisation edge.

// runKeys feeds keys to a real editor loop and returns once the loop has
// exited, so the editor can then be inspected safely.
func runKeys(t *testing.T, seed string, keys func(scr tcell.SimulationScreen)) *Editor {
	t.Helper()

	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(80, 24)
	defer scr.Fini()

	e, err := New(scr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.SetBackupRoot(t.TempDir())
	e.SetWhichKeyDelay(0)
	if seed != "" {
		if err := e.Buf().Insert(text.Pos{}, []rune(seed)); err != nil {
			t.Fatalf("seed: %v", err)
		}
		e.Buf().BreakUndo()
		e.Buf().SetModified(false)
		e.Active().Pt = text.Pos{}
	}

	done := make(chan struct{})
	go func() { _ = e.Loop(); close(done) }()

	keys(scr)

	// C-g closes any prompt still open, then C-x C-c leaves.
	scr.InjectKey(tcell.KeyCtrlG, 0, tcell.ModCtrl)
	time.Sleep(40 * time.Millisecond)
	scr.InjectKey(tcell.KeyCtrlX, 0, tcell.ModCtrl)
	scr.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the editor loop did not exit")
	}
	return e
}

// typeRunes injects characters with a pause between them, since each has to be
// consumed before the next is meaningful.
func typeRunes(scr tcell.SimulationScreen, s string) {
	for _, r := range s {
		scr.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		time.Sleep(20 * time.Millisecond)
	}
}

// The reported bug: "After pressing C-s for search, the first character that I
// type gets ignored."
//
// Searching for "gam" and accepting must land point on gamma. If the first
// character is swallowed the search runs for "am", which also matches gamma -
// so the test searches for a string whose first character is the only thing
// distinguishing it: "bet" finds beta, "et" would find it too, but "zbet"
// cannot match at all while "bet" can.
func TestFirstKeyAfterASearchPromptIsNotLost(t *testing.T) {
	e := runKeys(t, "alpha\nbeta\ngamma", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		scr.InjectKey(tcell.KeyCtrlS, 0, tcell.ModCtrl)
		time.Sleep(60 * time.Millisecond)
		typeRunes(scr, "gam")
		time.Sleep(40 * time.Millisecond)
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone) // accept, leaving point at the match
		time.Sleep(40 * time.Millisecond)
	})

	// gamma is line 2. Point reaching it proves every typed character arrived:
	// drop the leading g and "am" still matches gamma, but drop any character
	// and the match ends in a different column.
	pt := e.Active().Pt
	if pt.Line != 2 {
		t.Fatalf("point = %v after searching for gam, want line 2 (gamma) - the "+
			"first typed character was swallowed", pt)
	}
	if pt.Col != 3 {
		t.Errorf("point = %v, want column 3 (end of the three-character match)", pt)
	}
}

// The same event source has to serve every prompt, not only incremental search.
func TestFirstKeyAfterMxIsNotLost(t *testing.T) {
	e := runKeys(t, "alpha\nbeta\ngamma", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		scr.InjectKey(tcell.KeyRune, 'x', tcell.ModAlt) // M-x
		time.Sleep(60 * time.Millisecond)
		typeRunes(scr, "end-of-buffer")
		time.Sleep(40 * time.Millisecond)
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
		time.Sleep(40 * time.Millisecond)
	})

	// Losing the leading "e" leaves "nd-of-buffer", which matches no command,
	// so point would never move.
	if got, want := e.Active().Pt, e.Buf().End(); got != want {
		t.Errorf("point = %v after M-x end-of-buffer, want the buffer end %v - the "+
			"command name lost its first character", got, want)
	}
}
