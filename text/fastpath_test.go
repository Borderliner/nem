package text

import (
	"math/rand/v2"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// referenceSegments is the layout the way it was computed before the ASCII
// shortcut: every rune through the grapheme segmenter.
func referenceSegments(rs []rune, tw ColIdx) ([]segment, ColIdx) {
	var segs []segment
	rest := string(rs)
	state := -1
	col, idx := ColIdx(0), RuneIdx(0)
	for len(rest) > 0 {
		var cl string
		var w int
		cl, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		n := utf8.RuneCountInString(cl)
		cw := ColIdx(w)
		if cl == "\t" {
			cw = tw - (col % tw)
		}
		segs = append(segs, segment{start: idx, n: n, col: col, w: cw})
		col += cw
		idx += RuneIdx(n)
	}
	return segs, col
}

// The shortcut is an optimisation, so it must agree with the segmenter on
// everything: ASCII next to combining marks, emoji sequences, flags, wide
// glyphs, tabs, controls and lone carriage returns, in every arrangement.
func TestFastLayoutMatchesTheSegmenter(t *testing.T) {
	pieces := []string{
		"a", "Z", " ", "~", "{", "\t", "\r", "\x01", "\x7f",
		"e\u0301", "\u0301", "\u200d", "\ufe0f",
		"👍", "👍🏽", "👨\u200d👩\u200d👧", "🇮🇷", "🇺", "🇸",
		"漢", "字", "ｱ", "é", "ß", "\u00a0", "\u0915\u094d\u0937",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 20000 {
		var rs []rune
		for range rng.IntN(12) {
			rs = append(rs, []rune(pieces[rng.IntN(len(pieces))])...)
		}
		var l Line
		l.runes = rs
		l.build()
		want, width := referenceSegments(rs, effectiveTabWidth())
		if !slices.Equal(l.segs, want) || l.width != width {
			t.Fatalf("trial %d, %q:\n got %v width %d\nwant %v width %d", trial, string(rs), l.segs, l.width, want, width)
		}
	}
}
