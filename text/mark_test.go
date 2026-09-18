package text

import "testing"

// A fresh buffer has no mark. This is the distinction that keeps C-w from
// killing from the buffer start to point in a buffer the user never marked.
func TestFreshBufferHasNoMark(t *testing.T) {
	if NewBuffer().HasMark() {
		t.Error("NewBuffer().HasMark() = true, want false")
	}
}

// Setting the mark at the zero position must still count as having a mark —
// that is the case a bare Mark() == Pos{} check gets wrong.
func TestMarkAtOriginStillCountsAsSet(t *testing.T) {
	b := NewBuffer()
	b.SetMark(Pos{})
	if !b.HasMark() {
		t.Error("HasMark() = false after SetMark(Pos{}), want true")
	}
	if got := b.Mark(); got != (Pos{}) {
		t.Errorf("Mark() = %v, want zero Pos", got)
	}
}

func TestClearMarkForgetsIt(t *testing.T) {
	b := NewBuffer()
	b.SetMark(Pos{Line: 0, Col: 3})
	b.ClearMark()
	if b.HasMark() {
		t.Error("HasMark() = true after ClearMark(), want false")
	}
	if got := b.Mark(); got != (Pos{}) {
		t.Errorf("Mark() = %v after ClearMark(), want zero Pos", got)
	}
}

// Edits adjust the mark, and must not resurrect a cleared one.
func TestEditsDoNotResurrectClearedMark(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("hello")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if b.HasMark() {
		t.Error("HasMark() = true after an edit on an unmarked buffer, want false")
	}
}
