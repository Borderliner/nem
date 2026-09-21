package syntax

import "strings"

// goLexer classifies Go source.
//
// Two constructs survive a line break - a /* */ comment and a raw string in
// backticks - so those are the only modes it carries.
type goLexer struct{}

func (goLexer) Name() string { return "go" }

const (
	goNormal uint8 = iota
	goInBlockComment
	goInRawString
)

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

var goConstants = map[string]bool{
	"true": true, "false": true, "nil": true, "iota": true,
}

// goTypes are the predeclared type names. They are classified as Type even in
// call position, so a conversion like string(b) reads as a type rather than a
// function - which is what it is.
var goTypes = map[string]bool{
	"bool": true, "byte": true, "complex64": true, "complex128": true, "error": true,
	"float32": true, "float64": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "rune": true, "string": true, "uint": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"any": true, "comparable": true,
}

func (l goLexer) Lex(line []rune, in State) ([]Span, State) {
	var b builder
	i := 0

	// Finish whatever the line above left open before reading anything new.
	switch in.mode() {
	case goInBlockComment:
		end, closed := scanUntil(line, 0, '*', '/')
		b.add(0, end, Comment)
		if !closed {
			return b.out, mkState(goInBlockComment, 0)
		}
		i = end
	case goInRawString:
		end, closed := scanRune(line, 0, '`')
		b.add(0, end, String)
		if !closed {
			return b.out, mkState(goInRawString, 0)
		}
		i = end
	}

	// pendingType marks that the previous word was `type`, so the next
	// identifier names a type. This is the one place the grammar makes a type
	// name unambiguous without parsing.
	pendingType := false

	for i < len(line) {
		r := line[i]

		// Whitespace does not interrupt `type` and its name.
		if r == ' ' || r == '\t' {
			i++
			continue
		}

		switch {
		case r == '/' && i+1 < len(line) && line[i+1] == '/':
			b.add(i, len(line), Comment)
			return b.out, 0

		case r == '/' && i+1 < len(line) && line[i+1] == '*':
			end, closed := scanUntil(line, i+2, '*', '/')
			b.add(i, end, Comment)
			if !closed {
				return b.out, mkState(goInBlockComment, 0)
			}
			i = end

		case r == '`':
			end, closed := scanRune(line, i+1, '`')
			b.add(i, end, String)
			if !closed {
				return b.out, mkState(goInRawString, 0)
			}
			i = end

		case r == '"' || r == '\'':
			end := scanQuoted(line, i, r)
			b.add(i, end, String)
			i = end

		case isDigit(r) || (r == '.' && i+1 < len(line) && isDigit(line[i+1])):
			end := scanGoNumber(line, i)
			b.add(i, end, Number)
			i = end

		case isIdentStart(r):
			end := scanIdent(line, i)
			word := string(line[i:end])
			b.add(i, end, goClassify(word, line, end, pendingType))
			pendingType = word == "type"
			i = end
			continue // the assignment above is the only place this is set

		case strings.ContainsRune("+-*/%&|^<>=!~:", r):
			end := scanRunOf(line, i, func(r rune) bool {
				return strings.ContainsRune("+-*/%&|^<>=!~:", r)
			})
			b.add(i, end, Operator)
			i = end

		case strings.ContainsRune("()[]{},;.", r):
			b.add(i, i+1, Punctuation)
			i++

		default:
			i++ // anything else is Plain
		}
		pendingType = false
	}
	return b.out, 0
}

// goClassify decides what an identifier is.
//
// Order matters: keywords first, so `if (` is not read as a call; then
// predeclared types, so `string(b)` is a conversion rather than a function; and
// only then the call-position rule.
func goClassify(word string, line []rune, end int, pendingType bool) Class {
	switch {
	case pendingType:
		return Type
	case goKeywords[word]:
		return Keyword
	case goConstants[word]:
		return Constant
	case goTypes[word]:
		return Type
	case end < len(line) && line[end] == '(':
		// An identifier immediately followed by an open paren. This covers a
		// call, a declaration (`func main(`) and a method name after its
		// receiver (`func (e *Editor) Redraw(`) with one rule and no lookbehind.
		return Function
	}
	return Plain
}

// scanGoNumber consumes one numeric literal, covering the forms Go actually
// allows: hex, binary and octal prefixes, underscores as separators, a hex
// float's p-exponent, and a trailing i for an imaginary literal.
func scanGoNumber(line []rune, i int) int {
	digits := func(r rune) bool { return isDigit(r) || r == '_' }

	j := i
	if line[j] == '.' {
		j = scanRunOf(line, j+1, digits)
		return goImaginary(line, scanExponent(line, j, 'e', 'E'))
	}

	if line[j] == '0' && j+1 < len(line) {
		switch line[j+1] {
		case 'x', 'X':
			j = scanRunOf(line, j+2, func(r rune) bool { return isHexDigit(r) || r == '_' || r == '.' })
			return goImaginary(line, scanExponent(line, j, 'p', 'P'))
		case 'b', 'B', 'o', 'O':
			return goImaginary(line, scanRunOf(line, j+2, digits))
		}
	}

	j = scanRunOf(line, j, digits)
	if j < len(line) && line[j] == '.' {
		j = scanRunOf(line, j+1, digits)
	}
	return goImaginary(line, scanExponent(line, j, 'e', 'E'))
}

func goImaginary(line []rune, j int) int {
	if j < len(line) && line[j] == 'i' {
		return j + 1
	}
	return j
}
