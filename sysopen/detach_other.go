//go:build !unix

package sysopen

import "os/exec"

// detach has nothing to do where there is no controlling terminal to escape.
func detach(*exec.Cmd) {}
