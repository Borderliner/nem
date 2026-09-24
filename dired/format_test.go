package dired

import (
	"io/fs"
	"math"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Borderliner/nem/syntax"
)

// now is the test clock. Every date below is relative to it, so the expected
// lines do not depend on when the tests run.
var now = time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

var (
	recent = now.Add(-26 * time.Hour)                               // Sep 23 10:00
	old    = time.Date(2025, time.January, 2, 3, 4, 0, 0, time.UTC) // Jan  2  2025
)

// sample is a small fixed directory, built by hand so Format is tested
// against exact values rather than whatever a filesystem reports.
func sample() []Entry {
	return []Entry{
		{Name: "run.sh", Mode: 0o755, Size: 120, ModTime: recent, resolved: 0o755},
		{Name: "main.go", Mode: 0o644, Size: 1500, ModTime: old},
		{Name: "link.txt", Mode: fs.ModeSymlink | 0o777, Size: 8, ModTime: recent, Target: "real.txt", resolved: 0o644},
		{Name: ".profile", Mode: 0o644, Size: 10, ModTime: old},
		{Name: "docs", Mode: fs.ModeDir | 0o755, Size: 4096, ModTime: recent, IsDir: true},
	}
}

func p(s string) string { return filepath.FromSlash(s) }

// checkSpans asserts what the renderer relies on: spans are non-empty,
// ascending, non-overlapping and inside the line. A violation corrupts the
// screen rather than merely miscolouring it.
func checkSpans(t *testing.T, l *Listing) {
	t.Helper()
	if len(l.Spans) != len(l.Lines) {
		t.Fatalf("%d span lists for %d lines", len(l.Spans), len(l.Lines))
	}
	for i, line := range l.Lines {
		checkLineSpans(t, i, line, l.Spans[i])
	}
}

func checkLineSpans(t *testing.T, i int, line string, spans []syntax.Span) {
	t.Helper()
	n := utf8.RuneCountInString(line)
	prev := 0
	for j, s := range spans {
		if s.Start >= s.End {
			t.Errorf("line %d (%q): span %d %+v is empty", i, line, j, s)
		}
		if s.Start < prev {
			t.Errorf("line %d (%q): span %d %+v overlaps or precedes the previous (ends at %d)", i, line, j, s, prev)
		}
		if s.Start < 0 || s.End > n {
			t.Errorf("line %d (%q): span %d %+v outside the %d-rune line", i, line, j, s, n)
		}
		prev = s.End
	}
}

func TestFormatWithDetails(t *testing.T) {
	marks := map[string]rune{"docs": 'D', "main.go": '*', "run.sh": ' '}
	l := Format(p("/home/u/src/proj"), sample(), marks, Options{}, now, p("/home/u"))

	want := []string{
		" " + p("~/src/proj/") + "  1 dir · 3 files · 1.6K · 1 hidden",
		"",
		" D drwxr-xr-x     -  Sep 23 10:00  docs/",
		"   lrwxrwxrwx     8  Sep 23 10:00  link.txt → real.txt",
		" * -rw-r--r--  1.5K  Jan  2  2025  main.go",
		"   -rwxr-xr-x   120  Sep 23 10:00  run.sh",
	}
	if !slices.Equal(l.Lines, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(l.Lines, "\n"), strings.Join(want, "\n"))
	}
	if l.Dir != p("/home/u/src/proj") || l.Hidden != 1 {
		t.Errorf("Dir %q Hidden %d, want %q 1", l.Dir, l.Hidden, p("/home/u/src/proj"))
	}
	checkSpans(t, l)

	wantSpans := [][]syntax.Span{
		{{Start: 1, End: 7, Class: syntax.Comment}, {Start: 7, End: 12, Class: syntax.Function}, {Start: 14, End: 47, Class: syntax.Comment}},
		nil,
		{{Start: 1, End: 2, Class: syntax.Number}, {Start: 3, End: 13, Class: syntax.Comment}, {Start: 21, End: 33, Class: syntax.Comment}, {Start: 35, End: 40, Class: syntax.Number}},
		{{Start: 3, End: 13, Class: syntax.Comment}, {Start: 21, End: 33, Class: syntax.Comment}, {Start: 35, End: 43, Class: syntax.Keyword}, {Start: 43, End: 54, Class: syntax.Comment}},
		{{Start: 1, End: 2, Class: syntax.Type}, {Start: 3, End: 13, Class: syntax.Comment}, {Start: 21, End: 33, Class: syntax.Comment}, {Start: 35, End: 42, Class: syntax.Type}},
		{{Start: 3, End: 13, Class: syntax.Comment}, {Start: 21, End: 33, Class: syntax.Comment}, {Start: 35, End: 41, Class: syntax.String}},
	}
	for i := range wantSpans {
		if !slices.Equal(l.Spans[i], wantSpans[i]) {
			t.Errorf("line %d (%q) spans:\n got %v\nwant %v", i, l.Lines[i], l.Spans[i], wantSpans[i])
		}
	}
}

func TestFormatWithoutDetailsShowingHidden(t *testing.T) {
	l := Format(p("/home/u/src/proj"), sample(), nil, Options{ShowHidden: true, HideDetails: true}, now, "")
	want := []string{
		" " + p("/home/u/src/proj/") + "  1 dir · 4 files · 1.6K",
		"",
		"   docs/",
		"   link.txt → real.txt",
		"   main.go",
		"   .profile",
		"   run.sh",
	}
	if !slices.Equal(l.Lines, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(l.Lines, "\n"), strings.Join(want, "\n"))
	}
	if l.Hidden != 0 {
		t.Errorf("Hidden = %d with ShowHidden, want 0", l.Hidden)
	}
	checkSpans(t, l)
	if got, want := l.Spans[5], []syntax.Span{{Start: 3, End: 11, Class: syntax.Comment}}; !slices.Equal(got, want) {
		t.Errorf(".profile spans = %v, want %v", got, want)
	}
}

// The editor finds names by column, so the column must be where the name
// really is on every line, in both layouts.
func TestNameColumnIsWhereTheNameStarts(t *testing.T) {
	for _, opts := range []Options{{ShowHidden: true}, {ShowHidden: true, HideDetails: true}} {
		l := Format(p("/x"), sample(), map[string]rune{"docs": '*'}, opts, now, "")
		col := NameColumn(opts)
		for i, e := range l.Entries {
			line := []rune(l.Lines[FirstEntry+i])
			if got := string(line[col : col+len(e.Name)]); got != e.Name {
				t.Errorf("HideDetails=%v: line %q has %q at column %d, want %q",
					opts.HideDetails, string(line), got, col, e.Name)
			}
			if line[MarkColumn] != ' ' && line[MarkColumn] != '*' {
				t.Errorf("line %q: column %d holds %q, want the mark", string(line), MarkColumn, line[MarkColumn])
			}
		}
	}
	if NameColumn(Options{}) != 35 || NameColumn(Options{HideDetails: true}) != 3 {
		t.Errorf("NameColumn = %d / %d, want 35 / 3", NameColumn(Options{}), NameColumn(Options{HideDetails: true}))
	}
}

func TestFormatEntryMatchesFormat(t *testing.T) {
	marks := map[string]rune{"docs": 'D', "main.go": '*', ".profile": 'x'}
	for _, opts := range []Options{
		{ShowHidden: true},
		{ShowHidden: true, HideDetails: true},
		{ShowHidden: true, Sort: BySize},
	} {
		l := Format(p("/x"), sample(), marks, opts, now, "")
		for i, e := range l.Entries {
			line, spans := FormatEntry(e, marks[e.Name], opts, now)
			if line != l.Lines[FirstEntry+i] {
				t.Errorf("FormatEntry(%s) = %q, Format has %q", e.Name, line, l.Lines[FirstEntry+i])
			}
			if !slices.Equal(spans, l.Spans[FirstEntry+i]) {
				t.Errorf("FormatEntry(%s) spans %v, Format has %v", e.Name, spans, l.Spans[FirstEntry+i])
			}
		}
	}
}

func TestEntryAtAndLineOfRoundTrip(t *testing.T) {
	l := Format(p("/x"), sample(), nil, Options{}, now, "")
	for i, e := range l.Entries {
		line, ok := l.LineOf(e.Name)
		if !ok || line != FirstEntry+i {
			t.Errorf("LineOf(%q) = %d, %v; want %d, true", e.Name, line, ok, FirstEntry+i)
		}
		got, ok := l.EntryAt(line)
		if !ok || got.Name != e.Name {
			t.Errorf("EntryAt(%d) = %q, %v; want %q, true", line, got.Name, ok, e.Name)
		}
	}
	for _, line := range []int{-1, 0, 1, len(l.Lines), len(l.Lines) + 5} {
		if e, ok := l.EntryAt(line); ok {
			t.Errorf("EntryAt(%d) = %q, true; want false", line, e.Name)
		}
	}
	if _, ok := l.LineOf(".profile"); ok {
		t.Error("LineOf found .profile, which this listing hides")
	}
	if _, ok := l.LineOf("absent"); ok {
		t.Error("LineOf found an entry that does not exist")
	}
}

func TestFormatPlaceholders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []Entry
		header  string
		line    string
	}{
		{"empty", nil, " " + p("/tmp/x/") + "  0 dirs · 0 files", "   (empty)"},
		{"only hidden", []Entry{{Name: ".a", Mode: 0o644, Size: 3, ModTime: old}},
			" " + p("/tmp/x/") + "  0 dirs · 0 files · 1 hidden", "   (only hidden files)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := Format(p("/tmp/x"), tc.entries, nil, Options{}, now, "")
			want := []string{tc.header, "", tc.line}
			if !slices.Equal(l.Lines, want) {
				t.Fatalf("lines = %q, want %q", l.Lines, want)
			}
			checkSpans(t, l)
			if got, want := l.Spans[2], []syntax.Span{{Start: 3, End: len(tc.line), Class: syntax.Comment}}; !slices.Equal(got, want) {
				t.Errorf("placeholder spans = %v, want %v", got, want)
			}
			if _, ok := l.EntryAt(FirstEntry); ok {
				t.Error("EntryAt returned an entry for the placeholder line")
			}
			if len(l.Entries) != 0 {
				t.Errorf("Entries = %v, want none", l.Entries)
			}
		})
	}
}

func TestHeaderPath(t *testing.T) {
	for _, tc := range []struct {
		dir, home string
		path      string
		spans     []syntax.Span
	}{
		{"/home/u", "/home/u", "~/", []syntax.Span{{Start: 1, End: 3, Class: syntax.Function}}},
		{"/home/u/x", "/home/u", "~/x/", []syntax.Span{{Start: 1, End: 3, Class: syntax.Comment}, {Start: 3, End: 5, Class: syntax.Function}}},
		{"/home/u/x", "/home/u/", "~/x/", []syntax.Span{{Start: 1, End: 3, Class: syntax.Comment}, {Start: 3, End: 5, Class: syntax.Function}}},
		// A sibling sharing the home prefix is not inside home.
		{"/home/user2", "/home/u", "/home/user2/", []syntax.Span{{Start: 1, End: 7, Class: syntax.Comment}, {Start: 7, End: 13, Class: syntax.Function}}},
		{"/", "/home/u", "/", []syntax.Span{{Start: 1, End: 2, Class: syntax.Function}}},
		{"/etc", "", "/etc/", []syntax.Span{{Start: 1, End: 2, Class: syntax.Comment}, {Start: 2, End: 6, Class: syntax.Function}}},
		{"/home/u/x", "", "/home/u/x/", []syntax.Span{{Start: 1, End: 9, Class: syntax.Comment}, {Start: 9, End: 11, Class: syntax.Function}}},
		{"/home/u/dir/", "/home/u", "~/dir/", []syntax.Span{{Start: 1, End: 3, Class: syntax.Comment}, {Start: 3, End: 7, Class: syntax.Function}}},
		// Rune columns, not bytes: the dimmed prefix ends after "é".
		{"/é/ü", "", "/é/ü/", []syntax.Span{{Start: 1, End: 4, Class: syntax.Comment}, {Start: 4, End: 6, Class: syntax.Function}}},
	} {
		l := Format(p(tc.dir), nil, nil, Options{}, now, p(tc.home))
		wantLine := " " + p(tc.path) + "  0 dirs · 0 files"
		if l.Lines[0] != wantLine {
			t.Errorf("dir %q home %q: header %q, want %q", tc.dir, tc.home, l.Lines[0], wantLine)
			continue
		}
		pathEnd := 1 + utf8.RuneCountInString(tc.path)
		wantSpans := append(slices.Clone(tc.spans), syntax.Span{Start: pathEnd + 2, End: utf8.RuneCountInString(wantLine), Class: syntax.Comment})
		if !slices.Equal(l.Spans[0], wantSpans) {
			t.Errorf("dir %q home %q: header spans %v, want %v", tc.dir, tc.home, l.Spans[0], wantSpans)
		}
	}
}

func TestHeaderSummaryCounts(t *testing.T) {
	d := func(name string) Entry {
		return Entry{Name: name, Mode: fs.ModeDir | 0o755, IsDir: true, Size: 4096, ModTime: old}
	}
	f := func(name string, size int64) Entry { return Entry{Name: name, Mode: 0o644, Size: size, ModTime: old} }
	for _, tc := range []struct {
		name    string
		entries []Entry
		opts    Options
		want    string
	}{
		{"singular", []Entry{d("a"), f("b", 1)}, Options{}, "1 dir · 1 file · 1 B"},
		{"plural", []Entry{d("a"), d("b"), f("c", 1), f("d", 2)}, Options{}, "2 dirs · 2 files · 3 B"},
		{"no dirs", []Entry{f("c", 2048)}, Options{}, "0 dirs · 1 file · 2.0K"},
		// Directory sizes are the size of the directory file itself, which
		// says nothing about what is in it, so they are not totalled.
		{"dirs not totalled", []Entry{d("a")}, Options{}, "1 dir · 0 files"},
		{"hidden counted", []Entry{f(".a", 5), d(".b"), f("c", 1)}, Options{}, "0 dirs · 1 file · 1 B · 2 hidden"},
		{"hidden shown", []Entry{f(".a", 5), d(".b"), f("c", 1)}, Options{ShowHidden: true}, "1 dir · 2 files · 6 B"},
	} {
		l := Format(p("/x"), tc.entries, nil, tc.opts, now, "")
		want := " " + p("/x/") + "  " + tc.want
		if l.Lines[0] != want {
			t.Errorf("%s: header %q, want %q", tc.name, l.Lines[0], want)
		}
	}
}

func TestNameColouring(t *testing.T) {
	link := fs.ModeSymlink | 0o777
	for _, tc := range []struct {
		name string
		e    Entry
		mark rune
		want syntax.Class // Plain means no span at all
	}{
		{"flag beats dir", Entry{Name: "d", Mode: fs.ModeDir | 0o755, IsDir: true}, 'D', syntax.Number},
		{"mark beats dir", Entry{Name: "d", Mode: fs.ModeDir | 0o755, IsDir: true}, '*', syntax.Type},
		{"other mark like *", Entry{Name: "f", Mode: 0o755}, 'C', syntax.Type},
		{"flag beats broken link", Entry{Name: "l", Mode: link, Broken: true}, 'D', syntax.Number},
		{"broken link", Entry{Name: "l", Mode: link, Broken: true}, ' ', syntax.Number},
		{"link to dir", Entry{Name: "l", Mode: link, IsDir: true, resolved: fs.ModeDir | 0o755}, ' ', syntax.Keyword},
		{"link to program", Entry{Name: "l", Mode: link, resolved: 0o755}, ' ', syntax.Keyword},
		{"dir", Entry{Name: "d", Mode: fs.ModeDir | 0o755, IsDir: true}, ' ', syntax.Function},
		{"hidden dir", Entry{Name: ".git", Mode: fs.ModeDir | 0o755, IsDir: true}, ' ', syntax.Function},
		{"executable", Entry{Name: "run", Mode: 0o755}, ' ', syntax.String},
		{"hidden executable", Entry{Name: ".run", Mode: 0o700}, ' ', syntax.String},
		{"pipe", Entry{Name: "fifo", Mode: fs.ModeNamedPipe | 0o644}, ' ', syntax.Constant},
		{"socket", Entry{Name: "sock", Mode: fs.ModeSocket | 0o755}, ' ', syntax.Constant},
		{"char device", Entry{Name: "tty", Mode: fs.ModeDevice | fs.ModeCharDevice | 0o620}, ' ', syntax.Constant},
		{"hidden special", Entry{Name: ".s", Mode: fs.ModeSocket | 0o644}, ' ', syntax.Constant},
		{"hidden", Entry{Name: ".profile", Mode: 0o644}, ' ', syntax.Comment},
		{"plain", Entry{Name: "notes", Mode: 0o644}, ' ', syntax.Plain},
		{"absent mark is unmarked", Entry{Name: "notes", Mode: 0o644}, 0, syntax.Plain},
	} {
		for _, opts := range []Options{{}, {HideDetails: true}} {
			tc.e.ModTime = recent
			line, spans := FormatEntry(tc.e, tc.mark, opts, now)
			checkLineSpans(t, 0, line, spans)
			col := NameColumn(opts)
			got := syntax.Plain
			for _, s := range spans {
				if s.Start == col {
					got = s.Class
					end := col + utf8.RuneCountInString(tc.e.Name)
					if tc.e.IsDir {
						end++ // the trailing "/" is part of the name
					}
					if s.End != end {
						t.Errorf("%s: name span %+v, want it to end at %d", tc.name, s, end)
					}
				}
			}
			if got != tc.want {
				t.Errorf("%s (HideDetails=%v): name coloured %v, want %v", tc.name, opts.HideDetails, got, tc.want)
			}
		}
	}
}

func TestMarkIsNormalised(t *testing.T) {
	e := Entry{Name: "f", Mode: 0o644, ModTime: recent}
	for _, tc := range []struct {
		in, want rune
	}{
		{0, ' '},
		{' ', ' '},
		{'*', '*'},
		{'D', 'D'},
		{'\n', '?'},
		{-1, '?'},
		{'é', 'é'},
	} {
		line, spans := FormatEntry(e, tc.in, Options{HideDetails: true}, now)
		if got := []rune(line)[MarkColumn]; got != tc.want {
			t.Errorf("mark %q shown as %q, want %q", tc.in, got, tc.want)
		}
		checkLineSpans(t, 0, line, spans)
	}
}

func TestPermString(t *testing.T) {
	for _, tc := range []struct {
		mode fs.FileMode
		want string
	}{
		{0o644, "-rw-r--r--"},
		{0, "----------"},
		{fs.ModeDir | 0o755, "drwxr-xr-x"},
		{fs.ModeSymlink | 0o777, "lrwxrwxrwx"},
		{fs.ModeNamedPipe | 0o600, "prw-------"},
		{fs.ModeSocket | 0o755, "srwxr-xr-x"},
		{fs.ModeDevice | fs.ModeCharDevice | 0o620, "crw--w----"},
		{fs.ModeDevice | 0o660, "brw-rw----"},
		{fs.ModeSetuid | 0o755, "-rwsr-xr-x"},
		{fs.ModeSetuid | 0o644, "-rwSr--r--"},
		{fs.ModeSetgid | 0o755, "-rwxr-sr-x"},
		{fs.ModeSetgid | 0o745, "-rwxr-Sr-x"},
		{fs.ModeDir | fs.ModeSticky | 0o777, "drwxrwxrwt"},
		{fs.ModeDir | fs.ModeSticky | 0o776, "drwxrwxrwT"},
		{fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky | 0o777, "-rwsrwsrwt"},
		{fs.ModeIrregular | 0o644, "-rw-r--r--"},
	} {
		// A real ModTime, so mode 0 means "no permissions", not "unknown".
		if got := permString(Entry{Mode: tc.mode, ModTime: old}); got != tc.want {
			t.Errorf("permString(%v) = %q, want %q", tc.mode, got, tc.want)
		}
	}
	if got := permString(Entry{Name: "vanished"}); got != "??????????" {
		t.Errorf("permString of an entry whose Lstat failed = %q, want ??????????", got)
	}
}

func TestHumanSize(t *testing.T) {
	const K, M, G = 1 << 10, 1 << 20, 1 << 30
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0K"},
		{1023, "1.0K"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{10188, "9.9K"}, // 9.949K
		{10189, "10K"},  // 9.950K would print as "10.0K"
		{10239, "10K"},  // 9.999K likewise
		{10240, "10K"},
		{999 * K, "999K"},
		{1023487, "999K"}, // 999.499K
		{1023488, "1.0M"}, // 999.5K would round to "1000K"
		{1048575, "1.0M"},
		{1048576, "1.0M"},
		{15 * M, "15M"},
		{5 * G, "5.0G"},
		{math.MaxInt64, "8.0E"},
		{-1, "0"},
	} {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// Every size must fit the four-rune column, or every name after it shifts.
// The boundaries are where rounding pushes a value into a fifth rune.
func TestHumanSizeNeverExceedsFourRunes(t *testing.T) {
	var ns []int64
	for k := range 63 {
		for d := int64(-3); d <= 3; d++ {
			ns = append(ns, int64(1)<<k+d)
		}
	}
	for unit := 1.0; unit < 1<<61; unit *= 1024 {
		for _, v := range []float64{9.95, 99.5, 999.5, 1000, 1023.9} {
			if v*unit > math.MaxInt64/2 {
				continue // converting would overflow int64
			}
			c := int64(v * unit)
			for d := int64(-3); d <= 3; d++ {
				ns = append(ns, c+d)
			}
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 100000 {
		// Spread across every magnitude, not uniformly over int64, which
		// would put almost every sample in the exabytes.
		ns = append(ns, rng.Int64N(int64(1)<<rng.IntN(63)+1))
	}
	ns = append(ns, math.MaxInt64)
	for _, n := range ns {
		if s := humanSize(n); utf8.RuneCountInString(s) > 4 {
			t.Fatalf("humanSize(%d) = %q, longer than 4 runes", n, s)
		}
	}
}

func TestFormatDate(t *testing.T) {
	for _, tc := range []struct {
		name string
		t    time.Time
		want string
	}{
		{"an hour ago", now.Add(-time.Hour), "Sep 24 11:00"},
		{"exactly six months ago", now.Add(-sixMonths), "Mar 26 12:00"},
		{"just past six months", now.Add(-sixMonths - time.Hour), "Mar 26  2026"},
		{"years ago", time.Date(2020, time.February, 3, 4, 5, 0, 0, time.UTC), "Feb  3  2020"},
		{"within a minute ahead", now.Add(30 * time.Second), "Sep 24 12:00"},
		{"beyond a minute ahead", now.Add(2 * time.Minute), "Sep 24  2026"},
		{"next year", time.Date(2027, time.January, 5, 0, 0, 0, 0, time.UTC), "Jan  5  2027"},
		{"another zone", time.Date(2026, time.September, 24, 1, 30, 0, 0, time.FixedZone("X", 5*3600)), "Sep 23 20:30"},
		{"five-digit year", time.Date(12345, time.January, 1, 0, 0, 0, 0, time.UTC), "Jan  1 12345"},
		{"absurd year", time.Date(123456, time.January, 1, 0, 0, 0, 0, time.UTC), "Jan  1 ?????"},
		{"zero", time.Time{}, "            "},
	} {
		got := formatDate(tc.t, now)
		if got != tc.want {
			t.Errorf("%s: formatDate = %q, want %q", tc.name, got, tc.want)
		}
		if n := utf8.RuneCountInString(got); n != 12 {
			t.Errorf("%s: formatDate = %q, %d runes, want 12", tc.name, got, n)
		}
	}
}

// The clock's location, not the machine's, decides what a date reads as.
func TestFormatDateUsesTheClocksLocation(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	got := formatDate(time.Date(2026, time.September, 24, 0, 0, 0, 0, time.UTC), now.In(tokyo))
	if want := "Sep 24 09:00"; got != want {
		t.Errorf("formatDate = %q, want %q", got, want)
	}
}

// A newline in a file name is legal on Unix. Printed raw it would split one
// entry over two buffer lines and throw off EntryAt for everything below.
func TestNamesAreSanitised(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"a\nb\tc\r", "a?b?c?"},
		{"x\xffy", "x?y"},
		{"del\x7f", "del?"},
		{"c1\u0085", "c1?"},
		{"héllo wörld", "héllo wörld"},
		{"�", "�"}, // a real replacement character is valid text
	} {
		if got := sanitize(tc.in); got != tc.want {
			t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	e := Entry{Name: "evil\nname\t", Mode: fs.ModeSymlink | 0o777, Target: "t\nx", ModTime: recent}
	entries := []Entry{e, {Name: "next", Mode: 0o644, ModTime: recent}}
	l := Format(p("/tmp/new\nline"), entries, nil, Options{}, now, "")
	for i, line := range l.Lines {
		if strings.ContainsAny(line, "\n\t\r") {
			t.Errorf("line %d %q contains a control character", i, line)
		}
	}
	checkSpans(t, l)
	line, ok := l.LineOf("evil\nname\t")
	if !ok {
		t.Fatal("LineOf lost the entry with the unsanitised name")
	}
	if want := "evil?name? → t?x"; !strings.HasSuffix(l.Lines[line], want) {
		t.Errorf("line %q, want it to end %q", l.Lines[line], want)
	}
	if got, _ := l.EntryAt(line); got.Name != e.Name {
		t.Errorf("EntryAt(%d).Name = %q, want the raw name %q", line, got.Name, e.Name)
	}
}

func TestEveryLineHasValidSpans(t *testing.T) {
	entries := append(sample(),
		Entry{Name: "vanished"},
		Entry{Name: "fifo", Mode: fs.ModeNamedPipe | 0o644, ModTime: old},
		Entry{Name: "broken", Mode: fs.ModeSymlink | 0o777, Broken: true, Target: "nowhere", ModTime: recent},
		Entry{Name: "empty-target", Mode: fs.ModeSymlink | 0o777, ModTime: recent},
		Entry{Name: "日本語", Mode: 0o644, Size: 1 << 40, ModTime: old},
		Entry{Name: "big", Mode: 0o644, Size: math.MaxInt64, ModTime: old},
	)
	marks := map[string]rune{"fifo": 'D', "日本語": '*', "vanished": 'D'}
	for _, opts := range []Options{{}, {ShowHidden: true}, {HideDetails: true}, {ShowHidden: true, HideDetails: true, Sort: ByTime}} {
		checkSpans(t, Format(p("/x/y"), entries, marks, opts, now, p("/x")))
	}
}
