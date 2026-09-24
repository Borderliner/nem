package command

import (
	"slices"

	"github.com/Borderliner/nem/text"
)

// Automatic pairs: typing an opening bracket or a quote inserts its partner
// too, with point between; typing the partner steps over it; Backspace between
// an empty pair takes both. With a selection, the pair goes around it (see the
// editor's delete-selection).
//
// The rule for when to pair is emacs's electric-pair-mode, conservatively: only
// where nothing would be pushed out of place - before a blank, a line end, a
// closing bracket or punctuation - so typing ( in front of a word inserts just
// the (. A quote is not paired straight after a word character, which is what
// keeps the apostrophe in "don't" from growing a twin.

// AutoPair turns automatic pairs on. It is set from the auto-pair setting.
var AutoPair = true

// PairFor returns the character that closes r, when r opens a pair.
func PairFor(r rune) (rune, bool) {
	switch r {
	case '(':
		return ')', true
	case '[':
		return ']', true
	case '{':
		return '}', true
	case '"', '\'', '`':
		return r, true
	}
	return 0, false
}

func isPairQuote(r rune) bool { return r == '"' || r == '\'' || r == '`' }

// editingText reports whether the current buffer is text being edited: one of
// the editor's, not the minibuffer - a search for "(" must search for "(",
// not "()" - and not read-only, where there is nothing to pair.
func editingText(e Env) bool {
	return !e.Buf().ReadOnly() && slices.Contains(e.Buffers(), e.Buf())
}

// pairsAllowedBefore reports whether r, the character after point, is one a
// pair may be inserted in front of.
func pairsAllowedBefore(r rune) bool {
	switch {
	case r == '\n' || isBlank(r) || isCloseBracket(r):
		return true
	case r == ',' || r == ';' || r == ':' || r == '.':
		return true
	}
	return false
}

// autoPairInsert handles a typed r when automatic pairs apply, reporting
// whether it did - stepping over a closer, or inserting a pair.
func autoPairInsert(e Env, r rune) (bool, error) {
	if !AutoPair || !editingText(e) {
		return false, nil
	}
	b, p := e.Buf(), e.Win().Pt
	next := runeAt(b, p)

	// Typing the character already after point steps over it, so a closer
	// that was inserted automatically is not doubled when typed by hand.
	if (isCloseBracket(r) || isPairQuote(r)) && next == r {
		edSetPoint(e, nextRune(b, p))
		return true, nil
	}

	closer, opens := PairFor(r)
	if !opens || !pairsAllowedBefore(next) {
		return false, nil
	}
	if isPairQuote(r) && p.Col > 0 && isSymbolRune(b.Line(p.Line).At(p.Col-1)) {
		return false, nil
	}
	if err := b.Insert(p, []rune{r, closer}); err != nil {
		return true, err
	}
	edSetPoint(e, text.Pos{Line: p.Line, Col: p.Col + 1})
	return true, nil
}

// autoPairDelete handles Backspace between an empty pair, deleting both
// halves, and reports whether it did.
func autoPairDelete(e Env) (bool, error) {
	if !AutoPair || !editingText(e) {
		return false, nil
	}
	b, p := e.Buf(), e.Win().Pt
	if p.Col == 0 {
		return false, nil
	}
	rs := b.Line(p.Line).View()
	if int(p.Col) >= len(rs) {
		return false, nil
	}
	if closer, ok := PairFor(rs[p.Col-1]); !ok || rs[p.Col] != closer {
		return false, nil
	}
	from := text.Pos{Line: p.Line, Col: p.Col - 1}
	if err := b.Delete(from, text.Pos{Line: p.Line, Col: p.Col + 1}); err != nil {
		return true, err
	}
	edSetPoint(e, from)
	return true, nil
}
