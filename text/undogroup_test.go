package text

import "testing"

// moveLineDown performs the edit shape that motivated undo grouping: a line is
// deleted and re-inserted lower down. Two primitives, one user action.
func moveLineDown(t *testing.T, b *Buffer) {
	t.Helper()
	b.BeginUndoGroup()
	defer b.EndUndoGroup()

	// Remove "one\n" from the top.
	if err := b.Delete(Pos{0, 0}, Pos{1, 0}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Put it back after what is now the first line.
	if err := b.Insert(Pos{1, 0}, []rune("one\n")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func TestMoveLinesShapeIsOneUndo(t *testing.T) {
	b := bufFresh("one\ntwo\nthree")
	moveLineDown(t, b)

	if got, want := b.String(), "two\none\nthree"; got != want {
		t.Fatalf("after move: %q, want %q", got, want)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo reported nothing to undo")
	}
	if got, want := b.String(), "one\ntwo\nthree"; got != want {
		t.Errorf("after one Undo: %q, want %q — the group did not undo as a unit", got, want)
	}
	if _, ok := b.Undo(); ok {
		t.Error("a second Undo succeeded; the group should have been the only unit")
	}
}

func TestUndoGroupRedoesAsOneUnit(t *testing.T) {
	b := bufFresh("one\ntwo\nthree")
	moveLineDown(t, b)
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo reported nothing to redo")
	}
	if got, want := b.String(), "two\none\nthree"; got != want {
		t.Errorf("after one Redo: %q, want %q — the group did not redo as a unit", got, want)
	}
	if _, ok := b.Redo(); ok {
		t.Error("a second Redo succeeded; the group should have been the only unit")
	}
}

// Point lands where the user's edit began — the position of the earliest
// primitive in the group — for both undo and redo.
func TestUndoGroupPointLandsWhereTheEditBegan(t *testing.T) {
	b := bufFresh("abcdefgh")

	b.BeginUndoGroup()
	if err := b.Delete(Pos{0, 0}, Pos{0, 3}); err != nil { // drop "abc"
		t.Fatalf("Delete: %v", err)
	}
	if err := b.Insert(Pos{0, 2}, []rune("XYZ")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup()

	land, ok := b.Undo()
	if !ok {
		t.Fatal("Undo failed")
	}
	if want := (Pos{0, 0}); land != want {
		t.Errorf("undo landed at %v, want %v (start of the earliest primitive)", land, want)
	}

	land, ok = b.Redo()
	if !ok {
		t.Fatal("Redo failed")
	}
	if want := (Pos{0, 0}); land != want {
		t.Errorf("redo landed at %v, want %v (start of the earliest primitive)", land, want)
	}
}

// Groups nest by depth. An inner End must not close the unit, or a helper that
// groups its own edits would split its caller's group in two.
func TestUndoGroupsNest(t *testing.T) {
	b := bufFresh("base")

	b.BeginUndoGroup()
	if err := b.Insert(b.End(), []rune("-one")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.BeginUndoGroup()
	if err := b.Insert(b.End(), []rune("-two")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup() // inner: must close nothing
	if err := b.Insert(b.End(), []rune("-three")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup() // outer: closes the unit

	if got, want := b.String(), "base-one-two-three"; got != want {
		t.Fatalf("after nested group: %q, want %q", got, want)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "base"; got != want {
		t.Errorf("after one Undo: %q, want %q — nesting split the unit", got, want)
	}
}

func TestEndUndoGroupWithoutBeginIsNoOp(t *testing.T) {
	b := bufFresh("hello")
	b.EndUndoGroup() // must not panic
	b.EndUndoGroup()

	if err := b.Insert(b.End(), []rune("!")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed after a stray EndUndoGroup")
	}
	if got, want := b.String(), "hello"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The realistic failure: a command opens a group and returns an error before
// closing it. Dispatch calls BreakUndo between commands, which must abandon the
// group so later edits cannot join it. Otherwise one C-/ undoes the session.
func TestAbandonedGroupDoesNotSwallowLaterEdits(t *testing.T) {
	b := bufFresh("x")

	b.BeginUndoGroup()
	if err := b.Insert(b.End(), []rune("-inside")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// No EndUndoGroup: the command bailed out.
	b.BreakUndo()

	if err := b.Insert(b.End(), []rune("-after")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "x-inside"; got != want {
		t.Errorf("after one Undo: %q, want %q — the later edit joined the abandoned group", got, want)
	}
}

// An abandoned group is still a group: what it did record undoes together.
func TestAbandonedGroupStillUndoesAsAUnit(t *testing.T) {
	b := bufFresh("x")

	b.BeginUndoGroup()
	if err := b.Insert(b.End(), []rune("-a")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := b.Insert(b.End(), []rune("-b")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// Never closed.

	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "x"; got != want {
		t.Errorf("after one Undo: %q, want %q — the abandoned group did not undo as a unit", got, want)
	}
}

// Grouping must not leak into ordinary typing either side of it.
func TestGroupDoesNotDisableCoalescingOutsideIt(t *testing.T) {
	b := bufFresh("")

	at := typeRunes(t, b, Pos{0, 0}, "abc")

	b.BeginUndoGroup()
	if err := b.Insert(at, []rune("[G]")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup()
	at.Col += 3

	typeRunes(t, b, at, "xyz")

	// Three units: "abc", the group, "xyz".
	for _, want := range []string{"abc[G]", "abc", ""} {
		if _, ok := b.Undo(); !ok {
			t.Fatalf("Undo failed before reaching %q", want)
		}
		if got := b.String(); got != want {
			t.Fatalf("after Undo: %q, want %q", got, want)
		}
	}
}

// A single rune inserted just after prior typing normally coalesces into it.
// Inside a group it must not, or the group would swallow the earlier typing.
func TestCoalescingDoesNotCrossAGroupBoundary(t *testing.T) {
	b := bufFresh("")
	at := typeRunes(t, b, Pos{0, 0}, "ab")

	b.BeginUndoGroup()
	if err := b.Insert(at, []rune{'c'}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup()

	if got, want := b.String(), "abc"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "ab"; got != want {
		t.Errorf("after Undo: %q, want %q — the grouped rune merged into earlier typing", got, want)
	}
}

// Saving in the middle of a group makes that exact state unreachable, because
// undo now moves in group-sized steps. The buffer must report modified from then
// on rather than claiming to match the file.
func TestSaveMidGroupThenUndoPastIt(t *testing.T) {
	b := bufFresh("start")

	b.BeginUndoGroup()
	if err := b.Insert(b.End(), []rune("-one")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.SetModified(false) // as a save mid-group would
	if err := b.Insert(b.End(), []rune("-two")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	b.EndUndoGroup()

	if !b.Modified() {
		t.Error("Modified() = false after editing past the saved point")
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "start"; got != want {
		t.Fatalf("after Undo: %q, want %q", got, want)
	}
	if !b.Modified() {
		t.Error("Modified() = false after undoing past a mid-group save; that state is unreachable")
	}
	if _, ok := b.Redo(); !ok {
		t.Fatal("Redo failed")
	}
	if !b.Modified() {
		t.Error("Modified() = false after redoing; the mid-group saved state is still unreachable")
	}
}

func TestNewEditAfterUndoDiscardsRedoBranchForGroups(t *testing.T) {
	b := bufFresh("one\ntwo\nthree")
	moveLineDown(t, b)
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}

	if err := b.Insert(Pos{0, 0}, []rune("NEW")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, ok := b.Redo(); ok {
		t.Error("Redo succeeded after a fresh edit; the group's redo branch should have been discarded")
	}
}

func TestMarkAdjustsAcrossGroupedEdits(t *testing.T) {
	b := bufFresh("one\ntwo\nthree")
	b.SetMark(Pos{2, 0}) // on "three"

	moveLineDown(t, b)
	if got := b.Mark(); got.Line >= b.NumLines() || got.Line < 0 {
		t.Errorf("mark %v out of range after a grouped edit", got)
	}

	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got := b.Mark(); got.Line >= b.NumLines() || got.Line < 0 {
		t.Errorf("mark %v out of range after undoing a grouped edit", got)
	}
}

// Two adjacent groups must stay separate units. A boolean group flag rather
// than an identity would fuse them.
func TestAdjacentGroupsStaySeparate(t *testing.T) {
	b := bufFresh("base")

	for _, s := range []string{"-one", "-two"} {
		b.BeginUndoGroup()
		if err := b.Insert(b.End(), []rune(s)); err != nil {
			t.Fatalf("Insert: %v", err)
		}
		if err := b.Insert(b.End(), []rune("!")); err != nil {
			t.Fatalf("Insert: %v", err)
		}
		b.EndUndoGroup()
	}

	if got, want := b.String(), "base-one!-two!"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, ok := b.Undo(); !ok {
		t.Fatal("Undo failed")
	}
	if got, want := b.String(), "base-one!"; got != want {
		t.Errorf("after one Undo: %q, want %q — adjacent groups fused into one unit", got, want)
	}
}
