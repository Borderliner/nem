// Package highlight makes syntax colouring affordable on every keystroke.
//
// syntax lexes one line given the state the line above left behind, so
// colouring line N means knowing the state at line N, which means lexing from
// line 0. At roughly a microsecond a line that is milliseconds of work per
// frame on a large file - paid on every keystroke, while editing near the
// bottom, for text that has not changed.
//
// Cache removes that cost with one idea: keep the incoming state of every line.
// An edit at line N cannot affect any state above N, so re-lexing starts there.
// More importantly it also STOPS early - re-lexing runs downward only until a
// recomputed state matches the one already cached, because from that line on
// the lexer is in exactly the state it was in before and nothing below can have
// changed. Typing inside a function reaches that after one line; opening a
// block comment does not reach it until the comment closes, which is precisely
// when everything below really has changed colour. One rule covers both.
package highlight

import (
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"slices"
)

// spanCacheLimit bounds how many lines' spans are held.
//
// States are four bytes a line and worth keeping for the whole file; spans are
// a slice each and are only ever wanted for lines on screen. The limit is
// generous next to a screenful so ordinary scrolling never evicts, and eviction
// is all-or-nothing because the alternative - tracking recency per line - costs
// more bookkeeping than re-lexing a screenful, which is about forty
// microseconds.
const spanCacheLimit = 512

// Cache holds per-line lexer states for one buffer.
//
// A Cache belongs to a single buffer: it identifies edits through the buffer's
// own tracking, which is consumed on read. Pointing one Cache at two buffers
// gives correct but needlessly slow results, since each switch looks like a
// change it cannot account for.
type Cache struct {
	lex syntax.Lexer

	// states[i] is the state line i starts in. states[0] is always the zero
	// state, meaning start of file. Lexing line i yields states[i+1], so the
	// slice holds one more entry than the buffer has lines.
	states []syntax.State

	// validTo is how far the states are known correct: states[0..validTo].
	validTo int

	// trustFrom and trustTo bound the states that were correct BEFORE the most
	// recent edit and still describe the same lines afterwards. They are stale
	// but were right for the previous text, which is exactly what the
	// convergence check compares against; without them an edit would force
	// re-lexing to the bottom of the file.
	//
	// It takes two bounds rather than one because inserting lines leaves a gap.
	// Shifting the tail down to make room moves each old state to the line it
	// still describes, but the vacated entries in between describe nothing at
	// all - they are leftovers. A single "trusted up to here" bound cannot
	// express a hole, and a leftover that happened to equal a freshly computed
	// state would converge the cache onto garbage and colour the rest of the
	// file wrong.
	trustFrom, trustTo int

	spans map[int][]syntax.Span

	lastRev  uint64
	numLines int
	attached bool
}

// New returns a cache that lexes with lex.
func New(lex syntax.Lexer) *Cache {
	if lex == nil {
		lex = syntax.PlainLexer{}
	}
	return &Cache{lex: lex, spans: map[int][]syntax.Span{}}
}

// SetLexer changes the language and discards everything, since states computed
// by one lexer mean nothing to another.
func (c *Cache) SetLexer(lex syntax.Lexer) {
	if lex == nil {
		lex = syntax.PlainLexer{}
	}
	c.lex = lex
	c.forget()
}

// Lexer returns the lexer in use.
func (c *Cache) Lexer() syntax.Lexer { return c.lex }

// Invalidate discards what is known from fromLine downward.
//
// Callers do not normally need this: edits are detected through the buffer. It
// is here for changes the buffer cannot report, such as a setting that alters
// how text is classified.
func (c *Cache) Invalidate(fromLine int) {
	if fromLine <= 0 {
		c.forget()
		return
	}
	if fromLine < c.validTo {
		c.validTo = fromLine
	}
	// The caller is reporting a change the buffer cannot describe, so nothing
	// below can be assumed to still hold.
	c.trustFrom, c.trustTo = 0, 0
	c.dropSpans(fromLine)
}

// Spans returns the classified runs of one line.
//
// It returns nil for a line outside the buffer rather than panicking: a caller
// redrawing while a buffer shrinks underneath it should show nothing for a line
// that no longer exists, not crash.
func (c *Cache) Spans(b *text.Buffer, line int) []syntax.Span {
	if b == nil || line < 0 || line >= b.NumLines() {
		return nil
	}
	c.sync(b)
	c.extend(b, line)

	if s, ok := c.spans[line]; ok {
		return s
	}
	s, _ := c.lex.Lex(b.Line(line).Runes(), c.stateAt(line))
	c.store(line, s)
	return s
}

// sync brings the cache into line with whatever has happened to the buffer.
func (c *Cache) sync(b *text.Buffer) {
	from, delta, rev := b.TakeDirty()
	n := b.NumLines()

	if !c.attached {
		c.attached, c.lastRev, c.numLines = true, rev, n
		c.forget()
		return
	}

	if rev == c.lastRev {
		// Nothing changed. A line count that moved anyway means something
		// edited the buffer without the tracking seeing it, so trust nothing.
		if n != c.numLines {
			c.numLines = n
			c.forget()
		}
		return
	}

	// Exactly one mutation since the last read means from and delta describe
	// that single edit and can be applied line by line. Several mutations
	// collapse into one lowest line and a summed delta, which does not describe
	// any one of them - shifting by it could align a stale state against the
	// wrong line and converge on a wrong answer, so the stale states are
	// dropped instead. Correct, and slower only when a reader skips frames.
	single := rev == c.lastRev+1
	c.lastRev = rev
	c.numLines = n

	if from >= n {
		// The revision moved but no line was reported: another reader consumed
		// the record. Nothing can be inferred, so keep nothing.
		c.forget()
		return
	}

	if delta != 0 && !single {
		// Several edits collapsed into one summed delta cannot be applied line
		// by line, so the stale states are misaligned and worth nothing.
		c.dropFrom(from, n)
		return
	}

	was := c.validTo
	if delta != 0 {
		c.shift(from, delta, n)
	} else {
		c.fit(n)
	}
	if from < c.validTo {
		c.validTo = from
	}

	// The edited line's own incoming state is unchanged - it comes from the
	// line above. Everything from the next line down is what may have moved.
	c.trustFrom, c.trustTo = from+1, was+delta
	if delta > 0 {
		// Skip the gap the insertion opened: those entries describe no line.
		c.trustFrom = from + 1 + delta
	}
	c.clampTrust(n)

	if delta == 0 {
		// Only the edited line's text changed: an insertion carrying no newline
		// and a deletion within one line both touch exactly one line. Lines
		// below keep their spans, and rightly so - identical text lexed from an
		// identical state gives an identical answer. Where the state does
		// change, extend overwrites their spans on its way down, and it stops
		// exactly where the state stops changing.
		c.dropSpanAt(from)
	} else {
		// Lines moved, so every cached span below the edit now belongs to the
		// wrong line.
		c.dropSpans(from)
	}
}

// dropFrom keeps the states above line and trusts nothing below.
func (c *Cache) dropFrom(line, n int) {
	c.fit(n)
	if line < c.validTo {
		c.validTo = line
	}
	c.trustFrom, c.trustTo = 0, 0
	c.dropSpans(line)
}

// clampTrust confines the trusted window to the states that exist, and empties
// it when it has collapsed.
func (c *Cache) clampTrust(n int) {
	if c.trustTo > n {
		c.trustTo = n
	}
	if c.trustFrom < 0 {
		c.trustFrom = 0
	}
	if c.trustFrom > c.trustTo {
		c.trustFrom, c.trustTo = 0, 0
	}
}

// extend lexes downward until line has been reached.
//
// It covers line itself rather than stopping above it, so that the spans it
// computes on the way are the ones the caller wants. Stopping one line short
// made rendering lex every line twice: once here for its state, once again for
// its spans.
func (c *Cache) extend(b *text.Buffer, line int) {
	n := b.NumLines()
	c.fit(n)

	for c.validTo <= line {
		i := c.validTo
		if i >= n {
			return
		}
		spans, out := c.lex.Lex(b.Line(i).Runes(), c.states[i])
		c.store(i, spans)

		next := i + 1
		// The convergence check, and the reason the whole package exists: if
		// the state this line leaves matches what it left before the edit, the
		// lexer is in the condition it was in then, so every line below is
		// unchanged and can be trusted without lexing any of it.
		if next >= c.trustFrom && next <= c.trustTo && c.states[next] == out {
			c.validTo = c.trustTo
			c.trustFrom, c.trustTo = 0, 0
			return
		}
		c.states[next] = out
		c.validTo = next
	}
}

// stateAt returns the state line starts in, which extend has already ensured.
func (c *Cache) stateAt(line int) syntax.State {
	if line < 0 || line >= len(c.states) {
		return 0
	}
	return c.states[line]
}

// shift moves each surviving state to the line it still describes, so that
// pressing Enter does not throw away everything below the cursor.
//
// Old line j becomes new line j+delta for every j after the edit, and the
// slice ends up n+1 long. It is shifted in place: rebuilt, it allocated a
// state per line of the file on every newline, which on a large file was most
// of what a keystroke allocated. TestShiftMatchesTheRebuild pins it to the
// straightforward rebuild it replaced, since overlapping moves are where this
// kind of code goes quietly wrong.
func (c *Cache) shift(from, delta, n int) {
	at := min(from+1, len(c.states))
	switch {
	case delta > 0:
		c.states = slices.Insert(c.states, at, make([]syntax.State, delta)...)
	case delta < 0:
		c.states = slices.Delete(c.states, at, min(at-delta, len(c.states)))
	}
	if need := n + 1; len(c.states) < need {
		c.states = append(c.states, make([]syntax.State, need-len(c.states))...)
	} else {
		c.states = c.states[:need]
	}
	c.states[0] = 0
}

// dropSpanAt discards one line's cached spans.
func (c *Cache) dropSpanAt(line int) { delete(c.spans, line) }

// dropSpans discards cached spans from line downward. Their text may have
// changed, and a span is cheap to recompute once its state is known.
func (c *Cache) dropSpans(line int) {
	if line <= 0 {
		clear(c.spans)
		return
	}
	for k := range c.spans {
		if k >= line {
			delete(c.spans, k)
		}
	}
}

// forget discards everything, so the next request lexes from the top.
func (c *Cache) forget() {
	c.validTo, c.trustFrom, c.trustTo = 0, 0, 0
	if len(c.states) > 0 {
		c.states[0] = 0
	}
	clear(c.spans)
}

// fit grows the state slice to cover a buffer of n lines.
func (c *Cache) fit(n int) {
	need := n + 1
	switch {
	case len(c.states) < need:
		grown := make([]syntax.State, need)
		copy(grown, c.states)
		c.states = grown
	case len(c.states) > need:
		c.states = c.states[:need]
		if c.validTo > n {
			c.validTo = n
		}
		c.clampTrust(n)
	}
	if len(c.states) > 0 {
		c.states[0] = 0
	}
}

// store caches one line's spans, dropping everything if the cache has grown
// past its limit.
func (c *Cache) store(line int, spans []syntax.Span) {
	if len(c.spans) >= spanCacheLimit {
		clear(c.spans)
	}
	c.spans[line] = spans
}
