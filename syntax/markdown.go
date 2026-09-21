package syntax

// markdownLexer classifies Markdown.
//
// Mapping prose onto a class set designed for code is approximate by nature, so
// the mapping is chosen for readability rather than fidelity: literal text
// (code spans and fenced blocks) is String, structure (headings) is Keyword,
// and quoted matter is Comment because it reads as an aside. The one thing that
// genuinely must be right is the fenced block, because it spans lines - a fence
// whose close is missed recolours the remainder of the document.
type markdownLexer struct{}

func (markdownLexer) Name() string { return "markdown" }

const mdInFence uint8 = 1

// mdFenceEncode packs a fence's length and delimiter into State's param, so a
// ``` block is not closed by a ~~~ line, and a longer fence is not closed by a
// shorter one.
func mdFenceEncode(length int, tilde bool) uint16 {
	p := uint16(length & 0xFFF)
	if tilde {
		p |= 1 << 12
	}
	return p
}

func mdFenceDecode(p uint16) (length int, tilde bool) {
	return int(p & 0xFFF), p&(1<<12) != 0
}

func (l markdownLexer) Lex(line []rune, in State) ([]Span, State) {
	var b builder

	if in.mode() == mdInFence {
		length, tilde := mdFenceDecode(in.param())
		b.add(0, len(line), String)
		if mdIsFenceClose(line, length, tilde) {
			return b.out, 0
		}
		return b.out, in
	}

	if length, tilde, ok := mdFenceOpen(line); ok {
		b.add(0, len(line), String)
		return b.out, mkState(mdInFence, mdFenceEncode(length, tilde))
	}

	if mdIsHeading(line) {
		b.add(0, len(line), Keyword)
		return b.out, 0
	}

	i := scanRunOf(line, 0, func(r rune) bool { return r == ' ' })
	if i < len(line) && line[i] == '>' {
		b.add(0, len(line), Comment)
		return b.out, 0
	}

	// A list marker is consumed before inline scanning so that the * in "* item"
	// is read as a bullet rather than as the opening of emphasis.
	if end, ok := mdListMarker(line, i); ok {
		b.add(i, end, Punctuation)
		i = end
	}

	mdInline(&b, line, i)
	return b.out, 0
}

func mdInline(b *builder, line []rune, from int) {
	for i := from; i < len(line); {
		switch line[i] {
		case '`':
			n := scanRunOf(line, i, func(r rune) bool { return r == '`' }) - i
			if end, ok := mdCloseTicks(line, i+n, n); ok {
				b.add(i, end, String)
				i = end
				continue
			}
			b.add(i, i+n, String)
			i += n
		case '[':
			if textEnd, urlEnd, ok := mdLink(line, i); ok {
				b.add(i, textEnd, Function)
				b.add(textEnd, urlEnd, String)
				i = urlEnd
				continue
			}
			i++
		case '*', '_':
			if end, ok := mdEmphasis(line, i); ok {
				b.add(i, end, Constant)
				i = end
				continue
			}
			i++
		default:
			i++
		}
	}
}

func mdIsHeading(line []rune) bool {
	i := scanRunOf(line, 0, func(r rune) bool { return r == ' ' })
	if i > 3 {
		return false
	}
	h := scanRunOf(line, i, func(r rune) bool { return r == '#' })
	n := h - i
	if n < 1 || n > 6 {
		return false
	}
	return h >= len(line) || line[h] == ' '
}

func mdFenceOpen(line []rune) (length int, tilde, ok bool) {
	i := scanRunOf(line, 0, func(r rune) bool { return r == ' ' })
	if i > 3 || i >= len(line) {
		return 0, false, false
	}
	c := line[i]
	if c != '`' && c != '~' {
		return 0, false, false
	}
	end := scanRunOf(line, i, func(r rune) bool { return r == c })
	if end-i < 3 {
		return 0, false, false
	}
	return end - i, c == '~', true
}

// mdIsFenceClose requires a line of nothing but the delimiter, at least as long
// as the opener. A line of code that merely starts with backticks does not
// close the block.
func mdIsFenceClose(line []rune, length int, tilde bool) bool {
	c := '`'
	if tilde {
		c = '~'
	}
	i := scanRunOf(line, 0, func(r rune) bool { return r == ' ' })
	end := scanRunOf(line, i, func(r rune) bool { return r == c })
	if end-i < length {
		return false
	}
	return scanRunOf(line, end, func(r rune) bool { return r == ' ' || r == '\t' }) == len(line)
}

func mdListMarker(line []rune, i int) (end int, ok bool) {
	if i >= len(line) {
		return 0, false
	}
	if r := line[i]; r == '-' || r == '*' || r == '+' {
		if i+1 < len(line) && line[i+1] == ' ' {
			return i + 1, true
		}
		return 0, false
	}
	j := scanRunOf(line, i, isDigit)
	if j > i && j < len(line) && (line[j] == '.' || line[j] == ')') {
		if j+1 < len(line) && line[j+1] == ' ' {
			return j + 1, true
		}
	}
	return 0, false
}

func mdCloseTicks(line []rune, from, n int) (int, bool) {
	for j := from; j < len(line); j++ {
		if line[j] != '`' {
			continue
		}
		k := scanRunOf(line, j, func(r rune) bool { return r == '`' })
		if k-j == n {
			return k, true
		}
		j = k - 1
	}
	return 0, false
}

func mdLink(line []rune, i int) (textEnd, urlEnd int, ok bool) {
	closeBracket, found := scanRune(line, i+1, ']')
	if !found || closeBracket >= len(line) || line[closeBracket] != '(' {
		return 0, 0, false
	}
	end, found := scanRune(line, closeBracket+1, ')')
	if !found {
		return 0, 0, false
	}
	return closeBracket, end, true
}

// mdEmphasis pairs a run of * or _ with a later run of the same rune. The
// content must be non-empty and must not begin or end with a space, which is
// what keeps "a * b * c" from being read as emphasis.
func mdEmphasis(line []rune, i int) (int, bool) {
	d := line[i]
	n := scanRunOf(line, i, func(r rune) bool { return r == d }) - i
	if n > 3 || i+n >= len(line) || line[i+n] == ' ' {
		return 0, false
	}
	for j := i + n + 1; j < len(line); j++ {
		if line[j] != d {
			continue
		}
		k := scanRunOf(line, j, func(r rune) bool { return r == d })
		if k-j >= n && line[j-1] != ' ' {
			return j + n, true
		}
		j = k - 1
	}
	return 0, false
}
