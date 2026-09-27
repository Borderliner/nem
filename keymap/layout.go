package keymap

import "strings"

// Keyboard layouts. With a Persian, Arabic, Hebrew, Russian or Greek layout
// switched on, the key where x is sends ط, or ч, or χ - so Ctrl on it arrives
// as C-ط, which nothing is bound to, and every key binding stops working the
// moment the layout changes. Emacs users meet this too, and reverse-im is
// the usual answer: read the letter back as the Latin key in its place. That
// is what LatinKey does, from tables of where each layout puts its letters.
//
// Only where a key is a command, not text: the editor asks for the Latin key
// when Ctrl or Meta is held, when the key continues a prefix, and wherever a
// letter can only be a command. Typed on its own into a buffer, ط is still ط.

// Layout chooses between the layouts that put one letter on different keys.
type Layout int

const (
	// LayoutAuto reads every layout nem knows. Persian and Arabic keyboards
	// put a few shared letters - ط د ذ ز ظ - on different keys; auto follows
	// the Persian ones.
	LayoutAuto Layout = iota
	// LayoutArabic is auto following the Arabic keyboard for those letters.
	LayoutArabic
	// LayoutOff reads nothing back: keys are what the terminal says.
	LayoutOff
)

// layouts are where each layout puts its letters, as pairs: a US key, then
// the letter that layout sends from it. Shifted letters pair with the
// shifted key - Shift+C is ژ on a Persian keyboard, so ژ pairs with C.
var layouts = map[string]string{
	// Persian, the standard layout: ISIRI 9147, and Windows's Persian
	// (Standard).
	"persian": "qض wص eث rق tف yغ uع iه oخ pح [ج ]چ " +
		"aش sس dی fب gل hا jت kن lم ;ک 'گ " +
		"zظ xط cز vر bذ nد mپ ,و Cژ",
	// Arabic (101). It shares most letters with Persian, on the same keys;
	// ط د ذ ز ظ it puts elsewhere, and it has the Arabic yeh and kaf.
	"arabic": "`ذ qض wص eث rق tف yغ uع iه oخ pح [ج ]د " +
		"aش sس dي fب gل hا jت kن lم ;ك 'ط " +
		"zئ xء cؤ vر nى mة ,و .ز /ظ",
	// Hebrew (SI-1452). q and w send / and ', which are Latin already.
	"hebrew": "eק rר tא yט uו iן oם pפ " +
		"aש sד dג fכ gע hי jח kל lך ;ף " +
		"zז xס cב vה bנ nמ mצ ,ת .ץ",
	// Russian (ЙЦУКЕН).
	"russian": "`ё qй wц eу rк tе yн uг iш oщ pз [х ]ъ " +
		"aф sы dв fа gп hр jо kл lд ;ж 'э " +
		"zя xч cс vм bи nт mь ,б .ю " +
		"~Ё QЙ WЦ EУ RК TЕ YН UГ IШ OЩ PЗ {Х }Ъ " +
		"AФ SЫ DВ FА GП HР JО KЛ LД :Ж \"Э " +
		"ZЯ XЧ CС VМ BИ NТ MЬ <Б >Ю",
	// Ukrainian and Belarusian: the letters they put where Russian has
	// others.
	"ukrainian": "sі ]ї 'є oў SІ }Ї \"Є OЎ",
	// Greek.
	"greek": "wς eε rρ tτ yυ uθ iι oο pπ " +
		"aα sσ dδ fφ gγ hη jξ kκ lλ " +
		"zζ xχ cψ vω bβ nν mμ " +
		"EΕ RΡ TΤ YΥ UΘ IΙ OΟ PΠ AΑ SΣ DΔ FΦ GΓ HΗ JΞ KΚ LΛ " +
		"ZΖ XΧ CΨ VΩ BΒ NΝ MΜ",
}

// latin is each layout's letters mapped to their keys, merged: auto's with
// Persian over Arabic where they differ, arabic's the other way round.
var latin = map[Layout]map[rune]rune{
	LayoutAuto:   merged("arabic", "hebrew", "russian", "ukrainian", "greek", "persian"),
	LayoutArabic: merged("persian", "hebrew", "russian", "ukrainian", "greek", "arabic"),
}

// merged maps the letters of the named layouts to their keys, a later layout
// winning a letter an earlier one also has. Persian and Arabic-Indic digits,
// which those layouts put on the number row, are read as the digits they
// are.
func merged(names ...string) map[rune]rune {
	out := map[rune]rune{}
	for _, name := range names {
		for _, pair := range strings.Fields(layouts[name]) {
			rs := []rune(pair)
			out[rs[1]] = rs[0]
		}
	}
	for d := range rune(10) {
		out['٠'+d] = '0' + d
		out['۰'+d] = '0' + d
	}
	return out
}

// LatinKey is the key of a US keyboard where a layout puts r, and false for
// a rune no layout nem knows puts anywhere - including every Latin one.
func LatinKey(r rune, l Layout) (rune, bool) {
	if r < 0x80 {
		return r, false
	}
	m, ok := latin[l]
	if !ok {
		return r, false
	}
	if k, ok := m[r]; ok {
		return k, true
	}
	return r, false
}
