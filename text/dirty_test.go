package text

import "testing"

// A fresh buffer reports nothing dirty, so a cache attaching to an untouched
// buffer does no work.
func TestFreshBufferHasNothingDirty(t *testing.T) {
	b := NewBuffer()
	from, delta, rev := b.TakeDirty()
	if from < b.NumLines() {
		t.Errorf("TakeDirty from = %d, want >= NumLines (%d): nothing has been edited", from, b.NumLines())
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0", delta)
	}
	if rev != 0 {
		t.Errorf("rev = %d, want 0 on a fresh buffer", rev)
	}
}

func TestInsertReportsTheLineItTouched(t *testing.T) {
	b := seed(t, "one", "two", "three")
	b.TakeDirty() // consume the seeding

	if err := b.Insert(Pos{Line: 1, Col: 1}, []rune("X")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	from, delta, rev := b.TakeDirty()
	if from != 1 {
		t.Errorf("from = %d, want 1", from)
	}
	if delta != 0 {
		t.Errorf("delta = %d, want 0 for an edit within one line", delta)
	}
	if rev == 0 {
		t.Error("rev did not advance")
	}
}

// A newline adds a line, and the cache needs the count change to keep its
// per-line states aligned with the buffer.
func TestInsertingNewlinesReportsTheLineDelta(t *testing.T) {
	b := seed(t, "one", "two")
	b.TakeDirty()

	if err := b.Insert(Pos{Line: 0, Col: 3}, []rune("\nA\nB")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	from, delta, _ := b.TakeDirty()
	if from != 0 {
		t.Errorf("from = %d, want 0", from)
	}
	if delta != 2 {
		t.Errorf("delta = %d, want 2 lines added", delta)
	}
}

func TestDeletingLinesReportsANegativeDelta(t *testing.T) {
	b := seed(t, "one", "two", "three", "four")
	b.TakeDirty()

	if err := b.Delete(Pos{Line: 1, Col: 0}, Pos{Line: 3, Col: 0}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	from, delta, _ := b.TakeDirty()
	if from != 1 {
		t.Errorf("from = %d, want 1", from)
	}
	if delta != -2 {
		t.Errorf("delta = %d, want -2 lines removed", delta)
	}
}

// TakeDirty consumes: a second call with no edit in between reports nothing.
func TestTakeDirtyConsumes(t *testing.T) {
	b := seed(t, "one", "two")
	b.TakeDirty()

	if err := b.Insert(Pos{Line: 0, Col: 0}, []rune("X")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if from, _, _ := b.TakeDirty(); from != 0 {
		t.Fatalf("first take from = %d, want 0", from)
	}
	from, delta, _ := b.TakeDirty()
	if from < b.NumLines() {
		t.Errorf("second take from = %d, want nothing dirty", from)
	}
	if delta != 0 {
		t.Errorf("second take delta = %d, want 0", delta)
	}
}

// Several edits between reads accumulate to the lowest line touched, because
// everything below the lowest may have been affected.
func TestSeveralEditsAccumulateToTheLowestLine(t *testing.T) {
	b := seed(t, "one", "two", "three", "four")
	b.TakeDirty()

	if err := b.Insert(Pos{Line: 3, Col: 0}, []rune("X")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := b.Insert(Pos{Line: 1, Col: 0}, []rune("Y")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	from, _, _ := b.TakeDirty()
	if from != 1 {
		t.Errorf("from = %d, want the lowest line touched (1)", from)
	}
}

// Undo and redo reach insertRaw and deleteRaw directly, bypassing Insert and
// Delete. Tracking the public methods alone would leave a cache believing the
// buffer was untouched after an undo, and the colours would be wrong until the
// next edit happened to invalidate the same region.
func TestUndoAndRedoAreTracked(t *testing.T) {
	b := seed(t, "one", "two", "three")
	if err := b.Insert(Pos{Line: 2, Col: 0}, []rune("X")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.TakeDirty()

	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo reported nothing to undo")
	}
	if from, _, _ := b.TakeDirty(); from != 2 {
		t.Errorf("after Undo from = %d, want 2", from)
	}

	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo reported nothing to redo")
	}
	if from, _, _ := b.TakeDirty(); from != 2 {
		t.Errorf("after Redo from = %d, want 2", from)
	}
}

// Revision advances once per mutation, which is what lets a reader tell "one
// edit happened, and I know exactly where" from "several did, trust nothing".
func TestRevisionAdvancesOncePerMutation(t *testing.T) {
	b := seed(t, "one")
	_, _, start := b.TakeDirty()

	for i := 0; i < 3; i++ {
		if err := b.Insert(Pos{Line: 0, Col: 0}, []rune("X")); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	_, _, end := b.TakeDirty()
	if end-start != 3 {
		t.Errorf("revision advanced by %d over 3 edits, want 3", end-start)
	}
}

// A no-op edit must not advance the revision, or a cache would re-lex on every
// keystroke that inserted nothing.
func TestNoOpEditsDoNotAdvanceTheRevision(t *testing.T) {
	b := seed(t, "one")
	_, _, start := b.TakeDirty()

	if err := b.Insert(Pos{Line: 0, Col: 0}, nil); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := b.Delete(Pos{Line: 0, Col: 1}, Pos{Line: 0, Col: 1}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, end := b.TakeDirty(); end != start {
		t.Errorf("revision advanced from %d to %d on no-op edits", start, end)
	}
}

// seed returns a buffer holding lines.
func seed(t *testing.T, lines ...string) *Buffer {
	t.Helper()
	b := NewBuffer()
	s := ""
	for i, l := range lines {
		if i > 0 {
			s += "\n"
		}
		s += l
	}
	if s != "" {
		if err := b.Insert(Pos{}, []rune(s)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	b.BreakUndo()
	return b
}
