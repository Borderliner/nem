package fuzzy

import (
	"sync"
	"unicode"
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

// fold reports the matching function and whether case is being ignored.
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

func runesEqual(qr, cr rune, ignoreCase bool) bool {
	if qr == cr {
		return true
	}
	return ignoreCase && unicode.ToLower(qr) == unicode.ToLower(cr)
}

// isSubsequence reports whether q appears in c in order. A cheap O(len(c))
// rejection so the dynamic programming below only runs on real candidates,
// which matters because most candidates do not match.
func isSubsequence(q, c []rune, ignoreCase bool) bool {
	qi := 0
	for ci := 0; ci < len(c) && qi < len(q); ci++ {
		if runesEqual(q[qi], c[ci], ignoreCase) {
			qi++
		}
	}
	return qi == len(q)
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
// on c[j]. The maximum over j of the last row is the answer.
func bestAlignment(q, c []rune, ignoreCase bool) (int, []int) {
	n, m := len(q), len(c)
	s := scratchPool.Get().(*scratch)
	defer scratchPool.Put(s)
	s.resize(n, m)
	d, par, bon := s.d, s.par, s.bon

	for j := range c {
		bon[j] = boundaryBonus(c, j)
	}

	for j := 0; j < m; j++ {
		if runesEqual(q[0], c[j], ignoreCase) {
			score := scoreMatch + bon[j] - leadingPenalty(j)
			if ignoreCase && q[0] == c[j] {
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
			if !runesEqual(q[i], c[j], ignoreCase) {
				d[row+j], par[row+j] = negInf, -1
				continue
			}

			base := scoreMatch + bon[j]
			if ignoreCase && q[i] == c[j] {
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
		idx[i] = j
		j = par[i*m+j]
	}
	return bestScore, idx
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
