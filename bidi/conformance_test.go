package bidi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The conformance tests run Unicode's own test data for the algorithm.
// testdata holds a few hundred cases picked from each file, which cover every
// rule; setting BIDI_UCD to a directory holding the whole of
// BidiCharacterTest.txt and BidiTest.txt, of the version unicodeVersion
// names, runs all of them instead - some 770,000 cases, and a second or two.

// ucdFile opens the named conformance file.
func ucdFile(t *testing.T, name string) *bufio.Scanner {
	t.Helper()
	dir := "testdata"
	if d := os.Getenv("BIDI_UCD"); d != "" {
		dir = d
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	return sc
}

// conform checks what Levels and Order make of rs against a test's expected
// levels, "x" for a character X9 removes, and visual order, which leaves
// those characters out.
func conform(rs []rune, dir Direction, levels, order []string) error {
	got := Levels(rs, dir)
	if len(got) != len(rs) || len(levels) != len(rs) {
		return fmt.Errorf("%d levels for %d runes, want %d", len(got), len(rs), len(levels))
	}
	for i, want := range levels {
		if want != "x" && want != strconv.Itoa(int(got[i])) {
			return fmt.Errorf("levels %v, want %v", got, levels)
		}
	}
	var visual []string
	for _, i := range Order(got) {
		if levels[i] != "x" {
			visual = append(visual, strconv.Itoa(i))
		}
	}
	if strings.Join(visual, " ") != strings.Join(order, " ") {
		return fmt.Errorf("order %v, want %v (levels %v)", visual, order, got)
	}
	return nil
}

// TestBidiCharacterTest runs BidiCharacterTest.txt: code points, a paragraph
// direction (2 for P2 and P3 to decide), the paragraph level that comes to,
// and the levels and visual order.
func TestBidiCharacterTest(t *testing.T) {
	sc := ucdFile(t, "BidiCharacterTest.txt")
	line, cases, failed := 0, 0, 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if text == "" || text[0] == '#' {
			continue
		}
		fs := strings.Split(text, ";")
		if len(fs) != 5 {
			t.Fatalf("line %d: %d fields", line, len(fs))
		}
		var rs []rune
		for _, h := range strings.Fields(fs[0]) {
			n, err := strconv.ParseUint(h, 16, 32)
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			rs = append(rs, rune(n))
		}
		dir := LTR
		switch fs[1] {
		case "1":
			dir = RTL
		case "2":
			dir = ParagraphDirection(rs)
		}
		cases++
		err := conform(rs, dir, strings.Fields(fs[3]), strings.Fields(fs[4]))
		if err == nil && strconv.Itoa(int(dir)) != fs[2] {
			err = fmt.Errorf("paragraph level %d, want %s", dir, fs[2])
		}
		if err != nil {
			if failed++; failed <= 20 {
				t.Errorf("line %d: %s: %v", line, fs[0], err)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Errorf("%d of %d cases failed", failed, cases)
	}
	t.Logf("%d cases", cases)
}

// representative is a character of each bidi class, for BidiTest.txt, which
// gives its cases as classes. None of them is a paired bracket, which the
// file assumes.
var representative = map[string]rune{
	"L": 'a', "R": 0x05D0, "AL": 0x0627, "EN": '1', "ES": '+', "ET": '$',
	"AN": 0x0660, "CS": ',', "NSM": 0x0300, "BN": 0x00AD, "B": 0x2029,
	"S": '\t', "WS": ' ', "ON": '!', "LRE": 0x202A, "RLE": 0x202B,
	"PDF": 0x202C, "LRO": 0x202D, "RLO": 0x202E, "LRI": 0x2066,
	"RLI": 0x2067, "FSI": 0x2068, "PDI": 0x2069,
}

// TestBidiTest runs BidiTest.txt: sequences of classes, under @Levels and
// @Reorder lines giving what the sequences after them resolve to, each with
// a set of the paragraph directions it is to be tried in: 1 for P2 and P3
// to decide, 2 for left to right, 4 for right to left.
func TestBidiTest(t *testing.T) {
	sc := ucdFile(t, "BidiTest.txt")
	var levels, order []string
	line, cases, failed := 0, 0, 0
	for sc.Scan() {
		line++
		text := sc.Text()
		switch {
		case strings.HasPrefix(text, "@Levels:"):
			levels = strings.Fields(strings.TrimPrefix(text, "@Levels:"))
			continue
		case strings.HasPrefix(text, "@Reorder:"):
			order = strings.Fields(strings.TrimPrefix(text, "@Reorder:"))
			continue
		case strings.TrimSpace(text) == "", text[0] == '#', text[0] == '@':
			continue
		}
		input, set, ok := strings.Cut(text, ";")
		if !ok {
			t.Fatalf("line %d: no paragraph directions", line)
		}
		var rs []rune
		for _, c := range strings.Fields(input) {
			r, ok := representative[c]
			if !ok {
				t.Fatalf("line %d: unknown class %s", line, c)
			}
			rs = append(rs, r)
		}
		bits, err := strconv.Atoi(strings.TrimSpace(set))
		if err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		for _, d := range []struct {
			bit  int
			name string
		}{{1, "auto"}, {2, "LTR"}, {4, "RTL"}} {
			if bits&d.bit == 0 {
				continue
			}
			dir := LTR
			switch d.bit {
			case 1:
				dir = ParagraphDirection(rs)
			case 4:
				dir = RTL
			}
			cases++
			if err := conform(rs, dir, levels, order); err != nil {
				if failed++; failed <= 20 {
					t.Errorf("line %d: %s (%s): %v", line, input, d.name, err)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Errorf("%d of %d cases failed", failed, cases)
	}
	t.Logf("%d cases", cases)
}
