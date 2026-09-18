package command

import (
	"errors"
	"strings"
	"testing"
)

// --- construction -----------------------------------------------------------

func TestNewKillRingCoercesBadCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1, -60} {
		k := NewKillRing(capacity)
		if got := k.Capacity(); got != DefaultCapacity {
			t.Errorf("NewKillRing(%d).Capacity() = %d, want %d", capacity, got, DefaultCapacity)
		}
	}
}

func TestNewKillRingKeepsGoodCapacity(t *testing.T) {
	k := NewKillRing(7)
	if got := k.Capacity(); got != 7 {
		t.Errorf("Capacity() = %d, want 7", got)
	}
}

func TestEmptyRing(t *testing.T) {
	k := NewKillRing(60)
	if got := k.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if _, err := k.Yank(); !errors.Is(err, ErrKillRingEmpty) {
		t.Errorf("Yank() on empty ring err = %v, want ErrKillRingEmpty", err)
	}
}

// --- requirement 1: consecutive kills accumulate into ONE entry -------------

// The most-cited kill-ring behaviour: C-k C-k C-k then C-y restores all three
// lines as a single block.
func TestConsecutiveKillsAccumulateIntoOneEntry(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("line one\n")
	k.Kill("line two\n")
	k.Kill("line three\n")

	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1 — consecutive kills must accumulate, not push", got)
	}
	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	want := "line one\nline two\nline three\n"
	if got != want {
		t.Errorf("Yank() = %q, want %q", got, want)
	}
}

func TestAppendAccumulatesLikeKill(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("foo")
	k.Append(" bar")
	k.Append(" baz")

	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	if got, _ := k.Yank(); got != "foo bar baz" {
		t.Errorf("Yank() = %q, want %q", got, "foo bar baz")
	}
}

func TestAppendWithNoRunStartsNewEntry(t *testing.T) {
	k := NewKillRing(60)
	k.Append("solo")
	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	if got, _ := k.Yank(); got != "solo" {
		t.Errorf("Yank() = %q, want %q", got, "solo")
	}
}

// --- requirement 2: direction matters --------------------------------------

// Killing backward word-by-word over "foo bar" must yank back in reading
// order. M-DEL kills "bar", then M-DEL kills "foo " — prepending yields
// "foo bar", not "bar foo".
func TestBackwardKillsPrependIntoReadingOrder(t *testing.T) {
	k := NewKillRing(60)
	k.Prepend("bar")
	k.Prepend("foo ")

	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	if got != "foo bar" {
		t.Errorf("Yank() = %q, want %q (backward kills must not reverse the text)", got, "foo bar")
	}
}

func TestPrependWithNoRunStartsNewEntry(t *testing.T) {
	k := NewKillRing(60)
	k.Prepend("solo")
	if got, _ := k.Yank(); got != "solo" {
		t.Errorf("Yank() = %q, want %q", got, "solo")
	}
}

// A run that mixes directions grows at both ends, as emacs does when you
// alternate C-k and M-DEL without an intervening command.
func TestMixedDirectionRunGrowsBothEnds(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("middle")
	k.Append("-after")
	k.Prepend("before-")

	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	if got, _ := k.Yank(); got != "before-middle-after" {
		t.Errorf("Yank() = %q, want %q", got, "before-middle-after")
	}
}

// --- requirement 3: BreakRun ends accumulation -----------------------------

func TestBreakRunEndsAccumulation(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("first")
	k.BreakRun()
	k.Kill("second")

	if got := k.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 — BreakRun must end the run", got)
	}
	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	if got != "second" {
		t.Errorf("Yank() = %q, want %q (yank takes the newest entry only)", got, "second")
	}
}

func TestBreakRunIsIdempotent(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("a")
	k.BreakRun()
	k.BreakRun()
	k.Kill("b")
	if got := k.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
}

func TestBreakRunOnEmptyRingDoesNotPanic(t *testing.T) {
	k := NewKillRing(60)
	k.BreakRun()
	if got := k.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
}

// A yank is not a kill, so a kill after a yank must start a fresh entry
// rather than accumulating onto the entry that was just yanked.
func TestYankEndsTheKillRun(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("killed")
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	k.Kill("after yank")

	if got := k.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 — a kill after a yank must not accumulate", got)
	}
	if got, _ := k.Yank(); got != "after yank" {
		t.Errorf("Yank() = %q, want %q", got, "after yank")
	}
}

// --- requirement 4: YankPop validity --------------------------------------

func TestYankPopRequiresPrecedingYank(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("one")
	k.BreakRun()
	k.Kill("two")

	if _, err := k.YankPop(); !errors.Is(err, ErrNotAfterYank) {
		t.Errorf("YankPop() without a preceding Yank err = %v, want ErrNotAfterYank", err)
	}
}

func TestYankPopOnFreshRingErrors(t *testing.T) {
	k := NewKillRing(60)
	if _, err := k.YankPop(); !errors.Is(err, ErrNotAfterYank) {
		t.Errorf("YankPop() on fresh ring err = %v, want ErrNotAfterYank", err)
	}
}

func TestBreakRunInvalidatesYankPop(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("one")
	k.BreakRun()
	k.Kill("two")
	k.BreakRun()
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	k.BreakRun() // e.g. the user moved the cursor

	if _, err := k.YankPop(); !errors.Is(err, ErrNotAfterYank) {
		t.Errorf("YankPop() after BreakRun err = %v, want ErrNotAfterYank", err)
	}
}

func TestKillBetweenYankAndYankPopInvalidates(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("one")
	k.BreakRun()
	k.Kill("two")
	k.BreakRun()
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	k.Kill("interloper")

	if _, err := k.YankPop(); !errors.Is(err, ErrNotAfterYank) {
		t.Errorf("YankPop() after an intervening Kill err = %v, want ErrNotAfterYank", err)
	}
}

func TestPrependBetweenYankAndYankPopInvalidates(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("one")
	k.BreakRun()
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	k.Prepend("interloper")

	if _, err := k.YankPop(); !errors.Is(err, ErrNotAfterYank) {
		t.Errorf("YankPop() after an intervening Prepend err = %v, want ErrNotAfterYank", err)
	}
}

func TestYankPopStaysValidAcrossRepeatedPops(t *testing.T) {
	k := NewKillRing(60)
	for _, s := range []string{"one", "two", "three"} {
		k.Kill(s)
		k.BreakRun()
	}
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := k.YankPop(); err != nil {
			t.Fatalf("YankPop() #%d err = %v, want nil", i+1, err)
		}
	}
}

// --- requirement 5: YankPop rotates and wraps ------------------------------

func TestYankPopRotatesThroughRingAndWraps(t *testing.T) {
	k := NewKillRing(60)
	for _, s := range []string{"one", "two", "three"} {
		k.Kill(s)
		k.BreakRun()
	}

	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	// Newest first, then successively older, then wrapping to newest.
	want := []string{"three", "two", "one", "three", "two"}
	if got != want[0] {
		t.Fatalf("Yank() = %q, want %q", got, want[0])
	}
	for i, w := range want[1:] {
		got, err := k.YankPop()
		if err != nil {
			t.Fatalf("YankPop() #%d err = %v", i+1, err)
		}
		if got != w {
			t.Errorf("YankPop() #%d = %q, want %q", i+1, got, w)
		}
	}
}

// YankPop returns the full replacement text, not a delta — the caller swaps
// the previously yanked text for exactly what comes back.
func TestYankPopReturnsFullReplacement(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("short")
	k.BreakRun()
	k.Kill("a much longer entry")
	k.BreakRun()

	if got, _ := k.Yank(); got != "a much longer entry" {
		t.Fatalf("Yank() = %q", got)
	}
	got, err := k.YankPop()
	if err != nil {
		t.Fatalf("YankPop() err = %v", err)
	}
	if got != "short" {
		t.Errorf("YankPop() = %q, want the whole replacement %q", got, "short")
	}
}

func TestYankPopWithSingleEntryReturnsItself(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("only")
	k.BreakRun()
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	got, err := k.YankPop()
	if err != nil {
		t.Fatalf("YankPop() err = %v", err)
	}
	if got != "only" {
		t.Errorf("YankPop() = %q, want %q", got, "only")
	}
}

// After rotating with M-y, a subsequent C-y must yank the entry the pointer
// now rests on, as emacs does.
func TestYankAfterYankPopUsesRotatedPointer(t *testing.T) {
	k := NewKillRing(60)
	for _, s := range []string{"one", "two", "three"} {
		k.Kill(s)
		k.BreakRun()
	}
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	if got, _ := k.YankPop(); got != "two" {
		t.Fatalf("YankPop() = %q, want %q", got, "two")
	}
	k.BreakRun()
	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	if got != "two" {
		t.Errorf("Yank() after YankPop = %q, want %q", got, "two")
	}
}

// --- requirement 6: the ring evicts ---------------------------------------

func TestRingEvictsOldestAtCapacity(t *testing.T) {
	k := NewKillRing(3)
	for _, s := range []string{"one", "two", "three", "four"} {
		k.Kill(s)
		k.BreakRun()
	}

	if got := k.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	// "one" must have been evicted; rotation sees four, three, two only.
	want := []string{"three", "two", "four"}
	for i, w := range want {
		got, err := k.YankPop()
		if err != nil {
			t.Fatalf("YankPop() #%d err = %v", i+1, err)
		}
		if got != w {
			t.Errorf("YankPop() #%d = %q, want %q", i+1, got, w)
		}
	}
}

func TestAccumulationDoesNotEvict(t *testing.T) {
	k := NewKillRing(2)
	k.Kill("a")
	k.Kill("b")
	k.Kill("c")
	if got := k.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1", got)
	}
}

// --- requirement 7: empty kills -------------------------------------------

func TestEmptyKillIsNoOp(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("")
	k.Append("")
	k.Prepend("")

	if got := k.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 — an empty kill must not create an entry", got)
	}
	if _, err := k.Yank(); !errors.Is(err, ErrKillRingEmpty) {
		t.Errorf("Yank() err = %v, want ErrKillRingEmpty", err)
	}
}

// An empty kill is a no-op, so it must not disturb an in-progress yank run
// either — nothing happened, so nothing is invalidated.
func TestEmptyKillDoesNotInvalidateYankPop(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("one")
	k.BreakRun()
	k.Kill("two")
	k.BreakRun()
	if _, err := k.Yank(); err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	k.Kill("")

	got, err := k.YankPop()
	if err != nil {
		t.Errorf("YankPop() after an empty kill err = %v, want nil", err)
	}
	if got != "one" {
		t.Errorf("YankPop() = %q, want %q", got, "one")
	}
}

func TestEmptyKillDoesNotBreakAccumulation(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("foo")
	k.Kill("")
	k.Kill("bar")
	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	if got, _ := k.Yank(); got != "foobar" {
		t.Errorf("Yank() = %q, want %q", got, "foobar")
	}
}

// --- misc -----------------------------------------------------------------

func TestLenTracksDistinctEntries(t *testing.T) {
	k := NewKillRing(60)
	if got := k.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0", got)
	}
	for i, s := range []string{"a", "b", "c"} {
		k.Kill(s)
		k.BreakRun()
		if got := k.Len(); got != i+1 {
			t.Errorf("after %d kills Len() = %d, want %d", i+1, got, i+1)
		}
	}
}

func TestYankDoesNotConsume(t *testing.T) {
	k := NewKillRing(60)
	k.Kill("persistent")
	k.BreakRun()
	for i := 0; i < 3; i++ {
		got, err := k.Yank()
		if err != nil {
			t.Fatalf("Yank() #%d err = %v", i+1, err)
		}
		if got != "persistent" {
			t.Errorf("Yank() #%d = %q, want %q", i+1, got, "persistent")
		}
	}
	if got := k.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1 — yanking must not consume", got)
	}
}

func TestLargeAccumulationRun(t *testing.T) {
	k := NewKillRing(60)
	var want strings.Builder
	for i := 0; i < 1000; i++ {
		k.Kill("x")
		want.WriteString("x")
	}
	if got := k.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	got, err := k.Yank()
	if err != nil {
		t.Fatalf("Yank() err = %v", err)
	}
	if got != want.String() {
		t.Errorf("Yank() length = %d, want %d", len(got), want.Len())
	}
}

// The state machine as a table: drive a sequence of operations, assert the
// resulting entry count and what a yank produces.
func TestStateMachineSequences(t *testing.T) {
	type op struct {
		kind string // kill, append, prepend, break, yank
		arg  string
	}
	tests := []struct {
		name     string
		ops      []op
		wantLen  int
		wantYank string
	}{
		{
			name:     "single kill",
			ops:      []op{{"kill", "a"}},
			wantLen:  1,
			wantYank: "a",
		},
		{
			name:     "kill run then break then kill",
			ops:      []op{{"kill", "a"}, {"kill", "b"}, {"break", ""}, {"kill", "c"}},
			wantLen:  2,
			wantYank: "c",
		},
		{
			name:     "backward run",
			ops:      []op{{"prepend", "c"}, {"prepend", "b"}, {"prepend", "a"}},
			wantLen:  1,
			wantYank: "abc",
		},
		{
			name:     "break between every kill",
			ops:      []op{{"kill", "a"}, {"break", ""}, {"kill", "b"}, {"break", ""}, {"kill", "c"}},
			wantLen:  3,
			wantYank: "c",
		},
		{
			name:     "yank then kill starts new entry",
			ops:      []op{{"kill", "a"}, {"yank", ""}, {"kill", "b"}},
			wantLen:  2,
			wantYank: "b",
		},
		{
			name:     "empty kills interleaved",
			ops:      []op{{"kill", ""}, {"kill", "a"}, {"kill", ""}, {"append", "b"}},
			wantLen:  1,
			wantYank: "ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := NewKillRing(60)
			for _, o := range tt.ops {
				switch o.kind {
				case "kill":
					k.Kill(o.arg)
				case "append":
					k.Append(o.arg)
				case "prepend":
					k.Prepend(o.arg)
				case "break":
					k.BreakRun()
				case "yank":
					if _, err := k.Yank(); err != nil {
						t.Fatalf("Yank() err = %v", err)
					}
				}
			}
			if got := k.Len(); got != tt.wantLen {
				t.Errorf("Len() = %d, want %d", got, tt.wantLen)
			}
			got, err := k.Yank()
			if err != nil {
				t.Fatalf("Yank() err = %v", err)
			}
			if got != tt.wantYank {
				t.Errorf("Yank() = %q, want %q", got, tt.wantYank)
			}
		})
	}
}
