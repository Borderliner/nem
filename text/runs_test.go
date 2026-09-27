package text

import (
	"math/rand/v2"
	"testing"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// refCluster is one cluster as the segmenter sees it.
type refCluster struct {
	start RuneIdx
	n     int
	col   ColIdx
	w     ColIdx
}

// referenceClusters lays rs out a cluster at a time, with nothing joined
// into runs: what every lookup must agree with.
func referenceClusters(rs []rune, tw ColIdx) ([]refCluster, ColIdx) {
	var out []refCluster
	rest, state := string(rs), -1
	col, idx := ColIdx(0), RuneIdx(0)
	for len(rest) > 0 {
		var cl string
		var w int
		cl, rest, w, state = uniseg.FirstGraphemeClusterInString(rest, state)
		n := utf8.RuneCountInString(cl)
		cw := ColIdx(w)
		if cl == "\t" {
			cw = tw - (col % tw)
		}
		out = append(out, refCluster{idx, n, col, cw})
		col += cw
		idx += RuneIdx(n)
	}
	return out, col
}

// Runs change how the layout is kept, not what it says: every lookup answers
// as it would from one segment per cluster.
func TestRunsAnswerAsClustersWould(t *testing.T) {
	pieces := []string{
		"a", "bc", " ", "\t", "\x01", "é", "́", "漢字", "ｱ",
		"👍🏽", "🇮🇷", "سلام", "َ", "��", "é",
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for trial := range 5000 {
		var rs []rune
		for range rng.IntN(10) {
			rs = append(rs, []rune(pieces[rng.IntN(len(pieces))])...)
		}
		l := NewLine(rs)
		ref, width := referenceClusters(rs, effectiveTabWidth())
		fail := func(what string, args ...any) {
			t.Helper()
			t.Fatalf("trial %d, %q: %s %v\nclusters %+v\nsegments %+v", trial, string(rs), what, args, ref, l.segs)
		}

		if l.Width() != width {
			fail("width", l.Width(), width)
		}
		var got []refCluster
		for c := range l.Clusters() {
			got = append(got, refCluster{c.Start, len(c.Runes), c.Col, c.Width})
		}
		if len(got) != len(ref) {
			fail("cluster count", len(got), len(ref))
		}
		for i := range got {
			if got[i] != ref[i] {
				fail("cluster", i, got[i], ref[i])
			}
		}

		// Which cluster holds each rune, and covers each column.
		clusterOf := func(i RuneIdx) int {
			for k, c := range ref {
				if i >= c.start && i < c.start+RuneIdx(c.n) {
					return k
				}
			}
			return len(ref)
		}
		for i := RuneIdx(0); i <= RuneIdx(len(rs)); i++ {
			k := clusterOf(i)
			wantCol, wantNext, wantPrev := width, RuneIdx(len(rs)), RuneIdx(0)
			if k < len(ref) {
				wantCol = ref[k].col
				wantNext = ref[k].start + RuneIdx(ref[k].n)
			}
			for _, c := range ref {
				if c.start < i {
					wantPrev = c.start
				}
			}
			if got := l.DisplayCol(i); got != wantCol {
				fail("DisplayCol", i, got, wantCol)
			}
			if i < RuneIdx(len(rs)) {
				if got := l.NextGrapheme(i); got != wantNext {
					fail("NextGrapheme", i, got, wantNext)
				}
			}
			if got := l.PrevGrapheme(i); got != wantPrev {
				fail("PrevGrapheme", i, got, wantPrev)
			}
		}
		for c := ColIdx(0); c < width; c++ {
			k := 0
			for j, rc := range ref {
				if rc.col <= c {
					k = j
				}
			}
			want := ref[k].start
			if c == 0 {
				want = 0 // RuneAt's column 0 is always the line's start
			}
			if got := l.RuneAt(c); got != want {
				fail("RuneAt", c, got, want)
			}
			var first Cluster
			for cl := range l.ClustersFrom(c) {
				first = cl
				break
			}
			if first.Start != ref[k].start || first.Col != ref[k].col {
				fail("ClustersFrom", c, first.Start, ref[k].start)
			}
		}
	}
}
