package highlight_test

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/highlight"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// Several edits between two reads collapse into one lowest line and a summed
// delta that describes none of them individually. Shifting by that sum would
// align stale states against the wrong lines, so the cache drops them. This is
// the path an editor takes whenever it skips a redraw.
func TestSeveralEditsBetweenReadsStayCorrect(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var a = 1", "var b = 2", "var c = 3", "var d = 4")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	// Three edits with different line deltas, no read in between.
	if err := b.Insert(text.Pos{Line: 3, Col: 0}, []rune("/*\n")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := b.Delete(text.Pos{Line: 1, Col: 0}, text.Pos{Line: 2, Col: 0}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := b.Insert(text.Pos{Line: 0, Col: 0}, []rune("// head\n")); err != nil {
		t.Fatalf("insert: %v", err)
	}

	agrees(t, c, lex, b, "after three unread edits")
}

// The same, with the edits interleaved by reads, which is the fast path.
func TestEditsWithReadsBetweenStayCorrect(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var a = 1", "var b = 2", "var c = 3")
	c := highlight.New(lex)

	for _, edit := range []func() error{
		func() error { return b.Insert(text.Pos{Line: 2, Col: 0}, []rune("/*\n")) },
		func() error { return b.Insert(text.Pos{Line: 3, Col: 0}, []rune("*/\n")) },
		func() error { return b.Delete(text.Pos{Line: 0, Col: 0}, text.Pos{Line: 1, Col: 0}) },
	} {
		if err := edit(); err != nil {
			t.Fatalf("edit: %v", err)
		}
		agrees(t, c, lex, b, "after an edit with a read between")
	}
}

// Scrolling through a file longer than the span cache must keep giving right
// answers: eviction may cost work, never correctness.
func TestScrollingPastTheSpanCacheLimitStaysCorrect(t *testing.T) {
	lex := syntax.For("x.go")
	lines := make([]string, 1200)
	for i := range lines {
		switch i % 4 {
		case 0:
			lines[i] = "var x = 1"
		case 1:
			lines[i] = "// note"
		case 2:
			lines[i] = "func f() {}"
		default:
			lines[i] = `s := "text"`
		}
	}
	b := buf(t, lines...)
	c := highlight.New(lex)

	for i := 0; i < b.NumLines(); i++ {
		c.Spans(b, i)
	}
	// Back to the top, well past any eviction.
	for _, ln := range []int{0, 1, 2, 3, 600, 1199} {
		if got, want := c.Spans(b, ln), reference(lex, b, ln); !sameSpans(got, want) {
			t.Errorf("line %d after eviction\n  got  %v\n  want %v", ln, got, want)
		}
	}
}

// A buffer that shrinks must not leave the cache indexing lines that are gone.
func TestBufferShrinkingDoesNotStrandState(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "/*", "a", "b", "c", "*/", "var x = 1")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	if err := b.Delete(text.Pos{}, text.Pos{Line: 5, Col: 2}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	agrees(t, c, lex, b, "after deleting most of the buffer")

	if err := b.Delete(text.Pos{}, b.End()); err != nil {
		t.Fatalf("delete rest: %v", err)
	}
	agrees(t, c, lex, b, "after emptying it")
}

func TestNilLexerFallsBackToPlain(t *testing.T) {
	b := buf(t, "package main")
	c := highlight.New(nil)
	if _, ok := c.Lexer().(syntax.PlainLexer); !ok {
		t.Errorf("Lexer() = %T, want PlainLexer for a nil lexer", c.Lexer())
	}
	if got := c.Spans(b, 0); len(got) != 1 || got[0].Class != syntax.Plain {
		t.Errorf("Spans = %v, want one Plain span", got)
	}

	c.SetLexer(nil)
	if _, ok := c.Lexer().(syntax.PlainLexer); !ok {
		t.Errorf("after SetLexer(nil) Lexer() = %T, want PlainLexer", c.Lexer())
	}
}

// Invalidating mid-file must recolour from there without disturbing what is
// above it.
func TestInvalidateMidFile(t *testing.T) {
	lex := syntax.For("x.go")
	b := buf(t, "package main", "var a = 1", "var b = 2", "var c = 3")
	c := highlight.New(lex)
	agrees(t, c, lex, b, "before")

	c.Invalidate(2)
	agrees(t, c, lex, b, "after invalidating from line 2")
}

// Typing a run of characters is the commonest thing that happens to an editor,
// and each keystroke must cost about one line of lexing however deep in the
// file it lands.
func TestTypingDeepInAFileCostsOneLine(t *testing.T) {
	lex := syntax.For("x.go")
	lines := make([]string, 2000)
	for i := range lines {
		lines[i] = "\tvalue := 1"
	}
	b := buf(t, lines...)

	counted := &countingLexer{Lexer: lex}
	c := highlight.New(counted)
	warm(c, b, 1500)

	for i, r := range []rune("hello") {
		counted.n = 0
		if err := b.Insert(text.Pos{Line: 1500, Col: text.RuneIdx(i)}, []rune{r}); err != nil {
			t.Fatalf("insert: %v", err)
		}
		// Redraw a screenful around the edit, as the editor would.
		for ln := 1480; ln < 1520; ln++ {
			c.Spans(b, ln)
		}
		if counted.n > 4 {
			t.Errorf("keystroke %d re-lexed %d lines, want about one", i, counted.n)
		}
	}
}

// warm renders the screenful around line, as scrolling to it would.
//
// It deliberately does not render the whole file: the span cache is bounded, so
// walking every line of a large buffer evicts the very lines the caller is
// about to look at, and the measurement would then be of eviction rather than
// of the cache.
func warm(c *highlight.Cache, b *text.Buffer, line int) {
	lo, hi := line-20, line+20
	if lo < 0 {
		lo = 0
	}
	if hi > b.NumLines() {
		hi = b.NumLines()
	}
	for i := lo; i < hi; i++ {
		c.Spans(b, i)
	}
}

// goSource builds a realistic Go file of n lines.
func goSource(n int) []string {
	lines := make([]string, 0, n)
	lines = append(lines, "package main", "", `import "fmt"`, "")
	for len(lines) < n {
		lines = append(lines,
			"// describes the thing",
			"func handle(name string, count int) error {",
			"\tif count == 0 {",
			`\t\treturn fmt.Errorf("empty %s", name)`,
			"\t}",
			"\t/* a block",
			"\t   comment */",
			"\ts := `raw",
			"\tstring`",
			"\treturn nil",
			"}",
			"",
		)
	}
	return lines[:n]
}

func benchBuffer(b *testing.B, lines []string) *text.Buffer {
	b.Helper()
	buf := text.NewBuffer()
	if err := buf.Insert(text.Pos{}, []rune(strings.Join(lines, "\n"))); err != nil {
		b.Fatalf("seed: %v", err)
	}
	buf.BreakUndo()
	return buf
}

// BenchmarkNaiveFromTheTop is the baseline: what colouring one line near the
// bottom costs with no cache at all. It is what the editor would pay on every
// keystroke.
func BenchmarkNaiveFromTheTop(b *testing.B) {
	lex := syntax.For("x.go")
	buf := benchBuffer(b, goSource(10000))
	line := 9500

	b.ReportAllocs()
	for b.Loop() {
		var st syntax.State
		for i := 0; i < line; i++ {
			_, st = lex.Lex(buf.Line(i).Runes(), st)
		}
		_, _ = lex.Lex(buf.Line(line).Runes(), st)
	}
}

// BenchmarkColdCache is opening a file and rendering a screenful at the bottom:
// the states above still have to be computed once.
func BenchmarkColdCache(b *testing.B) {
	lex := syntax.For("x.go")
	lines := goSource(10000)

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		buf := benchBuffer(b, lines)
		c := highlight.New(lex)
		b.StartTimer()

		for ln := 9480; ln < 9520; ln++ {
			c.Spans(buf, ln)
		}
	}
}

// BenchmarkWarmKeystroke is the case that motivated the package: one character
// typed near the bottom of a large file, then a screenful redrawn.
func BenchmarkWarmKeystroke(b *testing.B) {
	lex := syntax.For("x.go")
	buf := benchBuffer(b, goSource(10000))
	c := highlight.New(lex)
	warm(c, buf, 9500)

	// Insert and delete alternately so the edited line stays a realistic
	// length. Only inserting would grow it by one character per iteration until
	// the benchmark was measuring a line thousands of characters wide.
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		var err error
		if i%2 == 0 {
			err = buf.Insert(text.Pos{Line: 9500, Col: 0}, []rune("x"))
		} else {
			err = buf.Delete(text.Pos{Line: 9500, Col: 0}, text.Pos{Line: 9500, Col: 1})
		}
		if err != nil {
			b.Fatalf("edit: %v", err)
		}
		i++
		for ln := 9480; ln < 9520; ln++ {
			c.Spans(buf, ln)
		}
	}
}
