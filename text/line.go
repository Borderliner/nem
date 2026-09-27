package text

import (
	"sort"
	"unicode/utf8"

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

	// keep is how much of a stale cache an edit left good: the runes before
	// it are as they were when it was built, so their clusters still are.
	// Zero when all of it must go.
	keep RuneIdx
	// plain is how many runes at the start are ASCII: all of them, for most
	// lines of code, which is what lets a caller skip work only text in
	// other scripts needs. Known once the cache is built.
	plain int
}

// NewLine returns a Line holding a copy of rs.
func NewLine(rs []rune) Line {
	cp := make([]rune, len(rs))
	copy(cp, rs)
	return Line{runes: cp}
}

// build recomputes the segment cache if it is stale.
//
// Plain ASCII takes a shortcut past the grapheme segmenter, which costs tens
// of nanoseconds a rune and was most of what a keystroke cost on a long line.
// Between two ASCII runes there is always a cluster boundary - no ASCII
// character extends, prepends or joins, and the one exception, CR LF, cannot
// occur inside a line - so a printable ASCII rune followed by another ASCII
// rune is a cluster of its own, one column wide. Everything else goes through
// uniseg a run at a time, each run ending at such a boundary, so segmenting
// can restart there exactly as it would have continued.
//
// After an edit only the clusters from just before it are measured again: a
// keystroke at the end of a 200KB line re-measured all of it, which was most
// of what the keystroke cost.
func (l *Line) build() {
	tw := effectiveTabWidth()
	if l.valid && l.tabW == tw {
		return
	}

	rs := l.runes
	n := len(rs)
	i, col := 0, ColIdx(0)
	if k := l.reusable(tw); k > 0 {
		last := l.segs[k-1]
		l.segs = l.segs[:k]
		i, col = int(last.start)+last.n, last.col+last.w
	} else if cap(l.segs) >= n {
		l.segs = l.segs[:0]
	} else {
		l.segs = make([]segment, 0, n)
	}
	if l.plain >= i {
		l.plain = i
		for l.plain < n && rs[l.plain] < 0x80 {
			l.plain++
		}
	}

	for i < n {
		r := rs[i]
		switch {
		case r >= 0x20 && r < 0x7f && (i+1 == n || rs[i+1] < 0x80):
			l.segs = append(l.segs, segment{start: RuneIdx(i), n: 1, col: col, w: 1})
			col++
			i++
			continue
		case r == '\t':
			// A control is always a cluster of its own, and tab stops are
			// ours to apply: uniseg reports a tab as zero-width.
			w := tw - (col % tw)
			l.segs = append(l.segs, segment{start: RuneIdx(i), n: 1, col: col, w: w})
			col += w
			i++
			continue
		}
		// The run up to the next boundary the fast path can vouch for.
		j := i + 1
		for j < n && (rs[j-1] >= 0x80 || rs[j] >= 0x80) {
			j++
		}
		col = l.segmentRun(i, j, col, tw)
		i = j
	}
	l.width = col
	l.valid = true
	l.tabW = tw
	l.keep = 0
}

// reusable is how many of the cache's segments an edit left good: those
// wholly before l.keep, less any at the end that the text after them could
// now join onto. The prefix is kept only up to a boundary the fast path can
// vouch for - two ASCII runes, or the end of the line - since a combining
// mark or a joiner typed after a cluster changes it.
func (l *Line) reusable(tw ColIdx) int {
	if l.keep <= 0 || l.tabW != tw {
		return 0
	}
	rs := l.runes
	k := sort.Search(len(l.segs), func(j int) bool {
		return int(l.segs[j].start)+l.segs[j].n > int(l.keep)
	})
	for ; k > 0; k-- {
		p := int(l.segs[k-1].start) + l.segs[k-1].n
		if p > len(rs) {
			continue
		}
		if p == len(rs) || (rs[p-1] < 0x80 && rs[p] < 0x80) {
			break
		}
	}
	return k
}

// segmentRun appends the clusters of runes [from, to) starting at display
// column col, and returns the column after them.
func (l *Line) segmentRun(from, to int, col, tw ColIdx) ColIdx {
	rest := string(l.runes[from:to])
	state := -1
	idx := RuneIdx(from)
	for len(rest) > 0 {
		var cl string
		var w int
		cl, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		n := utf8.RuneCountInString(cl)
		cw := ColIdx(w)
		if cl == "\t" {
			cw = tw - (col % tw)
		}
		l.segs = append(l.segs, segment{start: idx, n: n, col: col, w: cw})
		col += cw
		idx += RuneIdx(n)
	}
	return col
}

// setRunes replaces the line's content and invalidates the cache.
func (l *Line) setRunes(rs []rune) {
	l.runes = rs
	l.valid, l.keep = false, 0
}

// edited replaces the line's content after an edit at rune index at, which
// leaves the cache good for the runes before it.
func (l *Line) edited(rs []rune, at RuneIdx) {
	l.runes = rs
	if l.valid {
		l.keep = at
	} else {
		l.keep = min(l.keep, at)
	}
	l.valid = false
}

// ASCII reports whether every rune of the line is ASCII: nothing to lay out
// right to left, nothing wide. It is learnt while the line is measured, so
// asking costs nothing for a line on screen.
func (l *Line) ASCII() bool {
	l.build()
	return l.plain == len(l.runes)
}

// Len returns the number of runes in the line.
func (l *Line) Len() RuneIdx { return RuneIdx(len(l.runes)) }

// Runes returns a copy of the line's runes.
func (l *Line) Runes() []rune {
	cp := make([]rune, len(l.runes))
	copy(cp, l.runes)
	return cp
}

// At returns the rune at index i, which must be in range. Reading one rune
// through Runes copied the whole line, which made stepping along a line by
// runes cost the square of its length.
func (l *Line) At(i RuneIdx) rune { return l.runes[i] }

// View lends the line's runes without copying them, for code that only reads
// - a search scanning every line of a file, say. Like Cluster.Runes it is
// valid only until the line is edited, and must not be modified or kept.
func (l *Line) View() []rune { return l.runes }

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
