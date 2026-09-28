package syntax

import (
	"path"
	"strings"
	"testing"
)

// These tests use assertClass from invariants_test.go, which checks the span
// contract as well as the classification - so every assertion here doubles as a
// check that the engine emits well-formed spans.

// Every built-in definition must parse and compile. They ship inside the
// binary, so a broken one is a build-time mistake, and this is what turns it
// into a red CI run rather than a language quietly losing its colours.
func TestBuiltinLanguagesLoad(t *testing.T) {
	if err := BuiltinError(); err != nil {
		t.Fatalf("built-in languages failed to load: %v", err)
	}
	if n := len(Builtin().Languages()); n < 20 {
		t.Errorf("only %d built-in languages; the set has shrunk unexpectedly", n)
	}
}

// Two languages claiming the same file would leave which one colours it to the
// order they happen to be read in, and one of them could never be reached.
func TestNoTwoBuiltinLanguagesClaimOneFile(t *testing.T) {
	owner := map[string]string{}
	for _, l := range Builtin().Languages() {
		for _, g := range l.Def().Files {
			if prev, dup := owner[g]; dup {
				t.Errorf("%s and %s both claim %s", prev, l.Name(), g)
			}
			owner[g] = l.Name()
		}
		for _, g := range l.Def().Files {
			for other, name := range owner {
				if name == l.Name() || strings.ContainsAny(g, "*?[") {
					continue
				}
				// An exact name one language claims must not also be
				// matched by another's pattern, or the pattern loses.
				if ok, _ := path.Match(other, g); ok {
					t.Errorf("%s claims %s, which %s's %s also matches", l.Name(), g, name, other)
				}
			}
		}
	}
}

func TestDetectByName(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"main.go", "go"},
		{"init.lua", "lua"},
		{"a.json", "json"},
		{"README.md", "markdown"},
		{"main.c", "c"},
		{"a.cpp", "cpp"},
		{"run.py", "python"},
		{"deploy.sh", "sh"},
		{"lib.rs", "rust"},
		{"app.js", "javascript"},
		{"app.ts", "typescript"},
		{"conf.yaml", "yaml"},
		{"Cargo.toml", "toml"},
		{"page.html", "html"},
		{"doc.xml", "xml"},
		{"style.css", "css"},
		{"q.sql", "sql"},
		{"Makefile", "makefile"},
		{"GNUmakefile", "makefile"},
		{"Dockerfile", "dockerfile"},
		{"Dockerfile.dev", "dockerfile"},
		{"settings.ini", "ini"},
		{"go.syntax", "syntax"},
		{"/some/dir/.bashrc", "sh"},
		{"nothing.xyzzy", "text"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := For(tc.path).Name(); got != tc.want {
				t.Errorf("For(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetectByFirstLine(t *testing.T) {
	for _, tc := range []struct{ first, want string }{
		{"#!/bin/sh", "sh"},
		{"#!/usr/bin/env bash", "sh"},
		{"#!/usr/bin/env python3", "python"},
		{"#!/usr/bin/python3.12 -u", "python"},
		{"#!/usr/bin/env -S node --no-warnings", "javascript"},
		{"#!/usr/bin/env LC_ALL=C lua5.4", "lua"},
		{`<?xml version="1.0"?>`, "xml"},
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
	lex := lang(t, "c")
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
	lex := lang(t, "c")
	line := `const char *u = "http://example.com";`
	assertClass(t, lex, line, "http", String)
	assertClass(t, lex, line, "//", String)
}
func TestCBlockCommentSpansLines(t *testing.T) {
	lex := lang(t, "c")
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
	lex := lang(t, "python")
	assertClass(t, lex, "def greet(name):", "def", Keyword)
	assertClass(t, lex, "class Greeter:", "class", Keyword)
	assertClass(t, lex, "return None", "None", Constant)
	assertClass(t, lex, "x = 42", "42", Number)
	assertClass(t, lex, "s = \"hello\"", "\"hello\"", String)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, "print(x)", "print", Function)
}
func TestPythonTripleQuotedString(t *testing.T) {
	lex := lang(t, "python")
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
	lex := lang(t, "sh")
	assertClass(t, lex, "for i in 1 2 3; do", "for", Keyword)
	assertClass(t, lex, "echo \"hi $NAME\"", "\"hi $NAME\"", String)
	assertClass(t, lex, "NAME=${1:-world}", "${1:-world}", Constant)
	assertClass(t, lex, "# deploy script", "# deploy script", Comment)
	assertClass(t, lex, "set -eu", "set", Keyword)
}

// Spans must be rune indices. A byte offset lands in the wrong column the
// moment a line holds anything outside ASCII, and the text still looks right so
// only the colours are wrong.
func TestPatternsAreMatchedOnRunes(t *testing.T) {
	lex := lang(t, "python")
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
func TestRustRules(t *testing.T) {
	lex := lang(t, "rust")
	assertClass(t, lex, "pub fn main() {", "pub", Keyword)
	assertClass(t, lex, "let x: u32 = 5;", "u32", Type)
	assertClass(t, lex, "let ok = true;", "true", Constant)
	assertClass(t, lex, "println!(\"hi\");", "println!", Function)
	assertClass(t, lex, "let s = \"text\";", "\"text\"", String)
	assertClass(t, lex, "// a note", "// a note", Comment)
	assertClass(t, lex, "let n = 0xFF_u8;", "0xFF_u8", Number)
}
func TestJavaScriptRules(t *testing.T) {
	lex := lang(t, "javascript")
	assertClass(t, lex, "const x = 1;", "const", Keyword)
	assertClass(t, lex, "let v = null;", "null", Constant)
	assertClass(t, lex, "new Promise(r => r());", "Promise", Type)
	assertClass(t, lex, "s = `hi ${n}`;", "`hi ${n}`", String)
	assertClass(t, lex, "// note", "// note", Comment)
	assertClass(t, lex, "x = 1_000;", "1_000", Number)
}
func TestTypeScriptRules(t *testing.T) {
	lex := lang(t, "typescript")
	assertClass(t, lex, "interface Foo {", "interface", Keyword)
	assertClass(t, lex, "let n: number = 1;", "number", Type)
	assertClass(t, lex, "export const ok = true;", "true", Constant)
	assertClass(t, lex, "// note", "// note", Comment)
}
func TestYAMLRules(t *testing.T) {
	lex := lang(t, "yaml")
	assertClass(t, lex, "name: nem", "name", Function)
	assertClass(t, lex, "enabled: true", "true", Constant)
	assertClass(t, lex, "port: 8080", "8080", Number)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, `msg: "hello"`, `"hello"`, String)
}
func TestTOMLRules(t *testing.T) {
	lex := lang(t, "toml")
	assertClass(t, lex, "[package]", "[package]", Type)
	assertClass(t, lex, `name = "nem"`, `"nem"`, String)
	assertClass(t, lex, "edition = 2021", "2021", Number)
	assertClass(t, lex, "# a note", "# a note", Comment)
}
func TestINIRules(t *testing.T) {
	lex := lang(t, "ini")
	assertClass(t, lex, "[Unit]", "[Unit]", Type)
	assertClass(t, lex, "Restart=always", "Restart", Function)
	assertClass(t, lex, "; a note", "; a note", Comment)
	assertClass(t, lex, "# also a note", "# also a note", Comment)
}
func TestHTMLRules(t *testing.T) {
	lex := lang(t, "html")
	assertClass(t, lex, `<div class="x">`, "<div", Keyword)
	assertClass(t, lex, `<div class="x">`, `"x"`, String)
	assertClass(t, lex, "&amp; here", "&amp;", Constant)
}
func TestXMLRules(t *testing.T) {
	lex := lang(t, "xml")
	assertClass(t, lex, `<node id="1"/>`, "<node", Keyword)
	assertClass(t, lex, `<node id="1"/>`, `"1"`, String)
}
func TestCSSRules(t *testing.T) {
	lex := lang(t, "css")
	assertClass(t, lex, "  color: #ff8800;", "#ff8800", Constant)
	assertClass(t, lex, "  width: 12px;", "12px", Number)
	assertClass(t, lex, "@media screen {", "@media", Keyword)
}
func TestSQLRules(t *testing.T) {
	lex := lang(t, "sql")
	// SQL is written in either case, often in both at once.
	assertClass(t, lex, "SELECT * FROM t;", "SELECT", Keyword)
	assertClass(t, lex, "select * from t;", "select", Keyword)
	assertClass(t, lex, "x INTEGER NOT NULL", "INTEGER", Type)
	assertClass(t, lex, "-- a note", "-- a note", Comment)
	assertClass(t, lex, "WHERE s = 'v'", "'v'", String)
}
func TestMakefileRules(t *testing.T) {
	lex := lang(t, "makefile")
	assertClass(t, lex, "build: deps", "build:", Function)
	assertClass(t, lex, "\techo $(NAME)", "$(NAME)", Constant)
	assertClass(t, lex, ".PHONY: all", ".PHONY", Type)
	assertClass(t, lex, "# a note", "# a note", Comment)
}
func TestDockerfileRules(t *testing.T) {
	lex := lang(t, "dockerfile")
	assertClass(t, lex, "FROM alpine:3.19", "FROM", Keyword)
	assertClass(t, lex, "RUN apk add curl", "RUN", Keyword)
	assertClass(t, lex, "# a note", "# a note", Comment)
	assertClass(t, lex, "EXPOSE 8080", "8080", Number)
}

// A control-flow keyword followed by a parenthesis matches the call rule just
// as a function name does. RE2 has no lookahead, so the rule cannot exclude
// them - the keyword list repaints them instead, and that only works while the
// call rule comes first.
func TestKeywordsBeatTheCallRule(t *testing.T) {
	c := lang(t, "c")
	assertClass(t, c, "for (int i = 0; i < n; i++) {", "for", Keyword)
	assertClass(t, c, "if (x) return;", "if", Keyword)
	assertClass(t, c, "while (1) {}", "while", Keyword)
	assertClass(t, c, "printf(\"hi\");", "printf", Function)
	assertClass(t, c, "sizeof(int)", "sizeof", Keyword)

	py := lang(t, "python")
	assertClass(t, py, "def greet(name):", "def", Keyword)
	assertClass(t, py, "def greet(name):", "greet", Function)
}

// Lexing one line is what a keystroke costs; these keep it known rather than
// assumed.
func BenchmarkLexCLine(b *testing.B) {
	lex := Builtin().Language("c")
	line := []rune(`    for (int i = 0; i < n; i++) { total += arr[i] * 2; } // sum`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}
func BenchmarkLexPythonLine(b *testing.B) {
	lex := Builtin().Language("python")
	line := []rune(`    def greet(self, name="world"): return f"hello {name}"  # note`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}
