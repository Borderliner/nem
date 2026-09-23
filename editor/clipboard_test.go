package editor

import (
	"strings"
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

// --- C-y and text copied elsewhere ---------------------------------------

// fakeClipboard stands in for the desktop's clipboard tools.
type fakeClipboard struct {
	text  string
	reads int
}

func (f *fakeClipboard) read() (string, bool) {
	f.reads++
	return f.text, f.text != ""
}

// withClipboard installs a fake system clipboard holding s ("" for one with no
// text on it, as when it holds an image).
func withClipboard(e *Editor, s string) *fakeClipboard {
	f := &fakeClipboard{text: s}
	e.clip.read = f.read
	return f
}

// The reported bug: text copied in a browser, C-y in nem, and "kill ring is
// empty" with the text sitting right there on the clipboard.
func TestYankPastesTextCopiedInAnotherApplication(t *testing.T) {
	e, _ := newTestEditor(t)
	withClipboard(e, "copied in the browser")

	press(t, e, "C-y")

	wantText(t, e, "copied in the browser")
	if strings.Contains(e.Message(), "empty") {
		t.Errorf("echo = %q after yanking the clipboard", e.Message())
	}
}

// A copy made elsewhere after a kill is the newer of the two, so C-y takes it -
// and the kill is still one M-y away rather than lost.
func TestANewerCopyIsYankedAndOlderKillsStayReachable(t *testing.T) {
	e, _ := newTestEditor(t, "killed here")
	press(t, e, "C-k")
	wantText(t, e, "")

	withClipboard(e, "copied elsewhere")
	press(t, e, "C-y")
	wantText(t, e, "copied elsewhere")

	press(t, e, "M-y")
	wantText(t, e, "killed here")
}

// Text nem put on the clipboard itself comes straight back when it is read. It
// is already the ring's newest entry, and adding it again would leave M-y
// cycling through a duplicate.
func TestYankDoesNotReaddOurOwnKill(t *testing.T) {
	e, _ := newTestEditor(t, "mine")
	press(t, e, "C-k")
	before := e.Ring().Len()

	withClipboard(e, "mine") // the kill reached the clipboard over OSC 52
	press(t, e, "C-y")

	wantText(t, e, "mine")
	if got := e.Ring().Len(); got != before {
		t.Errorf("ring grew from %d to %d yanking our own kill back", before, got)
	}
}

// An unchanged clipboard is taken once. Taking it on every C-y would push a
// copy each time, and M-y would have to wade through them.
//
// It matters most on a terminal that ignores OSC 52: there the clipboard keeps
// its old text after a kill, and re-reading it must not bury the kill under a
// second copy of text the ring already holds.
func TestAnUnchangedClipboardIsTakenOnlyOnce(t *testing.T) {
	e, _ := newTestEditor(t)
	withClipboard(e, "external")

	wantYank(t, e, "external")
	wantYank(t, e, "external")
	if got := e.Ring().Len(); got != 1 {
		t.Errorf("ring holds %d entries after two yanks of one copy, want 1", got)
	}

	e.KillForward("a fresh kill") // the clipboard still says "external"
	e.Ring().BreakRun()
	wantYank(t, e, "a fresh kill")
}

// wantYank asserts what Yank returns: the clipboard rule on its own, without
// the buffer editing a yank command does around it.
func wantYank(t *testing.T, e *Editor, want string) {
	t.Helper()
	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got != want {
		t.Errorf("Yank = %q, want %q", got, want)
	}
}

// When the clipboard holds no text - an image, say, which the tools refuse to
// hand over as text - C-y yanks from the ring exactly as before.
func TestYankFallsBackToTheRingWhenTheClipboardHasNoText(t *testing.T) {
	e, _ := newTestEditor(t, "kept")
	press(t, e, "C-k")
	withClipboard(e, "")

	press(t, e, "C-y")
	wantText(t, e, "kept")
}

func TestYankWithNothingAnywhereStillSaysSo(t *testing.T) {
	e, _ := newTestEditor(t)
	withClipboard(e, "")
	press(t, e, "C-y")
	wantEcho(t, e, "kill ring is empty")
}

// clipboard = "off" means nem leaves the clipboard alone in both directions.
func TestClipboardOffNeverReadsTheClipboard(t *testing.T) {
	e, _ := newTestEditor(t, "ring text")
	press(t, e, "C-k")
	e.SetClipboardMode(ClipboardOff)
	f := withClipboard(e, "outside")

	press(t, e, "C-y")

	wantText(t, e, "ring text")
	if f.reads != 0 {
		t.Errorf("the clipboard was read %d times with clipboard = off", f.reads)
	}
}

// M-y walks what the ring already holds. Reading the clipboard there could push
// a new entry mid-cycle and shift everything M-y is stepping through.
func TestYankPopDoesNotReadTheClipboard(t *testing.T) {
	e, _ := newTestEditor(t)
	e.KillForward("one")
	e.Ring().BreakRun()
	e.KillForward("two")
	e.Ring().BreakRun()
	f := withClipboard(e, "")

	press(t, e, "C-y")
	reads := f.reads
	press(t, e, "M-y")

	wantText(t, e, "one")
	if f.reads != reads {
		t.Errorf("M-y read the clipboard")
	}
}

// C-y at a prompt reaches the clipboard too, which is how a path copied from a
// file manager gets into C-x C-f.
func TestYankAtAPromptTakesTheClipboard(t *testing.T) {
	e, scr := newTestEditor(t)
	withClipboard(e, "/tmp/elsewhere")
	go func() {
		scr.InjectKey(tcell.KeyCtrlY, 0, tcell.ModCtrl)
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}()

	got, err := e.ReadString(readOpts("Find file: ", nil))
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if got != "/tmp/elsewhere" {
		t.Errorf("ReadString = %q, want the clipboard's text", got)
	}
}
