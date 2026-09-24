package dired

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Borderliner/nem/syntax"
)

// FirstEntry is the buffer line of the first entry: line 0 is the header, line
// 1 is blank.
const FirstEntry = 2

// MarkColumn is the rune column of the mark on an entry line.
const MarkColumn = 1

// The fixed-width columns of an entry line with details shown. The editor
// finds the name and the mark by column rather than by parsing the line, so
// these are a contract, and every field before the name is padded to exactly
// its width whatever it holds.
const (
	permColumn     = 3
	permWidth      = 10
	sizeColumn     = permColumn + permWidth + 2 // 15
	sizeWidth      = 4
	dateColumn     = sizeColumn + sizeWidth + 2 // 21
	dateWidth      = 12
	detailsNameCol = dateColumn + dateWidth + 2 // 35
	bareNameCol    = 3
)

// sixMonths is where a date stops showing the time and starts showing the
// year, as ls does: past that, the year is the more useful thing to know.
const sixMonths = 182 * 24 * time.Hour

// Listing is a formatted directory: the buffer text line by line and, line for
// line, how to colour it.
type Listing struct {
	// Dir is the directory, absolute and cleaned.
	Dir string
	// Lines are the header, a blank line, then one line per shown entry - or a
	// single placeholder line when nothing is shown.
	Lines []string
	// Spans colour Lines; len(Spans) == len(Lines).
	Spans [][]syntax.Span
	// Entries are the shown entries in display order; Entries[i] is on line
	// FirstEntry+i.
	Entries []Entry
	// Hidden counts the hidden entries this listing omits (0 when ShowHidden).
	Hidden int
}

// Format lays out entries (as Read returns them, in any order) for display.
//
// marks maps an entry's Name to its mark: ' ' or absent is unmarked, '*' is
// marked, 'D' is flagged for deletion, and any other rune is shown as it is
// and coloured like '*'. now decides between showing a time and a year, and is
// a parameter so a test's output does not depend on when it runs. home, if not
// "", is abbreviated to "~" in the header. The caller passes an absolute dir.
func Format(dir string, entries []Entry, marks map[string]rune, opts Options, now time.Time, home string) *Listing {
	l := &Listing{Dir: filepath.Clean(dir)}
	shown := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Hidden() && !opts.ShowHidden {
			l.Hidden++
			continue
		}
		shown = append(shown, e)
	}
	sortEntries(shown, opts.Sort)
	l.Entries = shown

	n := FirstEntry + max(len(shown), 1)
	l.Lines = make([]string, 0, n)
	l.Spans = make([][]syntax.Span, 0, n)

	head, headSpans := header(l.Dir, home, shown, l.Hidden)
	l.Lines = append(l.Lines, head, "")
	l.Spans = append(l.Spans, headSpans, nil)

	if len(shown) == 0 {
		// A buffer with nothing under the header reads as broken; saying why
		// it is empty - and that hidden files exist, when they do - answers
		// the question before the user asks it.
		text := "(empty)"
		if l.Hidden > 0 {
			text = "(only hidden files)"
		}
		line := "   " + text
		l.Lines = append(l.Lines, line)
		l.Spans = append(l.Spans, []syntax.Span{{Start: 3, End: utf8.RuneCountInString(line), Class: syntax.Comment}})
		return l
	}

	for _, e := range shown {
		line, spans := FormatEntry(e, marks[e.Name], opts, now)
		l.Lines = append(l.Lines, line)
		l.Spans = append(l.Spans, spans)
	}
	return l
}

// FormatEntry lays out one entry line exactly as Format would put it in a
// listing, so the editor can redraw a single line when its mark changes
// without re-formatting the whole directory.
func FormatEntry(e Entry, mark rune, opts Options, now time.Time) (string, []syntax.Span) {
	mark = normalizeMark(mark)

	var b strings.Builder
	var spans []syntax.Span
	add := func(start, end int, c syntax.Class) {
		if end > start {
			spans = append(spans, syntax.Span{Start: start, End: end, Class: c})
		}
	}

	b.WriteByte(' ')
	b.WriteRune(mark)
	b.WriteByte(' ')
	switch mark {
	case ' ':
	case 'D':
		add(MarkColumn, MarkColumn+1, syntax.Number)
	default:
		add(MarkColumn, MarkColumn+1, syntax.Type)
	}

	if !opts.HideDetails {
		b.WriteString(permString(e))
		b.WriteString("  ")
		size := "-"
		if !e.IsDir {
			size = humanSize(e.Size)
		}
		fmt.Fprintf(&b, "%*s", sizeWidth, size)
		b.WriteString("  ")
		b.WriteString(formatDate(e.ModTime, now))
		b.WriteString("  ")
		add(permColumn, permColumn+permWidth, syntax.Comment)
		if !e.ModTime.IsZero() {
			add(dateColumn, dateColumn+dateWidth, syntax.Comment)
		}
	}

	name := sanitize(e.Name)
	if e.IsDir {
		// A marker as in ls -F, not a path separator, so "/" on every OS.
		name += "/"
	}
	start := NameColumn(opts)
	end := start + utf8.RuneCountInString(name)
	b.WriteString(name)
	if c, ok := nameClass(e, mark); ok {
		add(start, end, c)
	}

	if e.Mode&fs.ModeSymlink != 0 {
		// The target is shown even when Readlink failed and it is empty: the
		// arrow alone still tells the user this is a link.
		target := " → " + sanitize(e.Target)
		b.WriteString(target)
		add(end, end+utf8.RuneCountInString(target), syntax.Comment)
	}
	return b.String(), spans
}

// normalizeMark maps an absent mark to ' ' and keeps any control rune out of
// the line, where a newline would split one entry across two buffer lines.
func normalizeMark(r rune) rune {
	switch {
	case r == 0:
		return ' '
	case !utf8.ValidRune(r) || unicode.IsControl(r):
		return '?'
	}
	return r
}

// nameClass decides how an entry's name is coloured.
//
// The mark wins over everything, because it is what the user is acting on
// right now: a directory flagged for deletion has to look flagged, not like a
// directory. After that, the more surprising property wins - a broken link
// over a link, a link over what it points to.
func nameClass(e Entry, mark rune) (syntax.Class, bool) {
	switch {
	case mark == 'D':
		return syntax.Number, true
	case mark != ' ':
		return syntax.Type, true
	case e.Broken:
		return syntax.Number, true
	case e.Mode&fs.ModeSymlink != 0:
		return syntax.Keyword, true
	case e.IsDir:
		return syntax.Function, true
	case e.Executable():
		return syntax.String, true
	case e.special():
		return syntax.Constant, true
	case e.Hidden():
		return syntax.Comment, true
	}
	return syntax.Plain, false
}

// NameColumn is the rune column at which an entry's name starts on its line.
// It depends only on opts: everything before the name is fixed-width.
func NameColumn(opts Options) int {
	if opts.HideDetails {
		return bareNameCol
	}
	return detailsNameCol
}

// EntryAt returns the entry on buffer line line, and false for the header, the
// blank line, the placeholder, or a line out of range.
func (l *Listing) EntryAt(line int) (Entry, bool) {
	i := line - FirstEntry
	if i < 0 || i >= len(l.Entries) {
		return Entry{}, false
	}
	return l.Entries[i], true
}

// LineOf returns the buffer line of the shown entry called name.
//
// It scans rather than consulting an index built by Format: the editor may
// edit Entries in place after an operation, and a scan cannot go stale.
func (l *Listing) LineOf(name string) (int, bool) {
	for i, e := range l.Entries {
		if e.Name == name {
			return FirstEntry + i, true
		}
	}
	return 0, false
}

// header builds line 0: the abbreviated directory, then a summary of what is
// shown.
func header(dir, home string, shown []Entry, hidden int) (string, []syntax.Span) {
	path := sanitize(abbreviate(dir, home))
	sep := string(filepath.Separator)
	if !strings.HasSuffix(path, sep) {
		path += sep
	}

	dirs, files := 0, 0
	var total int64
	for _, e := range shown {
		if e.IsDir {
			dirs++
			continue
		}
		files++
		total += e.Size
	}
	summary := plural(dirs, "dir") + " · " + plural(files, "file") + " · " + humanSize(total)
	if hidden > 0 {
		summary += fmt.Sprintf(" · %d hidden", hidden)
	}

	var spans []syntax.Span
	pathStart := 1
	pathEnd := pathStart + utf8.RuneCountInString(path)
	// The final component is what the user needs at a glance; the leading
	// path is context, so it is dimmed. "/" and "~/" have no leading part.
	if cut := strings.LastIndex(strings.TrimSuffix(path, sep), sep); cut >= 0 {
		mid := pathStart + utf8.RuneCountInString(path[:cut+len(sep)])
		spans = append(spans, syntax.Span{Start: pathStart, End: mid, Class: syntax.Comment})
		pathStart = mid
	}
	spans = append(spans, syntax.Span{Start: pathStart, End: pathEnd, Class: syntax.Function})
	sumStart := pathEnd + 2
	spans = append(spans, syntax.Span{Start: sumStart, End: sumStart + utf8.RuneCountInString(summary), Class: syntax.Comment})

	return " " + path + "  " + summary, spans
}

// abbreviate replaces a leading home directory with "~".
func abbreviate(dir, home string) string {
	if home == "" {
		return dir
	}
	home = filepath.Clean(home)
	if dir == home {
		return "~"
	}
	prefix := home
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if rest, ok := strings.CutPrefix(dir, prefix); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return dir
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// permString renders the mode as ls does, in exactly permWidth runes.
// fs.FileMode.String is not usable here: it prints one letter per type bit
// set, so its width varies and would shift every column after it.
func permString(e Entry) string {
	if e.unknown() {
		return strings.Repeat("?", permWidth)
	}
	m := e.Mode
	var b [permWidth]byte
	switch {
	case m&fs.ModeDir != 0:
		b[0] = 'd'
	case m&fs.ModeSymlink != 0:
		b[0] = 'l'
	case m&fs.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&fs.ModeSocket != 0:
		b[0] = 's'
	case m&fs.ModeCharDevice != 0:
		// Checked before ModeDevice: a character device carries both bits.
		b[0] = 'c'
	case m&fs.ModeDevice != 0:
		b[0] = 'b'
	default:
		b[0] = '-'
	}
	const rwx = "rwxrwxrwx"
	for i := range 9 {
		if m&(1<<(8-i)) != 0 {
			b[1+i] = rwx[i]
		} else {
			b[1+i] = '-'
		}
	}
	// The special bits share the execute slots, lowercase when execute is
	// also set and uppercase when it is not - the uppercase form is usually a
	// mistake worth noticing, which is why ls distinguishes them.
	special := func(slot int, set bool, lower byte) {
		if !set {
			return
		}
		if b[slot] == 'x' {
			b[slot] = lower
		} else {
			b[slot] = lower - 'a' + 'A'
		}
	}
	special(3, m&fs.ModeSetuid != 0, 's')
	special(6, m&fs.ModeSetgid != 0, 's')
	special(9, m&fs.ModeSticky != 0, 't')
	return string(b[:])
}

// humanSize renders a byte count in at most sizeWidth runes.
//
// The rounding thresholds are what keep it inside that width: a value that
// would round up to 10.0 is shown as an integer instead, and one that would
// round up to 1000 moves to the next unit, so neither can grow a fifth rune.
func humanSize(n int64) string {
	if n < 0 {
		n = 0
	}
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	v := float64(n)
	for _, unit := range "KMGTPE" {
		v /= 1024
		switch {
		case v < 9.95:
			return strconv.FormatFloat(v, 'f', 1, 64) + string(unit)
		case v < 999.5:
			return strconv.FormatFloat(v, 'f', 0, 64) + string(unit)
		}
	}
	// Unreachable: the largest int64 is under 8E.
	return "?"
}

// formatDate renders t in exactly dateWidth runes: the time of day for a
// recent file, the year otherwise.
//
// A date more than a minute in the future counts as old, as in ls: a clock
// skewed file shown with only a time of day would pass for today's.
func formatDate(t, now time.Time) string {
	if t.IsZero() {
		return strings.Repeat(" ", dateWidth)
	}
	t = t.In(now.Location())
	if !t.Before(now.Add(-sixMonths)) && !t.After(now.Add(time.Minute)) {
		return t.Format("Jan _2 15:04")
	}
	// The year is padded by hand rather than with the "2006" layout, which
	// prints five digits past 9999 and a sign before year 0 - either would
	// shift the name column. A filesystem can report such dates.
	year := strconv.Itoa(t.Year())
	if len(year) > 5 {
		year = "?????"
	}
	return fmt.Sprintf("%s %5s", t.Format("Jan _2"), year)
}

// sanitize makes a name safe to show on one buffer line: every control rune
// and every byte that is not valid UTF-8 becomes '?'.
//
// A newline in a file name is legal on Unix, and printed raw it would split
// one entry across two lines and misalign every EntryAt below it.
func sanitize(s string) string {
	clean := true
	for _, r := range s {
		if r == utf8.RuneError || unicode.IsControl(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || unicode.IsControl(r) {
			b.WriteByte('?')
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}
