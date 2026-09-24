//go:build unix

package dired

import "syscall"

// errCrossDevice is what rename reports when src and dst are on different
// filesystems.
var errCrossDevice error = syscall.EXDEV
