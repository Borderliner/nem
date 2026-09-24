// Package fuzzy ranks candidate strings against a short query typed by a user.
//
// Matching is subsequence-based: the query's runes must appear in the candidate
// in order but need not be adjacent, so "fwc" finds "forward-char". Scoring
// rewards matches that land at the start of a word and runs of adjacent runes,
// and charges for the gaps between them, so the ranking reflects how a reader
// would judge the match rather than merely whether one exists.
//
// Ordering is total and stable, and pinned by test against a fixed candidate
// set: predictability is a feature here, because a menu that reorders for
// reasons the user cannot see is worse than one that ranks imperfectly.
package fuzzy

import (
	"runtime"
	"slices"
	"sync"
)

// Match describes how a query matched one candidate.
type Match struct {
	// Score is higher for a better match. It is comparable only between
	// candidates scored against the same query.
	Score int
	// Indices are the RUNE offsets in the candidate that the query matched,
	// strictly ascending, one per query rune. Callers emphasise exactly these
	// positions, which is why they are rune offsets and not byte offsets.
	Indices []int
}

// Score reports how well query matches candidate, and whether it matches at all.
//
// An empty query matches every candidate with score 0 and no indices, so a
// prompt showing everything before the user types needs no special case.
func Score(query, candidate string) (Match, bool) {
	q := []rune(query)
	if len(q) == 0 {
		return Match{}, true
	}
	return scoreRunes(q, []rune(candidate), smartCaseFold(q))
}

// scoreRunes is the shared core, taking pre-decoded runes so Rank can decode the
// query once and reuse one candidate buffer across the whole pass. Decoding per
// candidate inside Score cost one allocation per candidate on every keystroke,
// which dominated ranking a large directory.
func scoreRunes(q, c []rune, ignoreCase bool) (Match, bool) {
	if len(q) > len(c) {
		return Match{}, false
	}
	if !isSubsequence(q, c, ignoreCase) {
		return Match{}, false
	}
	score, idx := bestAlignment(q, c, ignoreCase)
	return Match{Score: score, Indices: idx}, true
}

// Ranked is one candidate and how it matched.
type Ranked struct {
	Candidate string
	Match     Match
}

// parallelMin is how many candidates it takes for Rank to share the scoring
// out across the CPUs. Below it one core finishes before the others would have
// started; above it - every file in a large project - scoring on one core took
// a tenth of a second a keystroke.
const parallelMin = 4096

// Rank returns the matching candidates, best first.
//
// The order is total: by score descending, then by shorter candidate, then by
// the candidate's position in the input. That last tie-break comes free from
// a stable sort over a slice built in input order - an explicit position map
// would cost an allocation per candidate on every keystroke for nothing.
func Rank(query string, candidates []string) []Ranked {
	out := make([]Ranked, 0, len(candidates))
	// An empty query is a prompt before the first keystroke: everything matches
	// equally, so input order is the answer. Sorting would apply the
	// shorter-wins tie-break and reorder the menu before the user typed
	// anything.
	if query == "" {
		for _, cand := range candidates {
			out = append(out, Ranked{Candidate: cand})
		}
		return out
	}

	q := []rune(query)
	ignoreCase := smartCaseFold(q)
	if workers := runtime.GOMAXPROCS(0); len(candidates) >= parallelMin && workers > 1 {
		// Each worker scores a run of the candidates, and the runs are joined
		// in input order, so the result is exactly the one-core result.
		parts := make([][]Ranked, workers)
		size := (len(candidates) + workers - 1) / workers
		var wg sync.WaitGroup
		for w := range workers {
			lo, hi := min(w*size, len(candidates)), min((w+1)*size, len(candidates))
			wg.Go(func() { parts[w] = score(q, candidates[lo:hi], ignoreCase, nil) })
		}
		wg.Wait()
		for _, p := range parts {
			out = append(out, p...)
		}
	} else {
		out = score(q, candidates, ignoreCase, out)
	}
	slices.SortStableFunc(out, func(x, y Ranked) int {
		if x.Match.Score != y.Match.Score {
			return y.Match.Score - x.Match.Score
		}
		return len(x.Candidate) - len(y.Candidate)
	})
	return out
}

// score appends to out the candidates q matches, in input order.
func score(q []rune, candidates []string, ignoreCase bool, out []Ranked) []Ranked {
	var cbuf []rune
	for _, cand := range candidates {
		cbuf = decodeInto(cbuf, cand)
		if m, ok := scoreRunes(q, cbuf, ignoreCase); ok {
			out = append(out, Ranked{Candidate: cand, Match: m})
		}
	}
	return out
}
