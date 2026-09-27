package fuzzy

import (
	"sync"
	"unicode"
	"unicode/utf8"
)

// Scoring constants. The relative sizes matter far more than the absolute
// values: a boundary hit must be worth more than closing a one-rune gap, and a
// consecutive run must be worth more than a boundary, so that typing more of a
// word always strengthens the match.
const (
	// scoreMatch is earned by every matched rune, so a longer query that still
	// matches always outscores a shorter one.
	scoreMatch = 16

	// bonusBoundary is earned where a match lands at the start of a word: the
	// string start, just after a separator, or at a lower-to-upper transition.
	// This is the rule that makes "fwc" find forward-char instead of whichever
	// candidate happens to contain f, w and c mid-word.
	bonusBoundary = 8

	// bonusConsecutive is earned by a rune matching immediately after the
	// previous one, so "forw" strongly prefers forward-char over a candidate
	// where those runes are scattered.
	bonusConsecutive = 8

	// penaltyGapStart and penaltyGapExtend charge for skipped runes. Opening a
	// gap costs more than widening one, so several small gaps are worse than one
	// larger gap of the same total width.
	penaltyGapStart  = 3
	penaltyGapExtend = 1

	// penaltyLeading charges for runes before the first match, so an earlier
	// match wins between two otherwise equal alignments. This carries the
	// "prefer the start" job on its own: an earlier attempt weighted the first
	// rune's boundary bonus instead, which made anchoring at the string start
	// worth more than a four-rune gap costs - so "abc" against "axbxabc" scored
	// the scattered a-b-c above the contiguous tail. Capped, so a match deep
	// inside a long path is not drowned out entirely.
	penaltyLeading    = 1
	maxPenaltyLeading = 8

	// bonusCaseMatch is a nudge for matching case exactly while searching
	// case-insensitively, so "Ch" prefers a literal "Ch" over "ch".
	bonusCaseMatch = 1
)

// negInf marks an unreachable cell. Far enough below any real score that it can
// never win a maximum, and far enough above math.MinInt that adding a penalty
// cannot overflow.
const negInf = -1 << 30

// isBoundarySep reports whether r separates words.
func isBoundarySep(r rune) bool {
	switch r {
	case '-', '_', '/', '.', ' ', '\t', ':', ',':
		return true
	}
	return false
}

// boundaryBonus reports the bonus for matching at position j of c.
func boundaryBonus(c []rune, j int) int {
	if j == 0 {
		return bonusBoundary
	}
	if isBoundarySep(c[j-1]) {
		return bonusBoundary
	}
	if unicode.IsLower(c[j-1]) && unicode.IsUpper(c[j]) {
		return bonusBoundary
	}
	return 0
}

// scratch holds the dynamic programming tables. Reused through a pool because
// Rank scores every candidate on every keystroke.
type scratch struct {
	d   []int // best score with q[i] matched at c[j], row-major
	par []int // the c index that q[i-1] matched, for backtracking
	bon []int // boundaryBonus per candidate position

	// qf and cf hold the query and candidate folded, when case is ignored,
	// so a cell compares two runes rather than folding both first: each rune
	// of the candidate is folded once, not once for every rune of the query.
	qf, cf []rune
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

func (s *scratch) resize(n, m int) {
	if cap(s.d) < n*m {
		s.d = make([]int, n*m)
		s.par = make([]int, n*m)
	}
	s.d, s.par = s.d[:n*m], s.par[:n*m]
	if cap(s.bon) < m {
		s.bon = make([]int, m)
	}
	s.bon = s.bon[:m]
}

// folded returns q and c as a cell compares them: folded into s's buffers
// when case is ignored, and as they are when it is not.
func (s *scratch) folded(q, c []rune, ignoreCase bool) (qf, cf []rune) {
	if !ignoreCase {
		return q, c
	}
	s.qf, s.cf = s.qf[:0], s.cf[:0]
	for _, r := range q {
		s.qf = append(s.qf, fold(r))
	}
	for _, r := range c {
		s.cf = append(s.cf, fold(r))
	}
	return s.qf, s.cf
}

// smartCaseFold reports whether case is to be ignored in matching q.
//
// Smart case, matching nem's isearch: a query typed entirely in lower case
// ignores case, and a single upper-case rune makes the whole query sensitive.
func smartCaseFold(q []rune) bool {
	for _, r := range q {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// fold returns r as it is compared when case is ignored: in lower case.
//
// Folding runs for every rune of every candidate, and calling out to
// unicode.ToLower for each was a third of the time spent ranking. ASCII is
// lowered inline instead - the same answer, since for ASCII ToLower only
// maps A-Z to a-z - and anything else still goes through unicode, which
// knows that the Kelvin sign is a k.
func fold(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return r
	}
	return unicode.ToLower(r)
}

// runesEqual reports whether qr matches cr, ignoring case if asked.
func runesEqual(qr, cr rune, ignoreCase bool) bool {
	return qr == cr || ignoreCase && fold(qr) == fold(cr)
}

// isSubsequence reports whether q appears in c in order. A cheap O(len(c))
// rejection so the dynamic programming below only runs on real candidates,
// which matters because most candidates do not match. It folds the rune it
// is looking for once, not at every rune it looks at.
func isSubsequence(q, c []rune, ignoreCase bool) bool {
	if len(q) == 0 {
		return true
	}
	qi, want := 0, q[0]
	if ignoreCase {
		want = fold(want)
	}
	for _, r := range c {
		if ignoreCase {
			r = fold(r)
		}
		if r != want {
			continue
		}
		if qi++; qi == len(q) {
			return true
		}
		if want = q[qi]; ignoreCase {
			want = fold(want)
		}
	}
	return false
}

// leadingPenalty charges for runes skipped before the first match.
func leadingPenalty(j int) int {
	if p := j * penaltyLeading; p < maxPenaltyLeading {
		return p
	}
	return maxPenaltyLeading
}

// bestAlignment finds the highest-scoring way to match q within c and returns
// the score with the matched rune indices.
//
// This is a full dynamic program rather than a greedy scan, so it finds the best
// alignment and not merely the first one: "abc" against "axbxabc" scores the
// contiguous tail, which is what a reader would call the match. A greedy pass
// would lock onto a@0 and never reconsider.
//
// d[i][j] is the best total score for matching q[0..i] with q[i] landing exactly
// on c[lo+j]. The maximum over j of the last row is the answer.
//
// The table spans only c[lo..hi], from the first rune q[0] matches to the last
// rune q[n-1] does: no alignment starts before the one or ends after the
// other, so the columns cut off are ones that could never be part of the
// answer, and the table is the same without them. For a word matched deep in
// a long path it is a fraction of the width.
func bestAlignment(q, c []rune, ignoreCase bool) (int, []int) {
	n := len(q)
	head, tail := q[0], q[n-1]
	if ignoreCase {
		head, tail = fold(head), fold(tail)
	}
	lo, hi := 0, len(c)-1
	for ; lo <= hi; lo++ {
		if r := c[lo]; r == head || ignoreCase && fold(r) == head {
			break
		}
	}
	for ; hi >= lo; hi-- {
		if r := c[hi]; r == tail || ignoreCase && fold(r) == tail {
			break
		}
	}
	if lo > hi {
		// Unreachable, as below: the caller has established that q fits in c.
		return 0, nil
	}
	w := c[lo : hi+1]
	m := len(w)
	s := scratchPool.Get().(*scratch)
	defer scratchPool.Put(s)
	s.resize(n, m)
	d, par, bon := s.d, s.par, s.bon
	qf, wf := s.folded(q, w, ignoreCase)

	for j := range w {
		bon[j] = boundaryBonus(c, lo+j)
	}

	for j := 0; j < m; j++ {
		if qf[0] == wf[j] {
			score := scoreMatch + bon[j] - leadingPenalty(lo+j)
			if ignoreCase && q[0] == w[j] {
				score += bonusCaseMatch
			}
			d[j] = score
		} else {
			d[j] = negInf
		}
		par[j] = -1
	}

	for i := 1; i < n; i++ {
		row, prev := i*m, (i-1)*m
		// acc is the best value of d[i-1][k] + penaltyGapExtend*k over the k
		// that are far enough back to need a gap. Folding the distance-dependent
		// penalty into the accumulator is what keeps this linear per row instead
		// of quadratic.
		acc, accK := negInf, -1
		for j := 0; j < m; j++ {
			if k := j - 2; k >= 0 && d[prev+k] > negInf {
				if v := d[prev+k] + penaltyGapExtend*k; v > acc {
					acc, accK = v, k
				}
			}
			if qf[i] != wf[j] {
				d[row+j], par[row+j] = negInf, -1
				continue
			}

			base := scoreMatch + bon[j]
			if ignoreCase && q[i] == w[j] {
				base += bonusCaseMatch
			}

			best, from := negInf, -1
			if j > 0 && d[prev+j-1] > negInf {
				best, from = d[prev+j-1]+bonusConsecutive, j-1
			}
			if accK >= 0 {
				if v := acc - penaltyGapStart - penaltyGapExtend*(j-2); v > best {
					best, from = v, accK
				}
			}
			if from < 0 {
				d[row+j], par[row+j] = negInf, -1
				continue
			}
			d[row+j], par[row+j] = base+best, from
		}
	}

	last := (n - 1) * m
	bestJ, bestScore := -1, negInf
	for j := 0; j < m; j++ {
		if d[last+j] > bestScore {
			bestScore, bestJ = d[last+j], j
		}
	}
	if bestJ < 0 {
		// Unreachable: isSubsequence already established that q fits in c, so
		// the last row has at least one live cell. Kept as a guard so a future
		// change to the caller cannot turn a missing match into a bad index.
		return 0, nil
	}

	idx := make([]int, n)
	for i, j := n-1, bestJ; i >= 0; i-- {
		idx[i] = lo + j
		j = par[i*m+j]
	}
	return bestScore, idx
}

// scoreRun scores t matched as one contiguous run at c[s:], by the rules
// bestAlignment scores any alignment by: every rune its match and boundary
// bonus, every rune after the first the consecutive bonus, and the runes
// skipped before it their penalty. An exact or anchored term is so on the
// same scale as a fuzzy one, and a run that starts a word still wins.
func scoreRun(t, c []rune, s int, ignoreCase bool) int {
	score := (len(t)-1)*bonusConsecutive - leadingPenalty(s)
	for k, r := range t {
		score += scoreMatch + boundaryBonus(c, s+k)
		if ignoreCase && r == c[s+k] {
			score += bonusCaseMatch
		}
	}
	return score
}

// runesAt reports whether t matches c rune for rune at offset s. The caller
// makes sure it fits.
func runesAt(c []rune, s int, t []rune, ignoreCase bool) bool {
	for k, r := range t {
		if !runesEqual(r, c[s+k], ignoreCase) {
			return false
		}
	}
	return true
}

// indexRunes returns the first offset at or after from where t matches c as a
// contiguous run, or -1 if there is none. It looks for t's first rune, folded
// once, and compares the rest only where that is found: most candidates an
// exact term rejects never get past the first rune.
func indexRunes(c []rune, from int, t []rune, ignoreCase bool) int {
	if len(t) == 0 {
		return min(from, len(c))
	}
	first := t[0]
	if ignoreCase {
		first = fold(first)
	}
	for s := from; s+len(t) <= len(c); s++ {
		if r := c[s]; (r == first || ignoreCase && fold(r) == first) && runesAt(c, s+1, t[1:], ignoreCase) {
			return s
		}
	}
	return -1
}

// maxCells is the largest table bestAlignment fills: a query and candidate
// of lengths n and m take n*m cells. Typed queries against names and paths
// are thousands; a kill yanked into a prompt, against a buffer named after
// another, was billions - minutes of work and more memory than the machine
// has. Past it, firstAlignment scores the match in one pass.
const maxCells = 1 << 18

// firstAlignment matches each rune of q to the first rune of c it can take,
// scored by the same rules as bestAlignment. It is not the best alignment,
// but found in one pass and no memory beyond the indices, for a query and
// candidate too long to search every alignment of.
func firstAlignment(q, c []rune, ignoreCase bool) (int, []int) {
	idx := make([]int, 0, len(q))
	score := 0
	for j := 0; j < len(c) && len(idx) < len(q); j++ {
		r := q[len(idx)]
		if !runesEqual(r, c[j], ignoreCase) {
			continue
		}
		score += scoreMatch + boundaryBonus(c, j)
		if ignoreCase && r == c[j] {
			score += bonusCaseMatch
		}
		switch prev := len(idx) - 1; {
		case prev < 0:
			score -= leadingPenalty(j)
		case idx[prev] == j-1:
			score += bonusConsecutive
		default:
			score -= penaltyGapStart + penaltyGapExtend*(j-idx[prev]-2)
		}
		idx = append(idx, j)
	}
	return score, idx
}

// decodeInto decodes s into dst's storage, growing it only when necessary, so a
// ranking pass over many candidates does not allocate once per candidate.
func decodeInto(dst []rune, s string) []rune {
	dst = dst[:0]
	for _, r := range s {
		dst = append(dst, r)
	}
	return dst
}
