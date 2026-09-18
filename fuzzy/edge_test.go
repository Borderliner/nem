package fuzzy

import "testing"

func TestScoreEmptyQueryMatchesAnything(t *testing.T) {
	m, ok := Score("", "forward-char")
	if !ok {
		t.Fatal(`Score("", ...) reported no match; an empty query must match`)
	}
	if m.Score != 0 {
		t.Errorf("Score = %d, want 0", m.Score)
	}
	if m.Indices != nil {
		t.Errorf("Indices = %v, want nil", m.Indices)
	}
}

func TestScoreEmptyQueryMatchesEmptyCandidate(t *testing.T) {
	if _, ok := Score("", ""); !ok {
		t.Error(`Score("", "") reported no match`)
	}
}

func TestScoreEmptyCandidateRejectsNonEmptyQuery(t *testing.T) {
	if _, ok := Score("a", ""); ok {
		t.Error(`Score("a", "") reported a match, want none`)
	}
}

func TestScoreRejectsQueryLongerThanCandidate(t *testing.T) {
	if _, ok := Score("forward-char-and-more", "forward"); ok {
		t.Error("a query longer than the candidate matched, want no match")
	}
}

// An exact match must match, and be the strongest possible for that query.
func TestScoreExactMatch(t *testing.T) {
	m, ok := Score("undo", "undo")
	if !ok {
		t.Fatal("exact match reported no match")
	}
	if len(m.Indices) != 4 || m.Indices[0] != 0 || m.Indices[3] != 3 {
		t.Errorf("Indices = %v, want [0 1 2 3]", m.Indices)
	}
	other := mustScore(t, "undo", "u-n-d-o")
	if m.Score <= other {
		t.Errorf("exact %d must outscore scattered %d", m.Score, other)
	}
}

// A single rune query is the common case one keystroke in, and must not panic
// or lose its index.
func TestScoreSingleRuneQuery(t *testing.T) {
	m, ok := Score("y", "yank")
	if !ok {
		t.Fatal("no match")
	}
	if len(m.Indices) != 1 || m.Indices[0] != 0 {
		t.Errorf("Indices = %v, want [0]", m.Indices)
	}
}

func TestRankNilAndEmptyCandidates(t *testing.T) {
	if got := Rank("x", nil); len(got) != 0 {
		t.Errorf("Rank on nil candidates = %v, want empty", got)
	}
	if got := Rank("x", []string{}); len(got) != 0 {
		t.Errorf("Rank on empty candidates = %v, want empty", got)
	}
	if got := Rank("", nil); len(got) != 0 {
		t.Errorf("Rank empty query on nil candidates = %v, want empty", got)
	}
}

// Duplicate candidates must not break the stable tie-break, which keys on first
// occurrence.
func TestRankHandlesDuplicateCandidates(t *testing.T) {
	got := Rank("u", []string{"undo", "undo", "upcase-word"})
	if len(got) != 3 {
		t.Errorf("got %d results, want 3 (duplicates are not deduplicated)", len(got))
	}
}
