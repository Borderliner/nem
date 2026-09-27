package command

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Borderliner/nem/text"
)

// Replacing: the loop every replace command runs, whatever it matches and
// whether or not it asks about each match, and the commands that replace a
// regular expression rather than a string.

// RegisterReplace adds the replace commands to r. query-replace itself is
// registered with the search commands, where it has always been.
func RegisterReplace(r *Registry) error {
	cmds := []Command{
		{Name: "query-replace-regexp", Doc: "Replace matches of a regexp, asking about each one.", Fn: queryReplaceRegexp},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// queryReplaceRegexp is C-M-%: query-replace for a regular expression, in
// Go's syntax, whose replacement can refer back to what it matched as emacs's
// does - \& for the whole match, \1 to \9 for its groups, \\ for a
// backslash. Matches are found within lines, never across one.
func queryReplaceRegexp(e Env) error {
	if e.Buf().ReadOnly() {
		return text.ErrReadOnly
	}
	pat, err := e.ReadString(ReadOpts{Prompt: "Query replace regexp: ", History: "replace"})
	if err != nil {
		return err
	}
	if pat == "" {
		e.Echo("Nothing to replace")
		return nil
	}
	// Compiled before the replacement is asked for, so a mistake in the
	// pattern is said at once rather than after typing the rest.
	f, err := newRegexpFinder(pat)
	if err != nil {
		return err
	}
	to, err := e.ReadString(ReadOpts{Prompt: fmt.Sprintf("Query replace regexp %s with: ", pat), History: "replace"})
	if err != nil {
		return err
	}
	find, err := f.replacingWith(to)
	if err != nil {
		return err
	}
	return replaceMatches(e, replaceRun{
		find: find,
		ask:  fmt.Sprintf("Query replacing regexp %s with %s (y/n/!/q): ", pat, to),
		from: e.Win().Pt,
	})
}

// regexpFinder finds a regular expression's matches in a buffer, a line at a
// time.
type regexpFinder struct {
	// re is the pattern as typed, folded as emacs's smart case folds it.
	re *regexp.Regexp

	// after finds re's first match starting after the first rune of a
	// string. Searching from the middle of a line cannot simply search the
	// rest of it: ^ would match where the rest begins, and \b would not know
	// what came before. So the search is of the rest with the one rune before
	// it kept for context, and the match is made to start past that rune:
	// \A(?s:.) takes it, and a lazy (?s:.)*? then the fewest runes more that
	// let the pattern match, which is the leftmost match there is.
	after *regexp.Regexp
}

// errReplacement is a replacement using a backslash for something other
// than \&, \1 to \9 or \\, which emacs refuses in the same words.
var errReplacement = errors.New("invalid use of `\\' in replacement text")

// newRegexpFinder compiles pat, case-insensitively while it has no capital
// letters of its own.
func newRegexpFinder(pat string) (*regexpFinder, error) {
	if FoldCaseRegexp(pat) {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("invalid regexp: %w", err)
	}
	// A pattern that compiles alone compiles inside a group - unless it was
	// already at the limit of how large a pattern may be.
	after, err := regexp.Compile(`\A(?s:.)(?s:.)*?(` + pat + `)`)
	if err != nil {
		return nil, fmt.Errorf("invalid regexp: %w", err)
	}
	return &regexpFinder{re: re, after: after}, nil
}

// replacingWith is the finder for matches of f to be replaced with to, an
// emacs replacement: \& is the whole match and \0 too, \1 to \9 its groups,
// and \\ a backslash; anything else after a backslash is refused, as is a
// group the pattern does not have.
//
// The replacement is turned into a template for Go's Expand, in which $ is
// special, so a $ typed is written $$ and stays a dollar sign.
func (f *regexpFinder) replacingWith(to string) (func(*text.Buffer, text.Pos) (match, bool), error) {
	tmpl, err := expandTemplate(to, f.re.NumSubexp())
	if err != nil {
		return nil, err
	}
	return func(b *text.Buffer, from text.Pos) (match, bool) {
		from = b.ClampPos(from)
		for ln := from.Line; ln < b.NumLines(); ln++ {
			rs := b.Line(ln).View()
			base := 0 // the column src starts at
			var src string
			var idx []int
			if off := int(from.Col); ln == from.Line && off > 0 {
				base = off - 1
				src = string(rs[base:])
				if w := f.after.FindStringSubmatchIndex(src); w != nil {
					idx = w[2:] // the pattern's own groups, as re numbers them
				}
			} else {
				src = string(rs)
				idx = f.re.FindStringSubmatchIndex(src)
			}
			if idx == nil {
				continue
			}
			start := base + utf8.RuneCountInString(src[:idx[0]])
			end := start + utf8.RuneCountInString(src[idx[0]:idx[1]])
			return match{
				start: text.Pos{Line: ln, Col: text.RuneIdx(start)},
				end:   text.Pos{Line: ln, Col: text.RuneIdx(end)},
				to:    string(f.re.ExpandString(nil, tmpl, src, idx)),
			}, true
		}
		return match{}, false
	}, nil
}

// expandTemplate turns an emacs replacement into a template for Expand, for
// a pattern with groups groups.
func expandTemplate(to string, groups int) (string, error) {
	var b strings.Builder
	for i := 0; i < len(to); i++ {
		switch c := to[i]; c {
		case '$':
			b.WriteString("$$")
		case '\\':
			if i+1 == len(to) {
				return "", errReplacement
			}
			i++
			switch d := to[i]; {
			case d == '&' || d == '0':
				b.WriteString("${0}")
			case d >= '1' && d <= '9':
				if int(d-'0') > groups {
					return "", fmt.Errorf("replacement text refers to \\%c, and the regexp has %d groups", d, groups)
				}
				b.WriteString("${" + string(d) + "}")
			case d == '\\':
				b.WriteByte('\\')
			default:
				return "", errReplacement
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// FoldCaseRegexp is FoldCase for a regular expression: a capital letter
// makes the search case-sensitive only where it stands for itself. The one
// in \S or \W is part of an escape, and that in \p{Lu} or a group's name
// part of the syntax; none says anything about the case wanted.
func FoldCaseRegexp(pat string) bool {
	for i := 0; i < len(pat); i++ {
		switch {
		case pat[i] == '\\' && i+1 < len(pat):
			i++
			if (pat[i] == 'p' || pat[i] == 'P') && i+1 < len(pat) {
				i++
				if pat[i] == '{' {
					for i < len(pat) && pat[i] != '}' {
						i++
					}
				}
			}
		case strings.HasPrefix(pat[i:], "(?P<") || strings.HasPrefix(pat[i:], "(?<"):
			for i < len(pat) && pat[i] != '>' {
				i++
			}
		default:
			r, size := utf8.DecodeRuneInString(pat[i:])
			if unicode.IsUpper(r) {
				return false
			}
			i += size - 1
		}
	}
	return true
}

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
		msg := fmt.Sprintf("Replaced %d occurrence%s", n, plural(n))
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
