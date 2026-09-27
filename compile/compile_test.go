package compile

import "testing"

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		line       string
		file       string
		ln, col    int
		kind       Kind
		start, end int
	}{
		{"main.go:12:5: undefined: x", "main.go", 12, 5, Error, 0, 12},
		{"./pkg/a.go:3:1: warning: unused", "./pkg/a.go", 3, 1, Warning, 0, 14},
		{"    project_test.go:45: got 1, want 2", "project_test.go", 45, 0, Error, 4, 22},
		{"\t/home/r/x/grep.go:352 +0x1d", "/home/r/x/grep.go", 352, 0, Error, 1, 22},
		{"src/lib.c:7:3: note: declared here", "src/lib.c", 7, 3, Info, 0, 13},
		{"  --> src/main.rs:4:5", "src/main.rs", 4, 5, Error, 6, 21},
		{`  File "app/main.py", line 12, in run`, "app/main.py", 12, 0, Error, 8, 29},
		{"    at Object.<anonymous> (/app/x.js:10:5)", "/app/x.js", 10, 5, Error, 27, 41},
		{"    at async load (/app/y.js:3:9)", "/app/y.js", 3, 9, Error, 19, 32},
		{"    at /app/z.js:1:2", "/app/z.js", 1, 2, Error, 7, 20},
		{"src/a.ts(12,5): error TS2322: nope", "src/a.ts", 12, 5, Error, 0, 13},
		{`C:\src\m.c:9:2: error: x`, `C:\src\m.c`, 9, 2, Error, 0, 14},
	} {
		loc, ok := Parse(tc.line)
		if !ok {
			t.Errorf("Parse(%q) found no location", tc.line)
			continue
		}
		if loc.File != tc.file || loc.Line != tc.ln || loc.Col != tc.col || loc.Kind != tc.kind ||
			loc.Start != tc.start || loc.End != tc.end {
			t.Errorf("Parse(%q) = %+v, want %s:%d:%d kind %d at %d-%d", tc.line, loc, tc.file, tc.ln, tc.col, tc.kind, tc.start, tc.end)
		}
	}
}

// Lines that only look like locations - times, URLs, prose - are not.
func TestParseLeavesOtherLinesAlone(t *testing.T) {
	for _, line := range []string{
		"ok  \tgithub.com/x/y\t0.012s",
		"2026/09/27 10:00:03 listening",
		"10:42:07 build started",
		"http://example.com:8080/path",
		"--- FAIL: TestThing (0.00s)",
		"Error: something went wrong",
		"",
	} {
		if loc, ok := Parse(line); ok {
			t.Errorf("Parse(%q) = %+v, want no location", line, loc)
		}
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[31merror\x1b[0m: bad":            "error: bad",
		"\x1b]8;;http://x\x07link\x1b]8;;\x07": "link",
		"progress 10%\rprogress 50%\rdone":     "done",
		"line with crlf\r":                     "line with crlf",
		"bell\x07 and\ttab":                    "bell and\ttab",
		"plain":                                "plain",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}
