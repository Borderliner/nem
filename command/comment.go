package command

import (
	"strings"
	"unicode/utf8"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// Commenting: M-; turns the lines of the region, or the current line, into
// comments, or back again when they all are already.
//
// That is not quite emacs's comment-dwim, which without a region adds a
// comment at the end of the line. Toggling whole lines is what nearly every
// other editor does with its comment key, and it is the thing people reach
// for: silencing a block of code. An empty line still gets a comment started
// on it, which is emacs's behaviour there too.

// commentSyntax is how a kind of file writes a comment: a start marker, and an
// end marker for languages that have only block comments.
type commentSyntax struct{ start, end string }

// hash is the comment written where nem does not know the language: the most
// widely understood marker, and the one scripts and configuration use.
var hash = commentSyntax{start: "#"}

// languageOf is what an Env that knows the user's languages offers: the
// editor's does, and a command reaches it through this rather than through
// Env itself, so a test's Env need not.
type languageOf interface {
	LanguageOf(b *text.Buffer) *syntax.Language
}

// commentFor is how b's language writes a comment, or # where nem does not
// know the language.
func commentFor(e Env, b *text.Buffer) commentSyntax {
	if cs, ok := knownComment(e, b); ok {
		return cs
	}
	return hash
}

// knownComment is how b's language writes a comment, and false when nem
// would only be guessing. It is the comment the language's definition lists
// first - the user's definition, where there is one - so a language someone
// adds comments as it colours, with nothing else to write.
func knownComment(e Env, b *text.Buffer) (commentSyntax, bool) {
	var l *syntax.Language
	if lo, ok := e.(languageOf); ok {
		l = lo.LanguageOf(b)
	} else {
		first := ""
		if b.NumLines() > 0 {
			first = b.Line(0).String()
		}
		l = syntax.Builtin().Detect(b.Path(), first)
	}
	if l == nil {
		return commentSyntax{}, false
	}
	start, end, ok := l.Comment()
	return commentSyntax{start: start, end: end}, ok
}

// RegisterComment adds the comment commands to r.
func RegisterComment(r *Registry) error {
	return r.Register(Command{
		Name:        "comment-dwim",
		Doc:         "Comment or uncomment the lines of the region, or the current line.",
		Fn:          commentDwim,
		Interactive: true,
	})
}

func commentDwim(e Env) error {
	b, w := e.Buf(), e.Win()
	cs := commentFor(e, b)

	first, last := w.Pt.Line, w.Pt.Line
	if b.MarkActive() {
		lo, hi := text.OrderPos(w.Pt, b.Mark())
		first, last = lo.Line, hi.Line
		// A region ending at the start of a line does not reach into it:
		// selecting whole lines by dragging to the next line's start is the
		// usual way to select them.
		if hi.Col == 0 && hi.Line > lo.Line {
			last--
		}
	}

	if !b.MarkActive() && strings.TrimSpace(b.Line(first).String()) == "" {
		return commentStart(e, cs)
	}

	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if linesCommented(b, first, last, cs) {
		return uncommentLines(e, first, last, cs)
	}
	return commentLines(e, first, last, cs)
}

// commentStart begins a comment on an empty line, at its indentation, with
// point where the comment's text goes.
func commentStart(e Env, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	at := text.Pos{Line: w.Pt.Line, Col: b.Line(w.Pt.Line).Len()}
	ins := cs.start + " "
	if cs.end != "" {
		ins += " " + cs.end
	}
	if err := b.Insert(at, []rune(ins)); err != nil {
		return err
	}
	edSetPoint(e, text.Pos{Line: at.Line, Col: at.Col + text.RuneIdx(utf8.RuneCountInString(cs.start)+1)})
	return nil
}

// indentOf is how many runes of leading space and tab a line has.
func indentOf(rs []rune) int {
	n := 0
	for n < len(rs) && (rs[n] == ' ' || rs[n] == '\t') {
		n++
	}
	return n
}

// linesCommented reports whether every non-blank line in [first, last] starts,
// after its indentation, with the comment marker. Blank lines do not count
// either way, so a commented block with a gap in it still uncomments.
func linesCommented(b *text.Buffer, first, last int, cs commentSyntax) bool {
	any := false
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		ind := indentOf(rs)
		if ind == len(rs) {
			continue
		}
		if !strings.HasPrefix(string(rs[ind:]), cs.start) {
			return false
		}
		any = true
	}
	return any
}

// commentLines comments every non-blank line in [first, last], with all the
// markers in one column - the least indentation among them - so the block
// still reads as one.
func commentLines(e Env, first, last int, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	col := -1
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		if ind := indentOf(rs); ind < len(rs) && (col < 0 || ind < col) {
			col = ind
		}
	}
	if col < 0 {
		return nil
	}
	open := []rune(cs.start + " ")
	for i := first; i <= last; i++ {
		l := b.Line(i)
		if indentOf(l.View()) == int(l.Len()) {
			continue // blank lines stay blank
		}
		if cs.end != "" {
			if err := b.Insert(text.Pos{Line: i, Col: l.Len()}, []rune(" "+cs.end)); err != nil {
				return err
			}
		}
		if err := b.Insert(text.Pos{Line: i, Col: text.RuneIdx(col)}, open); err != nil {
			return err
		}
		if i == w.Pt.Line && int(w.Pt.Col) >= col {
			w.Pt.Col += text.RuneIdx(len(open))
		}
	}
	return nil
}

// uncommentLines removes the marker, and one space after it, from every
// commented line in [first, last], and the end marker where there is one.
func uncommentLines(e Env, first, last int, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		ind := indentOf(rs)
		body := string(rs[ind:])
		if !strings.HasPrefix(body, cs.start) {
			continue
		}
		if cs.end != "" {
			trimmed := strings.TrimRight(body, " \t")
			if strings.HasSuffix(trimmed, cs.end) {
				cut := strings.TrimSuffix(trimmed, cs.end)
				cut = strings.TrimSuffix(cut, " ")
				from := text.Pos{Line: i, Col: text.RuneIdx(ind + utf8.RuneCountInString(cut))}
				if err := b.Delete(from, text.Pos{Line: i, Col: b.Line(i).Len()}); err != nil {
					return err
				}
			}
		}
		n := utf8.RuneCountInString(cs.start)
		if rest := b.Line(i).View()[ind+n:]; len(rest) > 0 && rest[0] == ' ' {
			n++
		}
		if err := b.Delete(text.Pos{Line: i, Col: text.RuneIdx(ind)}, text.Pos{Line: i, Col: text.RuneIdx(ind + n)}); err != nil {
			return err
		}
		if i == w.Pt.Line && int(w.Pt.Col) > ind {
			w.Pt.Col = text.RuneIdx(max(ind, int(w.Pt.Col)-n))
		}
	}
	w.Pt = b.ClampPos(w.Pt)
	return nil
}
