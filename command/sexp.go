package command

import (
	"errors"

	"github.com/Borderliner/nem/text"
)

// Balanced expressions - emacs's sexps: a bracketed group, a string, or a
// word. C-M-f and C-M-b move over one; C-M-k kills one.
//
// Emacs reads these from each language's syntax table. nem has no syntax
// tables, so this uses the rules that hold across the languages people edit:
// (), [] and {} nest, "..." and `...` are strings with backslash escapes, and a
// word is letters, digits and underscores. A single quote is left alone, being
// an apostrophe in prose and comments as often as a string delimiter. Brackets
// inside strings are skipped, so f("(") is one expression, not an unbalanced
// one.

var (
	// errSexpEnd is moving forward from inside a group with nothing left in
	// it, or backward from its start: the words are emacs's.
	errSexpEnd = errors.New("containing expression ends prematurely")
	// errSexpUnbalanced is a group or string with no end in the buffer.
	errSexpUnbalanced = errors.New("unbalanced parentheses")
)

// RegisterSexp adds the balanced-expression commands to r.
func RegisterSexp(r *Registry) error {
	cmds := []Command{
		{Name: "forward-sexp", Doc: "Move over the next balanced expression, ARG times.", Fn: forwardSexp},
		{Name: "backward-sexp", Doc: "Move back over the previous balanced expression, ARG times.", Fn: backwardSexp},
		{Name: "kill-sexp", Doc: "Kill the balanced expression after point.", Fn: killSexp},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

func isSymbolRune(r rune) bool { return isWordRune(r) || r == '_' }
func isStringQuote(r rune) bool { return r == '"' || r == '`' }

// isSexpFiller is what separates expressions: blanks, line ends, and
// punctuation that is neither a bracket nor a quote.
func isSexpFiller(r rune) bool {
	return !isSymbolRune(r) && !isOpenBracket(r) && !isCloseBracket(r) && !isStringQuote(r)
}

func forwardSexp(e Env) error {
	n, _ := e.Arg()
	return moveSexps(e, n)
}

func backwardSexp(e Env) error {
	n, _ := e.Arg()
	return moveSexps(e, -n)
}

func moveSexps(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	var err error
	for ; n > 0 && err == nil; n-- {
		p, err = sexpForward(b, p)
	}
	for ; n < 0 && err == nil; n++ {
		p, err = sexpBackward(b, p)
	}
	edSetPoint(e, p)
	return err
}

func killSexp(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	end, err := sexpForward(b, p)
	if err != nil {
		return err
	}
	killed := string(b.Text(p, end))
	if err := b.Delete(p, end); err != nil {
		return err
	}
	e.KillForward(killed)
	edSetPoint(e, p)
	return nil
}

// sexpForward returns the end of the expression after p.
func sexpForward(b *text.Buffer, p text.Pos) (text.Pos, error) {
	orig, end := p, b.End()
	for p.Before(end) && isSexpFiller(runeAt(b, p)) {
		p = nextRune(b, p)
	}
	if !p.Before(end) {
		return p, nil
	}
	switch r := runeAt(b, p); {
	case isCloseBracket(r):
		return orig, errSexpEnd
	case isStringQuote(r):
		if s, err := stringEnd(b, p); err == nil {
			return s, nil
		}
		return orig, errSexpUnbalanced
	case isOpenBracket(r):
		depth := 0
		for q := p; q.Before(end); q = nextRune(b, q) {
			switch c := runeAt(b, q); {
			case isStringQuote(c):
				s, err := stringEnd(b, q)
				if err != nil {
					return orig, err
				}
				q = prevRune(b, s) // the loop's step lands just past the string
			case isOpenBracket(c):
				depth++
			case isCloseBracket(c):
				if depth--; depth == 0 {
					return nextRune(b, q), nil
				}
			}
		}
		return orig, errSexpUnbalanced
	default:
		for p.Before(end) && isSymbolRune(runeAt(b, p)) {
			p = nextRune(b, p)
		}
		return p, nil
	}
}

// stringEnd returns the position just past the string opening at p: after the
// next quote of the same kind that no backslash escapes.
func stringEnd(b *text.Buffer, p text.Pos) (text.Pos, error) {
	quote, end := runeAt(b, p), b.End()
	for q := nextRune(b, p); q.Before(end); q = nextRune(b, q) {
		switch runeAt(b, q) {
		case '\\':
			q = nextRune(b, q)
		case quote:
			return nextRune(b, q), nil
		}
	}
	return p, errSexpUnbalanced
}

// sexpBackward returns the start of the expression before p.
func sexpBackward(b *text.Buffer, p text.Pos) (text.Pos, error) {
	orig, start := p, text.Pos{}
	for start.Before(p) && isSexpFiller(runeAt(b, prevRune(b, p))) {
		p = prevRune(b, p)
	}
	if !start.Before(p) {
		return p, nil
	}
	switch r := runeAt(b, prevRune(b, p)); {
	case isOpenBracket(r):
		return orig, errSexpEnd
	case isStringQuote(r):
		if s, err := stringStart(b, prevRune(b, p)); err == nil {
			return s, nil
		}
		return orig, errSexpUnbalanced
	case isCloseBracket(r):
		depth := 0
		for q := prevRune(b, p); ; q = prevRune(b, q) {
			switch c := runeAt(b, q); {
			case isStringQuote(c) && !escaped(b, q):
				s, err := stringStart(b, q)
				if err != nil {
					return orig, err
				}
				q = s
			case isCloseBracket(c):
				depth++
			case isOpenBracket(c):
				if depth--; depth == 0 {
					return q, nil
				}
			}
			if !start.Before(q) {
				return orig, errSexpUnbalanced
			}
		}
	default:
		for start.Before(p) && isSymbolRune(runeAt(b, prevRune(b, p))) {
			p = prevRune(b, p)
		}
		return p, nil
	}
}

// stringStart returns the opening quote of the string whose closing quote is
// at p.
func stringStart(b *text.Buffer, p text.Pos) (text.Pos, error) {
	quote, start := runeAt(b, p), text.Pos{}
	for q := p; start.Before(q); {
		q = prevRune(b, q)
		if runeAt(b, q) == quote && !escaped(b, q) {
			return q, nil
		}
	}
	return p, errSexpUnbalanced
}

// escaped reports whether the rune at p is preceded by an odd number of
// backslashes, and so stands for itself rather than ending a string.
func escaped(b *text.Buffer, p text.Pos) bool {
	n := 0
	for q := p; q.Col > 0; {
		q = prevRune(b, q)
		if runeAt(b, q) != '\\' {
			break
		}
		n++
	}
	return n%2 == 1
}
