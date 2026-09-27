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
	"cmp"
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
	return p.match([]rune(candidate), nil)
}

// Ranked is one candidate and how it matched.
type Ranked struct {
	Candidate string
	Match     Match
	// Index is the candidate's place in the list given to Rank, so what
	// matched can be put back in the list's own order: for a list kept in
	// its order, or for the next keystroke's Rank to narrow.
	Index int
}

// parallelMin is how many candidates it takes for Rank to share the scoring
// out across the CPUs. Below it one core finishes before the others would have
// started; above it - every file in a large project - scoring on one core took
// a tenth of a second a keystroke.
const parallelMin = 4096

// Rank returns the matching candidates, best first.
//
// The order is total: by score descending, then by shorter candidate, then by
// the candidate's position in the input.
func Rank(query string, candidates []string) []Ranked {
	// An empty query is a prompt before the first keystroke: everything matches
	// equally, so input order is the answer. Sorting would apply the
	// shorter-wins tie-break and reorder the menu before the user typed
	// anything. A query of markers alone is the same prompt a keystroke
	// later, the ' or ^ typed before a word, and shows the same list.
	p := parse(query)
	if len(p.groups) == 0 {
		out := make([]Ranked, 0, len(candidates))
		for i, cand := range candidates {
			out = append(out, Ranked{Candidate: cand, Index: i})
		}
		return out
	}

	workers := runtime.GOMAXPROCS(0)
	if len(candidates) < parallelMin || workers < 2 {
		// Found in the list's order, so a stable sort settles the ties on
		// score and length without ever comparing places: for a list this
		// short, faster than pdqsort, which must. Each match's indices are
		// the result's, and the arena keeps them.
		out := make([]Ranked, 0, len(candidates))
		var a arena
		scan(&p, candidates, &a, false, func(i int, m Match) {
			out = append(out, Ranked{Candidate: candidates[i], Match: m, Index: i})
		})
		slices.SortStableFunc(out, func(x, y Ranked) int {
			if x.Match.Score != y.Match.Score {
				return cmp.Compare(y.Match.Score, x.Match.Score)
			}
			return len(x.Candidate) - len(y.Candidate)
		})
		return out
	}
	// Each worker scores and sorts a run of the candidates, and the runs are
	// merged. The order is total, so this is exactly the one-core result.
	// Sorting the whole on one core, after the scoring, was most of the time
	// a keystroke took over the files of a very large tree; and at tens of
	// thousands a run, a stable sort's extra log factor loses to pdqsort.
	runs := make([]run, workers)
	size := (len(candidates) + workers - 1) / workers
	var wg sync.WaitGroup
	for w := range workers {
		lo, hi := min(w*size, len(candidates)), min((w+1)*size, len(candidates))
		wg.Go(func() {
			r := &runs[w]
			var a arena
			scan(&p, candidates[lo:hi], &a, true, func(i int, m Match) { r.add(lo+i, len(candidates[lo+i]), m) })
			slices.SortFunc(r.hits, better)
		})
	}
	wg.Wait()
	return merge(runs, candidates)
}

// scan calls add with the place of each candidate p matches, in input order,
// and its match.
//
// The query was parsed once for the whole pass, and each worker shares it
// read-only. Candidates are decoded into one buffer reused across the pass,
// and indices are taken from a, reset for each candidate when add copies
// them out: an allocation for each candidate, on every keystroke, dominated
// ranking a large directory.
func scan(p *pattern, candidates []string, a *arena, reset bool, add func(int, Match)) {
	var cbuf []rune
	for i, cand := range candidates {
		if !p.admits(cand) {
			continue
		}
		cbuf = decodeInto(cbuf, cand)
		if reset {
			a.reset()
		}
		if m, ok := p.match(cbuf, a); ok {
			add(i, m)
		}
	}
}

// hit is a match as a worker finds and sorts it: numbers only. A Ranked holds a
// string and a slice, and a sort moves each element many times; moving
// pointers about the heap is work the collector has to be told of, which
// was a fifth of a keystroke's time over the files of a very large tree.
type hit struct {
	score  int
	length int // of the candidate, in bytes
	index  int // the candidate's place in the list
	// The match's indices are the run's ind[at:at+n].
	at, n int
}

// run is what one worker finds: the hits, in the list's order until they
// are sorted, and the indices of them all, end to end.
type run struct {
	hits []hit
	ind  []int
}

// better orders x before y when x is the better match: the higher score, then
// the shorter candidate, then the earlier in the list. No two candidates
// share a place in the list, so the order is total: runs sorted by it apart,
// by any sort, and merged, are the one ranking a sort of the whole makes.
func better(x, y hit) int {
	if x.score != y.score {
		return cmp.Compare(y.score, x.score)
	}
	if x.length != y.length {
		return x.length - y.length
	}
	return x.index - y.index
}

// ranked returns h, a hit of r's, as Rank reports it.
func (r *run) ranked(h hit, candidates []string) Ranked {
	var idx []int
	if h.n > 0 {
		// A match with no positive term has no indices, and they are nil,
		// as Score reports them.
		idx = r.ind[h.at : h.at+h.n : h.at+h.n]
	}
	return Ranked{Candidate: candidates[h.index], Match: Match{Score: h.score, Indices: idx}, Index: h.index}
}

// merge joins runs, each sorted by better, into one ranking.
//
// It is sized by what matched, not by what was offered: room for every file
// in a large project was megabytes a keystroke, nearly all of it for
// candidates the query had already turned away.
func merge(runs []run, candidates []string) []Ranked {
	n := 0
	for _, r := range runs {
		n += len(r.hits)
	}
	out := make([]Ranked, 0, n)
	// h is a heap of the runs with hits left, by the first of each.
	h := make([]*run, 0, len(runs))
	for i := range runs {
		if len(runs[i].hits) > 0 {
			h = append(h, &runs[i])
		}
	}
	less := func(i, j int) bool { return better(h[i].hits[0], h[j].hits[0]) < 0 }
	down := func(i int) {
		for {
			l := 2*i + 1
			if l >= len(h) {
				return
			}
			if r := l + 1; r < len(h) && less(r, l) {
				l = r
			}
			if !less(l, i) {
				return
			}
			h[i], h[l] = h[l], h[i]
			i = l
		}
	}
	for i := len(h)/2 - 1; i >= 0; i-- {
		down(i)
	}
	for len(h) > 1 {
		r := h[0]
		out = append(out, r.ranked(r.hits[0], candidates))
		if r.hits = r.hits[1:]; len(r.hits) == 0 {
			h[0] = h[len(h)-1]
			h = h[:len(h)-1]
		}
		down(0)
	}
	if len(h) == 1 {
		for _, x := range h[0].hits {
			out = append(out, h[0].ranked(x, candidates))
		}
	}
	return out
}

// add adds to r the match m of the candidate at index, of length bytes,
// copying its indices.
func (r *run) add(index, length int, m Match) {
	// Both grow by doubling, where append grows a long slice by a quarter
	// at a time: a keystroke that matched most of a very large tree copied
	// what it had found five times over.
	if len(r.hits) == cap(r.hits) {
		r.hits = slices.Grow(r.hits, max(len(r.hits), 64))
	}
	if cap(r.ind)-len(r.ind) < len(m.Indices) {
		r.ind = slices.Grow(r.ind, max(len(r.ind), len(m.Indices), 256))
	}
	r.hits = append(r.hits, hit{score: m.Score, length: length, index: index, at: len(r.ind), n: len(m.Indices)})
	r.ind = append(r.ind, m.Indices...)
}
