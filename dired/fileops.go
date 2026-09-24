package dired

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// None of the operations here overwrites anything. Deciding to replace a file
// is the caller's job - it asks the user, removes the old one, then calls
// again - so every refusal that is not "dst exists" must avoid fs.ErrExist:
// a caller that saw ErrExist for "dst is src" would remove dst, and with it
// the source.
var (
	errIntoItself = errors.New("cannot copy or move a directory into itself")
	errSameFile   = errors.New("source and destination are the same file")
	errSpecial    = errors.New("cannot copy a special file")
	errRoot       = errors.New("refusing to remove a filesystem root")
)

// Copy copies src to dst without following a symlink at src.
//
// A file keeps its contents, permission bits and, where the filesystem allows,
// its modification time. A directory is copied recursively. A symlink is
// recreated with the same link text. Copy refuses when dst exists (an error
// satisfying errors.Is(err, fs.ErrExist)), and when dst is src or lies inside
// it, since copying a directory into itself would never finish.
//
// A file is written to a temporary name beside dst and renamed into place, so
// a failure part-way never leaves a truncated dst that looks complete. A
// directory copy that fails part-way may leave a partial tree.
func Copy(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := refuse("copy", src, dst, info); err != nil {
		return err
	}
	return copyTree(src, dst, info)
}

// Move renames src to dst, falling back to copy-then-remove when they are on
// different filesystems, which rename cannot cross. It refuses exactly what
// Copy refuses.
func Move(src, dst string) error { return move(src, dst, os.Rename) }

// move is Move with rename injectable, so the cross-filesystem fallback can be
// driven by a test without a second filesystem.
func move(src, dst string, rename func(oldpath, newpath string) error) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if caseOnlyRename(src, dst, info) {
		return rename(src, dst)
	}
	if err := refuse("move", src, dst, info); err != nil {
		return err
	}
	if err := rename(src, dst); err != nil {
		if !isCrossDevice(err) {
			return err
		}
		return moveAcross(src, dst)
	}
	return nil
}

// moveAcross moves by copying then removing the source. The source is only
// removed once the copy is complete, so a failure leaves it intact.
func moveAcross(src, dst string) error {
	if err := Copy(src, dst); err != nil {
		return err
	}
	return Remove(src)
}

// caseOnlyRename reports a rename that changes only the case of a name on a
// filesystem that ignores case. There dst "exists" because it IS src, and
// renaming "readme" to "README" is how case is changed - refusing it as an
// overwrite would make it impossible from dired.
func caseOnlyRename(src, dst string, info fs.FileInfo) bool {
	if src == dst || !strings.EqualFold(filepath.Clean(src), filepath.Clean(dst)) {
		return false
	}
	d, err := os.Lstat(dst)
	return err == nil && os.SameFile(info, d)
}

// Remove deletes path: a directory with everything in it, anything else on
// its own. A symlink to a directory removes the link and never touches the
// target's contents. Unlike os.RemoveAll, a path that does not exist is an
// error wrapping fs.ErrNotExist: the user asked to delete something they saw,
// and it silently not being there is worth telling them.
func Remove(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return &fs.PathError{Op: "remove", Path: path, Err: pe.Err}
		}
		return err
	}
	if !info.IsDir() {
		// Lstat, not Stat: a symlink to a directory reports as a symlink,
		// so it lands here and only the link goes.
		return os.Remove(path)
	}
	if abs, err := filepath.Abs(path); err == nil && filepath.Dir(abs) == abs {
		return &fs.PathError{Op: "remove", Path: path, Err: errRoot}
	}
	return os.RemoveAll(path)
}

// refuse returns the error for a destination that must not be written, or nil.
//
// The into-itself check comes before the existence check on purpose. Copying
// /a to an existing /a/sub must not report ErrExist, or the caller would
// remove /a/sub - part of the source - before discovering the copy is refused
// anyway.
func refuse(op, src, dst string, info fs.FileInfo) error {
	if info.IsDir() && inside(src, dst, info) {
		return &fs.PathError{Op: op, Path: dst, Err: errIntoItself}
	}
	d, err := os.Lstat(dst)
	switch {
	case err == nil:
		// Another name for src itself - a hard link, or a differently cased
		// name on a case-insensitive filesystem - must not read as "exists",
		// for the same reason as above.
		if os.SameFile(info, d) {
			return &fs.PathError{Op: op, Path: dst, Err: errSameFile}
		}
		return &fs.PathError{Op: op, Path: dst, Err: fs.ErrExist}
	case errors.Is(err, fs.ErrNotExist):
		return nil
	default:
		return err
	}
}

// inside reports whether dst is the directory src or somewhere below it.
//
// The lexical check catches the plain case. The physical one resolves dst's
// parent through any symlinks and walks up it comparing file identity, which
// catches a path that reaches into src through a link, or through a different
// case on a case-insensitive filesystem - either would otherwise recurse until
// the disk filled.
func inside(src, dst string, info fs.FileInfo) bool {
	s, err1 := filepath.Abs(src)
	d, err2 := filepath.Abs(dst)
	if err1 == nil && err2 == nil && under(s, d) {
		return true
	}
	if err2 != nil {
		return false
	}
	// dst's parent not resolving means it does not exist, and then neither
	// can dst be created, so there is nothing to recurse into.
	dir, err := filepath.EvalSymlinks(filepath.Dir(d))
	if err != nil {
		return false
	}
	for {
		if fi, err := os.Stat(dir); err == nil && os.SameFile(fi, info) {
			return true
		}
		up := filepath.Dir(dir)
		if up == dir {
			return false
		}
		dir = up
	}
}

// under reports whether child is parent or lies below it, lexically.
func under(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// copyTree copies whatever src is, without following it if it is a link.
func copyTree(src, dst string, info fs.FileInfo) error {
	m := info.Mode()
	switch {
	case m&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case m.IsDir():
		return copyDir(src, dst, info)
	case m.IsRegular():
		return copyFile(src, dst, info)
	}
	// A pipe, socket or device cannot be recreated portably, and reading one
	// as a file could block forever or consume someone else's data.
	return &fs.PathError{Op: "copy", Path: src, Err: errSpecial}
}

func copyDir(src, dst string, info fs.FileInfo) error {
	// Created owner-writable and given its real mode only once full: a
	// read-only source directory would otherwise yield a destination that
	// nothing can be copied into.
	if err := os.Mkdir(dst, 0o700); err != nil {
		return err
	}
	names, err := readNames(src)
	if err != nil {
		return err
	}
	for _, name := range names {
		s := filepath.Join(src, name)
		fi, err := os.Lstat(s)
		if err != nil {
			return err
		}
		if err := copyTree(s, filepath.Join(dst, name), fi); err != nil {
			return err
		}
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	// Last, because writing the children bumps the directory's own time.
	// Best effort: not every filesystem stores it, and a copy is no less a
	// copy without it.
	_ = os.Chtimes(dst, time.Time{}, info.ModTime())
	return nil
}

func copyFile(src, dst string, info fs.FileInfo) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Beside dst so the final rename stays on one filesystem and is atomic.
	// The name is short and fixed rather than derived from dst, which may
	// already be as long as a name can be.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".nem-copy-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err = io.Copy(tmp, in); err != nil {
		return err
	}
	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	_ = os.Chtimes(tmp.Name(), time.Time{}, info.ModTime())
	return os.Rename(tmp.Name(), dst)
}

// isCrossDevice reports a rename that failed only because src and dst are on
// different filesystems. errors.Is unwraps the *os.LinkError rename returns.
func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
