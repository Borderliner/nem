package editor

import (
	"github.com/Borderliner/nem/memory"
)

// What nem remembers between sessions lives in the memory package; this is
// where the editor feeds it and reads it back.

// UseStateDir loads what was remembered in dir and keeps remembering there.
// Until it is called nothing persists, which is what keeps a test from
// reading or writing anyone's real history.
func (e *Editor) UseStateDir(dir string) error {
	m, err := memory.Load(dir)
	if err != nil {
		return err
	}
	e.mem = m
	return nil
}

// saveMemory writes the memory, quietly: a history that could not be saved is
// not worth interrupting anyone over, and the next save tries again.
func (e *Editor) saveMemory() { _ = e.mem.Save() }

// SaveMemory is for the end of a session.
func (e *Editor) SaveMemory() { e.saveMemory() }
