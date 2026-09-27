// Package bidi lays out right-to-left text for terminals that cannot. Most of
// them - kitty, Ghostty, Alacritty, WezTerm, foot - draw cells strictly left to
// right and never join Arabic-script letters, so Persian, Arabic or Hebrew
// typed into nem shows backwards and, for the Arabic script, as a string of
// isolated letters. Emacs gets round this by doing the Unicode Bidirectional
// Algorithm itself and drawing text in visual order; nem does the same, and
// this is the algorithm.
//
// The renderer asks HasRTL of every line, which costs next to nothing for
// text with no right-to-left in it. For a line that has some it takes the
// line's direction from ParagraphDirection, resolves an embedding level for
// every rune with Levels, groups the runes into grapheme clusters each given
// the level of its first rune, and draws the clusters in the order Order
// gives, the Mirror of a bracket at an odd level in its place.
//
// It implements UAX #9 for one paragraph at a time - nem takes each line as
// its own paragraph - through rule L2, and checks itself against Unicode's
// conformance data. What it leaves to the renderer is what UAX #9 leaves to
// one: which glyph to draw (L4, through Mirror) and keeping combining marks
// with their base (L3, which clustering does).
package bidi

import xbidi "golang.org/x/text/unicode/bidi"

//go:generate go run gen.go

// Direction is a paragraph's (or run's) direction.
type Direction int

const (
	LTR Direction = iota
	RTL
)

// maxDepth is the deepest embedding level the explicit rules make (BD2).
// The implicit rules can take a character one or two past it, so a resolved
// level is at most maxDepth+1.
const maxDepth = 125

// class is a character's bidirectional type: its Bidi_Class, and later what
// the rules have made of it.
type class uint8

const (
	tL   class = iota // left-to-right
	tR                // right-to-left
	tEN               // European number
	tES               // European separator
	tET               // European terminator
	tAN               // Arabic number
	tCS               // common separator
	tB                // paragraph separator
	tS                // segment separator
	tWS               // whitespace
	tON               // other neutral
	tBN               // boundary neutral
	tNSM              // nonspacing mark
	tAL               // Arabic letter
	tLRO              // left-to-right override
	tRLO              // right-to-left override
	tLRE              // left-to-right embedding
	tRLE              // right-to-left embedding
	tPDF              // pop directional format
	tLRI              // left-to-right isolate
	tRLI              // right-to-left isolate
	tFSI              // first strong isolate
	tPDI              // pop directional isolate
)

// fromX translates golang.org/x/text's classes, which are the character
// database's, into ours.
var fromX = [...]class{
	xbidi.L: tL, xbidi.R: tR, xbidi.EN: tEN, xbidi.ES: tES, xbidi.ET: tET,
	xbidi.AN: tAN, xbidi.CS: tCS, xbidi.B: tB, xbidi.S: tS, xbidi.WS: tWS,
	xbidi.ON: tON, xbidi.BN: tBN, xbidi.NSM: tNSM, xbidi.AL: tAL, xbidi.Control: tON,
	xbidi.LRO: tLRO, xbidi.RLO: tRLO, xbidi.LRE: tLRE, xbidi.RLE: tRLE,
	xbidi.PDF: tPDF, xbidi.LRI: tLRI, xbidi.RLI: tRLI, xbidi.FSI: tFSI,
	xbidi.PDI: tPDI,
}

// lowClass holds the classes of everything below U+0900 - ASCII, the other
// Latin, Greek and Cyrillic, and Hebrew, Arabic and the scripts beside them
// - which is what nearly every line is made of, so they skip the trie.
var lowClass [0x0900]class

func init() {
	for r := range lowClass {
		lowClass[r] = lookup(rune(r))
	}
}

func classOf(r rune) class {
	if 0 <= r && r < rune(len(lowClass)) {
		return lowClass[r]
	}
	return lookup(r)
}

func lookup(r rune) class {
	p, _ := xbidi.LookupRune(r)
	if c := p.Class(); int(c) < len(fromX) {
		return fromX[c]
	}
	return tON
}

// removed reports whether X9 takes characters of class c out of the
// paragraph: the embedding and override controls and the boundary neutrals,
// ZWJ and ZWNJ among them. They get no level of their own.
func removed(c class) bool {
	return c == tBN || tLRO <= c && c <= tPDF
}

func isIsolateInitiator(c class) bool { return tLRI <= c && c <= tFSI }

// HasRTL reports whether rs holds any strong right-to-left character (bidi
// class R or AL) - the cheap check the renderer makes on every line so text
// without any costs nothing. It also reports an explicit right-to-left
// embedding, override or isolate (RLE, RLO, RLI), which turn even
// left-to-right letters around.
//
// Everything below U+0590 is passed over with one comparison, and the
// character database is only asked about the ranges that hold right-to-left
// scripts.
func HasRTL(rs []rune) bool {
	for _, r := range rs {
		if r < 0x0590 {
			continue
		}
		if isRTL(r) {
			return true
		}
	}
	return false
}

// isRTL is HasRTL for one character at or above U+0590, kept apart so the
// loop over the rest stays small.
func isRTL(r rune) bool {
	switch {
	case r <= 0x08FF, 0xFB1D <= r && r <= 0xFEFE,
		0x10800 <= r && r <= 0x10FFF, 0x1E800 <= r && r <= 0x1EFFF:
		c := classOf(r)
		return c == tR || c == tAL
	case r == 0x200F, r == 0x202B, r == 0x202E, r == 0x2067: // RLM, RLE, RLO, RLI
		return true
	}
	return false
}

// ParagraphDirection is the direction of the first strong character (UAX #9
// P2/P3, skipping characters between an isolate initiator and its matching
// PDI), LTR when there is none.
func ParagraphDirection(rs []rune) Direction {
	d, _ := firstStrong(rs, false)
	return d
}

// firstStrong finds the direction of the first strong character of rs that
// is not inside an isolate (P2), and whether there was one. Isolated says
// rs starts just inside an isolate, as it does when an FSI is looking at its
// own contents: the PDI that closes it ends the search.
func firstStrong(rs []rune, isolated bool) (Direction, bool) {
	depth := 0
	for _, r := range rs {
		switch classOf(r) {
		case tL:
			if depth == 0 {
				return LTR, true
			}
		case tR, tAL:
			if depth == 0 {
				return RTL, true
			}
		case tLRI, tRLI, tFSI:
			depth++
		case tPDI:
			if depth > 0 {
				depth--
			} else if isolated {
				return LTR, false
			}
		}
	}
	return LTR, false
}

// Levels resolves the embedding level of every rune of one paragraph - nem
// treats each line as its own paragraph - laid out with base direction dir.
// It applies X1–X10 (explicit embeddings, overrides and isolates: LRE RLE
// LRO RLO PDF LRI RLI FSI PDI), W1–W7, N0 (bracket pairs), N1–N2, I1–I2 and
// L1 (segment separators, and whitespace/isolate formatting characters
// before them and at the end of the line, reset to the paragraph level).
// Characters removed by X9 (BN and the explicit embedding controls) get the
// level of the character before them (or the paragraph level at the start),
// so len(result) == len(rs) and every index is usable.
//
// A paragraph separator inside rs does not start a new paragraph: it is
// given the paragraph level, and ends the whitespace before it for L1, and
// that is all.
func Levels(rs []rune, dir Direction) []uint8 {
	n := len(rs)
	levels := make([]uint8, n)
	if n == 0 {
		return levels
	}
	buf := make([]class, 3*n)
	p := paragraph{
		rs:      rs,
		initial: buf[:n:n],
		types:   buf[n : 2*n : 2*n],
		work:    buf[2*n:],
		levels:  levels,
	}
	if dir == RTL {
		p.base = 1
	}

	explicit, removals, isolates := false, false, false
	for i, r := range rs {
		c := classOf(r)
		p.initial[i] = c
		switch {
		case c == tBN:
			removals = true
		case tLRO <= c && c <= tPDF:
			explicit, removals = true, true
		case tLRI <= c:
			explicit, isolates = true, true
		}
	}
	copy(p.types, p.initial)

	if explicit {
		p.explicit()
		p.isolatingRuns(isolates)
	} else {
		// The common case: with no explicit controls every character is at
		// the paragraph level, so the whole paragraph, less what X9
		// removes, is one isolating run sequence.
		for i := range levels {
			levels[i] = p.base
		}
		d := direction(p.base)
		s := sequence{level: p.base, sos: d, eos: d}
		if removals {
			s.idx = make([]int32, 0, n)
			for i, c := range p.initial {
				if !removed(c) {
					s.idx = append(s.idx, int32(i))
				}
			}
		}
		p.resolve(s)
	}
	p.lineLevels()
	return levels
}

// paragraph is one paragraph being resolved.
type paragraph struct {
	rs []rune
	// initial holds each rune's class as the character database gives it,
	// and types the classes the explicit rules leave, overrides applied.
	initial, types []class
	// work is where an isolating run sequence's classes are resolved.
	work   []class
	levels []uint8
	base   uint8
}

// direction is the direction of text at level l: R when it is odd.
func direction(l uint8) class {
	if l&1 == 1 {
		return tR
	}
	return tL
}

// explicit applies X1–X8: it walks the paragraph with a stack of the
// embeddings, overrides and isolates open at each point, setting each
// character's embedding level and, under an override, its class.
func (p *paragraph) explicit() {
	type entry struct {
		level    uint8
		override class // tON for none
		isolate  bool
	}
	var stack [maxDepth + 2]entry
	sp := 0
	stack[0] = entry{level: p.base, override: tON}
	// X1's counters: embeddings and isolates that went past maxDepth (and
	// so were not pushed), and isolates that were.
	overflowIsolates, overflowEmbeddings, validIsolates := 0, 0, 0

	for i, c := range p.initial {
		switch c {
		case tRLE, tLRE, tRLO, tLRO: // X2–X5
			l := next(stack[sp].level, c == tRLE || c == tRLO)
			if l <= maxDepth && overflowIsolates == 0 && overflowEmbeddings == 0 {
				o := tON
				if c == tRLO {
					o = tR
				} else if c == tLRO {
					o = tL
				}
				sp++
				stack[sp] = entry{level: l, override: o}
			} else if overflowIsolates == 0 {
				overflowEmbeddings++
			}
		case tRLI, tLRI, tFSI: // X5a–X5c
			p.levels[i] = stack[sp].level
			if o := stack[sp].override; o != tON {
				p.types[i] = o
			}
			rtl := c == tRLI
			if c == tFSI {
				d, _ := firstStrong(p.rs[i+1:], true)
				rtl = d == RTL
			}
			l := next(stack[sp].level, rtl)
			if l <= maxDepth && overflowIsolates == 0 && overflowEmbeddings == 0 {
				validIsolates++
				sp++
				stack[sp] = entry{level: l, override: tON, isolate: true}
			} else {
				overflowIsolates++
			}
		case tPDI: // X6a
			if overflowIsolates > 0 {
				overflowIsolates--
			} else if validIsolates > 0 {
				overflowEmbeddings = 0
				for !stack[sp].isolate {
					sp--
				}
				sp--
				validIsolates--
			}
			p.levels[i] = stack[sp].level
			if o := stack[sp].override; o != tON {
				p.types[i] = o
			}
		case tPDF: // X7
			if overflowIsolates == 0 {
				if overflowEmbeddings > 0 {
					overflowEmbeddings--
				} else if !stack[sp].isolate && sp > 0 {
					sp--
				}
			}
		case tB: // X8
			p.levels[i] = p.base
		case tBN: // removed by X9: lineLevels gives it a level
		default: // X6
			p.levels[i] = stack[sp].level
			if o := stack[sp].override; o != tON {
				p.types[i] = o
			}
		}
	}
}

// next is the least level above l of the given direction: odd for
// right-to-left, even for left-to-right.
func next(l uint8, rtl bool) uint8 {
	if rtl {
		return (l + 1) | 1
	}
	return (l + 2) &^ 1
}
