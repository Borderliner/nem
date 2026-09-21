package nanorc

import "github.com/Borderliner/nem/syntax"

// Lexer applies one Syntax's rules to a line at a time.
//
// It satisfies syntax.Lexer, so a nanorc-described language plugs into the same
// incremental highlight cache the hand-written lexers use.
type Lexer struct{ s *Syntax }

// Name reports the language, as the .nanorc named it.
func (l Lexer) Name() string { return l.s.Name }

// Comment reports the line-comment introducer the .nanorc declared, or "".
func (l Lexer) Comment() string { return l.s.Comment }

// State layout, within the fixed one syntax.State documents:
//
//	mode 0            nothing open
//	mode 1, param i   inside multiline rule i
//
// The rule index is stable for a given Syntax, so two lines that are genuinely
// in the same situation compare equal - which is what the highlight cache uses
// to decide it can stop re-lexing.
const modeInMultiline = 1

func openState(idx int) syntax.State {
	return syntax.State(uint32(modeInMultiline) | uint32(uint16(idx))<<8)
}

func openRule(s syntax.State) (idx int, open bool) {
	if uint8(s&0xFF) != modeInMultiline {
		return 0, false
	}
	return int(uint16((s >> 8) & 0xFFFF)), true
}

// unset marks a rune no rule has claimed. It is deliberately not syntax.Plain:
// a rule may legitimately paint Plain over an earlier rule, and the two cases
// have to stay distinguishable while painting.
const unset = syntax.Class(0xFF)

// Lex classifies one line.
//
// Rules are applied in the order the .nanorc lists them and each paints over
// whatever came before, which is nano's own precedence. Painting into a
// per-rune array rather than collecting spans is what makes that exact: regex
// matches in these files overlap constantly, and resolving overlaps after the
// fact would need the same array anyway.
func (l Lexer) Lex(line []rune, in syntax.State) ([]syntax.Span, syntax.State) {
	n := len(line)
	if n == 0 {
		// An empty line cannot close a multiline rule, so whatever was open
		// stays open.
		return nil, in
	}

	paint := make([]syntax.Class, n)
	for i := range paint {
		paint[i] = unset
	}

	src := string(line)
	// Regexes work in byte offsets; spans are rune indices. They coincide for
	// an ASCII line, which is the overwhelming majority, so the mapping table
	// is built only when the line actually holds a multi-byte rune.
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
			return byteOff
		}
		if byteOff >= len(b2r) {
			return n
		}
		return b2r[byteOff]
	}

	fill := func(from, to int, c syntax.Class) {
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

	out := syntax.State(0)
	scanFrom := 0 // byte offset after any carried-in region was closed

	// A multiline rule carried in from the line above claims the start of this
	// line, up to its terminator or the whole line.
	if idx, open := openRule(in); open && idx < len(l.s.rules) {
		r := l.s.rules[idx]
		if r.multiline() {
			if m := r.endRe.FindStringIndex(src); m != nil {
				fill(0, runeAt(m[1]), r.class)
				scanFrom = m[1]
			} else {
				fill(0, n, r.class)
				return spansOf(paint, n), in
			}
		}
	}

	openIdx, openAt := -1, -1
	for i, r := range l.s.rules {
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
				// The leftmost unterminated opener is the one still open at
				// end of line; a later rule opening further right is inside it.
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
	if openIdx >= 0 {
		out = openState(openIdx)
	}
	return spansOf(paint, n), out
}

// spansOf coalesces the painted array into the ascending, non-overlapping,
// non-empty spans syntax.Lexer promises.
func spansOf(paint []syntax.Class, n int) []syntax.Span {
	var out []syntax.Span
	i := 0
	for i < n {
		c := paint[i]
		j := i + 1
		for j < n && paint[j] == c {
			j++
		}
		if c != unset {
			out = append(out, syntax.Span{Start: i, End: j, Class: c})
		}
		i = j
	}
	return out
}
