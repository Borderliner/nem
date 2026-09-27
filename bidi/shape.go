package bidi

import (
	"slices"
	"unicode"
)

// joining is a character's joining type (ArabicShaping.txt): which of its
// neighbours an Arabic-script letter reaches out to. Right and left are the
// visual sides in right-to-left text, so a right-joining letter joins the
// one before it in logical order.
type joining uint8

const (
	joinU joining = iota // non-joining: hamza, ZWNJ, spaces, digits, Latin
	joinD                // dual-joining: joins on both sides, as beh and seen do
	joinR                // right-joining: only to the letter before, as alef and reh do
	joinL                // left-joining: only to the letter after
	joinC                // join-causing: tatweel and ZWJ, which both sides join
	joinT                // transparent: harakat and other marks, passed over
)

// joiningRange gives the characters lo to hi one joining type.
type joiningRange struct {
	lo, hi rune
	t      joining
}

// arabicJoining holds the joining types of the Arabic blocks, U+0600 to
// U+08FF, where every letter Shape looks at and nearly all its neighbours
// are, so they skip the search.
var arabicJoining [0x0900 - 0x0600]joining

func init() {
	for i := range arabicJoining {
		arabicJoining[i] = lookupJoining(0x0600 + rune(i))
	}
}

// joiningOf is r's joining type.
func joiningOf(r rune) joining {
	switch {
	case r < 0x80:
		return joinU
	case 0x0600 <= r && r < 0x0900:
		return arabicJoining[r-0x0600]
	}
	return lookupJoining(r)
}

// lookupJoining finds r's joining type in the tables. ArabicShaping.txt
// lists the letters; of the characters it leaves out, marks and format
// characters are transparent and everything else is non-joining.
func lookupJoining(r rune) joining {
	i, found := slices.BinarySearchFunc(joiningTypes[:], r, func(e joiningRange, r rune) int {
		switch {
		case e.hi < r:
			return -1
		case e.lo > r:
			return 1
		}
		return 0
	})
	if found {
		return joiningTypes[i].t
	}
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return joinT
	}
	return joinU
}

// The presentation forms of a letter, as forms holds them.
const (
	isolated = iota
	final
	initial
	medial
)

// Shape returns rs with each Arabic-script letter replaced by its contextual
// presentation form - isolated, initial, medial or final - from the Arabic
// Presentation Forms-A and -B blocks, one rune for one rune so every index is
// unchanged. rs is in logical order. Letters with no presentation form, and
// everything that is not Arabic script, are returned unchanged.
//
// A letter joins the one before it when that one reaches forward - it is
// dual-joining, left-joining or a tatweel or ZWJ - and joins the one after
// it when it reaches forward itself and that one reaches back. Harakat and
// other marks between them are passed over and left as they are; a ZWNJ, as
// in "می‌خواهم", keeps the letters either side of it apart. Lam and alef
// stay two letters: their ligature is one character, and would take one
// cell for two runes.
//
// The result is a new slice; rs is not changed.
func Shape(rs []rune) []rune {
	out := slices.Clone(rs)
	if out == nil {
		out = []rune{}
	}
	// i is the character being shaped and cur its joining type; prev and
	// next are those of the nearest characters either side of it that are
	// not transparent.
	prev := joinU
	i, cur := nextJoining(rs, 0)
	for i < len(rs) {
		j, next := nextJoining(rs, i+1)
		if r := rs[i]; firstShaped <= r && r <= lastShaped {
			before := prev == joinD || prev == joinL || prev == joinC
			after := next == joinD || next == joinR || next == joinC
			form := isolated
			switch cur {
			case joinD:
				switch {
				case before && after:
					form = medial
				case before:
					form = final
				case after:
					form = initial
				}
			case joinR:
				if before {
					form = final
				}
			case joinL:
				if after {
					form = initial
				}
			}
			if f := forms[r-firstShaped][form]; f != 0 {
				out[i] = rune(f)
			}
		}
		prev, i, cur = cur, j, next
	}
	return out
}

// nextJoining finds the first character of rs from i on that is not
// transparent, and its joining type: len(rs) and joinU when there is none,
// the end of the text joining nothing.
func nextJoining(rs []rune, i int) (int, joining) {
	for ; i < len(rs); i++ {
		if t := joiningOf(rs[i]); t != joinT {
			return i, t
		}
	}
	return len(rs), joinU
}
