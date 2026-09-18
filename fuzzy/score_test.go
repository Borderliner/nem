package fuzzy

import "testing"

func mustScore(t *testing.T, query, candidate string) int {
	t.Helper()
	m, ok := Score(query, candidate)
	if !ok {
		t.Fatalf("Score(%q, %q) reported no match", query, candidate)
	}
	return m.Score
}

// The rule that makes fuzzy matching feel deliberate rather than random: runes
// landing at the start of a word beat the same runes buried mid-word. Without
// it, "fwc" ranks a coincidental match above forward-char.
func TestWordBoundaryMatchesOutrankMidWordMatches(t *testing.T) {
	for _, tc := range []struct{ name, query, boundary, midWord string }{
		{"separator", "ab", "a-b", "xaxb"},
		{"underscore", "ab", "a_b", "xaxb"},
		{"slash", "ab", "a/b", "xaxb"},
		{"dot", "ab", "a.b", "xaxb"},
		{"space", "ab", "a b", "xaxb"},
		{"camel case", "ab", "aB", "xaxb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hi, lo := mustScore(t, tc.query, tc.boundary), mustScore(t, tc.query, tc.midWord)
			if hi <= lo {
				t.Errorf("%q scored %d and %q scored %d; the boundary match must score higher",
					tc.boundary, hi, tc.midWord, lo)
			}
		})
	}
}

// Consecutive runes read as a real prefix of a word, so they must beat the same
// runes split apart. Both candidates are scored against the SAME query, because
// scores from different queries are not comparable.
func TestConsecutiveRunesOutrankSplitRunes(t *testing.T) {
	together := mustScore(t, "ab", "ab-x") // adjacent
	apart := mustScore(t, "ab", "a-b-x")   // separated, though both at boundaries
	if together <= apart {
		t.Errorf("consecutive %d must outscore gapped %d", together, apart)
	}
}

// A wider gap between matched runes is a weaker match.
func TestWiderGapsScoreLower(t *testing.T) {
	near, far := mustScore(t, "ab", "axb"), mustScore(t, "ab", "axxxxb")
	if near <= far {
		t.Errorf("gap of 1 scored %d and gap of 4 scored %d; the nearer match must score higher", near, far)
	}
}

// The first matched rune carries the most weight, because a query is nearly
// always the beginning of what the user has in mind.
func TestMatchingTheFirstRuneAtTheStartWins(t *testing.T) {
	atStart, later := mustScore(t, "c", "char-x"), mustScore(t, "c", "x-char")
	if atStart <= later {
		t.Errorf("start match %d must outscore later boundary match %d", atStart, later)
	}
}

// The boundary bonus, isolated. The pair above it differ in match POSITION as
// well as in boundaries, so leading-position arithmetic alone kept them ordered
// and the test passed with the boundary bonus set to zero. These candidates put
// both matches at identical indices with an identical gap, so the boundary bonus
// is the only term that can separate them.
func TestBoundaryBonusIsWhatSeparatesIdenticalAlignments(t *testing.T) {
	for _, tc := range []struct{ name, query, boundary, plain string }{
		// b lands at index 2 in both, gap of 1 in both. Only "a-b" has b after
		// a separator.
		{"separator at same index", "ab", "a-b", "axb"},
		{"underscore at same index", "ab", "a_b", "axb"},
		{"slash at same index", "ab", "a/b", "axb"},
		{"dot at same index", "ab", "a.b", "axb"},
		{"space at same index", "ab", "a b", "axb"},
		// Both length 2 with b at index 1: "aB" is a camel boundary, "ab" is not.
		// "ab" even earns the exact-case bonus, so only a boundary bonus larger
		// than that can put "aB" ahead.
		{"camel at same index", "ab", "aB", "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hi, lo := mustScore(t, tc.query, tc.boundary), mustScore(t, tc.query, tc.plain)
			if hi <= lo {
				t.Errorf("%q scored %d and %q scored %d; only the boundary bonus distinguishes these, so the boundary match must win",
					tc.boundary, hi, tc.plain, lo)
			}
		})
	}
}
