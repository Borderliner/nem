//go:build linux || darwin || freebsd

package dired

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Borderliner/nem/syntax"
)

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("cannot create a named pipe here: %v", err)
	}
}

// Opening a pipe to copy it would block until something writes to it, so
// Copy must refuse it outright, and inside a tree as much as on its own.
func TestCopyRefusesSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	mkfifo(t, fifo)

	if err := Copy(fifo, filepath.Join(dir, "copy")); !errors.Is(err, errSpecial) {
		t.Errorf("Copy(fifo) = %v, want the special-file refusal", err)
	}
	if exists(filepath.Join(dir, "copy")) {
		t.Error("a refused copy created its destination")
	}

	tree := filepath.Join(dir, "tree")
	mkdir(t, tree)
	mkfifo(t, filepath.Join(tree, "fifo"))
	if err := Copy(tree, filepath.Join(dir, "tree2")); !errors.Is(err, errSpecial) {
		t.Errorf("Copy(tree holding a fifo) = %v, want the special-file refusal", err)
	}
}

func TestReadAndFormatANamedPipe(t *testing.T) {
	dir := t.TempDir()
	mkfifo(t, filepath.Join(dir, "fifo"))
	entries, err := Read(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Read = %v, %v; want the one pipe", entries, err)
	}
	e := entries[0]
	if e.Mode&fs.ModeNamedPipe == 0 {
		t.Fatalf("mode %v, want a named pipe", e.Mode)
	}
	line, spans := FormatEntry(e, ' ', Options{}, now)
	if got := []rune(line)[permColumn]; got != 'p' {
		t.Errorf("line %q: type %q, want 'p'", line, got)
	}
	if last := spans[len(spans)-1]; last.Start != NameColumn(Options{}) || last.Class != syntax.Constant {
		t.Errorf("name span %+v, want Constant at column %d", last, NameColumn(Options{}))
	}
}

// A file copy that fails part-way must not leave a truncated dst that looks
// complete, nor its temporary file. Reading a directory as a file fails after
// the temporary file exists, which is the path under test.
func TestCopyFileFailureLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mkdir(t, src)
	info, err := os.Lstat(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	mkdir(t, out)
	dst := filepath.Join(out, "dst")
	if err := copyFile(src, dst, info); err == nil {
		t.Fatal("copyFile of a directory succeeded; the test needs it to fail mid-copy")
	}
	if exists(dst) {
		t.Error("a failed copy left a destination file")
	}
	noTempFiles(t, out)
}
