package dired

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// noTempFiles fails if a copy left its temporary file behind in dir.
func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	names, err := readNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.HasPrefix(n, ".nem-copy-") {
			t.Errorf("temporary file %s left in %s", n, dir)
		}
	}
}

// tree describes a directory as relative path -> contents, with "/" marking a
// directory and "-> x" a symlink to x, so two trees compare with one ==.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			out[rel] = "-> " + target
		case d.IsDir():
			out[rel] = "/"
		default:
			out[rel] = readFile(t, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src.txt"), filepath.Join(dir, "dst.txt")
	writeFile(t, src, "contents\n", 0o640)

	if err := Copy(src, dst); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if got := readFile(t, dst); got != "contents\n" {
		t.Errorf("dst holds %q, want %q", got, "contents\n")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o640 {
			t.Errorf("dst mode %v, want %v", got, fs.FileMode(0o640))
		}
	}
	if !info.ModTime().Equal(fixedTime) {
		t.Errorf("dst mtime %v, want the source's %v", info.ModTime(), fixedTime)
	}
	if got := readFile(t, src); got != "contents\n" {
		t.Errorf("source changed to %q", got)
	}
	noTempFiles(t, dir)
}

func TestCopyDirectoryTree(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mkdir(t, src)
	writeFile(t, filepath.Join(src, "a.txt"), "a", 0o644)
	mkdir(t, filepath.Join(src, "sub"))
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b", 0o600)
	mkdir(t, filepath.Join(src, "sub", "deeper"))
	writeFile(t, filepath.Join(src, "sub", "deeper", "c"), "c", 0o755)
	mkdir(t, filepath.Join(src, "empty"))
	if err := os.Chmod(filepath.Join(src, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "dst")
	if err := Copy(src, dst); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if a, b := tree(t, src), tree(t, dst); !sameTree(a, b) {
		t.Errorf("copied tree differs:\n src %v\n dst %v", a, b)
	}
	if runtime.GOOS != "windows" {
		for rel, want := range map[string]fs.FileMode{"sub": 0o750, "sub/b.txt": 0o600, "sub/deeper/c": 0o755} {
			info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != want {
				t.Errorf("%s: mode %v, want %v", rel, got, want)
			}
		}
	}
}

// A read-only source directory must still be copyable: the destination is
// only made read-only once everything is in it.
func TestCopyReadOnlyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not restrict writing on Windows")
	}
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "ro"), filepath.Join(dir, "copy")
	mkdir(t, src)
	writeFile(t, filepath.Join(src, "f"), "x", 0o444)
	if err := os.Chmod(src, 0o555); err != nil {
		t.Fatal(err)
	}
	// Restore write permission so the temporary directory can be removed.
	t.Cleanup(func() { os.Chmod(src, 0o755); os.Chmod(dst, 0o755) })

	if err := Copy(src, dst); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if got := readFile(t, filepath.Join(dst, "f")); got != "x" {
		t.Errorf("copied file holds %q, want %q", got, "x")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o555 {
		t.Errorf("copied directory mode %v, want %v", got, fs.FileMode(0o555))
	}
}

func TestCopySymlinkIsNotFollowed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "real"), "data", 0o644)
	link := filepath.Join(dir, "link")
	symlinkOrSkip(t, "real", link)
	broken := filepath.Join(dir, "broken")
	symlinkOrSkip(t, "no/such/target", broken)

	for _, src := range []string{link, broken} {
		dst := src + ".copy"
		if err := Copy(src, dst); err != nil {
			t.Fatalf("Copy(%s): %v", src, err)
		}
		info, err := os.Lstat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("%s: copy has mode %v, want a symlink", filepath.Base(src), info.Mode())
		}
		want, _ := os.Readlink(src)
		if got, _ := os.Readlink(dst); got != want {
			t.Errorf("%s: copy links to %q, want the same text %q", filepath.Base(src), got, want)
		}
	}
}

func TestCopyRefusesAnExistingDestination(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), "new", 0o644)
	writeFile(t, filepath.Join(dir, "b"), "old", 0o644)
	mkdir(t, filepath.Join(dir, "d"))
	mkdir(t, filepath.Join(dir, "e"))

	for _, tc := range []struct{ src, dst string }{{"a", "b"}, {"d", "e"}, {"a", "e"}, {"d", "b"}} {
		err := Copy(filepath.Join(dir, tc.src), filepath.Join(dir, tc.dst))
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("Copy(%s, %s) = %v, want an error satisfying fs.ErrExist", tc.src, tc.dst, err)
		}
	}
	if got := readFile(t, filepath.Join(dir, "b")); got != "old" {
		t.Errorf("the existing file was overwritten: it holds %q", got)
	}

	// A dangling symlink is still something at dst.
	symlinkOrSkip(t, "nowhere", filepath.Join(dir, "dangling"))
	if err := Copy(filepath.Join(dir, "a"), filepath.Join(dir, "dangling")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Copy onto a dangling symlink = %v, want fs.ErrExist", err)
	}
}

// Copying a directory into itself would recurse until the disk filled. The
// refusal must not look like ErrExist: a caller that saw that would remove
// the destination - part of the source - and try again.
func TestCopyRefusesIntoItself(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mkdir(t, src)
	mkdir(t, filepath.Join(src, "existing"))
	writeFile(t, filepath.Join(src, "f"), "x", 0o644)

	for _, dst := range []string{
		src,
		filepath.Join(src, "new"),
		filepath.Join(src, "existing"),
		filepath.Join(src, "existing", "deeper"),
		filepath.Join(src, "existing", "..", "sneaky"),
	} {
		err := Copy(src, dst)
		if !errors.Is(err, errIntoItself) {
			t.Errorf("Copy(src, %s) = %v, want the into-itself refusal", dst, err)
		}
		if errors.Is(err, fs.ErrExist) {
			t.Errorf("Copy(src, %s) reported ErrExist, inviting the caller to delete part of the source", dst)
		}
	}
	if exists(filepath.Join(src, "new")) || exists(filepath.Join(src, "sneaky")) {
		t.Error("a refused copy still created its destination")
	}

	// A symlink that leads back into src is the same trap by another path.
	back := filepath.Join(dir, "back")
	symlinkOrSkip(t, src, back)
	if err := Copy(src, filepath.Join(back, "new")); !errors.Is(err, errIntoItself) {
		t.Errorf("Copy(src, back/new) through a symlink into src = %v, want the into-itself refusal", err)
	}

	// A sibling whose name merely starts with src's is not inside it.
	if err := Copy(src, src+"2"); err != nil {
		t.Errorf("Copy(src, src2) = %v, want success", err)
	}
}

// A second name for the same file must be refused without ErrExist, or the
// caller would delete it - and with it the only copy of the data.
func TestCopyAndMoveRefuseTheSameFile(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeFile(t, a, "only copy", 0o644)
	if err := os.Link(a, b); err != nil {
		t.Skipf("cannot create hard links here: %v", err)
	}
	for name, op := range map[string]func(string, string) error{"Copy": Copy, "Move": Move} {
		err := op(a, b)
		if !errors.Is(err, errSameFile) || errors.Is(err, fs.ErrExist) {
			t.Errorf("%s(a, hard link to a) = %v, want the same-file refusal and not ErrExist", name, err)
		}
	}
	if readFile(t, a) != "only copy" || readFile(t, b) != "only copy" {
		t.Error("a refused operation changed the file")
	}
}

func TestCopyMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := Copy(filepath.Join(dir, "absent"), filepath.Join(dir, "x")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Copy of a missing source = %v, want fs.ErrNotExist", err)
	}
}

func TestMove(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), "a", 0o644)
	mkdir(t, filepath.Join(dir, "d"))
	writeFile(t, filepath.Join(dir, "d", "inner"), "i", 0o644)

	if err := Move(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
		t.Fatalf("Move file: %v", err)
	}
	if exists(filepath.Join(dir, "a")) || readFile(t, filepath.Join(dir, "b")) != "a" {
		t.Error("after Move(a, b), want a gone and b holding a's contents")
	}

	if err := Move(filepath.Join(dir, "d"), filepath.Join(dir, "e")); err != nil {
		t.Fatalf("Move dir: %v", err)
	}
	if exists(filepath.Join(dir, "d")) || readFile(t, filepath.Join(dir, "e", "inner")) != "i" {
		t.Error("after Move(d, e), want d gone and e/inner present")
	}
}

func TestMoveRefusesAnExistingDestination(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a"), "a", 0o644)
	writeFile(t, filepath.Join(dir, "b"), "b", 0o644)
	if err := Move(filepath.Join(dir, "a"), filepath.Join(dir, "b")); !errors.Is(err, fs.ErrExist) {
		t.Errorf("Move onto an existing file = %v, want fs.ErrExist", err)
	}
	if readFile(t, filepath.Join(dir, "a")) != "a" || readFile(t, filepath.Join(dir, "b")) != "b" {
		t.Error("a refused Move changed a file")
	}
}

func TestMoveRefusesIntoItself(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mkdir(t, src)
	mkdir(t, filepath.Join(src, "sub"))
	for _, dst := range []string{filepath.Join(src, "new"), filepath.Join(src, "sub")} {
		err := Move(src, dst)
		if !errors.Is(err, errIntoItself) || errors.Is(err, fs.ErrExist) {
			t.Errorf("Move(src, %s) = %v, want the into-itself refusal and not ErrExist", dst, err)
		}
	}
	if !exists(filepath.Join(src, "sub")) {
		t.Error("a refused Move disturbed the source")
	}
}

// crossDevice is a rename that fails the way one across filesystems does.
func crossDevice(oldpath, newpath string) error {
	return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errCrossDevice}
}

func TestMoveFallsBackToCopyAcrossFilesystems(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	mkdir(t, src)
	writeFile(t, filepath.Join(src, "f"), "payload", 0o644)
	mkdir(t, filepath.Join(src, "sub"))
	writeFile(t, filepath.Join(src, "sub", "g"), "more", 0o644)
	want := tree(t, src)

	dst := filepath.Join(dir, "dst")
	if err := move(src, dst, crossDevice); err != nil {
		t.Fatalf("move with a cross-device rename: %v", err)
	}
	if exists(src) {
		t.Error("source still present after a cross-device move")
	}
	if got := tree(t, dst); !sameTree(got, want) {
		t.Errorf("moved tree %v, want %v", got, want)
	}

	// The fallback on its own, for a single file.
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeFile(t, a, "x", 0o644)
	if err := moveAcross(a, b); err != nil {
		t.Fatalf("moveAcross: %v", err)
	}
	if exists(a) || readFile(t, b) != "x" {
		t.Error("after moveAcross(a, b), want a gone and b holding its contents")
	}
}

// Any other rename failure is reported as it is: copying and deleting on a
// permission error would turn a refusal into data loss.
func TestMoveReportsOtherRenameErrors(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	writeFile(t, src, "a", 0o644)
	denied := func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: fs.ErrPermission}
	}
	if err := move(src, dst, denied); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("move = %v, want the rename's own error", err)
	}
	if !exists(src) || exists(dst) {
		t.Error("a failed rename must leave the source alone and create nothing")
	}
}

func TestIsCrossDevice(t *testing.T) {
	if !isCrossDevice(crossDevice("a", "b")) {
		t.Error("isCrossDevice missed the platform's cross-device error inside a LinkError")
	}
	for _, err := range []error{nil, fs.ErrPermission, &os.LinkError{Op: "rename", Err: fs.ErrNotExist}} {
		if isCrossDevice(err) {
			t.Errorf("isCrossDevice(%v) = true, want false", err)
		}
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()

	f := filepath.Join(dir, "f")
	writeFile(t, f, "x", 0o644)
	if err := Remove(f); err != nil || exists(f) {
		t.Errorf("Remove(file) = %v, exists afterwards %v; want nil, false", err, exists(f))
	}

	d := filepath.Join(dir, "d")
	mkdir(t, d)
	mkdir(t, filepath.Join(d, "sub"))
	writeFile(t, filepath.Join(d, "sub", "g"), "g", 0o644)
	if err := Remove(d); err != nil || exists(d) {
		t.Errorf("Remove(dir tree) = %v, exists afterwards %v; want nil, false", err, exists(d))
	}

	missing := filepath.Join(dir, "missing")
	err := Remove(missing)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove(missing) = %v, want fs.ErrNotExist", err)
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Op != "remove" || pe.Path != missing {
		t.Errorf("Remove(missing) = %#v, want a *fs.PathError for remove %s", err, missing)
	}
}

// Deleting a link to a directory must delete the link. Following it would
// wipe out a directory the user never selected.
func TestRemoveSymlinkToDirectoryKeepsTheTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	mkdir(t, target)
	writeFile(t, filepath.Join(target, "precious"), "keep me", 0o644)
	link := filepath.Join(dir, "link")
	symlinkOrSkip(t, target, link)

	if err := Remove(link); err != nil {
		t.Fatalf("Remove(link): %v", err)
	}
	if exists(link) {
		t.Error("the link survived Remove")
	}
	if got := readFile(t, filepath.Join(target, "precious")); got != "keep me" {
		t.Errorf("the link's target was disturbed: precious holds %q", got)
	}
	names, _ := readNames(target)
	if !slices.Equal(names, []string{"precious"}) {
		t.Errorf("target now holds %v, want just precious", names)
	}
}
