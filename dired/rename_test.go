package dired

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// files makes a file per name in a fresh directory, each holding its own name,
// so a test can tell after renaming which file ended up where.
func files(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// holds checks that each name in dir holds what the want map says: the name
// of the file that was there first.
func holds(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	for name, was := range want {
		if got := readFile(t, filepath.Join(dir, name)); got != was {
			t.Errorf("%s holds %q, want %q", name, got, was)
		}
	}
	names, err := readNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.HasPrefix(n, ".nem-rename-") {
			t.Errorf("temporary name %s left behind", n)
		}
	}
}

func TestRenameAllRenames(t *testing.T) {
	dir := files(t, "a", "b")
	if err := RenameAll(dir, []Rename{{"a", "x"}}); err != nil {
		t.Fatal(err)
	}
	holds(t, dir, map[string]string{"x": "a", "b": "b"})
	if exists(filepath.Join(dir, "a")) {
		t.Error("a is still there")
	}
}

// Renames happen as if at one instant: a swap, and a run shifted along one,
// work although each takes a name another holds.
func TestRenameAllSwapsAndShifts(t *testing.T) {
	dir := files(t, "a", "b")
	if err := RenameAll(dir, []Rename{{"a", "b"}, {"b", "a"}}); err != nil {
		t.Fatal(err)
	}
	holds(t, dir, map[string]string{"a": "b", "b": "a"})

	dir = files(t, "1", "2", "3")
	if err := RenameAll(dir, []Rename{{"1", "2"}, {"2", "3"}, {"3", "4"}}); err != nil {
		t.Fatal(err)
	}
	holds(t, dir, map[string]string{"2": "1", "3": "2", "4": "3"})
}

// Into a subdirectory, and out to the parent, by a name with a slash in it.
func TestRenameAllMovesBetweenDirectories(t *testing.T) {
	dir := files(t, "a", "b", "sub/")
	if err := RenameAll(dir, []Rename{{"a", "sub/a"}, {"b", "../b-moved"}}); err != nil {
		t.Fatal(err)
	}
	holds(t, filepath.Join(dir, "sub"), map[string]string{"a": "a"})
	holds(t, filepath.Dir(dir), map[string]string{"b-moved": "b"})
}

// Every refusal is made before anything moves.
func TestRenameAllRefusesWithoutMovingAnything(t *testing.T) {
	for _, tc := range []struct {
		name string
		rs   []Rename
		want string
	}{
		{"overwrite", []Rename{{"a", "x"}, {"b", "c"}}, "c already exists"},
		{"two to one", []Rename{{"a", "x"}, {"b", "x"}}, "a and b cannot both be named x"},
		{"no directory", []Rename{{"a", "x"}, {"b", "nowhere/b"}}, "no directory nowhere"},
		{"into a file", []Rename{{"a", "x"}, {"b", "c/b"}}, "no directory c"},
		{"into itself", []Rename{{"a", "x"}, {"sub", "sub/deeper"}}, "sub cannot go inside itself"},
		{"into a renamed one", []Rename{{"a", "sub/a"}, {"sub", "dir"}}, "which is being renamed too"},
		{"no such file", []Rename{{"a", "x"}, {"gone", "y"}}, "gone:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := files(t, "a", "b", "c", "sub/")
			err := RenameAll(dir, tc.rs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one saying %q", err, tc.want)
			}
			holds(t, dir, map[string]string{"a": "a", "b": "b", "c": "c"})
			if exists(filepath.Join(dir, "x")) {
				t.Error("x was created")
			}
		})
	}
}

// A rename that fails part-way puts back the ones already made.
func TestRenameAllUndoesAFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot write to")
	}
	dir := files(t, "a", "b", "locked/")
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	err := RenameAll(dir, []Rename{{"b", "c"}, {"a", "locked/a"}})
	if err == nil || !strings.Contains(err.Error(), "nothing was renamed") {
		t.Fatalf("err = %v, want a failure that renamed nothing", err)
	}
	holds(t, dir, map[string]string{"a": "a", "b": "b"})
	if exists(filepath.Join(dir, "c")) {
		t.Error("b's rename to c was not undone")
	}
}
