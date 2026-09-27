package editorconfig

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A section's name is a glob, matched against the path of a file relative to
// the directory holding the .editorconfig. The specification's syntax:
//
//	*          any run of characters but /
//	**         any run of characters at all
//	?          any one character but /
//	[abc]      one of the characters listed; a-z ranges work
//	[!abc]     one character that is none of them, nor /
//	{a,b,c}    any of the comma-separated globs, which may nest
//	{1..30}    a whole number in that range
//	\x         x itself, whatever it would otherwise mean
//
// A glob with no / in it matches the file's name in any directory below the
// .editorconfig, so [*.go] reaches every Go file in the tree; one with a /
// is anchored where the .editorconfig is, so [src/*.go] reaches only those
// directly in src.
//
// Globs are translated into regular expressions rather than matched by hand.
// The syntax nests, through braces, and a translation that recurses where the
// glob does is shorter and easier to trust than a matcher that backtracks.

// glob is a compiled section name.
type glob struct {
	re *regexp.Regexp
	// ranges are the bounds of the {n..m} in the glob, in order. Each is a
	// capture group in re - the only ones it has - checked after matching,
	// since a regular expression cannot compare numbers.
	ranges [][2]int
}

// compileGlob compiles a section name, or returns nil when it cannot be
// compiled - a character range written backwards, say. A nil glob matches
// nothing, so the section is passed over rather than the file.
func compileGlob(pat string) *glob {
	switch {
	case !strings.Contains(pat, "/"):
		pat = "**/" + pat
	case !strings.HasPrefix(pat, "/"):
		pat = "/" + pat
	}
	var t translator
	src := "^" + t.translate([]rune(pat)) + "$"
	re, err := regexp.Compile(src)
	if err != nil {
		return nil
	}
	return &glob{re: re, ranges: t.ranges}
}

// match reports whether g matches rel, a path relative to the .editorconfig's
// directory with a leading slash: "/main.go", "/src/main.go".
func (g *glob) match(rel string) bool {
	if g == nil {
		return false
	}
	m := g.re.FindStringSubmatchIndex(rel)
	if m == nil {
		return false
	}
	for i, r := range g.ranges {
		from, to := m[2*i+2], m[2*i+3]
		if from < 0 {
			continue // in an alternative that did not match
		}
		n, err := strconv.Atoi(rel[from:to])
		if err != nil || n < r[0] || n > r[1] {
			return false
		}
	}
	return true
}

// translator turns glob syntax into a regular expression, collecting the
// numeric ranges it meets on the way.
type translator struct {
	ranges [][2]int
}

// numericRange is the inside of a {n..m}.
var numericRange = regexp.MustCompile(`^([+-]?[0-9]+)\.\.([+-]?[0-9]+)$`)

func (t *translator) translate(pat []rune) string {
	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; c {
		case '\\':
			if i+1 < len(pat) {
				i++
				b.WriteString(regexp.QuoteMeta(string(pat[i])))
			} else {
				b.WriteString(`\\`)
			}
		case '*':
			if i+1 < len(pat) && pat[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '/':
			// a/**/b reaches a/b as well as a/x/b: the ** may stand for no
			// directories at all, and then it takes one of the slashes with
			// it.
			if hasPrefix(pat[i+1:], "**/") {
				b.WriteString("(?:/|/.*/)")
				i += 3
			} else {
				b.WriteByte('/')
			}
		case '[':
			if class, n := bracket(pat[i:]); n > 0 {
				b.WriteString(class)
				i += n - 1
			} else {
				b.WriteString(`\[`)
			}
		case '{':
			end := closingBrace(pat, i)
			if end < 0 {
				b.WriteString(`\{`)
				break
			}
			inner := pat[i+1 : end]
			alts := splitAlternatives(inner)
			switch {
			case len(alts) > 1:
				b.WriteString("(?:")
				for j, a := range alts {
					if j > 0 {
						b.WriteByte('|')
					}
					b.WriteString(t.translate(a))
				}
				b.WriteByte(')')
			case numericRange.MatchString(string(inner)):
				m := numericRange.FindStringSubmatch(string(inner))
				lo, _ := strconv.Atoi(m[1])
				hi, _ := strconv.Atoi(m[2])
				t.ranges = append(t.ranges, [2]int{min(lo, hi), max(lo, hi)})
				b.WriteString(`([+-]?[0-9]+)`)
			default:
				// Braces around a single glob are not a choice between one
				// thing; the specification has them match themselves.
				b.WriteString(`\{` + t.translate(inner) + `\}`)
			}
			i = end
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

func hasPrefix(rs []rune, s string) bool {
	return strings.HasPrefix(string(rs), s)
}

// bracket translates the [...] at the start of pat, returning the character
// class and how many runes of pat it took; 0 means the [ is just a [ - it is
// never closed, or it holds a /, which a single character of a name cannot
// be.
func bracket(pat []rune) (string, int) {
	i := 1
	negate := i < len(pat) && (pat[i] == '!' || pat[i] == '^')
	if negate {
		i++
	}
	start := i
	if i < len(pat) && pat[i] == ']' {
		i++ // a ] straight after the [ is one of the characters
	}
	for i < len(pat) && pat[i] != ']' {
		switch pat[i] {
		case '/':
			return "", 0
		case '\\':
			i++
		}
		i++
	}
	if i >= len(pat) {
		return "", 0
	}
	body := pat[start:i]

	var b strings.Builder
	b.WriteByte('[')
	if negate {
		b.WriteString(`^/`)
	}
	for j := 0; j < len(body); j++ {
		lo := body[j]
		if lo == '\\' && j+1 < len(body) {
			j++
			lo = body[j]
		}
		hi := lo
		if j+2 < len(body) && body[j+1] == '-' {
			j += 2
			hi = body[j]
			if hi == '\\' && j+1 < len(body) {
				j++
				hi = body[j]
			}
		}
		// Every character spelled by its code point, so nothing in the
		// class can be mistaken for class syntax.
		fmt.Fprintf(&b, `\x{%x}`, lo)
		if hi != lo {
			fmt.Fprintf(&b, `-\x{%x}`, hi)
		}
	}
	b.WriteByte(']')
	return b.String(), i + 1
}

// closingBrace is the index of the } that closes the { at pat[open], or -1
// when there is none.
func closingBrace(pat []rune, open int) int {
	depth := 0
	for i := open; i < len(pat); i++ {
		switch pat[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitAlternatives splits the inside of a {...} at the commas that are its
// own, not those of a brace group nested inside it or escaped.
func splitAlternatives(pat []rune) [][]rune {
	var out [][]rune
	depth, from := 0, 0
	for i := 0; i < len(pat); i++ {
		switch pat[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, pat[from:i])
				from = i + 1
			}
		}
	}
	return append(out, pat[from:])
}
