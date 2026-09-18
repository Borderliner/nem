package command_test

import (
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
)

// Helpers here are prefixed ln because package command_test is shared with
// motion_test.go, buffers_test.go, region_test.go, edit_test.go and
// search_test.go, several of which already define run, mustRun and wantPoint.

// lnSetup returns a fake seeded with lines and a registry holding the line
// commands, with point at the origin.
func lnSetup(t *testing.T, lines ...string) *commandtest.Fake {
	t.Helper()
	f := commandtest.New(lines...)
	if err := command.RegisterLines(f.Reg); err != nil {
		t.Fatalf("RegisterLines: %v", err)
	}
	f.SetPoint(text.Pos{})
	return f
}

// lnRun dispatches a command through the registry, failing on error.
func lnRun(t *testing.T, f *commandtest.Fake, name string) {
	t.Helper()
	if err := f.Run(name); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// lnSelect sets the mark at from and point at to, which is how a user builds a
// region: C-SPC at one end, move to the other.
func lnSelect(t *testing.T, f *commandtest.Fake, from, to text.Pos) {
	t.Helper()
	f.SetPoint(from)
	f.Buf().SetMark(f.Buf().ClampPos(from))
	f.SetPoint(to)
}

// lnEchoed reports whether any echo message contains want.
func lnEchoed(f *commandtest.Fake, want string) bool {
	for _, m := range f.Echoes {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// lnRegionText is the text currently between point and mark, which is what the
// user sees highlighted.
func lnRegionText(f *commandtest.Fake) string {
	b := f.Buf()
	if !b.HasMark() {
		return ""
	}
	lo, hi := text.OrderPos(f.Point(), b.Mark())
	return string(b.Text(lo, hi))
}

func TestMoveLinesDownSingleLine(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "two\none\nthree"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point().Line, 1; got != want {
		t.Errorf("point line = %d, want %d (point follows the line it moved)", got, want)
	}
}

func TestMoveLinesUpSingleLine(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	f.SetPoint(text.Pos{Line: 2})
	lnRun(t, f, "move-lines-up")
	if got, want := f.Text(), "one\nthree\ntwo"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point().Line, 1; got != want {
		t.Errorf("point line = %d, want %d", got, want)
	}
}

// The column must not move, so the cursor stays where the eye expects it.
func TestMoveLinesPreservesColumn(t *testing.T) {
	f := lnSetup(t, "abcdef", "ghijkl", "mnopqr")
	f.SetPoint(text.Pos{Line: 1, Col: 4})
	lnRun(t, f, "move-lines-up")
	if got, want := (text.Pos{Line: 0, Col: 4}), f.Point(); got != want {
		t.Errorf("point = %v, want %v", want, got)
	}
}

// A region spanning two lines moves as a block, and the same text stays
// selected afterwards. A naive implementation loses the region after the first
// move, so the second then moves the wrong line - hence two successive moves.
func TestMoveLinesKeepsTheRegionAcrossSuccessiveMoves(t *testing.T) {
	f := lnSetup(t, "one", "two", "three", "four", "five")
	lnSelect(t, f, text.Pos{Line: 1}, text.Pos{Line: 2, Col: 5})

	before := lnRegionText(f)
	if before != "two\nthree" {
		t.Fatalf("setup: region = %q, want %q", before, "two\nthree")
	}

	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "one\nfour\ntwo\nthree\nfive"; got != want {
		t.Fatalf("after one move text = %q, want %q", got, want)
	}
	if got := lnRegionText(f); got != before {
		t.Errorf("after one move region = %q, want %q still selected", got, before)
	}

	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "one\nfour\nfive\ntwo\nthree"; got != want {
		t.Errorf("after two moves text = %q, want %q", got, want)
	}
	if got := lnRegionText(f); got != before {
		t.Errorf("after two moves region = %q, want %q still selected", got, before)
	}
}

func TestMoveLinesUpKeepsTheRegion(t *testing.T) {
	f := lnSetup(t, "one", "two", "three", "four")
	lnSelect(t, f, text.Pos{Line: 2}, text.Pos{Line: 3, Col: 4})
	before := lnRegionText(f)

	lnRun(t, f, "move-lines-up")
	if got, want := f.Text(), "one\nthree\nfour\ntwo"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got := lnRegionText(f); got != before {
		t.Errorf("region = %q, want %q", got, before)
	}
}

// A region ending at column 0 does not include that line: C-a C-SPC C-n is a
// very common way to select exactly one line, and moving two would be wrong.
func TestRegionEndingAtColumnZeroExcludesThatLine(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	lnSelect(t, f, text.Pos{Line: 0}, text.Pos{Line: 1, Col: 0})
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "two\none\nthree"; got != want {
		t.Errorf("text = %q, want %q (only line 1 should have moved)", got, want)
	}
}

// Three moves must cost exactly three undos, and the text must come back
// byte-identical. Without undo grouping each move is a delete plus an insert and
// this costs six - which is the reason grouping was built.
func TestThreeMovesUndoInThreeSteps(t *testing.T) {
	f := lnSetup(t, "one", "two", "three", "four")
	original := f.Text()

	for i := 0; i < 3; i++ {
		lnRun(t, f, "move-lines-down")
	}
	if f.Text() == original {
		t.Fatal("setup: three moves changed nothing")
	}

	b := f.Buf()
	for i := 0; i < 3; i++ {
		if _, ok := b.Undo(); !ok {
			t.Fatalf("undo %d reported nothing to undo", i+1)
		}
	}
	// Three undos suffice only if each move is one unit. Ungrouped, a move is a
	// delete plus an insert, so three moves would be six units and three undos
	// would leave the text partly reverted.
	if got := f.Text(); got != original {
		t.Errorf("after three undos text = %q, want the original %q", got, original)
	}
}

func TestMoveLinesUpAtTopRefuses(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	lnSelect(t, f, text.Pos{Line: 0}, text.Pos{Line: 0, Col: 3})
	before, region := f.Text(), lnRegionText(f)

	lnRun(t, f, "move-lines-up")

	if got := f.Text(); got != before {
		t.Errorf("text = %q, want it byte-identical at %q", got, before)
	}
	if got := lnRegionText(f); got != region {
		t.Errorf("region = %q, want %q intact after a refusal", got, region)
	}
	if !lnEchoed(f, "Beginning of buffer") {
		t.Errorf("echoes = %v, want a beginning-of-buffer message", f.Echoes)
	}
}

func TestMoveLinesDownAtBottomRefuses(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	f.SetPoint(text.Pos{Line: 2})
	before := f.Text()

	lnRun(t, f, "move-lines-down")

	if got := f.Text(); got != before {
		t.Errorf("text = %q, want it byte-identical at %q", got, before)
	}
	if !lnEchoed(f, "End of buffer") {
		t.Errorf("echoes = %v, want an end-of-buffer message", f.Echoes)
	}
}

// A refused move must not record an undo unit - pressing the key at the edge
// should not cost a C-/ that appears to do nothing.
//
// Checked by consequence: the fake's seeding insert is the only unit in the log,
// so if the refusal added none, the first undo reverts the seed and empties the
// buffer. If the refusal recorded an empty unit, that undo is consumed by it and
// the text still reads "one\ntwo".
func TestRefusedMoveRecordsNoUndoUnit(t *testing.T) {
	f := lnSetup(t, "one", "two")
	lnRun(t, f, "move-lines-up")
	if _, ok := f.Buf().Undo(); !ok {
		t.Fatal("nothing to undo at all; the fake should have a seeding unit")
	}
	if got := f.Text(); got != "" {
		t.Errorf("text = %q after one undo, want %q - the refused move left a unit behind", got, "")
	}
}

func TestMoveLinesWithArgument(t *testing.T) {
	f := lnSetup(t, "one", "two", "three", "four", "five")
	f.ArgN, f.ArgExplicit = 3, true
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "two\nthree\nfour\none\nfive"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point().Line, 3; got != want {
		t.Errorf("point line = %d, want %d", got, want)
	}
}

// An argument that overshoots moves as far as it can and reports, rather than
// refusing outright: C-u 20 M-<down> should land the line at the bottom.
func TestArgumentPastTheEdgeMovesAsFarAsItCan(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	f.ArgN, f.ArgExplicit = 20, true
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "two\nthree\none"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if !lnEchoed(f, "End of buffer") {
		t.Errorf("echoes = %v, want an end-of-buffer report after a partial move", f.Echoes)
	}
}

// A negative argument reverses direction, as it does for the motion commands.
func TestNegativeArgumentReversesDirection(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	f.SetPoint(text.Pos{Line: 2})
	f.ArgN, f.ArgExplicit = -1, true
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "one\nthree\ntwo"; got != want {
		t.Errorf("text = %q, want %q (a negative argument moves up)", got, want)
	}
}

// Moving the final line must not disturb the buffer's trailing-newline
// metadata, which text stores separately rather than as an empty last line.
func TestMovingTheFinalLineKeepsTextIntact(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	f.SetPoint(text.Pos{Line: 2})
	lnRun(t, f, "move-lines-up")
	if got, want := f.Text(), "one\nthree\ntwo"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	// And back again, which exercises inserting past the new last line.
	f.SetPoint(text.Pos{Line: 1})
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "one\ntwo\nthree"; got != want {
		t.Errorf("round trip text = %q, want the original %q", got, want)
	}
}

func TestMoveLinesOnASingleLineBuffer(t *testing.T) {
	for _, name := range []string{"move-lines-up", "move-lines-down"} {
		t.Run(name, func(t *testing.T) {
			f := lnSetup(t, "only")
			lnRun(t, f, name)
			if got, want := f.Text(), "only"; got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
		})
	}
}

// An empty region - mark exactly at point - behaves like no region at all.
func TestEmptyRegionMovesPointsLine(t *testing.T) {
	f := lnSetup(t, "one", "two", "three")
	lnSelect(t, f, text.Pos{Line: 1}, text.Pos{Line: 1})
	lnRun(t, f, "move-lines-down")
	if got, want := f.Text(), "one\nthree\ntwo"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestBothCommandsAreRegisteredAndDocumented(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterLines(r); err != nil {
		t.Fatalf("RegisterLines: %v", err)
	}
	for _, name := range []string{"move-lines-up", "move-lines-down"} {
		c, ok := r.Lookup(name)
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if !c.Interactive {
			t.Errorf("%s is not interactive, so M-x cannot reach it", name)
		}
		if c.Doc == "" {
			t.Errorf("%s has no doc string", name)
		}
	}
}
