package text

import (
	"iter"
	"sort"
)

// Cluster is one grapheme cluster's placement within a line: the runes it is
// made of, where they start, and the screen columns they occupy.
//
// A cluster is the smallest unit that occupies a cell, which is why it rather
// than the rune is what a renderer walks. "é" written as e+U+0301 is two runes
// in one cell; a ZWJ family emoji is seven runes in two cells; a tab is one rune
// spanning however many columns reach the next tab stop.
type Cluster struct {
	// Runes is lent from the line, not copied. It is valid only until the line
	// is edited, and must not be mutated or retained. Copy it if you need to
	// keep it.
	Runes []rune
	// Start is the rune index where the cluster begins.
	Start RuneIdx
	// Col is the display column where the cluster begins.
	Col ColIdx
	// Width is how many display columns the cluster occupies. Zero for a
	// combining mark that merged into the preceding cluster is impossible here,
	// since such a mark is part of that cluster rather than one of its own.
	Width ColIdx
}

// Clusters iterates the line's grapheme clusters in order, lending each one's
// runes.
//
// This exists because the render path walks every visible line on every
// keystroke. Reconstructing cluster boundaries from NextGrapheme plus two
// DisplayCol lookups costs a binary search per lookup over the segment cache,
// and Runes() copies the whole line; iterating the cache directly does neither.
//
// Breaking out of the loop stops the walk, which the render path relies on to
// abandon a line as soon as a cluster passes the right edge of the window.
func (l *Line) Clusters() iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		l.build()
		l.clusters(0, 0, yield)
	}
}

// clusters yields the clusters of the segments from segs[k], starting at
// rune from within the first of them.
func (l *Line) clusters(k int, from RuneIdx, yield func(Cluster) bool) {
	for ; k < len(l.segs); k++ {
		s := l.segs[k]
		if s.one {
			lo := int(s.start)
			if !yield(Cluster{Runes: l.runes[lo : lo+int(s.n)], Start: RuneIdx(s.start), Col: ColIdx(s.col), Width: ColIdx(s.w)}) {
				return
			}
			continue
		}
		// A run: a cluster for each of its runes.
		for i := max(RuneIdx(s.start), from); i < s.end(); i++ {
			if !yield(Cluster{Runes: l.runes[i : i+1], Start: i, Col: s.colOf(i), Width: ColIdx(s.w)}) {
				return
			}
		}
	}
}

// ClustersFrom is Clusters starting from the cluster that covers display
// column col, or the first one after it - the first cluster a window scrolled
// to col can show any of.
//
// A renderer scrolled far along a long line otherwise walks every cluster to
// its left on each frame just to skip it; the segment cache knows every
// cluster's column, so a binary search finds the place instead.
func (l *Line) ClustersFrom(col ColIdx) iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		l.build()
		// The last segment starting at or before col is the one covering it,
		// and in a run, the cluster the column falls in.
		k := max(sort.Search(len(l.segs), func(k int) bool { return ColIdx(l.segs[k].col) > col })-1, 0)
		from := RuneIdx(0)
		if k < len(l.segs) {
			if s := l.segs[k]; !s.one && col > ColIdx(s.col) {
				from = RuneIdx(s.start) + RuneIdx(min(int32(col-ColIdx(s.col))/s.w, s.n-1))
			}
		}
		l.clusters(k, from, yield)
	}
}
