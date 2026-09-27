package project

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
	"unsafe"
)

// MaxMatches is where a search stops: past it the list is too long to read,
// and the search should be narrowed instead.
const MaxMatches = 10_000

// maxFileSize is the largest file searched. Anything bigger is data - a log,
// a dump, a bundle - rather than something a person wrote.
const maxFileSize = 16 << 20

// shownWidth is how much of a long matching line a result shows: minified
// code puts a whole file on one line, and a result that long is unreadable.
const shownWidth = 240

// Match is a line that matched a search.
type Match struct {
	// File is the file, relative to the project root, as Files lists it.
	File string
	// Line is the line's index, from 0, and Col the rune column where the
	// first match on it starts.
	Line, Col int
	// Text is the line as a result shows it: all of it, or for a long line
	// the part around the first match, with an ellipsis for what is left out.
	Text string
	// Spans are the rune ranges of Text that matched, start and end.
	Spans [][2]int
}

// Lines supplies a file's text for searching, as lines, when the caller has it
// already - an open buffer, whose unsaved edits are what should be searched,
// and whose line numbers are the ones the user will be taken to. False reads
// the file from disk.
type Lines func(rel string) ([]string, bool)

// Options bounds a search.
type Options struct {
	// Max is how many matches the search stops at: MaxMatches when 0.
	Max int
	// Stop abandons the search when it is set: one run as the pattern is
	// typed has been overtaken by the next keystroke.
	Stop *atomic.Bool
}

// Search finds the lines of files, relative to root, that re matches, in the
// order of files and then of lines. More reports that it stopped at
// MaxMatches.
//
// Files are searched in parallel, and a file that is not text - one with a
// NUL byte near its start - or is larger than any source file is passed over.
func Search(root string, files []string, re *regexp.Regexp, open Lines) (matches []Match, more bool) {
	return SearchWith(root, files, re, open, Options{})
}

// SearchWith is Search within opt's bounds. A search stopped by opt.Stop
// returns what it had found.
func SearchWith(root string, files []string, re *regexp.Regexp, open Lines, opt Options) (matches []Match, more bool) {
	limit := opt.Max
	if limit <= 0 {
		limit = MaxMatches
	}
	stopped := func() bool { return opt.Stop != nil && opt.Stop.Load() }
	s := newSearcher(re)
	found := make([][]Match, len(files))
	var next, total atomic.Int64
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			// Each worker reads every file into the one buffer, and lowers
			// it, when the search ignores case, into the other.
			var buf, low []byte
			for {
				i := int(next.Add(1) - 1)
				if i >= len(files) || total.Load() >= int64(limit) || stopped() {
					return
				}
				found[i] = s.file(root, files[i], open, &buf, &low)
				total.Add(int64(len(found[i])))
			}
		})
	}
	wg.Wait()

	for _, ms := range found {
		for _, m := range ms {
			if len(matches) == limit {
				return matches, true
			}
			matches = append(matches, m)
		}
	}
	return matches, total.Load() > int64(limit)
}

// file is a search's work on one file: its open buffer's lines if it has
// one, or its text read from the disk.
func (s searcher) file(root, rel string, open Lines, buf, low *[]byte) []Match {
	if lines, ok := open(rel); ok {
		return s.lines(rel, lines)
	}
	data := readText(filepath.Join(root, filepath.FromSlash(rel)), buf)
	if data == nil {
		return nil
	}
	return s.text(rel, data, low)
}

// MatchLines finds the lines re matches among lines, which file names: one
// file's part of a search, or a single buffer's, for occur.
func MatchLines(file string, lines []string, re *regexp.Regexp) []Match {
	return newSearcher(re).lines(file, lines)
}

// lines finds the matching lines among lines. A line without the text every
// match needs is passed over before the pattern is tried on it.
func (s searcher) lines(file string, lines []string) []Match {
	var out []Match
	for i, line := range lines {
		if !s.lineMayMatch(line) {
			continue
		}
		b := unsafe.Slice(unsafe.StringData(line), len(line))
		if locs := s.locs(b); len(locs) > 0 {
			out = append(out, shown(file, i, line, locs))
		}
	}
	return out
}

// text finds the matching lines of a file's text, as ripgrep does: it looks
// through the whole text for what every match needs - with the processor's
// fast byte search, not a line at a time - and only a line where that turns
// up is split out and the pattern tried on it. Line numbers are counted, by
// the same fast search for newlines, only as far as the next such line.
// Splitting every file into lines to try each was nearly all of what a
// search cost, and running the pattern over a whole file, which Go's regexp
// does far more slowly than over a line, most of the rest.
//
// Ignoring case, the literal is looked for in a copy of the text lowered -
// eight bytes at a time, by arithmetic on words rather than a byte at a time -
// with the same fast search as when case counts. Hunting its letter in both
// cases instead took two scans, and a call per place either turned up; this
// is the common case, since a search typed in lower case ignores case. The
// copy's bytes are where the text's are, so a place found in it is the
// place in the text. low holds the copy, grown as need be.
func (s searcher) text(file string, data []byte, low *[]byte) []Match {
	var out []Match
	find := s.index
	if s.fold && s.lit != nil && low != nil {
		lowered := lowerASCII(data, low)
		lit := s.lowLit
		find = func(b []byte) int {
			// b is data from some offset; the same offset of the copy.
			off := len(data) - len(b)
			return bytes.Index(lowered[off:], lit)
		}
	}
	line, counted := 0, 0 // line is the number of the line starting at counted
	for pos := 0; pos < len(data); {
		hit := pos
		if s.lit != nil {
			i := find(data[pos:])
			if i < 0 {
				break
			}
			hit = pos + i
			if !s.neighbours(data, hit) {
				pos = hit + 1
				continue
			}
		}
		start := counted + bytes.LastIndexByte(data[counted:hit], '\n') + 1
		end := len(data)
		if i := bytes.IndexByte(data[hit:], '\n'); i >= 0 {
			end = hit + i
		}
		line += bytes.Count(data[counted:start], []byte{'\n'})
		counted = start
		l := bytes.TrimSuffix(data[start:end], []byte{'\r'})
		if locs := s.locs(l); len(locs) > 0 {
			out = append(out, shown(file, line, string(l), locs))
		}
		pos = end + 1
	}
	return out
}

// neighbours reports whether the bytes either side of the literal found at
// hit are ones a match can have there.
func (s searcher) neighbours(data []byte, hit int) bool {
	if s.before != nil && (hit == 0 || !s.before.has(data[hit-1])) {
		return false
	}
	end := hit + len(s.lit)
	return s.after == nil || end < len(data) && s.after.has(data[end])
}

// locs is where the pattern matches in line, as byte ranges.
func (s searcher) locs(line []byte) [][]int {
	if !s.only {
		return s.re.FindAllIndex(line, -1)
	}
	var locs [][]int
	for at := 0; at <= len(line); {
		i := s.index(line[at:])
		if i < 0 {
			break
		}
		locs = append(locs, []int{at + i, at + i + len(s.lit)})
		at += i + max(len(s.lit), 1)
	}
	return locs
}

// readText reads a text file into buf, growing it as need be, or gives nil
// for one that is not text, too large, or unreadable. What it returns is buf
// and is good until the next read into it.
func readText(p string, buf *[]byte) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxFileSize {
		return nil
	}
	n := int(fi.Size())
	if cap(*buf) < n {
		*buf = make([]byte, n)
	}
	data := (*buf)[:n]
	if _, err := io.ReadFull(f, data); err != nil {
		return nil
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return nil
	}
	return data
}

// splitLines is a file's text as lines, without their line endings.
func splitLines(data []byte) []string {
	s := strings.TrimSuffix(string(data), "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// searcher is a pattern made ready to search with.
//
// A pattern that is only text is looked for as text, which is much faster:
// with the bytes it is, or, ignoring case - which is how a search typed in
// lower case is made - with a scan for its rarest letter in either case.
//
// A pattern that is more than text usually still needs some: need\w+ := has
// "need" in every match. The longest such piece, lit, is looked for, and the
// pattern is tried only where it turns up.
type searcher struct {
	re   *regexp.Regexp
	lit  []byte
	fold bool
	// only reports that the pattern is the literal and nothing else, so
	// finding the literal is finding a match.
	only bool
	// lowLit is lit lowered, for a search that ignores case.
	lowLit []byte
	// before and after are the bytes that can come just before and just
	// after the literal in a match, when the pattern says: [A-Z]+_[0-9]
	// needs a capital before the _ and a digit after it. Most places the
	// literal turns up fail that, and are passed over without the pattern
	// being tried on their line. Nil where anything can.
	before, after *byteSet
}

// byteSet is a set of bytes.
type byteSet [4]uint64

func (s *byteSet) add(b byte)      { s[b>>6] |= 1 << (b & 63) }
func (s *byteSet) has(b byte) bool { return s[b>>6]&(1<<(b&63)) != 0 }
func (s *byteSet) union(o *byteSet) {
	for i := range s {
		s[i] |= o[i]
	}
}
func (s *byteSet) addRange(lo, hi int) {
	for b := lo; b <= hi && b < utf8.RuneSelf; b++ {
		s.add(byte(b))
	}
	if hi >= utf8.RuneSelf {
		// A rune beyond ASCII is several bytes, any of which can be the
		// one next to the literal.
		for b := utf8.RuneSelf; b < 256; b++ {
			s.add(byte(b))
		}
	}
}

// edgeSet is the bytes the first rune (or last, for last) of what re
// matches can be made of, or nil when re does not say: it can match
// nothing, or it is not so simple as one character or a run of them.
func edgeSet(re *syntax.Regexp, last bool) *byteSet {
	switch re.Op {
	case syntax.OpConcat:
		// From the edge inward: an operand that can match nothing - x* or
		// x? - lets the one beyond it be at the edge too, and one that is
		// only a place, as \b is, is passed over.
		var acc byteSet
		for i := range re.Sub {
			sub := re.Sub[i]
			if last {
				sub = re.Sub[len(re.Sub)-1-i]
			}
			switch sub.Op {
			case syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpBeginLine,
				syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpEmptyMatch:
				continue
			case syntax.OpStar, syntax.OpQuest:
				s := edgeSet(sub.Sub[0], last)
				if s == nil {
					return nil
				}
				acc.union(s)
				continue
			}
			s := edgeSet(sub, last)
			if s == nil {
				return nil
			}
			acc.union(s)
			return &acc
		}
		return nil
	case syntax.OpPlus:
		return edgeSet(re.Sub[0], last)
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return edgeSet(re.Sub[0], last)
		}
	case syntax.OpCharClass:
		var s byteSet
		for i := 0; i+1 < len(re.Rune); i += 2 {
			s.addRange(int(re.Rune[i]), int(re.Rune[i+1]))
		}
		return &s
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return nil
		}
		r := re.Rune[0]
		if last {
			r = re.Rune[len(re.Rune)-1]
		}
		var s byteSet
		s.addRange(int(r), int(r))
		if re.Flags&syntax.FoldCase != 0 && r < utf8.RuneSelf {
			s.add(lower(byte(r)))
			s.add(upper(byte(r)))
		}
		return &s
	}
	return nil
}

func newSearcher(re *regexp.Regexp) searcher {
	p := searcher{re: re}
	parsed, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return p
	}
	parsed = parsed.Simplify()
	lit := parsed
	switch parsed.Op {
	case syntax.OpLiteral:
		p.only = true
	case syntax.OpConcat:
		// Every operand of a concatenation is needed, so its longest
		// literal operand is, and whatever its neighbours say must come
		// next to it.
		lit = nil
		k := 0
		for i, sub := range parsed.Sub {
			if sub.Op == syntax.OpLiteral && (lit == nil || len(sub.Rune) > len(lit.Rune)) {
				lit, k = sub, i
			}
		}
		if lit == nil {
			return p
		}
		if k > 0 {
			p.before = edgeSet(parsed.Sub[k-1], true)
		}
		if k+1 < len(parsed.Sub) {
			p.after = edgeSet(parsed.Sub[k+1], false)
		}
	default:
		return p
	}
	fold := lit.Flags&syntax.FoldCase != 0
	for _, r := range lit.Rune {
		if r >= utf8.RuneSelf && fold {
			p.only = false
			return p // folding beyond ASCII is left to the pattern
		}
	}
	p.lit, p.fold = []byte(string(lit.Rune)), fold
	if len(p.lit) == 0 {
		p.lit, p.only = nil, false
	}
	if fold {
		p.lowLit = bytes.ToLower(p.lit)
	}
	return p
}

// lowerASCII is data with its ASCII capitals lowered, in to's storage, grown
// as need be. Eight bytes at a time: a byte is a capital when adding to it
// what takes 'A' to 0x80 sets its top bit and adding what takes 'Z'+1 there
// does not, and a byte already past 0x7f - part of a character beyond ASCII -
// is left alone. Lowering a capital is setting its 0x20 bit.
func lowerASCII(data []byte, to *[]byte) []byte {
	if cap(*to) < len(data) {
		*to = make([]byte, len(data))
	}
	out := (*to)[:len(data)]
	const ones = 0x0101010101010101
	const tops = 0x8080808080808080
	i := 0
	for ; i+8 <= len(data); i += 8 {
		v := binary.LittleEndian.Uint64(data[i:])
		seven := v &^ tops
		capital := (seven + (0x80-'A')*ones) &^ (seven + (0x80-'Z'-1)*ones) &^ v & tops
		binary.LittleEndian.PutUint64(out[i:], v|capital>>2)
	}
	for ; i < len(data); i++ {
		out[i] = lower(data[i])
	}
	return out
}

// index is where the text every match needs next turns up in data, or -1.
func (p searcher) index(data []byte) int {
	if p.fold {
		return indexFold(data, p.lit)
	}
	return bytes.Index(data, p.lit)
}

// commonness ranks bytes by how often they turn up in code and prose, most
// often first; a byte not in it is rarer than all of them.
const commonness = " etaoinsrlcdhupmfgbywvkxjqz_.,;:()[]{}=\"'0123456789"

// rarest is the index of needle's least common byte, which a search anchors
// on: scanning for the n of "needle" stops at every n in the file, and for
// the d hardly at all.
func rarest(needle []byte) int {
	best, rank := 0, -1
	for i, c := range needle {
		r := strings.IndexByte(commonness, lower(c))
		if r < 0 {
			return i
		}
		if r > rank {
			best, rank = i, r
		}
	}
	return best
}

// lineMayMatch reports whether line has the text every match needs: a cheap
// check before the pattern is tried on it.
func (p searcher) lineMayMatch(line string) bool {
	switch {
	case p.lit == nil:
		return true
	case p.fold:
		// The line is only read, so it is looked at as bytes in place rather
		// than copied: this runs on every line of every file that passed.
		return indexFold(unsafe.Slice(unsafe.StringData(line), len(line)), p.lit) >= 0
	}
	return strings.Contains(line, string(p.lit))
}

// indexFold is the first place in data that needle, which is ASCII, occurs
// ignoring case, or -1.
func indexFold(data, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	a := rarest(needle)
	lo, up := lower(needle[a]), upper(needle[a])
	for i := a; i+len(needle)-a <= len(data); {
		// The next of either case: the other one is looked for only as far
		// as the first, so neither scan runs past what is needed.
		j := bytes.IndexByte(data[i:], lo)
		if lo != up {
			end := len(data)
			if j >= 0 {
				end = i + j
			}
			if k := bytes.IndexByte(data[i:end], up); k >= 0 {
				j = k
			}
		}
		if j < 0 {
			return -1
		}
		i += j
		if start := i - a; start >= 0 && start+len(needle) <= len(data) && bytes.EqualFold(data[start:start+len(needle)], needle) {
			return start
		}
		i++
	}
	return -1
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func upper(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// shown builds a line's Match from the byte ranges that matched in it.
func shown(rel string, i int, line string, locs [][]int) Match {
	col := func(b int) int { return utf8.RuneCountInString(line[:b]) }
	m := Match{File: rel, Line: i, Col: col(locs[0][0])}

	// The part shown: the whole line, or shownWidth runes of it starting a
	// little before the first match.
	from, to := 0, len(line)
	lead, tail := "", ""
	if utf8.RuneCountInString(line) > shownWidth {
		from = backRunes(line, locs[0][0], shownWidth/4)
		to = forwardRunes(line, from, shownWidth)
		if from > 0 {
			lead = "…"
		}
		if to < len(line) {
			tail = "…"
		}
	}
	text := line[from:to]
	m.Text = lead + sanitize(text) + tail

	offset := utf8.RuneCountInString(lead)
	for _, l := range locs {
		s, e := max(l[0], from), min(l[1], to)
		if s >= e {
			continue
		}
		m.Spans = append(m.Spans, [2]int{
			offset + utf8.RuneCountInString(line[from:s]),
			offset + utf8.RuneCountInString(line[from:e]),
		})
	}
	return m
}

// backRunes steps back n runes from byte offset b.
func backRunes(s string, b, n int) int {
	for ; n > 0 && b > 0; n-- {
		_, size := utf8.DecodeLastRuneInString(s[:b])
		b -= size
	}
	return b
}

// forwardRunes steps forward n runes from byte offset b.
func forwardRunes(s string, b, n int) int {
	for ; n > 0 && b < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[b:])
		b += size
	}
	return b
}

// sanitize keeps a result on its one line: a tab becomes a space, so the
// columns a result shows are the runes it has, and any other control
// character a '?', as a listing shows one in a file name. Bytes that are not
// UTF-8 stay one rune each, U+FFFD, so the spans still line up.
func sanitize(s string) string {
	clean := true
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
