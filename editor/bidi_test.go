package editor

import (
	"os"
	"path/filepath"
	"testing"
)

// The arrows go the way a line reads: in a line of Persian in prose, <right>
// goes back and <left> forward; in code, and in a line of English, as ever.
func TestArrowsFollowTheLine(t *testing.T) {
	e, _ := newTestEditor(t, "سلام", "hello")
	e.th.Bidi = true
	e.active.Pt.Col = 2
	press(t, e, "<right>")
	wantPt(t, e, 0, 1)
	press(t, e, "<left>", "<left>")
	wantPt(t, e, 0, 3)

	press(t, e, "C-n", "C-a", "<right>")
	wantPt(t, e, 1, 1)

	p := filepath.Join(t.TempDir(), "a.go")
	if err := os.WriteFile(p, []byte("// سلام\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.visitFile(p); err != nil {
		t.Fatal(err)
	}
	e.active.Pt.Col = 4
	press(t, e, "<right>")
	wantPt(t, e, 0, 5)
}
