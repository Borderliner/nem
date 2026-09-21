package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

func TestModelineNamesScratchBufferWhenPathless(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hi"))
	got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})
	if !strings.Contains(got, ScratchName) {
		t.Errorf("modeline = %q, want it to contain %q", got, ScratchName)
	}
}

func TestModelineShowsBaseNameNotFullPath(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/home/reza/Lab/Projects/nem/text/buffer.go")
	w := view.NewWindow(b)

	got := modelineString(DefaultTheme(), w, 60, true, modelineInfo{})
	if !strings.Contains(got, "buffer.go") {
		t.Errorf("modeline = %q, want the base name", got)
	}
	if strings.Contains(got, "/home/reza") {
		t.Errorf("modeline = %q, want no directory component", got)
	}
}

func TestModelineModifiedMark(t *testing.T) {
	b := bufferOf(t, "hi")
	w := view.NewWindow(b)

	clean := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})
	if strings.ContainsRune(clean, ModifiedMark) {
		t.Errorf("clean modeline = %q, want no modified mark", clean)
	}

	if err := b.Insert(b.End(), []rune("!")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	dirty := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})
	if !strings.ContainsRune(dirty, ModifiedMark) {
		t.Errorf("modified modeline = %q, want the modified mark", dirty)
	}
}

// The mark occupies a cell whether or not it is showing, so the buffer name
// must sit at the same screen column either way. Without this the name jumps
// sideways the instant you type the first character, which is exactly the kind
// of jitter that makes an interface feel cheap.
func TestModifiedMarkDoesNotShiftTheName(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/tmp/steady.go")
	w := view.NewWindow(b)

	clean := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})
	if err := b.Insert(b.End(), []rune("!")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	dirty := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})

	// Measure the DISPLAY column, not the byte offset: the mark is three bytes
	// of UTF-8 where a space is one, so strings.Index would report a shift that
	// the user never sees.
	col := func(s string) int {
		plain := stripANSI(s)
		i := strings.Index(plain, "steady.go")
		if i < 0 {
			t.Fatalf("name missing from modeline %q", plain)
		}
		return lipgloss.Width(plain[:i])
	}
	if got, want := col(dirty), col(clean); got != want {
		t.Errorf("name starts at column %d when modified and %d when clean; it must not move", got, want)
	}
	if lipgloss.Width(clean) != lipgloss.Width(dirty) {
		t.Errorf("modeline width changed with the modified state: %d vs %d",
			lipgloss.Width(clean), lipgloss.Width(dirty))
	}
}

// Nothing in the chrome may paint a background. A background colour is what
// makes a terminal program look like it is squatting in your terminal instead
// of living in it, and it is the one thing that would clash with whichever
// palette the user has already chosen.
func TestModelinePaintsNoBackground(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/tmp/plain.go")
	if err := b.Insert(b.End(), []rune("!")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	w := view.NewWindow(b)

	for _, active := range []bool{true, false} {
		got := modelineString(DefaultTheme(), w, 40, active, modelineInfo{})
		// SGR 48 is "set background"; 4x in the 40-47 range is a basic one.
		if strings.Contains(got, "\x1b[48") || strings.Contains(got, ";48;") {
			t.Errorf("active=%v modeline sets a background colour: %q", active, got)
		}
	}
}

func TestModelineShowsLineAndColumn(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hello", "world"))
	w.Pt = text.Pos{Line: 1, Col: 3}

	// Line is 1-based and column 0-based, as emacs reports them.
	if got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{}); !strings.Contains(got, "2:3") {
		t.Errorf("modeline = %q, want 2:3", got)
	}
}

func TestModelineColumnIsDisplayColumn(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "日本語"))
	w.Pt = text.Pos{Line: 0, Col: 2} // two wide glyphs: display column 4

	if got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{}); !strings.Contains(got, "1:4") {
		t.Errorf("modeline = %q, want 1:4 (display column, not rune index)", got)
	}
}

func TestModelineFillsExactlyTheGivenWidth(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hi"))
	for _, width := range []int{10, 20, 40, 80} {
		got := modelineString(DefaultTheme(), w, width, true, modelineInfo{})
		if n := lipgloss.Width(got); n != width {
			t.Errorf("width %d: modeline measured %d cells, want exactly %d (%q)", width, n, width, got)
		}
	}
}

func TestModelineNeverExceedsANarrowWindow(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/tmp/a-rather-long-file-name-that-will-not-fit.go")
	w := view.NewWindow(b)

	for _, width := range []int{1, 2, 4, 8, 12} {
		got := modelineString(DefaultTheme(), w, width, true, modelineInfo{})
		if n := lipgloss.Width(got); n > width {
			t.Errorf("width %d: modeline measured %d cells, want at most %d (%q)", width, n, width, got)
		}
	}
}

func TestModelineDistinguishesActiveFromInactive(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hi"))
	active := modelineString(DefaultTheme(), w, 40, true, modelineInfo{})
	inactive := modelineString(DefaultTheme(), w, 40, false, modelineInfo{})

	if active == inactive {
		t.Error("active and inactive modelines render identically; the user cannot tell which window has focus")
	}
}

// The caller names its own buffers. Without this the renderer can only guess
// from the path, and every path-less buffer reads *scratch*.
func TestModelineUsesNameOf(t *testing.T) {
	b := bufferOf(t, "hi")
	w := view.NewWindow(b)
	named := func(*text.Buffer) string { return "*Buffer List*" }

	got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{Name: named})
	if !strings.Contains(got, "*Buffer List*") {
		t.Errorf("modeline = %q, want it to use the supplied name", got)
	}
	if strings.Contains(got, ScratchName) {
		t.Errorf("modeline = %q, still falls back to %q", got, ScratchName)
	}
}

// A supplied name wins over the file name, so a buffer renamed in the editor's
// map is reported as the editor sees it.
func TestNameOfWinsOverThePath(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/tmp/on-disk.go")
	w := view.NewWindow(b)

	got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{Name: func(*text.Buffer) string { return "renamed" }})
	if !strings.Contains(got, "renamed") {
		t.Errorf("modeline = %q, want the supplied name to win", got)
	}
}

// ui is a library: a caller that tracks no names must still get a sensible
// modeline rather than a blank one.
func TestModelineFallsBackWithoutNameOf(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/tmp/thing.go")
	w := view.NewWindow(b)

	if got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{}); !strings.Contains(got, "thing.go") {
		t.Errorf("modeline = %q, want the file name when NameOf is nil", got)
	}
	// And a NameFunc that knows nothing about this buffer falls back too, which
	// is what a map miss returns.
	empty := func(*text.Buffer) string { return "" }
	if got := modelineString(DefaultTheme(), w, 40, true, modelineInfo{Name: empty}); !strings.Contains(got, "thing.go") {
		t.Errorf("modeline = %q, want the file name when NameOf returns empty", got)
	}
}
