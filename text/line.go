package text

import (
	"sort"

	"github.com/rivo/uniseg"
)

// TabWidth is the number of columns between tab stops. Changing it
// invalidates every line's cached layout lazily, on next use.
var TabWidth ColIdx = 8

func effectiveTabWidth() ColIdx {
	if TabWidth < 1 {
		return 1
	}
	return TabWidth
}

// segment is one grapheme cluster's placement within a line.
type segment struct {
	start RuneIdx // rune index where the cluster begins
	n     int     // runes in the cluster
	col   ColIdx  // display column where the cluster begins
	w     ColIdx  // display width of the cluster
}

// Line is a single line of text plus a cached grapheme/column layout.
// The cache is built lazily and invalidated on edit.
//
// Guaranteed: DisplayCol(Len()) == Width(). Renderers rely on it to size the
// last grapheme of a line, computing a cluster's width as the difference
// between its own start column and the next boundary's - and for the final
// cluster that next boundary is Len(). Anything changing DisplayCol must keep
// this true; text/guarantee_test.go pins it across every width class.
type Line struct {
	runes []rune
	segs  []segment
	width ColIdx
	valid bool
	tabW  ColIdx // TabWidth in effect when the cache was built
}

// NewLine returns a Line holding a copy of rs.
func NewLine(rs []rune) Line {
	cp := make([]rune, len(rs))
	copy(cp, rs)
	return Line{runes: cp}
}

// build recomputes the segment cache if it is stale.
func (l *Line) build() {
	tw := effectiveTabWidth()
	if l.valid && l.tabW == tw {
		return
	}
	if cap(l.segs) >= len(l.runes) {
		l.segs = l.segs[:0]
	} else {
		l.segs = make([]segment, 0, len(l.runes))
	}

	rest := string(l.runes)
	state := -1
	col := ColIdx(0)
	idx := RuneIdx(0)
	for len(rest) > 0 {
		var cl string
		var w int
		cl, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		n := len([]rune(cl))
		cw := ColIdx(w)
		// uniseg reports tabs as zero-width; tab stops are ours to apply.
		if cl == "\t" {
			cw = tw - (col % tw)
		}
		l.segs = append(l.segs, segment{start: idx, n: n, col: col, w: cw})
		col += cw
		idx += RuneIdx(n)
	}
	l.width = col
	l.valid = true
	l.tabW = tw
}

// setRunes replaces the line's content and invalidates the cache.
func (l *Line) setRunes(rs []rune) {
	l.runes = rs
	l.valid = false
}

// Len returns the number of runes in the line.
func (l *Line) Len() RuneIdx { return RuneIdx(len(l.runes)) }

// Runes returns a copy of the line's runes.
func (l *Line) Runes() []rune {
	cp := make([]rune, len(l.runes))
	copy(cp, l.runes)
	return cp
}

// String returns the line's text.
func (l *Line) String() string { return string(l.runes) }

// Width returns the line's total display width in columns.
func (l *Line) Width() ColIdx {
	l.build()
	return l.width
}

// DisplayCol returns the screen column at which the grapheme containing rune
// index i begins. Out-of-range indices clamp to the ends of the line, so
// DisplayCol(Len()) is the line's Width - a guarantee callers depend on to
// measure the final cluster without a special case.
func (l *Line) DisplayCol(i RuneIdx) ColIdx {
	l.build()
	if i <= 0 {
		return 0
	}
	if i >= l.Len() {
		return l.width
	}
	k := sort.Search(len(l.segs), func(k int) bool { return l.segs[k].start > i }) - 1
	if k < 0 {
		return 0
	}
	return l.segs[k].col
}

// RuneAt returns the rune index of the grapheme occupying display column c.
// A column landing inside a wide glyph clamps to that glyph's first rune, so
// the cursor never sits in the right half of a CJK character or an emoji.
func (l *Line) RuneAt(c ColIdx) RuneIdx {
	l.build()
	if c <= 0 {
		return 0
	}
	if c >= l.width {
		return l.Len()
	}
	k := sort.Search(len(l.segs), func(k int) bool { return l.segs[k].col > c }) - 1
	if k < 0 {
		return 0
	}
	return l.segs[k].start
}

// NextGrapheme returns the next grapheme boundary strictly after i, clamped to
// the end of the line.
func (l *Line) NextGrapheme(i RuneIdx) RuneIdx {
	l.build()
	if i < 0 {
		i = 0
	}
	if i >= l.Len() {
		return l.Len()
	}
	k := sort.Search(len(l.segs), func(k int) bool { return l.segs[k].start > i })
	if k >= len(l.segs) {
		return l.Len()
	}
	return l.segs[k].start
}

// PrevGrapheme returns the previous grapheme boundary strictly before i,
// clamped to the start of the line.
func (l *Line) PrevGrapheme(i RuneIdx) RuneIdx {
	l.build()
	if i <= 0 {
		return 0
	}
	if i > l.Len() {
		i = l.Len()
	}
	k := sort.Search(len(l.segs), func(k int) bool { return l.segs[k].start >= i }) - 1
	if k < 0 {
		return 0
	}
	return l.segs[k].start
}
