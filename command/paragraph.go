package command

import (
	"strings"
	"unicode/utf8"

	"github.com/Borderliner/nem/text"
)

// Paragraphs: runs of non-blank lines, separated by blank ones. M-{ and M-}
// move between them, and M-q re-wraps one to the fill column.

// FillColumn is the width M-q fills paragraphs to, in columns. It is set from
// the fill-column setting; seventy is emacs's default, and it leaves room for
// a diff's markers or an email's quoting inside eighty columns.
var FillColumn = 70

// RegisterParagraph adds the paragraph commands to r.
func RegisterParagraph(r *Registry) error {
	cmds := []Command{
		{Name: "forward-paragraph", Doc: "Move to the end of the paragraph, ARG paragraphs forward.", Fn: forwardParagraph},
		{Name: "backward-paragraph", Doc: "Move to the start of the paragraph, ARG paragraphs back.", Fn: backwardParagraph},
		{Name: "fill-paragraph", Doc: "Re-wrap the paragraph at point to the fill column.", Fn: fillParagraph},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// blankLine reports whether line i holds nothing but spaces and tabs.
func blankLine(b *text.Buffer, i int) bool {
	rs := b.Line(i).View()
	return indentOf(rs) == len(rs)
}

func forwardParagraph(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return paragraphsBack(e, -n)
	}
	return paragraphsForward(e, n)
}

func backwardParagraph(e Env) error {
	n, _ := e.Arg()
	if n < 0 {
		return paragraphsForward(e, -n)
	}
	return paragraphsBack(e, n)
}

// paragraphsForward moves to the blank line after the paragraph, n times,
// stopping at the end of the buffer.
func paragraphsForward(e Env, n int) error {
	b := e.Buf()
	last := b.NumLines() - 1
	i := e.Win().Pt.Line
	for range n {
		for i < last && blankLine(b, i) {
			i++
		}
		for i <= last && !blankLine(b, i) {
			i++
		}
		if i > last {
			edSetPoint(e, b.End())
			return nil
		}
	}
	edSetPoint(e, text.Pos{Line: i})
	return nil
}

// paragraphsBack moves to the blank line before the paragraph, n times,
// stopping at the start of the buffer.
func paragraphsBack(e Env, n int) error {
	b, p := e.Buf(), e.Win().Pt
	i := p.Line
	if p.Col == 0 {
		i-- // at the start of a line, that line is not behind point
	}
	for range n {
		for i >= 0 && blankLine(b, i) {
			i--
		}
		for i >= 0 && !blankLine(b, i) {
			i--
		}
		if i < 0 {
			edSetPoint(e, text.Pos{})
			return nil
		}
	}
	edSetPoint(e, text.Pos{Line: i})
	return nil
}

// fillParagraph is M-q: the paragraph around point, its words re-flowed into
// lines no wider than FillColumn.
//
// A paragraph of comments is filled as comments. The first line's indentation
// and comment marker are its prefix; the paragraph is the lines around it
// that carry the same prefix and something after it; and every filled line
// gets the prefix back. That is what makes M-q useful in code, where the
// paragraphs worth re-wrapping are nearly all comments.
func fillParagraph(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	prefix := fillPrefix(b, p.Line)
	belongs := func(i int) bool {
		s := b.Line(i).String()
		return strings.HasPrefix(s, prefix) && strings.TrimSpace(s[len(prefix):]) != ""
	}
	if !belongs(p.Line) {
		return nil // a blank line, or a comment line with nothing in it
	}
	first, last := p.Line, p.Line
	for first > 0 && belongs(first-1) {
		first--
	}
	for last < b.NumLines()-1 && belongs(last+1) {
		last++
	}

	// Point is carried across by how many non-blank runes precede it in the
	// paragraph's text: that count is the same before and after re-flowing.
	marker := 0
	var words []string
	for i := first; i <= last; i++ {
		body := b.Line(i).String()[len(prefix):]
		if i == p.Line {
			runes := []rune(b.Line(i).String())
			for k := utf8.RuneCountInString(prefix); k < int(p.Col) && k < len(runes); k++ {
				if !isBlank(runes[k]) {
					marker++
				}
			}
		} else if i < p.Line {
			for _, r := range body {
				if !isBlank(r) {
					marker++
				}
			}
		}
		words = append(words, strings.Fields(body)...)
	}

	lines := fillWords(words, prefix, FillColumn)
	filled := strings.Join(lines, "\n")
	from := text.Pos{Line: first}
	to := text.Pos{Line: last, Col: b.Line(last).Len()}
	if string(b.Text(from, to)) == filled {
		return nil // already filled; no undo step for nothing
	}
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if err := b.Delete(from, to); err != nil {
		return err
	}
	if err := b.Insert(from, []rune(filled)); err != nil {
		return err
	}
	edSetPoint(e, fillPoint(lines, first, utf8.RuneCountInString(prefix), marker))
	return nil
}

// fillPrefix is what every line of line i's paragraph starts with: its
// indentation, and a comment marker with the space after it when the file's
// kind of comment starts the line.
func fillPrefix(b *text.Buffer, i int) string {
	rs := b.Line(i).View()
	n := indentOf(rs)
	if cs, ok := knownComment(b.Path()); ok && cs.end == "" {
		if strings.HasPrefix(string(rs[n:]), cs.start) {
			n += utf8.RuneCountInString(cs.start)
			for n < len(rs) && isBlank(rs[n]) {
				n++
			}
		}
	}
	return string(rs[:n])
}

// fillWords lays words into lines that start with prefix and fit width
// columns. A word longer than the width gets a line to itself rather than
// being broken: a URL split in two is no longer a URL.
func fillWords(words []string, prefix string, width int) []string {
	pl := text.NewLine([]rune(prefix))
	prefixW := int(pl.Width())
	var lines []string
	cur, curW := "", 0
	for _, w := range words {
		ww := text.NewLine([]rune(w))
		wW := int(ww.Width())
		switch {
		case cur == "":
			cur, curW = w, wW
		case prefixW+curW+1+wW <= width:
			cur, curW = cur+" "+w, curW+1+wW
		default:
			lines = append(lines, prefix+cur)
			cur, curW = w, wW
		}
	}
	if cur != "" {
		lines = append(lines, prefix+cur)
	}
	return lines
}

// fillPoint finds where the marker-th non-blank rune of the filled lines is,
// the paragraph's first line being first.
func fillPoint(lines []string, first, prefixLen, marker int) text.Pos {
	for i, l := range lines {
		rs := []rune(l)
		for k := prefixLen; k < len(rs); k++ {
			if isBlank(rs[k]) {
				continue
			}
			if marker == 0 {
				return text.Pos{Line: first + i, Col: text.RuneIdx(k)}
			}
			marker--
		}
	}
	last := len(lines) - 1
	return text.Pos{Line: first + last, Col: text.RuneIdx(utf8.RuneCountInString(lines[last]))}
}
