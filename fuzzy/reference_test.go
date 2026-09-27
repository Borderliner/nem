package fuzzy

import (
	"slices"
	"unicode"
)

// The scorer as it was written first, before it was made fast: every column
// of the table, every rune folded where it is compared, every candidate
// decoded. The fast one skips work only where it cannot change the answer,
// and these are what the tests hold it to - rune for rune, score for score -
// so a shortcut that does change the answer fails a test instead of quietly
// reordering somebody's menu.

func refRunesEqual(qr, cr rune, ignoreCase bool) bool {
	return qr == cr || ignoreCase && unicode.ToLower(qr) == unicode.ToLower(cr)
}

func refIsSubsequence(q, c []rune, ignoreCase bool) bool {
	qi := 0
	for ci := 0; ci < len(c) && qi < len(q); ci++ {
		if refRunesEqual(q[qi], c[ci], ignoreCase) {
			qi++
		}
	}
	return qi == len(q)
}

// refScore scores a plain query, as Score did before there was any syntax.
func refScore(query, candidate string) (Match, bool) {
	q, c := []rune(query), []rune(candidate)
	if len(q) == 0 {
		return Match{}, true
	}
	ignoreCase := smartCaseFold(q)
	if len(q) > len(c) || !refIsSubsequence(q, c, ignoreCase) {
		return Match{}, false
	}
	var score int
	var idx []int
	if len(q)*len(c) > maxCells {
		score, idx = refFirstAlignment(q, c, ignoreCase)
	} else {
		score, idx = refBestAlignment(q, c, ignoreCase)
	}
	return Match{Score: score, Indices: idx}, true
}

// refRank ranks a plain query, as Rank did before there was any syntax.
func refRank(query string, candidates []string) []Ranked {
	out := []Ranked{}
	for i, cand := range candidates {
		if m, ok := refScore(query, cand); ok {
			out = append(out, Ranked{Candidate: cand, Match: m, Index: i})
		}
	}
	if query == "" {
		return out
	}
	slices.SortStableFunc(out, func(x, y Ranked) int {
		if x.Match.Score != y.Match.Score {
			return y.Match.Score - x.Match.Score
		}
		return len(x.Candidate) - len(y.Candidate)
	})
	return out
}

func refBestAlignment(q, c []rune, ignoreCase bool) (int, []int) {
	n, m := len(q), len(c)
	d, par, bon := make([]int, n*m), make([]int, n*m), make([]int, m)
	for j := range c {
		bon[j] = boundaryBonus(c, j)
	}
	for j := 0; j < m; j++ {
		d[j], par[j] = negInf, -1
		if refRunesEqual(q[0], c[j], ignoreCase) {
			d[j] = scoreMatch + bon[j] - leadingPenalty(j)
			if ignoreCase && q[0] == c[j] {
				d[j] += bonusCaseMatch
			}
		}
	}
	for i := 1; i < n; i++ {
		row, prev := i*m, (i-1)*m
		acc, accK := negInf, -1
		for j := 0; j < m; j++ {
			if k := j - 2; k >= 0 && d[prev+k] > negInf {
				if v := d[prev+k] + penaltyGapExtend*k; v > acc {
					acc, accK = v, k
				}
			}
			d[row+j], par[row+j] = negInf, -1
			if !refRunesEqual(q[i], c[j], ignoreCase) {
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
			if from >= 0 {
				d[row+j], par[row+j] = base+best, from
			}
		}
	}
	last := (n - 1) * m
	bestJ, bestScore := -1, negInf
	for j := 0; j < m; j++ {
		if d[last+j] > bestScore {
			bestScore, bestJ = d[last+j], j
		}
	}
	idx := make([]int, n)
	for i, j := n-1, bestJ; i >= 0; i-- {
		idx[i] = j
		j = par[i*m+j]
	}
	return bestScore, idx
}

func refFirstAlignment(q, c []rune, ignoreCase bool) (int, []int) {
	idx := make([]int, 0, len(q))
	score := 0
	for j := 0; j < len(c) && len(idx) < len(q); j++ {
		r := q[len(idx)]
		if !refRunesEqual(r, c[j], ignoreCase) {
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
