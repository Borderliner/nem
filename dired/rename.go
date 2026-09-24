package dired

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// Rename is one file's new name, for RenameAll. Both are relative to the
// directory RenameAll is given, or absolute.
type Rename struct{ From, To string }

// RenameAll renames files as if all at the same instant, which is what editing
// a listing's names as text asks for: a name one file gives up can be taken by
// another, so two files can swap names, or a numbered run shift along by one.
//
// Nothing is overwritten, and nothing moves until every rename has been
// checked: no two may take one name, a name may be taken only if one of the
// renames frees it, and a file may go only into a directory that exists and
// is not itself being renamed. Then each file goes to a temporary name beside
// it, and from there to its new one. If a step fails, everything already moved
// is moved back, so either all of it happens or none of it does - short of
// the moving back failing too, which the error then says.
//
// Errors name the files as the renames give them, since they are meant for the
// user who typed those names.
func RenameAll(dir string, rs []Rename) error {
	abs := func(p string) string {
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Join(dir, p)
	}
	from := make([]string, len(rs))
	to := make([]string, len(rs))
	for i, r := range rs {
		from[i], to[i] = abs(r.From), abs(r.To)
	}
	if err := checkRenames(rs, from, to); err != nil {
		return err
	}

	tmp := make([]string, len(rs))
	// back undoes the first n moves to a temporary name and the first m on to
	// the new one, newest first, and says what could not be put back.
	back := func(n, m int) error {
		var stuck []error
		for i := m - 1; i >= 0; i-- {
			if err := Move(to[i], tmp[i]); err != nil {
				stuck = append(stuck, fmt.Errorf("%s is left as %s", rs[i].From, rs[i].To))
				tmp[i] = "" // not there to go back from
			}
		}
		for i := n - 1; i >= 0; i-- {
			if tmp[i] == "" {
				continue
			}
			if err := os.Rename(tmp[i], from[i]); err != nil {
				stuck = append(stuck, fmt.Errorf("%s is left as %s", rs[i].From, tmp[i]))
			}
		}
		return errors.Join(stuck...)
	}
	failed := func(i int, err error, n, m int) error {
		var le *os.LinkError
		if errors.As(err, &le) {
			err = le.Err
		}
		msg := fmt.Sprintf("renaming %s to %s: %v", rs[i].From, rs[i].To, err)
		if stuck := back(n, m); stuck != nil {
			return fmt.Errorf("%s; and putting the others back failed: %w", msg, stuck)
		}
		return errors.New(msg + "; nothing was renamed")
	}

	for i := range rs {
		t, err := freeName(filepath.Dir(from[i]))
		if err == nil {
			err = os.Rename(from[i], t)
		}
		if err != nil {
			return failed(i, err, i, 0)
		}
		tmp[i] = t
	}
	for i := range rs {
		if err := Move(tmp[i], to[i]); err != nil {
			return failed(i, err, len(rs), i)
		}
	}
	return nil
}

// checkRenames is RenameAll's look before it leaps. from and to are the
// renames' paths made absolute.
func checkRenames(rs []Rename, from, to []string) error {
	infos := make([]fs.FileInfo, len(rs))
	for i := range rs {
		fi, err := os.Lstat(from[i])
		if err != nil {
			return fmt.Errorf("%s: %w", rs[i].From, underlying(err))
		}
		infos[i] = fi
	}
	// frees reports whether a path names one of the files being renamed, and
	// so will be free by the time anything takes it: exactly, or in a
	// different case on a filesystem that ignores case.
	frees := func(fi fs.FileInfo) bool {
		for _, f := range infos {
			if os.SameFile(f, fi) {
				return true
			}
		}
		return false
	}

	taken := make(map[string]int, len(rs))
	for i := range rs {
		if j, dup := taken[to[i]]; dup {
			return fmt.Errorf("%s and %s cannot both be named %s", rs[j].From, rs[i].From, rs[i].To)
		}
		taken[to[i]] = i

		parent := filepath.Dir(to[i])
		for j := range rs {
			if !under(from[j], parent) {
				continue
			}
			if j == i {
				return fmt.Errorf("%s cannot go inside itself", rs[i].From)
			}
			return fmt.Errorf("%s cannot go into %s, which is being renamed too", rs[i].From, rs[j].From)
		}
		if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
			return fmt.Errorf("there is no directory %s to put %s in", filepath.Dir(rs[i].To), rs[i].From)
		}
		switch fi, err := os.Lstat(to[i]); {
		case err == nil:
			if !frees(fi) {
				return fmt.Errorf("%s already exists", rs[i].To)
			}
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("%s: %w", rs[i].To, underlying(err))
		}
	}
	return nil
}

// freeName is a name in dir that nothing has, for a file to wait under while
// the others move.
func freeName(dir string) (string, error) {
	for n := 0; n < 1000; n++ {
		p := filepath.Join(dir, ".nem-rename-"+strconv.Itoa(os.Getpid())+"-"+strconv.Itoa(n))
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
	}
	return "", fmt.Errorf("no free temporary name in %s", dir)
}

// underlying strips the path from an error about a path, for a message that
// names the file its own way.
func underlying(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
