//go:build unix

package editor

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A named pipe is refused rather than read. Reading one blocks until something
// writes to it, which froze nem with no way out but killing it.
func TestOpeningANamedPipeIsRefused(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	e, _ := newTestEditor(t)

	done := make(chan error, 1)
	go func() {
		_, err := e.OpenFile(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("OpenFile(fifo) = %v, want a refusal", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OpenFile blocked on a named pipe")
	}
}
