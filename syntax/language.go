package syntax

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Language is one compiled definition, and is itself a Lexer.
//
// A line is read left to right, one token at a time, and at each place the
// first of these that applies wins: a match rule, in the order written; the
// start of a comment, string or region, the longest if several start there;
// a number; a name, classified; an operator or punctuation rune. Reading
// tokens, rather than letting later rules paint over earlier ones as nano's
// files do, is what makes a // inside a string a string and a quote inside a
// comment a comment, with no ordering of rules to get right.
type Language struct {
	name string
	def  *Def

	files    []string
	shebangs map[string]bool
	headers  []*regexp.Regexp

	ignoreCase bool
	extra      runeSet // runes a name may hold besides letters, digits and _
	// words gives each listed word its class, and what it declares the
	// name after it to be: one lookup a name.
	words     map[string]wordInfo
	calls     string
	noNumbers bool
	numbers   *regexp.Regexp // nil: the default scanner
	ops       runeSet
	puncts    runeSet
	regions   []*region
	matches   []*matcher
	// starts is what any match rule or region can begin with, anywhere but
	// at the start of a line, where startsAt0 is: a rune outside both is
	// never asked about any of them.
	starts, startsAt0 first
	// matchers counts the matchers a line may cache the next match of.
	matchers int

	// What a region's start captured, interned: a line that opens [==[
	// leaves the same state wherever it is, so the highlight cache's
	// convergence check still means what it did.
	mu      sync.Mutex
	capIDs  map[string]uint16
	capEnds []*matcher // by id-1: each region end with its captures filled in
}

// wordInfo is what a language knows of a word.
type wordInfo struct {
	class    Class // unclaimed when the word is not in a list
	declares Class // unclaimed when it declares nothing
}

// region is a compiled RegionDef.
type region struct {
	class     Class
	start     []rune // a literal start, or
	startRe   *matcher
	end       []rune // a literal end, or
	endRe     *matcher
	endTmpl   string // an end filled in with what startRe captured
	toEOL     bool
	escape    rune
	escapeRe  *matcher
	doubled   bool
	nested    bool
	multiline bool
}

// matcher is a pattern compiled twice: as written, for the start of a line,
// and with ^ made to fail, for anywhere else - it is matched on what is left
// of the line, where ^ would otherwise match wherever that happens to begin.
type matcher struct {
	atStart, mid *regexp.Regexp
	// here and hereMid are the same, anchored where they are tried: a
	// pattern whose first runes are few is asked only where one of them is,
	// and there failing takes a rune or two, where a search ran on through
	// the rest of the line.
	here, hereMid *regexp.Regexp
	// firstAt and firstMid are what each can begin with.
	firstAt, firstMid first
	// id numbers the matcher within its language, for the line's cache.
	id    int
	class Class
	// groups gives the class of each capture group, or unclaimed, when the
	// groups name the classes.
	groups []Class
}

func newMatcher(re *regexp.Regexp) *matcher {
	m := &matcher{atStart: re, mid: midLine(re)}
	m.firstAt, m.firstMid = firstOf(m.atStart), firstOf(m.mid)
	m.here = regexp.MustCompile(`\A(?:` + m.atStart.String() + `)`)
	m.hereMid = regexp.MustCompile(`\A(?:` + m.mid.String() + `)`)
	return m
}

// unclaimed marks a group or rune that no class claims.
const unclaimed = Class(0xFF)

// runeSet is a set of runes, quick for ASCII.
type runeSet struct {
	ascii [utf8.RuneSelf]bool
	other map[rune]bool
}

func makeRuneSet(s string) runeSet {
	var set runeSet
	for _, r := range s {
		if r < utf8.RuneSelf {
			set.ascii[r] = true
			continue
		}
		if set.other == nil {
			set.other = map[rune]bool{}
		}
		set.other[r] = true
	}
	return set
}

func (s *runeSet) has(r rune) bool {
	if r < utf8.RuneSelf {
		return r >= 0 && s.ascii[r]
	}
	return s.other[r]
}

const (
	defaultOperators   = "+-*/%&|^<>=!~?:"
	defaultPunctuation = "()[]{},;."
)

// maxRegions is how many regions a language may have: State keeps the open
// one in eight bits, and 0 means none.
const maxRegions = 254

// Compile makes a Language of a definition, one already merged with what it
// is like.
func Compile(d *Def) (*Language, error) {
	if d.Like != "" {
		return nil, fmt.Errorf("%s: like %s is not resolved", d.Source, d.Like)
	}
	if len(d.Regions) > maxRegions {
		return nil, fmt.Errorf("%s: %d regions; at most %d", d.Source, len(d.Regions), maxRegions)
	}
	l := &Language{
		name:       d.Name,
		def:        d,
		files:      d.Files,
		headers:    d.Headers,
		ignoreCase: d.IgnoreCase,
		extra:      makeRuneSet(d.WordChars),
		words:      map[string]wordInfo{},
		calls:      d.Calls,
		capIDs:     map[string]uint16{},
	}
	if l.calls == "" {
		l.calls = "paren"
	}
	for _, s := range d.Shebangs {
		if l.shebangs == nil {
			l.shebangs = map[string]bool{}
		}
		l.shebangs[s] = true
	}
	info := func(w string) wordInfo {
		if wi, ok := l.words[l.key(w)]; ok {
			return wi
		}
		return wordInfo{class: unclaimed, declares: unclaimed}
	}
	for _, w := range d.Words {
		wi := info(w.Word)
		wi.class = w.Class
		l.words[l.key(w.Word)] = wi
	}
	for _, w := range d.Declares {
		wi := info(w.Word)
		wi.declares = w.Class
		l.words[l.key(w.Word)] = wi
	}
	switch d.Numbers {
	case "":
	case "none":
		l.noNumbers = true
	default:
		l.numbers = d.numbersRe
	}
	ops, puncts := defaultOperators, defaultPunctuation
	if d.Operators != nil {
		ops = *d.Operators
	}
	if d.Punctuation != nil {
		puncts = *d.Punctuation
	}
	l.ops, l.puncts = makeRuneSet(ops), makeRuneSet(puncts)
	for _, rd := range d.Regions {
		r := &region{
			class: rd.Class, start: []rune(rd.Start), end: []rune(rd.End), endTmpl: rd.EndTmpl,
			toEOL: rd.ToEOL, escape: rd.Escape, doubled: rd.Doubled, nested: rd.Nested, multiline: rd.Multiline,
		}
		if rd.StartRe != nil {
			r.startRe = newMatcher(rd.StartRe)
		}
		if rd.EndRe != nil {
			r.endRe = newMatcher(rd.EndRe)
		}
		if rd.EscapeRe != nil {
			r.escapeRe = newMatcher(rd.EscapeRe)
		}
		l.regions = append(l.regions, r)
	}
	for _, md := range d.Matches {
		m := newMatcher(md.Re)
		m.class = md.Class
		if md.Named {
			m.groups = make([]Class, md.Re.NumSubexp()+1)
			for i, n := range md.Re.SubexpNames() {
				m.groups[i] = unclaimed
				if c, ok := classNames[n]; ok && n != "" {
					m.groups[i] = c
				}
			}
		}
		l.matches = append(l.matches, m)
	}
	l.index()
	return l, nil
}

// index numbers the matchers and gathers what any of them can begin with.
func (l *Language) index() {
	add := func(m *matcher) {
		if m == nil {
			return
		}
		m.id = l.matchers
		l.matchers++
	}
	for _, m := range l.matches {
		add(m)
		l.starts.add(m.firstMid)
		l.startsAt0.add(m.firstAt)
	}
	for _, r := range l.regions {
		add(r.startRe)
		add(r.endRe)
		add(r.escapeRe)
		if r.startRe != nil {
			l.starts.add(r.startRe.firstMid)
			l.startsAt0.add(r.startRe.firstAt)
			continue
		}
		var f first
		f.addRune(r.start[0], l.ignoreCase)
		l.starts.add(f)
		l.startsAt0.add(f)
	}
}

// Name reports the language.
func (l *Language) Name() string { return l.name }

// Def returns the definition the language was compiled from.
func (l *Language) Def() *Def { return l.def }

// Comment reports how the language writes a comment, for M-;: the first
// comment its definition lists, and that comment's end if it is a block
// comment. A language lists the one it writes first - Lisp's ;; before its
// ; - so a definition says which, with nothing else to learn. ok is false
// for a language with no comments at all.
func (l *Language) Comment() (start, end string, ok bool) {
	for _, r := range l.def.Regions {
		if r.Class == Comment && r.StartRe == nil && (r.ToEOL || r.End != "") {
			return r.Start, r.End, true
		}
	}
	return "", "", false
}

// key is a word as the word lists hold it.
func (l *Language) key(w string) string {
	if l.ignoreCase {
		return strings.ToLower(w)
	}
	return w
}

func (l *Language) isWordStart(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiWordStart[r] || l.extra.ascii[r]
	}
	return unicode.IsLetter(r) || l.extra.has(r)
}

func (l *Language) isWordPart(r rune) bool {
	if r < utf8.RuneSelf {
		return asciiWordPart[r] || l.extra.ascii[r]
	}
	return isWordRune(r) || l.extra.has(r)
}

// The ASCII names are made of, looked up rather than asked of unicode: a
// name's every rune is.
var asciiWordStart, asciiWordPart = func() (start, part [utf8.RuneSelf]bool) {
	for r := rune(0); r < utf8.RuneSelf; r++ {
		start[r] = r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
		part[r] = start[r] || isDigit(r)
	}
	return
}()

// State, for a Language:
//
//	bits 0..7    the open region's index + 1; 0 when none is open
//	bits 8..15   how deep a nested region is
//	bits 16..31  the id of what the region's start captured, or 0
func regionState(k, depth int, capID uint16) State {
	return State(uint32(k+1) | uint32(min(depth, 255))<<8 | uint32(capID)<<16)
}

func openRegion(s State) (k, depth int, capID uint16, open bool) {
	if s&0xFF == 0 {
		return 0, 0, 0, false
	}
	return int(s&0xFF) - 1, int(s>>8) & 0xFF, uint16(s >> 16), true
}

// capture interns what region k's start captured, returning its id and the
// region's end with the captures filled in.
func (l *Language) capture(k int, caps []string) (uint16, *matcher) {
	key := fmt.Sprint(k) + "\x00" + strings.Join(caps, "\x00")
	l.mu.Lock()
	defer l.mu.Unlock()
	if id, ok := l.capIDs[key]; ok {
		return id, l.capEnds[id-1]
	}
	re, err := regexp.Compile(fillCaptures(l.regions[k].endTmpl, caps))
	if err != nil {
		// Checked when the definition was read, with captures of every
		// kind; a capture cannot make a valid pattern invalid, since it is
		// quoted. Kept safe all the same: a region that cannot end.
		re = regexp.MustCompile(`[^\x00-\x{10FFFF}]`)
	}
	m := newMatcher(re)
	m.id = l.matchers + len(l.capEnds)
	if len(l.capEnds) >= 0xFFFF {
		// Out of ids, after 65,535 different delimiters in one session: the
		// region still ends where it should on this line, and a line after
		// it that the state cannot describe loses the capture.
		return 0, m
	}
	l.capEnds = append(l.capEnds, m)
	id := uint16(len(l.capEnds))
	l.capIDs[key] = id
	return id, m
}

func (l *Language) capEnd(id uint16) *matcher {
	l.mu.Lock()
	defer l.mu.Unlock()
	if id == 0 || int(id) > len(l.capEnds) {
		return nil
	}
	return l.capEnds[id-1]
}

// Lex classifies one line.
func (l *Language) Lex(line []rune, in State) ([]Span, State) {
	x := lineLexer{l: l, line: line, n: len(line)}
	i := 0
	if k, depth, capID, open := openRegion(in); open && k < len(l.regions) {
		r := l.regions[k]
		end := r.endRe
		if r.endTmpl != "" {
			end = l.capEnd(capID)
		}
		to, closed, depth := x.inRegion(r, end, 0, depth)
		x.b.add(0, to, r.class)
		if !closed {
			return x.b.out, regionState(k, depth, capID)
		}
		i = to
	}
	return x.lex(i)
}

// lineLexer is the reading of one line.
type lineLexer struct {
	l    *Language
	line []rune
	n    int
	b    builder

	// The line as a string, with the byte offset of each rune when they
	// differ, made only once a pattern is matched: most lines of most
	// languages need none.
	src     string
	haveSrc bool
	byteOf  []int // rune index -> byte offset; nil when the line is ASCII
	runeOf  []int // byte offset -> rune index; nil when the line is ASCII

	// found caches each matcher's next match, by its id.
	found []nextMatch
	// loc is where at reports a match, in runes.
	loc []int

	pending Class // what a declares word made of the next name, or unclaimed
}

// nextMatch is where a pattern next matches at or after from, in bytes.
type nextMatch struct {
	set  bool
	from int
	loc  []int // nil: no match at or after from
}

func (x *lineLexer) ensureSrc() {
	if x.haveSrc {
		return
	}
	x.haveSrc = true
	x.src = string(x.line)
	if len(x.src) == x.n {
		return
	}
	x.byteOf = make([]int, x.n+1)
	x.runeOf = make([]int, len(x.src)+1)
	ri := 0
	for bi := range x.src {
		x.byteOf[ri] = bi
		x.runeOf[bi] = ri
		ri++
	}
	x.byteOf[x.n] = len(x.src)
	x.runeOf[len(x.src)] = x.n
}

func (x *lineLexer) byteAt(i int) int {
	if x.byteOf == nil {
		return i
	}
	return x.byteOf[i]
}

func (x *lineLexer) runeAt(b int) int {
	if x.runeOf == nil {
		return b
	}
	return x.runeOf[b]
}

// at reports whether m matches starting at rune i, and its submatches as rune
// indices when it does. The next match is kept, so a pattern is searched once
// for each place it matches rather than once for each place it is asked
// about.
func (x *lineLexer) at(m *matcher, i int) ([]int, bool) {
	if i < x.n {
		f := &m.firstMid
		if i == 0 {
			f = &m.firstAt
		}
		if !f.has(x.line[i]) {
			return nil, false
		}
	}
	x.ensureSrc()
	b := x.byteAt(i)
	f := &m.firstMid
	if i == 0 {
		f = &m.firstAt
	}
	if !f.any {
		re := m.hereMid
		if b == 0 {
			re = m.here
		}
		loc := re.FindStringSubmatchIndex(x.src[b:])
		if loc == nil {
			return nil, false
		}
		x.loc = x.loc[:0]
		for _, v := range loc {
			if v >= 0 {
				v = x.runeAt(b + v)
			}
			x.loc = append(x.loc, v)
		}
		return x.loc, true
	}
	if m.id >= len(x.found) {
		x.found = append(x.found, make([]nextMatch, m.id+1-len(x.found))...)
	}
	nm := &x.found[m.id]
	if !nm.set || b < nm.from || (nm.loc != nil && nm.loc[0] < b) {
		*nm = nextMatch{set: true, from: b}
		re := m.mid
		if b == 0 {
			re = m.atStart
		}
		if loc := re.FindStringSubmatchIndex(x.src[b:]); loc != nil {
			for k := range loc {
				if loc[k] >= 0 {
					loc[k] += b
				}
			}
			nm.loc = loc
		}
	}
	if nm.loc == nil || nm.loc[0] != b {
		return nil, false
	}
	x.loc = x.loc[:0]
	for _, v := range nm.loc {
		if v >= 0 {
			v = x.runeAt(v)
		}
		x.loc = append(x.loc, v)
	}
	return x.loc, true
}

// hasAt reports whether s is at rune i: in any case, in a language that
// ignores case, so its REM is its rem.
func (x *lineLexer) hasAt(i int, s []rune) bool {
	if len(s) == 0 || i+len(s) > x.n {
		return false
	}
	for k, r := range s {
		c := x.line[i+k]
		if c != r && (!x.l.ignoreCase || unicode.ToLower(c) != unicode.ToLower(r)) {
			return false
		}
	}
	return true
}

func (x *lineLexer) lex(i int) ([]Span, State) {
	l := x.l
	x.pending = unclaimed
	for i < x.n {
		c := x.line[i]
		starts := &l.starts
		if i == 0 {
			starts = &l.startsAt0
		}
		if !starts.has(c) {
			goto token
		}
		if to, ok := x.match(i); ok {
			x.pending = unclaimed
			i = to
			continue
		}
		if k, to, caps, ok := x.regionStart(i); ok {
			x.pending = unclaimed
			r := l.regions[k]
			var capID uint16
			end := r.endRe
			if r.endTmpl != "" {
				capID, end = l.capture(k, caps)
			}
			depth := 0
			if r.nested {
				depth = 1
			}
			stop, closed, depth := x.inRegion(r, end, to, depth)
			x.b.add(i, stop, r.class)
			if !closed {
				if r.multiline {
					return x.b.out, regionState(k, depth, capID)
				}
				return x.b.out, 0
			}
			i = stop
			continue
		}
	token:
		if to, ok := x.number(i); ok {
			x.pending = unclaimed
			x.b.add(i, to, Number)
			i = to
			continue
		}
		if l.isWordStart(c) || isDigit(c) {
			i = x.word(i)
			continue
		}
		switch {
		case c == ' ' || c == '\t':
		case l.ops.has(c):
			x.pending = unclaimed
			x.b.add(i, i+1, Operator)
		case l.puncts.has(c):
			x.pending = unclaimed
			x.b.add(i, i+1, Punctuation)
		default:
			x.pending = unclaimed
		}
		i++
	}
	return x.b.out, 0
}

// match tries the match rules at i, in order, returning where the one that
// matched leaves off.
func (x *lineLexer) match(i int) (int, bool) {
	for _, m := range x.l.matches {
		loc, ok := x.at(m, i)
		if !ok || loc[1] <= i {
			continue
		}
		if m.groups == nil {
			x.b.add(i, loc[1], m.class)
			return loc[1], true
		}
		// Groups named by class: each coloured, and what follows the last
		// of them left to be read again.
		last := -1
		for g := 1; g < len(m.groups); g++ {
			if m.groups[g] == unclaimed || loc[2*g] < 0 {
				continue
			}
			last = max(last, loc[2*g+1])
		}
		if last <= i {
			continue
		}
		pos := i
		for g := 1; g < len(m.groups); g++ {
			s, e := loc[2*g], loc[2*g+1]
			if m.groups[g] == unclaimed || s < 0 || s < pos {
				continue
			}
			x.b.add(s, e, m.groups[g])
			pos = e
		}
		return last, true
	}
	return 0, false
}

// regionStart finds the region that starts at i: the longest start, the
// first written of those as long. It returns the region, where its start
// ends, and what the start captured.
func (x *lineLexer) regionStart(i int) (k, to int, caps []string, ok bool) {
	best, bestTo := -1, i
	var bestLoc []int
	for j, r := range x.l.regions {
		if r.startRe == nil {
			if x.hasAt(i, r.start) && i+len(r.start) > bestTo && x.wordEnds(i+len(r.start), r.start) {
				best, bestTo, bestLoc = j, i+len(r.start), nil
			}
			continue
		}
		if loc, ok := x.at(r.startRe, i); ok && loc[1] > bestTo {
			best, bestTo, bestLoc = j, loc[1], loc
		}
	}
	if best < 0 {
		return 0, 0, nil, false
	}
	if x.l.regions[best].endTmpl != "" {
		caps = make([]string, 9)
		for g := 1; g < len(bestLoc)/2 && g <= 9; g++ {
			if bestLoc[2*g] >= 0 {
				caps[g-1] = string(x.line[bestLoc[2*g]:bestLoc[2*g+1]])
			}
		}
	}
	return best, bestTo, caps, true
}

// inRegion reads the inside of region r from j, to its end, returning where
// the region stops, whether it closed on this line, and how deep it is if it
// did not. end is the region's end pattern, if it has one.
func (x *lineLexer) inRegion(r *region, end *matcher, j, depth int) (int, bool, int) {
	if r.toEOL {
		return x.n, true, 0
	}
	for j <= x.n {
		if j < x.n {
			if r.escape != 0 && x.line[j] == r.escape {
				j = min(j+2, x.n)
				continue
			}
			if r.escapeRe != nil {
				if loc, ok := x.at(r.escapeRe, j); ok && loc[1] > j {
					j = loc[1]
					continue
				}
			}
			if r.doubled && x.hasAt(j, r.end) && x.hasAt(j+len(r.end), r.end) {
				j += 2 * len(r.end)
				continue
			}
			if r.nested {
				if to, ok := x.startsAt(r, j); ok {
					depth++
					j = to
					continue
				}
			}
		}
		if to, ok := x.endsAt(r, end, j); ok {
			if r.nested && depth > 1 {
				depth--
				j = max(to, j+1)
				continue
			}
			return to, true, 0
		}
		j++
	}
	return x.n, false, depth
}

// wordEnds reports whether a delimiter ending at to stands alone there: one
// that ends in a letter, as REM, dnl and @c do, must not run on into a longer
// word, or @code would open a texinfo comment.
func (x *lineLexer) wordEnds(to int, delim []rune) bool {
	if len(delim) == 0 || !isWordRune(delim[len(delim)-1]) || to >= x.n {
		return true
	}
	return !x.l.isWordPart(x.line[to])
}

// startsAt reports whether r starts at j, and where its start ends.
func (x *lineLexer) startsAt(r *region, j int) (int, bool) {
	if r.startRe == nil {
		return j + len(r.start), x.hasAt(j, r.start)
	}
	loc, ok := x.at(r.startRe, j)
	if !ok || loc[1] <= j {
		return 0, false
	}
	return loc[1], true
}

// endsAt reports whether r ends at j, and where its end ends.
func (x *lineLexer) endsAt(r *region, end *matcher, j int) (int, bool) {
	if end == nil {
		return j + len(r.end), x.hasAt(j, r.end)
	}
	loc, ok := x.at(end, j)
	if !ok {
		return 0, false
	}
	return loc[1], true
}

// number reads a number at i, if one starts there and is not the start of a
// name: Lisp's 1+ is a name.
func (x *lineLexer) number(i int) (int, bool) {
	l := x.l
	if l.noNumbers {
		return 0, false
	}
	c := x.line[i]
	if !isDigit(c) {
		// A sign or a dot before a digit may start one, as .5 and -1 do,
		// but not a dot after a dot: 0..10 is a range.
		if i+1 >= x.n || !isDigit(x.line[i+1]) || unicode.IsLetter(c) || c == '_' {
			return 0, false
		}
		if c == '.' && i > 0 && x.line[i-1] == '.' {
			return 0, false
		}
	}
	if i > 0 && l.isWordPart(x.line[i-1]) {
		return 0, false // inside a name
	}
	var to int
	if l.numbers == nil {
		to = scanNumber(x.line, i)
	} else {
		x.ensureSrc()
		b := x.byteAt(i)
		loc := l.numbers.FindStringIndex(x.src[b:])
		if loc == nil {
			return 0, false
		}
		to = x.runeAt(b + loc[1])
	}
	if to <= i || (to < x.n && l.isWordPart(x.line[to])) {
		return 0, false
	}
	return to, true
}

// scanNumber is the default number: 0x, 0b and 0o forms, digits with _
// between them, a fraction, an exponent - p for a hex one - and any suffix
// of letters and digits, which covers 1.5i, 10UL, 1_000i64 and 1.0f.
func scanNumber(line []rune, i int) int {
	n := len(line)
	run := func(j int, ok func(rune) bool) int {
		for j < n && (ok(line[j]) || line[j] == '_') {
			j++
		}
		return j
	}
	suffix := func(j int) int {
		for j < n && (line[j] == '_' || line[j] < utf8.RuneSelf && (isDigit(line[j]) || unicode.IsLetter(line[j]))) {
			j++
		}
		return j
	}
	j := i
	if line[j] == '0' && j+1 < n {
		switch line[j+1] {
		case 'x', 'X':
			k := run(j+2, isHexDigit)
			if k < n && line[k] == '.' {
				k = run(k+1, isHexDigit)
			}
			return suffix(scanExponent(line, k, 'p', 'P'))
		case 'b', 'B':
			return suffix(run(j+2, func(r rune) bool { return r == '0' || r == '1' }))
		case 'o', 'O':
			return suffix(run(j+2, func(r rune) bool { return r >= '0' && r <= '7' }))
		}
	}
	switch {
	case line[j] == '.':
		if j+1 >= n || !isDigit(line[j+1]) {
			return i
		}
		j = run(j+1, isDigit)
	case isDigit(line[j]):
		j = run(j, isDigit)
		if j+1 < n && line[j] == '.' && isDigit(line[j+1]) {
			j = run(j+1, isDigit)
		}
	default:
		return i
	}
	return suffix(scanExponent(line, j, 'e', 'E'))
}

// word reads a name at i and classifies it: a keyword, type, constant or
// function by the lists, then whatever a declares word before it made it,
// then a call. It returns where the name ends.
func (x *lineLexer) word(i int) int {
	l := x.l
	j := i + 1
	for j < x.n && l.isWordPart(x.line[j]) {
		j++
	}
	var kb [64]byte
	key := kb[:0]
	for _, r := range x.line[i:j] {
		switch {
		case r >= utf8.RuneSelf:
			if l.ignoreCase {
				r = unicode.ToLower(r)
			}
			key = utf8.AppendRune(key, r)
		case l.ignoreCase && 'A' <= r && r <= 'Z':
			key = append(key, byte(r+'a'-'A'))
		default:
			key = append(key, byte(r))
		}
	}
	wi, ok := l.words[string(key)]
	if !ok {
		wi = wordInfo{class: unclaimed, declares: unclaimed}
	}
	class, known := wi.class, wi.class != unclaimed
	declares, isDecl := wi.declares, wi.declares != unclaimed
	switch {
	case known:
		// A keyword between a declaring word and the name, as body is in
		// Ada's package body Name, keeps what the name will be.
	case x.pending != unclaimed:
		class = x.pending
		x.pending = unclaimed
	case l.calls == "paren" && x.nextIs(j, '('):
		class = Function
	case l.calls == "lisp" && x.prevIs(i, '('):
		class = Function
	default:
		class = Plain
	}
	if isDecl {
		x.pending = declares
	}
	if class != Plain {
		x.b.add(i, j, class)
	}
	return j
}

// nextIs reports whether the first rune at or after j that is not a blank is r.
func (x *lineLexer) nextIs(j int, r rune) bool {
	for j < x.n && (x.line[j] == ' ' || x.line[j] == '\t') {
		j++
	}
	return j < x.n && x.line[j] == r
}

// prevIs reports whether the last rune before i that is not a blank is r.
func (x *lineLexer) prevIs(i int, r rune) bool {
	for i > 0 && (x.line[i-1] == ' ' || x.line[i-1] == '\t') {
		i--
	}
	return i > 0 && x.line[i-1] == r
}

// Words lists a language's words of one class, sorted, for tests and for
// describing a language.
func (l *Language) Words(c Class) []string {
	var out []string
	for w, wi := range l.words {
		if wi.class == c {
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}
