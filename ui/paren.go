package ui

import (
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// ParenScanLimit caps how many runes a bracket search examines in each
// direction.
//
// The search runs on every frame in which point sits next to a bracket, so it
// cannot be unbounded: a large file with one stray opening brace would otherwise
// scan to the end of the buffer on every keystroke. Twenty thousand runes is far
// more than any real nesting needs and is a fraction of a millisecond, so the
// cap only ever bites on genuinely unbalanced text - which is exactly the case
// that should give up and report a mismatch.
const ParenScanLimit = 20000

// lineHL carries the style overrides falling on one line. At most two cells are
// ever overridden - the two halves of a bracket pair - so it is a fixed array
// passed by value and never allocates.
//
// When syntax highlighting lands it will want something richer than this; the
// shape to reach for then is a per-cluster style lookup, not a longer array.
type lineHL struct {
	idx   [2]text.RuneIdx
	style [2]tcell.Style
	n     int
}

// styleFor returns the override for the cluster starting at rune index i, or
// base when that cluster is not highlighted.
func (h lineHL) styleFor(i text.RuneIdx, base tcell.Style) tcell.Style {
	for k := 0; k < h.n; k++ {
		if h.idx[k] == i {
			return h.style[k]
		}
	}
	return base
}

// parenHL is a frame's bracket highlight: one position when a bracket has no
// partner, two when it does, none when point is not next to a bracket.
type parenHL struct {
	pos   [2]text.Pos
	style [2]tcell.Style
	n     int
}

// onLine narrows the highlight to the cells falling on the given buffer line.
// A partner scrolled off screen simply never matches a drawn line, which is why
// drawing needs no special case for it.
func (p parenHL) onLine(line int) lineHL {
	var out lineHL
	for k := 0; k < p.n; k++ {
		if p.pos[k].Line == line {
			out.idx[out.n] = p.pos[k].Col
			out.style[out.n] = p.style[k]
			out.n++
		}
	}
	return out
}

// bracketPartner reports the counterpart of a bracket rune and whether r opens.
func bracketPartner(r rune) (partner rune, opens bool, ok bool) {
	switch r {
	case '(':
		return ')', true, true
	case '[':
		return ']', true, true
	case '{':
		return '}', true, true
	case ')':
		return '(', false, true
	case ']':
		return '[', false, true
	case '}':
		return '{', false, true
	}
	return 0, false, false
}

func isOpener(r rune) bool  { _, opens, ok := bracketPartner(r); return ok && opens }
func isCloser(r rune) bool  { _, opens, ok := bracketPartner(r); return ok && !opens }
func isBracket(r rune) bool { _, _, ok := bracketPartner(r); return ok }

// runeAtPos returns the first rune of the grapheme starting at p.
//
// It walks clusters rather than calling Line.Runes(), which copies the whole
// line - this runs on the render path.
func runeAtPos(b *text.Buffer, p text.Pos) (rune, bool) {
	if p.Line < 0 || p.Line >= b.NumLines() {
		return 0, false
	}
	l := b.Line(p.Line)
	if p.Col < 0 || p.Col >= l.Len() {
		return 0, false
	}
	// From the cluster at p's column rather than from the start of the line:
	// this runs twice a frame, and walking a long line to reach point was a
	// fifth of a keystroke's cost on one.
	for c := range l.ClustersFrom(l.DisplayCol(p.Col)) {
		if c.Start == p.Col {
			return c.Runes[0], true
		}
		if c.Start > p.Col {
			break
		}
	}
	return 0, false
}

// bracketNear returns the bracket to match: the one at point if there is one,
// otherwise the one immediately before it.
//
// Both cases matter. Point sits on a bracket when you have just moved onto it,
// and sits just after one when you have just typed it - and the second is when
// you most want to see the match.
func bracketNear(b *text.Buffer, pt text.Pos) (text.Pos, rune, bool) {
	if r, ok := runeAtPos(b, pt); ok && isBracket(r) {
		return pt, r, true
	}
	if pt.Col > 0 && pt.Line >= 0 && pt.Line < b.NumLines() {
		prev := text.Pos{Line: pt.Line, Col: b.Line(pt.Line).PrevGrapheme(pt.Col)}
		if r, ok := runeAtPos(b, prev); ok && isBracket(r) {
			return prev, r, true
		}
	}
	return text.Pos{}, 0, false
}

// matchParenAt computes the bracket highlight for a window whose point is at pt.
//
// This lives in ui and runs at draw time because a command could not do it:
// Env deliberately cannot reach the screen, so a command has nowhere to report a
// highlight. Nothing about bracket matching belongs in the command layer.
func matchParenAt(b *text.Buffer, pt text.Pos, th Theme) parenHL {
	if b == nil || b.NumLines() == 0 {
		return parenHL{}
	}
	pt = b.ClampPos(pt)
	at, r, ok := bracketNear(b, pt)
	if !ok {
		return parenHL{}
	}

	partner, opens, _ := bracketPartner(r)

	var (
		match text.Pos
		found bool
	)
	if opens {
		match, found = scanForward(b, at, partner)
	} else {
		match, found = scanBackward(b, at, partner)
	}

	var out parenHL
	if !found {
		// An unmatched bracket is still worth marking - that is the signal the
		// user needs, and it is why this is not simply "no highlight".
		out.pos[0], out.style[0], out.n = at, th.ParenMismatch, 1
		return out
	}
	out.pos[0], out.style[0] = at, th.ParenMatch
	out.pos[1], out.style[1] = match, th.ParenMatch
	out.n = 2
	return out
}

// scanForward finds the closer matching an opener at from.
//
// Depth counts intervening pairs of any kind, so the first closer found at depth
// zero is the candidate. If that candidate is the wrong kind of bracket - "([)"
// - the pair is reported as unmatched rather than silently accepted, which is
// what the user needs to see.
func scanForward(b *text.Buffer, from text.Pos, partner rune) (text.Pos, bool) {
	depth := 0
	scanned := 0
	for ln := from.Line; ln < b.NumLines(); ln++ {
		l := b.Line(ln)
		scanned += int(l.Len()) + 1
		if scanned > ParenScanLimit {
			return text.Pos{}, false
		}
		for c := range l.Clusters() {
			if ln == from.Line && c.Start <= from.Col {
				continue
			}
			r := c.Runes[0]
			switch {
			case isOpener(r):
				depth++
			case isCloser(r):
				if depth == 0 {
					if r == partner {
						return text.Pos{Line: ln, Col: c.Start}, true
					}
					return text.Pos{}, false
				}
				depth--
			}
		}
	}
	return text.Pos{}, false
}

// scanBackward finds the opener matching a closer at from. It is the mirror of
// scanForward; since the cluster iterator only runs forward, each line's bracket
// positions are collected and then walked in reverse. Brackets are sparse, so
// the scratch slice stays small and is reused across lines.
func scanBackward(b *text.Buffer, from text.Pos, partner rune) (text.Pos, bool) {
	depth := 0
	scanned := 0
	type ref struct {
		col text.RuneIdx
		r   rune
	}
	var refs []ref

	for ln := from.Line; ln >= 0; ln-- {
		l := b.Line(ln)
		scanned += int(l.Len()) + 1
		if scanned > ParenScanLimit {
			return text.Pos{}, false
		}
		refs = refs[:0]
		for c := range l.Clusters() {
			if ln == from.Line && c.Start >= from.Col {
				break
			}
			if r := c.Runes[0]; isBracket(r) {
				refs = append(refs, ref{c.Start, r})
			}
		}
		for k := len(refs) - 1; k >= 0; k-- {
			switch r := refs[k].r; {
			case isCloser(r):
				depth++
			case isOpener(r):
				if depth == 0 {
					if r == partner {
						return text.Pos{Line: ln, Col: refs[k].col}, true
					}
					return text.Pos{}, false
				}
				depth--
			}
		}
	}
	return text.Pos{}, false
}
