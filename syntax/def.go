package syntax

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	regsyntax "regexp/syntax"
	"sort"
	"strings"
	"unicode"
)

// A .syntax file describes one language, and every language nem colours is
// one: its own, embedded from languages/, and the user's, read from the
// config directory. The format is documented for users on the wiki's
// Languages page and its design in docs/design; this file reads it.
//
//	language ada
//	files *.adb *.ads
//	ignore-case
//	comment --
//	string " doubled
//	keywords begin end if then else
//	declares function function procedure
//
// Most of a language is words and delimiters, so most directives are lists of
// them, and a regexp is needed only for what those cannot say. Parsing checks
// everything a definition could get wrong - an unknown directive, a class that
// does not exist, a pattern that does not compile - and reports it with the
// file and line, since a mistake here otherwise looks like the language simply
// not being coloured.

// Def is a language as a .syntax file describes it, before it is compiled.
// It is kept, rather than only the compiled Language, because another
// definition may start from it with like.
type Def struct {
	Name string
	// Like names the language this one starts from, or "".
	Like string
	// Source is the file it was read from, for errors.
	Source string

	Files    []string // globs, lower-cased
	Shebangs []string
	Headers  []*regexp.Regexp

	IgnoreCase bool
	WordChars  string
	Words      []WordDef
	Declares   []WordDef
	Calls      string // "", "paren", "lisp" or "none"

	// Numbers is "" for the default, "none", or a pattern.
	Numbers     string
	numbersRe   *regexp.Regexp
	Operators   *string // nil for the default
	Punctuation *string

	Regions []*RegionDef
	Matches []*MatchDef
}

// WordDef is a word and the class it is given.
type WordDef struct {
	Word  string
	Class Class
}

// RegionDef is a comment, a string or a region: text from a start to an end.
type RegionDef struct {
	Class Class
	// Start is a literal delimiter, or StartRe a pattern.
	Start   string
	StartRe *regexp.Regexp
	// End is a literal delimiter, or EndRe a pattern, or EndTmpl a pattern
	// that refers to what StartRe captured and is compiled once it has.
	// ToEOL ends the region with the line: a line comment.
	End     string
	EndRe   *regexp.Regexp
	EndTmpl string
	ToEOL   bool

	// Escape is the rune that escapes the next one, or 0; EscapeRe a pattern
	// that escapes what it matches. Doubled means the end written twice is
	// the end escaped: "" inside "…".
	Escape   rune
	EscapeRe *regexp.Regexp
	Doubled  bool

	Nested    bool
	Multiline bool
}

// MatchDef is a pattern and what it colours: the whole match Class, or each
// of its named groups by its name.
type MatchDef struct {
	Class Class
	Re    *regexp.Regexp
	// Named reports whether the groups name the classes.
	Named bool
}

// classNames are the classes a definition may name. An unknown name is an
// error rather than plain text, since colouring `keyowrd` rules as nothing
// would look like the rule not working.
var classNames = map[string]Class{
	"plain":       Plain,
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

var languageName = regexp.MustCompile(`^[a-z0-9][a-z0-9+#._-]*$`)

// ParseDef reads one .syntax file. source names it in errors.
func ParseDef(source string, r io.Reader) (*Def, error) {
	p := defParser{def: &Def{Source: source}, source: source}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		p.lineNo++
		if err := p.line(strings.TrimRight(sc.Text(), " \t\r")); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", source, p.lineNo, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if err := p.closeRegion(); err != nil {
		return nil, fmt.Errorf("%s:%d: %w", source, p.regionLine, err)
	}
	if p.def.Name == "" {
		return nil, fmt.Errorf("%s: no language directive", source)
	}
	return p.def, nil
}

type defParser struct {
	def        *Def
	source     string
	lineNo     int
	directives int
	// region is the region block being read, and regionLine where it began.
	region     *RegionDef
	regionLine int
	regionTmpl string // its end, while the start's groups are not yet known
}

func (p *defParser) line(line string) error {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || trimmed[0] == '#' {
		return nil
	}
	if trimmed != line {
		// Indented: a field of the region above.
		if p.region == nil {
			return fmt.Errorf("an indented line belongs to a region, and there is none above it")
		}
		return p.regionField(trimmed)
	}
	if err := p.closeRegion(); err != nil {
		return err
	}
	name, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	p.directives++
	d := p.def
	if name != "language" && d.Name == "" {
		return fmt.Errorf("%s before language: a definition starts by naming its language", name)
	}
	switch name {
	case "language":
		if d.Name != "" {
			return fmt.Errorf("a second language directive: one language to a file")
		}
		if !languageName.MatchString(rest) {
			return fmt.Errorf("language name %q: lower-case letters, digits and + # . _ -", rest)
		}
		d.Name = rest
	case "like":
		if p.directives != 2 {
			return fmt.Errorf("like comes straight after language, since what follows adds to it")
		}
		if !languageName.MatchString(rest) {
			return fmt.Errorf("like %q: not a language name", rest)
		}
		d.Like = rest
	case "files":
		for _, g := range strings.Fields(rest) {
			if _, err := path.Match(g, ""); err != nil {
				return fmt.Errorf("files %q: %w", g, err)
			}
			d.Files = append(d.Files, strings.ToLower(g))
		}
	case "shebang":
		d.Shebangs = append(d.Shebangs, strings.Fields(rest)...)
	case "header":
		re, err := regexp.Compile(rest)
		if err != nil {
			return fmt.Errorf("header: %w", err)
		}
		d.Headers = append(d.Headers, re)
	case "ignore-case":
		if rest != "" {
			return fmt.Errorf("ignore-case takes nothing after it")
		}
		d.IgnoreCase = true
	case "word-chars":
		d.WordChars += strings.Join(strings.Fields(rest), "")
	case "keywords", "types", "constants", "functions":
		class := map[string]Class{"keywords": Keyword, "types": Type, "constants": Constant, "functions": Function}[name]
		for _, w := range strings.Fields(rest) {
			d.Words = append(d.Words, WordDef{Word: w, Class: class})
		}
	case "declares":
		cls, words, _ := strings.Cut(rest, " ")
		class, err := className(cls)
		if err != nil {
			return fmt.Errorf("declares: %w", err)
		}
		if strings.TrimSpace(words) == "" {
			return fmt.Errorf("declares %s: which words declare one?", cls)
		}
		for _, w := range strings.Fields(words) {
			d.Declares = append(d.Declares, WordDef{Word: w, Class: class})
		}
	case "calls":
		switch rest {
		case "paren", "lisp", "none":
			d.Calls = rest
		default:
			return fmt.Errorf("calls %q: paren, lisp or none", rest)
		}
	case "numbers":
		if rest == "" {
			return fmt.Errorf("numbers: a pattern, or none")
		}
		if rest != "none" {
			re, err := compileAnchored(rest)
			if err != nil {
				return fmt.Errorf("numbers: %w", err)
			}
			d.numbersRe = re
		}
		d.Numbers = rest
	case "operators", "punctuation":
		set := strings.Join(strings.Fields(rest), "")
		if set == "none" {
			set = ""
		}
		if name == "operators" {
			d.Operators = &set
		} else {
			d.Punctuation = &set
		}
	case "comment":
		return p.comment(rest)
	case "string":
		return p.string(rest)
	case "region":
		class, err := className(rest)
		if err != nil {
			return fmt.Errorf("region: %w", err)
		}
		p.region = &RegionDef{Class: class}
		p.regionLine = p.lineNo
	case "match":
		return p.match(rest)
	default:
		return fmt.Errorf("unknown directive %q", name)
	}
	return nil
}

// className reads a class name.
func className(s string) (Class, error) {
	c, ok := classNames[s]
	if !ok {
		return 0, fmt.Errorf("unknown class %q; the classes are %s", s, classNameList())
	}
	return c, nil
}

// comment reads comment START [END] [nested].
func (p *defParser) comment(rest string) error {
	f := strings.Fields(rest)
	nested := len(f) > 0 && f[len(f)-1] == "nested"
	if nested {
		f = f[:len(f)-1]
	}
	r := &RegionDef{Class: Comment, Escape: 0}
	switch len(f) {
	case 1:
		if nested {
			return fmt.Errorf("comment %s: only a comment with an end can nest", f[0])
		}
		r.Start, r.ToEOL = f[0], true
	case 2:
		r.Start, r.End, r.Nested, r.Multiline = f[0], f[1], nested, true
	default:
		return fmt.Errorf("comment: a start, and an end for a block comment, then nested if it nests")
	}
	p.def.Regions = append(p.def.Regions, r)
	return nil
}

// string reads string START [END] [raw|doubled|escape C] [multiline].
func (p *defParser) string(rest string) error {
	f := strings.Fields(rest)
	if len(f) == 0 {
		return fmt.Errorf("string: which delimiter?")
	}
	r := &RegionDef{Class: String, Start: f[0], End: f[0], Escape: '\\'}
	f = f[1:]
	if len(f) > 0 && !isStringOption(f[0]) {
		r.End, f = f[0], f[1:]
	}
	for len(f) > 0 {
		switch f[0] {
		case "raw":
			r.Escape = 0
		case "doubled":
			r.Escape, r.Doubled = 0, true
		case "escape":
			if len(f) < 2 || len([]rune(f[1])) != 1 {
				return fmt.Errorf("string %s: escape takes the one rune that escapes", r.Start)
			}
			r.Escape = []rune(f[1])[0]
			f = f[1:]
		case "multiline":
			r.Multiline = true
		default:
			return fmt.Errorf("string %s: %q is not raw, doubled, escape or multiline", r.Start, f[0])
		}
		f = f[1:]
	}
	p.def.Regions = append(p.def.Regions, r)
	return nil
}

func isStringOption(s string) bool {
	switch s {
	case "raw", "doubled", "escape", "multiline":
		return true
	}
	return false
}

// regionField reads one indented line of a region block.
func (p *defParser) regionField(line string) error {
	name, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	r := p.region
	switch name {
	case "start":
		re, err := compileMid(rest)
		if err != nil {
			return fmt.Errorf("start: %w", err)
		}
		r.StartRe = re
	case "end":
		if rest == "" {
			return fmt.Errorf("end: a pattern")
		}
		p.regionTmpl = rest
	case "escape":
		re, err := compileMid(rest)
		if err != nil {
			return fmt.Errorf("escape: %w", err)
		}
		r.EscapeRe = re
	case "nested":
		r.Nested = true
	case "multiline":
		r.Multiline = true
	default:
		return fmt.Errorf("unknown region field %q: start, end, escape, nested or multiline", name)
	}
	return nil
}

var captureRef = regexp.MustCompile(`\\[1-9]`)

// closeRegion finishes the region block being read, if there is one.
func (p *defParser) closeRegion() error {
	r := p.region
	if r == nil {
		return nil
	}
	p.region = nil
	if r.StartRe == nil || p.regionTmpl == "" {
		return fmt.Errorf("region: a start and an end, each on an indented line below it")
	}
	tmpl := p.regionTmpl
	p.regionTmpl = ""
	if refs := captureRef.FindAllString(tmpl, -1); refs != nil {
		groups := r.StartRe.NumSubexp()
		for _, ref := range refs {
			if n := int(ref[1] - '0'); n > groups {
				return fmt.Errorf("region: end refers to %s, but start has %d groups", ref, groups)
			}
		}
		// Checked now with the groups filled in by something, so a bad
		// pattern is found here rather than on the first line that opens it.
		if _, err := compileMid(fillCaptures(tmpl, make([]string, 9))); err != nil {
			return fmt.Errorf("region end: %w", err)
		}
		r.EndTmpl = tmpl
	} else {
		re, err := compileMid(tmpl)
		if err != nil {
			return fmt.Errorf("region end: %w", err)
		}
		r.EndRe = re
	}
	p.def.Regions = append(p.def.Regions, r)
	return nil
}

// fillCaptures writes into an end pattern what the start captured, quoted.
func fillCaptures(tmpl string, caps []string) string {
	return captureRef.ReplaceAllStringFunc(tmpl, func(ref string) string {
		return regexp.QuoteMeta(caps[ref[1]-'1'])
	})
}

// match reads match CLASS REGEX, or match REGEX with named groups.
func (p *defParser) match(rest string) error {
	m := &MatchDef{}
	pattern := rest
	if cls, pat, ok := strings.Cut(rest, " "); ok {
		if c, known := classNames[cls]; known {
			m.Class, pattern = c, strings.TrimSpace(pat)
		}
	}
	if pattern == "" {
		return fmt.Errorf("match: a pattern")
	}
	re, err := compileMid(pattern)
	if err != nil {
		return fmt.Errorf("match: %w", err)
	}
	m.Re = re
	named := false
	for _, n := range re.SubexpNames() {
		if n == "" {
			continue
		}
		if _, ok := classNames[n]; !ok {
			return fmt.Errorf("match: group %q is not a class; the classes are %s", n, classNameList())
		}
		named = true
	}
	if named && pattern != rest {
		return fmt.Errorf("match: a class, or groups named by class, not both")
	}
	if !named && pattern == rest {
		return fmt.Errorf("match %q: which class? match CLASS PATTERN, or name the groups by class", rest)
	}
	m.Named = named
	p.def.Matches = append(p.def.Matches, m)
	return nil
}

// compileMid compiles a pattern to be matched from any place in a line.
// It is checked here; the compiled forms a Language uses are made when it
// is compiled.
func compileMid(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

// compileAnchored compiles a pattern to be matched only where it starts.
func compileAnchored(pattern string) (*regexp.Regexp, error) {
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, err
	}
	return regexp.Compile(`\A(?:` + pattern + `)`)
}

// midLine returns re as it must be matched from anywhere but the start of a
// line: with every ^ and \A made to fail. A pattern is matched on what is
// left of the line, where ^ would otherwise match wherever that begins.
func midLine(re *regexp.Regexp) *regexp.Regexp {
	tree, err := regsyntax.Parse(re.String(), regsyntax.Perl)
	if err != nil {
		return re
	}
	if !neverAtStart(tree) {
		return re
	}
	out, err := regexp.Compile(tree.String())
	if err != nil {
		return re
	}
	return out
}

// neverAtStart replaces tree's line and text starts with a node that never
// matches, reporting whether it had any.
func neverAtStart(tree *regsyntax.Regexp) bool {
	changed := false
	for _, sub := range tree.Sub {
		if sub.Op == regsyntax.OpBeginLine || sub.Op == regsyntax.OpBeginText {
			*sub = regsyntax.Regexp{Op: regsyntax.OpNoMatch}
			changed = true
			continue
		}
		if neverAtStart(sub) {
			changed = true
		}
	}
	if tree.Op == regsyntax.OpBeginLine || tree.Op == regsyntax.OpBeginText {
		*tree = regsyntax.Regexp{Op: regsyntax.OpNoMatch}
		changed = true
	}
	return changed
}

// merge returns base with own on top: what like does. Lists add up, own's
// words and rules first so they win; a setting own makes replaces base's.
// base's files, shebangs and headers are kept only when own is base itself,
// changed - a new language that starts from go must not claim *.go.
func merge(base, own *Def) *Def {
	d := *own
	d.Like = ""
	if base.Name == own.Name {
		d.Files = append(append([]string(nil), base.Files...), own.Files...)
		d.Shebangs = append(append([]string(nil), base.Shebangs...), own.Shebangs...)
		d.Headers = append(append([]*regexp.Regexp(nil), base.Headers...), own.Headers...)
	}
	d.IgnoreCase = base.IgnoreCase || own.IgnoreCase
	d.WordChars = base.WordChars + own.WordChars
	d.Words = append(append([]WordDef(nil), base.Words...), own.Words...)
	d.Declares = append(append([]WordDef(nil), base.Declares...), own.Declares...)
	if d.Calls == "" {
		d.Calls = base.Calls
	}
	if d.Numbers == "" {
		d.Numbers, d.numbersRe = base.Numbers, base.numbersRe
	}
	if d.Operators == nil {
		d.Operators = base.Operators
	}
	if d.Punctuation == nil {
		d.Punctuation = base.Punctuation
	}
	d.Regions = append(append([]*RegionDef(nil), own.Regions...), base.Regions...)
	d.Matches = append(append([]*MatchDef(nil), own.Matches...), base.Matches...)
	return &d
}

// isWordRune reports whether r is a letter, digit, mark or underscore: what
// every language's names are made of.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
