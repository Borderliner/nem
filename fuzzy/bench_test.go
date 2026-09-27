package fuzzy

import (
	"fmt"
	"testing"
)

// manyCandidates approximates file completion in a large directory, which is the
// worst case: every keystroke re-ranks the whole set.
func manyCandidates(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("src/pkg%d/internal/handler_%d_test.go", i%37, i)
	}
	return out
}

func BenchmarkRank2000(b *testing.B) {
	c := manyCandidates(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Rank("hdlr", c)
	}
}

// The common case: M-x over nem's real command set.
func BenchmarkRankCommands(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Rank("fwc", nemCommands)
	}
}

// Most candidates do not match, so the cheap subsequence rejection carries most
// of the load. This measures that path.
func BenchmarkRankMostlyRejected(b *testing.B) {
	c := manyCandidates(2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Rank("qqqq", c)
	}
}

// manyPaths approximates project-find-file in a large repository: paths a
// few directories deep, with the mix of sources, tests and other files a real
// tree has, so a query rejects most of them for different reasons.
func manyPaths(n int) []string {
	tops := []string{"editor", "buffer", "internal", "ui", "fuzzy", "cmd", "docs", "vendor"}
	subs := []string{"core", "edit", "display", "io", "keymap", "search", "util", "testdata"}
	names := []string{"window", "buffer", "command", "render", "handler", "parse", "index", "state"}
	exts := []string{".go", "_test.go", ".md", ".go", ".txt", ".json"}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s/%s/%s_%d%s",
			tops[i%len(tops)], subs[i/len(tops)%len(subs)], names[i/64%len(names)], i, exts[i%len(exts)])
	}
	return out
}

// Every file in a large project, which Rank shares out across the CPUs.
func BenchmarkRankPaths100k(b *testing.B) {
	c := manyPaths(100_000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Rank("edwin", c)
	}
}
