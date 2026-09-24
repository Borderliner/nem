package command

import (
	"fmt"
	"unicode"

	"github.com/Borderliner/nem/text"
)

// RegisterInfo adds the commands that report on the text without changing it.
func RegisterInfo(r *Registry) error {
	cmds := []Command{
		{Name: "count-words", Doc: "Count the lines, words and characters in the region, or the buffer.", Fn: countWords},
		{Name: "what-cursor-position", Doc: "Describe the character at point and where point is.", Fn: whatCursorPosition},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// countWords is M-=, in emacs's words: "Region has 3 lines, 12 words, and 57
// characters." A line the region ends at the very start of is not counted,
// as in emacs, since none of it is in the region.
func countWords(e Env) error {
	b, w := e.Buf(), e.Win()
	what := "Buffer"
	from, to := text.Pos{}, b.End()
	if b.MarkActive() {
		what = "Region"
		from, to = text.OrderPos(w.Pt, b.Mark())
	}
	lines := to.Line - from.Line
	if to.Col > 0 || to.Line == from.Line && from != to {
		lines++
	}
	words, chars := 0, 0
	inWord := false
	for _, r := range b.Text(from, to) {
		chars++
		if isWordRune(r) {
			if !inWord {
				words++
			}
			inWord = true
		} else {
			inWord = false
		}
	}
	e.Echo("%s has %s, %s, and %s.", what, plural2(lines, "line"), plural2(words, "word"), plural2(chars, "character"))
	return nil
}

// plural2 is "1 line" or "3 lines".
func plural2(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// whatCursorPosition is C-x =: "Char: a (97, #o141, #x61) point=5 of 20
// (20%) column=4", as emacs puts it, with point counted in characters from
// one.
func whatCursorPosition(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	offset, total := 0, 0
	for i := range b.NumLines() {
		n := int(b.Line(i).Len())
		if i < p.Line {
			offset += n + 1
		}
		total += n + 1
	}
	total-- // no newline after the last line
	offset += int(p.Col)
	col := b.Line(p.Line).DisplayCol(p.Col)

	if p == b.End() {
		e.Echo("point=%d of %d (EOB) column=%d", offset+1, total+1, col)
		return nil
	}
	r := runeAt(b, p)
	pct := 0
	if total > 0 {
		pct = offset * 100 / total
	}
	e.Echo("Char: %s (%d, #o%o, #x%x) point=%d of %d (%d%%) column=%d",
		charName(r), r, r, r, offset+1, total+1, pct, col)
	return nil
}

// charName is how emacs shows a character: itself when it can be seen, a key
// name when it is whitespace or a control.
func charName(r rune) string {
	switch {
	case r == ' ':
		return "SPC"
	case r == '\t':
		return "TAB"
	case r == '\n':
		return "C-j"
	case r < 0x20:
		return "C-" + string(rune(r+'@'+0x20))
	case !unicode.IsPrint(r):
		return fmt.Sprintf("U+%04X", r)
	}
	return string(r)
}
