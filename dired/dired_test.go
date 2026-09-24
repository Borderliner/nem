package dired

import (
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// fixedTime is a whole second, so any filesystem can store it exactly.
var fixedTime = time.Date(2025, time.March, 4, 5, 6, 7, 0, time.UTC)

func writeFile(t *testing.T, path, content string, perm fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	// WriteFile's mode is filtered through the umask; the tests need the
	// exact bits they asked for.
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// symlinkOrSkip makes a symlink, skipping the test where the platform will not
// let an unprivileged user create one (Windows without developer mode).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

func byName(t *testing.T, entries []Entry) map[string]Entry {
	t.Helper()
	m := make(map[string]Entry, len(entries))
	for _, e := range entries {
		if _, dup := m[e.Name]; dup {
			t.Fatalf("Read listed %q twice", e.Name)
		}
		m[e.Name] = e
	}
	return m
}

func TestReadPlainEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "hello", 0o644)
	writeFile(t, filepath.Join(dir, ".hidden"), "", 0o644)
	writeFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n", 0o755)
	mkdir(t, filepath.Join(dir, "sub"))

	entries, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := byName(t, entries)
	if len(got) != 5 {
		t.Fatalf("Read returned %d entries, want 4 and the parent: %v", len(got), entries)
	}
	if up := got[ParentName]; !up.IsParent() || !up.IsDir || up.Hidden() {
		t.Errorf(".. = %+v, want a directory that is the parent and not hidden", up)
	}

	a := got["a.txt"]
	if !a.Mode.IsRegular() || a.Size != 5 || !a.ModTime.Equal(fixedTime) || a.IsDir || a.Target != "" || a.Broken {
		t.Errorf("a.txt = %+v, want a regular 5-byte file dated %v", a, fixedTime)
	}
	if a.Hidden() || a.Executable() {
		t.Errorf("a.txt: Hidden %v Executable %v, want false false", a.Hidden(), a.Executable())
	}

	if sub := got["sub"]; !sub.IsDir || !sub.Mode.IsDir() {
		t.Errorf("sub = %+v, want a directory", sub)
	}
	if h := got[".hidden"]; !h.Hidden() {
		t.Errorf(".hidden: Hidden() = false, want true")
	}
	// Windows has no execute bits; Go synthesises them from the extension.
	if runtime.GOOS != "windows" {
		if r := got["run.sh"]; !r.Executable() {
			t.Errorf("run.sh (mode %v): Executable() = false, want true", r.Mode)
		}
		if s := got["sub"]; s.Executable() {
			t.Errorf("sub: Executable() = true; a directory is never executable")
		}
	}
}

func TestReadSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real.txt"), "data", 0o644)
	writeFile(t, filepath.Join(dir, "prog"), "", 0o755)
	mkdir(t, filepath.Join(dir, "sub"))
	symlinkOrSkip(t, "real.txt", filepath.Join(dir, "link.txt"))
	symlinkOrSkip(t, "sub", filepath.Join(dir, "linkdir"))
	symlinkOrSkip(t, "nowhere", filepath.Join(dir, "dangling"))
	symlinkOrSkip(t, "prog", filepath.Join(dir, "proglink"))

	entries, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	got := byName(t, entries)

	for _, tc := range []struct {
		name, target  string
		isDir, broken bool
	}{
		{"link.txt", "real.txt", false, false},
		{"linkdir", "sub", true, false},
		{"dangling", "nowhere", false, true},
	} {
		e := got[tc.name]
		if e.Mode&fs.ModeSymlink == 0 {
			t.Errorf("%s: mode %v, want a symlink (Lstat, not Stat)", tc.name, e.Mode)
		}
		if e.Target != tc.target || e.IsDir != tc.isDir || e.Broken != tc.broken {
			t.Errorf("%s: Target %q IsDir %v Broken %v, want %q %v %v",
				tc.name, e.Target, e.IsDir, e.Broken, tc.target, tc.isDir, tc.broken)
		}
		if e.Executable() {
			t.Errorf("%s: Executable() = true, want false", tc.name)
		}
	}
	if runtime.GOOS != "windows" {
		if e := got["proglink"]; !e.Executable() {
			t.Errorf("proglink -> an executable: Executable() = false, want true (the target's bits count)")
		}
	}
}

// A directory that can be listed but not searched yields names whose Lstat
// fails. They must still be listed, or the user cannot see files ls shows.
func TestReadKeepsEntriesWhoseLstatFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no search permission to withhold on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "locked")
	mkdir(t, sub)
	writeFile(t, filepath.Join(sub, "secret"), "x", 0o644)
	if err := os.Chmod(sub, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sub, 0o755) })

	entries, err := Read(sub)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 || !entries[0].IsParent() {
		t.Fatalf("Read returned %v, want the parent and the one entry", entries)
	}
	e := entries[1]
	if e.Name != "secret" || e.Mode != 0 || e.Size != 0 || !e.ModTime.IsZero() {
		t.Errorf("entry = %+v, want only the name known", e)
	}
	if !e.unknown() {
		t.Errorf("unknown() = false for an entry whose Lstat failed")
	}
}

func TestReadMissingDirectoryIsAnError(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("Read of a missing directory returned no error")
	}
}

func TestExecutable(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    Entry
		want bool
	}{
		{"plain file", Entry{Mode: 0o644}, false},
		{"owner exec", Entry{Mode: 0o744}, true},
		{"other exec only", Entry{Mode: 0o601}, true},
		{"directory", Entry{Mode: fs.ModeDir | 0o755, IsDir: true}, false},
		{"pipe with x", Entry{Mode: fs.ModeNamedPipe | 0o755}, false},
		{"link to program", Entry{Mode: fs.ModeSymlink | 0o777, resolved: 0o755}, true},
		{"link to data", Entry{Mode: fs.ModeSymlink | 0o777, resolved: 0o644}, false},
		{"link to dir", Entry{Mode: fs.ModeSymlink | 0o777, resolved: fs.ModeDir | 0o755, IsDir: true}, false},
		{"broken link", Entry{Mode: fs.ModeSymlink | 0o777, Broken: true}, false},
	} {
		if got := tc.e.Executable(); got != tc.want {
			t.Errorf("%s: Executable() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSortKeyStringAndNext(t *testing.T) {
	for _, tc := range []struct {
		k    SortKey
		name string
		next SortKey
	}{
		{ByName, "name", ByTime},
		{ByTime, "time", BySize},
		{BySize, "size", ByName},
	} {
		if got := tc.k.String(); got != tc.name {
			t.Errorf("%d.String() = %q, want %q", tc.k, got, tc.name)
		}
		if got := tc.k.Next(); got != tc.next {
			t.Errorf("%v.Next() = %v, want %v", tc.k, got, tc.next)
		}
	}
	if (Options{}).Sort != ByName {
		t.Error("the zero Options must sort by name")
	}
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestSortOrders(t *testing.T) {
	t0 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	dir := func(name string, size int64, h int) Entry {
		return Entry{Name: name, Mode: fs.ModeDir | 0o755, IsDir: true, Size: size, ModTime: at(h)}
	}
	file := func(name string, size int64, h int) Entry {
		return Entry{Name: name, Mode: 0o644, Size: size, ModTime: at(h)}
	}
	entries := []Entry{
		file("b.txt", 10, 1),
		dir("Zeta", 1, 5),
		file("C.md", 300, 3),
		file(".bashrc", 300, 3),
		dir("alpha", 9000, 1),
		file("B.txt", 10, 9),
		file("a.txt", 50, 7),
		dir(".git", 50, 5),
		file("..dots", 1, 0),
	}

	for _, tc := range []struct {
		key  SortKey
		want []string
	}{
		// Case folded, one leading dot ignored, raw name breaking the tie
		// between B.txt and b.txt; "..dots" keeps its second dot.
		{ByName, []string{"alpha", ".git", "Zeta", "..dots", "a.txt", "B.txt", "b.txt", ".bashrc", "C.md"}},
		// Newest first; .git and Zeta share a time and fall back to name.
		{ByTime, []string{".git", "Zeta", "alpha", "B.txt", "a.txt", ".bashrc", "C.md", "b.txt", "..dots"}},
		// Directories by name whatever their size; files largest first, and
		// equal sizes by name.
		{BySize, []string{"alpha", ".git", "Zeta", ".bashrc", "C.md", "a.txt", "B.txt", "b.txt", "..dots"}},
	} {
		t.Run(tc.key.String(), func(t *testing.T) {
			// Every input order must give the same answer: the order is total.
			rng := rand.New(rand.NewPCG(1, 2))
			for range 50 {
				in := slices.Clone(entries)
				rng.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
				sortEntries(in, tc.key)
				if got := names(in); !slices.Equal(got, tc.want) {
					t.Fatalf("sorted = %q\n          want %q", got, tc.want)
				}
			}
		})
	}
}

// A filesystem root has nowhere above it, so it gets no parent entry.
func TestReadRootHasNoParent(t *testing.T) {
	entries, err := Read(string(filepath.Separator))
	if err != nil {
		t.Skipf("cannot read the root here: %v", err)
	}
	for _, e := range entries {
		if e.IsParent() {
			t.Fatal("the root lists a parent")
		}
	}
}
