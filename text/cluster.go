package text

import "iter"

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
		for _, s := range l.segs {
			lo := int(s.start)
			if !yield(Cluster{
				Runes: l.runes[lo : lo+s.n],
				Start: s.start,
				Col:   s.col,
				Width: s.w,
			}) {
				return
			}
		}
	}
}
