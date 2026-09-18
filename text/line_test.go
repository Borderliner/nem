package text

import "testing"

// mixed exercises every width class at once:
//
//	rune idx: a=0  \t=1  世=2  é=3,4  family=5..11  b=12   (13 runes)
//	columns:  a@0  \t@1(w7) 世@8(w2) é@10(w1) family@11(w2) b@13(w1)  => width 14
const mixed = "a\t世é\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466b"

func TestLineWidth(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want ColIdx
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"tab at col 0", "\t", 8},
		{"tab mid line", "ab\tc", 9},
		{"tab exactly on stop", "abcdefgh\t", 16},
		{"two tabs", "\t\t", 16},
		{"combining eacute", "é", 1},
		{"cjk", "世界", 4},
		{"zwj family", "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466", 2},
		{"mixed", mixed, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLine([]rune(tt.s))
			if got := l.Width(); got != tt.want {
				t.Errorf("Width() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLineDisplayCol(t *testing.T) {
	tests := []struct {
		name string
		s    string
		idx  RuneIdx
		want ColIdx
	}{
		{"ascii start", "hello", 0, 0},
		{"ascii mid", "hello", 3, 3},
		{"ascii end", "hello", 5, 5},
		{"after tab", "a\tb", 2, 8},
		{"tab itself", "a\tb", 1, 1},
		{"cjk second glyph", "世界", 1, 2},
		{"cjk end", "世界", 2, 4},
		{"past combining", "éx", 2, 1},
		{"mixed a", mixed, 0, 0},
		{"mixed tab", mixed, 1, 1},
		{"mixed cjk", mixed, 2, 8},
		{"mixed eacute", mixed, 3, 10},
		{"mixed family", mixed, 5, 11},
		{"mixed b", mixed, 12, 13},
		{"mixed end", mixed, 13, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLine([]rune(tt.s))
			if got := l.DisplayCol(tt.idx); got != tt.want {
				t.Errorf("DisplayCol(%d) = %d, want %d", tt.idx, got, tt.want)
			}
		})
	}
}

func TestLineRuneAtClampsIntoWideGlyphs(t *testing.T) {
	tests := []struct {
		name string
		s    string
		col  ColIdx
		want RuneIdx
	}{
		{"ascii", "hello", 3, 3},
		{"ascii past end", "hello", 99, 5},
		{"inside tab", "a\tb", 4, 1}, // col 4 is inside the tab's run 1..7
		{"just after tab", "a\tb", 8, 2},
		{"cjk left half", "世界", 0, 0},
		{"cjk right half", "世界", 1, 0}, // clamp to start of 世
		{"cjk second left", "世界", 2, 1},
		{"cjk second right", "世界", 3, 1}, // clamp to start of 界
		{"family left half", mixed, 11, 5},
		{"family right half", mixed, 12, 5}, // clamp to start of the ZWJ cluster
		{"mixed b", mixed, 13, 12},
		{"negative", "hello", -5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLine([]rune(tt.s))
			if got := l.RuneAt(tt.col); got != tt.want {
				t.Errorf("RuneAt(%d) = %d, want %d", tt.col, got, tt.want)
			}
		})
	}
}

func TestLineGraphemeStepping(t *testing.T) {
	// boundaries of `mixed`, in order
	want := []RuneIdx{0, 1, 2, 3, 5, 12, 13}
	l := NewLine([]rune(mixed))

	var fwd []RuneIdx
	for i := RuneIdx(0); ; {
		fwd = append(fwd, i)
		n := l.NextGrapheme(i)
		if n == i {
			break
		}
		i = n
	}
	if len(fwd) != len(want) {
		t.Fatalf("forward walk = %v, want %v", fwd, want)
	}
	for i := range want {
		if fwd[i] != want[i] {
			t.Fatalf("forward walk = %v, want %v", fwd, want)
		}
	}

	var back []RuneIdx
	for i := l.Len(); ; {
		back = append([]RuneIdx{i}, back...)
		p := l.PrevGrapheme(i)
		if p == i {
			break
		}
		i = p
	}
	if len(back) != len(want) {
		t.Fatalf("backward walk = %v, want %v", back, want)
	}
	for i := range want {
		if back[i] != want[i] {
			t.Fatalf("backward walk = %v, want %v", back, want)
		}
	}
}

func TestLineGraphemeSteppingClampsAtEnds(t *testing.T) {
	l := NewLine([]rune("hi"))
	if got := l.PrevGrapheme(0); got != 0 {
		t.Errorf("PrevGrapheme(0) = %d, want 0", got)
	}
	if got := l.NextGrapheme(2); got != 2 {
		t.Errorf("NextGrapheme(len) = %d, want 2", got)
	}
	empty := NewLine(nil)
	if got := empty.NextGrapheme(0); got != 0 {
		t.Errorf("empty NextGrapheme(0) = %d, want 0", got)
	}
	if got := empty.PrevGrapheme(0); got != 0 {
		t.Errorf("empty PrevGrapheme(0) = %d, want 0", got)
	}
}

// The property the whole cursor model rests on.
func TestLineRoundTripAtEveryBoundary(t *testing.T) {
	for _, s := range []string{
		"", "hello", "a\tb", "\t\t", "世界", "é", mixed,
		"\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466x",
		"ab\tcd\t世éf",
	} {
		l := NewLine([]rune(s))
		for i := RuneIdx(0); ; {
			if got := l.RuneAt(l.DisplayCol(i)); got != i {
				t.Errorf("%q: RuneAt(DisplayCol(%d)=%d) = %d, want %d",
					s, i, l.DisplayCol(i), got, i)
			}
			n := l.NextGrapheme(i)
			if n == i {
				break
			}
			i = n
		}
	}
}

func TestLineTabWidthIsConfigurable(t *testing.T) {
	old := TabWidth
	defer func() { TabWidth = old }()

	l := NewLine([]rune("a\tb"))
	if got := l.Width(); got != 9 {
		t.Fatalf("with TabWidth=8, Width() = %d, want 9", got)
	}
	TabWidth = 4
	if got := l.Width(); got != 5 {
		t.Errorf("with TabWidth=4, Width() = %d, want 5 (cache must rebuild)", got)
	}
	if got := l.DisplayCol(2); got != 4 {
		t.Errorf("with TabWidth=4, DisplayCol(2) = %d, want 4", got)
	}
}

func TestLineRunesAreCopied(t *testing.T) {
	src := []rune("hello")
	l := NewLine(src)
	got := l.Runes()
	if string(got) != "hello" {
		t.Fatalf("Runes() = %q, want %q", string(got), "hello")
	}
	got[0] = 'J'
	if string(l.Runes()) != "hello" {
		t.Error("mutating the slice returned by Runes() corrupted the line")
	}
	src[1] = 'X'
	if string(l.Runes()) != "hello" {
		t.Error("mutating the slice passed to NewLine corrupted the line")
	}
}

func TestTabWidthBelowOneIsTreatedAsOne(t *testing.T) {
	old := TabWidth
	defer func() { TabWidth = old }()

	TabWidth = 0
	l := NewLine([]rune("a\tb"))
	if got := l.Width(); got != 3 {
		t.Errorf("with TabWidth=0, Width() = %d, want 3 (tab treated as 1 column)", got)
	}
	TabWidth = -4
	l2 := NewLine([]rune("\t"))
	if got := l2.Width(); got != 1 {
		t.Errorf("with TabWidth=-4, Width() = %d, want 1", got)
	}
}

func TestGraphemeSteppingClampsOutOfRangeInput(t *testing.T) {
	l := NewLine([]rune("ab"))
	if got := l.PrevGrapheme(99); got != 1 {
		t.Errorf("PrevGrapheme(99) = %d, want 1", got)
	}
	if got := l.NextGrapheme(-5); got != 1 {
		t.Errorf("NextGrapheme(-5) = %d, want 1", got)
	}
	if got := l.DisplayCol(-3); got != 0 {
		t.Errorf("DisplayCol(-3) = %d, want 0", got)
	}
}
