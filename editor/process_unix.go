//go:build unix

package editor

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts cmd in a process group of its own, so stopping it
// stops what it started too: the compiler make runs, the tests go test runs.
// Signalling the shell alone would leave those running on, orphaned.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interruptGroup asks cmd's group to stop, as C-c at a terminal does.
func interruptGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// killGroup stops cmd's group outright.
func killGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
