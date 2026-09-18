// Package text implements nem's buffer model: lines, positions, the two
// mutation primitives every edit composes from, and the undo log.
//
// It has no dependency on a terminal or on any other nem package, so all of
// it is testable with plain table-driven tests.
//
// Three coordinate spaces meet in this package and must never be conflated:
//
//   - RuneIdx  an index into a line's []rune. Storage and editing.
//   - grapheme a cluster boundary; the only place a cursor may rest.
//   - ColIdx   a display column on screen. Tabs jump to the next tab stop,
//     CJK glyphs occupy two cells, combining marks occupy none.
//
// RuneIdx and ColIdx are distinct types so the compiler rejects mixing them.
package text

// RuneIdx is an index into a line's []rune. It is not a screen column.
type RuneIdx int

// ColIdx is a display column on screen. It is not a rune index.
type ColIdx int

// Pos is a location in a buffer: a line number and a rune index within it.
type Pos struct {
	Line int
	Col  RuneIdx
}

// Before reports whether p sorts strictly before q.
func (p Pos) Before(q Pos) bool {
	return p.Line < q.Line || (p.Line == q.Line && p.Col < q.Col)
}

// After reports whether p sorts strictly after q.
func (p Pos) After(q Pos) bool { return q.Before(p) }

// Equal reports whether p and q are the same position.
func (p Pos) Equal(q Pos) bool { return p.Line == q.Line && p.Col == q.Col }

// MinPos returns whichever of p and q sorts earlier.
func MinPos(p, q Pos) Pos {
	if q.Before(p) {
		return q
	}
	return p
}

// MaxPos returns whichever of p and q sorts later.
func MaxPos(p, q Pos) Pos {
	if q.After(p) {
		return q
	}
	return p
}

// OrderPos returns p and q sorted, for normalizing a region whose endpoints
// may be in either order (point and mark, typically).
func OrderPos(p, q Pos) (lo, hi Pos) { return MinPos(p, q), MaxPos(p, q) }
