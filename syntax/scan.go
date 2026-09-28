package syntax

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isHexDigit(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
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
	for k < len(line) && (isDigit(line[k]) || line[k] == '_') {
		k++
	}
	return k
}
