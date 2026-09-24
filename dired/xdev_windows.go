package dired

import "syscall"

// errCrossDevice is what rename reports when src and dst are on different
// volumes: ERROR_NOT_SAME_DEVICE, which the syscall package does not name.
var errCrossDevice error = syscall.Errno(17)
