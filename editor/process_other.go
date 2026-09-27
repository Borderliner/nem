//go:build !unix

package editor

import "os/exec"

// ownProcessGroup does nothing where there are no process groups to join.
func ownProcessGroup(*exec.Cmd) {}

// interruptGroup stops cmd; without process groups, what it started may
// outlive it.
func interruptGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// killGroup stops cmd.
func killGroup(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
