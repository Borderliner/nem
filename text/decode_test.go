package text

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// referenceDecode is how LoadFile split a file before decodeLines: convert,
// split on newlines, convert each part.
func referenceDecode(data []byte) (lines [][]rune, crlf, finalNL bool) {
	content := string(data)
	if i := strings.IndexByte(content, '\n'); i > 0 && content[i-1] == '\r' {
		crlf = true
	}
	if crlf {
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}
	if content == "" {
		return [][]rune{{}}, crlf, false
	}
	parts := strings.Split(content, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
		finalNL = true
	}
	for _, p := range parts {
		lines = append(lines, []rune(p))
	}
	return lines, crlf, finalNL
}

// decodeLines must split every file the way the conversion it replaced did:
// line endings of either kind and mixed, lone carriage returns, invalid and
// truncated UTF-8, empty files and files that are only newlines.
func TestDecodeLinesMatchesTheConversion(t *testing.T) {
	pieces := []string{"a", "xyz", "\n", "\r", "\r\n", "é", "漢", "👍", "\xff", "\xe2\x82", "\x80", "\t", ""}
	rng := rand.New(rand.NewPCG(5, 6))
	for trial := range 30000 {
		var b strings.Builder
		for range rng.IntN(10) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		data := []byte(b.String())

		got, crlf, finalNL := decodeLines(data)
		want, wantCRLF, wantFinal := referenceDecode(data)
		if crlf != wantCRLF || finalNL != wantFinal || len(got) != len(want) {
			t.Fatalf("trial %d, %q: crlf %v final %v %d lines; want %v %v %d",
				trial, data, crlf, finalNL, len(got), wantCRLF, wantFinal, len(want))
		}
		for i := range got {
			if !slices.Equal(got[i].runes, want[i]) {
				t.Fatalf("trial %d, %q: line %d = %q, want %q", trial, data, i, string(got[i].runes), string(want[i]))
			}
		}
	}
}

// Lines share one array, so growing one must not write into the next.
func TestDecodedLinesDoNotShareSpace(t *testing.T) {
	lines, _, _ := decodeLines([]byte("ab\ncd\nef"))
	b := &Buffer{lines: lines, undo: newUndoLog(), dirtyFrom: noDirtyLine}
	if err := b.Insert(Pos{0, 2}, []rune("XYZ")); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "abXYZ\ncd\nef" {
		t.Errorf("after growing line 0: %q", got)
	}
}
