package editor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Borderliner/nem/highlight"
	"github.com/Borderliner/nem/syntax"
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
	if b == nil || e.raw[b] {
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
	if st := e.compileOf(b); st != nil {
		return st.lineSpans(line)
	}
	return e.cacheFor(b).Spans(b, line)
}

// languages returns the languages buffers are coloured in: nem's own, and
// once the config has loaded, the user's from beside init.lua.
func (e *Editor) languages() *syntax.Set {
	if e.langs == nil {
		return syntax.Builtin()
	}
	return e.langs
}

// loadLanguages reads the user's language definitions from dir, over nem's.
// A broken definition is reported, as a broken init.lua is, and costs only
// itself: every other language, the user's and nem's, is still coloured.
func (e *Editor) loadLanguages(dir string) []error {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	e.syntaxDir = dir
	set, errs := syntax.Load(dir)
	e.langs = set
	if len(errs) > 0 {
		more := ""
		if len(errs) > 1 {
			more = fmt.Sprintf(" (and %d more)", len(errs)-1)
		}
		e.Echo("syntax: %v%s", errs[0], more)
	}
	return errs
}

// reloadLanguagesAfterSaving reads the definitions again when a file saved is
// one of them, and recolours every buffer, so a language is written and tried
// in one sitting: the definition in one window, a file of it in the other.
func (e *Editor) reloadLanguagesAfterSaving(path string) {
	if e.syntaxDir == "" || filepath.Ext(path) != ".syntax" {
		return
	}
	if dir, err := filepath.Abs(filepath.Dir(path)); err != nil || !sameDir(dir, e.syntaxDir) {
		return
	}
	if errs := e.loadLanguages(e.syntaxDir); len(errs) > 0 {
		return // reported
	}
	for b := range e.hl {
		e.retuneHighlight(b)
	}
	e.Echo("syntax: %d languages", len(e.langs.Languages()))
}

// sameDir reports whether a and b name one directory.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ia, err1 := os.Stat(a)
	ib, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(ia, ib)
}

// lexerFor picks the lexer for a buffer: its language by its name, or by its
// first line, which is what identifies a script with no extension by its #!.
// A buffer nothing matches gets the plain lexer, so *scratch* and *Buffer
// List* render uncoloured without a special case.
func (e *Editor) lexerFor(b *text.Buffer) syntax.Lexer {
	return e.languages().For(b.Path(), firstLine(b))
}

// firstLine is a buffer's first line, or "".
func firstLine(b *text.Buffer) string {
	if b.NumLines() == 0 {
		return ""
	}
	return b.Line(0).String()
}

// LanguageOf reports the language a buffer is written in, or nil: what M-;
// asks, to write a comment the way the language does.
func (e *Editor) LanguageOf(b *text.Buffer) *syntax.Language {
	if b == nil {
		return nil
	}
	return e.languages().Detect(b.Path(), firstLine(b))
}

// cacheFor returns the buffer's highlight cache, creating it on first use.
//
// Caches are per buffer because the cache holds one lexer state per line: they
// describe a particular buffer's contents and mean nothing for another.
func (e *Editor) cacheFor(b *text.Buffer) *highlight.Cache {
	if c, ok := e.hl[b]; ok {
		return c
	}
	c := highlight.New(e.lexerFor(b))
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
	// The same language is the same Language: a reloaded definition is a
	// new one, and must replace the old even though its name is the same.
	lex := e.lexerFor(b)
	if lex == c.Lexer() {
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
