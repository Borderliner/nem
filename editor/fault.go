package editor

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Borderliner/nem/backup"
)

// Ending badly without losing work. Two things end a session other than the
// user asking: a signal from outside - the terminal closed, an SSH connection
// dropped, the system shutting down - and a bug in nem itself. Before this,
// either took every unsaved buffer with it: a signal killed nem where it
// stood, and a panic unwound it to exit.
//
// Now a signal ends the session as a quit does, but without asking - there
// is nobody to ask - after writing every unsaved buffer where it will be
// found: a file's to its autosave, which opening it again offers back, and a
// buffer with no file to a file of its own among the rescued. And a bug is
// caught where it happens, reported, and the session carries on, with the
// unsaved work written away first in case the bug left something half done.

// maxFaultReports is how many bugs a session writes reports of. A bug in
// drawing would otherwise write one every frame.
const maxFaultReports = 5

// SignalError is what Loop returns when a signal ended the session.
type SignalError struct {
	Signal os.Signal
	// Saved lists where the unsaved buffers were written.
	Saved []string
	// Err says what could not be written, if anything.
	Err error
}

func (s *SignalError) Error() string {
	msg := fmt.Sprintf("stopped by %v", s.Signal)
	if len(s.Saved) > 0 {
		msg += "; unsaved work written to " + strings.Join(s.Saved, ", ")
	}
	if s.Err != nil {
		msg += "; " + s.Err.Error()
	}
	return msg
}

// SetSignals has the event loop end the session when a signal arrives on ch.
// The caller chooses the signals, with signal.Notify.
func (e *Editor) SetSignals(ch <-chan os.Signal) { e.signals = ch }

// stop ends the session for sig, writing the unsaved work away first. Every
// prompt open gives up, and the event loop returns.
func (e *Editor) stop(sig os.Signal) {
	saved, err := e.EmergencySave()
	e.stopped = &SignalError{Signal: sig, Saved: saved, Err: err}
	e.quit = true
}

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
