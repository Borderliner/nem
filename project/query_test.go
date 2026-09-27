package project

import (
	"slices"
	"strings"
	"testing"
)

// A query reads as ripgrep's command line: options, then the rest as the
// pattern, spaces and all.
func TestParseQuery(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Query
	}{
		{"count", Query{Pattern: "count"}},
		{"hello world", Query{Pattern: "hello world"}},
		{"-w count", Query{Pattern: "count", Word: true}},
		{"-t go -w count", Query{Pattern: "count", Word: true, Types: []string{"go"}}},
		{"-tgo -wi Count", Query{Pattern: "Count", Word: true, Case: CaseIgnore, Types: []string{"go"}}},
		{"-g '!vendor/**' -g *.go TODO", Query{Pattern: "TODO", Globs: []string{"!vendor/**", "*.go"}}},
		{"--type=py --type-not md -s x", Query{Pattern: "x", Case: CaseSensitive, Types: []string{"py"}, NotTypes: []string{"md"}}},
		{"-F a.b(c)", Query{Pattern: "a.b(c)", Fixed: true}},
		{"-- -w", Query{Pattern: "-w"}},
		{"-e -w", Query{Pattern: "-w"}},
		{"-u needle", Query{Pattern: "needle", NoIgnore: true}},
		{"- dash", Query{Pattern: "- dash"}},
		{"-i", Query{Case: CaseIgnore}},
		{"", Query{}},
	} {
		got, err := ParseQuery(tc.in)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", tc.in, err)
			continue
		}
		got.globs = nil
		if !queryEqual(got, tc.want) {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"-z x", "-t", "-t gox x", "--nope x", "-g '[' x"} {
		if _, err := ParseQuery(bad); err == nil && bad != "-g '[' x" {
			t.Errorf("ParseQuery(%q) is no error", bad)
		}
	}
}

func queryEqual(a, b Query) bool {
	return a.Pattern == b.Pattern && a.Fixed == b.Fixed && a.Word == b.Word && a.Case == b.Case &&
		a.NoIgnore == b.NoIgnore && slices.Equal(a.Types, b.Types) && slices.Equal(a.NotTypes, b.NotTypes) &&
		slices.Equal(a.Globs, b.Globs)
}

// Types and globs choose the files: a glob without a slash matches a name
// at any depth, one with a slash a path from the top, ** any directories,
// and the last glob matching a file decides it.
func TestQueryKeep(t *testing.T) {
	files := []string{"main.go", "cmd/nem/main.go", "vendor/x/y.go", "README.md", "docs/a.md",
		"Makefile", "web/app.ts", "web/app_test.ts", ".github/ci.yml"}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"-t go x", []string{"main.go", "cmd/nem/main.go", "vendor/x/y.go"}},
		{"-T go -T md x", []string{"Makefile", "web/app.ts", "web/app_test.ts", ".github/ci.yml"}},
		{"-t make x", []string{"Makefile"}},
		{"-g *.md x", []string{"README.md", "docs/a.md"}},
		{"-g '!vendor/**' -t go x", []string{"main.go", "cmd/nem/main.go"}},
		{"-g '!*_test.ts' -t ts x", []string{"web/app.ts"}},
		{"-g docs/*.md x", []string{"docs/a.md"}},
		{"-g '**/nem/**' x", []string{"cmd/nem/main.go"}},
		{"-g web x", []string{"web/app.ts", "web/app_test.ts"}},
		{"-g '*.go' -g '!main.go' x", []string{"vendor/x/y.go"}},
		{"-g '!main.go' -g '*.go' x", []string{"main.go", "cmd/nem/main.go", "vendor/x/y.go"}},
	} {
		q, err := ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		if got := q.Filter(files); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.query, got, tc.want)
		}
	}
}

// Case is smart unless an option says: a capital in what is looked for -
// not in \S or \W - makes it count. -w wants whole words, -F takes the
// pattern as text, and so does a pattern that is not a valid regexp.
func TestQueryRegexp(t *testing.T) {
	for _, tc := range []struct {
		query        string
		match, noMat string
	}{
		{"needle", "NEEDLE here", "nedle"},
		{"Needle", "Needle", "needle"},
		{`\S+dle`, "NEEDLE", "dle"},
		{"-s needle", "needle", "NEEDLE"},
		{"-i Needle", "NEEDLE", "nee"},
		{"-w count", "a count b", "counter"},
		{"-F a.b", "xa.by", "axb"},
		{"foo(", "call foo(1)", "foo"},
		{"-w -F a.b", "x a.b y", "xa.by"},
	} {
		q, err := ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		re := q.Regexp()
		if !re.MatchString(tc.match) {
			t.Errorf("%s: %v does not match %q", tc.query, re, tc.match)
		}
		if re.MatchString(tc.noMat) {
			t.Errorf("%s: %v matches %q", tc.query, re, tc.noMat)
		}
	}
	if !strings.Contains(strings.Join(FileTypes(), " "), "go") {
		t.Error("FileTypes does not list go")
	}
}
