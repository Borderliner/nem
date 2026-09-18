package text

import (
	"math/rand"
	"testing"
)

// typeRunes simulates keystrokes: one single-rune Insert per character.
func typeRunes(t *testing.T, b *Buffer, at Pos, s string) Pos {
	t.Helper()
	for _, r := range s {
		if err := b.Insert(at, []rune{r}); err != nil {
			t.Fatalf("Insert(%v, %q) error = %v", at, string(r), err)
		}
		if r == '\n' {
			at = Pos{at.Line + 1, 0}
		} else {
			at.Col++
		}
	}
	return at
}

func TestUndoCoalescesConsecutiveTyping(t *testing.T) {
	b := NewBuffer()
	typeRunes(t, b, Pos{0, 0}, "hello")
	if got := b.String(); got != "hello" {
		t.Fatalf("after typing: %q", got)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() reported nothing to undo")
	}
	if got := b.String(); got != "" {
		t.Errorf("one Undo after typing 'hello' = %q, want empty", got)
	}
}

func TestBreakUndoSplitsTypingIntoUnits(t *testing.T) {
	b := NewBuffer()
	at := typeRunes(t, b, Pos{0, 0}, "hel")
	b.BreakUndo()
	typeRunes(t, b, at, "lo")
	if got := b.String(); got != "hello" {
		t.Fatalf("after typing: %q", got)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() reported nothing to undo")
	}
	if got := b.String(); got != "hel" {
		t.Errorf("Undo() = %q, want %q", got, "hel")
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("second Undo() reported nothing to undo")
	}
	if got := b.String(); got != "" {
		t.Errorf("second Undo() = %q, want empty", got)
	}
}

func TestUndoRedoRoundTrip(t *testing.T) {
	b := bufFrom("hello world")
	if err := b.Delete(Pos{0, 5}, Pos{0, 11}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("after delete: %q", got)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if got := b.String(); got != "hello world" {
		t.Errorf("after Undo: %q, want %q", got, "hello world")
	}
	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo() failed")
	}
	if got := b.String(); got != "hello" {
		t.Errorf("after Redo: %q, want %q", got, "hello")
	}
}

func TestUndoRestoresMultilineDeletes(t *testing.T) {
	b := bufFrom("one\ntwo\nthree")
	if err := b.Delete(Pos{0, 1}, Pos{2, 2}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "oree" {
		t.Fatalf("after delete: %q", got)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if got := b.String(); got != "one\ntwo\nthree" {
		t.Errorf("after Undo: %q", got)
	}
}

func TestNewEditDiscardsRedoBranch(t *testing.T) {
	b := bufFrom("hello")
	if err := b.Delete(Pos{0, 0}, Pos{0, 5}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if got := b.String(); got != "hello" {
		t.Fatalf("after undo: %q", got)
	}
	// A fresh edit here must throw the redo branch away.
	if err := b.Insert(Pos{0, 5}, []rune("!")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	if _, ok := b.Redo(); ok {
		t.Error("Redo() succeeded after a new edit; redo branch should be gone")
	}
	if got := b.String(); got != "hello!" {
		t.Errorf("String() = %q, want %q", got, "hello!")
	}
}

func TestUndoOnEmptyLogReportsNothing(t *testing.T) {
	b := NewBuffer()
	if _, ok := b.Undo(); ok {
		t.Error("Undo() on a fresh buffer reported success")
	}
	if _, ok := b.Redo(); ok {
		t.Error("Redo() on a fresh buffer reported success")
	}
}

func TestUndoReturnsCursorLanding(t *testing.T) {
	b := bufFrom("hello")
	// undoing an insert puts point where the insert began
	if err := b.Insert(Pos{0, 2}, []rune("XYZ")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	pos, ok := b.Undo()
	if !ok {
		t.Fatal("Undo() failed")
	}
	if !pos.Equal(Pos{0, 2}) {
		t.Errorf("undo of insert landed at %v, want {0 2}", pos)
	}
	// redoing that insert puts point after the reinserted text
	pos, ok = b.Redo()
	if !ok {
		t.Fatal("Redo() failed")
	}
	if !pos.Equal(Pos{0, 5}) {
		t.Errorf("redo of insert landed at %v, want {0 5}", pos)
	}
}

func TestNewlineBreaksCoalescing(t *testing.T) {
	b := NewBuffer()
	typeRunes(t, b, Pos{0, 0}, "ab\ncd")
	if got := b.String(); got != "ab\ncd" {
		t.Fatalf("after typing: %q", got)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	// "cd" undone as its own unit; the newline ended the previous one.
	if got := b.String(); got != "ab\n" {
		t.Errorf("after Undo: %q, want %q", got, "ab\n")
	}
}

func TestUndoClearsModifiedBackToSavedState(t *testing.T) {
	b := bufFrom("hello")
	b.SetModified(false)
	if err := b.Insert(Pos{0, 5}, []rune("!")); err != nil {
		t.Fatal(err)
	}
	if !b.Modified() {
		t.Fatal("Modified() = false after edit")
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if b.Modified() {
		t.Error("Modified() = true after undoing back to the saved state")
	}
}

// bufFresh builds a buffer and then clears its history, so "undo everything"
// returns to s rather than to an empty buffer.
func bufFresh(s string) *Buffer {
	b := bufFrom(s)
	b.undo = newUndoLog()
	return b
}

func randPos(rng *rand.Rand, b *Buffer) Pos {
	ln := rng.Intn(b.NumLines())
	return Pos{ln, RuneIdx(rng.Intn(int(b.Line(ln).Len()) + 1))}
}

func randText(rng *rand.Rand) []rune {
	alphabet := []rune{'a', 'b', 'Z', '\n', '\t', '世', 'é', '́'}
	n := 1 + rng.Intn(6)
	out := make([]rune, n)
	for i := range out {
		out[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return out
}

// Undoing every edit must reproduce the original text exactly, and redoing
// them must reproduce the final text exactly. This is the property that the
// mark/savePoint adjustment arithmetic either satisfies or quietly violates.
func TestRandomEditsUndoAndRedoExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(20260918))
	for trial := 0; trial < 300; trial++ {
		b := bufFresh("alpha\nbeta\ngamma\ndelta")
		orig := b.String()

		nEdits := 1 + rng.Intn(8)
		for i := 0; i < nEdits; i++ {
			if rng.Intn(2) == 0 {
				if err := b.Insert(randPos(rng, b), randText(rng)); err != nil {
					t.Fatalf("trial %d: Insert error = %v", trial, err)
				}
			} else {
				if err := b.Delete(randPos(rng, b), randPos(rng, b)); err != nil {
					t.Fatalf("trial %d: Delete error = %v", trial, err)
				}
			}
			b.BreakUndo()
		}
		final := b.String()

		undone := 0
		for {
			if _, ok := b.Undo(); !ok {
				break
			}
			undone++
		}
		if got := b.String(); got != orig {
			t.Fatalf("trial %d: undoing all %d edits gave %q, want %q", trial, undone, got, orig)
		}

		for {
			if _, ok := b.Redo(); !ok {
				break
			}
		}
		if got := b.String(); got != final {
			t.Fatalf("trial %d: redoing all edits gave %q, want %q", trial, got, final)
		}
	}
}

// Marks must stay within the buffer no matter what edits land around them.
func TestMarkStaysInBoundsUnderRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for trial := 0; trial < 300; trial++ {
		b := bufFresh("one\ntwo\nthree\nfour")
		b.SetMark(randPos(rng, b))
		for i := 0; i < 6; i++ {
			if rng.Intn(2) == 0 {
				if err := b.Insert(randPos(rng, b), randText(rng)); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := b.Delete(randPos(rng, b), randPos(rng, b)); err != nil {
					t.Fatal(err)
				}
			}
			m := b.Mark()
			if m.Line < 0 || m.Line >= b.NumLines() {
				t.Fatalf("trial %d edit %d: mark line %d out of range (%d lines)",
					trial, i, m.Line, b.NumLines())
			}
			if m.Col < 0 || m.Col > b.Line(m.Line).Len() {
				t.Fatalf("trial %d edit %d: mark col %d out of range (line len %d)",
					trial, i, m.Col, b.Line(m.Line).Len())
			}
		}
	}
}

// Saving, undoing past the save, then editing makes the saved state
// unreachable: the buffer must report modified from then on.
func TestSavedStateCanBecomeUnreachable(t *testing.T) {
	b := bufFresh("hello")
	if err := b.Insert(Pos{0, 5}, []rune("!")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	b.SetModified(false) // saved state is "hello!"

	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if !b.Modified() {
		t.Error("Modified() = false after undoing away from the saved state")
	}

	// This edit discards the redo branch, so "hello!" can never be reached again.
	if err := b.Insert(Pos{0, 5}, []rune("?")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	if !b.Modified() {
		t.Error("Modified() = false although the saved state is unreachable")
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo() failed")
	}
	if !b.Modified() {
		t.Error("Modified() = false at a state that was never saved")
	}
}

func TestSetModifiedTrueForcesDirty(t *testing.T) {
	b := bufFresh("hello")
	if b.Modified() {
		t.Fatal("fresh buffer already modified")
	}
	b.SetModified(true)
	if !b.Modified() {
		t.Error("Modified() = false after SetModified(true)")
	}
}
