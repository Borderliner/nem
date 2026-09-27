//go:build unix

package text

import (
	"io/fs"
	"os"
	"syscall"
)

// replaceable reports whether a file can be replaced by renaming another
// over it without changing what it is: it has no other name linked to it,
// and nem can give the new file its owner - it owns it already, or is root.
func replaceable(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	if st.Nlink > 1 {
		return false
	}
	uid := os.Geteuid()
	return uid == 0 || int(st.Uid) == uid
}

// keepOwner gives f the owner and group of the file it is to replace. Its
// group may be one nem cannot give it, and then the file is written in
// place instead.
func keepOwner(f *os.File, fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errCannotReplace
	}
	mine, err := f.Stat()
	if err != nil {
		return err
	}
	if cur, ok := mine.Sys().(*syscall.Stat_t); ok && cur.Uid == st.Uid && cur.Gid == st.Gid {
		return nil
	}
	return f.Chown(int(st.Uid), int(st.Gid))
}
