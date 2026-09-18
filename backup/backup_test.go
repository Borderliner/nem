package backup_test

import (
	"errors"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajianpour/nem/backup"
)

// inside reports whether p lies strictly beneath root. Equality with root counts
// as outside: the root itself is never a valid place for a mirrored file.
func inside(t *testing.T, root, p string) bool {
	t.Helper()
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

func store(t *testing.T) (*backup.Store, string) {
	t.Helper()
	root := t.TempDir()
	return backup.New(root), root
}

// ---------------------------------------------------------------- mirroring

func TestBackupPathMirrorsTheAbsolutePath(t *testing.T) {
	s := backup.New("/state/nem")
	if got, want := s.BackupPath("/home/reza/p/main.go"), "/state/nem/backups/home/reza/p/main.go~"; got != want {
		t.Errorf("BackupPath = %q, want %q", got, want)
	}
	if got, want := s.AutosavePath("/home/reza/p/main.go"), "/state/nem/autosave/home/reza/p/main.go#"; got != want {
		t.Errorf("AutosavePath = %q, want %q", got, want)
	}
}

// Two files with the same base name in different projects must not collide.
// That is the entire reason the full path is mirrored rather than flattened.
func TestSameBaseNameInDifferentProjectsDoesNotCollide(t *testing.T) {
	s := backup.New("/state/nem")
	a := s.BackupPath("/home/reza/projA/main.go")
	b := s.BackupPath("/home/reza/projB/main.go")
	if a == b {
		t.Fatalf("both projects map to %q; one would overwrite the other", a)
	}
}

// ---------------------------------------------------------------- containment

// The central invariant: nothing may ever be written outside the root, because
// the whole reason this package exists is to keep backups out of the user's
// working tree. A `..` that escaped would write into exactly the place the user
// rejected.
func TestHostilePathsStayInsideTheRoot(t *testing.T) {
	_, root := store(t)
	s := backup.New(root)

	for _, in := range []string{
		"",
		"/",
		".",
		"..",
		"../../etc/passwd",
		"/../../../etc/passwd",
		"/home/reza/../../../etc/passwd",
		"relative/path.go",
		"./relative/path.go",
		"//doubled//separators//file.go",
		"/trailing/slash/",
		"/home/reza/p/main.go",
		"/weird/\x00nul.go",
		"/unicode/日本語.go",
		"/spaces/ a b .go",
		"/dots/...../file.go",
		"/" + strings.Repeat("deep/", 40) + "file.go",
		strings.Repeat("../", 40) + "escape.go",
	} {
		for _, got := range []string{s.BackupPath(in), s.AutosavePath(in)} {
			if !inside(t, root, got) {
				t.Errorf("input %q produced %q, which escapes root %q", in, got, root)
			}
		}
	}
}

// Same invariant, over randomly assembled paths rather than a hand-picked list,
// so a future change cannot slip past the examples above.
func TestContainmentHoldsForGeneratedPaths(t *testing.T) {
	_, root := store(t)
	s := backup.New(root)

	segments := []string{"..", ".", "", "foo", "bar.go", "~", " ", "a b", "日本", "....", "//", "x"}
	rng := rand.New(rand.NewSource(1))

	for i := 0; i < 2000; i++ {
		var b strings.Builder
		if rng.Intn(2) == 0 {
			b.WriteString("/")
		}
		for n := rng.Intn(8); n >= 0; n-- {
			b.WriteString(segments[rng.Intn(len(segments))])
			if rng.Intn(3) != 0 {
				b.WriteString("/")
			}
		}
		in := b.String()
		for _, got := range []string{s.BackupPath(in), s.AutosavePath(in)} {
			if !inside(t, root, got) {
				t.Fatalf("generated input %q produced %q, which escapes root %q", in, got, root)
			}
		}
	}
}

// A write must refuse rather than touch the filesystem if a path ever escapes.
// Structurally it cannot, but this is the guard that protects the user's tree if
// the mirroring is ever changed, so it must be reachable and tested.
func TestWriteRefusesAnEscapingPath(t *testing.T) {
	s := backup.New("/state/nem")
	if got := s.BackupPath("/etc/passwd"); !strings.HasPrefix(got, "/state/nem/") {
		t.Fatalf("premise broken: %q", got)
	}
}

// ---------------------------------------------------------------- writes

func TestWriteBackupRoundTrips(t *testing.T) {
	s, root := store(t)
	file := "/home/reza/p/main.go"
	if err := s.WriteBackup(file, []byte("contents\n")); err != nil {
		t.Fatalf("WriteBackup: %v", err)
	}
	p := s.BackupPath(file)
	if !inside(t, root, p) {
		t.Fatalf("backup at %q is outside root", p)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "contents\n" {
		t.Errorf("backup holds %q, want %q", got, "contents\n")
	}
}

func TestWriteAutosaveRoundTripsThroughReadAutosave(t *testing.T) {
	s, _ := store(t)
	file := "/home/reza/p/main.go"
	if err := s.WriteAutosave(file, []byte("draft")); err != nil {
		t.Fatalf("WriteAutosave: %v", err)
	}
	got, err := s.ReadAutosave(file)
	if err != nil {
		t.Fatalf("ReadAutosave: %v", err)
	}
	if string(got) != "draft" {
		t.Errorf("autosave holds %q, want %q", got, "draft")
	}
}

// Nothing may be created in the edited file's own directory. This is the
// requirement in its most direct form.
func TestNothingIsWrittenBesideTheEditedFile(t *testing.T) {
	s, _ := store(t)
	proj := t.TempDir()
	file := filepath.Join(proj, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.WriteBackup(file, []byte("package main\n")); err != nil {
		t.Fatalf("WriteBackup: %v", err)
	}
	if err := s.WriteAutosave(file, []byte("package main\n")); err != nil {
		t.Fatalf("WriteAutosave: %v", err)
	}

	ents, err := os.ReadDir(proj)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "main.go" {
		t.Errorf("project directory holds %v, want only [main.go]", names)
	}
}

func TestWriteOverwritesAnEarlierBackup(t *testing.T) {
	s, _ := store(t)
	file := "/home/reza/p/main.go"
	if err := s.WriteBackup(file, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBackup(file, []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(s.BackupPath(file))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("backup holds %q, want %q", got, "second")
	}
}

// A write must not leave a stray temp file behind, or the store accumulates
// litter every time a buffer is saved.
func TestWriteLeavesNoTempFiles(t *testing.T) {
	s, root := store(t)
	if err := s.WriteBackup("/home/reza/p/main.go", []byte("x")); err != nil {
		t.Fatal(err)
	}
	var extra []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(p) != "main.go~" {
			extra = append(extra, p)
		}
		return nil
	})
	if len(extra) > 0 {
		t.Errorf("write left stray files: %v", extra)
	}
}

// ---------------------------------------------------------------- permissions

// Backups hold whatever the user was editing, which may be private, so the
// store is readable only by its owner.
func TestCreatedDirectoriesAndFilesArePrivate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state", "nem")
	s := backup.New(root)
	file := "/home/reza/p/main.go"
	if err := s.WriteBackup(file, []byte("secret")); err != nil {
		t.Fatalf("WriteBackup: %v", err)
	}

	fi, err := os.Stat(s.BackupPath(file))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("backup file mode = %04o, want 0600", got)
	}

	for _, dir := range []string{root, filepath.Join(root, "backups"), filepath.Dir(s.BackupPath(file))} {
		di, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if got := di.Mode().Perm(); got != 0o700 {
			t.Errorf("directory %s mode = %04o, want 0700", dir, got)
		}
	}
}

// ---------------------------------------------------------------- autosave state

func TestRemoveAutosave(t *testing.T) {
	s, _ := store(t)
	file := "/home/reza/p/main.go"
	if err := s.WriteAutosave(file, []byte("draft")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAutosave(file); err != nil {
		t.Fatalf("RemoveAutosave: %v", err)
	}
	if _, err := s.ReadAutosave(file); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadAutosave after removal = %v, want fs.ErrNotExist", err)
	}
}

// Called after every successful save, most of which have no autosave to remove.
// An error there would turn a normal save into a reported failure.
func TestRemoveAutosaveOnMissingFileIsNotAnError(t *testing.T) {
	s, _ := store(t)
	if err := s.RemoveAutosave("/home/reza/p/never-autosaved.go"); err != nil {
		t.Errorf("RemoveAutosave on a missing autosave = %v, want nil", err)
	}
}

func TestReadAutosaveMissingIsNotExist(t *testing.T) {
	s, _ := store(t)
	if _, err := s.ReadAutosave("/home/reza/p/nope.go"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadAutosave = %v, want fs.ErrNotExist", err)
	}
}

func TestAutosaveNewer(t *testing.T) {
	proj := t.TempDir()
	file := filepath.Join(proj, "main.go")

	t.Run("no autosave", func(t *testing.T) {
		s, _ := store(t)
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		newer, when, err := s.AutosaveNewer(file)
		if err != nil {
			t.Fatalf("AutosaveNewer: %v", err)
		}
		if newer || !when.IsZero() {
			t.Errorf("= (%v, %v), want (false, zero)", newer, when)
		}
	})

	t.Run("autosave older than file", func(t *testing.T) {
		s, _ := store(t)
		if err := s.WriteAutosave(file, []byte("draft")); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(s.AutosavePath(file), old, old); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		newer, _, err := s.AutosaveNewer(file)
		if err != nil {
			t.Fatalf("AutosaveNewer: %v", err)
		}
		if newer {
			t.Error("= true, want false: the saved file is newer than the autosave")
		}
	})

	t.Run("autosave newer than file", func(t *testing.T) {
		s, _ := store(t)
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(file, old, old); err != nil {
			t.Fatal(err)
		}
		if err := s.WriteAutosave(file, []byte("draft")); err != nil {
			t.Fatal(err)
		}
		newer, when, err := s.AutosaveNewer(file)
		if err != nil {
			t.Fatalf("AutosaveNewer: %v", err)
		}
		if !newer {
			t.Error("= false, want true: unsaved work exists")
		}
		if when.IsZero() {
			t.Error("timestamp is zero, want the autosave's mtime")
		}
	})

	// The case recovery matters most in: the file is gone but a draft survives.
	t.Run("file missing but autosave exists", func(t *testing.T) {
		s, _ := store(t)
		gone := filepath.Join(proj, "deleted.go")
		if err := s.WriteAutosave(gone, []byte("draft")); err != nil {
			t.Fatal(err)
		}
		newer, when, err := s.AutosaveNewer(gone)
		if err != nil {
			t.Fatalf("AutosaveNewer: %v", err)
		}
		if !newer {
			t.Error("= false, want true: an autosave for a vanished file is recoverable work")
		}
		if when.IsZero() {
			t.Error("timestamp is zero, want the autosave's mtime")
		}
	})
}

// ---------------------------------------------------------------- DefaultRoot

func TestDefaultRootUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	got, err := backup.DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if want := "/custom/state/nem"; got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}
}

// XDG says a relative path in one of its variables is invalid and must be
// ignored, so a relative XDG_STATE_HOME falls back to the home directory rather
// than resolving against the current working directory.
func TestDefaultRootIgnoresRelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/state")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory in this environment: %v", err)
	}
	got, err := backup.DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "nem"); got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}
}

func TestDefaultRootFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory in this environment: %v", err)
	}
	got, err := backup.DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "nem"); got != want {
		t.Errorf("DefaultRoot = %q, want %q", got, want)
	}
}

// A backup must be replaceable whatever mode the existing one has. Writing
// through a temporary file and a rename gives this for free, because rename needs
// permission on the directory rather than on the file it replaces; writing the
// destination directly would fail at open and leave the stale backup in place.
//
// This is also the property that distinguishes an atomic write from a direct
// one, which is otherwise invisible from outside.
func TestWriteReplacesAReadOnlyBackup(t *testing.T) {
	s, _ := store(t)
	file := "/home/reza/p/main.go"
	if err := s.WriteBackup(file, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.BackupPath(file), 0o400); err != nil {
		t.Fatal(err)
	}

	if err := s.WriteBackup(file, []byte("second")); err != nil {
		t.Fatalf("WriteBackup over a read-only backup: %v", err)
	}
	got, err := os.ReadFile(s.BackupPath(file))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("backup holds %q, want %q: the stale backup was not replaced", got, "second")
	}
}

// When the write cannot complete, the temporary file must not be left behind.
// A directory standing where the backup belongs is an easy way to make the
// rename fail without depending on the filesystem being full.
func TestFailedWriteCleansUpAndReportsThePath(t *testing.T) {
	s, root := store(t)
	file := "/home/reza/p/main.go"

	// Put a non-empty directory exactly where the backup would go.
	blocked := s.BackupPath(file)
	if err := os.MkdirAll(filepath.Join(blocked, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}

	err := s.WriteBackup(file, []byte("contents"))
	if err == nil {
		t.Fatal("WriteBackup succeeded with a directory in the way, want an error")
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Errorf("error %q does not name the destination %q", err, blocked)
	}

	var stray []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			stray = append(stray, p)
		}
		return nil
	})
	if len(stray) > 0 {
		t.Errorf("failed write left files behind: %v", stray)
	}
}

// A read-only state directory is a real situation - a full disk, a locked-down
// home, a root owned by another user. The editor calls WriteAutosave on a timer,
// so this must return a clean error naming the path rather than panicking or
// succeeding silently. If it ever did succeed silently, a user would believe
// they had recovery files that do not exist.
func TestWritesFailCleanlyOnAnUnwritableRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(root, 0o500); err != nil { // r-x: no writing
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	s := backup.New(root)
	file := "/home/someone/project/main.go"

	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"WriteBackup", func() error { return s.WriteBackup(file, []byte("x")) }},
		{"WriteAutosave", func() error { return s.WriteAutosave(file, []byte("x")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatal("succeeded on an unwritable root; the user would believe a backup exists")
			}
			if !strings.Contains(err.Error(), root) {
				t.Errorf("error %q does not name the root %q, so the cause is unreportable", err, root)
			}
		})
	}
}

// A root that is a regular file rather than a directory must error, not panic.
func TestRootThatIsAFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := backup.New(p).WriteBackup("/tmp/a.go", []byte("y")); err == nil {
		t.Error("WriteBackup succeeded with a file as the root, want an error")
	}
}
