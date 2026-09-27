package project

import (
	"bytes"
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

// Search finds the lines of files, relative to root, that re matches, in the
// order of files and then of lines. More reports that it stopped at
// MaxMatches.
//
// Files are searched in parallel, and a file that is not text - one with a
// NUL byte near its start - or is larger than any source file is passed over.
func Search(root string, files []string, re *regexp.Regexp, open Lines) (matches []Match, more bool) {
	pre := newPrefilter(re)
	found := make([][]Match, len(files))
	var next, total atomic.Int64
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			var buf []byte // each worker reads every file into the one buffer
			for {
				i := int(next.Add(1) - 1)
				if i >= len(files) || total.Load() >= MaxMatches {
					return
				}
				found[i] = searchFile(root, files[i], re, pre, open, &buf)
				total.Add(int64(len(found[i])))
			}
		})
	}
	wg.Wait()

	for _, ms := range found {
		for _, m := range ms {
			if len(matches) == MaxMatches {
				return matches, true
			}
			matches = append(matches, m)
		}
	}
	return matches, total.Load() > MaxMatches
}

// searchFile is Search's work on one file.
//
// A file is looked at whole before it is split into lines, and passed over
// if the pattern cannot match anywhere in it: most files of a project do not
// hold what is searched for, and splitting every one into lines to search
// each was nearly all of a search's time and memory.
func searchFile(root, rel string, re *regexp.Regexp, pre prefilter, open Lines, buf *[]byte) []Match {
	lines, ok := open(rel)
	if !ok {
		data := readText(filepath.Join(root, filepath.FromSlash(rel)), buf)
		if data == nil || !pre.mayMatch(data) {
			return nil
		}
		lines = splitLines(data)
	}
	return matchLines(rel, lines, re, pre)
}

// MatchLines finds the lines re matches among lines, which file names: one
// file's part of a search, or a single buffer's, for occur.
func MatchLines(file string, lines []string, re *regexp.Regexp) []Match {
	return matchLines(file, lines, re, newPrefilter(re))
}

// matchLines is MatchLines with the pattern's prefilter already made. A line
// without the text every match needs is passed over before the pattern is
// tried on it, as a file is.
func matchLines(file string, lines []string, re *regexp.Regexp, pre prefilter) []Match {
	var out []Match
	for i, line := range lines {
		if !pre.lineMayMatch(line) {
			continue
		}
		locs := re.FindAllStringIndex(line, -1)
		if len(locs) == 0 {
			continue
		}
		// An empty match - a pattern like "x*" - says nothing about where to
		// look; a line matched only by those is still a match, at its start.
		out = append(out, shown(file, i, line, locs))
	}
	return out
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

// prefilter answers whether a file can hold a match at all, faster than
// searching it a line at a time.
//
// Anything the pattern matches within a line it also matches in the whole
// text read with ^ and $ at every line - the pattern is compiled that way
// for this - so a file it finds nothing in has no matching line. The
// converse need not hold, since a match in the whole text may run across
// lines; a file that passes is searched line by line all the same.
//
// A pattern that is only text is looked for as text, which is much faster:
// with the bytes it is, or, ignoring case - which is how a search typed in
// lower case is made - with a scan for its first letter in either case.
//
// A pattern that is more than text usually still needs some: need\w+ := has
// "need" in every match. The longest such piece is looked for first, and a
// file without it is passed over without the pattern being tried at all.
type prefilter struct {
	re      *regexp.Regexp
	literal []byte
	fold    bool
	// only reports that the pattern is the literal and nothing else, so
	// finding the literal is finding a match.
	only bool
}

func newPrefilter(re *regexp.Regexp) prefilter {
	p := prefilter{re: regexp.MustCompile("(?m)" + re.String())}
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
		// literal operand is.
		lit = nil
		for _, sub := range parsed.Sub {
			if sub.Op == syntax.OpLiteral && (lit == nil || len(sub.Rune) > len(lit.Rune)) {
				lit = sub
			}
		}
		if lit == nil {
			return p
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
	p.literal, p.fold = []byte(string(lit.Rune)), fold
	return p
}

// mayMatch reports whether data can hold a match.
func (p prefilter) mayMatch(data []byte) bool {
	if p.literal != nil {
		has := bytes.Contains(data, p.literal)
		if p.fold {
			has = indexFold(data, p.literal) >= 0
		}
		if !has || p.only {
			return has
		}
	}
	return p.re.Match(data)
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

// lineMayMatch is mayMatch for a line: only the cheap check of the text
// every match needs, since the pattern is tried on the line next anyway.
func (p prefilter) lineMayMatch(line string) bool {
	switch {
	case p.literal == nil:
		return true
	case p.fold:
		// The line is only read, so it is looked at as bytes in place rather
		// than copied: this runs on every line of every file that passed.
		return indexFold(unsafe.Slice(unsafe.StringData(line), len(line)), p.literal) >= 0
	}
	return strings.Contains(line, string(p.literal))
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
