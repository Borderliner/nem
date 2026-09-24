package command

import (
	"errors"
	"fmt"

	"github.com/Borderliner/nem/text"
)

// Whitespace and joining: the small commands that tidy a line without
// retyping it.

// RegisterSpace adds the whitespace commands to r.
func RegisterSpace(r *Registry) error {
	cmds := []Command{
		{Name: "just-one-space", Doc: "Leave exactly one space around point.", Fn: justOneSpace},
		{Name: "delete-horizontal-space", Doc: "Delete all spaces and tabs around point.", Fn: deleteHorizontalSpace},
		{Name: "delete-indentation", Doc: "Join this line to the previous one, with one space between.", Fn: deleteIndentation},
		{Name: "back-to-indentation", Doc: "Move to the first non-blank character on the line.", Fn: backToIndentation},
		{Name: "zap-to-char", Doc: "Kill up to and including the next occurrence of a character, ARG times.", Fn: zapToChar},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

func isBlank(r rune) bool { return r == ' ' || r == '\t' }

// blanksAround is the run of spaces and tabs around p on its line.
func blanksAround(b *text.Buffer, p text.Pos) (from, to text.Pos) {
	rs := b.Line(p.Line).View()
	lo, hi := int(p.Col), int(p.Col)
	for lo > 0 && isBlank(rs[lo-1]) {
		lo--
	}
	for hi < len(rs) && isBlank(rs[hi]) {
		hi++
	}
	return text.Pos{Line: p.Line, Col: text.RuneIdx(lo)}, text.Pos{Line: p.Line, Col: text.RuneIdx(hi)}
}

func deleteHorizontalSpace(e Env) error {
	b := e.Buf()
	from, to := blanksAround(b, e.Win().Pt)
	if err := b.Delete(from, to); err != nil {
		return err
	}
	edSetPoint(e, from)
	return nil
}

func justOneSpace(e Env) error {
	b := e.Buf()
	from, to := blanksAround(b, e.Win().Pt)
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if err := b.Delete(from, to); err != nil {
		return err
	}
	if err := b.Insert(from, []rune{' '}); err != nil {
		return err
	}
	edSetPoint(e, text.Pos{Line: from.Line, Col: from.Col + 1})
	return nil
}

// deleteIndentation is M-^: this line joins the end of the previous one. The
// indentation that started it and any blanks that ended the other become one
// space - or none at all, where a space would be wrong: at the start of a
// line, just inside an opening bracket, or before a closing one.
func deleteIndentation(e Env) error {
	b, w := e.Buf(), e.Win()
	ln := w.Pt.Line
	if ln == 0 {
		edSetPoint(e, text.Pos{})
		return nil
	}
	prev := b.Line(ln - 1).View()
	end := len(prev)
	for end > 0 && isBlank(prev[end-1]) {
		end--
	}
	cur := b.Line(ln).View()
	start := indentOf(cur)

	from := text.Pos{Line: ln - 1, Col: text.RuneIdx(end)}
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if err := b.Delete(from, text.Pos{Line: ln, Col: text.RuneIdx(start)}); err != nil {
		return err
	}
	joined := b.Line(ln - 1).View()
	at := int(from.Col)
	space := at > 0 && at < len(joined) &&
		!isOpenBracket(joined[at-1]) && !isCloseBracket(joined[at])
	if space {
		if err := b.Insert(from, []rune{' '}); err != nil {
			return err
		}
	}
	edSetPoint(e, from)
	return nil
}

func isOpenBracket(r rune) bool  { return r == '(' || r == '[' || r == '{' }
func isCloseBracket(r rune) bool { return r == ')' || r == ']' || r == '}' }

func backToIndentation(e Env) error {
	p := e.Win().Pt
	edSetPoint(e, text.Pos{Line: p.Line, Col: text.RuneIdx(indentOf(e.Buf().Line(p.Line).View()))})
	return nil
}

// errZapNotFound is zap-to-char finding no such character.
var errZapNotFound = errors.New("search failed")

// zapToChar is M-z: kill from point through the ARGth occurrence of a
// character, backwards with a negative ARG. The killed text goes on the kill
// ring, as any kill does, so a zap too far is a C-y away from undone.
func zapToChar(e Env) error {
	n, _ := e.Arg()
	if n == 0 {
		return nil
	}
	c, err := e.ReadChar("Zap to char: ", nil)
	if err != nil {
		return err
	}
	b, p := e.Buf(), e.Win().Pt

	target := p
	found := 0
	if n > 0 {
		for q := p; q.Before(b.End()); q = nextRune(b, q) {
			if runeAt(b, q) == c {
				if found++; found == n {
					target = nextRune(b, q)
					break
				}
			}
		}
	} else {
		for q := p; (text.Pos{}).Before(q); {
			q = prevRune(b, q)
			if runeAt(b, q) == c {
				if found++; found == -n {
					target = q
					break
				}
			}
		}
	}
	if target == p {
		return fmt.Errorf("%w: %q", errZapNotFound, c)
	}

	from, to := text.OrderPos(p, target)
	killed := string(b.Text(from, to))
	if err := b.Delete(from, to); err != nil {
		return err
	}
	if n > 0 {
		e.KillForward(killed)
	} else {
		e.KillBackward(killed)
	}
	edSetPoint(e, from)
	return nil
}
