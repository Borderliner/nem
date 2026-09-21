package command

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

func buf(t *testing.T, lines ...string) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	s := ""
	for i, l := range lines {
		if i > 0 {
			s += "\n"
		}
		s += l
	}
	if s != "" {
		if err := b.Insert(text.Pos{}, []rune(s)); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}
	b.BreakUndo()
	return b
}

// The case that motivates including combining marks: a decomposed "é" must not
// split a word. Without Mn/Mc in isWordRune, forward-word stops between the
// base letter and its accent.
func TestCombiningMarkDoesNotBreakAWord(t *testing.T) {
	if !isWordRune('́') {
		t.Fatal("isWordRune(U+0301 combining acute) = false; decomposed accents would split words")
	}
	b := buf(t, "café next")
	got := forwardWordPos(b, text.Pos{})
	// "cafe" + combining acute = 5 runes, so the word ends at rune index 5.
	if want := (text.Pos{Line: 0, Col: 5}); got != want {
		t.Errorf("forwardWordPos = %v, want %v (end of decomposed 'café')", got, want)
	}
}

func TestForwardWordPos(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		from  text.Pos
		want  text.Pos
	}{
		{"start of word", []string{"foo bar"}, text.Pos{}, text.Pos{Line: 0, Col: 3}},
		{"mid word", []string{"foo bar"}, text.Pos{Line: 0, Col: 1}, text.Pos{Line: 0, Col: 3}},
		{"skips spaces", []string{"foo bar"}, text.Pos{Line: 0, Col: 3}, text.Pos{Line: 0, Col: 7}},
		{"crosses lines", []string{"foo", "bar"}, text.Pos{Line: 0, Col: 3}, text.Pos{Line: 1, Col: 3}},
		{"punctuation", []string{"a.b"}, text.Pos{}, text.Pos{Line: 0, Col: 1}},
		{"digits are words", []string{"a1b2 x"}, text.Pos{}, text.Pos{Line: 0, Col: 4}},
		{"at buffer end", []string{"foo"}, text.Pos{Line: 0, Col: 3}, text.Pos{Line: 0, Col: 3}},
		{"cjk", []string{"日本語 x"}, text.Pos{}, text.Pos{Line: 0, Col: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := forwardWordPos(buf(t, tc.lines...), tc.from); got != tc.want {
				t.Errorf("forwardWordPos = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBackwardWordPos(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		from  text.Pos
		want  text.Pos
	}{
		{"end of word", []string{"foo bar"}, text.Pos{Line: 0, Col: 7}, text.Pos{Line: 0, Col: 4}},
		{"mid word", []string{"foo bar"}, text.Pos{Line: 0, Col: 6}, text.Pos{Line: 0, Col: 4}},
		{"skips spaces back", []string{"foo bar"}, text.Pos{Line: 0, Col: 4}, text.Pos{Line: 0, Col: 0}},
		{"crosses lines back", []string{"foo", "bar"}, text.Pos{Line: 1, Col: 0}, text.Pos{Line: 0, Col: 0}},
		{"at buffer start", []string{"foo"}, text.Pos{}, text.Pos{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := backwardWordPos(buf(t, tc.lines...), tc.from); got != tc.want {
				t.Errorf("backwardWordPos = %v, want %v", got, tc.want)
			}
		})
	}
}

// Round trip: from the start of a word, forward then backward returns to it.
func TestWordScanRoundTrip(t *testing.T) {
	b := buf(t, "alpha beta gamma", "delta epsilon")
	for _, start := range []text.Pos{
		{Line: 0, Col: 0}, {Line: 0, Col: 6}, {Line: 0, Col: 11}, {Line: 1, Col: 0},
	} {
		end := forwardWordPos(b, start)
		if got := backwardWordPos(b, end); got != start {
			t.Errorf("round trip from %v: forward to %v, back to %v", start, end, got)
		}
	}
}
