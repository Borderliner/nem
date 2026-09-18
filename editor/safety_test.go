package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/backup"
	"github.com/hajianpour/nem/text"
)

// safeEditor returns an editor whose backup store is a temporary directory, plus
// that directory and a file path inside a separate temporary directory.
//
// The two directories are deliberately separate: it is what lets a test assert
// that nothing was written beside the edited file.
func safeEditor(t *testing.T) (*Editor, *backup.Store, string) {
	t.Helper()
	e, _ := newTestEditor(t)
	root := t.TempDir()
	e.SetBackupRoot(root)
	file := filepath.Join(t.TempDir(), "main.go")
	return e, backup.New(root), file
}

// visit opens path and makes it the active buffer.
func visit(t *testing.T, e *Editor, path string) *text.Buffer {
	t.Helper()
	b, err := e.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", path, err)
	}
	e.Active().Visit(b)
	return b
}

// edit appends text to a buffer, leaving it modified.
func edit(t *testing.T, b *text.Buffer, s string) {
	t.Helper()
	if err := b.Insert(b.End(), []rune(s)); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

// feedAnswer injects a one-rune reply for the next ReadChar prompt. ReadChar
// polls for events, so the answer has to arrive from another goroutine — which is
// what feed does.
func feedAnswer(t *testing.T, e *Editor, s string) {
	t.Helper()
	scr, ok := e.scr.(tcell.SimulationScreen)
	if !ok {
		t.Fatal("editor is not running on a simulation screen")
	}
	feed(t, scr, txt(s))
}

// age pushes a file's modification time into the past, so an autosave written
// afterwards is unambiguously newer than the file.
func age(t *testing.T, path string, d time.Duration) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	when := fi.ModTime().Add(-d)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatalf("Chtimes(%q): %v", path, err)
	}
}

// --- backups -------------------------------------------------------------

// The backup must hold what was on disk BEFORE the save. A backup taken after
// the write holds the new contents and preserves nothing at all — it is the one
// ordering mistake that makes the whole feature useless while looking like it
// works.
func TestBackupHoldsTheContentsFromBeforeTheSave(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	edit(t, b, "replacement")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	got, err := os.ReadFile(store.BackupPath(file))
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	if string(got) != "original\n" {
		t.Errorf("backup = %q, want the pre-save contents %q", got, "original\n")
	}
}

// Backups are once per session, so a long editing session preserves the file as
// it was when you opened it rather than as it was one save ago.
func TestBackupIsTakenOncePerSession(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	for _, add := range []string{"second", "third"} {
		edit(t, b, add)
		if err := e.SaveBuffer(b, ""); err != nil {
			t.Fatalf("SaveBuffer: %v", err)
		}
	}

	got, err := os.ReadFile(store.BackupPath(file))
	if err != nil {
		t.Fatalf("reading the backup: %v", err)
	}
	if string(got) != "first\n" {
		t.Errorf("backup = %q, want the session's original contents %q", got, "first\n")
	}
}

// A file that does not exist yet has nothing to preserve, and saving it must not
// fail or invent an empty backup.
func TestNoBackupForAFileThatDidNotExist(t *testing.T) {
	e, store, file := safeEditor(t)

	b := visit(t, e, file)
	edit(t, b, "brand new")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	if _, err := os.Stat(store.BackupPath(file)); !os.IsNotExist(err) {
		t.Errorf("a backup exists for a file that had no previous contents (err = %v)", err)
	}
}

// The reason this whole package exists. Nothing may appear beside the edited
// file: a file~ in a working tree shows up as an untracked file in git status,
// which is precisely what the central store avoids.
func TestNothingIsWrittenBesideTheEditedFile(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	edit(t, b, "more")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if err := e.RunAutosave(time.Now()); err != nil {
		t.Fatalf("RunAutosave: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range entries {
		names = append(names, d.Name())
	}
	if len(names) != 1 || names[0] != "main.go" {
		t.Errorf("directory holds %v, want only main.go", names)
	}
}

// --- external changes ----------------------------------------------------

// The worst failure available to this code is silently discarding someone else's
// edit. Answering no must leave the file byte-identical and the buffer still
// modified, so nothing is lost on either side.
func TestRefusedOverwriteLeavesTheFileByteIdentical(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	edit(t, b, "mine")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("first SaveBuffer: %v", err)
	}

	// Something else rewrites the file, then we try to save over it.
	const theirs = "written by someone else\n"
	if err := os.WriteFile(file, []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	edit(t, b, "more of mine")

	feedAnswer(t, e, "n")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer after an external change: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != theirs {
		t.Errorf("file = %q, want it untouched at %q", got, theirs)
	}
	if !b.Modified() {
		t.Error("buffer was marked clean by a refused save; the user would think it was written")
	}
}

// Answering yes goes ahead, because the user has been told and decided.
func TestAcceptedOverwriteWrites(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	edit(t, b, "mine")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("first SaveBuffer: %v", err)
	}

	if err := os.WriteFile(file, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	feedAnswer(t, e, "y")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "theirs") {
		t.Errorf("file = %q, want nem's contents after an accepted overwrite", got)
	}
	if b.Modified() {
		t.Error("buffer still modified after an accepted save")
	}
}

// A file nem has never read has no recorded fingerprint, so there is nothing to
// compare and no reason to interrogate the user.
func TestUnknownFileIsNotTreatedAsChanged(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("not opened by nem\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b := e.Buf() // the scratch buffer, never associated with this path
	edit(t, b, "content")
	// No answer is fed: a prompt here would block the test, which is the point.
	if err := e.SaveBuffer(b, file); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if got, _ := os.ReadFile(file); !strings.Contains(string(got), "content") {
		t.Errorf("file = %q, want nem's contents", got)
	}
}

// --- autosave ------------------------------------------------------------

func TestAutosaveWritesModifiedBuffers(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	edit(t, b, "unsaved work")

	if err := e.RunAutosave(time.Now()); err != nil {
		t.Fatalf("RunAutosave: %v", err)
	}
	got, err := os.ReadFile(store.AutosavePath(file))
	if err != nil {
		t.Fatalf("reading the autosave: %v", err)
	}
	if !strings.Contains(string(got), "unsaved work") {
		t.Errorf("autosave = %q, want it to hold the unsaved text", got)
	}
}

func TestAutosaveDueRequiresIdleAndModification(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	t0 := time.Now()

	e.NoteInput(t0)
	if e.AutosaveDue(t0.Add(31 * time.Second)) {
		t.Error("due with no modified buffer")
	}

	edit(t, b, "x")
	if e.AutosaveDue(t0.Add(5 * time.Second)) {
		t.Error("due before the idle interval elapsed")
	}
	if !e.AutosaveDue(t0.Add(31 * time.Second)) {
		t.Fatal("not due after the idle interval with a modified buffer")
	}

	// Having run once, an editor left alone must not rewrite the same autosave
	// on every pass through the loop.
	if err := e.RunAutosave(t0.Add(31 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if e.AutosaveDue(t0.Add(120 * time.Second)) {
		t.Error("due again with no input since the last autosave")
	}

	// New input restarts the clock.
	e.NoteInput(t0.Add(200 * time.Second))
	if !e.AutosaveDue(t0.Add(240 * time.Second)) {
		t.Error("not due after fresh input and another idle interval")
	}
}

func TestAutosaveDisabledAtZero(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	edit(t, b, "x")
	e.SetAutosaveIdle(0)

	t0 := time.Now()
	e.NoteInput(t0)
	if e.AutosaveDue(t0.Add(time.Hour)) {
		t.Error("autosave due although the interval is zero")
	}
}

// A stale autosave would offer to recover work that is already on disk.
func TestSuccessfulSaveRemovesTheAutosave(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	edit(t, b, "work")
	if err := e.RunAutosave(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.AutosavePath(file)); err != nil {
		t.Fatalf("setup: autosave missing: %v", err)
	}

	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if _, err := os.Stat(store.AutosavePath(file)); !os.IsNotExist(err) {
		t.Errorf("autosave survived a successful save (err = %v)", err)
	}
}

// A failed autosave must be visible. A user who believes they have recovery
// files and does not is worse off than one who knows they have none.
func TestAutosaveFailureIsReportedNotSwallowed(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("saved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	edit(t, b, "work")

	// A root that cannot be written to at all.
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	e.SetBackupRoot(locked)

	if err := e.RunAutosave(time.Now()); err == nil {
		t.Fatal("RunAutosave reported success against an unwritable root")
	}
	wantEcho(t, e, "autosave failed")
}

// A broken store must not stop the user saving their actual file.
func TestBackupFailureDoesNotBlockTheSave(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	edit(t, b, "important")

	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	e.SetBackupRoot(locked)

	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer refused because a backup failed: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "important") {
		t.Errorf("file = %q, want the user's work saved despite the backup failure", got)
	}
}

// --- recovery ------------------------------------------------------------

func TestOpenFileAnnouncesANewerAutosave(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	age(t, file, 10*time.Minute)
	if err := store.WriteAutosave(file, []byte("rescued work\n")); err != nil {
		t.Fatal(err)
	}

	visit(t, e, file)
	wantEcho(t, e, "recover-file")
}

// Recovery replaces the buffer in one undo group and leaves it modified: the
// recovered text is not what is on disk, and marking it clean would invite the
// user to quit believing it had been saved.
func TestRecoverFileRestoresAutosaveAsOneUndo(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAutosave(file, []byte("rescued work")); err != nil {
		t.Fatal(err)
	}

	b := visit(t, e, file)
	before := b.String()

	if err := e.RecoverFile(b); err != nil {
		t.Fatalf("RecoverFile: %v", err)
	}
	if got := b.String(); got != "rescued work" {
		t.Errorf("buffer = %q, want the autosaved text", got)
	}
	if !b.Modified() {
		t.Error("recovered buffer is marked clean; the user would quit thinking it was saved")
	}

	if _, ok := b.Undo(); !ok {
		t.Fatal("recovery left nothing to undo")
	}
	if got := b.String(); got != before {
		t.Errorf("after one undo buffer = %q, want the pre-recovery %q", got, before)
	}
}

func TestRecoverFileWithoutAnAutosaveReports(t *testing.T) {
	e, _, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, file)
	if err := e.RecoverFile(b); err == nil {
		t.Error("RecoverFile succeeded with no autosave present")
	}
}

// Turning backups off must stop both writes, not just one of them.
func TestBackupDisabledWritesNothing(t *testing.T) {
	e, store, file := safeEditor(t)
	if err := os.WriteFile(file, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.SetBackupEnabled(false)

	b := visit(t, e, file)
	edit(t, b, "more")
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if err := e.RunAutosave(time.Now()); err != nil {
		t.Fatalf("RunAutosave: %v", err)
	}

	for _, p := range []string{store.BackupPath(file), store.AutosavePath(file)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists although backups are disabled (err = %v)", p, err)
		}
	}
	if got := e.BackupRoot(); got == "" {
		t.Error("BackupRoot is empty; a user told an autosave exists needs to know where to look")
	}
}
