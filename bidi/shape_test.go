package bidi

import (
	"fmt"
	"slices"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestShape(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []rune
	}{
		{"empty", "", []rune{}},
		{"salam", "سلام", []rune{0xFEB3, 0xFEE0, 0xFE8E, 0xFEE1}},
		{"ketab", "کتاب", []rune{0xFB90, 0xFE98, 0xFE8E, 0xFE8F}},
		{"panjere", "پنجره", []rune{0xFB58, 0xFEE8, 0xFEA0, 0xFEAE, 0xFEE9}},
		{"gol", "گل", []rune{0xFB94, 0xFEDE}},
		{"yek", "یک", []rune{0xFBFE, 0xFB8F}},
		{"mamnun", "ممنون", []rune{0xFEE3, 0xFEE4, 0xFEE8, 0xFEEE, 0xFEE5}},
		{"chay", "چای", []rune{0xFB7C, 0xFE8E, 0xFBFC}},
		{"mozhe", "مژه", []rune{0xFEE3, 0xFB8B, 0xFEE9}},
		{"yeh in the middle", "بیب", []rune{0xFE91, 0xFBFF, 0xFE90}},
		{"teh marbuta", "مة", []rune{0xFEE3, 0xFE94}},
		{"alef madda keeps beh apart", "آب", []rune{0xFE81, 0xFE8F}},
		{"lam and alef stay two letters", "لا", []rune{0xFEDF, 0xFE8E}},
		{"a letter alone", "ب", []rune{0xFE8F}},
		{"hamza", "ء", []rune{0xFE80}},

		// ZWNJ breaks the join, ZWJ and tatweel make one.
		{"mikhaham", "می\u200cخواهم", []rune{0xFEE3, 0xFBFD, 0x200C, 0xFEA7, 0xFEEE, 0xFE8D, 0xFEEB, 0xFEE2}},
		{"ZWJ", "ب\u200d", []rune{0xFE91, 0x200D}},
		{"tatweel after", "بـ", []rune{0xFE91, 0x0640}},
		{"tatweel before", "ـب", []rune{0x0640, 0xFE90}},
		{"tatweel between", "بـب", []rune{0xFE91, 0x0640, 0xFE90}},

		// Harakat are passed over, and left as they are.
		{"fatha", "ب\u064eب", []rune{0xFE91, 0x064E, 0xFE90}},
		{"kasra", "ک\u0650تاب", []rune{0xFB90, 0x0650, 0xFE98, 0xFE8E, 0xFE8F}},
		{"shadda and fatha", "م\u0651\u064eا", []rune{0xFEE3, 0x0651, 0x064E, 0xFE8E}},

		// Urdu letters, and ones with fewer forms than their joining.
		{"tteh", "ٹٹٹ", []rune{0xFB68, 0xFB69, 0xFB67}},
		{"yeh barree", "بے", []rune{0xFE91, 0xFBAF}},
		{"heh goal", "ہہ", []rune{0xFBA8, 0xFBA7}},
		{"heh doachashmee", "ھ", []rune{0xFBAA}},
		{"heh with yeh above", "خانۀ", []rune{0xFEA7, 0xFE8E, 0xFEE7, 0xFBA5}},
		{"ddal and rreh", "ڈڑ", []rune{0xFB88, 0xFB8C}},
		{"noon ghunna at the end", "بں", []rune{0xFE91, 0xFB9F}},
		{"noon ghunna has no initial form", "ںب", []rune{0x06BA, 0xFE90}},
		{"a letter with no forms still joins", "ب\u0620ب", []rune{0xFE91, 0x0620, 0xFE90}},

		{"Latin and digits untouched", "nem سلام 123 ۱۲۳!", append([]rune("nem "), 0xFEB3, 0xFEE0, 0xFE8E, 0xFEE1, ' ', '1', '2', '3', ' ', '۱', '۲', '۳', '!')},
		{"Hebrew untouched", "שלום", []rune("שלום")},
		{"a Latin letter breaks the join", "بxب", []rune{0xFE8F, 'x', 0xFE8F}},
	} {
		rs := []rune(tc.text)
		in := slices.Clone(rs)
		got := Shape(rs)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: Shape(%q) = %s, want %s", tc.name, tc.text, hexRunes(got), hexRunes(tc.want))
		}
		if !slices.Equal(rs, in) {
			t.Errorf("%s: Shape changed its argument", tc.name)
		}
	}
}

func hexRunes(rs []rune) string {
	var s []string
	for _, r := range rs {
		s = append(s, fmt.Sprintf("%04X", r))
	}
	return fmt.Sprint(s)
}

// TestShapeKeepsLength checks every letter that has forms, alone and between
// others: one rune in, one rune out, and nothing but Arabic letters touched.
func TestShapeKeepsLength(t *testing.T) {
	for r := rune(firstShaped); r <= lastShaped; r++ {
		for _, rs := range [][]rune{{r}, {'a', r, 'b'}, {0x0628, r, 0x0628}, {r, r, r}} {
			got := Shape(rs)
			if len(got) != len(rs) {
				t.Fatalf("Shape(%q) is %d runes, want %d", string(rs), len(got), len(rs))
			}
			for i := range rs {
				if got[i] != rs[i] && !(0x0600 <= rs[i] && rs[i] <= 0x06FF) {
					t.Errorf("Shape(%q) changed %U", string(rs), rs[i])
				}
			}
		}
	}
}

// TestForms checks the presentation forms: each is in one of the
// presentation form blocks, and is a compatibility equivalent of its letter,
// which is what the database says it is a form of.
func TestForms(t *testing.T) {
	for i, fs := range forms {
		base := rune(firstShaped + i)
		for f, form := range fs {
			if form == 0 {
				continue
			}
			r := rune(form)
			if !(0xFB50 <= r && r <= 0xFDFF || 0xFE70 <= r && r <= 0xFEFF) {
				t.Errorf("form %d of %U is %U, outside the presentation forms", f, base, r)
			}
			if a, b := norm.NFKC.String(string(r)), norm.NFKC.String(string(base)); a != b {
				t.Errorf("form %d of %U is %U, which is %q, not %q", f, base, r, a, b)
			}
		}
		if j := joiningOf(base); j == joinR && (fs[initial] != 0 || fs[medial] != 0) {
			t.Errorf("right-joining %U has an initial or medial form", base)
		}
	}

	// The letters of Arabic itself, and those Persian and Urdu add, all
	// have their forms.
	var letters []rune
	for r := rune(0x0621); r <= 0x064A; r++ {
		if r < 0x063B || r > 0x0640 {
			letters = append(letters, r)
		}
	}
	letters = append(letters, []rune("پچژکگیٹڈڑںہھےۀ")...)
	for _, r := range letters {
		if forms[r-firstShaped][isolated] == 0 {
			t.Errorf("%U (%c) has no isolated form", r, r)
		}
	}
}

func TestJoiningOf(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want joining
	}{
		{'a', joinU},
		{' ', joinU},
		{'۱', joinU},
		{0x0621, joinU}, // hamza
		{0x0627, joinR}, // alef
		{0x0628, joinD}, // beh
		{0x06CC, joinD}, // Farsi yeh
		{0x0698, joinR}, // jeh
		{0x0640, joinC}, // tatweel
		{0x200D, joinC}, // ZWJ
		{0x200C, joinU}, // ZWNJ
		{0x064E, joinT}, // fatha
		{0x0670, joinT}, // superscript alef
		{0x06D6, joinT}, // small high ligature
		{0x0300, joinT}, // combining grave
		{0x200F, joinT}, // RLM, a format character
		{0xA872, joinL}, // Phags-pa superfixed ra
	} {
		if got := joiningOf(tc.r); got != tc.want {
			t.Errorf("joiningOf(%U) = %d, want %d", tc.r, got, tc.want)
		}
	}
}

func BenchmarkShape(b *testing.B) {
	rs := []rune(line)
	b.ReportAllocs()
	for b.Loop() {
		Shape(rs)
	}
}
