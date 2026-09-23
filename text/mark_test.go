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

// Setting the mark is not selecting. Yank and M-< set the mark as a bookmark,
// and if that alone made the region live, the next keystroke would replace
// everything between it and point.
func TestSetMarkDoesNotActivateTheRegion(t *testing.T) {
	b := NewBuffer()
	b.SetMark(Pos{Line: 0, Col: 0})
	if !b.HasMark() {
		t.Fatal("HasMark() = false after SetMark, want true")
	}
	if b.MarkActive() {
		t.Error("MarkActive() = true after a bare SetMark, want false")
	}
}

// Deactivating ends the selection but keeps the mark, so C-x C-x can still
// return to it.
func TestActivateAndDeactivateKeepTheMark(t *testing.T) {
	b := bufFrom("hello")
	b.SetMark(Pos{Line: 0, Col: 3})

	b.ActivateMark()
	if !b.MarkActive() {
		t.Fatal("MarkActive() = false after ActivateMark, want true")
	}

	b.DeactivateMark()
	if b.MarkActive() {
		t.Error("MarkActive() = true after DeactivateMark, want false")
	}
	if !b.HasMark() {
		t.Error("DeactivateMark discarded the mark, want it kept")
	}
	if got, want := b.Mark(), (Pos{Line: 0, Col: 3}); got != want {
		t.Errorf("Mark() = %v after DeactivateMark, want %v", got, want)
	}
}

// There is no region without a mark, so activating one must not invent it.
func TestActivateMarkWithoutAMarkDoesNothing(t *testing.T) {
	b := NewBuffer()
	b.ActivateMark()
	if b.MarkActive() || b.HasMark() {
		t.Errorf("ActivateMark on an unmarked buffer: MarkActive() = %v, HasMark() = %v, want both false",
			b.MarkActive(), b.HasMark())
	}
}

// Clearing the mark ends the selection too, and a later SetMark must not bring
// the old active state back with it.
func TestClearMarkDeactivates(t *testing.T) {
	b := NewBuffer()
	b.SetMark(Pos{})
	b.ActivateMark()

	b.ClearMark()
	if b.MarkActive() {
		t.Error("MarkActive() = true after ClearMark, want false")
	}

	b.SetMark(Pos{})
	if b.MarkActive() {
		t.Error("SetMark after ClearMark revived the old active region")
	}
}
