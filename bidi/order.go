package bidi

import "slices"

// Order returns the visual order for items with the given levels (UAX #9
// L2): out[v] is the index of the item drawn at visual position v, left to
// right. Items are whatever the caller levelled - runes, or grapheme
// clusters each given the level of its first rune.
//
// It reverses every run of items at or above each level, from the highest
// down to the lowest odd one, so text at an odd level reads right to left
// and a number or a Latin word inside it, two levels up, reads left to
// right again.
func Order(levels []uint8) []int {
	out := make([]int, len(levels))
	for i := range out {
		out[i] = i
	}
	if len(levels) == 0 {
		return out
	}
	hi, lo := levels[0], levels[0]
	for _, l := range levels[1:] {
		hi, lo = max(hi, l), min(lo, l)
	}
	// Going down to the lowest level rounded up to odd, rather than to the
	// lowest odd level present, changes nothing: no item is at an odd level
	// below that one, so the runs at each such level are the runs at the
	// even level above it, and the two reversals cancel.
	for l := int(hi); l >= int(lo|1); l-- {
		for v := 0; v < len(out); {
			if int(levels[out[v]]) < l {
				v++
				continue
			}
			w := v + 1
			for w < len(out) && int(levels[out[w]]) >= l {
				w++
			}
			slices.Reverse(out[v:w])
			v = w
		}
	}
	return out
}

// mirrorEntry pairs a character with its Bidi_Mirroring_Glyph.
type mirrorEntry struct{ r, m rune }

// Mirror returns the mirrored glyph for r (UAX #9 L4, Unicode's
// Bidi_Mirroring_Glyph) - '(' for ')', '<' for '>', '«' for '»' and so on -
// and whether it has one. The renderer draws the mirror for a mirrored
// character at an odd (right-to-left) level. A character with no mirror
// comes back as it is.
func Mirror(r rune) (rune, bool) {
	if r < mirrors[0].r || r > mirrors[len(mirrors)-1].r {
		return r, false
	}
	i, found := slices.BinarySearchFunc(mirrors[:], r, func(e mirrorEntry, r rune) int {
		return int(e.r - r)
	})
	if !found {
		return r, false
	}
	return mirrors[i].m, true
}
