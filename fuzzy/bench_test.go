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
