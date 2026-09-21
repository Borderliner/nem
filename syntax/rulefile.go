package syntax

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// A .nemrc file describes a language nem has no hand-written lexer for.
//
// The format exists because the alternative was bundling nano's .nanorc files,
// and those are GPL: exactly one of the forty-four is CC0, nineteen say "GPL
// version 3 or newer", and the rest carry no statement and fall under nano's
// own licence. Embedding them would have decided this project's licensing as a
// side effect of a highlighting feature. So these rules are written from each
// language's grammar instead. A keyword list is a fact about a language; the
// regex that matches it is ours.
//
// The format differs from nano's in one way that matters, and it is the reason
// it is worth having a second one: a rule states its CLASS rather than a
// colour.
//
//	syntax python "\.pyi?$"
//	header "^#!.*python"
//	comment "#"
//
//	class keyword  "\b(def|class|return|if|else)\b"
//	class string   start="\"\"\"" end="\"\"\""
//	class comment  "#.*$"
//
// nano's files name colours, which carry no shared meaning across them -
// strings are red in java and green in rust - so reading one means guessing the
// class from what the pattern matches. Stating it removes the guess, and keeps
// the class-to-theme indirection that lets nem carry both a light and a dark
// palette.
//
// Patterns are RE2 as Go writes it, not POSIX ERE. Inside a quoted argument
// only \" is special, so a pattern is written exactly as it would be in a Go
// regexp literal.

// classNames maps the names a .nemrc may use to the classes nem renders.
//
// An unknown name is a load error rather than a default. These files ship
// inside the binary, so a typo is a bug of ours to be caught in CI, not a user's
// input to be tolerated - and silently colouring `keyowrd` rules as plain text
// would look like the rule simply not working.
var classNames = map[string]Class{
	"keyword":     Keyword,
	"string":      String,
	"comment":     Comment,
	"number":      Number,
	"function":    Function,
	"type":        Type,
	"constant":    Constant,
	"operator":    Operator,
	"punctuation": Punctuation,
}

func classNameList() string {
	names := make([]string, 0, len(classNames))
	for n := range classNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Rule is one compiled pattern and the class it paints.
//
// Either re is set, for a rule confined to one line, or startRe and endRe are,
// for a region that may span lines.
type Rule struct {
	re             *regexp.Regexp
	startRe, endRe *regexp.Regexp
	class          Class
}

func (r Rule) multiline() bool { return r.startRe != nil }

// RuleSet is one language, and is itself a Lexer.
type RuleSet struct {
	name string
	// comment is the line-comment introducer, kept because a comment-toggle
	// command will need it and nothing else records it.
	comment string

	files   []*regexp.Regexp // matched against the file name
	headers []*regexp.Regexp // matched against the first line, for shebangs
	rules   []Rule
}

// Name reports the language.
func (rs *RuleSet) Name() string { return rs.name }

// Comment reports the line-comment introducer, or "".
func (rs *RuleSet) Comment() string { return rs.comment }

// Rules reports how many rules the set carries, for tests that assert coverage.
func (rs *RuleSet) Rules() int { return len(rs.rules) }

// matchesFile reports whether this language claims a file name.
func (rs *RuleSet) matchesFile(base string) bool {
	for _, re := range rs.files {
		if re.MatchString(base) {
			return true
		}
	}
	return false
}

// matchesHeader reports whether this language claims a first line, which is how
// an extensionless script is recognised from its shebang.
func (rs *RuleSet) matchesHeader(first string) bool {
	if first == "" {
		return false
	}
	for _, re := range rs.headers {
		if re.MatchString(first) {
			return true
		}
	}
	return false
}

// ParseRuleFile reads one .nemrc.
//
// Unlike the nanorc reader, a problem here is fatal: these files are ours and
// ship in the binary, so a bad pattern is a build-time mistake rather than a
// third-party file to be tolerated. Failing loudly is what makes the "every
// embedded rule compiles" test meaningful.
func ParseRuleFile(name string, r io.Reader) (*RuleSet, error) {
	var (
		rs     *RuleSet
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
		args, err := splitArgs(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
		}
		if len(args) == 0 {
			continue
		}
		switch args[0].text {
		case "syntax":
			if len(args) < 3 {
				return nil, fmt.Errorf("%s:%d: syntax needs a name and at least one file pattern", name, lineNo)
			}
			if rs != nil {
				// One language per file. A second directive used to replace the
				// first silently, which cost sh its extension patterns and
				// looked like the language simply not matching.
				return nil, fmt.Errorf("%s:%d: a second syntax directive; put every file pattern on the first", name, lineNo)
			}
			rs = &RuleSet{name: args[1].text}
			for _, a := range args[2:] {
				re, err := regexp.Compile(a.text)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: file pattern %q: %w", name, lineNo, a.text, err)
				}
				rs.files = append(rs.files, re)
			}
		case "header":
			if rs == nil {
				return nil, fmt.Errorf("%s:%d: header before syntax", name, lineNo)
			}
			for _, a := range args[1:] {
				re, err := regexp.Compile(a.text)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: header %q: %w", name, lineNo, a.text, err)
				}
				rs.headers = append(rs.headers, re)
			}
		case "comment":
			if rs == nil {
				return nil, fmt.Errorf("%s:%d: comment before syntax", name, lineNo)
			}
			if len(args) > 1 {
				rs.comment = args[1].text
			}
		case "class":
			if rs == nil {
				return nil, fmt.Errorf("%s:%d: class before syntax", name, lineNo)
			}
			if err := rs.addRule(args); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
			}
		default:
			return nil, fmt.Errorf("%s:%d: unknown directive %q", name, lineNo, args[0].text)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if rs == nil {
		return nil, fmt.Errorf("%s: no syntax directive", name)
	}
	if len(rs.rules) == 0 {
		return nil, fmt.Errorf("%s: %s has no rules", name, rs.name)
	}
	return rs, nil
}

// addRule parses one class directive.
func (rs *RuleSet) addRule(args []arg) error {
	if len(args) < 3 {
		return fmt.Errorf("class needs a name and a pattern")
	}
	class, ok := classNames[args[1].text]
	if !ok {
		return fmt.Errorf("unknown class %q; known classes are %s", args[1].text, classNameList())
	}

	// class NAME start="..." end="..."
	if strings.HasPrefix(args[2].text, "start=") && !args[2].quoted {
		var startPat, endPat string
		for i := 2; i < len(args); i++ {
			switch {
			case args[i].text == "start=" && !args[i].quoted && i+1 < len(args) && args[i+1].quoted:
				startPat = args[i+1].text
				i++
			case args[i].text == "end=" && !args[i].quoted && i+1 < len(args) && args[i+1].quoted:
				endPat = args[i+1].text
				i++
			}
		}
		if startPat == "" || endPat == "" {
			return fmt.Errorf("class %s: start/end rule missing one half", args[1].text)
		}
		startRe, err := regexp.Compile(startPat)
		if err != nil {
			return fmt.Errorf("class %s start %q: %w", args[1].text, startPat, err)
		}
		endRe, err := regexp.Compile(endPat)
		if err != nil {
			return fmt.Errorf("class %s end %q: %w", args[1].text, endPat, err)
		}
		rs.rules = append(rs.rules, Rule{startRe: startRe, endRe: endRe, class: class})
		return nil
	}

	if !args[2].quoted {
		return fmt.Errorf("class %s: pattern must be quoted", args[1].text)
	}
	re, err := regexp.Compile(args[2].text)
	if err != nil {
		return fmt.Errorf("class %s pattern %q: %w", args[1].text, args[2].text, err)
	}
	rs.rules = append(rs.rules, Rule{re: re, class: class})
	return nil
}

// arg is one word of a directive: a bare word, or a quoted string with its
// quotes removed.
type arg struct {
	text   string
	quoted bool
}

// splitArgs splits a directive line.
//
// Inside a quoted argument only \" is special, and it yields a literal quote.
// Every other backslash is kept, so a pattern reads exactly as it would in a Go
// regexp literal: \b(def|class)\b rather than \\b(def|class)\\b. Requiring
// doubled backslashes in a file that is almost entirely regexes would make
// every rule harder to read and easier to get wrong.
//
// A bare word ends at a blank or a quote, so start="..." yields the word
// start= followed by the quoted value.
func splitArgs(line string) ([]arg, error) {
	var out []arg
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		if line[i] == '"' {
			var sb strings.Builder
			j := i + 1
			closed := false
			for j < len(line) {
				if line[j] == '\\' && j+1 < len(line) && line[j+1] == '"' {
					sb.WriteByte('"')
					j += 2
					continue
				}
				if line[j] == '"' {
					closed = true
					j++
					break
				}
				sb.WriteByte(line[j])
				j++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted argument")
			}
			out = append(out, arg{text: sb.String(), quoted: true})
			i = j
			continue
		}
		j := i
		for j < len(line) && line[j] != ' ' && line[j] != '\t' && line[j] != '"' {
			j++
		}
		// start= and end= keep their trailing = so the caller can tell them
		// from an ordinary bare word.
		out = append(out, arg{text: line[i:j]})
		i = j
	}
	return out, nil
}
