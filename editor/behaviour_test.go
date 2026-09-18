package editor

import (
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// --- the kill ring end to end --------------------------------------------

// Three C-k presses from the start of "a\nb\nc" kill "a", then the newline,
// then "b" — so the ring holds "a\nb" as ONE entry and a single C-y puts it
// back, restoring the buffer exactly.
//
// This is the most-cited kill-ring behaviour and it only works if dispatch
// leaves the kill run open across consecutive kills. If the run broke between
// presses, C-y would return just "b".
func TestKillKillYankRestoresBuffer(t *testing.T) {
	e, _ := newTestEditor(t, "a", "b", "c")

	press(t, e, "C-k", "C-k", "C-k")
	wantText(t, e, "\nc")

	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if want := "a\nb"; got != want {
		t.Fatalf("ring entry = %q, want %q (three presses kill text, newline, text)", got, want)
	}
	if n := e.Ring().Len(); n != 1 {
		t.Errorf("ring holds %d entries, want 1 — consecutive kills must accumulate", n)
	}

	e.Active().Pt = text.Pos{}
	press(t, e, "C-y")
	wantText(t, e, "a\nb\nc")
}

// A command between two kills breaks the run, so they land in separate entries
// and C-y brings back only the second. This is the other half of the rule and
// the half a missing BreakRun silently destroys.
func TestCommandBetweenKillsBreaksTheRun(t *testing.T) {
	e, _ := newTestEditor(t, "aaa", "bbb")

	press(t, e, "C-k") // kills "aaa"
	press(t, e, "C-n") // a non-kill: breaks the run
	press(t, e, "C-k") // kills "bbb" into a NEW entry

	if n := e.Ring().Len(); n != 2 {
		t.Fatalf("ring holds %d entries, want 2 — a non-kill command must break the run", n)
	}
	got, err := e.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if want := "bbb"; got != want {
		t.Errorf("ring entry = %q, want %q", got, want)
	}
}

// A nested dispatch does the bookkeeping, and the outer one must not repeat it.
//
// Here a wrapper command — which is not a kill — invokes kill-line. If the outer
// dispatch also ran bookkeeping it would break the kill run that the inner
// kill-line just extended, and last-command would name the wrapper rather than
// the command that actually ran. No prompt is involved, so this isolates the
// nesting rule from the minibuffer.
func TestNestedDispatchDoesTheBookkeepingOnce(t *testing.T) {
	e, _ := newTestEditor(t, "aaa", "bbb")
	if err := e.Registry().Register(command.Command{
		Name: "test-wrap", Doc: "t", Interactive: true,
		Fn: func(env command.Env) error { return env.Run("kill-line") },
	}); err != nil {
		t.Fatal(err)
	}

	press(t, e, "C-k") // kills "aaa", run open
	if err := e.Run("test-wrap"); err != nil {
		t.Fatalf("test-wrap: %v", err)
	}

	if n := e.Ring().Len(); n != 1 {
		t.Errorf("ring holds %d entries, want 1 — the outer dispatch broke the run", n)
	}
	if got := e.LastCommand(); got != "kill-line" {
		t.Errorf("LastCommand = %q, want %q — the innermost command is the real one", got, "kill-line")
	}
}

// M-x records the invoked command as last-command, not execute-extended-command,
// which is what makes M-x yank followed by M-y work.
func TestExecuteExtendedCommandRecordsTheInvokedCommand(t *testing.T) {
	e, scr := newTestEditor(t, "aaa", "bbb")
	feed(t, scr, txt("kill-line"), key(t, "RET"))
	press(t, e, "M-x")
	if got := e.LastCommand(); got != "kill-line" {
		t.Errorf("LastCommand = %q, want %q", got, "kill-line")
	}
}

// --- the goal column ------------------------------------------------------

// Moving down through a short line and out the other side returns to the
// original column. The column is established by the first vertical move and
// preserved across the run; anything else clears it.
func TestGoalColumnSurvivesAShortLine(t *testing.T) {
	e, _ := newTestEditor(t, "aaaaaaaaaa", "bb", "cccccccccc")
	e.Active().Pt = text.Pos{Line: 0, Col: 5}

	press(t, e, "C-n")
	wantPt(t, e, 1, 2) // clamped to the short line's end

	press(t, e, "C-n")
	wantPt(t, e, 2, 5) // and back out to the goal column
}

// A non-vertical command clears the goal column, so the next C-n starts a fresh
// one from wherever point actually is.
func TestOtherCommandsClearTheGoalColumn(t *testing.T) {
	e, _ := newTestEditor(t, "aaaaaaaaaa", "bb", "cccccccccc")
	e.Active().Pt = text.Pos{Line: 0, Col: 5}

	press(t, e, "C-n") // establishes goal 5
	press(t, e, "C-a") // clears it, point to {1,0}
	if got := e.Active().GoalCol; got != view.GoalColUnset {
		t.Fatalf("GoalCol = %d after C-a, want unset", got)
	}
	press(t, e, "C-n")
	wantPt(t, e, 2, 0) // fresh goal of 0, not the stale 5
}

// A yank moves point, so the goal column it leaves behind must not survive —
// otherwise the next C-n navigates by a column from before the insertion.
func TestYankClearsTheGoalColumn(t *testing.T) {
	e, _ := newTestEditor(t, "aaaaaaaaaa", "bb")
	e.Active().Pt = text.Pos{Line: 0, Col: 5}
	press(t, e, "C-n") // establishes a goal column
	e.Ring().KillForward("XY")
	press(t, e, "C-y")

	if got := e.Active().GoalCol; got != view.GoalColUnset {
		t.Errorf("GoalCol = %d after a yank, want unset", got)
	}
}

// --- windows --------------------------------------------------------------

// C-x 2 then C-x o gives two windows on one buffer with independent points,
// which is the case the whole view/window split exists for.
func TestSplitWindowsShareBufferWithIndependentPoints(t *testing.T) {
	e, _ := newTestEditor(t, "one", "two", "three")

	press(t, e, "C-x", "2")
	ws := e.Tree().Windows()
	if len(ws) != 2 {
		t.Fatalf("got %d windows after C-x 2, want 2", len(ws))
	}
	if ws[0].Buf != ws[1].Buf {
		t.Fatal("the split windows show different buffers, want the same one")
	}

	press(t, e, "C-n", "C-n") // move point in the active window only
	active := e.Active()
	var other *view.Window
	for _, w := range ws {
		if w != active {
			other = w
		}
	}
	if active.Pt == other.Pt {
		t.Error("both windows' points moved together, want independent points")
	}

	press(t, e, "C-x", "o")
	if e.Active() == active {
		t.Error("C-x o did not change the selected window")
	}

	// An edit through one window is visible through the other: it is one buffer.
	before := other.Buf.String()
	press(t, e, "C-k")
	if after := other.Buf.String(); after == before {
		t.Error("an edit through one window was not visible through the other")
	}
}

// Deleting the sole window is refused rather than emptying the frame.
func TestDeleteSoleWindowIsRefused(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	press(t, e, "C-x", "0")
	if len(e.Tree().Windows()) != 1 {
		t.Error("the sole window was deleted")
	}
}

// --- the universal argument ----------------------------------------------

func TestUniversalArgumentForms(t *testing.T) {
	for _, tc := range []struct {
		name  string
		specs []string
		want  text.RuneIdx
	}{
		{"C-u is four", []string{"C-u", "C-f"}, 4},
		{"C-u C-u is sixteen", []string{"C-u", "C-u", "C-f"}, 16},
		{"C-u digits", []string{"C-u", "1", "2", "C-f"}, 12},
		{"Meta digits", []string{"M-1", "M-2", "C-f"}, 12},
		{"no argument is one", []string{"C-f"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTestEditor(t, strings.Repeat("x", 40))
			press(t, e, tc.specs...)
			wantPt(t, e, 0, tc.want)
		})
	}
}

// A negative argument reverses direction: C-u - C-f goes back one.
func TestNegativeUniversalArgument(t *testing.T) {
	e, _ := newTestEditor(t, strings.Repeat("x", 40))
	e.Active().Pt = text.Pos{Line: 0, Col: 10}
	press(t, e, "C-u", "-", "C-f")
	wantPt(t, e, 0, 9)
}

// The argument is consumed by the command it modifies and must not leak into
// the next one.
func TestArgumentDoesNotLeakToTheNextCommand(t *testing.T) {
	e, _ := newTestEditor(t, strings.Repeat("x", 40))
	press(t, e, "C-u", "4", "C-f")
	wantPt(t, e, 0, 4)
	press(t, e, "C-f")
	wantPt(t, e, 0, 5)
}

// C-u 4 x inserts four x's: the argument reaches self-insert-command, which
// only works because the loop sets Seq.LastRune before dispatching.
func TestArgumentReachesSelfInsert(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "C-u", "4", "x")
	wantText(t, e, "xxxx")
}

// --- C-g ------------------------------------------------------------------

// C-g cancels a pending prefix. Without special handling this would be looked
// up as the sequence "C-x C-g" and reported undefined.
func TestQuitCancelsAPendingPrefix(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	press(t, e, "C-x")
	wantEcho(t, e, "C-x")
	press(t, e, "C-g")
	wantEcho(t, e, "Quit")

	// The prefix is gone, so a following "2" inserts rather than splitting.
	press(t, e, "2")
	wantText(t, e, "2x")
}

// C-g also abandons a half-typed argument.
func TestQuitCancelsAnArgument(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "C-u", "4", "C-g")
	wantEcho(t, e, "Quit")
	press(t, e, "x")
	wantText(t, e, "x") // one x, not four
}

// --- self-insert and undefined keys --------------------------------------

func TestUnboundPrintableKeyInserts(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "h", "i")
	wantText(t, e, "hi")
}

// An unbound control key reports itself rather than inserting anything.
func TestUnboundControlKeyIsReported(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "C-x", "C-z")
	wantEcho(t, e, "undefined")
	wantText(t, e, "")
}

// Typing then undoing removes the whole word: self-inserts coalesce into one
// undo unit.
func TestTypingCoalescesIntoOneUndoUnit(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "h", "e", "l", "l", "o")
	wantText(t, e, "hello")
	press(t, e, "C-_")
	wantText(t, e, "")
}
