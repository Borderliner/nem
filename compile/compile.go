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

// formats are the shapes of location compilers and runtimes print, most
// particular first: each is tried in turn and the first to match wins. The
// file may not contain spaces except in Python's quoted form - allowing them
// in the others makes a log line's timestamp read as a file name.
var formats = []*regexp.Regexp{
	// Python's tracebacks: File "app/main.py", line 12, in f
	regexp.MustCompile(`^\s*File "(?P<file>[^"]+)", line (?P<line>\d+)`),
	// Rust's pointer under a diagnostic:   --> src/main.rs:4:5
	regexp.MustCompile(`^\s*--> (?P<file>[^\s:]+):(?P<line>\d+):(?P<col>\d+)`),
	// A JavaScript stack frame: at f (/app/x.js:10:5), or at /app/x.js:10:5
	regexp.MustCompile(`\bat (?:[^()]*\()?(?P<file>[^()\s:]+):(?P<line>\d+):(?P<col>\d+)\)?\s*$`),
	// TypeScript and MSVC: src/a.ts(12,5): error TS2322
	regexp.MustCompile(`^\s*(?P<file>[^\s(:]+)\((?P<line>\d+)(?:,(?P<col>\d+))?\)\s?:`),
	// Everyone else - gcc, clang, go, javac, eslint's unix format, go test's
	// indented failures, Go's panics: file:line:col: or file:line: or a
	// bare file:line at the end of a line. A Windows drive letter is allowed
	// in front.
	regexp.MustCompile(`^\s*(?P<file>(?:[A-Za-z]:)?[^\s:]+):(?P<line>\d+)(?::(?P<col>\d+))?(?::|\s|$)`),
}

// Parse finds the place in a file line points at, and false if it points at
// none.
func Parse(line string) (Loc, bool) {
	for _, re := range formats {
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

// ansi matches the escape sequences a program writes for a terminal: colour
// and cursor control sequences, and the titles and links a terminal is told
// about.
var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// Clean turns one line of a program's output into plain text: escape
// sequences go, and a carriage return starts the line over, as it would on a
// terminal - a progress bar that redraws itself leaves only where it got to.
// Other control characters, which would only show as noise, are dropped, but
// a tab stays.
func Clean(line string) string {
	if strings.IndexByte(line, 0x1b) >= 0 {
		line = ansi.ReplaceAllString(line, "")
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
