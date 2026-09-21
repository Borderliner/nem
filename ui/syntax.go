package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/syntax"
	"github.com/hajianpour/nem/text"
	"os"
	"strings"
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
	// Dark palette. Measured against a near-black ground these run 4.4:1 to
	// 9.7:1, comfortably readable. Against white they run 1.7:1 to 3.8:1 and
	// are not: this is a dark-terminal palette and must not be used on a light
	// one, which is why there are two.
	darkKeyword  = "#c678dd"
	darkString   = "#98c379"
	darkComment  = "#7f848e"
	darkNumber   = "#d19a66"
	darkFunction = "#61afef"
	darkType     = "#e5c07b"

	// Light palette, measured 4.55:1 to 7.59:1 against white - every value
	// clears WCAG AA for body text. Comments are the tightest at 4.55, which is
	// deliberate: a comment should sit behind the code without becoming
	// unreadable.
	lightKeyword  = "#8250df"
	lightString   = "#116329"
	lightComment  = "#6e7781"
	lightNumber   = "#953800"
	lightFunction = "#0550ae"
	lightType     = "#7d4e00"
)

// defaultSyntaxStyles returns the built-in mapping from class to style.
//
// Constants share the number colour: true, false, nil and iota are literals, and
// reading them as the same kind of thing as 42 is both conventional and one
// fewer colour for a theme author to choose.
func defaultSyntaxStyles() [numSyntaxClasses]tcell.Style {
	return syntaxStyles(TerminalIsLight())
}

// syntaxStyles builds the class-to-style table for one palette.
//
// A single palette cannot serve both grounds. Colours with enough contrast on
// black are washed out on white and vice versa, so nem carries two and picks
// one rather than shipping a compromise that reads poorly on both.
func syntaxStyles(light bool) [numSyntaxClasses]tcell.Style {
	var s [numSyntaxClasses]tcell.Style
	for i := range s {
		s[i] = tcell.StyleDefault
	}
	fg := func(hex string) tcell.Style {
		return tcell.StyleDefault.Foreground(tcell.GetColor(hex))
	}

	kw, str, cmt := darkKeyword, darkString, darkComment
	num, fn, typ := darkNumber, darkFunction, darkType
	if light {
		kw, str, cmt = lightKeyword, lightString, lightComment
		num, fn, typ = lightNumber, lightFunction, lightType
	}

	s[syntax.Keyword] = fg(kw)
	s[syntax.String] = fg(str)
	s[syntax.Comment] = fg(cmt)
	s[syntax.Number] = fg(num)
	s[syntax.Function] = fg(fn)
	s[syntax.Type] = fg(typ)
	s[syntax.Constant] = fg(num)
	// Plain, Operator and Punctuation keep tcell.StyleDefault.
	return s
}

// UseSyntaxPalette switches the syntax colours between the two palettes.
func (t *Theme) UseSyntaxPalette(light bool) {
	t.SyntaxStyle = syntaxStyles(light)
}

// TerminalIsLight guesses whether the terminal has a light background.
//
// COLORFGBG is set by several terminals as "fg;bg" (sometimes "fg;default;bg")
// with the colour numbers of each. Background 7 or 15 is white or bright white;
// everything else is treated as dark. It is a heuristic - many terminals do not
// set it at all - so the answer is only a default, overridable with the theme
// setting. Guessing dark is the safer miss: most terminals are dark, and the
// dark palette on a light ground is faint rather than invisible.
func TerminalIsLight() bool {
	v := os.Getenv("COLORFGBG")
	if v == "" {
		return false
	}
	parts := strings.Split(v, ";")
	bg := parts[len(parts)-1]
	return bg == "7" || bg == "15"
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
