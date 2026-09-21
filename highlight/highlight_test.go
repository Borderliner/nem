package highlight_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/Borderliner/nem/highlight"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// reference is the obvious implementation: lex every line from the top, every
// time. It is what the cache must agree with, and the only definition of
// correct that does not restate the cache's own logic.
func reference(lex syntax.Lexer, b *text.Buffer, line int) []syntax.Span {
	var st syntax.State
	for i := 0; i < line; i++ {
		_, st = lex.Lex(b.Line(i).Runes(), st)
	}
	spans, _ := lex.Lex(b.Line(line).Runes(), st)
	return spans
}

// agrees checks every line of the buffer against the reference.
func agrees(t *testing.T, c *highlight.Cache, lex syntax.Lexer, b *text.Buffer, when string) {
	t.Helper()
	for i := 0; i < b.NumLines(); i++ {
		got, want := c.Spans(b, i), reference(lex, b, i)
		if !sameSpans(got, want) {
			t.Fatalf("%s: line %d\n  cached    %v\n  reference %v\n  text %q",
				when, i, got, want, string(b.Line(i).Runes()))
		}
	}
}

func sameSpans(a, b []syntax.Span) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func buf(t *testing.T, lines ...string) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	if s := strings.Join(lines, "\n"); s != "" {
		if err := b.Insert(text.Pos{}, []rune(s)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	b.BreakUndo()
	return b
}

// THE test. Everything else here is a named example of a case this covers.
//
// A caching bug shows up as "the colours are wrong somewhere below the edit",
// which is invisible in any test that only checks the line it just edited. This
// generates buffers and edits at random and compares every line against a full
// lex from the top.
func TestCachedSpansAlwaysMatchAFullLex(t *testing.T) {
	lex := syntax.For("x.go")
	// Fragments chosen so edits routinely open and close multi-line
	// constructs - that is where a cache goes wrong.
	frags := []string{
		"package main", "", "func f() {", "}", "\treturn 1",
		"\t// comment", "/*", "*/", "\ts := `raw", "raw`",
		"\tx := 42", "\ty := \"str\"", "/* both */", "`",
	}

	rng := rand.New(rand.NewSource(20260921))
	for trial := 0; trial < 60; trial++ {
		lines := make([]string, 3+rng.Intn(12))
		for i := range lines {
			lines[i] = frags[rng.Intn(len(frags))]
		}
		b := buf(t, lines...)
		c := highlight.New(lex)
		agrees(t, c, lex, b, fmt.Sprintf("trial %d, initial", trial))

		for edit := 0; edit < 8; edit++ {
			applyRandomEdit(t, rng, b, frags)
			agrees(t, c, lex, b, fmt.Sprintf("trial %d, after edit %d", trial, edit))
		}
	}
}

func applyRandomEdit(t *testing.T, rng *rand.Rand, b *text.Buffer, frags []string) {
	t.Helper()
	ln := rng.Intn(b.NumLines())
	l := b.Line(ln)
	switch rng.Intn(4) {
	case 0: // insert text within a line
		col := text.RuneIdx(rng.Intn(int(l.Len()) + 1))
		if err := b.Insert(text.Pos{Line: ln, Col: col}, []rune(frags[rng.Intn(len(frags))])); err != nil {
			t.Fatalf("insert: %v", err)
		}
	case 1: // insert a whole new line
		if err := b.Insert(text.Pos{Line: ln, Col: 0},
			[]rune(frags[rng.Intn(len(frags))]+"\n")); err != nil {
			t.Fatalf("insert line: %v", err)
		}
	case 2: // delete within a line
		if l.Len() > 0 {
			a := text.RuneIdx(rng.Intn(int(l.Len())))
			z := a + text.RuneIdx(1+rng.Intn(int(l.Len()-a)))
			if err := b.Delete(text.Pos{Line: ln, Col: a}, text.Pos{Line: ln, Col: z}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		}
	case 3: // join with the next line
		if ln+1 < b.NumLines() {
			if err := b.Delete(text.Pos{Line: ln, Col: l.Len()},
				text.Pos{Line: ln + 1, Col: 0}); err != nil {
				t.Fatalf("join: %v", err)
			}
		}
	}
}

// Opening a block comment must recolour everything below it, which is the case
// the early stop must NOT cut short.
func TestOpeningABlockCommentColoursEverythingBelow(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "func f() {}", "var x = 1", "var y = 2")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	// Turn line 1 into an unterminated block comment.
	if err := b.Insert(text.Pos{Line: 1, Col: 0}, []rune("/*")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	agrees(t, c, lex, b, "after opening a block comment")

	for _, ln := range []int{2, 3} {
		spans := c.Spans(b, ln)
		if len(spans) == 0 || spans[0].Class != syntax.Comment {
			t.Errorf("line %d = %v, want it inside the block comment", ln, spans)
		}
	}
}

// Closing one must restore the lines below to ordinary code.
func TestClosingABlockCommentRestoresTheLinesBelow(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "/*", "var x = 1", "var y = 2")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	if err := b.Insert(text.Pos{Line: 1, Col: 2}, []rune("*/")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	agrees(t, c, lex, b, "after closing the block comment")

	if spans := c.Spans(b, 3); len(spans) > 0 && spans[0].Class == syntax.Comment {
		t.Error("line 3 still reads as a comment after the block closed")
	}
}

// An edit deep in a file must not force re-lexing the whole thing. The cache is
// pointless if it does, and nothing else here would notice.
func TestAnEditStopsEarly(t *testing.T) {
	lex := syntax.For("x.go")
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = "\tx := 1"
	}
	b := buf(t, lines...)

	counted := &countingLexer{Lexer: lex}
	c := highlight.New(counted)
	for i := 0; i < b.NumLines(); i++ {
		c.Spans(b, i) // warm
	}

	counted.n = 0
	if err := b.Insert(text.Pos{Line: 200, Col: 2}, []rune("y")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	for i := 0; i < b.NumLines(); i++ {
		c.Spans(b, i)
	}
	// The edited line must be re-lexed, and convergence should stop almost
	// immediately after it. A generous bound still fails loudly if the early
	// stop is dropped and all 200 lines below are re-lexed.
	if counted.n > 20 {
		t.Errorf("re-lexed %d lines after a one-line edit, want the early stop to end it quickly", counted.n)
	}
}

// Lazy: rendering the top of a large file must not lex the whole file.
func TestOnlyLexesWhatIsAskedFor(t *testing.T) {
	lex := syntax.For("x.go")
	lines := make([]string, 5000)
	for i := range lines {
		lines[i] = "\tx := 1"
	}
	b := buf(t, lines...)

	counted := &countingLexer{Lexer: lex}
	c := highlight.New(counted)
	for i := 0; i < 40; i++ { // one screenful
		c.Spans(b, i)
	}
	if counted.n > 60 {
		t.Errorf("lexed %d lines to render 40, want roughly a screenful", counted.n)
	}
}

func TestLineCountChanges(t *testing.T) {
	lex := syntax.For("x.go")
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, b *text.Buffer)
	}{
		{"insert in the middle", func(t *testing.T, b *text.Buffer) {
			if err := b.Insert(text.Pos{Line: 1, Col: 0}, []rune("/*\n")); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete a line", func(t *testing.T, b *text.Buffer) {
			if err := b.Delete(text.Pos{Line: 1, Col: 0}, text.Pos{Line: 2, Col: 0}); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete the last line", func(t *testing.T, b *text.Buffer) {
			n := b.NumLines()
			if err := b.Delete(text.Pos{Line: n - 2, Col: b.Line(n - 2).Len()},
				text.Pos{Line: n - 1, Col: b.Line(n - 1).Len()}); err != nil {
				t.Fatal(err)
			}
		}},
		{"empty the buffer", func(t *testing.T, b *text.Buffer) {
			if err := b.Delete(text.Pos{}, b.End()); err != nil {
				t.Fatal(err)
			}
		}},
		{"insert many lines at once", func(t *testing.T, b *text.Buffer) {
			if err := b.Insert(text.Pos{Line: 0, Col: 0}, []rune("a\nb\nc\nd\n")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := buf(t, "package main", "/*", "var x = 1", "*/", "var y = 2")
			c := highlight.New(lex)
			agrees(t, c, lex, b, "before")
			tc.edit(t, b)
			agrees(t, c, lex, b, "after")
		})
	}
}

// Undo and redo reach the buffer through the raw primitives. If they were not
// tracked the colours would silently lag the text.
func TestUndoAndRedoAreReflected(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var x = 1", "var y = 2")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	if err := b.Insert(text.Pos{Line: 1, Col: 0}, []rune("/*")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	agrees(t, c, lex, b, "after opening a comment")

	if _, ok := b.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	agrees(t, c, lex, b, "after undo")

	if _, ok := b.Redo(); !ok {
		t.Fatal("nothing to redo")
	}
	agrees(t, c, lex, b, "after redo")
}

func TestSetLexerDiscardsState(t *testing.T) {
	b := buf(t, "-- lua comment", "x = 1")
	c := highlight.New(syntax.For("x.go"))
	c.Spans(b, 0)

	lua := syntax.For("x.lua")
	c.SetLexer(lua)
	agrees(t, c, lua, b, "after switching language")

	if spans := c.Spans(b, 0); len(spans) == 0 || spans[0].Class != syntax.Comment {
		t.Errorf("line 0 = %v, want a Lua comment after the lexer changed", spans)
	}
}

func TestInvalidateForcesARelex(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var x = 1")
	counted := &countingLexer{Lexer: lex}
	c := highlight.New(counted)
	c.Spans(b, 1)

	counted.n = 0
	c.Invalidate(0)
	c.Spans(b, 1)
	if counted.n == 0 {
		t.Error("Invalidate(0) did not force a re-lex")
	}
}

func TestOutOfRangeLinesReturnNothing(t *testing.T) {
	c := highlight.New(syntax.For("x.go"))
	b := buf(t, "package main")
	for _, ln := range []int{-1, 1, 99} {
		if got := c.Spans(b, ln); got != nil {
			t.Errorf("Spans(line %d) = %v, want nil", ln, got)
		}
	}
	if got := c.Spans(nil, 0); got != nil {
		t.Errorf("Spans(nil buffer) = %v, want nil", got)
	}
}

// A second reader consumes the buffer's change record, so this cache sees the
// revision move with no line to blame. That must degrade to re-lexing, not to
// showing stale colours.
func TestASecondReaderDoesNotCauseWrongColours(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var x = 1", "var y = 2")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	if err := b.Insert(text.Pos{Line: 1, Col: 0}, []rune("/*")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	b.TakeDirty() // a rival reader steals the notification

	agrees(t, c, lex, b, "after another reader consumed the change")
}

// countingLexer records how many lines were lexed.
type countingLexer struct {
	syntax.Lexer
	n int
}

func (c *countingLexer) Lex(line []rune, in syntax.State) ([]syntax.Span, syntax.State) {
	c.n++
	return c.Lexer.Lex(line, in)
}
