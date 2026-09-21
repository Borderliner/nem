package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/syntax"
	"github.com/hajianpour/nem/text"
)

// SpansFunc reports how one line is classified, for colouring.
//
// It is how the renderer asks about something it cannot work out on its own:
// lexing needs a language, a cache and the state of every line above, all of
// which live in the editor. This follows NameFunc - ui stays a pure function of
// the frame, and the caller that owns the state answers questions about it.
//
// A nil SpansFunc means no highlighting, so ui does not require the field.
type SpansFunc func(b *text.Buffer, line int) []syntax.Span

// numSyntaxClasses bounds the style table.
//
// syntax.Class is a closed set and Punctuation is its highest member. A class
// outside this range is a programming error rather than a user's problem, so
// the lookup falls back to plain text instead of panicking - a wrong colour is
// recoverable, a crash in the draw loop is not.
const numSyntaxClasses = int(syntax.Punctuation) + 1

// nem's syntax palette.
//
// Foregrounds only. The theme's central rule is that nothing paints a
// background, which is what lets the editor sit inside whatever terminal colours
// the user already runs rather than replacing them; syntax colour is the largest
// surface in the editor and so the easiest place to break that promise.
//
// Operators and punctuation are deliberately left plain. Colouring every brace
// and comma makes code read as noise, and the classes that carry meaning -
// keywords, strings, comments - stand out more when the scaffolding around them
// does not compete.
const (
	colourKeyword  = "#c678dd"
	colourString   = "#98c379"
	colourComment  = "#7f848e"
	colourNumber   = "#d19a66"
	colourFunction = "#61afef"
	colourType     = "#e5c07b"
)

// defaultSyntaxStyles returns the built-in mapping from class to style.
//
// Constants share the number colour: true, false, nil and iota are literals, and
// reading them as the same kind of thing as 42 is both conventional and one
// fewer colour for a theme author to choose.
func defaultSyntaxStyles() [numSyntaxClasses]tcell.Style {
	var s [numSyntaxClasses]tcell.Style
	for i := range s {
		s[i] = tcell.StyleDefault
	}
	fg := func(hex string) tcell.Style {
		return tcell.StyleDefault.Foreground(tcell.GetColor(hex))
	}
	s[syntax.Keyword] = fg(colourKeyword)
	s[syntax.String] = fg(colourString)
	s[syntax.Comment] = fg(colourComment)
	s[syntax.Number] = fg(colourNumber)
	s[syntax.Function] = fg(colourFunction)
	s[syntax.Type] = fg(colourType)
	s[syntax.Constant] = fg(colourNumber)
	// Plain, Operator and Punctuation keep tcell.StyleDefault.
	return s
}

// lineSyntax walks one line's spans alongside the draw loop.
//
// The draw loop visits clusters left to right and spans are ascending and
// non-overlapping, so a cursor that only moves forward answers every lookup in
// one pass rather than searching the span list per cell.
type lineSyntax struct {
	spans []syntax.Span
	th    *Theme
	i     int
}

// newLineSyntax prepares a walker, or a disabled one when there is nothing to
// colour.
func newLineSyntax(spans []syntax.Span, th *Theme) lineSyntax {
	if !th.Syntax {
		return lineSyntax{th: th}
	}
	return lineSyntax{spans: spans, th: th}
}

// styleAt returns the style for the cluster starting at rune index r, or base
// where no span covers it.
//
// r must not go backwards across calls, which the draw loop guarantees.
func (s *lineSyntax) styleAt(r text.RuneIdx, base tcell.Style) tcell.Style {
	idx := int(r)
	for s.i < len(s.spans) && s.spans[s.i].End <= idx {
		s.i++
	}
	if s.i >= len(s.spans) {
		return base
	}
	sp := s.spans[s.i]
	if idx < sp.Start {
		return base // in the gap before the next span
	}
	if int(sp.Class) >= numSyntaxClasses {
		return base
	}
	return s.th.SyntaxStyle[sp.Class]
}

// parenOver overlays the bracket highlight for the cluster at r onto style.
//
// It overlays rather than replaces so a matched bracket keeps whatever colour
// the lexer gave it and gains only weight and an underline. Replacing would make
// every matched bracket plain, which reads as the highlighter losing its place.
func parenOver(hl lineHL, r text.RuneIdx, style, base tcell.Style) tcell.Style {
	for k := 0; k < hl.n; k++ {
		if hl.idx[k] != r {
			continue
		}
		if style == base {
			// Nothing underneath to preserve, so use the bracket style whole.
			// overlay merges through Decompose, which reports attribute bits but
			// not an underline's style - curly, dotted - nor a hyperlink, so a
			// needless round-trip would quietly drop them.
			return hl.style[k]
		}
		return overlay(style, hl.style[k])
	}
	return style
}
