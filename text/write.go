package text

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Saving replaces a file whole or not at all. The new contents go into a
// temporary file beside it, flushed to the disk, and that is renamed over the
// file. Writing into the file itself, as nem did, emptied it first: a full
// disk, a crash or a power cut part-way through left it cut short, the old
// text gone and the new not all there.
//
// A rename puts a new file in the old one's place, though, and some files
// must stay the file they are. Another name linked to it would go on holding
// the old text; a file owned by someone else would change hands; a device or
// a pipe cannot be replaced at all; and a directory nem may not create files
// in can still hold one it may write. Those are written in place, as before -
// what emacs does, for the same reasons, with backup-by-copying.

// errCannotReplace reports that a file cannot be replaced by renaming over
// it, and must be written in place instead.
var errCannotReplace = errors.New("text: the file cannot be replaced")

// writeData writes data into f. A variable so a test can make a write fail
// part-way, as a full disk does.
var writeData = func(f *os.File, data []byte) error {
	_, err := f.Write(data)
	return err
}

// writeFile replaces the contents of the file at path with data. A link is
// followed, and what it points at written; the link stays a link.
func writeFile(path string, data []byte) error {
	target := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		target = real
	}
	fi, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return writeNew(target, data)
	}
	if err != nil {
		return err
	}
	// A file nem may not write is refused, as writing into it was refused.
	// Renaming over it would get round that - a read-only file replaced by
	// a writable one - where the user should be told instead.
	f, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	f.Close()

	if !fi.Mode().IsRegular() || !replaceable(fi) {
		return writeInPlace(target, data)
	}
	if err := writeReplacing(target, data, fi); !errors.Is(err, errCannotReplace) {
		return err
	}
	return writeInPlace(target, data)
}

// writeReplacing writes data to a temporary file beside target, with
// target's mode and owner, and renames it over target. Until the rename the
// file is untouched, so a failure before it - a full disk - leaves the file
// as it was; errCannotReplace says the file should be written in place.
func writeReplacing(target string, data []byte, fi fs.FileInfo) error {
	dir, base := filepath.Split(target)
	f, err := os.CreateTemp(dir, "."+base+".nem-*")
	if err != nil {
		return errCannotReplace
	}
	tmp := f.Name()
	// Removes the temporary file on every failure below. After the rename
	// the name is gone, and this does nothing.
	defer os.Remove(tmp)

	if err := keepOwner(f, fi); err != nil {
		f.Close()
		return errCannotReplace
	}
	if err := writeData(f, data); err != nil {
		f.Close()
		return err
	}
	// Flushed before the rename: a rename that reached the disk before the
	// data did would leave an empty file after a power cut.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	// After the owner, which clears the set-id bits.
	if err := f.Chmod(fi.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)); err != nil {
		f.Close()
		return errCannotReplace
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return errCannotReplace
	}
	syncDir(dir)
	return nil
}

// writeInPlace writes data into target itself: emptying it, then filling it.
func writeInPlace(target string, data []byte) error {
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	return finish(f, data)
}

// writeNew creates target with data. There is nothing there to lose, so it
// is written directly.
func writeNew(target string, data []byte) error {
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	return finish(f, data)
}

// finish writes data into f, flushes it and closes it. Only a regular file
// is flushed: a terminal or a pipe has nothing to flush, and says so.
func finish(f *os.File, data []byte) error {
	if err := writeData(f, data); err != nil {
		f.Close()
		return err
	}
	if fi, err := f.Stat(); err == nil && fi.Mode().IsRegular() {
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// syncDir flushes dir, so that a rename in it survives a power cut. Best
// effort: the data is on the disk already, and not every system lets a
// directory be flushed.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}
