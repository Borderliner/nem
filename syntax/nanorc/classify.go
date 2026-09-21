// Package nanorc reads GNU nano's syntax-highlighting definitions so nem can
// colour the languages it has no hand-written lexer for.
//
// nano ships around forty .nanorc files describing C, Python, Rust, shell, YAML
// and more. They are read from the system at runtime, exactly as nano reads
// them. Nothing from them is copied into this repository: they are GPL, and
// vendoring them would put nem's own licensing at issue. A machine without nano
// installed simply gets no extra languages.
//
// # Why the colours are thrown away
//
// A .nanorc rule names a colour - "color brightcyan" - while nem's renderer
// maps a semantic syntax.Class to a theme style. That indirection is what lets
// nem carry a light and a dark palette; taking nano's colour literally would
// hard-code a dark-terminal choice into every rule.
//
// The obvious fix is a colour-to-class table, and it does not work. Measuring
// the corpus, nano's colours carry no shared meaning across files: strings are
// red in java.nanorc and green in rust.nanorc, and every common colour is used
// for keywords in one file and something else in the next. They are per-file
// aesthetic choices.
//
// So the class is inferred from what a rule's pattern MATCHES rather than from
// the colour it was given. A pattern listing bare words between word boundaries
// is keywords; one wrapping a character class in quotes is strings; one running
// a comment introducer to end-of-line is comments. That is language-agnostic and
// does not depend on a convention that turns out not to exist.
package nanorc

import (
	"regexp"
	"strings"

	"github.com/Borderliner/nem/syntax"
)

// Patterns that identify what a rule is for. Each is deliberately narrow: a
// wrong guess miscolours a whole language, while no guess falls back to Keyword,
// which is what the majority of .nanorc rules highlight anyway.
var (
	// A string rule. Two shapes cover the corpus: a negated class holding a
	// quote - [^"] or [^'] - which is how a delimited literal is written, and a
	// pattern that both opens and closes on a quote.
	reStringClass = regexp.MustCompile(`\[\^[^\]]*["']`)
	reStringDelim = regexp.MustCompile(`^\\?["'].*\\?["']$`)

	// A comment introducer somewhere in the pattern. Whether it reaches the end
	// of the line is checked separately: nano writes both "#.*$" and the
	// unanchored "(^|[[:blank:]])#.*", where the .* already runs to the end.
	reCommentIntro = regexp.MustCompile(`#|//|--|;|%|!|dnl`)

	// Three or more bare words between word boundaries: a keyword list.
	reKeywordList = regexp.MustCompile(`\\[<b]\(?[A-Za-z_][A-Za-z0-9_]*(\|[A-Za-z_][A-Za-z0-9_]*){2,}`)

	// Digit-driven: a numeric literal rule.
	reNumber = regexp.MustCompile(`\[\[:digit:\]\]|\[0-9\]|\[\[:xdigit:\]\]`)

	// An identifier immediately before an open paren: a call or definition.
	reFunction = regexp.MustCompile(`[A-Za-z_\]\)\+\*]\s*\\\(|^\\?(def|function|sub|fn|proc)\b`)

	// A leading capital or a type-ish suffix list. Weak on purpose.
	reType = regexp.MustCompile(`\\[<b]\(?[A-Z][A-Za-z0-9_]*(\|[A-Z][A-Za-z0-9_]*)+`)
)

// classify infers what a rule colours from its pattern.
//
// Order matters: a comment rule may contain quotes, and a string rule may
// contain a comment character, so the most distinctive shapes are tested first.
func classify(pattern string) syntax.Class {
	switch {
	case isString(pattern):
		return syntax.String
	case looksLikeComment(pattern):
		return syntax.Comment
	case reKeywordList.MatchString(pattern):
		return syntax.Keyword
	case reType.MatchString(pattern):
		return syntax.Type
	case reNumber.MatchString(pattern):
		return syntax.Number
	case reFunction.MatchString(pattern):
		return syntax.Function
	}
	// Keyword is the fallback because it is what most .nanorc rules highlight -
	// builtins, attributes, tags, directives - and because an uncoloured rule
	// is a visible regression where a slightly-wrong colour is not.
	return syntax.Keyword
}

// classifyMultiline infers the class of a start/end pair.
//
// These are nearly always block comments or multi-line strings, and the opener
// says which: /*, <!--, {- and =begin introduce comments, while a quote or a
// heredoc marker introduces a string.
func classifyMultiline(start string) syntax.Class {
	switch {
	case strings.Contains(start, `/\*`), strings.Contains(start, `<!--`),
		strings.Contains(start, `\{-`), strings.Contains(start, `=begin`),
		strings.Contains(start, `"""`), strings.Contains(start, `'''`):
		if strings.Contains(start, `"""`) || strings.Contains(start, `'''`) {
			return syntax.String
		}
		return syntax.Comment
	case isString(start):
		return syntax.String
	}
	return classify(start)
}

// looksLikeComment reports whether a pattern runs a comment introducer to the
// end of the line.
//
// The trailing .* or $ is what separates a comment rule from a keyword list
// that merely happens to contain one of these characters, and the word-boundary
// check rejects the rest: a rule built from \<(...)\> is a word list whatever
// punctuation it holds.
func looksLikeComment(pattern string) bool {
	if strings.Contains(pattern, `\<`) || strings.Contains(pattern, `\b`) {
		return false
	}
	if !reCommentIntro.MatchString(pattern) {
		return false
	}
	return strings.HasSuffix(pattern, ".*") ||
		strings.HasSuffix(pattern, ".*$") ||
		strings.HasSuffix(pattern, "$")
}

// isString reports whether a pattern looks like a string-literal rule.
func isString(pattern string) bool {
	return reStringClass.MatchString(pattern) || reStringDelim.MatchString(pattern)
}
