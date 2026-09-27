package bidi

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	xbidi "golang.org/x/text/unicode/bidi"
)

// latin spells the right-to-left letters the tests use as capital Latin
// letters, so an expected visual string reads the same in any editor, bidi or
// not: "MALS" is سلام drawn right to left. Lowercase is Latin text.
var latin = map[rune]rune{
	'ا': 'A', 'ب': 'B', 'ت': 'T', 'د': 'D', 'ر': 'R', 'س': 'S', 'ف': 'F',
	'ق': 'Q', 'ک': 'K', 'ل': 'L', 'م': 'M', 'ن': 'N', 'و': 'W', 'ه': 'H',
	'ی': 'Y', 'خ': 'X',
	'ש': 'S', 'ל': 'L', 'ו': 'V', 'ם': 'M', 'ע': 'E',
}

// auto asks visual to take the paragraph's direction from its text.
const auto Direction = -1

// visual lays s out as a line of the screen would show it: in the order
// Order gives, with a mirrored character at an odd level drawn as its
// mirror, and the right-to-left letters spelt in Latin.
func visual(s string, dir Direction) string {
	rs := []rune(s)
	if dir == auto {
		dir = ParagraphDirection(rs)
	}
	levels := Levels(rs, dir)
	var b strings.Builder
	for _, i := range Order(levels) {
		r := rs[i]
		if levels[i]&1 == 1 {
			r, _ = Mirror(r)
		}
		if l, ok := latin[r]; ok {
			r = l
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestVisual(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		dir        Direction
		want       string
	}{
		{"pure Persian", "سلام دنیا", auto, "AYND MALS"},
		{"Persian with a Latin word", "سلام nem دنیا", auto, "AYND nem MALS"},
		{"English with a Persian phrase", "I said سلام دنیا today", auto, "I said AYND MALS today"},
		{"Hebrew", "שלום עולם", auto, "MLVE MVLS"},
		{"English with Hebrew", "hello שלום world", auto, "hello MVLS world"},

		// Numbers read left to right whatever is round them. Extended
		// Arabic-Indic digits are European numbers, made Arabic ones by
		// the letters before them (W2); Arabic-Indic digits are Arabic
		// numbers already.
		{"Persian digits", "قیمت ۱۲۳ تومان", RTL, "NAMWT ۱۲۳ TMYQ"},
		{"Latin digits", "سال 2024", auto, "2024 LAS"},
		{"Arabic-Indic digits", "سال ٣٤", auto, "٣٤ LAS"},
		{"Hebrew with digits", "שלום 123", auto, "123 MVLS"},
		{"Persian digits in English", "page ۱۲۳", auto, "page ۱۲۳"},
		{"thousands", "۱,۰۰۰ تومان", RTL, "NAMWT ۱,۰۰۰"},
		{"Latin thousands", "1,000 تومان", RTL, "NAMWT 1,000"},
		{"decimal and percent", "قیمت 12.5% است", RTL, "TSA %12.5 TMYQ"},
		{"plus in English", "call +98 21", auto, "call +98 21"},
		// A browser shows it the same way: the sign is a neutral after the
		// letters, not part of the number.
		{"plus in Persian", "تلفن +98 21", RTL, "21 98+ NFLT"},

		{"exclamation", "سلام!", auto, "!MALS"},
		{"guillemets", "«سلام»", auto, "«MALS»"},
		{"guillemets in English", "he said «سلام»", auto, "he said «MALS»"},
		{"parentheses", "(سلام)", auto, "(MALS)"},

		// N0: brackets take the direction of what they hold, or of the
		// text before them when it agrees, and are never split.
		{"Persian in English parentheses", "the word (سلام) means hello", auto, "the word (MALS) means hello"},
		{"Latin in Persian parentheses", "سلام (hello) دنیا", auto, "AYND (hello) MALS"},
		{"Latin in parentheses first", "(nem) سلام", RTL, "MALS (nem)"},
		{"parentheses going with the text before", "سلام (دنیا) nem", LTR, "(AYND) MALS nem"},
		{"nested brackets", "سلام (دنیا [nem])", auto, "([nem] AYND) MALS"},
		{"nested brackets in English", "nem (سلام [دنیا])", auto, "nem ([AYND] MALS)"},

		// L1: a tab, and the whitespace before it, go back to the
		// paragraph level, so columns stay in the paragraph's order.
		{"tab in right-to-left text", "abc\tdef", RTL, "def\tabc"},
		{"tab in left-to-right text", "سلام\tدنیا", LTR, "MALS\tAYND"},
		{"spaces before a tab", "abc  \tdef", RTL, "def\t  abc"},
		{"leading and trailing spaces", "  سلام  ", LTR, "  MALS  "},
		{"trailing spaces in an open isolate", "سلام \u2066nem  ", RTL, "  nem\u2066 MALS"},

		{"right-to-left isolate", "x \u2067سلام nem\u2069 y", LTR, "x \u2067nem MALS\u2069 y"},
		{"without the isolate", "x سلام nem y", LTR, "x MALS nem y"},
		{"first strong isolate", "x \u2068سلام nem\u2069 y", LTR, "x \u2068nem MALS\u2069 y"},
		{"left-to-right isolate", "سلام \u2066nem دنیا\u2069!", RTL, "!\u2069nem AYND\u2066 MALS"},
		{"override", "\u202eabc\u202c", LTR, "\u202e\u202ccba"},

		// Rune by rune a mark comes out before the letter it is on; a
		// renderer keeps the two together by ordering clusters instead.
		{"mark on a Persian letter", "س\u064eلام", auto, "MAL\u064eS"},
		{"ZWNJ inside a word", "می\u200cخواهم", auto, "MHAWX\u200cYM"},
	} {
		if got := visual(tc.text, tc.dir); got != tc.want {
			t.Errorf("%s: %q laid out as %q, want %q", tc.name, tc.text, got, tc.want)
		}
	}
}

func TestLevels(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		dir        Direction
		want       []uint8
	}{
		{"empty", "", LTR, []uint8{}},
		{"Latin", "nem", LTR, []uint8{0, 0, 0}},
		{"Latin in right-to-left", "nem", RTL, []uint8{2, 2, 2}},
		{"Persian", "سلام", RTL, []uint8{1, 1, 1, 1}},
		{"Persian in left-to-right", "سلام", LTR, []uint8{1, 1, 1, 1}},
		{"digits in Persian", "سال ۱۲", RTL, []uint8{1, 1, 1, 1, 2, 2}},
		{"mark takes its letter's level", "س\u064eلام", LTR, []uint8{1, 1, 1, 1, 1}},
		{"ZWNJ takes the level before it", "می\u200cخواهم", LTR, []uint8{1, 1, 1, 1, 1, 1, 1, 1}},
		{"ZWNJ at the start takes the paragraph's", "\u200cسلام", LTR, []uint8{0, 1, 1, 1, 1}},
		{"embedding controls", "a\u202bb\u202cc", LTR, []uint8{0, 0, 2, 2, 0}},
		{"trailing whitespace in an embedding", "\u202bab  ", LTR, []uint8{0, 2, 2, 0, 0}},
		{"isolate controls at the end", "سلام\u2066 \u2069", LTR, []uint8{1, 1, 1, 1, 0, 0, 0}},
		{"paragraph separator", "ab\u2029سل", RTL, []uint8{2, 2, 1, 1, 1}},
		{"segment separator", "ab\tcd", RTL, []uint8{2, 2, 1, 2, 2}},
		{"unmatched pops", "\u202c\u2069ab", RTL, []uint8{1, 1, 2, 2}},
	} {
		got := Levels([]rune(tc.text), tc.dir)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: Levels(%q, %v) = %v, want %v", tc.name, tc.text, tc.dir, got, tc.want)
		}
	}
}

// TestLevelsDeep nests embeddings past the deepest level UAX #9 allows: the
// ones past it are ignored, and nothing goes above 126.
func TestLevelsDeep(t *testing.T) {
	var b strings.Builder
	for range 70 {
		b.WriteString("\u202b\u202a") // RLE LRE: two levels each time
	}
	b.WriteString("a1")
	rs := []rune(b.String())
	levels := Levels(rs, LTR)
	if got := levels[len(levels)-2:]; !slices.Equal(got, []uint8{125 + 1, 125 + 1}) {
		t.Errorf("letter and digit at depth: levels %v, want [126 126]", got)
	}
	for i, l := range levels {
		if l > maxDepth+1 {
			t.Fatalf("level %d at %d", l, i)
		}
	}
}

func TestParagraphDirection(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Direction
	}{
		{"", LTR},
		{"hello", LTR},
		{"سلام", RTL},
		{"שלום", RTL},
		{"123 سلام", RTL},
		{"۱۲۳ nem", LTR},
		{"!!", LTR},
		{"\u200fnem", RTL},
		{"\u2067nem\u2069 سلام", RTL},
		{"\u2066سلام\u2069 nem", LTR},
		{"\u2067سلام", LTR},
		{"\u2069سلام", RTL},
		{"\u2068\u2067a\u2069\u2069b", LTR},
	} {
		if got := ParagraphDirection([]rune(tc.text)); got != tc.want {
			t.Errorf("ParagraphDirection(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestHasRTL(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"", false},
		{"hello, world", false},
		{"héllo ωorld", false},
		{"سلام", true},
		{"hello שלום", true},
		{"۱۲۳", false},
		{"٣٤", false},
		{"\u200f", true},
		{"\u202eabc", true},
		{"\u2067abc\u2069", true},
		{"\u2068abc\u2069", false},
	} {
		if got := HasRTL([]rune(tc.text)); got != tc.want {
			t.Errorf("HasRTL(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// TestHasRTLEveryCharacter checks HasRTL's shortcut ranges against the
// character database, all of it.
func TestHasRTLEveryCharacter(t *testing.T) {
	for r := rune(0); r <= 0x10FFFF; r++ {
		c := lookup(r)
		want := c == tR || c == tAL || c == tRLE || c == tRLO || c == tRLI
		if got := HasRTL([]rune{r}); got != want {
			t.Fatalf("HasRTL(%U) = %v, want %v", r, got, want)
		}
	}
}

func TestOrder(t *testing.T) {
	for _, tc := range []struct {
		levels []uint8
		want   []int
	}{
		{[]uint8{}, []int{}},
		{[]uint8{0, 0, 0}, []int{0, 1, 2}},
		{[]uint8{1, 1, 1}, []int{2, 1, 0}},
		{[]uint8{0, 1, 1, 0}, []int{0, 2, 1, 3}},
		{[]uint8{1, 2, 2, 1}, []int{3, 1, 2, 0}},
		{[]uint8{2, 2, 2}, []int{0, 1, 2}},
		{[]uint8{0, 2, 2, 0}, []int{0, 1, 2, 3}},
		{[]uint8{1, 1, 2, 2, 1, 3, 3}, []int{6, 5, 4, 2, 3, 1, 0}},
		{[]uint8{0, 125, 125, 0}, []int{0, 2, 1, 3}},
		{[]uint8{0, 126, 126, 0}, []int{0, 1, 2, 3}},
		{[]uint8{125, 126, 126, 125}, []int{3, 1, 2, 0}},
	} {
		if got := Order(tc.levels); !slices.Equal(got, tc.want) {
			t.Errorf("Order(%v) = %v, want %v", tc.levels, got, tc.want)
		}
	}
}

func TestMirror(t *testing.T) {
	for _, tc := range []struct {
		r, want rune
		ok      bool
	}{
		{'(', ')', true},
		{')', '(', true},
		{'[', ']', true},
		{'{', '}', true},
		{'<', '>', true},
		{'>', '<', true},
		{'«', '»', true},
		{'»', '«', true},
		{'‹', '›', true},
		{'≤', '≥', true},
		{'∈', '∋', true},
		{'⁅', '⁆', true},
		{'「', '」', true},
		{'a', 'a', false},
		{'/', '/', false},
		{'؟', '؟', false},
		{'س', 'س', false},
		{-1, -1, false},
		{0x10FFFF + 1, 0x10FFFF + 1, false},
	} {
		if got, ok := Mirror(tc.r); got != tc.want || ok != tc.ok {
			t.Errorf("Mirror(%U) = %U, %v, want %U, %v", tc.r, got, ok, tc.want, tc.ok)
		}
	}
}

// TestMirrorIsAnInvolution checks the table: what a character mirrors to
// mirrors back to it, so a pair of brackets drawn mirrored is still a pair.
func TestMirrorIsAnInvolution(t *testing.T) {
	for _, e := range mirrors {
		if back, ok := Mirror(e.m); !ok || back != e.r {
			t.Errorf("%U mirrors to %U, which mirrors to %U", e.r, e.m, back)
		}
	}
}

func TestTablesMatchClasses(t *testing.T) {
	if unicodeVersion != xbidi.UnicodeVersion {
		t.Errorf("tables.go is from Unicode %s and the bidi classes from %s: run go generate", unicodeVersion, xbidi.UnicodeVersion)
	}
	for _, b := range brackets {
		if c := classOf(b.r); c != tON {
			t.Errorf("bracket %U is of class %d, not ON", b.r, c)
		}
		if pair, open, _ := bracket(b.pair); pair != b.r || open == b.open {
			t.Errorf("bracket %U pairs with %U, which does not pair back", b.r, b.pair)
		}
	}
}

// line is a mixed line of the kind nem draws: Persian with Latin words,
// digits of both kinds, parentheses and a ZWNJ - 120 runes.
const line = "در فایل main.go تابع run را با دستور go run اجرا کنید؛ خروجی (۱۲۳۴ خط) در ترمینال nem می\u200cآید و 45% آن را اینجا می\u200cبینید."

func BenchmarkLevels(b *testing.B) {
	rs := []rune(line)
	if len(rs) != 120 {
		b.Fatalf("the line is %d runes", len(rs))
	}
	b.ReportAllocs()
	for b.Loop() {
		Levels(rs, RTL)
	}
}

// BenchmarkLine is all a renderer does for one line: resolve it and order
// it.
func BenchmarkLine(b *testing.B) {
	rs := []rune(line)
	b.ReportAllocs()
	for b.Loop() {
		dir := ParagraphDirection(rs)
		Order(Levels(rs, dir))
	}
}

func BenchmarkHasRTL(b *testing.B) {
	rs := []rune(strings.Repeat("func (b *Buffer) Insert(s string) { b.text = append(b.text, s...) } ", 2)[:120])
	hasRTL := HasRTL // called as the renderer will, not inlined into the loop
	b.ReportAllocs()
	for b.Loop() {
		if hasRTL(rs) {
			b.Fatal("a Go line has right-to-left text")
		}
	}
}

func ExampleOrder() {
	rs := []rune("سال 2024")
	levels := Levels(rs, ParagraphDirection(rs))
	for _, i := range Order(levels) {
		fmt.Printf("%c", rs[i])
	}
	fmt.Println()
	// Output: 2024 لاس
}
