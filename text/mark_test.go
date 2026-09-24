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

// Setting the mark remembers the one it replaces; popping walks back through
// them and round again, and edits keep every remembered position on its text.
func TestMarkRing(t *testing.T) {
	b := bufFrom("0123456789")
	b.SetMark(Pos{0, 1})
	b.SetMark(Pos{0, 5})
	b.SetMark(Pos{0, 8})

	var got []RuneIdx
	for range 4 {
		p, ok := b.PopMark()
		if !ok {
			t.Fatal("PopMark with a mark set reported none")
		}
		got = append(got, p.Col)
	}
	if want := []RuneIdx{8, 5, 1, 8}; !equalCols(got, want) {
		t.Errorf("popped %v, want %v", got, want)
	}

	// Insert two runes at the start: every remembered position moves with
	// its text.
	if err := b.Insert(Pos{}, []rune("ab")); err != nil {
		t.Fatal(err)
	}
	got = got[:0]
	for range 3 {
		p, _ := b.PopMark()
		got = append(got, p.Col)
	}
	if want := []RuneIdx{7, 3, 10}; !equalCols(got, want) {
		t.Errorf("after an insert, popped %v, want %v", got, want)
	}
}

func TestPopMarkWithNoMark(t *testing.T) {
	if _, ok := bufFrom("x").PopMark(); ok {
		t.Error("PopMark on a buffer with no mark reported one")
	}
}

func equalCols(a, b []RuneIdx) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
