package project

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
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
	found := make([][]Match, len(files))
	var next, total atomic.Int64
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= len(files) || total.Load() >= MaxMatches {
					return
				}
				found[i] = searchFile(root, files[i], re, open)
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
func searchFile(root, rel string, re *regexp.Regexp, open Lines) []Match {
	lines, ok := open(rel)
	if !ok {
		lines = readLines(filepath.Join(root, filepath.FromSlash(rel)))
	}
	var out []Match
	for i, line := range lines {
		locs := re.FindAllStringIndex(line, -1)
		if len(locs) == 0 {
			continue
		}
		// An empty match - a pattern like "x*" - says nothing about where to
		// look; a line matched only by those is still a match, at its start.
		out = append(out, shown(rel, i, line, locs))
	}
	return out
}

// readLines reads a text file as lines, or nil for one that is not text, too
// large, or unreadable.
func readLines(p string) []string {
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxFileSize {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return nil
	}
	s := string(data)
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
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
