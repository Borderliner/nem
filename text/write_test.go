package text

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// saveOver loads the file at path, replaces its text with s and saves it.
func saveOver(t *testing.T, path, s string) (*Buffer, error) {
	t.Helper()
	b, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(Pos{}, b.End()); err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(Pos{}, []rune(s)); err != nil {
		t.Fatal(err)
	}
	return b, b.Save()
}

func writeTestFile(t *testing.T, path, s string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil { // past the umask
		t.Fatal(err)
	}
}

func wantFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s holds %q, want %q", filepath.Base(path), got, want)
	}
}

// wantOnly checks dir holds exactly the names given: no temporary file left
// behind by a save, whether it worked or not.
func wantOnly(t *testing.T, dir string, names ...string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	if len(got) != len(names) {
		t.Fatalf("%s holds %q, want %q", dir, got, names)
	}
	for i := range got {
		if got[i] != names[i] {
			t.Fatalf("%s holds %q, want %q", dir, got, names)
		}
	}
}

// A save that fails part-way - a disk that fills up - leaves the file as it
// was. Written in place, it was left cut short at whatever had fitted.
func TestAFailedSaveLeavesTheFileWhole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeTestFile(t, path, "the old text\n", 0o644)

	full := errors.New("no space left on device")
	was := writeData
	writeData = func(f *os.File, data []byte) error {
		_, _ = f.Write(data[:len(data)/2])
		return full
	}
	defer func() { writeData = was }()

	b, err := saveOver(t, path, "a new text, longer than the old")
	if !errors.Is(err, full) {
		t.Fatalf("save: %v, want the write's error", err)
	}
	wantFile(t, path, "the old text\n")
	wantOnly(t, dir, "notes.txt")
	if !b.Modified() {
		t.Error("the buffer is marked saved after the save failed")
	}
}

// A save leaves nothing behind but the file.
func TestASaveLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeTestFile(t, path, "old\n", 0o644)
	if _, err := saveOver(t, path, "new"); err != nil {
		t.Fatal(err)
	}
	wantFile(t, path, "new\n")
	wantOnly(t, dir, "notes.txt")
}

// Replaced, a file keeps its mode: a script stays executable, a private file
// private.
func TestASaveKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows files have no mode bits beyond read-only")
	}
	for _, mode := range []os.FileMode{0o755, 0o600, 0o640} {
		path := filepath.Join(t.TempDir(), "run.sh")
		writeTestFile(t, path, "echo old\n", mode)
		if _, err := saveOver(t, path, "echo new"); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != mode {
			t.Errorf("mode %v after the save, want %v", got, mode)
		}
	}
}

// Saved through a symbolic link, the file it points at is written and the
// link stays a link.
func TestASaveThroughALinkWritesWhatItPointsAt(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	writeTestFile(t, real, "old\n", 0o644)
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
	if _, err := saveOver(t, link, "new"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a file")
	}
	wantFile(t, real, "new\n")
}

// A file with another name linked to it is written in place, so both names
// go on naming the same file. Replaced, the other name kept the old text.
func TestASaveKeepsHardLinksTogether(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nem cannot see a file's link count on Windows")
	}
	dir := t.TempDir()
	one, two := filepath.Join(dir, "one.txt"), filepath.Join(dir, "two.txt")
	writeTestFile(t, one, "old\n", 0o644)
	if err := os.Link(one, two); err != nil {
		t.Skipf("cannot link files here: %v", err)
	}
	if _, err := saveOver(t, one, "new"); err != nil {
		t.Fatal(err)
	}
	wantFile(t, two, "new\n")
}

// A read-only file is refused, as it was when nem wrote into it. Renaming a
// new file over it would get round the protection the user put there.
func TestASaveRefusesAReadOnlyFile(t *testing.T) {
	if runtime.GOOS != "windows" && os.Geteuid() == 0 {
		t.Skip("root may write any file")
	}
	path := filepath.Join(t.TempDir(), "locked.txt")
	writeTestFile(t, path, "old\n", 0o444)
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := saveOver(t, path, "new"); err == nil {
		t.Error("saved over a read-only file")
	}
	wantFile(t, path, "old\n")
}

// A file nem may write, in a directory it may not create files in, is
// written in place: there is nowhere to put a temporary file.
func TestASaveInALockedDirectoryWritesInPlace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directories have no write bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("root may create files anywhere")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "notes.txt")
	writeTestFile(t, path, "old\n", 0o644)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := saveOver(t, path, "new"); err != nil {
		t.Fatal(err)
	}
	wantFile(t, path, "new\n")
}
