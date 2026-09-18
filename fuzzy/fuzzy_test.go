package fuzzy

import "testing"

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
