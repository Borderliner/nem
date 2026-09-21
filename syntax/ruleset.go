package syntax

// State layout for a RuleSet, within the fixed one State documents:
//
//	mode 0            nothing open
//	mode 1, param i   inside multiline rule i
//
// The rule index is stable for a given RuleSet, so two lines genuinely in the
// same situation compare equal - which is what the highlight cache relies on to
// decide it can stop re-lexing downward.
const ruleSetInRegion = 1

// unclaimed marks a rune no rule has painted.
//
// It is deliberately not Plain: a rule may legitimately paint Plain over an
// earlier rule, and "nothing claimed this" has to stay distinguishable from
// "a rule chose Plain" while painting.
const unclaimed = Class(0xFF)

// Lex classifies one line against the set's rules.
//
// Rules apply in file order and each paints over whatever came before. Painting
// into a per-rune array rather than collecting spans is what makes that exact:
// matches overlap constantly - a keyword inside a string, a number inside a
// comment - and resolving overlaps afterwards would need the same array anyway.
// The later rule wins because a .nemrc lists broad rules first and narrow ones
// after, which is how a string containing the word `if` stays a string.
func (rs *RuleSet) Lex(line []rune, in State) ([]Span, State) {
	n := len(line)
	if n == 0 {
		// An empty line cannot close an open region, so whatever was open
		// stays open.
		return nil, in
	}

	paint := make([]Class, n)
	for i := range paint {
		paint[i] = unclaimed
	}

	src := string(line)

	// Regexes report byte offsets; spans are rune indices. They coincide for an
	// ASCII line, which is nearly every line of source, so the mapping table is
	// built only when the line actually holds a multi-byte rune.
	var b2r []int
	if len(src) != n {
		b2r = make([]int, len(src)+1)
		ri := 0
		for bi := range src {
			b2r[bi] = ri
			ri++
		}
		b2r[len(src)] = n
	}
	runeAt := func(byteOff int) int {
		if b2r == nil {
			if byteOff > n {
				return n
			}
			return byteOff
		}
		if byteOff >= len(b2r) {
			return n
		}
		return b2r[byteOff]
	}

	fill := func(from, to int, c Class) {
		if from < 0 {
			from = 0
		}
		if to > n {
			to = n
		}
		for i := from; i < to; i++ {
			paint[i] = c
		}
	}

	scanFrom := 0 // byte offset after any region carried in from above was closed

	// A region open at the end of the previous line claims the start of this
	// one, up to its terminator or the whole line.
	if idx, open := openRegion(in); open && idx < len(rs.rules) {
		r := rs.rules[idx]
		if r.multiline() {
			if m := r.endRe.FindStringIndex(src); m != nil {
				fill(0, runeAt(m[1]), r.class)
				scanFrom = m[1]
			} else {
				fill(0, n, r.class)
				return spansFrom(paint, n), in
			}
		}
	}

	openIdx, openAt := -1, -1
	for i, r := range rs.rules {
		if !r.multiline() {
			for _, m := range r.re.FindAllStringIndex(src[scanFrom:], -1) {
				fill(runeAt(scanFrom+m[0]), runeAt(scanFrom+m[1]), r.class)
			}
			continue
		}
		// Walk start/end pairs. A start with no end opens the next line.
		pos := scanFrom
		for pos <= len(src) {
			sm := r.startRe.FindStringIndex(src[pos:])
			if sm == nil {
				break
			}
			s0, s1 := pos+sm[0], pos+sm[1]
			em := r.endRe.FindStringIndex(src[s1:])
			if em == nil {
				fill(runeAt(s0), n, r.class)
				// The leftmost unterminated opener is the one still open at end
				// of line; anything opening further right is inside it.
				if openAt == -1 || s0 < openAt {
					openIdx, openAt = i, s0
				}
				break
			}
			fill(runeAt(s0), runeAt(s1+em[1]), r.class)
			pos = s1 + em[1]
			if em[1] == 0 {
				pos++ // a zero-width end would spin
			}
		}
	}

	out := State(0)
	if openIdx >= 0 {
		out = mkState(ruleSetInRegion, uint16(openIdx))
	}
	return spansFrom(paint, n), out
}

func openRegion(s State) (idx int, open bool) {
	if s.mode() != ruleSetInRegion {
		return 0, false
	}
	return int(s.param()), true
}

// spansFrom coalesces the painted array into the ascending, non-overlapping,
// non-empty spans the Lexer interface promises.
func spansFrom(paint []Class, n int) []Span {
	var b builder
	i := 0
	for i < n {
		c := paint[i]
		j := i + 1
		for j < n && paint[j] == c {
			j++
		}
		if c != unclaimed {
			b.add(i, j, c)
		}
		i = j
	}
	return b.out
}
