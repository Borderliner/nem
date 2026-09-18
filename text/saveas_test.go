package text

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A failed SaveAs must not leave the buffer claiming the new path. Otherwise a
// later Save writes to a file the user never asked for, and they have no way to
// know it happened.
func TestFailedSaveAsDoesNotAdoptThePath(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("contents")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	orig := filepath.Join(t.TempDir(), "original.txt")
	b.SetPath(orig)

	bad := filepath.Join(t.TempDir(), "no-such-dir", "target.txt")
	if err := b.SaveAs(bad); err == nil {
		t.Fatal("SaveAs to an unwritable path succeeded, want an error")
	}
	if got := b.Path(); got != orig {
		t.Errorf("Path() = %q after a failed SaveAs, want the original %q", got, orig)
	}
	if !b.Modified() {
		t.Error("Modified() = false after a failed SaveAs; the user would believe this was written")
	}
	if _, err := os.Stat(bad); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file appeared at %q despite the failure", bad)
	}
}

func TestSaveAsAdoptsThePathOnSuccess(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("contents")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := b.SaveAs(target); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	if got := b.Path(); got != target {
		t.Errorf("Path() = %q, want %q", got, target)
	}
	if b.Modified() {
		t.Error("Modified() = true after a successful SaveAs")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "contents\n" && string(got) != "contents" {
		t.Errorf("file holds %q, want the buffer contents", got)
	}
}

func TestSaveAsWithEmptyPathErrors(t *testing.T) {
	if err := NewBuffer().SaveAs(""); !errors.Is(err, ErrNoPath) {
		t.Errorf("SaveAs(\"\") = %v, want ErrNoPath", err)
	}
}
