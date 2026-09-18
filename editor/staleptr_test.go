package editor

import (
	"testing"

	"github.com/hajianpour/nem/text"
)

// Point lives in the window, and a buffer has no idea which windows are showing
// it. So an edit that shortens a buffer in one window can leave another
// window's point past the end of the text. text.Buffer.Line is an unguarded
// slice index, so the next command run in that window indexes out of range and
// takes the whole editor down - losing unsaved work in every other buffer.
//
// This is not a scripting problem. Ordinary editing reaches it.
func TestShorteningABufferDoesNotStrandAnotherWindowsPoint(t *testing.T) {
	e, _ := newTestEditor(t, "one", "two", "three", "four", "five")

	press(t, e, "C-x", "2") // split
	press(t, e, "C-x", "o") // move to the other window
	press(t, e, "M->")      // send its point to the end of the buffer

	other := e.Active()
	if other.Pt.Line == 0 {
		t.Fatalf("setup: expected the other window's point past line 0, got %v", other.Pt)
	}

	press(t, e, "C-x", "o") // back to the first window
	press(t, e, "M-<")
	press(t, e, "C-u", "2", "0", "C-k") // kill far more lines than exist

	if n := e.Buf().NumLines(); n > 1 {
		t.Fatalf("setup: expected the buffer emptied, still %d lines", n)
	}

	// The other window's point must have been brought back into the buffer.
	if other.Pt.Line >= e.Buf().NumLines() {
		t.Errorf("other window's point is %v, past the end of a %d-line buffer",
			other.Pt, e.Buf().NumLines())
	}

	// And acting in that window must not panic.
	press(t, e, "C-x", "o")
	press(t, e, "C-f")
	press(t, e, "C-n")
	press(t, e, "C-e")
	if got := e.Active().Pt; got.Line >= e.Buf().NumLines() {
		t.Errorf("point %v still out of range after moving", got)
	}
	_ = text.Pos{}
}
