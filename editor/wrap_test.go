package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Line wrap is off unless init.lua turns it on, and C-x x t turns it on and
// off while nem runs.
func TestLineWrapIsASettingAndAToggle(t *testing.T) {
	e, _ := newTestEditor(t)
	if e.th.Wrap || e.WrapWidth() != 0 {
		t.Fatal("lines are wrapped by default")
	}
	cfg := filepath.Join(t.TempDir(), "init.lua")
	if err := os.WriteFile(cfg, []byte(`nem.set("line-wrap", true)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.LoadConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if !e.th.Wrap {
		t.Fatal(`nem.set("line-wrap", true) did not wrap lines`)
	}
	// An 80-column screen, less a gutter of two.
	if got := e.WrapWidth(); got != 78 {
		t.Errorf("wrapping at %d columns, want 78", got)
	}
	press(t, e, "C-x", "x", "t")
	wantEcho(t, e, "Line wrap off")
	if e.th.Wrap {
		t.Error("C-x x t left lines wrapped")
	}
	press(t, e, "C-x", "x", "t")
	wantEcho(t, e, "Line wrap on")
}

// Wrapped, C-n goes down a row of a long line rather than past it, and the
// cursor is drawn on that row.
func TestCnWalksAWrappedLine(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("words and more ", 12)) // 179 columns
	e, scr := newTestEditor(t, long, "next")
	e.SetLineWrap(true)
	e.Redraw()
	press(t, e, "C-n")
	if pt := e.active.Pt; pt.Line != 0 || pt.Col == 0 {
		t.Fatalf("C-n went to %v; want the long line's second row", pt)
	}
	e.Redraw()
	if _, y, _ := scr.GetCursor(); y != 1 {
		t.Errorf("the cursor is on screen row %d, want 1", y)
	}
	press(t, e, "C-n", "C-n")
	if pt := e.active.Pt; pt.Line != 1 {
		t.Errorf("three rows down is %v; want the next line, after the long one's three rows", pt)
	}
}
