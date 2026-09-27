package bidi

import "slices"

// sequence is an isolating run sequence (BD13): the level runs that the
// weak, neutral and implicit rules treat as one stretch of text, an isolate
// initiator's run carrying on after its matching PDI as if what was isolated
// between them were not there.
type sequence struct {
	// idx holds the positions in the paragraph of the sequence's
	// characters, in order; nil means all of them.
	idx []int32
	// level is the embedding level all of them share.
	level uint8
	// sos and eos are the directions, tL or tR, of what comes before the
	// sequence and after it (X10).
	sos, eos class
}

// isolatingRuns applies X9 and X10 once the explicit rules have set every
// level: it splits what X9 leaves of the paragraph into level runs, chains
// them into isolating run sequences and resolves each. Isolates says the
// paragraph has isolate initiators or PDIs; without them every level run is
// a sequence of its own.
func (p *paragraph) isolatingRuns(isolates bool) {
	n := len(p.rs)
	// kept is the paragraph as X9 leaves it: the positions of the
	// characters it does not remove.
	kept := make([]int32, 0, n)
	for i, c := range p.initial {
		if !removed(c) {
			kept = append(kept, int32(i))
		}
	}
	if len(kept) == 0 {
		return
	}

	// The level runs (BD7), as ranges of kept.
	type span struct{ start, end int }
	var runs []span
	start := 0
	for k := 1; k <= len(kept); k++ {
		if k == len(kept) || p.levels[kept[k]] != p.levels[kept[k-1]] {
			runs = append(runs, span{start, k})
			start = k
		}
	}

	// Chain them into sequences. Every sequence's sos and eos is worked out
	// before any is resolved, as X10 asks, since resolving one rewrites
	// levels its neighbours' sos and eos are taken from.
	var seqs []sequence
	add := func(idx []int32, first, last span) {
		s := sequence{idx: idx, level: p.levels[idx[0]]}
		before, after := p.base, p.base
		if first.start > 0 {
			before = p.levels[kept[first.start-1]]
		}
		// An isolate initiator that ends a sequence has no matching PDI:
		// what follows it is inside the isolate, so it is measured
		// against the paragraph instead.
		if last.end < len(kept) && !isIsolateInitiator(p.initial[idx[len(idx)-1]]) {
			after = p.levels[kept[last.end]]
		}
		s.sos, s.eos = direction(max(before, s.level)), direction(max(after, s.level))
		seqs = append(seqs, s)
	}

	if !isolates {
		for _, r := range runs {
			add(kept[r.start:r.end], r, r)
		}
	} else {
		matching := matchingPDIs(p.initial)
		// startsRun maps the position of a PDI that begins a level run to
		// that run, for an initiator's run to be continued by it.
		startsRun := make(map[int32]int, 4)
		for k, r := range runs {
			if i := kept[r.start]; p.initial[i] == tPDI {
				startsRun[i] = k
			}
		}
		chained := make([]bool, len(runs))
		buf := make([]int32, 0, len(kept))
		for k := range runs {
			if chained[k] {
				continue
			}
			from := len(buf)
			cur := k
			for {
				chained[cur] = true
				buf = append(buf, kept[runs[cur].start:runs[cur].end]...)
				last := kept[runs[cur].end-1]
				if !isIsolateInitiator(p.initial[last]) {
					break
				}
				pdi, ok := matching[last]
				if !ok {
					break
				}
				nextRun, ok := startsRun[pdi]
				if !ok {
					break
				}
				cur = nextRun
			}
			add(buf[from:len(buf):len(buf)], runs[k], runs[cur])
		}
	}

	for _, s := range seqs {
		p.resolve(s)
	}
}

// matchingPDIs pairs each isolate initiator with its matching PDI (BD9): the
// first PDI after it that is not closing an isolate opened after it.
func matchingPDIs(cs []class) map[int32]int32 {
	m := make(map[int32]int32, 4)
	var open []int32
	for i, c := range cs {
		switch {
		case isIsolateInitiator(c):
			open = append(open, int32(i))
		case c == tPDI && len(open) > 0:
			m[open[len(open)-1]] = int32(i)
			open = open[:len(open)-1]
		}
	}
	return m
}

// resolve applies the weak rules (W1–W7), the neutral rules (N0–N2) and the
// implicit ones (I1–I2) to one isolating run sequence, and sets the levels
// of its characters.
func (p *paragraph) resolve(s sequence) {
	n := len(p.rs)
	if s.idx != nil {
		n = len(s.idx)
	}
	pos := func(i int) int {
		if s.idx == nil {
			return i
		}
		return int(s.idx[i])
	}
	// The rules work on a copy of the sequence's classes, laid out end to
	// end: p.types keeps what the explicit rules left, which N0 needs.
	t := p.work[:n]
	for i := range t {
		t[i] = p.types[pos(i)]
	}

	weak(t, s.sos)
	p.brackets(t, s, pos)
	neutral(t, s.sos, s.eos, direction(s.level))

	// I1 and I2: text against the direction of its level goes up one,
	// numbers in left-to-right text up two.
	for i, c := range t {
		l := s.level
		switch {
		case c == tL:
			l += l & 1
		case c == tR:
			l += 1 - l&1
		default: // tEN, tAN
			l += 2 - l&1
		}
		p.levels[pos(i)] = l
	}
}

// weak applies W1–W7 to a sequence's classes t. They settle what the weak
// types - marks, numbers and the separators and terminators around them -
// are, from the strong text before them.
func weak(t []class, sos class) {
	// W1: a nonspacing mark takes the class of what it is on, or ON after
	// an isolate initiator or PDI, which are neutral.
	prev := sos
	for i, c := range t {
		switch {
		case c == tNSM:
			t[i] = prev
		case tLRI <= c && c <= tPDI:
			prev = tON
		default:
			prev = c
		}
	}

	// W2: a European number after Arabic letters is an Arabic number - the
	// same digits read the other way round - and W3: Arabic letters are
	// then just right-to-left.
	strong := sos
	for i, c := range t {
		switch c {
		case tL, tR:
			strong = c
		case tAL:
			strong = tAL
			t[i] = tR
		case tEN:
			if strong == tAL {
				t[i] = tAN
			}
		}
	}

	// W4: one separator between two numbers of a kind joins them: "1,000",
	// "12.5", "2+2". A sign in front of a number is not between two, and is
	// left to the neutral rules.
	for i := 1; i+1 < len(t); i++ {
		switch t[i] {
		case tES:
			if t[i-1] == tEN && t[i+1] == tEN {
				t[i] = tEN
			}
		case tCS:
			if t[i-1] == tEN && t[i+1] == tEN {
				t[i] = tEN
			} else if t[i-1] == tAN && t[i+1] == tAN {
				t[i] = tAN
			}
		}
	}

	// W5: terminators next to a European number - "%", "$", "#" - go with
	// it, and W6: the separators and terminators left over are neutral.
	for i := 0; i < len(t); {
		switch t[i] {
		case tET:
			j := i + 1
			for j < len(t) && t[j] == tET {
				j++
			}
			to := tON
			if i > 0 && t[i-1] == tEN || j < len(t) && t[j] == tEN {
				to = tEN
			}
			for ; i < j; i++ {
				t[i] = to
			}
			continue
		case tES, tCS:
			t[i] = tON
		}
		i++
	}

	// W7: a European number in left-to-right text is left-to-right.
	strong = sos
	for i, c := range t {
		switch c {
		case tL, tR:
			strong = c
		case tEN:
			if strong == tL {
				t[i] = tL
			}
		}
	}
}

// strongOf is the direction a resolved class counts as for the neutral
// rules - numbers count as right-to-left - or tON for a neutral.
func strongOf(c class) class {
	switch c {
	case tL:
		return tL
	case tR, tEN, tAN:
		return tR
	}
	return tON
}

// bracketPair is the positions, in a sequence, of an opening bracket and
// the closing bracket that pairs with it.
type bracketPair struct{ open, close int }

// maxBracketDepth is how many brackets BD16 keeps open at once; it stops
// looking for pairs in a sequence that goes deeper.
const maxBracketDepth = 63

// brackets applies N0: the two brackets of a pair take one direction, that
// of what they enclose, so "(سلام)" in English text keeps its parentheses
// round the Persian and facing each other.
func (p *paragraph) brackets(t []class, s sequence, pos func(int) int) {
	// BD16: find the pairs.
	type opener struct {
		want rune // the closing bracket that pairs with it
		at   int
	}
	var stack [maxBracketDepth]opener
	var pairsBuf [8]bracketPair
	pairs := pairsBuf[:0]
	depth := 0
scan:
	for i, c := range t {
		if c != tON {
			continue
		}
		r := p.rs[pos(i)]
		pair, open, ok := bracket(r)
		if !ok {
			continue
		}
		if open {
			if depth == maxBracketDepth {
				break scan
			}
			stack[depth] = opener{canonical(pair), i}
			depth++
			continue
		}
		r = canonical(r)
		for k := depth - 1; k >= 0; k-- {
			if stack[k].want == r {
				pairs = append(pairs, bracketPair{stack[k].at, i})
				depth = k
				break
			}
		}
	}
	if len(pairs) == 0 {
		return
	}
	slices.SortFunc(pairs, func(a, b bracketPair) int { return a.open - b.open })

	e := direction(s.level)
	for _, pr := range pairs {
		// N0 b–d: the embedding direction if the brackets hold any text
		// of it; else the other direction if they hold that and it is
		// also what comes before them; else the embedding direction if
		// they hold anything strong at all; else nothing, and they are
		// left to N1 and N2.
		found := tON
		for k := pr.open + 1; k < pr.close; k++ {
			if d := strongOf(t[k]); d != tON {
				found = d
				if d == e {
					break
				}
			}
		}
		if found == tON {
			continue
		}
		if found != e {
			before := s.sos
			for k := pr.open - 1; k >= 0; k-- {
				if d := strongOf(t[k]); d != tON {
					before = d
					break
				}
			}
			if before != found {
				found = e
			}
		}
		t[pr.open], t[pr.close] = found, found
		// Marks on a bracket that changed go with it: W1 made them
		// what the bracket was, ON, before N0 made it strong.
		for _, b := range [2]int{pr.open, pr.close} {
			for k := b + 1; k < len(t) && p.types[pos(k)] == tNSM; k++ {
				t[k] = found
			}
		}
	}
}

// bracketEntry is a paired bracket: the bracket it pairs with
// (Bidi_Paired_Bracket), and whether it opens.
type bracketEntry struct {
	r, pair rune
	open    bool
}

// bracket reports whether r is a paired bracket (BidiBrackets.txt): the
// bracket it pairs with, and whether it opens.
func bracket(r rune) (pair rune, open, ok bool) {
	switch r {
	case '(':
		return ')', true, true
	case ')':
		return '(', false, true
	case '[':
		return ']', true, true
	case ']':
		return '[', false, true
	case '{':
		return '}', true, true
	case '}':
		return '{', false, true
	}
	// Past ASCII the brackets start at U+0F3A, so Latin, Arabic and Hebrew
	// text is turned away without a search.
	if r < 0x0F3A || r > brackets[len(brackets)-1].r {
		return 0, false, false
	}
	i, found := slices.BinarySearchFunc(brackets[:], r, func(b bracketEntry, r rune) int {
		return int(b.r - r)
	})
	if !found {
		return 0, false, false
	}
	return brackets[i].pair, brackets[i].open, true
}

// canonical folds the two brackets with canonical equivalents onto them, so
// that U+2329 pairs with U+3009 and U+3008 with U+232A (BD16).
func canonical(r rune) rune {
	switch r {
	case 0x2329:
		return 0x3008
	case 0x232A:
		return 0x3009
	}
	return r
}

// neutral applies N1 and N2 to a sequence's classes t, in which only strong
// text, numbers and neutrals are left: a stretch of neutrals takes the
// direction of the text on both sides of it when they agree, and the
// embedding direction e when they do not.
func neutral(t []class, sos, eos, e class) {
	for i := 0; i < len(t); {
		if strongOf(t[i]) != tON {
			i++
			continue
		}
		j := i + 1
		for j < len(t) && strongOf(t[j]) == tON {
			j++
		}
		before, after := sos, eos
		if i > 0 {
			before = strongOf(t[i-1])
		}
		if j < len(t) {
			after = strongOf(t[j])
		}
		to := e
		if before == after {
			to = before
		}
		for ; i < j; i++ {
			t[i] = to
		}
	}
}

// lineLevels applies L1 - segment separators and paragraph separators, and
// the whitespace and isolate controls before them and at the end of the
// line, go back to the paragraph level, so a tab or a trailing space is
// never caught up in the text around it - and then gives every character
// X9 removed the level of the one before it.
func (p *paragraph) lineLevels() {
	trailing := true
	for i := len(p.rs) - 1; i >= 0; i-- {
		switch c := p.initial[i]; {
		case c == tB || c == tS:
			p.levels[i] = p.base
			trailing = true
		case c == tWS || tLRI <= c && c <= tPDI:
			if trailing {
				p.levels[i] = p.base
			}
		case removed(c):
			// Not there as far as L1 is concerned.
		default:
			trailing = false
		}
	}
	prev := p.base
	for i, c := range p.initial {
		if removed(c) {
			p.levels[i] = prev
		} else {
			prev = p.levels[i]
		}
	}
}
