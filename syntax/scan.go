package syntax

import "unicode"

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isHexDigit(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// isIdentStart and isIdentPart follow Go's and Lua's shared shape: a letter or
// underscore to start, digits allowed after. unicode.IsLetter rather than an
// ASCII range because Go identifiers may be any letter, and a lexer that stops
// at the first non-ASCII rune splits an identifier in half and colours the
// pieces differently.
func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool  { return isIdentStart(r) || unicode.IsDigit(r) }

// scanIdent returns the index just past the identifier starting at i.
func scanIdent(line []rune, i int) int {
	j := i + 1
	for j < len(line) && isIdentPart(line[j]) {
		j++
	}
	return j
}

// scanQuoted returns the index just past a quoted run starting at the opening
// quote, honouring backslash escapes. An unterminated quote ends at the line's
// end rather than running on, because a half-typed string is normal and must
// not swallow the rest of the file.
func scanQuoted(line []rune, i int, quote rune) int {
	j := i + 1
	for j < len(line) {
		switch line[j] {
		case '\\':
			j += 2 // skip the escaped rune, whatever it is
		case quote:
			return j + 1
		default:
			j++
		}
	}
	return len(line)
}

// scanUntil returns the index just past a two-rune terminator, and whether it
// was found. Used for /* */ and for Markdown's fences.
func scanUntil(line []rune, from int, a, b rune) (int, bool) {
	for j := from; j+1 < len(line); j++ {
		if line[j] == a && line[j+1] == b {
			return j + 2, true
		}
	}
	return len(line), false
}

// scanRune returns the index just past the next occurrence of r, and whether it
// was found.
func scanRune(line []rune, from int, r rune) (int, bool) {
	for j := from; j < len(line); j++ {
		if line[j] == r {
			return j + 1, true
		}
	}
	return len(line), false
}

// scanRunOf consumes a run of runes satisfying pred.
func scanRunOf(line []rune, i int, pred func(rune) bool) int {
	j := i
	for j < len(line) && pred(line[j]) {
		j++
	}
	return j
}

// scanExponent consumes a floating-point exponent if one is actually there.
//
// It commits only when a digit follows, so the 'e' in "1e" or the '-' in "1-2"
// is left for the next token rather than being swallowed into the number.
func scanExponent(line []rune, j int, lower, upper rune) int {
	if j >= len(line) || (line[j] != lower && line[j] != upper) {
		return j
	}
	k := j + 1
	if k < len(line) && (line[k] == '+' || line[k] == '-') {
		k++
	}
	if k >= len(line) || !isDigit(line[k]) {
		return j
	}
	return scanRunOf(line, k, func(r rune) bool { return isDigit(r) || r == '_' })
}
