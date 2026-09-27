package editor

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/Borderliner/nem/backup"
)

// Ending badly without losing work. A bug in nem used to end the session,
// unwinding it to exit with every unsaved buffer in it. Now a bug is caught
// where it happens and reported, and the session carries on - with the
// unsaved work written away first, in case the bug left something half
// done: a file's to its autosave, which opening it again offers back, and a
// buffer with no file to a file of its own among the rescued.

// maxFaultReports is how many bugs a session writes reports of. A bug in
// drawing would otherwise write one every frame.
const maxFaultReports = 5

// EmergencySave writes every buffer with unsaved changes where it will be
// found again, and reports where: a file's to its autosave - opening the
// file again says so, and M-x recover-file brings it back - and a buffer
// with no file to a file of its own among the rescued. It is for when nem is
// about to end, or may be, without anyone to ask what to do with them.
func (e *Editor) EmergencySave() (saved []string, err error) {
	s := &e.safe
	if s.store == nil || !s.enabled {
		return nil, nil
	}
	stamp := time.Now().Format("20060102-150405")
	var errs []error
	for _, b := range e.buffers {
		if !b.Modified() {
			continue
		}
		if p := b.Path(); p != "" {
			if err := s.store.WriteAutosave(p, b.Contents()); err != nil {
				errs = append(errs, err)
				continue
			}
			saved = append(saved, s.store.AutosavePath(p))
			continue
		}
		p, err := s.store.Keep(backup.Rescued, e.names[b]+"-"+stamp+".txt", b.Contents())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		saved = append(saved, p)
	}
	return saved, errors.Join(errs...)
}

// guard runs fn, catching a panic in it as a bug - see fault - and reporting
// it in the echo area. where names what was running, for the report.
func (e *Editor) guard(where string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			e.Echo("%v", e.fault(where, r, debug.Stack()))
		}
	}()
	fn()
}

// fault deals with a panic: a bug in nem, caught while where was running.
// The unsaved work is written away, a report is written for sending in, and
// the error returned says both.
func (e *Editor) fault(where string, r any, stack []byte) error {
	e.faults++
	saved, _ := e.EmergencySave()
	msg := fmt.Sprintf("nem bug in %s: %v", where, r)
	if len(saved) > 0 {
		msg += "; unsaved work autosaved"
	}
	if e.faults <= maxFaultReports && e.safe.store != nil {
		report := fmt.Sprintf("nem bug in %s: %v\n\n%s/%s, %s\n\n%s", where, r,
			runtime.GOOS, runtime.GOARCH, runtime.Version(), stack)
		name := time.Now().Format("20060102-150405") + ".txt"
		if p, err := e.safe.store.Keep(backup.Faults, name, []byte(report)); err == nil {
			msg += "; details in " + p
		}
	}
	return errors.New(msg)
}
