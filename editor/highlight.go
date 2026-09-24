package editor

import (
	"sync"

	"github.com/Borderliner/nem/highlight"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/syntax/nanorc"
	"github.com/Borderliner/nem/text"
)

// Syntax highlighting is wired here because it needs three things that live in
// three different places: a lexer chosen from the buffer's file name, a cache of
// per-line lexer state, and the buffer itself. The renderer has none of them,
// which is why ui asks through Frame.SpansOf rather than working it out - the
// same arrangement as Frame.NameOf and for the same reason.

// spansOf reports how one line of a buffer is classified.
//
// It is the SpansFunc handed to the renderer on every frame. A buffer with no
// recognised extension gets the plain lexer, which classifies nothing, so
// *scratch* and *Buffer List* render uncoloured without a special case here.
func (e *Editor) spansOf(b *text.Buffer, line int) []syntax.Span {
	if b == nil {
		return nil
	}
	// A listing is coloured from the Listing that produced it, not lexed.
	if st := e.diredOf(b); st != nil {
		if st.wd != nil {
			return st.wdiredSpans(b, line, st.spans(line))
		}
		return st.spans(line)
	}
	if st := e.grepOf(b); st != nil {
		return st.lineSpans(line)
	}
	return e.cacheFor(b).Spans(b, line)
}

// nanoSet holds the languages read from the system's nano installation.
//
// Loaded once and lazily: reading forty files is cheap but pointless for an
// editor that never opens a file nano covers, and doing it at init would charge
// every launch for it. sync.Once rather than a plain nil check because the
// editor may hold several buffers and the first lex of each could race - the
// input loop is single-threaded today, and this does not depend on that staying
// true.
var (
	nanoOnce sync.Once
	nanoSet  *nanorc.Set
)

func nanoLexers() *nanorc.Set {
	nanoOnce.Do(func() {
		// Problems are dropped rather than reported: a malformed file in a
		// system directory is not something the user of this editor can act on,
		// and one warning per launch would be noise. The affected language is
		// simply not offered.
		nanoSet, _ = nanorc.Load(nanorc.DefaultDirs()...)
	})
	return nanoSet
}

// lexerFor picks the lexer for a buffer.
//
// nem's hand-written lexers come first: they carry proper state across lines
// and are precise where a regex pass can only approximate. nano's definitions
// fill in everything else, which is most languages. A buffer nothing matches
// gets the plain lexer, so *scratch* and *Buffer List* render uncoloured
// without a special case.
func lexerFor(b *text.Buffer) syntax.Lexer {
	// The first line is read up front because the bundled rules need the
	// shebang too, not just nano's. An extensionless script called "deploy"
	// starting with #!/bin/sh must highlight on a machine with no nano
	// installed, which is the whole reason the rules are bundled.
	var first string
	if b.NumLines() > 0 {
		first = b.Line(0).String()
	}
	lex := syntax.ForWithHeader(b.Path(), first)
	if _, plain := lex.(syntax.PlainLexer); !plain {
		return lex
	}
	if b.Path() == "" {
		return lex // a nameless buffer has nothing to match on
	}
	if n := nanoLexers().For(b.Path(), first); n != nil {
		return n
	}
	return lex
}

// cacheFor returns the buffer's highlight cache, creating it on first use.
//
// Caches are per buffer because the cache holds one lexer state per line: they
// describe a particular buffer's contents and mean nothing for another.
func (e *Editor) cacheFor(b *text.Buffer) *highlight.Cache {
	if c, ok := e.hl[b]; ok {
		return c
	}
	c := highlight.New(lexerFor(b))
	e.hl[b] = c
	return c
}

// retuneHighlight picks the lexer again after a buffer's path changes.
//
// write-file is the case: a *scratch* buffer saved as main.go was lexed as plain
// text a moment ago and must now be lexed as Go. Without this the file stays
// uncoloured until the editor is restarted, which reads as highlighting being
// broken rather than as a stale lexer.
func (e *Editor) retuneHighlight(b *text.Buffer) {
	c, ok := e.hl[b]
	if !ok {
		return // no cache yet; cacheFor will pick the right lexer when one is made
	}
	lex := lexerFor(b)
	if lex.Name() == c.Lexer().Name() {
		return
	}
	c.SetLexer(lex)
}

// forgetHighlight drops a killed buffer's cache.
//
// The map is keyed by pointer and nothing else would ever remove the entry, so
// without this a long session leaks one cache per buffer the user opens and
// closes - along with a lexer state for every line each of them had.
func (e *Editor) forgetHighlight(b *text.Buffer) {
	delete(e.hl, b)
}
