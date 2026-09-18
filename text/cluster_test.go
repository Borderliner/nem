package text

import (
	"testing"
)

// flat is a Cluster flattened for comparison, since Cluster holds a slice.
type flat struct {
	s     string
	start RuneIdx
	col   ColIdx
	w     ColIdx
}

func collect(l *Line) []flat {
	var got []flat
	for c := range l.Clusters() {
		got = append(got, flat{string(c.Runes), c.Start, c.Col, c.Width})
	}
	return got
}

func TestClustersWalksGraphemesWithPlacement(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want []flat
	}{
		{"empty", "", nil},
		{"ascii", "abc", []flat{
			{"a", 0, 0, 1}, {"b", 1, 1, 1}, {"c", 2, 2, 1},
		}},
		{"tab at column zero", "\tx", []flat{
			{"\t", 0, 0, 8}, {"x", 1, 8, 1},
		}},
		{"tab mid line advances to the stop", "ab\tc", []flat{
			{"a", 0, 0, 1}, {"b", 1, 1, 1}, {"\t", 2, 2, 6}, {"c", 3, 8, 1},
		}},
		{"cjk is double width", "日本", []flat{
			{"日", 0, 0, 2}, {"本", 1, 2, 2},
		}},
		{"decomposed accent is one cluster of two runes", "éx", []flat{
			{"é", 0, 0, 1}, {"x", 2, 1, 1},
		}},
		{"zwj family is one cluster of seven runes", "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466", []flat{
			{"\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466", 0, 0, 2},
		}},
		{"every width class at once", mixed, []flat{
			{"a", 0, 0, 1},
			{"\t", 1, 1, 7},
			{"世", 2, 8, 2},
			{"é", 3, 10, 1},
			{"\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466", 5, 11, 2},
			{"b", 12, 13, 1},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLine([]rune(tc.in))
			got := collect(&l)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d clusters %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("cluster %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// The iterator must agree with the accessors it is meant to replace, or the
// render path would draw different glyph boundaries than the cursor uses.
func TestClustersAgreeWithGraphemeStepping(t *testing.T) {
	for _, s := range guaranteeCases {
		l := NewLine([]rune(s))
		var viaStep []RuneIdx
		for i := RuneIdx(0); i < l.Len(); i = l.NextGrapheme(i) {
			viaStep = append(viaStep, i)
		}
		var viaIter []RuneIdx
		for c := range l.Clusters() {
			viaIter = append(viaIter, c.Start)
			if got := l.DisplayCol(c.Start); got != c.Col {
				t.Errorf("%q: cluster at %d has Col %d, DisplayCol says %d", s, c.Start, c.Col, got)
			}
		}
		if len(viaStep) != len(viaIter) {
			t.Errorf("%q: stepping found %v, iterator found %v", s, viaStep, viaIter)
			continue
		}
		for i := range viaStep {
			if viaStep[i] != viaIter[i] {
				t.Errorf("%q: boundary %d: stepping %d, iterator %d", s, i, viaStep[i], viaIter[i])
			}
		}
	}
}

// Widths must sum to the line width, which is what lets the render path stop
// walking as soon as it passes the right edge.
func TestClusterWidthsSumToLineWidth(t *testing.T) {
	for _, s := range guaranteeCases {
		l := NewLine([]rune(s))
		sum := ColIdx(0)
		for c := range l.Clusters() {
			sum += c.Width
		}
		if sum != l.Width() {
			t.Errorf("%q: widths sum to %d, Width() = %d", s, sum, l.Width())
		}
	}
}

// Breaking out early must stop the walk. The render path breaks as soon as a
// cluster passes the right edge of the window, so this is load-bearing rather
// than a formality.
func TestClustersStopOnEarlyReturn(t *testing.T) {
	l := NewLine([]rune("abcdef"))
	n := 0
	for range l.Clusters() {
		n++
		if n == 2 {
			break
		}
	}
	if n != 2 {
		t.Errorf("walked %d clusters after breaking at 2", n)
	}
}

// Runes are lent from the line, not copied: that is the whole point of the
// iterator. Mutating the line's content must be visible through a fresh walk,
// and the slice must alias the line's backing array rather than a copy.
func TestClusterRunesAreLentNotCopied(t *testing.T) {
	l := NewLine([]rune("abc"))
	var first []rune
	for c := range l.Clusters() {
		first = c.Runes
		break
	}
	if len(first) != 1 || first[0] != 'a' {
		t.Fatalf("first cluster runes = %q, want \"a\"", string(first))
	}
	// &l.runes[0] is the line's own storage; the lent slice must point into it.
	if &first[0] != &l.runes[0] {
		t.Error("cluster runes were copied; the iterator should lend the line's storage")
	}
}

// Walking a line must not allocate. The render path walks every visible line on
// every keystroke, so an allocation here is an allocation per line per frame.
func TestClustersDoNotAllocate(t *testing.T) {
	l := NewLine([]rune(mixed))
	l.Width() // build the cache outside the measurement
	got := testing.AllocsPerRun(100, func() {
		for c := range l.Clusters() {
			_ = c.Width
		}
	})
	if got != 0 {
		t.Errorf("walking a line allocated %v times per run, want 0", got)
	}
}

// A realistic line of source code, walked the way the render path walks it.
const benchLine = "\tif err := scr.SetContent(x+int(sx), y, runes[i], runes[i+1:n], style); err != nil {"

// BenchmarkWalkViaAccessors measures the old approach: copy the line's runes,
// then per cluster call NextGrapheme plus two DisplayCol lookups, each a binary
// search over the segment cache.
func BenchmarkWalkViaAccessors(b *testing.B) {
	l := NewLine([]rune(benchLine))
	l.Width()
	for b.Loop() {
		runes := l.Runes()
		for i := RuneIdx(0); i < l.Len(); {
			n := l.NextGrapheme(i)
			if n <= i {
				break
			}
			start := l.DisplayCol(i)
			w := l.DisplayCol(n) - start
			_, _ = runes[i], w
			i = n
		}
	}
}

// BenchmarkWalkViaClusters measures the same walk through the iterator, which
// reads the segment cache in order and lends the runes.
func BenchmarkWalkViaClusters(b *testing.B) {
	l := NewLine([]rune(benchLine))
	l.Width()
	for b.Loop() {
		for c := range l.Clusters() {
			_, _ = c.Runes[0], c.Width
		}
	}
}
