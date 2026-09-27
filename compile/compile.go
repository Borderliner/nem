// Package compile reads the output of a build or a test run the way emacs's
// compilation mode does: it cleans what a program printed for a terminal into
// plain lines, and finds the lines that point at a place in a file - an
// error, a warning, a failing test - so the editor can take you there.
//
// It knows nothing of buffers or processes. The editor runs the command and
// feeds each line through here, and decides for itself which of the files a
// line names exist.
package compile

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind is how serious a located line is.
type Kind int

const (
	Error Kind = iota
	Warning
	Info
)

// Loc is a place in a file that a line of output points at.
type Loc struct {
	// File is the path as the line gives it: relative to wherever the
	// command ran, as often as not.
	File string
	// Line and Col count from 1, as compilers do. Col is 0 when the line
	// gives none.
	Line, Col int
	Kind      Kind
	// Start and End are the rune range of the line that names the place,
	// for the editor to colour.
	Start, End int
}

// format is one shape of location, and what a line must hold before the
// pattern is worth trying on it. A build can print a million lines, nearly
// none of them locations, and a plain substring rejects one in a fraction of
// the time a pattern takes to fail.
type format struct {
	re *regexp.Regexp
	// hint is text the line must contain, and digitAfter a byte that must be
	// followed somewhere by a digit; either may be empty or zero.
	hint       string
	digitAfter byte
}

// formats are the shapes of location compilers and runtimes print, most
// particular first: each is tried in turn and the first to match wins. The
// file may not contain spaces except in Python's quoted form - allowing them
// in the others makes a log line's timestamp read as a file name.
var formats = []format{
	// Python's tracebacks: File "app/main.py", line 12, in f
	{re: regexp.MustCompile(`^\s*File "(?P<file>[^"]+)", line (?P<line>\d+)`), hint: `File "`},
	// Rust's pointer under a diagnostic:   --> src/main.rs:4:5
	{re: regexp.MustCompile(`^\s*--> (?P<file>[^\s:]+):(?P<line>\d+):(?P<col>\d+)`), hint: "--> "},
	// A JavaScript stack frame: at f (/app/x.js:10:5), or at /app/x.js:10:5
	{re: regexp.MustCompile(`\bat (?:[^()]*\()?(?P<file>[^()\s:]+):(?P<line>\d+):(?P<col>\d+)\)?\s*$`), hint: "at ", digitAfter: ':'},
	// TypeScript and MSVC: src/a.ts(12,5): error TS2322
	{re: regexp.MustCompile(`^\s*(?P<file>[^\s(:]+)\((?P<line>\d+)(?:,(?P<col>\d+))?\)\s?:`), digitAfter: '('},
	// Everyone else - gcc, clang, go, javac, eslint's unix format, go test's
	// indented failures, Go's panics: file:line:col: or file:line: or a
	// bare file:line at the end of a line. A Windows drive letter is allowed
	// in front.
	{re: regexp.MustCompile(`^\s*(?P<file>(?:[A-Za-z]:)?[^\s:]+):(?P<line>\d+)(?::(?P<col>\d+))?(?::|\s|$)`), digitAfter: ':'},
}

// worthTrying reports whether line holds what f needs before its pattern
// can match.
func (f format) worthTrying(line string) bool {
	if f.hint != "" && !strings.Contains(line, f.hint) {
		return false
	}
	if f.digitAfter == 0 {
		return true
	}
	for i := 0; i+1 < len(line); i++ {
		if line[i] == f.digitAfter && line[i+1] >= '0' && line[i+1] <= '9' {
			return true
		}
	}
	return false
}

// Parse finds the place in a file line points at, and false if it points at
// none.
func Parse(line string) (Loc, bool) {
	for _, f := range formats {
		if !f.worthTrying(line) {
			continue
		}
		re := f.re
		m := re.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		group := func(name string) (string, int, int) {
			i := re.SubexpIndex(name)
			if i < 0 || m[2*i] < 0 {
				return "", -1, -1
			}
			return line[m[2*i]:m[2*i+1]], m[2*i], m[2*i+1]
		}
		file, fs, _ := group("file")
		ln, _, le := group("line")
		col, _, ce := group("col")
		n, err := strconv.Atoi(ln)
		if err != nil || n == 0 || isNumber(file) {
			continue
		}
		loc := Loc{File: file, Line: n, Kind: kindOf(line)}
		loc.Col, _ = strconv.Atoi(col)
		end := le
		if ce > end {
			end = ce
		}
		loc.Start = utf8.RuneCountInString(line[:fs])
		loc.End = utf8.RuneCountInString(line[:end])
		return loc, true
	}
	return Loc{}, false
}

// isNumber reports a "file" that is only digits: a time like 10:42:07 read
// as file 10, line 42.
func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// kindOf judges a located line's seriousness by what it says, as emacs does
// when a format has no field for it.
func kindOf(line string) Kind {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "warning"):
		return Warning
	case strings.Contains(l, "note:") || strings.Contains(l, "info:"):
		return Info
	}
	return Error
}

// Clean turns one line of a program's output into plain text: escape
// sequences go, and a carriage return starts the line over, as it would on a
// terminal - a progress bar that redraws itself leaves only where it got to.
// Other control characters, which would only show as noise, are dropped, but
// a tab stays.
func Clean(line string) string {
	if strings.IndexByte(line, 0x1b) >= 0 {
		line = stripEscapes(line)
	}
	line = strings.TrimSuffix(line, "\r")
	if i := strings.LastIndexByte(line, '\r'); i >= 0 {
		line = line[i+1:]
	}
	clean := true
	for i := 0; i < len(line); i++ {
		if c := line[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return line
	}
	var b strings.Builder
	for _, r := range line {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// stripEscapes removes the escape sequences a program writes for a terminal:
// colour and cursor control sequences (ESC [ ... final), the titles and links
// a terminal is told about (ESC ] ... BEL or ESC \), character set switches
// (ESC ( B, which tput sgr0 ends with), and two-byte escapes.
// By hand rather than by a pattern, since a test run in colour has one on
// nearly every line.
func stripEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			j := strings.IndexByte(s[i:], 0x1b)
			if j < 0 {
				j = len(s) - i
			}
			b.WriteString(s[i : i+j])
			i += j
			continue
		}
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI: parameters and intermediates, then a final byte
			j := i + 2
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
				j++
			}
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
				j++
			}
			if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
				j++
			}
			i = j
		case ']': // OSC: up to BEL or ESC \
			j := i + 2
			for j < len(s) && s[j] != 0x07 && s[j] != 0x1b {
				j++
			}
			switch {
			case j < len(s) && s[j] == 0x07:
				j++
			case j+1 < len(s) && s[j] == 0x1b && s[j+1] == '\\':
				j += 2
			}
			i = j
		case '(', ')', '*', '+': // a character set: one more byte names it
			i = min(i+3, len(s))
		default:
			if s[i+1] >= 0x40 && s[i+1] <= 0x5f {
				i += 2
			} else {
				i++
			}
		}
	}
	return b.String()
}
