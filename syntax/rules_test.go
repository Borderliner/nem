package syntax

import (
	"strings"
	"testing"
)

// These tests use assertClass from invariants_test.go, which checks the span
// contract as well as the classification - so every assertion here doubles as a
// check that the rule engine emits well-formed spans.

// lexerNamed returns a bundled lexer by language name, failing if absent.
func lexerNamed(t *testing.T, name string) Lexer {
	t.Helper()
	rs := EmbeddedRuleSet(name)
	if rs == nil {
		t.Fatalf("no bundled language %q; have %v", name, EmbeddedLanguages())
	}
	return rs
}

// Every bundled file must parse and every pattern must compile. These ship
// inside the binary, so a broken rule is a build-time mistake and this is what
// turns it into a red CI run rather than a language quietly losing its colours.
func TestEmbeddedRulesAllLoad(t *testing.T) {
	if err := EmbeddedError(); err != nil {
		t.Fatalf("bundled rules failed to load: %v", err)
	}
	langs := EmbeddedLanguages()
	if len(langs) < 10 {
		t.Errorf("only %d bundled languages (%v); the set has shrunk unexpectedly", len(langs), langs)
	}
	for _, name := range langs {
		rs := EmbeddedRuleSet(name)
		if rs.Rules() == 0 {
			t.Errorf("%s has no rules", name)
		}
	}
}

// A language nem describes itself must not be shadowed, and the four
// hand-written lexers must beat everything.
func TestPrecedenceNativeThenEmbedded(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"main.go", "go"},         // native
		{"init.lua", "lua"},       // native
		{"a.json", "json"},        // native
		{"README.md", "markdown"}, // native
		{"main.c", "c"},           // bundled
		{"run.py", "python"},      // bundled
		{"deploy.sh", "sh"},       // bundled
		{"nothing.xyzzy", "text"}, // neither
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := For(tc.path).Name(); got != tc.want {
				t.Errorf("For(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// A shebang identifies a script with no extension, which is how most of them
// arrive.
func TestHeaderMatchesShebang(t *testing.T) {
	for _, tc := range []struct{ first, want string }{
		{"#!/bin/sh", "sh"},
		{"#!/usr/bin/env bash", "sh"},
		{"#!/usr/bin/env python3", "python"},
		{"just some text", "text"},
	} {
		t.Run(tc.first, func(t *testing.T) {
			if got := ForWithHeader("deploy", tc.first).Name(); got != tc.want {
				t.Errorf("ForWithHeader(deploy, %q) = %q, want %q", tc.first, got, tc.want)
			}
		})
	}
}

func TestCRules(t *testing.T) {
	lex := lexerNamed(t, "c")
	assertClass(t, lex, "static int count = 0;", "static", Keyword)
	assertClass(t, lex, "static int count = 0;", "int", Type)
	assertClass(t, lex, "return NULL;", "NULL", Constant)
	assertClass(t, lex, "x = 0xDEADBEEF;", "0xDEADBEEF", Number)
	assertClass(t, lex, "y = 3.14e-2;", "3.14e-2", Number)
	assertClass(t, lex, "puts(\"hi\");", "\"hi\"", String)
	assertClass(t, lex, "int n = 1; // trailing", "// trailing", Comment)
	assertClass(t, lex, "#include <stdio.h>", "#include", Keyword)
	assertClass(t, lex, "char c = 'x';", "'x'", String)
}

// The precedence choice, stated as a test so it cannot drift silently.
func TestStringsWinOverComments(t *testing.T) {
	lex := lexerNamed(t, "c")
	line := `const char *u = "http://example.com";`
	assertClass(t, lex, line, "http", String)
	assertClass(t, lex, line, "//", String)
}

func TestCBlockCommentSpansLines(t *testing.T) {
	lex := lexerNamed(t, "c")
	_, st := lex.Lex([]rune("/* opening"), 0)
	if st == 0 {
		t.Fatal("an unterminated block comment left no state open")
	}
	spans, st2 := lex.Lex([]rune("still inside"), st)
	if len(spans) != 1 || spans[0].Class != Comment {
		t.Errorf("middle line = %v, want one Comment span", spans)
	}
	if st2 != st {
		t.Errorf("state changed while still inside the comment: %v -> %v", st, st2)
	}
	spans, st3 := lex.Lex([]rune("done */ int x;"), st)
	if st3 != 0 {
		t.Errorf("state still open after the terminator: %v", st3)
	}
	if len(spans) == 0 || spans[0].Class != Comment {
		t.Errorf("closing line did not start as a comment: %v", spans)
	}
}

func TestPythonRules(t *testing.T) {
	lex := lexerNamed(t, "python")
	assertClass(t, lex, "def greet(name):", "def", Keyword)
	assertClass(t, lex, "class Greeter:", "class", Keyword)
	assertClass(t, lex, "return None", "None", Constant)
	assertClass(t, lex, "x = 42", "42", Number)
	assertClass(t, lex, "s = \"hello\"", "\"hello\"", String)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, "print(x)", "print", Function)
}

func TestPythonTripleQuotedString(t *testing.T) {
	lex := lexerNamed(t, "python")
	// Opening and closing on one line.
	assertClass(t, lex, `d = """one line"""`, `"""one line"""`, String)

	// And spanning lines.
	_, st := lex.Lex([]rune(`def f():`), 0)
	_, st = lex.Lex([]rune(`    """starts here`), st)
	if st == 0 {
		t.Fatal("an unterminated triple quote left no state open")
	}
	spans, _ := lex.Lex([]rune(`    still the docstring`), st)
	if len(spans) != 1 || spans[0].Class != String {
		t.Errorf("continuation line = %v, want one String span", spans)
	}
}

func TestShellRules(t *testing.T) {
	lex := lexerNamed(t, "sh")
	assertClass(t, lex, "for i in 1 2 3; do", "for", Keyword)
	assertClass(t, lex, "echo \"hi $NAME\"", "\"hi $NAME\"", String)
	assertClass(t, lex, "NAME=${1:-world}", "${1:-world}", Constant)
	assertClass(t, lex, "# deploy script", "# deploy script", Comment)
	assertClass(t, lex, "set -eu", "set", Keyword)
}

// Spans must be rune indices. A byte offset lands in the wrong column the
// moment a line holds anything outside ASCII, and the text still looks right so
// only the colours are wrong.
func TestBundledSpansAreRuneIndices(t *testing.T) {
	lex := lexerNamed(t, "python")
	line := []rune(`x = "日本語" # ok`)
	spans, _ := lex.Lex(line, 0)
	for _, s := range spans {
		if s.Start < 0 || s.End > len(line) || s.Start >= s.End {
			t.Fatalf("span %v outside the %d-rune line", s, len(line))
		}
	}
	assertClass(t, lex, string(line), `"日本語"`, String)
	assertClass(t, lex, string(line), "# ok", Comment)
}

// The invariants the Lexer interface promises, over every bundled language.
func TestBundledLexersObeyTheContract(t *testing.T) {
	samples := []string{
		"",
		"   ",
		`x = "unterminated`,
		"/* unterminated block",
		`日本語 = "混ざった" # コメント`,
		"\tif (x) { y(); } // done",
		"### ## #",
		`a'b"c\d`,
	}
	for _, name := range EmbeddedLanguages() {
		lex := EmbeddedRuleSet(name)
		t.Run(name, func(t *testing.T) {
			var st State
			for _, s := range samples {
				line := []rune(s)
				spans, next := lex.Lex(line, st)
				prevEnd := 0
				for _, sp := range spans {
					if sp.Start < prevEnd {
						t.Fatalf("%q: span %v overlaps or is out of order", s, sp)
					}
					if sp.Start >= sp.End {
						t.Fatalf("%q: empty span %v", s, sp)
					}
					if sp.End > len(line) {
						t.Fatalf("%q: span %v past the end of a %d-rune line", s, sp, len(line))
					}
					prevEnd = sp.End
				}
				st = next
			}
		})
	}
}

// An unknown class name must fail loudly. These files are ours, so a typo is a
// bug to be caught, and defaulting would colour the rule wrongly forever.
func TestUnknownClassIsAnError(t *testing.T) {
	src := `syntax thing "\.thing$"` + "\n" + `class keyowrd "\bfoo\b"` + "\n"
	_, err := ParseRuleFile("bad.nemrc", strings.NewReader(src))
	if err == nil {
		t.Fatal("a misspelled class name was accepted")
	}
	if !strings.Contains(err.Error(), "keyowrd") {
		t.Errorf("error %q does not name the offending class", err)
	}
}

func TestRuleFileErrors(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no syntax directive", `class keyword "\bfoo\b"`, "before syntax"},
		{"bad pattern", "syntax t \"\\.t$\"\nclass keyword \"\\b(unclosed\"", "pattern"},
		{"unterminated quote", `syntax t "\.t$` + "\n", "unterminated"},
		{"unknown directive", "syntax t \"\\.t$\"\nlinter pyflakes\n", "unknown directive"},
		{"no rules", "syntax t \"\\.t$\"\n", "no rules"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRuleFile("x.nemrc", strings.NewReader(tc.src))
			if err == nil {
				t.Fatalf("accepted bad input %q", tc.src)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// A backslash inside a quoted pattern is kept, so rules read as Go regexp
// literals rather than needing everything doubled.
func TestPatternsKeepTheirBackslashes(t *testing.T) {
	src := "syntax t \"\\.t$\"\nclass keyword \"\\bfoo\\b\"\n"
	rs, err := ParseRuleFile("t.nemrc", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseRuleFile: %v", err)
	}
	assertClass(t, rs, "a foo b", "foo", Keyword)
	assertClass(t, rs, "a foobar b", "foobar", Plain)
}

func TestRustRules(t *testing.T) {
	lex := lexerNamed(t, "rust")
	assertClass(t, lex, "pub fn main() {", "pub", Keyword)
	assertClass(t, lex, "let x: u32 = 5;", "u32", Type)
	assertClass(t, lex, "let ok = true;", "true", Constant)
	assertClass(t, lex, "println!(\"hi\");", "println!", Function)
	assertClass(t, lex, "let s = \"text\";", "\"text\"", String)
	assertClass(t, lex, "// a note", "// a note", Comment)
	assertClass(t, lex, "let n = 0xFF_u8;", "0xFF_u8", Number)
}

func TestJavaScriptRules(t *testing.T) {
	lex := lexerNamed(t, "javascript")
	assertClass(t, lex, "const x = 1;", "const", Keyword)
	assertClass(t, lex, "let v = null;", "null", Constant)
	assertClass(t, lex, "new Promise(r => r());", "Promise", Type)
	assertClass(t, lex, "s = `hi ${n}`;", "`hi ${n}`", String)
	assertClass(t, lex, "// note", "// note", Comment)
	assertClass(t, lex, "x = 1_000;", "1_000", Number)
}

func TestTypeScriptRules(t *testing.T) {
	lex := lexerNamed(t, "typescript")
	assertClass(t, lex, "interface Foo {", "interface", Keyword)
	assertClass(t, lex, "let n: number = 1;", "number", Type)
	assertClass(t, lex, "export const ok = true;", "true", Constant)
	assertClass(t, lex, "// note", "// note", Comment)
}

func TestYAMLRules(t *testing.T) {
	lex := lexerNamed(t, "yaml")
	assertClass(t, lex, "name: nem", "name", Function)
	assertClass(t, lex, "enabled: true", "true", Constant)
	assertClass(t, lex, "port: 8080", "8080", Number)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, `msg: "hello"`, `"hello"`, String)
}

func TestTOMLRules(t *testing.T) {
	lex := lexerNamed(t, "toml")
	assertClass(t, lex, "[package]", "[package]", Type)
	assertClass(t, lex, `name = "nem"`, `"nem"`, String)
	assertClass(t, lex, "edition = 2021", "2021", Number)
	assertClass(t, lex, "# a note", "# a note", Comment)
}

func TestINIRules(t *testing.T) {
	lex := lexerNamed(t, "ini")
	assertClass(t, lex, "[Unit]", "[Unit]", Type)
	assertClass(t, lex, "Restart=always", "Restart", Function)
	assertClass(t, lex, "; a note", "; a note", Comment)
	assertClass(t, lex, "# also a note", "# also a note", Comment)
}

func TestHTMLRules(t *testing.T) {
	lex := lexerNamed(t, "html")
	assertClass(t, lex, `<div class="x">`, "<div", Keyword)
	assertClass(t, lex, `<div class="x">`, `"x"`, String)
	assertClass(t, lex, "&amp; here", "&amp;", Constant)
}

func TestXMLRules(t *testing.T) {
	lex := lexerNamed(t, "xml")
	assertClass(t, lex, `<node id="1"/>`, "<node", Keyword)
	assertClass(t, lex, `<node id="1"/>`, `"1"`, String)
}

func TestCSSRules(t *testing.T) {
	lex := lexerNamed(t, "css")
	assertClass(t, lex, "  color: #ff8800;", "#ff8800", Constant)
	assertClass(t, lex, "  width: 12px;", "12px", Number)
	assertClass(t, lex, "@media screen {", "@media", Keyword)
}

func TestSQLRules(t *testing.T) {
	lex := lexerNamed(t, "sql")
	// SQL is written in either case, often in both at once.
	assertClass(t, lex, "SELECT * FROM t;", "SELECT", Keyword)
	assertClass(t, lex, "select * from t;", "select", Keyword)
	assertClass(t, lex, "x INTEGER NOT NULL", "INTEGER", Type)
	assertClass(t, lex, "-- a note", "-- a note", Comment)
	assertClass(t, lex, "WHERE s = 'v'", "'v'", String)
}

func TestMakefileRules(t *testing.T) {
	lex := lexerNamed(t, "makefile")
	assertClass(t, lex, "build: deps", "build:", Function)
	assertClass(t, lex, "\techo $(NAME)", "$(NAME)", Constant)
	assertClass(t, lex, ".PHONY: all", ".PHONY", Type)
	assertClass(t, lex, "# a note", "# a note", Comment)
}

func TestDockerfileRules(t *testing.T) {
	lex := lexerNamed(t, "dockerfile")
	assertClass(t, lex, "FROM alpine:3.19", "FROM", Keyword)
	assertClass(t, lex, "RUN apk add curl", "RUN", Keyword)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, "EXPOSE 8080", "8080", Number)
}

// A language nem bundles must never fall through to nano's version of it.
// editor only consults nanorc when For returns the plain lexer, so this is what
// guarantees the precedence the wiring depends on.
func TestBundledLanguagesNeverFallThrough(t *testing.T) {
	for _, path := range []string{
		"main.c", "a.cpp", "run.py", "deploy.sh", "lib.rs", "app.js", "app.ts",
		"conf.yaml", "Cargo.toml", "page.html", "doc.xml", "style.css",
		"q.sql", "Makefile", "Dockerfile", "settings.ini",
	} {
		t.Run(path, func(t *testing.T) {
			if _, plain := For(path).(PlainLexer); plain {
				t.Errorf("For(%q) returned the plain lexer, so nano's definition would shadow ours", path)
			}
		})
	}
}

// Lexing one line is what a keystroke costs. The nanorc path runs about 52µs
// for a line of C; this exists so the bundled path's cost is known rather than
// assumed.
func BenchmarkBundledLexCLine(b *testing.B) {
	lex := EmbeddedRuleSet("c")
	line := []rune(`    for (int i = 0; i < n; i++) { total += arr[i] * 2; } // sum`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}

func BenchmarkBundledLexPythonLine(b *testing.B) {
	lex := EmbeddedRuleSet("python")
	line := []rune(`    def greet(self, name="world"): return f"hello {name}"  # note`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}

// No bundled language may claim a file a hand-written lexer handles.
//
// ForWithHeader checks native first, so such an overlap would be harmless
// today - which is exactly why it needs a test. The ordering is invisible while
// the two sets are disjoint, so nothing would catch a bundled go.nemrc silently
// becoming dead weight, or the order being swapped later and the rule file
// quietly displacing a real lexer.
func TestNoBundledLanguageShadowsANativeOne(t *testing.T) {
	for _, path := range []string{
		"main.go", "init.lua", "data.json", "README.md", "doc.markdown",
	} {
		t.Run(path, func(t *testing.T) {
			if nativeFor(path) == nil {
				t.Fatalf("test bug: %q is not handled natively", path)
			}
			if lex := embeddedFor(path, ""); lex != nil {
				t.Errorf("bundled language %q also claims %q; native wins today, "+
					"so the rule file is dead weight and the two could diverge",
					lex.Name(), path)
			}
		})
	}
}

// A control-flow keyword followed by a parenthesis matches the call rule just
// as a function name does. RE2 has no lookahead, so the rule cannot exclude
// them - the keyword list repaints them instead, and that only works while the
// call rule comes first.
func TestKeywordsBeatTheCallRule(t *testing.T) {
	c := lexerNamed(t, "c")
	assertClass(t, c, "for (int i = 0; i < n; i++) {", "for", Keyword)
	assertClass(t, c, "if (x) return;", "if", Keyword)
	assertClass(t, c, "while (1) {}", "while", Keyword)
	assertClass(t, c, "printf(\"hi\");", "printf", Function)
	assertClass(t, c, "sizeof(int)", "sizeof", Keyword)

	py := lexerNamed(t, "python")
	assertClass(t, py, "def greet(name):", "def", Keyword)
	assertClass(t, py, "def greet(name):", "greet", Function)
}
