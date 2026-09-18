package fuzzy

import (
	"reflect"
	"testing"
)

// The scorer finds the BEST alignment, not the first one a left-to-right scan
// stumbles into. A greedy pass locks onto a@0 and can never reconsider, so it
// would report the scattered match and score it far lower than the contiguous
// run a reader would point at.
func TestBestAlignmentPrefersTheContiguousRun(t *testing.T) {
	m, ok := Score("abc", "axbxabc")
	if !ok {
		t.Fatal("no match")
	}
	if want := []int{4, 5, 6}; !reflect.DeepEqual(m.Indices, want) {
		t.Errorf("Indices = %v, want %v (the contiguous tail)", m.Indices, want)
	}
}

// Same rule where the better alignment is a word boundary rather than a
// contiguous run.
func TestBestAlignmentPrefersTheBoundaryRun(t *testing.T) {
	m, ok := Score("ab", "xaxb-ab")
	if !ok {
		t.Fatal("no match")
	}
	if want := []int{5, 6}; !reflect.DeepEqual(m.Indices, want) {
		t.Errorf("Indices = %v, want %v (after the separator)", m.Indices, want)
	}
}

// A lower-case query ignores case, so the user need not reach for shift to find
// a capitalised name.
func TestLowerCaseQueryIgnoresCase(t *testing.T) {
	if _, ok := Score("ch", "CHAR"); !ok {
		t.Error(`Score("ch", "CHAR") reported no match; a lower-case query must ignore case`)
	}
}

// One upper-case rune makes the whole query case-sensitive, which is the same
// rule nem's incremental search uses - so there is one rule to learn, not two.
func TestUpperCaseQueryBecomesCaseSensitive(t *testing.T) {
	if _, ok := Score("Ch", "char"); ok {
		t.Error(`Score("Ch", "char") matched; an upper-case rune must make the query case-sensitive`)
	}
	if _, ok := Score("Ch", "Char"); !ok {
		t.Error(`Score("Ch", "Char") reported no match`)
	}
}

// While ignoring case, an exact-case match is still the better one.
func TestExactCaseIsPreferredWhileIgnoringCase(t *testing.T) {
	exact, folded := mustScore(t, "ch", "ch-x"), mustScore(t, "ch", "CH-x")
	if exact <= folded {
		t.Errorf("exact-case %d must outscore folded %d", exact, folded)
	}
}
