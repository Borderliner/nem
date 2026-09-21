package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// screenRow reads row y of the simulation screen as a string.
func screenRow(t *testing.T, scr tcell.SimulationScreen, y int) string {
	t.Helper()
	cells, w, h := scr.GetContents()
	if y < 0 || y >= h {
		t.Fatalf("row %d out of bounds (height %d)", y, h)
	}
	var b strings.Builder
	for x := 0; x < w; x++ {
		rs := cells[y*w+x].Runes
		if len(rs) == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteString(string(rs))
	}
	return b.String()
}

// modelineRow renders a frame and returns the active window's modeline. With a
// single window the tree owns every row but the echo line, so the modeline is
// the row above it.
func modelineRow(t *testing.T, e *Editor, scr tcell.SimulationScreen) string {
	t.Helper()
	e.Redraw()
	_, h := scr.Size()
	return screenRow(t, scr, h-2)
}

// Every buffer must be named by the modeline, not just the file-backed ones.
//
// Buffer names live in the editor, which owns the name map; text.Buffer
// deliberately carries only a path. The renderer had no way to ask, so it
// derived a name from the path and called everything path-less *scratch* -
// meaning C-x C-b rendered its listing correctly while the modeline insisted
// you were still looking at *scratch*. Each half was right on its own, which is
// why no unit test caught it.
func TestModelineNamesEachBuffer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		want  string
		avoid string
	}{
		{name: "scratch", keys: nil, want: "*scratch*"},
		{name: "buffer list", keys: []string{"C-x", "C-b"}, want: "*Buffer List*", avoid: "*scratch*"},
		{name: "bindings", keys: []string{"<f1>", "b"}, want: "*Bindings*", avoid: "*scratch*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, scr := newTestEditor(t)
			press(t, e, tc.keys...)

			got := modelineRow(t, e, scr)
			if !strings.Contains(got, tc.want) {
				t.Errorf("modeline = %q, want it to name %q", strings.TrimRight(got, " "), tc.want)
			}
			if tc.avoid != "" && strings.Contains(got, tc.avoid) {
				t.Errorf("modeline = %q, still claims to be %q", strings.TrimRight(got, " "), tc.avoid)
			}
		})
	}
}

// write-file renames a buffer in the editor's map, and the modeline must follow.
// The name is now supplied by the editor rather than derived from the path, so a
// stale map would keep showing the old name even though the file moved.
func TestModelineFollowsARename(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	before := modelineRow(t, e, scr)
	if !strings.Contains(before, "*scratch*") {
		t.Fatalf("setup: modeline = %q, want the scratch buffer", strings.TrimRight(before, " "))
	}

	target := filepath.Join(t.TempDir(), "renamed.go")
	if err := e.SaveBuffer(e.Buf(), target); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	after := modelineRow(t, e, scr)
	if !strings.Contains(after, "renamed.go") {
		t.Errorf("modeline = %q, want it to follow the rename to renamed.go", strings.TrimRight(after, " "))
	}
	if strings.Contains(after, "*scratch*") {
		t.Errorf("modeline = %q, still shows the old name", strings.TrimRight(after, " "))
	}
}

// Two windows onto different buffers must each name their own, which is what
// makes a split useful when one pane holds a listing.
func TestSplitWindowsNameTheirOwnBuffers(t *testing.T) {
	e, scr := newTestEditor(t, "alpha")
	press(t, e, "C-x", "2")   // split
	press(t, e, "C-x", "C-b") // the active pane shows the buffer list

	e.Redraw()
	cells, w, h := scr.GetContents()
	rows := make([]string, 0, h)
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; x++ {
			rs := cells[y*w+x].Runes
			if len(rs) == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteString(string(rs))
		}
		rows = append(rows, b.String())
	}
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"*Buffer List*", "*scratch*"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no modeline names %q; each pane must name its own buffer", want)
		}
	}
}
