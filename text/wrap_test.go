package text

import (
	"slices"
	"testing"
)

// rowsOf is the text of each row a line folds into.
func rowsOf(s string, width ColIdx) []string {
	l := NewLine([]rune(s))
	starts := l.WrapRows(width)
	var out []string
	for i, st := range starts {
		end := RuneIdx(l.Len())
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		out = append(out, string(l.runes[st:end]))
	}
	return out
}

// A line folds after a space where it can, keeping words whole; a word
// longer than a row breaks inside itself; a space reaching the edge stays
// on its row.
func TestWrapRows(t *testing.T) {
	for _, tc := range []struct {
		line  string
		width ColIdx
		want  []string
	}{
		{"", 10, []string{""}},
		{"hello world", 20, []string{"hello world"}},
		{"abcd", 4, []string{"abcd"}},
		{"hello world", 8, []string{"hello ", "world"}},
		{"hello world foo", 11, []string{"hello world ", "foo"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"ab abcdefghij", 4, []string{"ab ", "abcd", "efgh", "ij"}},
		{"one two three four", 9, []string{"one two ", "three ", "four"}},
		{"\tx", 4, []string{"\t", "x"}},
		{"日本語", 3, []string{"日", "本", "語"}},
		{"日本", 1, []string{"日", "本"}},
		{"a  b", 2, []string{"a  ", "b"}},
	} {
		if got := rowsOf(tc.line, tc.width); !slices.Equal(got, tc.want) {
			t.Errorf("%q at %d: rows %q, want %q", tc.line, tc.width, got, tc.want)
		}
	}
}

// Every row but a glyph wider than the whole row fits in the width, and the
// rows together are the whole line, in order.
func TestWrapRowsFitAndCoverTheLine(t *testing.T) {
	lines := []string{
		"The quick brown fox jumps over the lazy dog, twice over.",
		"func main() {\tfmt.Println(\"سلام دنیا\", 42)\t}",
		"日本語のテキストと English words mixed together here",
		"ééé 👩‍💻👩‍💻 aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for _, s := range lines {
		for width := ColIdx(1); width <= 30; width++ {
			l := NewLine([]rune(s))
			starts := l.WrapRows(width)
			if starts[0] != 0 || !slices.IsSorted(starts) {
				t.Fatalf("%q at %d: starts %v", s, width, starts)
			}
			for i, st := range starts {
				end := RuneIdx(l.Len())
				if i+1 < len(starts) {
					end = starts[i+1]
				}
				if st >= end && l.Len() > 0 {
					t.Fatalf("%q at %d: row %d is empty", s, width, i)
				}
				// The row's width, less a blank hanging past the edge.
				last := end
				for last > st && (l.runes[last-1] == ' ' || l.runes[last-1] == '\t') {
					last--
				}
				w := l.DisplayCol(last) - l.DisplayCol(st)
				if w > width && l.NextGrapheme(st) < last {
					t.Errorf("%q at %d: row %d, %q, is %d wide", s, width, i, string(l.runes[st:end]), w)
				}
			}
		}
	}
}

// The rows are kept, and measured again once the line changes.
func TestWrapRowsFollowAnEdit(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("hello world")); err != nil {
		t.Fatal(err)
	}
	if got := b.Line(0).WrapRows(8); !slices.Equal(got, []RuneIdx{0, 6}) {
		t.Fatalf("rows %v", got)
	}
	if err := b.Insert(Pos{Col: 0}, []rune("oh ")); err != nil {
		t.Fatal(err)
	}
	if got := b.Line(0).WrapRows(8); !slices.Equal(got, []RuneIdx{0, 9}) {
		t.Errorf("after an edit the rows are %v, want [0 9]: oh hello / world", got)
	}
	if got := RowOf([]RuneIdx{0, 6}, 5); got != 0 {
		t.Errorf("RowOf 5 = %d", got)
	}
	if got := RowOf([]RuneIdx{0, 6}, 6); got != 1 {
		t.Errorf("RowOf 6 = %d", got)
	}
	if got := RowOf([]RuneIdx{0, 6}, 11); got != 1 {
		t.Errorf("RowOf the end = %d", got)
	}
}
