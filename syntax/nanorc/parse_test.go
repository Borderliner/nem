package nanorc

import (
	"strings"
	"testing"

	"github.com/Borderliner/nem/syntax"
)

func parseString(t *testing.T, body string) (*Syntax, []error) {
	t.Helper()
	s, skips, err := Parse("test.nanorc", strings.NewReader(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s, skips
}

func TestParseDirectives(t *testing.T) {
	s, skips := parseString(t, `
# a leading comment
syntax demo "\.demo$" "\.dm$"
header "^#!.*demo"
comment "//"
magic "demo script"
linter demolint
formatter demofmt
tabgives "    "
color green "\<(if|else|while)\>"
`)
	if len(skips) != 0 {
		t.Errorf("unexpected skips: %v", skips)
	}
	if s.Name != "demo" {
		t.Errorf("Name = %q, want demo", s.Name)
	}
	if s.Comment != "//" {
		t.Errorf("Comment = %q, want //", s.Comment)
	}
	if len(s.files) != 2 {
		t.Errorf("file patterns = %d, want 2", len(s.files))
	}
	if len(s.headers) != 1 {
		t.Errorf("header patterns = %d, want 1", len(s.headers))
	}
	if len(s.rules) != 1 {
		t.Errorf("rules = %d, want 1", len(s.rules))
	}
	// magic, linter, formatter and tabgives must be ignored in silence: nem has
	// no use for them and a warning each would be noise on every launch.
}

// nano's quoting is not shell quoting. An argument ends at the first quote
// followed by end-of-line or a blank, which is what lets a pattern contain a
// bare quote inside a character class. Splitting on the first quote truncates a
// quarter of the real corpus.
func TestQuotingFollowsNanoNotShell(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{"bare quote in a class", `color blue "//[^"]*$"`, `//[^"]*$`},
		{"escaped quote", `color blue "\"[^\"]*\""`, `\"[^\"]*\"`},
		{"no terminator runs to end", `color blue "abc`, `abc`},
		{"trailing text after close", `color blue "abc" extra`, `abc`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toks := tokenize(tc.line)
			if len(toks) < 3 {
				t.Fatalf("tokenize gave %d tokens: %#v", len(toks), toks)
			}
			if toks[2].text != tc.want {
				t.Errorf("pattern = %q, want %q", toks[2].text, tc.want)
			}
		})
	}
}

func TestTranslateWordBoundaries(t *testing.T) {
	if got, want := translate(`\<word\>`), `\bword\b`; got != want {
		t.Errorf("translate = %q, want %q", got, want)
	}
}

// POSIX treats a backslash inside a bracket expression as an ordinary
// character; RE2 treats it as an escape. Without translating, [^"\] eats the
// closing bracket and every string-literal rule in the corpus fails.
func TestTranslatePosixBracketSemantics(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`[^"\]`, `[^"\\]`},
		{`["'\abfnrtv]`, `["'\\abfnrtv]`},
		{`[[:alnum:]_]`, `[[:alnum:]_]`},
		{`\\[[:blank:]]*`, `\\[[:blank:]]*`},
		{`[]]`, `[\]]`},
		{`[^]]`, `[^\]]`},
	} {
		if got := translate(tc.in); got != tc.want {
			t.Errorf("translate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslatedPatternsCompile(t *testing.T) {
	// The shapes that failed before the bracket fix, taken from the real files.
	for _, pat := range []string{
		`"([^"\]|\\.)*"`,
		`'([^'\]|\\(["'\abfnrtv]|x[[:xdigit:]]{1,2}))'`,
		`//[^"]*$`,
		`\<(if|else)\>`,
	} {
		if _, err := compile(pat, false); err != nil {
			t.Errorf("compile(%q): %v", pat, err)
		}
	}
}

// One unsupported regex must cost its own rule and nothing else. These files
// are third party and nem does not control their correctness.
func TestBadPatternIsSkippedNotFatal(t *testing.T) {
	s, skips := parseString(t, `
syntax demo "\.demo$"
color green "\<(good)\>"
color red "@(unbalanced|paren)|extra)"
color blue "\<(alsogood)\>"
`)
	if len(skips) != 1 {
		t.Errorf("skips = %d, want 1: %v", len(skips), skips)
	}
	if len(s.rules) != 2 {
		t.Errorf("rules = %d, want the two good ones kept", len(s.rules))
	}
}

func TestSyntaxDirectiveRequired(t *testing.T) {
	if _, _, err := Parse("x.nanorc", strings.NewReader("color green \"x\"\n")); err == nil {
		t.Error("a file with no syntax directive should be an error")
	}
}

func TestMultilineRuleParses(t *testing.T) {
	s, skips := parseString(t, `
syntax demo "\.demo$"
color blue start="/\*" end="\*/"
`)
	if len(skips) != 0 {
		t.Fatalf("skips: %v", skips)
	}
	if len(s.rules) != 1 || !s.rules[0].multiline() {
		t.Fatalf("want one multiline rule, got %#v", s.rules)
	}
	if got := s.rules[0].class; got != syntax.Comment {
		t.Errorf("class = %v, want Comment for a /* */ rule", got)
	}
}

func TestClassifyInfersFromPatternNotColour(t *testing.T) {
	// The same colour must be able to mean different things, because in the
	// real corpus it does: strings are red in java.nanorc and green in
	// rust.nanorc.
	for _, tc := range []struct {
		pattern string
		want    syntax.Class
	}{
		{`\<(if|else|while|for)\>`, syntax.Keyword},
		{`"([^"\]|\\.)*"`, syntax.String},
		{`#.*$`, syntax.Comment},
		{`//.*$`, syntax.Comment},
		// Unanchored: the .* already reaches the end of the line, and python's
		// real rule is written this way.
		{`(^|[[:blank:]])#.*`, syntax.Comment},
		{`;.*$`, syntax.Comment},
		// A word list is a keyword list even though it contains punctuation a
		// comment introducer would use.
		{`\<(a|b|c|d)\>`, syntax.Keyword},
		{`\<[0-9]+\>`, syntax.Number},
		{`[[:digit:]]+`, syntax.Number},
	} {
		if got := classify(tc.pattern); got != tc.want {
			t.Errorf("classify(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}
