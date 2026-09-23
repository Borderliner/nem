package editor

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
)

// The reported bug: after C-x h, Backspace did nothing at all, because point
// sat at the buffer start and there was no character before it to delete. The
// selection was simply ignored.
func TestSelectAllThenBackspaceEmptiesTheBuffer(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta", "gamma")

	press(t, e, "C-x", "h")
	press(t, e, "<backspace>")

	if got := e.Buf().String(); got != "" {
		t.Errorf("buffer = %q after C-x h then Backspace, want empty", got)
	}
}

// C-d must behave the same way, and neither may delete a character beyond the
// region: superseding means the region deletion IS the whole operation.
func TestSelectAllThenDeleteCharEmptiesTheBuffer(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")

	press(t, e, "C-x", "h")
	press(t, e, "C-d")

	if got := e.Buf().String(); got != "" {
		t.Errorf("buffer = %q after C-x h then C-d, want empty", got)
	}
}

// The supersede rule, stated where it can fail: with a partial region selected,
// Backspace must remove exactly the region and not one character more.
func TestSupersedeDoesNotEatAnExtraCharacter(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
	}{
		{"backspace", "<backspace>"},
		{"delete-char", "C-d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTestEditor(t, "abcdef")
			// Select "cd": mark at column 2, point at column 4.
			e.Active().Pt = text.Pos{Line: 0, Col: 2}
			press(t, e, "C-SPC", "C-f", "C-f")
			press(t, e, tc.key)

			if got, want := e.Buf().String(), "abef"; got != want {
				t.Errorf("buffer = %q, want %q — a character beyond the region was taken", got, want)
			}
		})
	}
}

// Typing over a selection replaces it.
func TestSelectAllThenTypingReplacesEverything(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta", "gamma")

	press(t, e, "C-x", "h")
	press(t, e, "x")

	if got := e.Buf().String(); got != "x" {
		t.Errorf("buffer = %q after C-x h then typing x, want %q", got, "x")
	}
}

// Only the selected part of a line is replaced.
func TestTypingReplacesOnlyTheSelection(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 2}
	press(t, e, "C-SPC", "C-f", "C-f") // select "cd"
	press(t, e, "Z")

	if got, want := e.Buf().String(), "abZef"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// RET with a selection replaces it with a line break.
func TestNewlineReplacesTheSelection(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 2}
	press(t, e, "C-SPC", "C-f", "C-f") // select "cd"
	press(t, e, "RET")

	if got, want := e.Buf().String(), "ab\nef"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// A yank over a selection replaces it rather than inserting alongside it.
func TestYankReplacesTheSelection(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")

	// Put "XY" on the kill ring by killing it from the end of the line.
	e.Active().Pt = text.Pos{Line: 0, Col: 6}
	press(t, e, "C-SPC")
	e.Active().Pt = text.Pos{Line: 0, Col: 4}
	press(t, e, "C-w") // kills "ef"
	if got := e.Buf().String(); got != "abcd" {
		t.Fatalf("setup: buffer = %q, want %q", got, "abcd")
	}

	// Now select "bc" and yank over it.
	e.Active().Pt = text.Pos{Line: 0, Col: 1}
	press(t, e, "C-SPC", "C-f", "C-f")
	press(t, e, "C-y")

	if got, want := e.Buf().String(), "aefd"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// The region deletion and the command that follows are one undo unit. Without
// grouping, undoing a replacement would leave the buffer with the selection
// gone and the typed character removed — a state the user never saw.
func TestReplacementUndoesInOneStep(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta", "gamma")
	before := e.Buf().String()

	press(t, e, "C-x", "h")
	press(t, e, "x")
	if got := e.Buf().String(); got != "x" {
		t.Fatalf("setup: buffer = %q, want %q", got, "x")
	}

	press(t, e, "C-_") // one undo

	if got := e.Buf().String(); got != before {
		t.Errorf("after one undo buffer = %q, want the original %q", got, before)
	}
}

// A superseding deletion undoes in one step too.
func TestSupersedeUndoesInOneStep(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	before := e.Buf().String()

	press(t, e, "C-x", "h")
	press(t, e, "<backspace>")
	press(t, e, "C-_")

	if got := e.Buf().String(); got != before {
		t.Errorf("after one undo buffer = %q, want the original %q", got, before)
	}
}

// kill-region consumes the region itself. If delete-selection ran first the
// region would be gone before the kill, and the kill ring would get nothing —
// silently losing what the user asked to cut.
func TestKillRegionIsNotDoubleDeleted(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 2}
	press(t, e, "C-SPC", "C-f", "C-f") // select "cd"
	press(t, e, "C-w")

	if got, want := e.Buf().String(), "abef"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	// The proof the kill ring received it: yank it back.
	press(t, e, "C-y")
	if got, want := e.Buf().String(), "abcdef"; got != want {
		t.Errorf("after yank buffer = %q, want %q — the kill ring was empty", got, want)
	}
}

// M-w copies without removing, and must keep doing so.
func TestKillRingSaveIsNotAffected(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 2}
	press(t, e, "C-SPC", "C-f", "C-f")
	press(t, e, "M-w")

	if got, want := e.Buf().String(), "abcdef"; got != want {
		t.Errorf("buffer = %q after M-w, want it unchanged %q", got, want)
	}
}

// TAB with a block selected indents it in a modern editor. Deleting the
// selection instead would be destructive and surprising, so it is deliberately
// not delete-selection aware.
func TestTabDoesNotDeleteTheSelection(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 2}
	press(t, e, "C-SPC", "C-f", "C-f")
	press(t, e, "TAB")

	if got := e.Buf().String(); !strings.Contains(got, "cd") {
		t.Errorf("buffer = %q, want the selection intact", got)
	}
}

// With no mark at all, every command behaves exactly as before.
func TestNoMarkLeavesCommandsAlone(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 3}
	press(t, e, "<backspace>")

	if got, want := e.Buf().String(), "abdef"; got != want {
		t.Errorf("buffer = %q, want %q — Backspace should delete one character", got, want)
	}
}

// An empty region is not a selection. The command runs normally, and the mark —
// which the user set deliberately — is left alone.
func TestEmptyRegionIsNotASelection(t *testing.T) {
	e, _ := newTestEditor(t, "abcdef")
	e.Active().Pt = text.Pos{Line: 0, Col: 3}
	press(t, e, "C-SPC") // mark and point coincide
	press(t, e, "<backspace>")

	if got, want := e.Buf().String(), "abdef"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if !e.Buf().HasMark() {
		t.Error("the mark was cleared by a command that consumed no selection")
	}
}

// Once the selection has been consumed there is no selection.
func TestTheMarkIsInactiveAfterwards(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	press(t, e, "C-x", "h")
	press(t, e, "x")

	if e.Buf().MarkActive() {
		t.Error("the region is still active after the selection was replaced")
	}
}

// Turning the setting off restores the previous behaviour exactly: Backspace at
// the buffer start with everything selected does nothing.
func TestDeleteSelectionCanBeTurnedOff(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	e.SetDeleteSelection(false)
	before := e.Buf().String()

	press(t, e, "C-x", "h")
	press(t, e, "<backspace>")

	if got := e.Buf().String(); got != before {
		t.Errorf("buffer = %q with delete-selection off, want it unchanged %q", got, before)
	}
}

// self-insert-command reads the triggering rune from Seq().LastRune, which the
// event loop records before dispatch. Deleting the region happens inside
// dispatch, so it must not disturb it.
func TestReplacementKeepsTheTriggeringRune(t *testing.T) {
	e, _ := newTestEditor(t, "alpha")
	press(t, e, "C-x", "h")
	press(t, e, "Q")

	if got := e.Buf().String(); got != "Q" {
		t.Errorf("buffer = %q, want %q — the triggering rune was lost", got, "Q")
	}
}

// A multi-line selection replaced by one character.
func TestMultiLineSelectionIsReplaced(t *testing.T) {
	e, _ := newTestEditor(t, "one", "two", "three", "four")
	e.Active().Pt = text.Pos{Line: 1, Col: 1}
	press(t, e, "C-SPC", "C-n", "C-n") // through to line 3, column 1
	press(t, e, "z")

	if got, want := e.Buf().String(), "one\ntzour"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}
