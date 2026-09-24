package command_test

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

func lastEcho(t *testing.T, echoes []string) string {
	t.Helper()
	if len(echoes) == 0 {
		t.Fatal("nothing was echoed")
	}
	return echoes[len(echoes)-1]
}

func TestCountWords(t *testing.T) {
	// A final newline ends the last line rather than starting another, so
	// this is two lines, as emacs counts it.
	f := fileEnv("a.txt", "one two", "three, four five", "")
	runAll(t, f, "count-words")
	if got, want := lastEcho(t, f.Echoes), "Buffer has 2 lines, 5 words, and 25 characters."; got != want {
		t.Errorf("buffer: %q, want %q", got, want)
	}

	// A region ending at the start of a line does not count that line.
	selectLines(f, 0, 1)
	runAll(t, f, "count-words")
	if got, want := lastEcho(t, f.Echoes), "Region has 1 line, 2 words, and 8 characters."; got != want {
		t.Errorf("region: %q, want %q", got, want)
	}
}

func TestWhatCursorPosition(t *testing.T) {
	f := fileEnv("a.txt", "ab", "c d")
	f.SetPoint(text.Pos{Line: 1, Col: 1})
	runAll(t, f, "what-cursor-position")
	if got, want := lastEcho(t, f.Echoes), "Char: SPC (32, #o40, #x20) point=5 of 7 (66%) column=1"; got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	f.SetPoint(text.Pos{Line: 1, Col: 3})
	runAll(t, f, "what-cursor-position")
	if got, want := lastEcho(t, f.Echoes), "point=7 of 7 (EOB) column=3"; got != want {
		t.Errorf("at the end: %q, want %q", got, want)
	}
}
