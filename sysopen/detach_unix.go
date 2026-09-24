//go:build unix

package sysopen

import (
	"os/exec"
	"syscall"
)

// detach starts the app in a session of its own, so closing nem's terminal
// does not take the PDF viewer down with it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
