package nanorc

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Borderliner/nem/syntax"
)

// rule is one compiled colour rule.
//
// Either re is set, for a rule confined to a line, or startRe and endRe are,
// for one that spans lines.
type rule struct {
	re             *regexp.Regexp
	startRe, endRe *regexp.Regexp
	class          syntax.Class
}

func (r rule) multiline() bool { return r.startRe != nil }

// Syntax is one language's rules.
type Syntax struct {
	Name string
	// Comment is the line-comment introducer the file declares, kept because a
	// future comment-toggle command needs it and nothing else records it.
	Comment string

	files   []*regexp.Regexp // match against the file name
	headers []*regexp.Regexp // match against the first line, for shebangs
	rules   []rule
}

// token is one word of a directive line: a bare word, or a quoted string with
// its quotes removed.
type token struct {
	text   string
	quoted bool
}

// tokenize splits a directive line the way nano does.
//
// nano's quoting is not shell quoting, and assuming it is truncates a quarter
// of the corpus. A quoted argument ends at the first double quote FOLLOWED BY
// end-of-line or a blank - so in
//
//	color brightblue "//[^"]*$|(^|[[:blank:]])//.*"
//
// the quote inside [^"] is followed by ']' and is therefore not a terminator,
// while the final one is. Splitting on the first bare quote instead cuts the
// pattern to "//[^", which no longer compiles.
//
// Nothing is unescaped: a backslash in a .nanorc pattern belongs to the regex,
// including \" which means a literal quote.
func tokenize(line string) []token {
	var out []token
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		if line[i] == '"' {
			next, val := scanQuotedArg(line, i)
			out = append(out, token{text: val, quoted: true})
			i = next
			continue
		}
		// A bare word. It stops at a quote as well as at a blank so that
		// start="..." yields the word start= and then the quoted value.
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '"' {
			j++
		}
		out = append(out, token{text: line[i:j]})
		i = j
	}
	return out
}

// scanQuotedArg returns the index after the argument and its contents.
//
// An argument with no terminator runs to end of line, which nano also accepts
// and which several of its own files rely on.
func scanQuotedArg(line string, i int) (next int, val string) {
	i++ // opening quote
	for j := i; j < len(line); j++ {
		if line[j] != '"' {
			continue
		}
		if j+1 >= len(line) || line[j+1] == ' ' || line[j+1] == '\t' {
			return j + 1, line[i:j]
		}
	}
	return len(line), line[i:]
}

// translate rewrites POSIX ERE into what Go's RE2 understands.
//
// Two differences matter across nano's corpus.
//
// \< and \> are GNU's start- and end-of-word assertions; RE2 spells both \b.
//
// The subtler one is bracket expressions. POSIX treats a backslash inside
// [...] as an ordinary character, so [^"\] means "not a quote and not a
// backslash" - a shape a quarter of these files use for string literals. RE2
// reads the same \] as an escaped bracket, swallows the class terminator, and
// the pattern fails to compile. Doubling backslashes inside a class restores
// the POSIX meaning.
//
// Nothing else needs translating: the corpus has no backreferences and no
// lookahead, which are the constructs RE2 cannot express at all.
func translate(pat string) string {
	pat = strings.NewReplacer(`\<`, `\b`, `\>`, `\b`).Replace(pat)

	var sb strings.Builder
	sb.Grow(len(pat) + 8)
	inClass := false
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		if !inClass {
			switch {
			case c == '\\' && i+1 < len(pat):
				sb.WriteByte(c)
				i++
				sb.WriteByte(pat[i])
			case c == '[':
				inClass = true
				sb.WriteByte(c)
				j := i + 1
				if j < len(pat) && pat[j] == '^' {
					sb.WriteByte('^')
					j++
				}
				// POSIX: a ] immediately after [ or [^ is a literal member.
				if j < len(pat) && pat[j] == ']' {
					sb.WriteString(`\]`)
					j++
				}
				i = j - 1
			default:
				sb.WriteByte(c)
			}
			continue
		}
		switch {
		case c == '\\':
			sb.WriteString(`\\`) // literal in POSIX, an escape in RE2
		case c == '[' && i+1 < len(pat) && pat[i+1] == ':':
			// A named class such as [:alnum:] - copy it whole.
			if k := strings.Index(pat[i:], ":]"); k >= 0 {
				sb.WriteString(pat[i : i+k+2])
				i += k + 1
			} else {
				sb.WriteByte(c)
			}
		case c == ']':
			inClass = false
			sb.WriteByte(c)
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

func compile(pat string, ignoreCase bool) (*regexp.Regexp, error) {
	p := translate(pat)
	if ignoreCase {
		p = "(?i)" + p
	}
	return regexp.Compile(p)
}

// Parse reads one .nanorc file.
//
// A pattern that will not compile is skipped on its own: nano's files are third
// party and a single unsupported regex must not cost a language its other
// twelve rules. The skipped patterns are returned so a caller can report them
// without the failure being silent.
func Parse(name string, r io.Reader) (*Syntax, []error, error) {
	var (
		s      *Syntax
		skips  []error
		lineNo int
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		toks := tokenize(line)
		if len(toks) == 0 {
			continue
		}
		switch toks[0].text {
		case "syntax":
			if len(toks) < 2 {
				return nil, skips, fmt.Errorf("%s:%d: syntax needs a name", name, lineNo)
			}
			s = &Syntax{Name: toks[1].text}
			for _, t := range toks[2:] {
				re, err := compile(t.text, false)
				if err != nil {
					skips = append(skips, fmt.Errorf("%s:%d: file pattern %q: %w", name, lineNo, t.text, err))
					continue
				}
				s.files = append(s.files, re)
			}
		case "header":
			if s == nil {
				continue
			}
			for _, t := range toks[1:] {
				re, err := compile(t.text, false)
				if err != nil {
					skips = append(skips, fmt.Errorf("%s:%d: header %q: %w", name, lineNo, t.text, err))
					continue
				}
				s.headers = append(s.headers, re)
			}
		case "comment":
			if s != nil && len(toks) > 1 {
				s.Comment = toks[1].text
			}
		case "color", "icolor":
			if s == nil {
				continue
			}
			if err := s.addColour(toks, toks[0].text == "icolor"); err != nil {
				skips = append(skips, fmt.Errorf("%s:%d: %w", name, lineNo, err))
			}
		default:
			// magic, linter, formatter, tabgives and anything nano adds later.
			// Ignoring them is silent by design: nem has no use for them and a
			// warning per file per launch would be noise.
		}
	}
	if err := sc.Err(); err != nil {
		return nil, skips, fmt.Errorf("%s: %w", name, err)
	}
	if s == nil {
		return nil, skips, fmt.Errorf("%s: no syntax directive", name)
	}
	return s, skips, nil
}

// addColour parses one color/icolor directive onto the syntax.
func (s *Syntax) addColour(toks []token, ignoreCase bool) error {
	// toks[0] is color/icolor, toks[1] the colour name, which is discarded -
	// see the package comment for why.
	if len(toks) < 3 {
		return fmt.Errorf("colour rule needs a pattern")
	}
	rest := toks[2:]

	// start="..." end="..."
	if rest[0].text == "start=" || strings.HasPrefix(rest[0].text, "start=") {
		var startPat, endPat string
		for i := 0; i < len(rest); i++ {
			switch {
			case strings.HasPrefix(rest[i].text, "start=") && !rest[i].quoted:
				if v := strings.TrimPrefix(rest[i].text, "start="); v != "" {
					startPat = v
				} else if i+1 < len(rest) && rest[i+1].quoted {
					startPat = rest[i+1].text
					i++
				}
			case strings.HasPrefix(rest[i].text, "end=") && !rest[i].quoted:
				if v := strings.TrimPrefix(rest[i].text, "end="); v != "" {
					endPat = v
				} else if i+1 < len(rest) && rest[i+1].quoted {
					endPat = rest[i+1].text
					i++
				}
			}
		}
		if startPat == "" || endPat == "" {
			return fmt.Errorf("start/end rule missing one half")
		}
		startRe, err := compile(startPat, ignoreCase)
		if err != nil {
			return fmt.Errorf("start %q: %w", startPat, err)
		}
		endRe, err := compile(endPat, ignoreCase)
		if err != nil {
			return fmt.Errorf("end %q: %w", endPat, err)
		}
		s.rules = append(s.rules, rule{
			startRe: startRe, endRe: endRe,
			class: classifyMultiline(startPat),
		})
		return nil
	}

	re, err := compile(rest[0].text, ignoreCase)
	if err != nil {
		return fmt.Errorf("pattern %q: %w", rest[0].text, err)
	}
	s.rules = append(s.rules, rule{re: re, class: classify(rest[0].text)})
	return nil
}
