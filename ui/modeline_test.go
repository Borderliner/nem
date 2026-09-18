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
	got := modelineString(DefaultTheme(), w, 40, true)
	if !strings.Contains(got, ScratchName) {
		t.Errorf("modeline = %q, want it to contain %q", got, ScratchName)
	}
}

func TestModelineShowsBaseNameNotFullPath(t *testing.T) {
	b := bufferOf(t, "hi")
	b.SetPath("/home/reza/Lab/Projects/nem/text/buffer.go")
	w := view.NewWindow(b)

	got := modelineString(DefaultTheme(), w, 60, true)
	if !strings.Contains(got, "buffer.go") {
		t.Errorf("modeline = %q, want the base name", got)
	}
	if strings.Contains(got, "/home/reza") {
		t.Errorf("modeline = %q, want no directory component", got)
	}
}

func TestModelineModifiedFlag(t *testing.T) {
	b := bufferOf(t, "hi")
	w := view.NewWindow(b)

	clean := modelineString(DefaultTheme(), w, 40, true)
	if !strings.Contains(clean, "--") {
		t.Errorf("clean modeline = %q, want the -- flag", clean)
	}

	if err := b.Insert(b.End(), []rune("!")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	dirty := modelineString(DefaultTheme(), w, 40, true)
	if !strings.Contains(dirty, "**") {
		t.Errorf("modified modeline = %q, want the ** flag", dirty)
	}
}

func TestModelineShowsLineAndColumn(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hello", "world"))
	w.Pt = text.Pos{Line: 1, Col: 3}

	// Line is 1-based and column 0-based, as emacs reports them.
	if got := modelineString(DefaultTheme(), w, 40, true); !strings.Contains(got, "2:3") {
		t.Errorf("modeline = %q, want 2:3", got)
	}
}

func TestModelineColumnIsDisplayColumn(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "日本語"))
	w.Pt = text.Pos{Line: 0, Col: 2} // two wide glyphs: display column 4

	if got := modelineString(DefaultTheme(), w, 40, true); !strings.Contains(got, "1:4") {
		t.Errorf("modeline = %q, want 1:4 (display column, not rune index)", got)
	}
}

func TestModelineFillsExactlyTheGivenWidth(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hi"))
	for _, width := range []int{10, 20, 40, 80} {
		got := modelineString(DefaultTheme(), w, width, true)
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
		got := modelineString(DefaultTheme(), w, width, true)
		if n := lipgloss.Width(got); n > width {
			t.Errorf("width %d: modeline measured %d cells, want at most %d (%q)", width, n, width, got)
		}
	}
}

func TestModelineDistinguishesActiveFromInactive(t *testing.T) {
	w := view.NewWindow(bufferOf(t, "hi"))
	active := modelineString(DefaultTheme(), w, 40, true)
	inactive := modelineString(DefaultTheme(), w, 40, false)

	if active == inactive {
		t.Error("active and inactive modelines render identically; the user cannot tell which window has focus")
	}
}
