package syntax

import "strings"

// jsonLexer classifies JSON.
//
// It carries no state: a JSON string may not legally contain a raw newline, so
// an unterminated quote is a typo rather than a construct continuing onto the
// next line, and ending it at the line break keeps one mistake from recolouring
// the rest of the file.
type jsonLexer struct{}

func (jsonLexer) Name() string { return "json" }

var jsonConstants = map[string]bool{"true": true, "false": true, "null": true}

func (l jsonLexer) Lex(line []rune, _ State) ([]Span, State) {
	var b builder
	for i := 0; i < len(line); {
		r := line[i]
		switch {
		case r == '"':
			end := scanQuoted(line, i, '"')
			// A string followed by a colon is a key. Keys get Function because
			// that is this set's class for a name in naming position, and
			// telling keys from values is most of what makes JSON readable.
			b.add(i, end, jsonStringClass(line, end))
			i = end

		case isDigit(r) || (r == '-' && i+1 < len(line) && isDigit(line[i+1])):
			j := i
			if line[j] == '-' {
				j++
			}
			j = scanRunOf(line, j, isDigit)
			if j < len(line) && line[j] == '.' {
				j = scanRunOf(line, j+1, isDigit)
			}
			j = scanExponent(line, j, 'e', 'E')
			b.add(i, j, Number)
			i = j

		case isIdentStart(r):
			end := scanIdent(line, i)
			if jsonConstants[string(line[i:end])] {
				b.add(i, end, Constant)
			}
			i = end

		case strings.ContainsRune("{}[],:", r):
			b.add(i, i+1, Punctuation)
			i++

		default:
			i++
		}
	}
	return b.out, 0
}

// jsonStringClass distinguishes a key from a value by looking past the closing
// quote for a colon.
func jsonStringClass(line []rune, end int) Class {
	for j := end; j < len(line); j++ {
		switch line[j] {
		case ' ', '\t':
			continue
		case ':':
			return Function
		}
		break
	}
	return String
}
