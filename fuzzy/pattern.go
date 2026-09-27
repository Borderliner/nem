package fuzzy

import "slices"

// maxSyntaxLen is the longest query, in runes, read as search syntax. A
// longer one is a single fuzzy term, spaces, quotes and all: nobody types a
// query that long, so it was yanked into the prompt, and a kill is text to
// find as it stands. Read as syntax, each of its words would match anywhere
// and in any order, and a word that happened to start with ! would turn the
// match inside out.
const maxSyntaxLen = 256

// termKind says where a term's text must appear in a candidate.
type termKind uint8

const (
	termFuzzy  termKind = iota // its runes in order, gaps allowed: abc
	termExact                  // one contiguous run anywhere: 'abc, and !abc
	termPrefix                 // at the very start: ^abc
	termSuffix                 // at the very end: abc$
	termEqual                  // the whole candidate: ^abc$
)

// term is one word of a query.
type term struct {
	kind termKind
	// not inverts the term: the candidate must NOT have the text where kind
	// says. Such a term only ever rejects. It adds nothing to the score and
	// no indices, because what it describes is by definition not there.
	not  bool
	text []rune
	// ignoreCase is smart case, decided per term, so that "Makefile !test"
	// is exact about the one and not the other.
	ignoreCase bool
}

// group is terms joined by a lone |. It holds when any one of them does.
type group []term

// pattern is a parsed query. A candidate matches when every group holds.
type pattern struct {
	groups []group
	// plain is set for a query of one fuzzy term and nothing else, which is
	// what nearly every query still is. It is matched exactly as queries were
	// before the syntax existed, with none of the work of combining terms.
	plain bool
	// lone is where the groups of a single fuzzy term begin. They sort last,
	// and admits looks for them in a candidate before it is decoded.
	lone int
}

// parse reads query as fzf reads its extended search syntax.
//
// Words are separated by runs of spaces, and a backslash before a space
// makes it part of the word instead. A word reads as:
//
//	abc     fuzzy: its runes in order, gaps allowed
//	'abc    exact: abc somewhere, contiguous
//	^abc    prefix: the candidate starts with abc
//	abc$    suffix: the candidate ends with abc; 'abc$ is the same
//	^abc$   the candidate is abc
//	!abc    the candidate does not contain abc, and !'abc is the same;
//	        !^abc, !abc$ and !^abc$ invert the anchored forms
//	|       joins the terms either side of it into a group
//
// A | binds tighter than the spaces around it, so "a | b c" is (a or b) and
// c. A word of markers alone - ', ^, $, !, !^ - is dropped, as is a | with no
// term on one side of it: that is what they are while the word after them is
// still being typed, and dropping them rather than matching them literally
// keeps the list from emptying on one keystroke and refilling on the next.
func parse(query string) pattern {
	q := []rune(query)
	if len(q) > maxSyntaxLen {
		t := term{kind: termFuzzy, text: q, ignoreCase: smartCaseFold(q)}
		return pattern{groups: []group{{t}}, plain: true}
	}
	var p pattern
	or := false // a | since the last term, so the next joins its group
	for _, word := range splitWords(q) {
		if len(word) == 1 && word[0] == '|' {
			or = len(p.groups) > 0
			continue
		}
		t, ok := parseTerm(word)
		if !ok {
			continue
		}
		if last := len(p.groups) - 1; or {
			p.groups[last] = append(p.groups[last], t)
		} else {
			p.groups = append(p.groups, group{t})
		}
		or = false
	}
	p.plain = len(p.groups) == 1 && p.groups[0].loneFuzzy()
	// Groups are tested cheapest first. The score is a sum and the indices a
	// sorted union, so the order changes nothing but the time.
	slices.SortStableFunc(p.groups, func(a, b group) int { return a.cost() - b.cost() })
	if p.lone = slices.IndexFunc(p.groups, group.loneFuzzy); p.lone < 0 {
		p.lone = len(p.groups)
	}
	return p
}

// splitWords splits q into words at runs of spaces, taking a backslash-escaped
// space as part of a word. It works in place, since dropping an escape only
// ever shortens what is left, and the words share q's storage.
func splitWords(q []rune) [][]rune {
	var words [][]rune
	n, start := 0, 0 // q[:n] is what is kept; the current word is q[start:n]
	for i := 0; i < len(q); i++ {
		switch {
		case q[i] == '\\' && i+1 < len(q) && q[i+1] == ' ':
			q[n] = ' '
			n++
			i++
		case q[i] == ' ':
			if n > start {
				words = append(words, q[start:n:n])
			}
			start = n
		default:
			q[n] = q[i]
			n++
		}
	}
	if n > start {
		words = append(words, q[start:n:n])
	}
	return words
}

// parseTerm reads a word's markers, reporting false for a word of markers
// alone.
func parseTerm(w []rune) (term, bool) {
	t := term{kind: termFuzzy}
	if w[0] == '!' {
		// Negation is exact, as in fzf: a fuzzy match is so easily had that
		// "not a fuzzy match" would reject nearly every candidate.
		t.not, t.kind, w = true, termExact, w[1:]
	}
	start, end := false, false
	if len(w) > 0 && w[0] == '\'' {
		t.kind, w = termExact, w[1:]
	} else if len(w) > 0 && w[0] == '^' {
		start, w = true, w[1:]
	}
	if len(w) > 0 && w[len(w)-1] == '$' {
		end, w = true, w[:len(w)-1]
	}
	switch {
	case start && end:
		t.kind = termEqual
	case start:
		t.kind = termPrefix
	case end:
		t.kind = termSuffix
	}
	if len(w) == 0 {
		return term{}, false
	}
	t.text, t.ignoreCase = w, smartCaseFold(w)
	return t, true
}

// cost ranks a group by how cheaply it is tested: anchored terms compare a
// few runes at a fixed place, exact ones scan, and fuzzy ones scan and match
// most of what they scan.
//
// A lone fuzzy term ranks apart, and last in the order. It is tested first
// all the same, by admits, in the candidate's string before it is decoded:
// most candidates it turns away for less than decoding them would cost.
func (g group) cost() int {
	if g.loneFuzzy() {
		return 3
	}
	c := 0
	for _, t := range g {
		switch t.kind {
		case termExact:
			c = max(c, 1)
		case termFuzzy:
			c = max(c, 2)
		}
	}
	return c
}

// found reports whether t's text is in c where t's kind says, ignoring
// negation. It is the cheap test, run before anything is scored: a scan at
// most, and no dynamic programming.
func (t *term) found(c []rune) bool {
	n, m := len(t.text), len(c)
	if n > m {
		return false
	}
	switch t.kind {
	case termFuzzy:
		return isSubsequence(t.text, c, t.ignoreCase)
	case termExact:
		return indexRunes(c, 0, t.text, t.ignoreCase) >= 0
	case termPrefix:
		return runesAt(c, 0, t.text, t.ignoreCase)
	case termSuffix:
		return runesAt(c, m-n, t.text, t.ignoreCase)
	case termEqual:
		return n == m && runesAt(c, 0, t.text, t.ignoreCase)
	}
	return false
}

// score scores a positive t against c, which found has already accepted,
// and returns the rune offsets it matched.
func (t *term) score(c []rune, a *arena) (int, []int) {
	n := len(t.text)
	switch t.kind {
	case termFuzzy:
		if n*len(c) > maxCells {
			return firstAlignment(t.text, c, t.ignoreCase, a)
		}
		return bestAlignment(t.text, c, t.ignoreCase, a)
	case termExact:
		// The best of its occurrences, as bestAlignment takes the best of its
		// alignments: 'go against "cargo/go.mod" is the go that starts a word.
		best, at := negInf, 0
		for s := indexRunes(c, 0, t.text, t.ignoreCase); s >= 0; s = indexRunes(c, s+1, t.text, t.ignoreCase) {
			if v := scoreRun(t.text, c, s, t.ignoreCase); v > best {
				best, at = v, s
			}
		}
		return best, runIndices(at, n, a)
	case termSuffix:
		at := len(c) - n
		return scoreRun(t.text, c, at, t.ignoreCase), runIndices(at, n, a)
	}
	return scoreRun(t.text, c, 0, t.ignoreCase), runIndices(0, n, a)
}

// runIndices returns the n offsets from at.
func runIndices(at, n int, a *arena) []int {
	idx := a.take(n)
	for k := range idx {
		idx[k] = at + k
	}
	return idx
}

// holds reports whether any of g's terms accepts c.
func (g group) holds(c []rune) bool {
	for i := range g {
		if g[i].found(c) != g[i].not {
			return true
		}
	}
	return false
}

// score scores the best of g's positive terms that accepts c. A group that
// holds only through a negated term scores nothing: preferring it to a
// positive match that scored below zero would throw that match's indices
// away for a term that matched nothing to show.
func (g group) score(c []rune, a *arena) (int, []int) {
	if len(g) == 1 {
		// holds has already accepted c, and a lone term needs no second look.
		if g[0].not {
			return 0, nil
		}
		return g[0].score(c, a)
	}
	best, bestIdx, ok := 0, []int(nil), false
	for i := range g {
		t := &g[i]
		if t.not || !t.found(c) {
			continue
		}
		if s, idx := t.score(c, a); !ok || s > best {
			best, bestIdx, ok = s, idx, true
		}
	}
	return best, bestIdx
}

// loneFuzzy reports whether g is a single fuzzy term, which admits tests.
func (g group) loneFuzzy() bool {
	return len(g) == 1 && g[0].kind == termFuzzy && !g[0].not
}

// admits reports whether s may match p, looking for p's lone fuzzy terms in
// the string itself. A candidate it turns away does not match; one it lets
// through is decoded and matched on the rest.
func (p *pattern) admits(s string) bool {
	for _, g := range p.groups[p.lone:] {
		t := &g[0]
		if len(t.text) > len(s) || !isSubsequenceIn(t.text, s, t.ignoreCase) {
			return false
		}
	}
	return true
}

// match scores c against p, where c is a candidate admits has let through,
// decoded, taking the indices from a. The lone fuzzy terms admits looked for
// are known to hold, and every other group is tested before any is scored,
// so a candidate that the last group rejects costs no alignment for the
// first.
func (p *pattern) match(c []rune, a *arena) (Match, bool) {
	if p.plain {
		s, idx := p.groups[0][0].score(c, a)
		return Match{Score: s, Indices: idx}, true
	}
	for _, g := range p.groups[:p.lone] {
		if !g.holds(c) {
			return Match{}, false
		}
	}
	var m Match
	merged := false
	for _, g := range p.groups {
		s, idx := g.score(c, a)
		m.Score += s
		switch {
		case idx == nil:
		case m.Indices == nil:
			// Every term's indices are a slice of their own, capped at its
			// length, so the first can take the rest: appending copies it.
			m.Indices = idx
		default:
			m.Indices = append(m.Indices, idx...)
			merged = true
		}
	}
	if merged {
		// Two terms can match the same rune - "go 'go" - and a highlighter
		// walks the offsets in order, so they are sorted and each kept once.
		slices.Sort(m.Indices)
		m.Indices = slices.Compact(m.Indices)
	}
	return m, true
}
