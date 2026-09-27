package project

import (
	"fmt"
	"path"
	"regexp"
	"regexp/syntax"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Queries. A search is typed as ripgrep's command line has it: options first,
// then the pattern, which is the rest of what was typed, spaces and all:
//
//	-t go -w count           count as a word, in Go files
//	-i -g '!vendor/**' TODO  TODO in any case, outside vendor
//	-F a.b(c)                a.b(c) as text, not a regexp
//
// The options are ripgrep's, and mean what they mean there: -i, -s and -S for
// the case, -w, -F, -t and -T for file types, -g for globs, -u to search
// files .gitignore leaves out too, -e or -- before a pattern that starts
// with a dash. Case is smart unless an option says otherwise, as it is in
// every nem search: ignored unless the pattern has a capital in it.

// Case says how a query treats case.
type Case int

const (
	// CaseSmart ignores case unless the pattern has a capital letter in it.
	CaseSmart Case = iota
	// CaseSensitive is -s.
	CaseSensitive
	// CaseIgnore is -i.
	CaseIgnore
)

// Query is a search, as typed.
type Query struct {
	// Pattern is what is looked for: a regexp, or text with Fixed.
	Pattern string
	// Fixed is -F: the pattern is text.
	Fixed bool
	// Word is -w: only whole words match.
	Word bool
	Case Case
	// Types and NotTypes are -t and -T: file types searched and passed over.
	Types, NotTypes []string
	// Globs are -g, in order; one starting with ! leaves out what it matches.
	Globs []string
	// NoIgnore is -u: files .gitignore leaves out are searched too.
	NoIgnore bool

	globs []glob
}

// glob is a -g compiled, and whether it leaves out what it matches.
type glob struct {
	re      *regexp.Regexp
	exclude bool
}

// fileTypes are the types -t and -T know, by the globs their files match -
// ripgrep's names for the common ones.
var fileTypes = map[string][]string{
	"c":          {"*.c", "*.h"},
	"cpp":        {"*.cpp", "*.cc", "*.cxx", "*.c++", "*.hpp", "*.hh", "*.hxx", "*.h", "*.inl"},
	"cs":         {"*.cs"},
	"css":        {"*.css", "*.scss", "*.sass", "*.less"},
	"csv":        {"*.csv"},
	"docker":     {"Dockerfile", "Dockerfile.*", "*.dockerfile"},
	"elisp":      {"*.el"},
	"go":         {"*.go"},
	"html":       {"*.html", "*.htm", "*.xhtml"},
	"java":       {"*.java"},
	"js":         {"*.js", "*.jsx", "*.mjs", "*.cjs", "*.vue"},
	"json":       {"*.json", "*.jsonl", "*.json5"},
	"kotlin":     {"*.kt", "*.kts"},
	"lua":        {"*.lua"},
	"make":       {"Makefile", "makefile", "GNUmakefile", "*.mk", "*.mak"},
	"markdown":   {"*.md", "*.markdown", "*.mdx"},
	"md":         {"*.md", "*.markdown", "*.mdx"},
	"nix":        {"*.nix"},
	"php":        {"*.php"},
	"proto":      {"*.proto"},
	"py":         {"*.py", "*.pyi"},
	"rb":         {"*.rb", "Gemfile", "Rakefile", "*.gemspec"},
	"ruby":       {"*.rb", "Gemfile", "Rakefile", "*.gemspec"},
	"rust":       {"*.rs"},
	"sh":         {"*.sh", "*.bash", "*.zsh", "*.fish", ".bashrc", ".zshrc", ".profile"},
	"sql":        {"*.sql"},
	"swift":      {"*.swift"},
	"toml":       {"*.toml"},
	"ts":         {"*.ts", "*.tsx", "*.mts", "*.cts"},
	"txt":        {"*.txt"},
	"typescript": {"*.ts", "*.tsx", "*.mts", "*.cts"},
	"xml":        {"*.xml", "*.xsd", "*.xsl", "*.svg"},
	"yaml":       {"*.yaml", "*.yml"},
	"zig":        {"*.zig"},
}

// FileTypes lists the names -t and -T know.
func FileTypes() []string {
	names := make([]string, 0, len(fileTypes))
	for n := range fileTypes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseQuery reads a query typed as ripgrep's command line: options, then
// the pattern. An option it does not know, or one missing its value, is an
// error; the pattern may be empty.
func ParseQuery(s string) (Query, error) {
	var q Query
	i := 0
	for {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		// A pattern starts at the first word that is not an option: one
		// not starting with a dash, or a dash on its own.
		if i >= len(s) || s[i] != '-' || i+1 == len(s) || s[i+1] == ' ' {
			break
		}
		tok, next := word(s, i)
		i = next
		// value is the option's value: after = in a long option, the rest
		// of a short one's word, or else the next word.
		value := func(inline string, has bool) (string, error) {
			if has {
				return inline, nil
			}
			for i < len(s) && s[i] == ' ' {
				i++
			}
			if i >= len(s) {
				return "", fmt.Errorf("%s needs a value", tok)
			}
			v, n := word(s, i)
			i = n
			return v, nil
		}
		switch {
		case tok == "--":
			for i < len(s) && s[i] == ' ' {
				i++
			}
			q.Pattern = s[i:]
			return q.compile()
		case strings.HasPrefix(tok, "--"):
			name, inline, has := strings.Cut(tok[2:], "=")
			var err error
			switch name {
			case "ignore-case":
				q.Case = CaseIgnore
			case "case-sensitive":
				q.Case = CaseSensitive
			case "smart-case":
				q.Case = CaseSmart
			case "word-regexp":
				q.Word = true
			case "fixed-strings":
				q.Fixed = true
			case "no-ignore":
				q.NoIgnore = true
			case "type":
				var v string
				if v, err = value(inline, has); err == nil {
					q.Types = append(q.Types, v)
				}
			case "type-not":
				var v string
				if v, err = value(inline, has); err == nil {
					q.NotTypes = append(q.NotTypes, v)
				}
			case "glob":
				var v string
				if v, err = value(inline, has); err == nil {
					q.Globs = append(q.Globs, v)
				}
			case "regexp":
				var v string
				if v, err = value(inline, has); err == nil {
					q.Pattern = v
				}
			default:
				return q, fmt.Errorf("unknown option %s", tok)
			}
			if err != nil {
				return q, err
			}
		default:
			// Short options, which run together: -wi is -w -i. One taking a
			// value takes the rest of the word, or else the next word.
			for k := 1; k < len(tok); k++ {
				c := tok[k]
				rest, has := tok[k+1:], k+1 < len(tok)
				var err error
				switch c {
				case 'i':
					q.Case = CaseIgnore
				case 's':
					q.Case = CaseSensitive
				case 'S':
					q.Case = CaseSmart
				case 'w':
					q.Word = true
				case 'F':
					q.Fixed = true
				case 'u':
					q.NoIgnore = true
				case 't', 'T', 'g', 'e':
					var v string
					if v, err = value(rest, has); err != nil {
						return q, fmt.Errorf("-%c needs a value", c)
					}
					switch c {
					case 't':
						q.Types = append(q.Types, v)
					case 'T':
						q.NotTypes = append(q.NotTypes, v)
					case 'g':
						q.Globs = append(q.Globs, v)
					case 'e':
						q.Pattern = v
					}
					k = len(tok) // the value took the rest
				default:
					return q, fmt.Errorf("unknown option -%c", c)
				}
			}
		}
	}
	if q.Pattern == "" {
		q.Pattern = s[i:]
	}
	return q.compile()
}

// word is the word of s starting at i, and where it ends. Quotes group, as
// in a shell: '!vendor/**' or "a b" are one word, without their quotes.
func word(s string, i int) (string, int) {
	var b strings.Builder
	for i < len(s) && s[i] != ' ' {
		switch c := s[i]; c {
		case '\'', '"':
			end := strings.IndexByte(s[i+1:], c)
			if end < 0 {
				b.WriteString(s[i+1:])
				return b.String(), len(s)
			}
			b.WriteString(s[i+1 : i+1+end])
			i += end + 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), i
}

// compile checks the query's file types and compiles its globs.
func (q Query) compile() (Query, error) {
	for _, t := range slices.Concat(q.Types, q.NotTypes) {
		if _, ok := fileTypes[t]; !ok {
			return q, fmt.Errorf("unknown file type %q", t)
		}
	}
	q.globs = nil
	for _, g := range q.Globs {
		exclude := strings.HasPrefix(g, "!")
		re, err := globRegexp(strings.TrimPrefix(g, "!"))
		if err != nil {
			return q, fmt.Errorf("bad glob %q", g)
		}
		q.globs = append(q.globs, glob{re, exclude})
	}
	return q, nil
}

// globRegexp compiles a glob as .gitignore reads one. Without a slash it
// matches a file's name at any depth; with one, its path from the top. **
// is any number of directories, * and ? anything but a slash, [...] a
// class.
func globRegexp(g string) (*regexp.Regexp, error) {
	anchored := strings.Contains(strings.TrimSuffix(g, "/"), "/")
	g = strings.TrimPrefix(g, "/")
	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(g); i++ {
		switch c := g[i]; {
		case strings.HasPrefix(g[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(g[i:], "/**") && i+3 == len(g):
			b.WriteString("(?:/.*)?")
			i += 2
		case strings.HasPrefix(g[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '[':
			end := strings.IndexByte(g[i+1:], ']')
			if end < 0 {
				b.WriteString(`\[`)
				continue
			}
			class := g[i+1 : i+1+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i += end + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	// A glob naming a directory matches everything in it.
	b.WriteString("(?:/.*)?$")
	return regexp.Compile(b.String())
}

// Keep reports whether the query searches the file at rel, a path from the
// top of what is searched: it is of a type -t names, if any, and of none -T
// names, and the last glob matching it does not leave it out - and matches
// it, when any glob is not one that leaves out.
func (q Query) Keep(rel string) bool {
	base := path.Base(rel)
	if len(q.Types) > 0 && !slices.ContainsFunc(q.Types, func(t string) bool { return isType(t, base) }) {
		return false
	}
	if slices.ContainsFunc(q.NotTypes, func(t string) bool { return isType(t, base) }) {
		return false
	}
	whitelist := false
	for i := len(q.globs) - 1; i >= 0; i-- {
		g := q.globs[i]
		if g.re.MatchString(rel) {
			return !g.exclude
		}
		whitelist = whitelist || !g.exclude
	}
	return !whitelist
}

// isType reports whether a file called base is of the type t.
func isType(t, base string) bool {
	for _, g := range fileTypes[t] {
		if ok, _ := path.Match(g, base); ok {
			return true
		}
	}
	return false
}

// Filter is the files the query searches, of files.
func (q Query) Filter(files []string) []string {
	if len(q.Types) == 0 && len(q.NotTypes) == 0 && len(q.globs) == 0 {
		return files
	}
	return slices.DeleteFunc(slices.Clone(files), func(f string) bool { return !q.Keep(f) })
}

// Regexp is the query's pattern compiled, with its options. A pattern that is
// not a valid regexp - one half typed, or meant as text - is looked for as
// text, as though -F were given, rather than refused.
func (q Query) Regexp() *regexp.Regexp {
	build := func(p string) (*regexp.Regexp, error) {
		if q.Word {
			p = `\b(?:` + p + `)\b`
		}
		if q.ignoresCase() {
			p = "(?i)" + p
		}
		return regexp.Compile(p)
	}
	if !q.Fixed {
		if re, err := build(q.Pattern); err == nil {
			return re
		}
	}
	re, _ := build(regexp.QuoteMeta(q.Pattern))
	return re
}

// ignoresCase reports whether the query ignores case.
func (q Query) ignoresCase() bool {
	switch q.Case {
	case CaseIgnore:
		return true
	case CaseSensitive:
		return false
	}
	return !hasCapital(q.Pattern, q.Fixed)
}

// hasCapital reports whether pattern has a capital letter in what it looks
// for - not in \W or \S, which are not letters to match.
func hasCapital(pattern string, fixed bool) bool {
	if !fixed {
		if re, err := syntax.Parse(pattern, syntax.Perl); err == nil {
			return literalCapital(re)
		}
	}
	return strings.IndexFunc(pattern, unicode.IsUpper) >= 0
}

// literalCapital reports whether re looks for a capital letter by name.
func literalCapital(re *syntax.Regexp) bool {
	if re.Op == syntax.OpLiteral && slices.ContainsFunc(re.Rune, unicode.IsUpper) {
		return true
	}
	return slices.ContainsFunc(re.Sub, literalCapital)
}
