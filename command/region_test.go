package command_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
)

// regionSetup returns a registry holding only the region commands, plus a fake
// whose buffer holds lines.
func regionSetup(t *testing.T, lines ...string) (*command.Registry, *commandtest.Fake) {
	t.Helper()
	r := command.NewRegistry()
	if err := command.RegisterRegion(r); err != nil {
		t.Fatalf("RegisterRegion: %v", err)
	}
	return r, commandtest.New(lines...)
}

// runRegion invokes a command and records it as the last command, standing in
// for the editor's dispatcher. It deliberately does not break the kill run: that is
// also dispatch's job, and baking it in here would hide accumulation bugs.
func runRegion(t *testing.T, r *command.Registry, f *commandtest.Fake, name string) {
	t.Helper()
	if err := r.Run(name, f); err != nil {
		t.Fatalf("run %s: %v", name, err)
	}
	f.SetLastCommand(name)
}

// regionEntries seeds the ring with separate entries, breaking the run between
// each so they do not accumulate into one.
func regionEntries(f *commandtest.Fake, ss ...string) {
	for i, s := range ss {
		if i > 0 {
			f.Ring().BreakRun()
		}
		f.KillForward(s)
	}
}

func regionHasEcho(f *commandtest.Fake, substr string) bool {
	for _, e := range f.Echoes {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestRegisterRegionRegistersEveryCommand(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterRegion(r); err != nil {
		t.Fatalf("RegisterRegion: %v", err)
	}
	want := []string{
		"exchange-point-and-mark", "kill-region", "kill-ring-save",
		"redo", "set-mark-command", "undo", "yank", "yank-pop",
	}
	if got := r.Names(); !slices.Equal(got, want) {
		t.Errorf("Names() = %q, want %q", got, want)
	}
	for _, name := range want {
		c, ok := r.Lookup(name)
		if !ok {
			t.Errorf("%s not registered", name)
			continue
		}
		if c.Doc == "" {
			t.Errorf("%s has no Doc; M-x and describe-key show it", name)
		}
	}
}

func TestRegisterRegionTwiceIsAnError(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterRegion(r); err != nil {
		t.Fatalf("first RegisterRegion: %v", err)
	}
	if err := command.RegisterRegion(r); !errors.Is(err, command.ErrDuplicateCommand) {
		t.Errorf("second RegisterRegion error = %v, want ErrDuplicateCommand", err)
	}
}

// --- mark ---

func TestSetMarkCommand(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	f.SetPoint(text.Pos{Line: 0, Col: 6})

	runRegion(t, r, f, "set-mark-command")

	if got, want := f.Buf().Mark(), (text.Pos{Line: 0, Col: 6}); got != want {
		t.Errorf("mark = %+v, want %+v", got, want)
	}
	if !regionHasEcho(f, "Mark set") {
		t.Errorf("Echoes = %q, want one containing %q", f.Echoes, "Mark set")
	}
	if got, want := f.Text(), "hello world"; got != want {
		t.Errorf("setting the mark changed the text to %q", got)
	}
}

func TestExchangePointAndMark(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	f.Buf().SetMark(text.Pos{Line: 0, Col: 2})
	f.SetPoint(text.Pos{Line: 0, Col: 8})

	runRegion(t, r, f, "exchange-point-and-mark")

	if got, want := f.Point(), (text.Pos{Line: 0, Col: 2}); got != want {
		t.Errorf("point = %+v, want %+v", got, want)
	}
	if got, want := f.Buf().Mark(), (text.Pos{Line: 0, Col: 8}); got != want {
		t.Errorf("mark = %+v, want %+v", got, want)
	}
}

// --- kill-region and kill-ring-save ---

// The region is [point, mark] normalised, so the user may set the mark on
// either side of point and get the same result.
func TestKillRegionNormalisesEndpointOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		point, mark text.Pos
	}{
		{"mark after point", text.Pos{Line: 0, Col: 0}, text.Pos{Line: 0, Col: 5}},
		{"mark before point", text.Pos{Line: 0, Col: 5}, text.Pos{Line: 0, Col: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, f := regionSetup(t, "hello world")
			f.SetPoint(tc.point)
			f.Buf().SetMark(tc.mark)

			runRegion(t, r, f, "kill-region")

			if got, want := f.Text(), " world"; got != want {
				t.Errorf("text = %q, want %q", got, want)
			}
			if got, want := f.Point(), (text.Pos{Line: 0, Col: 0}); got != want {
				t.Errorf("point = %+v, want %+v (the start of the killed region)", got, want)
			}
			got, err := f.Ring().Yank()
			if err != nil {
				t.Fatalf("Yank: %v", err)
			}
			if want := "hello"; got != want {
				t.Errorf("killed text = %q, want %q", got, want)
			}
		})
	}
}

// kill-ring-save must not touch the buffer. Getting this wrong destroys the
// user's text, so the buffer is asserted byte-identical.
func TestKillRingSaveLeavesBufferByteIdentical(t *testing.T) {
	for _, tc := range []struct {
		name        string
		point, mark text.Pos
	}{
		{"mark after point", text.Pos{Line: 0, Col: 0}, text.Pos{Line: 0, Col: 5}},
		{"mark before point", text.Pos{Line: 0, Col: 5}, text.Pos{Line: 0, Col: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, f := regionSetup(t, "hello world")
			f.SetPoint(tc.point)
			f.Buf().SetMark(tc.mark)
			before := f.Text()

			runRegion(t, r, f, "kill-ring-save")

			if got := f.Text(); got != before {
				t.Errorf("text = %q, want it unchanged at %q", got, before)
			}
			if got, want := f.Point(), tc.point; got != want {
				t.Errorf("point = %+v, want it unmoved at %+v", got, want)
			}
			got, err := f.Ring().Yank()
			if err != nil {
				t.Fatalf("Yank: %v", err)
			}
			if want := "hello"; got != want {
				t.Errorf("saved text = %q, want %q", got, want)
			}
		})
	}
}

func TestKillRegionSpanningLines(t *testing.T) {
	r, f := regionSetup(t, "one", "two", "three")
	f.SetPoint(text.Pos{Line: 0, Col: 1})
	f.Buf().SetMark(text.Pos{Line: 2, Col: 2})

	runRegion(t, r, f, "kill-region")

	if got, want := f.Text(), "oree"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	got, err := f.Ring().Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if want := "ne\ntwo\nth"; got != want {
		t.Errorf("killed text = %q, want %q", got, want)
	}
}

func TestKillRegionWithEmptyRegionIsANoOp(t *testing.T) {
	r, f := regionSetup(t, "hello")
	f.SetPoint(text.Pos{Line: 0, Col: 3})
	f.Buf().SetMark(text.Pos{Line: 0, Col: 3})

	runRegion(t, r, f, "kill-region")

	if got, want := f.Text(), "hello"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := f.Ring().Len(); got != 0 {
		t.Errorf("ring holds %d entries, want 0: an empty kill must not create one", got)
	}
}

// Consecutive kills accumulate into one entry because the ring owns run
// awareness. Nothing here calls BreakRun, which is dispatch's job.
func TestConsecutiveKillRegionsAccumulateIntoOneEntry(t *testing.T) {
	r, f := regionSetup(t, "abcdef")

	f.SetPoint(text.Pos{Line: 0, Col: 0})
	f.Buf().SetMark(text.Pos{Line: 0, Col: 2})
	runRegion(t, r, f, "kill-region")

	f.SetPoint(text.Pos{Line: 0, Col: 0})
	f.Buf().SetMark(text.Pos{Line: 0, Col: 2})
	runRegion(t, r, f, "kill-region")

	if got, want := f.Text(), "ef"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got := f.Ring().Len(); got != 1 {
		t.Errorf("ring holds %d entries, want 1: consecutive kills accumulate", got)
	}
	got, err := f.Ring().Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if want := "abcd"; got != want {
		t.Errorf("accumulated kill = %q, want %q", got, want)
	}
}

// --- yank ---

func TestYankInsertsAtPoint(t *testing.T) {
	r, f := regionSetup(t, "ab")
	regionEntries(f, "XY")
	f.SetPoint(text.Pos{Line: 0, Col: 1})

	runRegion(t, r, f, "yank")

	if got, want := f.Text(), "aXYb"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point(), (text.Pos{Line: 0, Col: 3}); got != want {
		t.Errorf("point = %+v, want %+v (end of the yanked text)", got, want)
	}
}

// Emacs leaves the mark at the start of yanked text so C-x C-x selects it.
func TestYankLeavesMarkAtStartOfInsertion(t *testing.T) {
	r, f := regionSetup(t, "ab")
	regionEntries(f, "XY")
	f.SetPoint(text.Pos{Line: 0, Col: 1})

	runRegion(t, r, f, "yank")

	if got, want := f.Buf().Mark(), (text.Pos{Line: 0, Col: 1}); got != want {
		t.Errorf("mark = %+v, want %+v", got, want)
	}
}

func TestYankMultilineTracksPointAcrossLines(t *testing.T) {
	r, f := regionSetup(t, "ab")
	regionEntries(f, "X\nY")
	f.SetPoint(text.Pos{Line: 0, Col: 1})

	runRegion(t, r, f, "yank")

	if got, want := f.Text(), "aX\nYb"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point(), (text.Pos{Line: 1, Col: 1}); got != want {
		t.Errorf("point = %+v, want %+v", got, want)
	}
}

func TestYankOnEmptyRingReportsSo(t *testing.T) {
	r, f := regionSetup(t, "ab")
	if err := r.Run("yank", f); !errors.Is(err, command.ErrKillRingEmpty) {
		t.Errorf("yank on empty ring = %v, want ErrKillRingEmpty", err)
	}
	if got, want := f.Text(), "ab"; got != want {
		t.Errorf("text = %q, want it unchanged at %q", got, want)
	}
}

// --- yank-pop: the fiddliest behaviour in the editor ---

// Each M-y must REPLACE what the previous yank inserted, never append to it.
// If the extent bookkeeping in Seq is wrong the buffer accumulates, which this
// catches immediately.
func TestYankPopReplacesRatherThanAccumulates(t *testing.T) {
	r, f := regionSetup(t)
	regionEntries(f, "one", "two", "three")

	runRegion(t, r, f, "yank")
	if got, want := f.Text(), "three"; got != want {
		t.Fatalf("after yank text = %q, want %q", got, want)
	}

	for _, want := range []string{"two", "one", "three"} {
		runRegion(t, r, f, "yank-pop")
		if got := f.Text(); got != want {
			t.Fatalf("after yank-pop text = %q, want exactly %q", got, want)
		}
		if got, wantPt := f.Point(), (text.Pos{Line: 0, Col: text.RuneIdx(len(want))}); got != wantPt {
			t.Errorf("point = %+v, want %+v", got, wantPt)
		}
	}
}

// The cycle closes on exactly the Nth pop, and every entry is visited once.
func TestYankPopCycleLengthEqualsRingSize(t *testing.T) {
	r, f := regionSetup(t)
	want := []string{"a", "b", "c", "d"}
	regionEntries(f, want...)

	runRegion(t, r, f, "yank")
	seen := map[string]bool{f.Text(): true}

	for i := 1; i < len(want); i++ {
		runRegion(t, r, f, "yank-pop")
		got := f.Text()
		if seen[got] {
			t.Fatalf("pop %d revisited %q after %d distinct entries; cycle closed early", i, got, len(seen))
		}
		seen[got] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("visited %d entries, want all %d", len(seen), len(want))
	}
	runRegion(t, r, f, "yank-pop")
	if got, first := f.Text(), "d"; got != first {
		t.Errorf("after a full cycle text = %q, want it back at %q", got, first)
	}
}

func TestYankPopMultilineReplacesWholeExtent(t *testing.T) {
	r, f := regionSetup(t, "[]")
	regionEntries(f, "p\nq\nr", "z")
	f.SetPoint(text.Pos{Line: 0, Col: 1})

	runRegion(t, r, f, "yank")
	if got, want := f.Text(), "[z]"; got != want {
		t.Fatalf("after yank text = %q, want %q", got, want)
	}

	runRegion(t, r, f, "yank-pop")
	if got, want := f.Text(), "[p\nq\nr]"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := f.Point(), (text.Pos{Line: 2, Col: 1}); got != want {
		t.Errorf("point = %+v, want %+v", got, want)
	}

	// Popping back to the single-line entry must collapse the three lines again.
	runRegion(t, r, f, "yank-pop")
	if got, want := f.Text(), "[z]"; got != want {
		t.Errorf("text = %q, want %q: the multiline extent was not fully removed", got, want)
	}
}

func TestYankPopWithoutPrecedingYankIsReported(t *testing.T) {
	r, f := regionSetup(t, "ab")
	regionEntries(f, "XY")

	if err := r.Run("yank-pop", f); err != nil {
		t.Fatalf("yank-pop should report through Echo, not return an error: %v", err)
	}
	if !regionHasEcho(f, "not a yank") {
		t.Errorf("Echoes = %q, want one mentioning that the previous command was not a yank", f.Echoes)
	}
	if got, want := f.Text(), "ab"; got != want {
		t.Errorf("text = %q, want it unchanged at %q", got, want)
	}
}

func TestYankPopAfterInterveningCommandIsReported(t *testing.T) {
	r, f := regionSetup(t)
	regionEntries(f, "one", "two")
	runRegion(t, r, f, "yank")

	// Something else ran in between.
	f.SetLastCommand("self-insert-command")

	if err := r.Run("yank-pop", f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !regionHasEcho(f, "not a yank") {
		t.Errorf("Echoes = %q, want one mentioning that the previous command was not a yank", f.Echoes)
	}
	if got, want := f.Text(), "two"; got != want {
		t.Errorf("text = %q, want it unchanged at %q", got, want)
	}
}

// The ring's own yank validity is a second gate: dispatch may have broken the
// run even when the command history still looks right.
func TestYankPopAfterBrokenRunIsReported(t *testing.T) {
	r, f := regionSetup(t)
	regionEntries(f, "one", "two")
	runRegion(t, r, f, "yank")

	f.Ring().BreakRun()

	if err := r.Run("yank-pop", f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !regionHasEcho(f, "not a yank") {
		t.Errorf("Echoes = %q, want one mentioning that the previous command was not a yank", f.Echoes)
	}
	if got, want := f.Text(), "two"; got != want {
		t.Errorf("text = %q, want it unchanged at %q", got, want)
	}
}

// --- undo and redo ---

func TestUndoRevertsTheLastChange(t *testing.T) {
	r, f := regionSetup(t, "hello")
	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 5}, []rune("!")); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	runRegion(t, r, f, "undo")

	if got, want := f.Text(), "hello"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestUndoHonoursTheUniversalArgument(t *testing.T) {
	r, f := regionSetup(t, "x")
	for _, s := range []string{"a", "b", "c"} {
		if err := f.Buf().Insert(f.Buf().End(), []rune(s)); err != nil {
			t.Fatalf("Insert %q: %v", s, err)
		}
		f.Buf().BreakUndo()
	}
	if got, want := f.Text(), "xabc"; got != want {
		t.Fatalf("setup text = %q, want %q", got, want)
	}

	f.ArgN, f.ArgExplicit = 3, true
	runRegion(t, r, f, "undo")

	if got, want := f.Text(), "x"; got != want {
		t.Errorf("text = %q, want %q after undoing three units", got, want)
	}
}

func TestUndoWithNothingLeftIsReported(t *testing.T) {
	r, f := regionSetup(t)

	runRegion(t, r, f, "undo")

	if !regionHasEcho(f, "No further undo") {
		t.Errorf("Echoes = %q, want one mentioning there is no further undo information", f.Echoes)
	}
}

func TestRedoRestoresAnUndoneChange(t *testing.T) {
	r, f := regionSetup(t, "hello")
	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 5}, []rune("!")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	runRegion(t, r, f, "undo")
	if got, want := f.Text(), "hello"; got != want {
		t.Fatalf("after undo text = %q, want %q", got, want)
	}

	runRegion(t, r, f, "redo")

	if got, want := f.Text(), "hello!"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestRedoWithNothingLeftIsReported(t *testing.T) {
	r, f := regionSetup(t, "hello")

	runRegion(t, r, f, "redo")

	if !regionHasEcho(f, "No further redo") {
		t.Errorf("Echoes = %q, want one mentioning there is no further redo information", f.Echoes)
	}
}

// Linear undo, by explicit design decision: a fresh edit after an undo discards
// the redo branch. This test exists to stop anyone quietly turning undo into a
// tree.
func TestFreshEditAfterUndoDiscardsTheRedoBranch(t *testing.T) {
	r, f := regionSetup(t, "hello")
	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 5}, []rune("!")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	runRegion(t, r, f, "undo")

	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 5}, []rune("?")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	f.Buf().BreakUndo()

	runRegion(t, r, f, "redo")

	if got, want := f.Text(), "hello?"; got != want {
		t.Errorf("text = %q, want %q: the redo branch must be gone", got, want)
	}
	if !regionHasEcho(f, "No further redo") {
		t.Errorf("Echoes = %q, want one reporting no further redo information", f.Echoes)
	}
}

func TestUndoMovesPointToTheChange(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 5}, []rune("XYZ")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	f.SetPoint(f.Buf().End())

	runRegion(t, r, f, "undo")

	if got := f.Point(); got.Line != 0 || got.Col != 5 {
		t.Errorf("point = %+v, want it at the undone change (line 0, col 5)", got)
	}
}

// --- refusing when there is no mark ---

// "Destructive by default" was the real bug here: without a mark, C-w used to
// kill everything from the buffer start to point. These tests assert the buffer
// and the kill ring are untouched, not merely that a message appeared — a test
// that only checked the message would still pass if the kill went through.
func TestRegionCommandsRefuseWithoutAMark(t *testing.T) {
	for _, name := range []string{"kill-region", "kill-ring-save", "exchange-point-and-mark"} {
		t.Run(name, func(t *testing.T) {
			r, f := regionSetup(t, "hello world")
			f.SetPoint(text.Pos{Line: 0, Col: 5})
			if f.Buf().HasMark() {
				t.Fatal("a fresh buffer must have no mark")
			}

			if err := r.Run(name, f); err != nil {
				t.Fatalf("%s should refuse through Echo, not return an error: %v", name, err)
			}

			if got, want := f.Text(), "hello world"; got != want {
				t.Errorf("text = %q, want it byte-identical at %q", got, want)
			}
			if got, want := f.Point(), (text.Pos{Line: 0, Col: 5}); got != want {
				t.Errorf("point = %+v, want it unmoved at %+v", got, want)
			}
			if got := f.Ring().Len(); got != 0 {
				t.Errorf("kill ring holds %d entries, want 0: a refused command must not kill", got)
			}
			if !regionHasEcho(f, "No mark set in this buffer") {
				t.Errorf("Echoes = %q, want emacs's \"No mark set in this buffer\"", f.Echoes)
			}
		})
	}
}

func TestSetMarkCommandMakesHasMarkTrue(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	f.SetPoint(text.Pos{Line: 0, Col: 6})
	if f.Buf().HasMark() {
		t.Fatal("a fresh buffer must have no mark")
	}

	runRegion(t, r, f, "set-mark-command")

	if !f.Buf().HasMark() {
		t.Error("HasMark() = false after set-mark-command, want true")
	}
}

// A mark deliberately set at the origin is a real mark. This is the case a bare
// Mark() == Pos{} check gets wrong, so the region commands must honour it.
func TestMarkSetAtOriginCountsAsSet(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	f.SetPoint(text.Pos{Line: 0, Col: 0})
	runRegion(t, r, f, "set-mark-command")
	f.SetPoint(text.Pos{Line: 0, Col: 5})

	runRegion(t, r, f, "kill-region")

	if got, want := f.Text(), " world"; got != want {
		t.Errorf("text = %q, want %q: a mark at the origin must be honoured", got, want)
	}
	if regionHasEcho(f, "No mark set") {
		t.Errorf("Echoes = %q, want no refusal", f.Echoes)
	}
}

// After ClearMark the region commands must refuse again, so the flag is genuinely
// consulted rather than being set once and assumed forever.
func TestClearMarkMakesRegionCommandsRefuseAgain(t *testing.T) {
	r, f := regionSetup(t, "hello world")
	f.Buf().SetMark(text.Pos{Line: 0, Col: 0})
	f.Buf().ClearMark()
	f.SetPoint(text.Pos{Line: 0, Col: 5})

	if err := r.Run("kill-region", f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := f.Text(), "hello world"; got != want {
		t.Errorf("text = %q, want it unchanged at %q", got, want)
	}
	if !regionHasEcho(f, "No mark set in this buffer") {
		t.Errorf("Echoes = %q, want a refusal", f.Echoes)
	}
}
