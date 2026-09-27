// Package fuzzy ranks candidate strings against a short query typed by a user.
//
// Matching is subsequence-based: the query's runes must appear in the candidate
// in order but need not be adjacent, so "fwc" finds "forward-char". Scoring
// rewards matches that land at the start of a word and runs of adjacent runes,
// and charges for the gaps between them, so the ranking reflects how a reader
// would judge the match rather than merely whether one exists.
//
// A query is read as fzf reads its extended search syntax. Spaces separate
// terms, and a candidate must match every one. A term is fuzzy, as above,
// unless it is marked:
//
//	'abc    exact: abc appears, contiguous
//	^abc    prefix: the candidate starts with abc
//	abc$    suffix: the candidate ends with abc
//	^abc$   the candidate is abc
//	!abc    the candidate does not contain abc; !^abc and !abc$ say it does
//	        not start or end with it, and !'abc is !abc
//	a | b   either: a lone | joins the terms each side into one that
//	        matches when any of them does, so "a | b c" is (a or b) and c
//
// A backslash before a space makes the space part of a term. Case is smart
// per term: a term with an upper-case letter is exact about case, and one
// without ignores it. A marker with no word after it yet - the ' typed before
// one, or a | with no term after it - is ignored rather than matched, so the
// list does not empty and refill as the user types. A query far longer than
// anyone types was yanked into the prompt, and is read as text, not syntax.
//
// A candidate scores the sum of its terms' scores. An exact or anchored term
// is scored as the same runes matched in a row would be, word starts and all,
// and a negated one adds nothing. So a query of one plain word ranks exactly
// as it would if there were no syntax at all.
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
	// strictly ascending. For a plain query there is one per query rune; for
	// one of several terms they are every term's together, each offset once,
	// and a negated term contributes none. Callers emphasise exactly these
	// positions, which is why they are rune offsets and not byte offsets.
	Indices []int
}

// Score reports how well query matches candidate, and whether it matches at all.
//
// An empty query matches every candidate with score 0 and no indices, so a
// prompt showing everything before the user types needs no special case. So
// does a query of markers alone, such as a ' with no word after it yet.
func Score(query, candidate string) (Match, bool) {
	p := parse(query)
	if len(p.groups) == 0 {
		return Match{}, true
	}
	if !p.admits(candidate) {
		return Match{}, false
	}
	return p.match([]rune(candidate))
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
	// anything. A query of markers alone is the same prompt a keystroke
	// later, the ' or ^ typed before a word, and shows the same list.
	p := parse(query)
	if len(p.groups) == 0 {
		for _, cand := range candidates {
			out = append(out, Ranked{Candidate: cand})
		}
		return out
	}

	if workers := runtime.GOMAXPROCS(0); len(candidates) >= parallelMin && workers > 1 {
		// Each worker scores a run of the candidates, and the runs are joined
		// in input order, so the result is exactly the one-core result.
		parts := make([][]Ranked, workers)
		size := (len(candidates) + workers - 1) / workers
		var wg sync.WaitGroup
		for w := range workers {
			lo, hi := min(w*size, len(candidates)), min((w+1)*size, len(candidates))
			wg.Go(func() { parts[w] = score(&p, candidates[lo:hi], nil) })
		}
		wg.Wait()
		for _, part := range parts {
			out = append(out, part...)
		}
	} else {
		out = score(&p, candidates, out)
	}
	slices.SortStableFunc(out, func(x, y Ranked) int {
		if x.Match.Score != y.Match.Score {
			return y.Match.Score - x.Match.Score
		}
		return len(x.Candidate) - len(y.Candidate)
	})
	return out
}

// score appends to out the candidates p matches, in input order.
//
// The query was parsed once for the whole pass, and each worker shares it
// read-only. Candidates are decoded into one buffer reused across the pass:
// decoding each into its own cost an allocation per candidate on every
// keystroke, which dominated ranking a large directory.
func score(p *pattern, candidates []string, out []Ranked) []Ranked {
	var cbuf []rune
	for _, cand := range candidates {
		if !p.admits(cand) {
			continue
		}
		cbuf = decodeInto(cbuf, cand)
		if m, ok := p.match(cbuf); ok {
			out = append(out, Ranked{Candidate: cand, Match: m})
		}
	}
	return out
}
