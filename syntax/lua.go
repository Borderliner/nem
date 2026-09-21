package syntax

import "strings"

// luaLexer classifies Lua source.
//
// Lua's long brackets are the reason this lexer carries a parameter as well as a
// mode. [[ ]], [=[ ]=] and [==[ ]==] are all distinct delimiters, and a closer
// only closes a bracket of its own level - so ]] inside a [==[ ... ]==] string
// is ordinary text. The level therefore has to survive the line break alongside
// the mode, which is what State's param field is for.
type luaLexer struct{}

func (luaLexer) Name() string { return "lua" }

const (
	luaNormal uint8 = iota
	luaInLongComment
	luaInLongString
)

var luaKeywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "for": true, "function": true, "goto": true, "if": true,
	"in": true, "local": true, "not": true, "or": true, "repeat": true,
	"return": true, "then": true, "until": true, "while": true,
}

var luaConstants = map[string]bool{"true": true, "false": true, "nil": true}

func (l luaLexer) Lex(line []rune, in State) ([]Span, State) {
	var b builder
	i := 0

	// Close whatever the line above left open, at its own level.
	switch in.mode() {
	case luaInLongComment, luaInLongString:
		cls := Comment
		if in.mode() == luaInLongString {
			cls = String
		}
		end, closed := luaLongClose(line, 0, int(in.param()))
		b.add(0, end, cls)
		if !closed {
			return b.out, in // unchanged: same construct, same level
		}
		i = end
	}

	for i < len(line) {
		r := line[i]
		if r == ' ' || r == '\t' {
			i++
			continue
		}

		switch {
		case r == '-' && i+1 < len(line) && line[i+1] == '-':
			// A comment, but --[[ opens a long comment that may span lines.
			if level, after, ok := luaLongOpen(line, i+2); ok {
				end, closed := luaLongClose(line, after, level)
				b.add(i, end, Comment)
				if !closed {
					return b.out, mkState(luaInLongComment, uint16(level))
				}
				i = end
				continue
			}
			b.add(i, len(line), Comment)
			return b.out, 0

		case r == '[':
			if level, after, ok := luaLongOpen(line, i); ok {
				end, closed := luaLongClose(line, after, level)
				b.add(i, end, String)
				if !closed {
					return b.out, mkState(luaInLongString, uint16(level))
				}
				i = end
				continue
			}
			b.add(i, i+1, Punctuation)
			i++

		case r == '"' || r == '\'':
			end := scanQuoted(line, i, r)
			b.add(i, end, String)
			i = end

		case isDigit(r) || (r == '.' && i+1 < len(line) && isDigit(line[i+1])):
			end := scanLuaNumber(line, i)
			b.add(i, end, Number)
			i = end

		case isIdentStart(r):
			end := scanIdent(line, i)
			word := string(line[i:end])
			switch {
			case luaKeywords[word]:
				b.add(i, end, Keyword)
			case luaConstants[word]:
				b.add(i, end, Constant)
			case end < len(line) && line[end] == '(':
				b.add(i, end, Function)
			default:
				b.add(i, end, Plain)
			}
			i = end

		case strings.ContainsRune("+-*/%^#<>=~", r):
			end := scanRunOf(line, i, func(r rune) bool { return strings.ContainsRune("+-*/%^#<>=~", r) })
			b.add(i, end, Operator)
			i = end

		case strings.ContainsRune("(){}];:,.", r):
			b.add(i, i+1, Punctuation)
			i++

		default:
			i++
		}
	}
	return b.out, 0
}

// luaLongOpen reports whether a long bracket opens at i, its level, and where
// its contents begin. [[ is level 0, [=[ level 1, [==[ level 2.
func luaLongOpen(line []rune, i int) (level, after int, ok bool) {
	if i >= len(line) || line[i] != '[' {
		return 0, 0, false
	}
	j := scanRunOf(line, i+1, func(r rune) bool { return r == '=' })
	if j < len(line) && line[j] == '[' {
		return j - i - 1, j + 1, true
	}
	return 0, 0, false
}

// luaLongClose finds the closer matching level, which is the whole point: a ]]
// does not end a [==[ string.
func luaLongClose(line []rune, from, level int) (end int, ok bool) {
	for j := from; j < len(line); j++ {
		if line[j] != ']' {
			continue
		}
		k := scanRunOf(line, j+1, func(r rune) bool { return r == '=' })
		if k < len(line) && line[k] == ']' && k-j-1 == level {
			return k + 1, true
		}
	}
	return len(line), false
}

func scanLuaNumber(line []rune, i int) int {
	j := i
	if line[j] == '.' {
		return scanExponent(line, scanRunOf(line, j+1, isDigit), 'e', 'E')
	}
	if line[j] == '0' && j+1 < len(line) && (line[j+1] == 'x' || line[j+1] == 'X') {
		j = scanRunOf(line, j+2, func(r rune) bool { return isHexDigit(r) || r == '.' })
		return scanExponent(line, j, 'p', 'P')
	}
	j = scanRunOf(line, j, isDigit)
	if j < len(line) && line[j] == '.' {
		j = scanRunOf(line, j+1, isDigit)
	}
	return scanExponent(line, j, 'e', 'E')
}
