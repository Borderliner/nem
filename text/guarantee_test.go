package text

import "testing"

// guaranteeCases covers every width class Line handles: plain ASCII, a tab
// (whose width depends on the column it starts at), a wide CJK glyph, a
// decomposed accent whose second rune is zero-width, a ZWJ cluster of seven
// runes rendering as one double-width glyph, and an empty line.
var guaranteeCases = []string{
	"",
	"abc",
	"\t",
	"\tx",
	"a\tb",
	"日本語",
	"🙂",
	"🙂a",
	"é",
	"éx",
	"\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466",
	mixed,
}

// DisplayCol(Len()) == Width() is a documented guarantee on Line, not an
// accident of the implementation. The render path relies on it to size the last
// grapheme of every line: it computes a cluster's width as
// DisplayCol(next) - DisplayCol(i), and for the final cluster `next` is Len().
// If the two disagreed, the last glyph on every line would be mismeasured.
func TestDisplayColAtLenEqualsWidth(t *testing.T) {
	for _, s := range guaranteeCases {
		l := NewLine([]rune(s))
		if got, want := l.DisplayCol(l.Len()), l.Width(); got != want {
			t.Errorf("%q: DisplayCol(Len()) = %d, Width() = %d", s, got, want)
		}
	}
}

// The guarantee must survive a TabWidth change, since the cache is rebuilt
// lazily and a stale width would break it silently.
func TestDisplayColAtLenEqualsWidthAcrossTabWidths(t *testing.T) {
	defer func(old ColIdx) { TabWidth = old }(TabWidth)
	for _, tw := range []ColIdx{1, 2, 4, 8, 17} {
		TabWidth = tw
		for _, s := range guaranteeCases {
			l := NewLine([]rune(s))
			if got, want := l.DisplayCol(l.Len()), l.Width(); got != want {
				t.Errorf("TabWidth=%d %q: DisplayCol(Len()) = %d, Width() = %d", tw, s, got, want)
			}
		}
	}
}

// Indices past the end clamp to the width rather than running off the segment
// cache, which is what lets callers pass Len() without a bounds check.
func TestDisplayColBeyondLenClampsToWidth(t *testing.T) {
	for _, s := range guaranteeCases {
		l := NewLine([]rune(s))
		for _, i := range []RuneIdx{l.Len(), l.Len() + 1, l.Len() + 100} {
			if got, want := l.DisplayCol(i), l.Width(); got != want {
				t.Errorf("%q: DisplayCol(%d) = %d, Width() = %d", s, i, got, want)
			}
		}
	}
}
