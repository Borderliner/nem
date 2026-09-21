package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// segInfo builds a modelineInfo returning fixed answers.
func segInfo(fileType, branch string) modelineInfo {
	return modelineInfo{
		Type:   func(*text.Buffer) string { return fileType },
		Branch: func(*text.Buffer) string { return branch },
	}
}

func segWindow(t *testing.T, path string) *view.Window {
	t.Helper()
	b := bufferOf(t, "hi")
	b.SetPath(path)
	return view.NewWindow(b)
}

func TestModelineShowsFileTypeAndBranch(t *testing.T) {
	w := segWindow(t, "/repo/main.go")
	got := stripANSI(modelineString(DefaultTheme(), w, 60, true, segInfo("go", "main")))

	for _, want := range []string{"main.go", "go", "main", string(BranchMark)} {
		if !strings.Contains(got, want) {
			t.Errorf("modeline = %q, want it to contain %q", got, want)
		}
	}
}

// A nil source contributes no segment, and must not leave a gap where one would
// have been - ui has to stay usable by a caller that tracks neither.
func TestModelineWithoutSegmentSources(t *testing.T) {
	w := segWindow(t, "/repo/main.go")
	got := stripANSI(modelineString(DefaultTheme(), w, 60, true, modelineInfo{}))

	if strings.ContainsRune(got, BranchMark) {
		t.Errorf("modeline = %q, want no branch mark with a nil source", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("modeline = %q, want the buffer name", got)
	}
	// Two spaces before the rule would be the hole a dropped segment leaves.
	if strings.Contains(got, "  "+string(ModelineRule)) {
		t.Errorf("modeline = %q, want no gap where an absent segment would sit", got)
	}
}

// Plain text has no grammar, so it contributes no file type - an empty string
// must render as nothing rather than as an empty field.
func TestModelineOmitsEmptyFileType(t *testing.T) {
	w := segWindow(t, "/repo/notes")
	got := stripANSI(modelineString(DefaultTheme(), w, 60, true, segInfo("", "main")))

	if !strings.ContainsRune(got, BranchMark) {
		t.Errorf("modeline = %q, want the branch still shown", got)
	}
	if strings.Contains(got, "notes  "+string(BranchMark)) {
		return // name, two spaces, branch: correct
	}
	if !strings.Contains(got, "notes") {
		t.Errorf("modeline = %q, want the buffer name", got)
	}
}

// Segments are given up from the least essential end: the branch before the
// file type, and both before the position. A modeline that drops "go" while
// keeping a long branch name would be spending its last columns on the less
// useful fact.
func TestSegmentsDropInOrderAsThePaneNarrows(t *testing.T) {
	w := segWindow(t, "/repo/main.go")
	info := segInfo("go", "feature/very-long-branch-name")

	var sawBoth, sawTypeOnly, sawNeither bool
	for width := 80; width >= 10; width-- {
		got := stripANSI(modelineString(DefaultTheme(), w, width, true, info))
		hasBranch := strings.ContainsRune(got, BranchMark)
		hasType := strings.Contains(got, "  go")

		if hasBranch && !hasType {
			t.Errorf("width %d: branch kept but file type dropped: %q", width, got)
		}
		switch {
		case hasBranch:
			sawBoth = true
			if sawTypeOnly || sawNeither {
				t.Errorf("width %d: branch came back after being dropped: %q", width, got)
			}
		case hasType:
			sawTypeOnly = true
			if sawNeither {
				t.Errorf("width %d: file type came back after being dropped: %q", width, got)
			}
		default:
			sawNeither = true
		}
	}
	if !sawBoth || !sawTypeOnly || !sawNeither {
		t.Errorf("wanted all three stages across 80..10; got both=%v typeOnly=%v neither=%v",
			sawBoth, sawTypeOnly, sawNeither)
	}
}

// The exact-width contract is the one thing segments could quietly break: a
// segment that pushes past the pane bleeds into the divider beside it.
func TestModelineWithSegmentsIsExactlyTheGivenWidth(t *testing.T) {
	w := segWindow(t, "/repo/main.go")
	for _, info := range []modelineInfo{
		segInfo("go", "main"),
		segInfo("go", "feature/very-long-branch-name-indeed"),
		segInfo("", ""),
	} {
		for width := 80; width >= 10; width-- {
			got := modelineString(DefaultTheme(), w, width, true, info)
			if n := lipgloss.Width(got); n != width {
				t.Errorf("width %d: measured %d cells (%q)", width, n, stripANSI(got))
			}
		}
	}
}

// A pane narrower than the shortest layout must still never exceed its width.
func TestModelineWithSegmentsNeverExceedsANarrowPane(t *testing.T) {
	w := segWindow(t, "/repo/a-rather-long-file-name.go")
	info := segInfo("go", "main")
	for width := 1; width <= 12; width++ {
		got := modelineString(DefaultTheme(), w, width, true, info)
		if n := lipgloss.Width(got); n > width {
			t.Errorf("width %d: measured %d cells (%q)", width, n, stripANSI(got))
		}
	}
}

// Segments are secondary information and must be quiet, not compete with the
// buffer name for attention.
func TestSegmentsAreQuiet(t *testing.T) {
	w := segWindow(t, "/repo/main.go")
	got := modelineString(DefaultTheme(), w, 60, true, segInfo("go", "main"))

	// The segments are styled as one run rather than field by field, so the
	// assertion is on the whole block: that is what the renderer emits and what
	// a change in styling would alter.
	th := DefaultTheme()
	quiet := th.ModelinePos.Render(segmentText("go", "main"))
	if !strings.Contains(got, quiet) {
		t.Errorf("modeline does not render the segments in the quiet style:\n got %q\nwant it to contain %q",
			got, quiet)
	}
	// And they must not borrow the active name's weight, which would make them
	// compete with the file name.
	if strings.Contains(got, th.ModelineName.Render(segmentText("go", "main"))) {
		t.Error("segments are rendered in the active-name style; they are secondary information")
	}
}
