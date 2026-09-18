package editor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/hajianpour/nem/backup"
	"github.com/hajianpour/nem/text"
)

// Data safety: backups, autosaves, recovery, and noticing when something else
// changed a file.
//
// All three write through the backup package, which keeps everything under one
// state directory rather than beside the edited file. Nothing here may construct
// a path next to the file being edited — that is the invariant the store exists
// to hold, and reintroducing a local path here would defeat it.
//
// The rule these share: a failure protects the user's work or gets out of the
// way, and never pretends. A backup that cannot be written must not block the
// save the user asked for; an autosave that cannot be written must say so rather
// than leave the user believing they have recovery files.

// DefaultAutosaveIdle is how long a modified buffer may sit untouched before its
// contents reach the autosave store.
const DefaultAutosaveIdle = 30 * time.Second

// stamp is what a file looked like on disk when nem last read or wrote it.
// Comparing it against a fresh stat is how an edit made by something else — a
// git checkout, another editor, a formatter — is noticed before nem overwrites
// it.
type stamp struct {
	size   int64
	mtime  time.Time
	exists bool
}

// stampOf reads a path's current fingerprint. A path that cannot be stat'ed at
// all reports as not existing, which is the conservative reading: nem then has
// nothing to compare against and will not claim a file changed.
func stampOf(path string) stamp {
	fi, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	return stamp{size: fi.Size(), mtime: fi.ModTime(), exists: true}
}

func (s stamp) sameAs(o stamp) bool {
	return s.exists == o.exists && s.size == o.size && s.mtime.Equal(o.mtime)
}

// safety holds everything the data-safety features need to remember.
type safety struct {
	store   *backup.Store
	enabled bool

	// backedUp is keyed by file path rather than by buffer, for two reasons. A
	// file is copied once per session, so a long session preserves the file as
	// it was when you opened it rather than as it was one save ago. And
	// write-file clobbers a file the buffer was never loaded from, which a
	// per-buffer key would miss entirely.
	backedUp map[string]bool

	// stamps records what each path looked like when nem last read or wrote it.
	stamps map[string]stamp

	idle time.Duration
	// lastInput is when a keystroke last arrived; autosaveAt is the lastInput
	// value current when the last autosave ran. Comparing them is what stops an
	// idle editor rewriting the same autosave on every pass through the loop.
	lastInput  time.Time
	autosaveAt time.Time

	// errReported suppresses repeat complaints about the same broken store. A
	// store that cannot be written must say so once; saying so every idle
	// interval would bury every other message in the echo area.
	errReported bool
}

func newSafety() safety {
	s := safety{
		enabled:  true,
		backedUp: map[string]bool{},
		stamps:   map[string]stamp{},
		idle:     DefaultAutosaveIdle,
	}
	// A missing home directory disables the store rather than failing startup:
	// an editor that will not open because it cannot find a backup directory is
	// worse than one that edits without backups.
	if root, err := backup.DefaultRoot(); err == nil {
		s.store = backup.New(root)
	}
	return s
}

// SetBackupRoot points the backup and autosave store at root. Tests use this to
// keep out of the real state directory.
func (e *Editor) SetBackupRoot(root string) { e.safe.store = backup.New(root) }

// BackupRoot reports where recovery files are kept, or "" when there is no
// store. Worth surfacing: a user who is told an autosave exists needs to know
// where to look.
func (e *Editor) BackupRoot() string {
	if e.safe.store == nil {
		return ""
	}
	return e.safe.store.Root()
}

// SetBackupEnabled turns backups and autosaves on or off.
func (e *Editor) SetBackupEnabled(on bool) { e.safe.enabled = on }

// SetAutosaveIdle sets how long a modified buffer may sit untouched before it is
// autosaved. Zero disables autosave.
func (e *Editor) SetAutosaveIdle(d time.Duration) { e.safe.idle = d }

// --- autosave ------------------------------------------------------------

// NoteInput records that a keystroke arrived, restarting the idle clock. The
// event loop calls this for every key event.
func (e *Editor) NoteInput(now time.Time) { e.safe.lastInput = now }

// AutosaveDue reports whether enough idle time has passed, since the last
// keystroke, to be worth writing recovery files.
//
// It is false when nothing has been typed since the previous autosave, so an
// editor left alone writes once and then stays quiet.
func (e *Editor) AutosaveDue(now time.Time) bool {
	s := &e.safe
	if s.idle <= 0 || s.store == nil || !s.enabled {
		return false
	}
	if s.lastInput.IsZero() || !s.autosaveAt.Before(s.lastInput) {
		return false
	}
	if now.Sub(s.lastInput) < s.idle {
		return false
	}
	return e.anyModifiedWithPath()
}

// RunAutosave writes every modified buffer that has a file to the autosave
// store.
//
// Errors are reported through the echo area and returned for tests; the event
// loop can ignore the return. A failure must be visible, because a user who
// believes they have recovery files and does not is worse off than one who knows
// they have none.
func (e *Editor) RunAutosave(now time.Time) error {
	s := &e.safe
	s.autosaveAt = s.lastInput
	if s.store == nil || !s.enabled {
		return nil
	}

	var firstErr error
	written := 0
	for _, b := range e.buffers {
		if !b.Modified() || b.Path() == "" {
			continue
		}
		if err := s.store.WriteAutosave(b.Path(), []byte(b.String())); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		written++
	}

	if firstErr != nil {
		e.reportSafetyFailure("autosave", firstErr)
		return firstErr
	}
	if written > 0 {
		s.errReported = false
	}
	return nil
}

func (e *Editor) anyModifiedWithPath() bool {
	for _, b := range e.buffers {
		if b.Modified() && b.Path() != "" {
			return true
		}
	}
	return false
}

// --- recovery ------------------------------------------------------------

// AutosaveAvailable reports whether path has an autosave holding work its saved
// file does not.
func (e *Editor) AutosaveAvailable(path string) (bool, time.Time) {
	if e.safe.store == nil || !e.safe.enabled {
		return false, time.Time{}
	}
	newer, at, err := e.safe.store.AutosaveNewer(path)
	if err != nil {
		return false, time.Time{}
	}
	return newer, at
}

// RecoverFile replaces b's contents with its autosave.
//
// The buffer is left modified on purpose: recovered text is not what is on disk,
// and marking it clean would invite the user to quit believing it had been
// saved. The whole replacement is one undo group, so a mistaken recovery is a
// single C-/ away.
func (e *Editor) RecoverFile(b *text.Buffer) error {
	if e.safe.store == nil || !e.safe.enabled {
		return errors.New("no backup store: recovery is unavailable")
	}
	path := b.Path()
	if path == "" {
		return text.ErrNoPath
	}

	content, err := e.safe.store.ReadAutosave(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("no autosave for %s", filepath.Base(path))
		}
		return err
	}

	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if err := b.Delete(text.Pos{}, b.End()); err != nil {
		return err
	}
	if err := b.Insert(text.Pos{}, []rune(string(content))); err != nil {
		return err
	}
	b.SetModified(true)
	e.Echo("Recovered %s from its autosave; not yet saved", filepath.Base(path))
	return nil
}

// --- saving --------------------------------------------------------------

// noteOnDisk records what target looks like now, so a later save can tell
// whether anything else has touched it.
func (e *Editor) noteOnDisk(target string) {
	if target != "" {
		e.safe.stamps[target] = stampOf(target)
	}
}

// changedOnDisk reports whether target differs from what nem last saw, and is
// false when nem has no record — an unknown file is not a changed one.
func (e *Editor) changedOnDisk(target string) bool {
	known, ok := e.safe.stamps[target]
	if !ok {
		return false
	}
	return !known.sameAs(stampOf(target))
}

// confirmOverwrite asks before writing over a file something else changed.
//
// Without a screen there is nobody to ask, so the save is refused: silently
// clobbering an external edit is the worst outcome available here, and refusing
// leaves both the file and the buffer recoverable.
func (e *Editor) confirmOverwrite(target string) bool {
	if !e.changedOnDisk(target) {
		return true
	}
	if e.scr == nil {
		return false
	}
	answer, err := e.ReadChar(
		fmt.Sprintf("%s changed on disk. Overwrite? (y/n) ", filepath.Base(target)),
		[]rune{'y', 'n', 'Y', 'N'},
	)
	if err != nil {
		return false
	}
	return answer == 'y' || answer == 'Y'
}

// backupBeforeWrite copies target's current contents into the store.
//
// Called before the write, which is the only useful order: a backup taken
// afterwards holds the new contents and preserves nothing. A failure is reported
// but never blocks the save — the user asked to save their work, and refusing
// because a backup could not be filed would lose more than it protects.
func (e *Editor) backupBeforeWrite(target string) {
	s := &e.safe
	if s.store == nil || !s.enabled || target == "" || s.backedUp[target] {
		return
	}

	prev, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// A new file has no previous contents to preserve. Recording it as
			// done stops every later save re-reading a file that is not there.
			s.backedUp[target] = true
			return
		}
		e.reportSafetyFailure("backup", err)
		return
	}
	if err := s.store.WriteBackup(target, prev); err != nil {
		e.reportSafetyFailure("backup", err)
		return
	}
	s.backedUp[target] = true
	s.errReported = false
}

// afterSave updates the on-disk record and discards the now-redundant autosave.
func (e *Editor) afterSave(target string) {
	e.noteOnDisk(target)
	if s := &e.safe; s.store != nil && s.enabled {
		// A stale autosave would offer to "recover" work that is already on
		// disk, so a successful save must clear it.
		if err := s.store.RemoveAutosave(target); err != nil {
			e.reportSafetyFailure("autosave cleanup", err)
		}
	}
}

// reportSafetyFailure surfaces a store failure once, then stays quiet until
// something succeeds.
func (e *Editor) reportSafetyFailure(what string, err error) {
	if e.safe.errReported {
		return
	}
	e.safe.errReported = true
	e.Echo("%s failed: %v", what, err)
}
