package fuzzy

import "slices"

// Narrows reports whether every candidate next matches is one prev matches
// too. A prompt can then rank next over only prev's matches rather than every
// candidate, as fzf does while a query is typed: nearly every keystroke adds
// to the query, and the list it filters shrinks as it goes.
//
// It is conservative. True is a promise; false says only that it could not
// tell. It is true, for the keystrokes that matter, when next is prev; when
// prev matches everything, being empty or a marker alone; when next adds to
// prev's last word, if that word is fuzzy, exact or a prefix - not a suffix,
// which is anchored where the addition goes, nor a negation, which excludes
// less as it grows; and when next adds a word to prev, unless the word joins
// prev's last group with a |.
//
// Those are cases of one rule, and the rule is what is checked: every group
// of prev's terms is implied by one of next's, taken from the parsed queries
// rather than their text. So "b\" to "b\ " is not a narrowing, though it adds
// to the word, because the escape turns the backslash into a space; and
// "a | b" to "a | bc" is, though it adds to a group, because a or bc implies
// a or b.
//
// Rank breaks its last ties by input order, so to rank next exactly as over
// every candidate, pass prev's matches in the order they were first given,
// not the order prev ranked them in.
func Narrows(prev, next string) bool {
	if prev == next {
		return true
	}
	p, n := parse(prev), parse(next)
	for _, pg := range p.groups {
		if !slices.ContainsFunc(n.groups, func(ng group) bool { return ng.implies(pg) }) {
			return false
		}
	}
	return true
}

// implies reports whether every candidate g holds for, h holds for too: when
// each of g's terms implies one of h's.
func (g group) implies(h group) bool {
	for i := range g {
		if !slices.ContainsFunc(h, func(u term) bool { return g[i].implies(&u) }) {
			return false
		}
	}
	return true
}

// implies reports whether every candidate t accepts, u accepts too.
//
// Case runs through it. A term that ignores case promises only that its text
// is there in some case, so it vouches for a term that ignores case too and
// not for one exact about it; a term exact about case vouches for either. A
// negation promises its text is absent, and there it is the other way about:
// absent in every case says more than absent in one.
func (t *term) implies(u *term) bool {
	if t.not != u.not {
		return false
	}
	if !t.not {
		if t.ignoreCase && !u.ignoreCase {
			return false
		}
		fold := u.ignoreCase
		switch u.kind {
		case termFuzzy:
			// Whatever t's kind, the candidate has t's runes in order.
			return isSubsequence(u.text, t.text, fold)
		case termExact:
			return t.kind != termFuzzy && indexRunes(t.text, 0, u.text, fold) >= 0
		case termPrefix:
			return (t.kind == termPrefix || t.kind == termEqual) && hasPrefix(t.text, u.text, fold)
		case termSuffix:
			return (t.kind == termSuffix || t.kind == termEqual) && hasSuffix(t.text, u.text, fold)
		case termEqual:
			return t.kind == termEqual && len(t.text) == len(u.text) && hasPrefix(t.text, u.text, fold)
		}
		return false
	}
	// Both negated. t says the candidate lacks t's text, where t's kind says;
	// u needs it to lack u's. That follows if having u's text would mean
	// having t's, so it is u's text that must contain t's.
	if !t.ignoreCase && u.ignoreCase {
		return false
	}
	fold := t.ignoreCase
	switch t.kind {
	case termExact:
		// Nowhere, so neither anywhere nor at an end nor as the whole.
		return indexRunes(u.text, 0, t.text, fold) >= 0
	case termPrefix:
		return (u.kind == termPrefix || u.kind == termEqual) && hasPrefix(u.text, t.text, fold)
	case termSuffix:
		return (u.kind == termSuffix || u.kind == termEqual) && hasSuffix(u.text, t.text, fold)
	case termEqual:
		return u.kind == termEqual && len(u.text) == len(t.text) && hasPrefix(u.text, t.text, fold)
	}
	return false
}

// hasPrefix reports whether s starts with pre.
func hasPrefix(s, pre []rune, ignoreCase bool) bool {
	return len(pre) <= len(s) && runesAt(s, 0, pre, ignoreCase)
}

// hasSuffix reports whether s ends with suf.
func hasSuffix(s, suf []rune, ignoreCase bool) bool {
	return len(suf) <= len(s) && runesAt(s, len(s)-len(suf), suf, ignoreCase)
}
