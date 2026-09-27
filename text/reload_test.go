package text

import (
	"os"
	"path/filepath"
	"testing"
)

// Revert brings the buffer back to the file as it is now, unmodified, and
// the revert itself can be undone.
func TestRevert(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(Pos{}, []rune("edit ")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	if err := os.WriteFile(p, []byte("changed\r\nelsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := b.Revert(); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "changed\nelsewhere" || b.Modified() {
		t.Errorf("after Revert: %q, modified %v", got, b.Modified())
	}
	if got := string(b.encode("\r\n", b.finalNL)); got != "changed\r\nelsewhere" {
		t.Errorf("the file's CRLF endings and lack of a final newline were not taken up: %q", got)
	}
	if _, ok := b.Undo(); !ok || b.String() != "edit one\ntwo" {
		t.Errorf("undoing the revert gave %q", b.String())
	}

	// A file that already matches leaves nothing to undo.
	c, _ := LoadFile(p)
	if err := c.Revert(); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Undo(); ok {
		t.Error("reverting an unchanged buffer left an undo step")
	}
}

func TestRevertNeedsAFile(t *testing.T) {
	if err := NewBuffer().Revert(); err != ErrNoPath {
		t.Errorf("Revert without a path = %v, want ErrNoPath", err)
	}
}
