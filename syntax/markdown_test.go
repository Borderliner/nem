package syntax

import "testing"

func TestMarkdownHeadings(t *testing.T) {
	lx := markdownLexer{}
	for _, src := range []string{"# One", "## Two", "###### Six", "   # Indented"} {
		line := []rune(src)
		spans, _ := lx.Lex(line, 0)
		if classAt(spans, len(line)-1) != Keyword {
			t.Errorf("%q should be a heading, got %s", src, describe(line, spans))
		}
	}
	// Seven hashes is not a heading, and neither is a hash with no space.
	for _, src := range []string{"####### Seven", "#NoSpace"} {
		line := []rune(src)
		spans, _ := lx.Lex(line, 0)
		if classAt(spans, 0) == Keyword {
			t.Errorf("%q should not be a heading", src)
		}
	}
}

func TestMarkdownFencedCodeSpansLines(t *testing.T) {
	_, spans := lexDoc(markdownLexer{}, "before\n```go\nfunc x() {}\n```\nafter\n")
	if classAt(spans[0], 0) == String {
		t.Error("text before the fence is not code")
	}
	for _, i := range []int{1, 2, 3} {
		if classAt(spans[i], 0) != String {
			t.Errorf("line %d should be part of the fenced block", i)
		}
	}
	if classAt(spans[4], 0) == String {
		t.Error("text after the closing fence is not code")
	}
}

// A fence is closed only by its own delimiter, at least as long. Getting this
// wrong recolours the remainder of the document, which is the single most
// visible mistake a Markdown highlighter can make.
func TestMarkdownFenceClosingRules(t *testing.T) {
	// A tilde fence does not close a backtick fence.
	_, spans := lexDoc(markdownLexer{}, "```\ncode\n~~~\nstill code\n```\nout\n")
	if classAt(spans[3], 0) != String {
		t.Error("~~~ must not close a ``` fence")
	}
	if classAt(spans[5], 0) == String {
		t.Error("``` should have closed the fence")
	}

	// And the reverse direction, which is the half that actually pins the
	// delimiter: a backtick run must not close a tilde fence. Testing only the
	// first direction passes even if the delimiter is ignored entirely, because
	// a tilde line contains no backticks either way.
	_, spansT := lexDoc(markdownLexer{}, "~~~\ncode\n```\nstill code\n~~~\nout\n")
	if classAt(spansT[3], 0) != String {
		t.Error("``` must not close a ~~~ fence")
	}
	if classAt(spansT[5], 0) == String {
		t.Error("~~~ should have closed the fence")
	}

	// A shorter run does not close a longer fence.
	_, spans2 := lexDoc(markdownLexer{}, "````\ncode\n```\nstill code\n````\nout\n")
	if classAt(spans2[3], 0) != String {
		t.Error("``` must not close a ```` fence")
	}
	if classAt(spans2[5], 0) == String {
		t.Error("```` should have closed the fence")
	}

	// A line of code that merely begins with backticks does not close it.
	_, spans3 := lexDoc(markdownLexer{}, "```\n``` and more text\nstill code\n```\nout\n")
	if classAt(spans3[2], 0) != String {
		t.Error("a fence line with trailing text must not close the block")
	}
}

func TestMarkdownInlineCode(t *testing.T) {
	lx := markdownLexer{}
	assertSpanCovers(t, lx, "use `code` here", "`code`", String)
	assertSpanCovers(t, lx, "a ``tick ` inside`` b", "``tick ` inside``", String)
}

func TestMarkdownEmphasisAndLinks(t *testing.T) {
	lx := markdownLexer{}
	assertSpanCovers(t, lx, "an *italic* word", "*italic*", Constant)
	assertSpanCovers(t, lx, "an _italic_ word", "_italic_", Constant)
	assertSpanCovers(t, lx, "a **bold** word", "**bold**", Constant)
	assertSpanCovers(t, lx, "see [the docs](http://x) now", "[the docs]", Function)
	assertSpanCovers(t, lx, "see [the docs](http://x) now", "(http://x)", String)
	// Spaced asterisks are multiplication, not emphasis.
	line := []rune("a * b * c")
	spans, _ := lx.Lex(line, 0)
	if classAt(spans, 2) == Constant {
		t.Errorf("spaced asterisks read as emphasis: %s", describe(line, spans))
	}
}

// A bullet is consumed before inline scanning, so the * that starts a list item
// is not read as the opening of emphasis.
func TestMarkdownListMarkersAreNotEmphasis(t *testing.T) {
	lx := markdownLexer{}
	for _, src := range []string{"* item", "- item", "+ item", "1. item", "2) item"} {
		line := []rune(src)
		spans, _ := lx.Lex(line, 0)
		checkSpans(t, src, line, spans)
		if classAt(spans, 0) != Punctuation {
			t.Errorf("%q: marker should be Punctuation, got %s", src, describe(line, spans))
		}
	}
}

func TestMarkdownBlockquote(t *testing.T) {
	line := []rune("> quoted text")
	spans, _ := markdownLexer{}.Lex(line, 0)
	if classAt(spans, 0) != Comment {
		t.Errorf("a blockquote should read as an aside, got %s", describe(line, spans))
	}
}
