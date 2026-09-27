package fuzzy

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// The behaviour the package exists for: an abbreviation made of characters
// scattered through a name still finds it.
func TestScoreMatchesASubsequence(t *testing.T) {
	m, ok := Score("fwc", "forward-char")
	if !ok {
		t.Fatal(`Score("fwc", "forward-char") reported no match, want a match`)
	}
	if m.Score <= 0 {
		t.Errorf("Score = %d, want a positive score", m.Score)
	}
}

// Runes not present in order must not match, or every candidate matches
// everything and ranking is meaningless.
func TestScoreRejectsANonSubsequence(t *testing.T) {
	if _, ok := Score("xyz", "forward-char"); ok {
		t.Error(`Score("xyz", "forward-char") reported a match, want none`)
	}
}

// Order matters: the same runes in the wrong order are not a subsequence.
func TestScoreRejectsOutOfOrderRunes(t *testing.T) {
	if _, ok := Score("cwf", "forward-char"); ok {
		t.Error(`Score("cwf", "forward-char") reported a match, want none`)
	}
}

// Ranking a list long enough to be shared across the CPUs gives exactly what
// one core gives, order included.
func TestRankOnManyCoresMatchesOne(t *testing.T) {
	var cands []string
	for i := range 3 * parallelMin {
		cands = append(cands, fmt.Sprintf("dir%d/sub%d/file_%d.go", i%37, i%11, i))
	}
	many := Rank("d3fi", cands)
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	one := Rank("d3fi", cands)
	if len(one) == 0 || !reflect.DeepEqual(many, one) {
		t.Fatalf("ranked %d candidates on many cores and %d on one, or in another order", len(many), len(one))
	}
}

// A query and a candidate too long to align exactly - a kill yanked into a
// prompt, against a buffer named after another - are matched in one pass
// instead: at once, and in memory that does not grow with the product of
// their lengths, which was billions of cells for two 58,000-rune strings.
func TestHugeQueriesMatchInOnePass(t *testing.T) {
	long := strings.Repeat("abc-def ", 8000)
	done := make(chan Match)
	go func() {
		m, ok := Score(long, long+"x")
		if !ok {
			t.Error("a string does not match itself with more after it")
		}
		done <- m
	}()
	select {
	case m := <-done:
		if len(m.Indices) != len([]rune(long)) || !slices.IsSorted(m.Indices) {
			t.Errorf("%d indices, sorted %v; want one per query rune, ascending", len(m.Indices), slices.IsSorted(m.Indices))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scoring a long query is still running")
	}
	if _, ok := Score(long+"z", long); ok {
		t.Error("a query longer than the candidate matched")
	}
}
