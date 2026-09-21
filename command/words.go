package command

import (
	"unicode"

	"github.com/Borderliner/nem/text"
)

// Word scanning lives here, in one place, because forward-word and kill-word
// disagreeing about where a word ends produces inconsistencies users notice and
// nobody can reproduce. Motion and editing commands both call these.

// isWordRune reports whether r is a word constituent.
//
// Letters and digits, as in emacs. Combining marks count too, and that is not
// incidental: "é" written as e+U+0301 has a base letter followed by U+0301,
// which is a nonspacing mark (category Mn) and therefore NOT a letter. Without
// Mn and Mc here, word motion would break in the middle of a decomposed
// accented character and stop mid-grapheme.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		unicode.In(r, unicode.Mn, unicode.Mc)
}

// forwardWordPos returns the position at the end of the next word at or after
// from: it skips any non-word runes, then consumes the word. At end of buffer it
// returns the buffer end rather than erroring, so commands can call it freely.
func forwardWordPos(b *text.Buffer, from text.Pos) text.Pos {
	p := b.ClampPos(from)
	end := b.End()

	// Skip non-word runes, crossing line boundaries.
	for p.Before(end) && !isWordRune(runeAt(b, p)) {
		p = nextRune(b, p)
	}
	// Consume the word.
	for p.Before(end) && isWordRune(runeAt(b, p)) {
		p = nextRune(b, p)
	}
	return p
}

// backwardWordPos returns the position at the start of the word at or before
// from, mirroring forwardWordPos. At the buffer start it returns the origin.
func backwardWordPos(b *text.Buffer, from text.Pos) text.Pos {
	p := b.ClampPos(from)
	origin := text.Pos{}

	for origin.Before(p) {
		prev := prevRune(b, p)
		if isWordRune(runeAt(b, prev)) {
			break
		}
		p = prev
	}
	for origin.Before(p) {
		prev := prevRune(b, p)
		if !isWordRune(runeAt(b, prev)) {
			break
		}
		p = prev
	}
	return p
}

// runeAt returns the rune at p, or '\n' at a line end. Treating the line break
// as a newline rune is what lets the scanners cross lines without special
// cases.
func runeAt(b *text.Buffer, p text.Pos) rune {
	if p.Line < 0 || p.Line >= b.NumLines() {
		return '\n'
	}
	l := b.Line(p.Line)
	if p.Col >= l.Len() {
		return '\n'
	}
	return l.Runes()[p.Col]
}

func nextRune(b *text.Buffer, p text.Pos) text.Pos {
	if p.Line >= b.NumLines() {
		return p
	}
	if p.Col < b.Line(p.Line).Len() {
		return text.Pos{Line: p.Line, Col: p.Col + 1}
	}
	if p.Line+1 < b.NumLines() {
		return text.Pos{Line: p.Line + 1, Col: 0}
	}
	return p
}

func prevRune(b *text.Buffer, p text.Pos) text.Pos {
	if p.Col > 0 {
		return text.Pos{Line: p.Line, Col: p.Col - 1}
	}
	if p.Line > 0 {
		return text.Pos{Line: p.Line - 1, Col: b.Line(p.Line - 1).Len()}
	}
	return p
}
