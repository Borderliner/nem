package syntax

import (
	"strings"
	"testing"
)

// goSource builds a Go file of roughly n lines out of constructs a real file
// contains - comments, strings, numbers, calls, a block comment and a raw
// string - so the benchmark measures the work the lexer actually does rather
// than a best case of bare identifiers.
func goSource(n int) [][]rune {
	chunk := []string{
		"// Package thing does something useful.",
		"package thing",
		"",
		"import (",
		"\t\"fmt\"",
		"\t\"strings\"",
		")",
		"",
		"/* A block comment",
		"   spanning several lines",
		"   to exercise carried state. */",
		"",
		"const tmpl = `a raw string",
		"that spans lines`",
		"",
		"type Widget struct {",
		"\tName  string",
		"\tCount int",
		"\tratio float64",
		"}",
		"",
		"// New returns a Widget.",
		"func New(name string, count int) *Widget {",
		"\treturn &Widget{Name: name, Count: count, ratio: 0.5}",
		"}",
		"",
		"func (w *Widget) Describe() string {",
		"\tif w.Count == 0 || w.Name == \"\" {",
		"\t\treturn \"empty\" // nothing to say",
		"\t}",
		"\tn := 1_000 + 0xff - 0b1010",
		"\treturn fmt.Sprintf(\"%s: %d (%v)\", w.Name, n, strings.ToLower(w.Name))",
		"}",
		"",
	}
	var lines [][]rune
	for len(lines) < n {
		for _, s := range chunk {
			lines = append(lines, []rune(s))
			if len(lines) >= n {
				break
			}
		}
	}
	return lines
}

func BenchmarkGoLexFile(b *testing.B) {
	lines := goSource(500)
	lx := goLexer{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var st State
		for _, l := range lines {
			_, st = lx.Lex(l, st)
		}
	}
}

// One line is the common case: a keystroke re-lexes the edited line and then
// only far enough to converge.
func BenchmarkGoLexOneLine(b *testing.B) {
	line := []rune("\treturn fmt.Sprintf(\"%s: %d\", w.Name, 1_000+0xff) // done")
	lx := goLexer{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lx.Lex(line, 0)
	}
}

func BenchmarkLuaLexFile(b *testing.B) {
	var lines [][]rune
	src := strings.Split(strings.Repeat(
		"local x = 1\nfunction f(a, b)\n  return a .. \"str\" -- note\nend\n", 125), "\n")
	for _, s := range src {
		lines = append(lines, []rune(s))
	}
	lx := luaLexer{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var st State
		for _, l := range lines {
			_, st = lx.Lex(l, st)
		}
	}
}
