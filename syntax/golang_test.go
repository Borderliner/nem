package syntax

import (
	"strings"
	"testing"
)

// lexDoc lexes a whole document, carrying state, and returns the lines with
// their spans. Multi-line constructs can only be tested this way.
func lexDoc(lx Lexer, doc string) ([][]rune, [][]Span) {
	lines := splitLines(doc)
	spans := make([][]Span, len(lines))
	var st State
	for i, l := range lines {
		spans[i], st = lx.Lex(l, st)
	}
	return lines, spans
}

// assertSpanCovers requires a span to cover exactly the given substring, which
// catches a boundary that is off by one in a way classAt would not.
func assertSpanCovers(t *testing.T, lx Lexer, src, sub string, want Class) {
	t.Helper()
	line := []rune(src)
	idx := indexOnce(t, src, sub)
	start := len([]rune(src[:idx]))
	end := start + len([]rune(sub))
	spans, _ := lx.Lex(line, 0)
	checkSpans(t, src, line, spans)
	for _, s := range spans {
		if s.Start == start && s.End == end && s.Class == want {
			return
		}
	}
	t.Errorf("in %q, no %v span covers exactly %q; got %s", src, want, sub, describe(line, spans))
}

func TestGoKeywordsAndIdentifiersThatMerelyStartLikeOne(t *testing.T) {
	lx := goLexer{}
	for _, tc := range []struct {
		src, sub string
		want     Class
	}{
		{"for i := range xs {", "for", Keyword},
		{"format := 1", "format", Plain},              // not "for"
		{"var iffy int", "iffy", Plain},               // not "if"
		{"funcs := map[string]int{}", "funcs", Plain}, // not "func"
		{"return nil", "return", Keyword},
		{"x := true", "true", Constant},
		{"y := iota", "iota", Constant},
		{"var s string", "string", Type},
		{"b := []byte(s)", "byte", Type}, // a conversion is a type, not a call
	} {
		assertClass(t, lx, tc.src, tc.sub, tc.want)
	}
}

func TestGoFunctionPositions(t *testing.T) {
	lx := goLexer{}
	for _, tc := range []struct{ src, sub string }{
		{"func main() {", "main"},
		{"func (e *Editor) Redraw() {", "Redraw"},
		{"fmt.Println(x)", "Println"},
		{"go handle(conn)", "handle"},
	} {
		assertClass(t, lx, tc.src, tc.sub, Function)
	}
	// A bare identifier is not a call.
	assertClass(t, lx, "x := y", "y", Plain)
	// The receiver in a method declaration is not the method.
	assertClass(t, lx, "func (rcv *Editor) Redraw() {", "rcv", Plain)
	// A keyword before a paren is still a keyword.
	assertClass(t, lx, "if (x) {", "if", Keyword)
}

func TestGoTypeAfterTypeKeyword(t *testing.T) {
	lx := goLexer{}
	assertClass(t, lx, "type Editor struct {", "Editor", Type)
	assertClass(t, lx, "type   Spaced  int", "Spaced", Type)
	assertClass(t, lx, "type Editor struct {", "type", Keyword)
}

func TestGoStringsAndRunes(t *testing.T) {
	lx := goLexer{}
	assertSpanCovers(t, lx, `s := "hello"`, `"hello"`, String)
	assertSpanCovers(t, lx, `s := "a\"b"`, `"a\"b"`, String) // escaped quote does not end it
	assertSpanCovers(t, lx, `r := '\''`, `'\''`, String)     // the awkward rune literal
	assertSpanCovers(t, lx, `r := '\n'`, `'\n'`, String)
	assertSpanCovers(t, lx, `s := "tab\there"`, `"tab\there"`, String)
	// A backslash at the very end must not read past the line.
	line := []rune(`s := "trailing\`)
	spans, _ := lx.Lex(line, 0)
	checkSpans(t, "trailing backslash", line, spans)
}

func TestGoNumbers(t *testing.T) {
	lx := goLexer{}
	for _, lit := range []string{
		"42", "0", "1_000_000", "0x1f", "0xDEAD_beef", "0b1010", "0o777",
		"3.14", "1e9", "1E-9", "6.02e23", "1.5i", "2i", "0x1.8p3", ".5",
	} {
		assertSpanCovers(t, lx, "x := "+lit+" + y", lit, Number)
	}
	// An identifier containing digits is not a number.
	assertClass(t, lx, "x1e9 := 2", "x1e9", Plain)
	// A minus is an operator, not part of the literal.
	assertSpanCovers(t, lx, "x := 1-2", "-", Operator)
}

func TestGoLineComment(t *testing.T) {
	lx := goLexer{}
	assertSpanCovers(t, lx, `x := 1 // trailing`, `// trailing`, Comment)
	// A // inside a string is not a comment.
	assertSpanCovers(t, lx, `u := "http://x"`, `"http://x"`, String)
}

func TestGoBlockCommentSpansLines(t *testing.T) {
	_, spans := lexDoc(goLexer{}, "/* open\n   still\n   here */ x := 1\n")
	if classAt(spans[0], 0) != Comment {
		t.Error("line 0 should open a comment")
	}
	if classAt(spans[1], 3) != Comment {
		t.Error("line 1 is inside the block comment and should be Comment")
	}
	if classAt(spans[2], 3) != Comment {
		t.Error("line 2 up to the close should be Comment")
	}
	// After the close, code resumes.
	lines, _ := lexDoc(goLexer{}, "/* open\n   still\n   here */ x := 1\n")
	last := lines[2]
	idx := strings.Index(string(last), "x :=")
	if classAt(spans[2], len([]rune(string(last)[:idx]))) == Comment {
		t.Error("code after the comment close is still Comment")
	}
}

func TestGoRawStringSpansLines(t *testing.T) {
	_, spans := lexDoc(goLexer{}, "s := `raw\nstill raw\nend` + x\n")
	if classAt(spans[0], 6) != String {
		t.Error("the backtick should open a string")
	}
	if classAt(spans[1], 0) != String {
		t.Error("line 1 is inside the raw string")
	}
	if classAt(spans[2], 0) != String {
		t.Error("line 2 up to the close is still the string")
	}
	if classAt(spans[2], 6) == String {
		t.Error("the + after the raw string closed is not part of it")
	}
}

// A // inside a raw string does not start a comment, and a ` inside a line
// comment does not open a string. Both are the kind of interaction that a
// lexer handling each construct separately gets wrong.
func TestGoConstructsDoNotLeakIntoEachOther(t *testing.T) {
	lx := goLexer{}
	spans, out := lx.Lex([]rune("s := `a // b`"), 0)
	if out != 0 {
		t.Errorf("a closed raw string left state %v", out)
	}
	if classAt(spans, 10) != String {
		t.Error("// inside a raw string should stay String")
	}

	_, out2 := lx.Lex([]rune("x := 1 // a ` backtick"), 0)
	if out2 != 0 {
		t.Errorf("a backtick inside a line comment opened a raw string: state %v", out2)
	}

	_, out3 := lx.Lex([]rune(`x := 1 // a /* comment`), 0)
	if out3 != 0 {
		t.Errorf("/* inside a line comment opened a block comment: state %v", out3)
	}
}
