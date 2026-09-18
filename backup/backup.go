// Package backup keeps crash-recovery copies of edited files outside the
// directories being edited.
//
// Emacs writes backups as file~ and autosaves as #file# beside the original.
// That clutters a working tree — every backup shows up as an untracked file in
// git status — so nem keeps everything in one state directory instead, with each
// file's absolute path mirrored beneath it:
//
//	/home/reza/p/main.go
//	  -> ~/.local/state/nem/backups/home/reza/p/main.go~
//	  -> ~/.local/state/nem/autosave/home/reza/p/main.go#
//
// Mirroring the whole path rather than flattening it keeps the store browsable
// and means two files with the same base name in different projects cannot
// overwrite each other's backups.
//
// The package's central invariant is that nothing is ever written outside the
// root. That is what keeps backups out of the user's tree, so it is enforced
// structurally — paths are mirrored from the cleaned absolute path, which cannot
// contain ".." — and checked again before any write.
package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrEscapesRoot reports a mirrored path that fell outside the store's root.
// Reaching it means the mirroring is broken; it exists so that a bug shows up as
// a refused write rather than as a file written into the user's project.
var ErrEscapesRoot = errors.New("backup: mirrored path escapes the store root")

const (
	backupsDir  = "backups"
	autosaveDir = "autosave"

	backupSuffix   = "~"
	autosaveSuffix = "#"

	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// DefaultRoot returns the state directory nem keeps its backups in:
// $XDG_STATE_HOME/nem, or ~/.local/state/nem when that is unset.
//
// A relative XDG_STATE_HOME is ignored, as the XDG base directory specification
// requires — resolving it against the working directory would scatter state
// wherever nem happened to be started from.
func DefaultRoot() (string, error) {
	if x := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "nem"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("backup: locating the home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "nem"), nil
}

// Store holds backups and autosaves beneath a single root directory.
//
// A Store is cheap to construct and creates nothing; directories appear on the
// first write. It holds no mutable state, so it is safe to share.
type Store struct {
	root string
}

// New returns a Store rooted at root. The directory need not exist yet.
func New(root string) *Store { return &Store{root: filepath.Clean(root)} }

// Root returns the directory the store writes beneath.
func (s *Store) Root() string { return s.root }

// BackupPath returns where file's backup is kept.
func (s *Store) BackupPath(file string) string {
	return s.mirror(backupsDir, file, backupSuffix)
}

// AutosavePath returns where file's autosave is kept.
func (s *Store) AutosavePath(file string) string {
	return s.mirror(autosaveDir, file, autosaveSuffix)
}

// mirror maps a file path to its place in the store.
//
// The absolute, cleaned path is what makes this safe: filepath.Abs cleans, and a
// cleaned absolute path cannot contain "..", because Clean resolves every one of
// them and cannot climb above the root. Mirroring the raw input instead would let
// "../../etc/passwd" join to root/../etc/passwd and escape — which is the one
// thing this package must never do.
//
// A path that resolves to the filesystem root leaves nothing to mirror, so the
// suffix alone becomes the file name. Nobody edits "/", but the result still has
// to land inside the store rather than beside it.
func (s *Store) mirror(subdir, file, suffix string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		// Abs fails only when the working directory cannot be determined. Clean
		// the input and carry on: the result is still mirrored beneath the root,
		// which is the property that matters.
		abs = filepath.Clean(file)
	}
	rel := strings.TrimPrefix(abs, string(filepath.Separator))
	return filepath.Join(s.root, subdir, rel+suffix)
}

// contains reports whether p lies strictly beneath the store's root. Equality
// with the root counts as outside: the root is never a valid file location.
func (s *Store) contains(p string) bool {
	rel, err := filepath.Rel(s.root, p)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// guard resolves a mirrored path and refuses one that escaped the root.
func (s *Store) guard(subdir, file, suffix string) (string, error) {
	p := s.mirror(subdir, file, suffix)
	if !s.contains(p) {
		return "", fmt.Errorf("%w: %s", ErrEscapesRoot, p)
	}
	return p, nil
}

// WriteBackup stores a copy of file's previous contents.
func (s *Store) WriteBackup(file string, content []byte) error {
	p, err := s.guard(backupsDir, file, backupSuffix)
	if err != nil {
		return err
	}
	return writeAtomic(p, content)
}

// WriteAutosave stores the current, unsaved contents of file.
func (s *Store) WriteAutosave(file string, content []byte) error {
	p, err := s.guard(autosaveDir, file, autosaveSuffix)
	if err != nil {
		return err
	}
	return writeAtomic(p, content)
}

// ReadAutosave returns file's autosaved contents. A missing autosave wraps
// fs.ErrNotExist, so callers can distinguish it with errors.Is.
func (s *Store) ReadAutosave(file string) ([]byte, error) {
	p, err := s.guard(autosaveDir, file, autosaveSuffix)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("backup: reading autosave %s: %w", p, err)
	}
	return content, nil
}

// RemoveAutosave discards file's autosave.
//
// A missing autosave is not an error: this is called after every successful
// save, and most saves have no autosave to discard. Reporting one would turn an
// ordinary save into a failure in the echo area.
func (s *Store) RemoveAutosave(file string) error {
	p, err := s.guard(autosaveDir, file, autosaveSuffix)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: removing autosave %s: %w", p, err)
	}
	return nil
}

// AutosaveNewer reports whether file has an autosave holding work the saved file
// does not, along with the autosave's modification time.
//
// A missing autosave is (false, zero, nil). An autosave whose file has since
// been deleted counts as newer: that is the case where recovery matters most.
func (s *Store) AutosaveNewer(file string) (bool, time.Time, error) {
	p, err := s.guard(autosaveDir, file, autosaveSuffix)
	if err != nil {
		return false, time.Time{}, err
	}

	as, err := os.Stat(p)
	if os.IsNotExist(err) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("backup: inspecting autosave %s: %w", p, err)
	}

	fi, err := os.Stat(file)
	if os.IsNotExist(err) {
		return true, as.ModTime(), nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("backup: inspecting %s: %w", file, err)
	}
	return as.ModTime().After(fi.ModTime()), as.ModTime(), nil
}

// writeAtomic writes content to p via a temporary file in the same directory and
// a rename, so a crash part-way through cannot leave a truncated file where a
// good one used to be. Surviving a crash is the point of this package, so the
// write path has to survive one too.
func writeAtomic(p string, content []byte) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("backup: creating %s: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, ".nem-*")
	if err != nil {
		return fmt.Errorf("backup: creating a temporary file in %s: %w", dir, err)
	}
	tmp := f.Name()
	// Cleans up every failure path below. After a successful rename the
	// temporary name no longer exists, so this becomes a harmless no-op.
	defer os.Remove(tmp)

	if _, err := f.Write(content); err != nil {
		f.Close()
		return fmt.Errorf("backup: writing %s: %w", tmp, err)
	}
	// Flush before renaming. A rename that became visible before the data did
	// would leave an empty backup after a power loss, which is worse than none.
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("backup: syncing %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: closing %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, fileMode); err != nil {
		return fmt.Errorf("backup: setting the mode of %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("backup: renaming %s to %s: %w", tmp, p, err)
	}
	syncDir(dir)
	return nil
}

// syncDir flushes a directory entry so the rename itself survives a crash.
//
// Best effort by design: the file's contents are already durable, and some
// filesystems refuse to sync a directory at all. Failing the write here would
// turn a successful autosave into a reported error for a guarantee the caller
// did not ask for.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
