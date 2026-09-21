package syntax

import (
	"math/rand"
	"strings"
	"testing"
)

// allLexers is every lexer the package offers. Each invariant below runs
// against all of them, because an invariant that holds only for the lexer its
// author was thinking about is not an invariant.
func allLexers() []Lexer {
	return []Lexer{PlainLexer{}, goLexer{}, luaLexer{}, jsonLexer{}, markdownLexer{}}
}

// checkSpans asserts the structural promises the Lexer interface makes. A
// renderer maps spans straight onto cells, so a violation here is corruption on
// screen rather than a wrong colour.
func checkSpans(t *testing.T, where string, line []rune, spans []Span) {
	t.Helper()
	prev := 0
	for i, s := range spans {
		if s.Start >= s.End {
			t.Errorf("%s: span %d is empty: %+v", where, i, s)
		}
		if s.Start < 0 || s.End > len(line) {
			t.Errorf("%s: span %d %+v out of range for a %d-rune line", where, i, s, len(line))
		}
		if s.Start < prev {
			t.Errorf("%s: span %d %+v overlaps or precedes the previous (ends at %d)", where, i, s, prev)
		}
		prev = s.End
	}
}

// A document lexed top to bottom, then each line re-lexed on its own with the
// state recorded for it, must give identical answers.
//
// This is the invariant the caching layer rests on: after an edit it re-lexes
// downward from the change and stops once an outgoing state matches the cached
// one. If Lex could depend on anything but (line, in) - a field on the
// receiver, a previous call - that early stop would leave stale colours below,
// and the bug would appear only after a particular sequence of edits.
func TestLexingIsPureAndResumable(t *testing.T) {
	docs := map[string]string{
		"go":       "package main\n\n/* a block\n   comment */\nfunc main() {\n\ts := `raw\nstring`\n\tprintln(s, 42)\n}\n",
		"lua":      "-- a comment\nlocal x = [==[\nlong string\n]==]\nfunction f(a)\n  return a .. \"hi\"\nend\n",
		"json":     "{\n  \"a\": 1,\n  \"b\": [true, null],\n  \"c\": \"text\"\n}\n",
		"markdown": "# Title\n\nSome *text* and `code`.\n\n```go\nfunc x() {}\n```\n\n> quote\n",
	}
	for _, lx := range allLexers() {
		doc, ok := docs[lx.Name()]
		if !ok {
			doc = docs["go"] // the plain lexer does not care what it is given
		}
		lines := splitLines(doc)

		// Full pass, recording the state each line started from.
		ins := make([]State, len(lines))
		outs := make([]State, len(lines))
		full := make([][]Span, len(lines))
		var st State
		for i, l := range lines {
			ins[i] = st
			full[i], st = lx.Lex(l, st)
			outs[i] = st
			checkSpans(t, lx.Name(), l, full[i])
		}

		// Each line on its own, from its recorded incoming state.
		for i, l := range lines {
			spans, out := lx.Lex(l, ins[i])
			if out != outs[i] {
				t.Errorf("%s line %d: resumed state %v, full-pass state %v", lx.Name(), i, out, outs[i])
			}
			if !sameSpans(spans, full[i]) {
				t.Errorf("%s line %d (%q): resumed spans %v, full-pass spans %v",
					lx.Name(), i, string(l), spans, full[i])
			}
		}
	}
}

// Re-lexing from an edit must converge: once an outgoing state matches what was
// cached, every line below is provably unchanged. This is the stopping rule, so
// it is worth pinning rather than assuming.
func TestReLexConvergesOnTheCachedState(t *testing.T) {
	lx := goLexer{}
	lines := splitLines("package main\n\nfunc a() {}\n\nfunc b() {}\n\nfunc c() {}\n")

	var st State
	outs := make([]State, len(lines))
	for i, l := range lines {
		_, st = lx.Lex(l, st)
		outs[i] = st
	}

	// Edit line 2 in a way that opens nothing. Re-lexing should converge at once.
	edited := []rune("func a() { println(1) }")
	_, out := lx.Lex(edited, outs[1])
	if out != outs[2] {
		t.Fatalf("an edit that opens nothing changed the outgoing state: %v vs %v", out, outs[2])
	}
}

// An unterminated construct is the normal state of a file being typed. None may
// hang, panic, or run off the end of the line.
func TestUnterminatedConstructsAreSafe(t *testing.T) {
	cases := []struct {
		lexer Lexer
		lines []string
	}{
		{goLexer{}, []string{`s := "unclosed`, "/* open forever", "r := `raw and open", `c := 'x`, "`", `"`, "'"}},
		{luaLexer{}, []string{"s = [==[ open", "--[[ open comment", `s = "unclosed`, "s = 'unclosed", "[=", "--[=", "]="}},
		{jsonLexer{}, []string{`{"a": "unclosed`, `{"a":`, `[`, `"`}},
		{markdownLexer{}, []string{"```go", "`inline unclosed", "# ", "```", "> ", "["}},
	}
	for _, c := range cases {
		for _, s := range c.lines {
			line := []rune(s)
			spans, _ := c.lexer.Lex(line, 0)
			checkSpans(t, c.lexer.Name()+" "+s, line, spans)
		}
	}
}

// Spans are rune indices. A byte offset survives an ASCII test suite untouched
// and then colours the wrong half of a line the first time someone writes a
// comment in Japanese - the text still reads correctly, so nothing looks broken
// except the colours.
func TestSpansAreRuneIndicesNotByteOffsets(t *testing.T) {
	for _, tc := range []struct {
		lexer Lexer
		line  string
	}{
		{goLexer{}, `s := "日本語のテキスト" // コメント`},
		{goLexer{}, `s := "🙂🚀" // emoji`},
		{goLexer{}, "s := \"café\" // decomposed"},
		{luaLexer{}, `s = "日本語" -- コメント`},
		{jsonLexer{}, `{"キー": "値"}`},
		{markdownLexer{}, "# 見出し `コード`"},
	} {
		line := []rune(tc.line)
		spans, _ := tc.lexer.Lex(line, 0)
		checkSpans(t, tc.lexer.Name()+" "+tc.line, line, spans)
		// A byte-offset bug shows up as an end past the rune count, which
		// checkSpans catches, but assert the tail is reachable too: the last
		// span must not claim more runes than exist.
		if n := len(spans); n > 0 && spans[n-1].End > len(line) {
			t.Errorf("%s: last span ends at %d, line has %d runes", tc.line, spans[n-1].End, len(line))
		}
	}
}

// Randomised input must not break the structural promises. Generated text is
// mostly nonsense, which is the point: it reaches delimiter combinations no
// hand-written case would think of.
func TestRandomInputKeepsTheInvariants(t *testing.T) {
	const alphabet = "ab{}[]()\"'`\\/*-=#:,.01 \t\n日🙂_"
	rng := rand.New(rand.NewSource(7))
	for _, lx := range allLexers() {
		var st State
		for i := 0; i < 400; i++ {
			n := rng.Intn(24)
			var sb strings.Builder
			for j := 0; j < n; j++ {
				r := []rune(alphabet)[rng.Intn(len([]rune(alphabet)))]
				if r == '\n' {
					r = ' '
				}
				sb.WriteRune(r)
			}
			line := []rune(sb.String())
			var spans []Span
			spans, st = lx.Lex(line, st)
			checkSpans(t, lx.Name()+" random", line, spans)
		}
	}
}

func TestForPicksTheLexerByExtension(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"main.go", "go"},
		{"init.lua", "lua"},
		{"a.json", "json"},
		{"README.md", "markdown"},
		{"NOTES.markdown", "markdown"},
		{"MAIN.GO", "go"},
		{"noext", "text"},
		{"archive.tar.gz", "text"},
		{"", "text"},
	} {
		if got := For(tc.path).Name(); got != tc.want {
			t.Errorf("For(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestForNeverReturnsNil(t *testing.T) {
	for _, p := range []string{"", "x", "x.unknown", "/", "."} {
		if For(p) == nil {
			t.Errorf("For(%q) returned nil; callers do not check", p)
		}
	}
}

func TestPlainLexerCoversTheLine(t *testing.T) {
	line := []rune("anything at all")
	spans, out := PlainLexer{}.Lex(line, 0)
	if out != 0 {
		t.Errorf("plain lexer carried state %v", out)
	}
	if len(spans) != 1 || spans[0] != (Span{0, len(line), Plain}) {
		t.Errorf("spans = %v, want one Plain span over the whole line", spans)
	}
	if spans, _ := (PlainLexer{}).Lex(nil, 0); len(spans) != 0 {
		t.Errorf("empty line produced %v, want no spans - an empty span is not allowed", spans)
	}
}

// splitLines splits a document into rune lines, dropping the trailing empty
// piece so a document ending in a newline does not gain a phantom line.
func splitLines(doc string) [][]rune {
	parts := strings.Split(doc, "\n")
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out
}

func sameSpans(a, b []Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// classAt reports the class covering a rune index, for readable assertions in
// the per-language tests. A position no span covers is Plain.
func classAt(spans []Span, i int) Class {
	for _, s := range spans {
		if i >= s.Start && i < s.End {
			return s.Class
		}
	}
	return Plain
}

// spanText is the source text a span covers, so a failure names what was
// misclassified rather than a pair of offsets.
func spanText(line []rune, s Span) string { return string(line[s.Start:s.End]) }

// assertClass checks that the given substring of a line is classified as want.
func assertClass(t *testing.T, lx Lexer, src, sub string, want Class) {
	t.Helper()
	line := []rune(src)
	idx := indexOnce(t, src, sub)
	at := len([]rune(src[:idx]))
	spans, _ := lx.Lex(line, 0)
	checkSpans(t, src, line, spans)
	if got := classAt(spans, at); got != want {
		t.Errorf("in %q, %q is %v, want %v (spans: %s)", src, sub, got, want, describe(line, spans))
	}
}

func describe(line []rune, spans []Span) string {
	var sb strings.Builder
	for _, s := range spans {
		sb.WriteString(s.Class.String())
		sb.WriteString("(")
		sb.WriteString(spanText(line, s))
		sb.WriteString(") ")
	}
	return sb.String()
}

// indexOnce locates sub in src and fails if it appears more than once.
//
// strings.Index silently returns the first hit, so asserting on "f" in
// "function f(a)" tests the f in "function" and reports the lexer as broken
// when it is correct. Requiring uniqueness turns that into a test-authoring
// error with a message saying so.
func indexOnce(t *testing.T, src, sub string) int {
	t.Helper()
	i := strings.Index(src, sub)
	if i < 0 {
		t.Fatalf("%q does not contain %q", src, sub)
	}
	if strings.Contains(src[i+len(sub):], sub) {
		t.Fatalf("%q appears more than once in %q; pick a distinctive substring "+
			"or the assertion tests the wrong occurrence", sub, src)
	}
	return i
}

// Class.String names classes for test failures and for debugging. It is
// exercised only on a failing path, so it needs a test of its own or a typo in
// it survives until the day someone is already debugging something else.
func TestClassString(t *testing.T) {
	seen := map[string]bool{}
	for c := Plain; c <= Punctuation; c++ {
		s := c.String()
		if s == "" || s == "Class(?)" {
			t.Errorf("class %d has no name", c)
		}
		if seen[s] {
			t.Errorf("two classes both name themselves %q", s)
		}
		seen[s] = true
	}
	if got := Class(200).String(); got != "Class(?)" {
		t.Errorf("unknown class named %q, want the fallback", got)
	}
}
