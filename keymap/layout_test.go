package keymap

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Every pair is a US key then one letter, and no layout puts two letters on
// one key or one letter on two.
func TestLayoutTablesAreWellFormed(t *testing.T) {
	for name, table := range layouts {
		keys, letters := map[rune]bool{}, map[rune]bool{}
		for _, pair := range strings.Fields(table) {
			if utf8.RuneCountInString(pair) != 2 {
				t.Errorf("%s: %q is not a key and a letter", name, pair)
				continue
			}
			rs := []rune(pair)
			if rs[0] >= 0x80 || rs[1] < 0x80 {
				t.Errorf("%s: %q is not a US key then a letter of the layout", name, pair)
			}
			if keys[rs[0]] || letters[rs[1]] {
				t.Errorf("%s: %q repeats a key or a letter", name, pair)
			}
			keys[rs[0]], letters[rs[1]] = true, true
		}
	}
}

func TestLatinKey(t *testing.T) {
	for _, tc := range []struct {
		r      rune
		layout Layout
		want   rune
		ok     bool
	}{
		{'ط', LayoutAuto, 'x', true},   // C-x on a Persian keyboard
		{'ب', LayoutAuto, 'f', true},   // C-f
		{'ل', LayoutAuto, 'g', true},   // C-g
		{'ی', LayoutAuto, 'd', true},   // Persian yeh
		{'ي', LayoutAuto, 'd', true},   // Arabic yeh
		{'ژ', LayoutAuto, 'C', true},   // Shift+C
		{'ط', LayoutArabic, '\'', true}, // where Arabic puts it
		{'ء', LayoutAuto, 'x', true},   // C-x on an Arabic keyboard
		{'ס', LayoutAuto, 'x', true},   // Hebrew
		{'ч', LayoutAuto, 'x', true},   // Russian
		{'Ч', LayoutAuto, 'X', true},
		{'і', LayoutAuto, 's', true}, // Ukrainian
		{'χ', LayoutAuto, 'x', true}, // Greek
		{'۲', LayoutAuto, '2', true}, // C-x 2 on a Persian keyboard
		{'٣', LayoutAuto, '3', true},
		{'x', LayoutAuto, 'x', false}, // Latin already
		{'日', LayoutAuto, '日', false},
		{'ط', LayoutOff, 'ط', false},
	} {
		got, ok := LatinKey(tc.r, tc.layout)
		if got != tc.want || ok != tc.ok {
			t.Errorf("LatinKey(%q, %d) = %q, %v; want %q, %v", tc.r, tc.layout, got, ok, tc.want, tc.ok)
		}
	}
}
