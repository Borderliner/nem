package command

import (
	"fmt"

	"github.com/Borderliner/nem/text"
)

// Replacing: the loop every replace command runs, whatever it matches and
// whether or not it asks about each match.

// match is one occurrence a replace command found: where it is, and what it
// is to become.
type match struct {
	start, end text.Pos
	to         string
}

// replaceRun is one run of a replace command.
type replaceRun struct {
	// find is the first match at or after from in b, if there is one.
	find func(b *text.Buffer, from text.Pos) (match, bool)

	// ask is the question put about each match, answered y, n, ! or q. Empty
	// replaces every match without asking, as ! does.
	ask string

	// from is where the run starts. With bounded set it stops at end, which
	// is kept on the same text as replacements before it change the lengths
	// of lines.
	from    text.Pos
	end     text.Pos
	bounded bool
}

var queryReplaceAnswers = []rune{'y', 'n', '!', 'q'}

// replaceMatches goes through the matches r finds, from r.from on, replacing
// each one - or those the user says yes to - and says how many it replaced.
//
// A match the buffer will not let be changed - in the part of a file listing
// that is not a name, say - is passed over rather than offered, as emacs does
// with query-replace-skip-read-only. Stopping there would leave every match
// after it unreplaced.
//
// A pattern can match nothing at all, as ^ does at the start of each line.
// The same empty match would then be found again for ever where the last one
// was replaced, so after one the search moves on a character first, and an
// empty match straight after any other is passed over - Go's own ReplaceAll
// has the same rule, and x* over "abc" gives -a-b-c- rather than hanging.
func replaceMatches(e Env, r replaceRun) error {
	b := e.Buf()
	at, end := r.from, r.end
	all := r.ask == ""
	n, skipped := 0, 0
	var last text.Pos // where the last match ended, when haveLast is set
	haveLast := false
	done := func() {
		msg := fmt.Sprintf("Replaced %d occurrences", n)
		if skipped > 0 {
			msg += fmt.Sprintf(" (skipped %d that cannot be edited)", skipped)
		}
		e.Echo("%s", msg)
	}

	for {
		m, ok := r.find(b, at)
		if !ok {
			break
		}
		empty := m.start == m.end
		if r.bounded && (end.Before(m.end) || (empty && !m.start.Before(end))) {
			break
		}
		if empty && haveLast && m.start == last {
			if at, ok = nextPos(b, m.start); !ok {
				break
			}
			haveLast = false
			continue
		}

		offered := CanReplace(b, m.start, m.end, m.to)
		replace := all && offered
		if !offered {
			skipped++
		} else if !all {
			e.Win().Pt = m.start
			c, err := e.ReadChar(r.ask, queryReplaceAnswers)
			if err != nil {
				return err
			}
			switch c {
			case 'y':
				replace = true
			case '!':
				replace, all = true, true
			case 'q':
				done()
				return nil
			}
		}

		at = m.end
		if replace {
			var err error
			if at, err = ReplaceMatch(b, m.start, m.end, m.to); err != nil {
				return err
			}
			end = shiftPast(end, m.end, at)
			n++
		}
		if offered {
			e.Win().Pt = at
		}
		last, haveLast = at, true
		if empty {
			if at, ok = nextPos(b, at); !ok {
				break
			}
		}
	}

	done()
	return nil
}

// nextPos is the position one rune after p, on the next line when p ends
// one; false at the end of the buffer.
func nextPos(b *text.Buffer, p text.Pos) (text.Pos, bool) {
	if p.Col < b.Line(p.Line).Len() {
		return text.Pos{Line: p.Line, Col: p.Col + 1}, true
	}
	if p.Line+1 < b.NumLines() {
		return text.Pos{Line: p.Line + 1}, true
	}
	return p, false
}

// shiftPast carries p, a position at or after old - the end of text just
// replaced - to where the same text is now that the replacement ends at now.
func shiftPast(p, old, now text.Pos) text.Pos {
	if p.Before(old) {
		return p
	}
	if p.Line == old.Line {
		return text.Pos{Line: now.Line, Col: now.Col + p.Col - old.Col}
	}
	return text.Pos{Line: p.Line + now.Line - old.Line, Col: p.Col}
}
