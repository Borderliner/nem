// Package syntax classifies source text one line at a time so a renderer can
// colour it.
//
// The design constraint that shapes everything here is that an editor re-lexes
// on every keystroke. Lexing a whole buffer to redraw one line would make
// typing in a large file feel heavy, so Lex takes a single line plus the State
// left by the line above it and returns the State it leaves behind. A caller
// caches one State per line, and after an edit re-lexes downward only until the
// outgoing State matches what it had cached - at which point nothing below can
// have changed, and it can stop.
//
// That convergence check is why State is a comparable value type rather than an
// interface or a pointer: == has to mean "the rest of the file is unaffected".
//
// tree-sitter is deliberately not used. It is a C library, and cgo would cost
// nem the single static binary that Go was chosen for. These are hand-written
// lexers, which are approximate at the edges - they are colouring text, not
// compiling it - and the tests pin the cases that actually break highlighters
// rather than trying to model each grammar completely.
package syntax

import (
	"path/filepath"
	"strings"
)

// Class is what a span of text is, for colouring purposes.
//
// The set is small and closed on purpose. A theme maps one style per class, so
// every class added is a colour a user has to choose and a decision a theme
// author has to make; a sprawling set makes a coherent theme impossible.
type Class uint8

const (
	// Plain is ordinary text. A region covered by no span is Plain, so a lexer
	// need not emit spans for the gaps between what it classifies.
	Plain Class = iota
	Keyword
	String
	Comment
	Number
	// Function is an identifier in a call or definition position.
	Function
	// Type is a type name where the grammar makes it unambiguous.
	Type
	// Constant is a literal like true, false, nil or iota.
	Constant
	Operator
	Punctuation
)

// String names the class, for test failures and debugging.
func (c Class) String() string {
	switch c {
	case Plain:
		return "Plain"
	case Keyword:
		return "Keyword"
	case String:
		return "String"
	case Comment:
		return "Comment"
	case Number:
		return "Number"
	case Function:
		return "Function"
	case Type:
		return "Type"
	case Constant:
		return "Constant"
	case Operator:
		return "Operator"
	case Punctuation:
		return "Punctuation"
	}
	return "Class(?)"
}

// Span is a half-open run of one class within a line.
//
// Start and End are RUNE indices, not byte offsets. The renderer converts them
// to display columns, and a byte offset would land in the wrong column the
// moment a line contains anything outside ASCII - which is exactly where a
// highlighter's mistakes are least forgivable, since the text still looks fine
// and only the colours are wrong.
type Span struct {
	Start, End int
	Class      Class
}

// State is what one line leaves open for the next: a block comment, a raw
// string, a fenced code block.
//
// It is opaque and its encoding is private to each lexer, but its layout is
// fixed so that the zero value means the same thing everywhere:
//
//	bits 0..7    mode   - lexer-specific; 0 always means "nothing is open"
//	bits 8..23   param  - lexer-specific; a Lua long-bracket level, a Markdown
//	                      fence length and its delimiter
//	bits 24..31  unused, always zero
//
// The zero State is therefore "start of file", which is what a caller passes
// for line 0 without needing to ask the lexer for an initial value.
type State uint32

func mkState(mode uint8, param uint16) State {
	return State(uint32(mode) | uint32(param)<<8)
}

func (s State) mode() uint8   { return uint8(s & 0xFF) }
func (s State) param() uint16 { return uint16((s >> 8) & 0xFFFF) }

// Lexer classifies one line at a time.
//
// Implementations must be pure: the same line and incoming State must always
// produce the same spans and outgoing State, with nothing carried in the
// receiver. A caller re-lexing from the middle of a file relies on that, and a
// lexer holding hidden state between calls would give different colours
// depending on how the user happened to scroll.
type Lexer interface {
	// Lex classifies one line. Spans are ascending, non-overlapping, non-empty
	// and within the line. Regions covered by no span are Plain.
	Lex(line []rune, in State) (spans []Span, out State)
	// Name identifies the language, for the modeline and for tests.
	Name() string
}

// For returns the lexer for a path, by file extension.
//
// It never returns nil: an unrecognised extension gets the plain lexer, so a
// caller never has to check before lexing.
func For(path string) Lexer {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return goLexer{}
	case ".lua":
		return luaLexer{}
	case ".json":
		return jsonLexer{}
	case ".md", ".markdown":
		return markdownLexer{}
	}
	return PlainLexer{}
}

// PlainLexer classifies nothing, for text nem has no grammar for.
type PlainLexer struct{}

func (PlainLexer) Name() string { return "text" }

// Lex returns the whole line as one Plain span, and never carries state.
func (PlainLexer) Lex(line []rune, _ State) ([]Span, State) {
	var b builder
	b.add(0, len(line), Plain)
	return b.out, 0
}

// builder accumulates spans while enforcing the invariants the interface
// promises, so no lexer has to remember them.
//
// It drops empty spans and merges a span into the previous one when they are
// contiguous and of the same class. Merging keeps the output canonical - two
// adjacent string literals produce one span rather than two - which both cuts
// the number of spans a renderer walks and makes tests compare a single obvious
// answer instead of an arbitrary tokenisation.
type builder struct {
	out []Span
}

func (b *builder) add(start, end int, c Class) {
	if end <= start {
		return
	}
	if b.out == nil {
		// Most lines hold a handful of spans, and growing from nil cost an
		// extra allocation or two on every line of a full-file lex. One
		// speculative allocation of a typical size removes them.
		b.out = make([]Span, 0, 8)
	}
	if n := len(b.out); n > 0 {
		if last := &b.out[n-1]; last.Class == c && last.End == start {
			last.End = end
			return
		}
	}
	b.out = append(b.out, Span{Start: start, End: end, Class: c})
}
