package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// hit sends key specs through HandleEvent, the way the event loop delivers
// them at the top level - which is where a macro records them.
func hit(t *testing.T, e *Editor, specs ...string) {
	t.Helper()
	for _, ev := range key(t, specs...) {
		e.HandleEvent(tcell.NewEventKey(ev.key, ev.r, ev.mod))
	}
}

// hitText types s through HandleEvent.
func hitText(e *Editor, s string) {
	for _, r := range s {
		e.HandleEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
	}
}

// Record an edit on one line with F3 ... F4, then play it on the next lines
// with F4 and with C-u 2 F4.
func TestKmacroRecordsAndPlays(t *testing.T) {
	e, _ := newTestEditor(t, "a", "b", "c", "d")
	hit(t, e, "<f3>")
	hitText(e, "- ")
	hit(t, e, "C-a", "C-n", "<f4>")
	wantText(t, e, "- a\nb\nc\nd")
	if !e.km.recording && len(e.km.last) == 0 {
		t.Fatal("nothing was recorded")
	}

	hit(t, e, "<f4>")
	wantText(t, e, "- a\n- b\nc\nd")
	hit(t, e, "C-u", "2", "<f4>")
	wantText(t, e, "- a\n- b\n- c\n- d")
}

// The keys that end the recording are not part of it: C-x ) and F4 both
// leave a macro that does only what was typed before them.
func TestKmacroLeavesOutTheEndingKeys(t *testing.T) {
	for _, end := range [][]string{{"<f4>"}, {"C-x", ")"}} {
		e, _ := newTestEditor(t, "")
		hit(t, e, "C-x", "(")
		hitText(e, "x")
		hit(t, e, end...)
		if got := len(e.km.last); got != 1 {
			t.Errorf("ending with %v recorded %d events, want just the x", end, got)
		}
	}
}

// C-u 0 F4 plays until a command fails - here C-n at the last line - and says
// how far it got.
func TestKmacroRepeatsUntilItFails(t *testing.T) {
	e, _ := newTestEditor(t, "1", "2", "3", "4", "5")
	hit(t, e, "<f3>")
	hitText(e, ">")
	hit(t, e, "C-a", "C-n", "<f4>")
	hit(t, e, "C-u", "0", "<f4>")
	wantText(t, e, ">1\n>2\n>3\n>4\n>5")
	wantEcho(t, e, "until it failed")
}

// A macro that searches replays its prompt's keys too - they came through the
// prompt's own loop, not the top level - and a search that fails stops it.
func TestKmacroReplaysPromptsAndStopsOnAFailedSearch(t *testing.T) {
	e, scr := newTestEditor(t, "x = 1", "y", "x = 2", "z")
	hit(t, e, "<f3>")
	feed(t, scr, txt("x ="), key(t, "RET"))
	hit(t, e, "C-s")
	hitText(e, "!")
	hit(t, e, "<f4>")
	wantText(t, e, "x =! 1\ny\nx = 2\nz")

	// Played until it fails: the second x = gets one, then the search finds
	// nothing more and the macro stops rather than typing ! where point is.
	hit(t, e, "C-u", "0", "<f4>")
	wantText(t, e, "x =! 1\ny\nx =! 2\nz")
}

// F3 while recording inserts a counter, which counts on as the macro plays.
func TestKmacroCounter(t *testing.T) {
	e, _ := newTestEditor(t, "", "", "")
	hit(t, e, "<f3>", "<f3>")
	hit(t, e, "C-n", "<f4>")
	hit(t, e, "C-u", "2", "<f4>")
	wantText(t, e, "0\n1\n2")
}

// After C-x e a bare e plays again; any other key ends that and does its job.
func TestKmacroCallThenE(t *testing.T) {
	e, _ := newTestEditor(t, "")
	hit(t, e, "C-x", "(")
	hitText(e, "ab")
	hit(t, e, "C-x", ")")
	hit(t, e, "C-x", "e")
	hitText(e, "e")
	wantText(t, e, "ababab")
	hitText(e, "q")
	hitText(e, "e")
	wantText(t, e, "abababqe")
}

func TestKmacroWithNothingRecorded(t *testing.T) {
	e, _ := newTestEditor(t, "text")
	hit(t, e, "<f4>")
	wantEcho(t, e, "no keyboard macro defined")
	wantText(t, e, "text")
}
