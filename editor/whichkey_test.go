package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Which-key's timing and its decision to show are deliberately separate, and
// these tests exercise the decision. A test that slept for a real delay and then
// asserted would flake on a loaded machine, and a flaky test is worse than none;
// only TestWhichKeyFiresThroughTheRealLoop involves a clock at all, and it can
// only fail in the "never fired" direction.

// --- arming --------------------------------------------------------------

func TestWhichKeyArmsOnAPendingPrefix(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	if e.whichKeyArmed() {
		t.Fatal("armed with no pending prefix")
	}
	press(t, e, "C-x")
	if !e.whichKeyArmed() {
		t.Error("not armed after C-x; the panel could never appear")
	}
}

func TestWhichKeyDoesNotArmForACompleteBinding(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-f")
	if e.whichKeyArmed() {
		t.Error("armed after a complete binding; nothing can follow it")
	}
}

// A prompt is what the user is looking at, so a panel describing a prefix must
// not cover it.
//
// The state is set directly rather than by opening a real prompt: press returns
// only once the prompt has closed, by which point mini is nil again, so driving
// this from the keyboard would assert on the wrong moment - and feeding C-x into
// a prompt leaves a pending prefix that swallows the accepting RET, which
// deadlocks the nested loop. A zero miniState is enough; the condition under
// test is only whether a prompt exists.
func TestWhichKeyDoesNotArmOrShowDuringAPrompt(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	if !e.whichKeyArmed() {
		t.Fatal("setup: not armed before the prompt")
	}

	e.mini = &miniState{}
	if e.whichKeyArmed() {
		t.Error("armed while a prompt was active; a panel must not cover a prompt")
	}
	e.fireWhichKey()
	if e.whichKeyPanel() != nil {
		t.Error("built a panel while a prompt was active")
	}
}

func TestWhichKeyDelayZeroNeverArmsOrShows(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	e.SetWhichKeyDelay(0)
	press(t, e, "C-x")
	if e.whichKeyArmed() {
		t.Error("armed with the delay disabled")
	}
	e.fireWhichKey()
	if e.whichKeyPanel() != nil {
		t.Error("built a panel with the delay disabled")
	}
}

func TestWhichKeyDefaultDelayIsUsedWhenUnset(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	if got := e.whichKeyDelay(); got != whichKeyDefaultDelay {
		t.Errorf("delay = %v, want the default %v", got, whichKeyDefaultDelay)
	}
}

func TestWhichKeyDoesNotReArmWhileShowing(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	e.fireWhichKey()
	if e.whichKeyPanel() == nil {
		t.Fatal("setup: no panel")
	}
	if e.whichKeyArmed() {
		t.Error("re-armed while already showing; the timer would fire repeatedly")
	}
}

// C-u 4 C-x is a pending prefix carrying an argument, and must still describe
// itself. The argument is resolved before keymap lookup, so this checks the two
// mechanisms do not interfere.
func TestWhichKeyArmsAfterAUniversalArgument(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-u", "4", "C-x")
	if !e.whichKeyArmed() {
		t.Error("not armed after C-u 4 C-x")
	}
}

// --- panel content -------------------------------------------------------

func wkText(t *testing.T, e *Editor) string {
	t.Helper()
	p := e.whichKeyPanel()
	if p == nil {
		t.Fatal("no which-key panel")
	}
	var b strings.Builder
	for _, ln := range p.Lines {
		b.WriteString(ln.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestWhichKeyPanelListsContinuations(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	e.fireWhichKey()

	body := wkText(t, e)
	for _, want := range []string{"undo", "find-file", "save-buffer", "other-window"} {
		if !strings.Contains(body, want) {
			t.Errorf("panel does not mention %q:\n%s", want, body)
		}
	}
}

func TestWhichKeyPanelTitleNamesThePendingSequence(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	e.fireWhichKey()
	if got := e.whichKeyPanel().Title; got != "C-x" {
		t.Errorf("title = %q, want %q", got, "C-x")
	}
}

// A sub-prefix cannot show a command name, so it must announce that it leads
// somewhere and how much is reachable beneath it.
//
// The binding is added here because nem's default keymap cannot exercise this:
// its longest sequence is two keys, so no pending prefix has a prefix among its
// continuations. A +prefix row first becomes reachable when a config adds a
// three-level binding, which is exactly when this must already work.
func TestWhichKeyMarksSubPrefixesWithACount(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	for _, spec := range []string{"C-c p q", "C-c p z"} {
		if err := bindSpec(e.keys, spec, "undo"); err != nil {
			t.Fatalf("binding %s: %v", spec, err)
		}
	}

	press(t, e, "C-c")
	e.fireWhichKey()
	body := wkText(t, e)
	if !strings.Contains(body, "+prefix") {
		t.Errorf("C-c panel should mark p as a prefix:\n%s", body)
	}
	// Two bindings live under C-c p, and the count is recursive, so it must say
	// 2 rather than counting p as one thing.
	if !strings.Contains(body, "(2)") {
		t.Errorf("C-c panel should report 2 bindings beneath p:\n%s", body)
	}
}

// A leaf continuation names its command, which is the common case.
func TestWhichKeyNamesLeafCommands(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "M-g")
	e.fireWhichKey()
	if body := wkText(t, e); !strings.Contains(body, "goto-line") {
		t.Errorf("M-g panel should name goto-line:\n%s", body)
	}
}

func TestWhichKeyPanelFitsTheFrame(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {40, 12}, {200, 60}, {20, 6}, {8, 4}} {
		scr := tcell.NewSimulationScreen("UTF-8")
		if err := scr.Init(); err != nil {
			t.Fatalf("init: %v", err)
		}
		scr.SetSize(size[0], size[1])
		e, err := New(scr)
		if err != nil {
			scr.Fini()
			t.Fatalf("New: %v", err)
		}
		press(t, e, "C-x")
		e.fireWhichKey()
		if p := e.whichKeyPanel(); p != nil {
			r := p.Rect
			if r.X < 0 || r.Y < 0 || r.X+r.W > size[0] || r.Y+r.H > size[1]-1 {
				t.Errorf("%dx%d: panel %v escapes the frame (echo row excluded)", size[0], size[1], r)
			}
			if len(p.Lines) > r.H-2 {
				t.Errorf("%dx%d: %d lines into a %d-row interior", size[0], size[1], len(p.Lines), r.H-2)
			}
		}
		scr.Fini()
	}
}

// --- dismissal -----------------------------------------------------------

// Any key hides the panel AND is still acted on. Swallowing it would make the
// feature cost a keystroke, which is exactly what "never slow down someone who
// knows the binding" forbids.
func TestAnyKeyDismissesThePanelAndIsStillProcessed(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	e.fireWhichKey()
	if e.whichKeyPanel() == nil {
		t.Fatal("setup: no panel")
	}

	press(t, e, "u") // C-x u is undo
	if e.whichKeyPanel() != nil {
		t.Error("panel still showing after a key")
	}
	if got := e.lastCmd; got != "undo" {
		t.Errorf("lastCmd = %q, want undo - the dismissing key was swallowed", got)
	}
}

func TestQuitKeyClearsThePrefixAndThePanel(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x")
	e.fireWhichKey()
	press(t, e, "C-g")

	if e.whichKeyPanel() != nil {
		t.Error("panel still showing after C-g")
	}
	if len(e.pending) != 0 {
		t.Errorf("pending = %v after C-g, want empty", e.pending)
	}
	wantEcho(t, e, "Quit")
}

func TestFireIsANoOpOnceThePrefixIsGone(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	press(t, e, "C-x", "u") // completes; pending is empty again
	e.fireWhichKey()
	if e.whichKeyPanel() != nil {
		t.Error("built a panel with no pending prefix")
	}
}

// --- the loop ------------------------------------------------------------

func TestLoopShutsDownCleanly(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	feed(t, scr, key(t, "C-x"), key(t, "C-c"))

	done := make(chan error, 1)
	go func() { done <- e.Loop() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Loop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not return; the event goroutine or a timer is stuck")
	}
}

// The only clock-dependent test, and it is one-directional: a which-key that
// never fires makes shows 0 and fails. A slow machine only makes firing more
// likely, so this cannot flake in the passing direction.
func TestWhichKeyFiresThroughTheRealLoop(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	e.SetWhichKeyDelay(time.Millisecond)

	go func() {
		for _, ev := range key(t, "C-x") {
			scr.InjectKey(ev.key, ev.r, ev.mod)
		}
		time.Sleep(150 * time.Millisecond) // long enough for a 1ms timer
		for _, ev := range key(t, "C-g", "C-x", "C-c") {
			scr.InjectKey(ev.key, ev.r, ev.mod)
		}
	}()

	done := make(chan error, 1)
	go func() { done <- e.Loop() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not return")
	}

	// Read after Loop returns: the return orders the loop's writes before this.
	if e.wk.shows == 0 {
		t.Error("which-key never fired through the real loop; the timer is not wired")
	}
}

// A frame too short for every continuation must say so. Dropping rows silently
// would let the panel present itself as the complete answer when it is not,
// which is worse than not showing the panel at all.
func TestWhichKeyReportsDroppedContinuations(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(72, 9) // too short for C-x's fifteen continuations
	e, err := New(scr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	press(t, e, "C-x")
	e.fireWhichKey()
	body := wkText(t, e)
	if !strings.Contains(body, "more") {
		t.Errorf("a truncated panel must report what it dropped:\n%s", body)
	}
}

// A frame with room for everything must not claim anything was dropped.
func TestWhichKeyReportsNothingDroppedWhenItAllFits(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(72, 24)
	e, err := New(scr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	press(t, e, "C-x")
	e.fireWhichKey()
	if body := wkText(t, e); strings.Contains(body, "more") {
		t.Errorf("a complete panel must not claim a truncation:\n%s", body)
	}
}
