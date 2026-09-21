package nanorc

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/syntax"
)

func lexerFor(t *testing.T, body string) Lexer {
	t.Helper()
	s, skips, err := Parse("t.nanorc", strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(skips) != 0 {
		t.Fatalf("unexpected skips: %v", skips)
	}
	return Lexer{s: s}
}

// classAt reports the class covering a rune index, for readable assertions.
func classAt(spans []syntax.Span, i int) syntax.Class {
	for _, s := range spans {
		if i >= s.Start && i < s.End {
			return s.Class
		}
	}
	return syntax.Plain
}

// checkContract asserts everything syntax.Lexer promises about spans. Every
// lexer test runs it, because a renderer fed overlapping or out-of-range spans
// corrupts the screen in ways that look like a font problem.
func checkContract(t *testing.T, line []rune, spans []syntax.Span) {
	t.Helper()
	prev := -1
	for i, s := range spans {
		if s.Start < 0 || s.End > len(line) {
			t.Errorf("span %d %v outside line of %d runes", i, s, len(line))
		}
		if s.End <= s.Start {
			t.Errorf("span %d %v is empty", i, s)
		}
		if s.Start < prev {
			t.Errorf("span %d %v overlaps or precedes the one before", i, s)
		}
		prev = s.End
	}
}

// nano applies rules in file order and each paints over the last.
//
// The two rules here overlap AND classify differently, which is the only way
// the order is observable: two rules of the same class paint the same colour
// whichever wins, so a test built from those passes with precedence reversed.
func TestLaterRulesPaintOverEarlier(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color blue "[[:digit:]]+"
color red "#.*"
`)
	line := []rune("x #123 y")
	spans, _ := lex.Lex(line, 0)
	checkContract(t, line, spans)

	// The comment rule is listed second and covers the digits, so it wins
	// there. With the order reversed the digits would come back as Number.
	for i := 2; i < len(line); i++ {
		if got := classAt(spans, i); got != syntax.Comment {
			t.Errorf("rune %d (%q) = %v, want Comment - the later rule must paint over the earlier",
				i, string(line[i]), got)
		}
	}
	if got := classAt(spans, 0); got != syntax.Plain {
		t.Errorf("rune 0 = %v, want Plain", got)
	}
}

// And the reverse ordering must give the opposite answer, so the test above is
// measuring order rather than something incidental about these two patterns.
func TestEarlierRuleKeepsWhatLaterRulesDoNotCover(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color red "#.*"
color blue "[[:digit:]]+"
`)
	line := []rune("x #123 y")
	spans, _ := lex.Lex(line, 0)
	checkContract(t, line, spans)

	if got := classAt(spans, 3); got != syntax.Number {
		t.Errorf("digits = %v, want Number now that its rule is listed last", got)
	}
	if got := classAt(spans, 2); got != syntax.Comment {
		t.Errorf("the # itself = %v, want Comment - the digit rule does not cover it", got)
	}
}

func TestOverlappingRulesProduceCleanSpans(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color green "\<(alpha|beta|gamma)\>"
color blue "[[:digit:]]+"
color red "#.*$"
`)
	line := []rune("alpha 123 beta # trailing")
	spans, _ := lex.Lex(line, 0)
	checkContract(t, line, spans)

	if got := classAt(spans, 0); got != syntax.Keyword {
		t.Errorf("alpha = %v, want Keyword", got)
	}
	if got := classAt(spans, 6); got != syntax.Number {
		t.Errorf("123 = %v, want Number", got)
	}
	if got := classAt(spans, 15); got != syntax.Comment {
		t.Errorf("# comment = %v, want Comment", got)
	}
}

// A block comment spans lines, and the state the lexer returns is what carries
// it. Losing that state ends the comment at the line break and miscolours
// everything below, which is the whole reason State exists.
func TestMultilineStateCarriesAcrossLines(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color blue start="/\*" end="\*/"
color green "\<(code)\>"
`)
	lines := []string{
		"code /* open",
		"still inside code",
		"closing */ code",
		"code again",
	}
	var st syntax.State
	got := make([][]syntax.Span, len(lines))
	for i, s := range lines {
		line := []rune(s)
		got[i], st = lex.Lex(line, st)
		checkContract(t, line, got[i])
	}

	if c := classAt(got[0], 0); c != syntax.Keyword {
		t.Errorf("line 0 'code' = %v, want Keyword", c)
	}
	if c := classAt(got[0], 6); c != syntax.Comment {
		t.Errorf("line 0 '/*' = %v, want Comment", c)
	}
	// The middle line is entirely inside the comment, including the word the
	// keyword rule would otherwise claim.
	if c := classAt(got[1], 0); c != syntax.Comment {
		t.Errorf("line 1 start = %v, want Comment", c)
	}
	if c := classAt(got[1], 13); c != syntax.Comment {
		t.Errorf("line 1 'code' inside the comment = %v, want Comment", c)
	}
	if c := classAt(got[2], 0); c != syntax.Comment {
		t.Errorf("line 2 up to the close = %v, want Comment", c)
	}
	if c := classAt(got[2], 11); c != syntax.Keyword {
		t.Errorf("line 2 after the close = %v, want Keyword", c)
	}
	if c := classAt(got[3], 0); c != syntax.Keyword {
		t.Errorf("line 3 = %v, want Keyword once the comment has closed", c)
	}
}

// Equal situations must produce equal states, or the highlight cache never
// converges and re-lexes to the bottom of the file on every keystroke.
func TestStateIsStableForEqualSituations(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color blue start="/\*" end="\*/"
`)
	_, a := lex.Lex([]rune("x /* open"), 0)
	_, b := lex.Lex([]rune("y /* open"), 0)
	if a != b {
		t.Errorf("states differ for equivalent lines: %v vs %v", a, b)
	}
	if a == 0 {
		t.Error("an unterminated block comment should leave a non-zero state")
	}
	_, closed := lex.Lex([]rune("*/"), a)
	if closed != 0 {
		t.Errorf("state after closing = %v, want zero", closed)
	}
}

// An empty line cannot close a block comment.
func TestEmptyLineKeepsStateOpen(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color blue start="/\*" end="\*/"
`)
	_, open := lex.Lex([]rune("/* start"), 0)
	spans, after := lex.Lex(nil, open)
	if len(spans) != 0 {
		t.Errorf("spans on an empty line = %v, want none", spans)
	}
	if after != open {
		t.Errorf("state = %v, want it unchanged at %v", after, open)
	}
}

// Spans are rune indices, so a line with multi-byte runes must not shift them.
func TestSpansAreRuneIndicesNotBytes(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color green "\<(end)\>"
`)
	line := []rune("日本語 end")
	spans, _ := lex.Lex(line, 0)
	checkContract(t, line, spans)
	if got := classAt(spans, 4); got != syntax.Keyword {
		t.Errorf("class at rune 4 = %v, want Keyword - byte offsets would land elsewhere", got)
	}
}

func TestUnterminatedStartRunsToEndOfLine(t *testing.T) {
	lex := lexerFor(t, `
syntax demo "\.demo$"
color blue start="/\*" end="\*/"
`)
	line := []rune("a /* b")
	spans, st := lex.Lex(line, 0)
	checkContract(t, line, spans)
	if classAt(spans, 5) != syntax.Comment {
		t.Error("text after an unterminated opener should be inside the comment")
	}
	if st == 0 {
		t.Error("want a carried state")
	}
}
