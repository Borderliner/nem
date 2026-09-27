package editor

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
)

// A process is a command nem runs for the user - a compilation, a shell
// command - whose output reaches the editor through the event loop.
//
// Its output is read on a goroutine, which must never touch the editor: the
// editor's state has exactly one owner, the loop, and that is what keeps it
// free of locks. So the goroutine only piles output up here and posts a wake
// event, and the loop takes the pile in when the wake arrives. One wake is
// outstanding at a time, however fast the output comes, so a build that
// prints a million lines cannot flood the event queue and crowd out the
// keyboard.
type process struct {
	cmd *exec.Cmd

	mu   sync.Mutex
	out  []byte // arrived and not yet taken in
	done bool   // exited, with all of its output in out
	code int    // exit status once done; -1 if it did not exit by itself
	err  error  // why it ended other than by exiting, once done

	woken  atomic.Bool
	post   func(*process) error
	killed atomic.Bool
}

// processWake is the event that tells the loop a process has something to
// take in.
type processWake struct{ p *process }

// readGrace is how long the output is waited for after a command exits. A
// command that leaves something running in the background - make starting a
// server, a shell command ending in & - hands it the output pipe, which then
// never closes; the command is finished all the same.
const readGrace = 250 * time.Millisecond

// shellCommand builds the command that runs line through the shell. It is
// sh rather than the user's login shell, as a Makefile's recipes are: what
// people type at M-! and M-x compile is written for sh far more often than
// for fish or nu, and sh behaves the same everywhere. The shell setting
// changes it.
func (e *Editor) shellCommand(line string) *exec.Cmd {
	if e.shell != "" {
		return exec.Command(e.shell, "-c", line)
	}
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", line)
	}
	return exec.Command("/bin/sh", "-c", line)
}

// startProcess runs line in dir, with stdin as its input when it is not nil,
// and its output and errors together, in the order it wrote them.
func (e *Editor) startProcess(line, dir string, stdin []byte) (*process, error) {
	cmd := e.shellCommand(line)
	cmd.Dir = dir
	// A pager waiting for a keypress nobody can send would hang the command,
	// and colour meant for a terminal is noise in a buffer.
	cmd.Env = append(os.Environ(), "TERM=dumb", "PAGER=cat", "GIT_PAGER=cat")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = w, w
	ownProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	w.Close() // the child has its own copy; ours would keep the pipe open

	p := &process{cmd: cmd, post: e.postWake}
	read := make(chan struct{})
	go func() {
		defer close(read)
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				p.mu.Lock()
				p.out = append(p.out, buf[:n]...)
				p.mu.Unlock()
				p.wake()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		select {
		case <-read:
		case <-time.After(readGrace):
			r.Close()
			<-read
		}
		p.mu.Lock()
		p.done, p.code, p.err = true, exitCode(err), err
		p.mu.Unlock()
		p.wake()
	}()
	return p, nil
}

// exitCode is the status a command exited with, or -1 if it was killed or
// never ran.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// postWake is how a process reaches the loop: an interrupt event carrying it.
func (e *Editor) postWake(p *process) error {
	if e.scr == nil {
		return errors.New("no screen")
	}
	return e.scr.PostEvent(tcell.NewEventInterrupt(processWake{p}))
}

// wake posts a wake unless one is already on its way. A full event queue -
// a paste arriving, keys held down - is waited out rather than dropping the
// wake, which would strand the output until something else happened.
func (p *process) wake() {
	if p.woken.Swap(true) {
		return
	}
	for range 1000 {
		if p.post(p) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.woken.Store(false)
}

// take hands over the output that has arrived, and whether the process is
// finished. The loop calls it on a wake; clearing the wake first means output
// arriving while it is taken in posts another.
func (p *process) take() (out []byte, done bool) {
	p.woken.Store(false)
	p.mu.Lock()
	defer p.mu.Unlock()
	out, p.out = p.out, nil
	return out, p.done
}

// result is how the process ended, once take has reported it done.
func (p *process) result() (code int, killed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code, p.killed.Load()
}

// kill stops the process and everything it started, as C-c in a terminal
// would, and makes sure of it a moment later if it did not listen.
func (p *process) kill() {
	if p.cmd.Process == nil || !p.killed.CompareAndSwap(false, true) {
		return
	}
	interruptGroup(p.cmd)
	time.AfterFunc(2*time.Second, func() {
		p.mu.Lock()
		done := p.done
		p.mu.Unlock()
		if !done {
			killGroup(p.cmd)
		}
	})
}
