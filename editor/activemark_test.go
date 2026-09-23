package editor

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// A mark can be SET without the region being ACTIVE. Yank and the buffer-edge
// jumps set the mark so C-x C-x can return or select, but the text between that
// mark and point is not a selection. Treating it as one made typing after those
// commands delete text the user never selected: M-> M-< x wiped the whole file.
//
// These tests are the reported cases, by name, followed by the intended
// behaviour the fix must preserve.

// reversedCells counts reverse-video cells in the text area, which is how the
// region is drawn. The modeline and echo rows are excluded.
func reversedCells(t *testing.T, e *Editor, scr tcell.SimulationScreen) int {
	t.Helper()
	e.Redraw()
	cells, w, h := scr.GetContents()
	n := 0
	for y := 0; y < h-2; y++ { // last two rows: modeline and echo area
		for x := 0; x < w; x++ {
			_, _, attr := cells[y*w+x].Style.Decompose()
			if attr&tcell.AttrReverse != 0 {
				n++
			}
		}
	}
	return n
}

// --- the reported bugs -------------------------------------------------------

func TestJumpToEndThenTopThenTypeKeepsTheFile(t *testing.T) {
	e, _ := newTestEditor(t, "one", "two", "three", "four")
	press(t, e, "M->", "M-<", "x")

	if got, want := e.Buf().String(), "xone\ntwo\nthree\nfour"; got != want {
		t.Errorf("M-> M-< x gave %q, want %q - typing after a buffer jump "+
			"deleted text the user never selected", got, want)
	}
}

func TestTypingAfterYankKeepsTheYank(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	press(t, e, "C-k", "C-y", "x")

	if got, want := e.Buf().String(), "abcx"; got != want {
		t.Errorf("C-k C-y x gave %q, want %q", got, want)
	}
}

func TestYankTwiceInsertsTwice(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	press(t, e, "C-k", "C-y", "C-y")

	if got, want := e.Buf().String(), "abcabc"; got != want {
		t.Errorf("C-k C-y C-y gave %q, want %q", got, want)
	}
}

func TestYankedTextIsNotHighlighted(t *testing.T) {
	e, scr := newTestEditor(t, "abc")
	press(t, e, "C-k", "C-y")

	if e.Buf().MarkActive() {
		t.Error("the region is active after C-y; yanked text is not a selection")
	}
	if n := reversedCells(t, e, scr); n != 0 {
		t.Errorf("%d cells drawn as selected after C-y, want none", n)
	}
	if !e.Buf().HasMark() {
		t.Error("yank no longer sets the mark, so C-x C-x cannot select what was yanked")
	}
}

// --- what must keep working --------------------------------------------------

func TestSelectingThenTypingReplacesTheSelection(t *testing.T) {
	e, scr := newTestEditor(t, "alpha beta")
	press(t, e, "C-SPC", "M-f")

	if !e.Buf().MarkActive() {
		t.Fatal("C-SPC did not activate the region")
	}
	if n := reversedCells(t, e, scr); n == 0 {
		t.Error("an active selection is not drawn")
	}
	press(t, e, "x")
	if got, want := e.Buf().String(), "x beta"; got != want {
		t.Errorf("typing over a selection gave %q, want %q", got, want)
	}
}

func TestSelectAllThenBackspaceStillEmptiesTheBuffer(t *testing.T) {
	e, _ := newTestEditor(t, "alpha", "beta")
	press(t, e, "C-x", "h", "<backspace>")

	if got := e.Buf().String(); got != "" {
		t.Errorf("C-x h Backspace left %q, want an empty buffer", got)
	}
}

// C-x C-x is how you turn a yank into a selection, deliberately.
func TestExchangeActivatesTheYankedText(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	press(t, e, "C-k", "C-y", "C-x", "C-x")

	if !e.Buf().MarkActive() {
		t.Fatal("C-x C-x did not activate the region")
	}
	press(t, e, "x")
	if got, want := e.Buf().String(), "x"; got != want {
		t.Errorf("C-y C-x C-x x gave %q, want %q - the yanked text should have been "+
			"replaced once C-x C-x made it a selection", got, want)
	}
}

// C-w uses the mark even when the region is inactive, as emacs does by default,
// so what was just yanked can still be killed.
func TestKillRegionWorksOnAnInactiveRegion(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	press(t, e, "C-k", "C-y")
	if e.Buf().MarkActive() {
		t.Fatal("precondition: the region should be inactive after C-y")
	}
	press(t, e, "C-w")

	if got := e.Buf().String(); got != "" {
		t.Errorf("C-y C-w left %q, want the yanked text killed", got)
	}
	if e.Buf().MarkActive() {
		t.Error("the region is active after C-w")
	}
}

func TestCopyLeavesTheRegionInactive(t *testing.T) {
	e, _ := newTestEditor(t, "alpha beta")
	press(t, e, "C-SPC", "M-f", "M-w")

	if e.Buf().MarkActive() {
		t.Error("the region is still active after M-w")
	}
	if got, want := e.Buf().String(), "alpha beta"; got != want {
		t.Errorf("M-w changed the buffer to %q", got)
	}
}

// Motion extends an active region rather than ending it.
func TestMotionKeepsTheRegionActive(t *testing.T) {
	e, scr := newTestEditor(t, "one", "two", "three", "four")
	press(t, e, "C-SPC", "C-n", "C-n", "C-n")

	if !e.Buf().MarkActive() {
		t.Fatal("motion deactivated the region")
	}
	if n := reversedCells(t, e, scr); n == 0 {
		t.Error("an active region spanning three lines is not drawn")
	}
}

// A buffer jump with an active region extends it instead of re-anchoring the
// mark - "select from here to the end of the file" is C-SPC M->.
func TestBufferJumpExtendsAnActiveRegion(t *testing.T) {
	e, _ := newTestEditor(t, "one", "two", "three")
	e.Active().Pt = text.Pos{Line: 1, Col: 0}
	press(t, e, "C-SPC", "M->")

	if !e.Buf().MarkActive() {
		t.Fatal("M-> deactivated the region")
	}
	if got, want := e.Buf().Mark(), (text.Pos{Line: 1, Col: 0}); got != want {
		t.Errorf("mark = %v after C-SPC M->, want it left where C-SPC set it (%v)", got, want)
	}
	press(t, e, "<backspace>")
	if got, want := e.Buf().String(), "one\n"; got != want {
		t.Errorf("deleting the selection gave %q, want %q", got, want)
	}
}

func TestMoveLinesKeepsTheSelectionAcrossRepeats(t *testing.T) {
	e, _ := newTestEditor(t, "a", "b", "c", "d")
	press(t, e, "C-SPC", "C-n") // select line "a" (point at start of "b")
	press(t, e, "M-<down>", "M-<down>")

	if got, want := e.Buf().String(), "b\nc\na\nd"; got != want {
		t.Errorf("two moves gave %q, want %q", got, want)
	}
	if !e.Buf().MarkActive() {
		t.Error("move-lines deactivated the selection, so the key cannot repeat")
	}
}

// With no active region, move-lines moves the line point is on - even if an
// inactive mark sits elsewhere, as it does after a yank.
func TestMoveLinesIgnoresAnInactiveMark(t *testing.T) {
	e, _ := newTestEditor(t, "a", "b", "c")
	press(t, e, "M->", "M-<") // mark left at the end, inactive; point at line 0
	press(t, e, "M-<down>")

	if got, want := e.Buf().String(), "b\na\nc"; got != want {
		t.Errorf("M-<down> with only an inactive mark gave %q, want %q - it should "+
			"move one line, not the span to the mark", got, want)
	}
}

func TestKeyboardQuitDeactivatesButKeepsTheMark(t *testing.T) {
	e, _ := newTestEditor(t, "alpha beta")
	press(t, e, "C-SPC", "M-f", "C-g")

	if e.Buf().MarkActive() {
		t.Error("the region is still active after C-g")
	}
	if !e.Buf().HasMark() {
		t.Fatal("C-g discarded the mark; C-x C-x can no longer return to it")
	}
	press(t, e, "x")
	if got, want := e.Buf().String(), "alphax beta"; got != want {
		t.Errorf("typing after C-g gave %q, want %q - nothing should be deleted", got, want)
	}
	press(t, e, "C-x", "C-x")
	if got, want := e.Active().Pt, (text.Pos{}); got != want {
		t.Errorf("C-x C-x after C-g went to %v, want the mark at %v", got, want)
	}
}

// Any command that changes the buffer ends the selection, as in emacs.
func TestAnEditDeactivatesTheRegion(t *testing.T) {
	for _, keys := range [][]string{
		{"C-d"}, // delete-char is a delete-selection command, but...
		{"C-/"}, // undo changes the buffer
		{"C-o"}, // open-line
		{"C-t"}, // transpose-chars
	} {
		t.Run(strings.Join(keys, " "), func(t *testing.T) {
			e, _ := newTestEditor(t, "alpha beta gamma")
			press(t, e, "x", "y") // something to undo
			e.Active().Pt = text.Pos{Line: 0, Col: 3}
			press(t, e, "C-SPC", "C-f", "C-f")
			if !e.Buf().MarkActive() {
				t.Fatal("precondition: region should be active")
			}
			press(t, e, keys...)
			if e.Buf().MarkActive() {
				t.Errorf("%v changed the buffer but left the region active", keys)
			}
		})
	}
}
