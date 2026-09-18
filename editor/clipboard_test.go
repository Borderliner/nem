package editor

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// clip returns the editor's simulation screen so a test can read back whatever
// was handed to the system clipboard.
func clipScreen(t *testing.T, e *Editor) tcell.SimulationScreen {
	t.Helper()
	scr, ok := e.scr.(tcell.SimulationScreen)
	if !ok {
		t.Fatal("editor is not running on a simulation screen")
	}
	return scr
}

func wantClipboard(t *testing.T, e *Editor, want string) {
	t.Helper()
	if got := string(clipScreen(t, e).GetClipboardData()); got != want {
		t.Errorf("system clipboard = %q, want %q", got, want)
	}
}

func TestKillReachesTheSystemClipboard(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("copied text")
	wantClipboard(t, e, "copied text")
}

// The kill ring accumulates consecutive kills into one entry, so C-k C-k must
// leave BOTH lines on the clipboard. Sending only the latest fragment would make
// the clipboard disagree with what C-y pastes, which is worse than not
// integrating at all — the user cannot see either one to compare.
func TestConsecutiveKillsSendTheWholeAccumulatedEntry(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("first\n")
	e.KillForward("second")
	wantClipboard(t, e, "first\nsecond")
}

// Backward kills extend the entry on the left, so the clipboard has to follow
// the same order the ring uses or M-DEL M-DEL would copy text reversed.
func TestBackwardKillsAccumulateInReadingOrder(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillBackward("bar")
	e.KillBackward("foo ")
	wantClipboard(t, e, "foo bar")
}

// A broken kill run starts a new clipboard entry rather than appending to the
// previous one.
func TestABrokenRunStartsAFreshClipboardEntry(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("first")
	e.Ring().BreakRun()
	e.KillForward("second")
	wantClipboard(t, e, "second")
}

// The clipboard must agree with what the ring will actually paste.
func TestClipboardMatchesWhatYankReturns(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("alpha ")
	e.KillForward("beta")

	yanked, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	wantClipboard(t, e, yanked)
}

func TestClipboardOffSendsNothing(t *testing.T) {
	e, _ := newTestEditor(t)
	e.SetClipboardMode(ClipboardOff)
	e.KillForward("secret")
	wantClipboard(t, e, "")
}

// An empty kill is a no-op in the ring, so it must not disturb the clipboard
// either.
func TestEmptyKillLeavesTheClipboardAlone(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("kept")
	e.KillForward("")
	wantClipboard(t, e, "kept")
}

// --- reading -------------------------------------------------------------

// A reply from the terminal becomes the newest ring entry, so the ordinary C-y
// picks it up without a second yank command.
func TestClipboardReplyBecomesTheNewestKill(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("older")
	e.SetClipboardReply([]byte("from elsewhere"))

	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got != "from elsewhere" {
		t.Errorf("yank = %q, want the clipboard reply", got)
	}
}

// Terminals echo back whatever was just set. Adding that to the ring would
// duplicate an entry it already holds.
func TestOurOwnClipboardTextIsNotEchoedBackIntoTheRing(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("mine")
	before := e.Ring().Len()

	e.SetClipboardReply([]byte("mine"))

	if got := e.Ring().Len(); got != before {
		t.Errorf("ring grew from %d to %d on an echo of our own text", before, got)
	}
}

func TestEmptyClipboardReplyIsIgnored(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("kept")
	before := e.Ring().Len()
	e.SetClipboardReply(nil)
	if got := e.Ring().Len(); got != before {
		t.Errorf("ring grew from %d to %d on an empty reply", before, got)
	}
}

// A reply arriving mid-run must not fuse with the kill in progress: it came from
// somewhere else entirely.
func TestClipboardReplyDoesNotJoinAnOpenKillRun(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("run ")
	e.SetClipboardReply([]byte("outside"))
	e.KillForward("continues")

	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got != "continues" {
		t.Errorf("yank = %q, want the later kill standing alone", got)
	}
}

func TestRequestClipboardIsInertWhenOff(t *testing.T) {
	e, _ := newTestEditor(t)
	e.SetClipboardMode(ClipboardOff)
	if got := e.ClipboardMode(); got != ClipboardOff {
		t.Fatalf("ClipboardMode = %v, want ClipboardOff", got)
	}
	e.RequestClipboard()
	wantEcho(t, e, "off")
}

func TestRequestClipboardAsksTheTerminal(t *testing.T) {
	e, _ := newTestEditor(t)
	e.RequestClipboard()
	wantEcho(t, e, "Asked the terminal")
}
