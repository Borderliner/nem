package command

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// Paths as typed at a prompt, as emacs reads them: ~ is the home directory,
// and a prompt that starts in one directory can be taken to another without
// erasing it, by typing ~/ or / after what is there.

// RestartPath is what a path typed at a prompt means once ~/ or // follows a
// directory in it: the path starts over there, at home or at the root. So
// find-file, which opens on the current directory, reaches a file in the
// home directory by typing ~/ straight after it, as emacs's minibuffer does.
func RestartPath(p string) string {
	home := strings.LastIndex(p, "/~/")
	root := strings.LastIndex(p, "//")
	switch {
	case home < 0 && root < 0:
		return p
	case home > root:
		return p[home+1:]
	}
	return p[root+1:]
}

// ExpandPath turns a path typed at a prompt into one the filesystem knows:
// restarted as RestartPath says, then ~ read as the home directory and ~name
// as that user's. A ~ nothing can be made of is left alone - a file may be
// called that.
func ExpandPath(p string) string {
	p = RestartPath(p)
	if !strings.HasPrefix(p, "~") {
		return p
	}
	name, rest, _ := strings.Cut(p[1:], "/")
	var dir string
	if name == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		dir = h
	} else {
		u, err := user.Lookup(name)
		if err != nil {
			return p
		}
		dir = u.HomeDir
	}
	if rest == "" && !strings.HasSuffix(p, "/") {
		return dir
	}
	return filepath.Join(dir, rest) + trailingSep(rest)
}

// trailingSep keeps the separator that marks a directory, which Join drops.
func trailingSep(rest string) string {
	if rest == "" || strings.HasSuffix(rest, "/") {
		return string(filepath.Separator)
	}
	return ""
}
