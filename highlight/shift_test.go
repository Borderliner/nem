package highlight

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/Borderliner/nem/syntax"
)

// rebuildShift is shift as it was first written: a fresh slice, filled by
// moving each state to where its line went. Kept as the reference the in-place
// version must agree with.
func rebuildShift(states []syntax.State, from, delta, n int) []syntax.State {
	need := n + 1
	moved := make([]syntax.State, need)
	head := min(from+1, len(states), need)
	copy(moved, states[:head])
	for j := from + 1; j < len(states); j++ {
		if k := j + delta; k > from && k < need {
			moved[k] = states[j]
		}
	}
	moved[0] = 0
	return moved
}

func TestShiftMatchesTheRebuild(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := range 50000 {
		size := rng.IntN(20)
		states := make([]syntax.State, size)
		for i := range states {
			states[i] = syntax.State(rng.IntN(1000) + 1)
		}
		from := rng.IntN(size + 3)
		delta := rng.IntN(9) - 4
		n := max(size-1+delta+rng.IntN(3)-1, 0)

		want := rebuildShift(slices.Clone(states), from, delta, n)
		c := &Cache{states: slices.Clone(states)}
		c.shift(from, delta, n)
		if !slices.Equal(c.states, want) {
			t.Fatalf("trial %d: shift(%d, %d, %d) of %v\n got %v\nwant %v", trial, from, delta, n, states, c.states, want)
		}
	}
}
