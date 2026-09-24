package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// What RET means at a completing prompt, driven through the real Loop.
//
// The rule is Vertico's: the highlighted candidate is the answer. Before it,
// RET at find-file and switch-to-buffer returned the typed text whenever the
// prompt did not require a match, so a highlighted file was never opened -
// "rea" at find-file made a new empty buffer called rea while readme.txt sat
// highlighted directly underneath it.

// stroke injects emacs key specs one at a time, pausing after each so the loop
// has taken it before the next arrives.
func stroke(t *testing.T, scr tcell.SimulationScreen, specs ...string) {
	t.Helper()
	for _, ev := range key(t, specs...) {
		scr.InjectKey(ev.key, ev.r, ev.mod)
		time.Sleep(40 * time.Millisecond)
	}
}

// prompted opens a prompt with prefix, types input into it and ends it with
// the keys in finish.
func prompted(t *testing.T, scr tcell.SimulationScreen, prefix []string, input string, finish ...string) {
	t.Helper()
	stroke(t, scr, prefix...)
	time.Sleep(40 * time.Millisecond)
	typeRunes(scr, input)
	time.Sleep(40 * time.Millisecond)
	stroke(t, scr, finish...)
}

var (
	findFileKeys = []string{"C-x", "C-f"}
	switchKeys   = []string{"C-x", "b"}
)

// writeFiles creates each name under dir with some text, making directories
// for any name containing a separator.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("contents of "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// bufferPaths lists the path of every buffer that has one.
func bufferPaths(e *Editor) []string {
	var out []string
	for _, b := range e.Buffers() {
		if p := b.Path(); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// The reported case: part of a name, the file highlighted, RET. The file opens,
// and nothing is created under the partial name - not a buffer that a later
// C-x C-s would turn into a stray file, and not the file itself.
func TestFindFileRetOpensTheHighlightedFile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "readme.txt")
	partial := filepath.Join(dir, "rea")

	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		prompted(t, scr, findFileKeys, partial, "RET")
	})

	want := filepath.Join(dir, "readme.txt")
	if got := e.Buf().Path(); got != want {
		t.Fatalf("visiting %q after RET on a highlighted readme.txt, want %q", got, want)
	}
	if got := e.Buf().String(); !strings.Contains(got, "contents of readme.txt") {
		t.Errorf("buffer = %q, want the file's text", got)
	}
	for _, p := range bufferPaths(e) {
		if p == partial {
			t.Errorf("a buffer was created for the partial name %q", partial)
		}
	}
	if _, err := os.Stat(partial); err == nil {
		t.Errorf("%s exists on disk; typing part of a name created it", partial)
	}
}

// The second reported case: part of a buffer name, RET, and the buffer shown
// rather than a new one invented under the fragment.
func TestSwitchToBufferRetVisitsTheHighlightedBuffer(t *testing.T) {
	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		// Somewhere else to be first. "other" matches no existing name, so
		// there is nothing to highlight and RET creates it.
		prompted(t, scr, switchKeys, "other", "RET")
		prompted(t, scr, switchKeys, "scr", "RET")
	})

	if got := e.BufferName(e.Buf()); got != "*scratch*" {
		t.Errorf("visiting %q after RET on a highlighted *scratch*, want *scratch*", got)
	}
	if _, ok := e.BufferByName("scr"); ok {
		t.Error("a buffer named scr was created from the typed fragment")
	}
	if _, ok := e.BufferByName("other"); !ok {
		t.Error("setup: RET with nothing highlighted did not create other")
	}
}

// A name typed in full is the answer even when fuzzy ranking prefers another.
// The scorer rewards a camelCase boundary, so fooBar outranks foobar for the
// input "foobar" - and RET must still visit foobar, the name that was typed.
//
// Buffers rather than files: foobar and fooBar cannot coexist on the
// case-insensitive filesystems macOS and Windows default to.
func TestRetPrefersAnExactMatchOverABetterRankedOne(t *testing.T) {
	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		prompted(t, scr, switchKeys, "fooBar", "M-RET")
		prompted(t, scr, switchKeys, "foobar", "M-RET")
		prompted(t, scr, switchKeys, "*scratch*", "RET")
		prompted(t, scr, switchKeys, "foobar", "RET")
	})

	if got := e.BufferName(e.Buf()); got != "foobar" {
		t.Errorf("visiting %q after typing foobar in full, want foobar", got)
	}
}

// The exact match is ranked first, not forced at RET, so moving off it is still
// honoured: what is highlighted when RET is pressed is what opens.
func TestRetTakesASelectionMovedOffTheExactMatch(t *testing.T) {
	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		prompted(t, scr, switchKeys, "fooBar", "M-RET")
		prompted(t, scr, switchKeys, "foobar", "M-RET")
		prompted(t, scr, switchKeys, "foobar", "C-n", "RET")
	})

	if got := e.BufferName(e.Buf()); got != "fooBar" {
		t.Errorf("visiting %q after C-n then RET, want the highlighted fooBar", got)
	}
}

// RET on a highlighted directory walks into it: the prompt stays open showing
// the directory's contents, so the next keystrokes pick a file inside it rather
// than trying to visit the directory itself.
//
// If the first RET closes the prompt instead, "no" and the second RET are typed
// into *scratch*, and the modified buffer makes C-x C-c ask before exiting. So
// a regression here fails as "the editor loop did not exit", not on the path.
func TestFindFileRetOnADirectoryDescends(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, filepath.Join("sub", "notes.txt"))

	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		prompted(t, scr, findFileKeys, filepath.Join(dir, "su"), "RET")
		// Still in the prompt, now listing sub/.
		time.Sleep(40 * time.Millisecond)
		typeRunes(scr, "no")
		time.Sleep(40 * time.Millisecond)
		stroke(t, scr, "RET")
	})

	want := filepath.Join(dir, "sub", "notes.txt")
	if got := e.Buf().Path(); got != want {
		t.Fatalf("visiting %q, want %q: RET on sub/ should have descended into it", got, want)
	}
	if got := e.Buf().String(); !strings.Contains(got, "contents of") {
		t.Errorf("buffer = %q, want the file's text", got)
	}
}

// M-RET is the escape hatch: exactly what was typed, whatever is highlighted.
// It is how a new file is created when its name fuzzy-matches an existing one,
// and it does not descend into a highlighted directory either.
func TestFindFileMetaRetBypassesTheHighlight(t *testing.T) {
	for _, tc := range []struct {
		name  string
		typed string
	}{
		{"file highlighted", "rea"},
		{"directory highlighted", "su"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, "readme.txt", filepath.Join("sub", "notes.txt"))
			typed := filepath.Join(dir, tc.typed)

			e := runKeys(t, "", func(scr tcell.SimulationScreen) {
				time.Sleep(60 * time.Millisecond)
				prompted(t, scr, findFileKeys, typed, "M-RET")
			})

			if got := e.Buf().Path(); got != typed {
				t.Errorf("visiting %q after M-RET, want the typed %q", got, typed)
			}
			if got := e.Buf().String(); got != "" {
				t.Errorf("buffer = %q, want a new empty one", got)
			}
		})
	}
}

// C-x b RET goes back to the buffer visited before this one, as in emacs, and a
// second C-x b RET returns. Candidates are offered most recent first with the
// current buffer last, so the previous buffer is the one highlighted.
//
// Sorted by name instead, RET from beta would pick *scratch*, which sorts
// before both - the regression this guards against.
func TestSwitchToBufferRetReturnsToThePreviousBuffer(t *testing.T) {
	for _, tc := range []struct {
		toggles int
		want    string
	}{
		{1, "alpha"},
		{2, "beta"},
		{3, "alpha"},
	} {
		e := runKeys(t, "", func(scr tcell.SimulationScreen) {
			time.Sleep(60 * time.Millisecond)
			prompted(t, scr, switchKeys, "alpha", "M-RET")
			prompted(t, scr, switchKeys, "beta", "M-RET")
			for range tc.toggles {
				stroke(t, scr, append(switchKeys, "RET")...)
			}
		})
		if got := e.BufferName(e.Buf()); got != tc.want {
			t.Errorf("after %d C-x b RET from beta, visiting %q, want %q", tc.toggles, got, tc.want)
		}
	}
}

// RET straight after walking into a directory opens its first entry in listing
// order. Before, every entry tied on the directory's own path and the shortest
// name won, which put .hidden ahead of notes.txt.
func TestFindFileRetAfterDescendingOpensTheFirstEntry(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, filepath.Join("sub", "notes.txt"), filepath.Join("sub", ".hidden"))

	e := runKeys(t, "", func(scr tcell.SimulationScreen) {
		time.Sleep(60 * time.Millisecond)
		prompted(t, scr, findFileKeys, filepath.Join(dir, "su"), "RET")
		time.Sleep(40 * time.Millisecond)
		stroke(t, scr, "RET")
	})

	want := filepath.Join(dir, "sub", "notes.txt")
	if got := e.Buf().Path(); got != want {
		t.Errorf("visiting %q, want %q", got, want)
	}
}
