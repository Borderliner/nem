//go:build !unix

package text

import (
	"io/fs"
	"os"
)

// replaceable reports whether a file can be replaced by renaming another
// over it. Outside Unix there is no owner to keep, and no link count to see.
func replaceable(fs.FileInfo) bool { return true }

// keepOwner does nothing outside Unix, where a new file takes its access
// from the directory it is made in.
func keepOwner(*os.File, fs.FileInfo) error { return nil }
