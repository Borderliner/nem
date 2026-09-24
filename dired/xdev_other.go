//go:build !unix && !windows

package dired

import "errors"

// errCrossDevice stands in on platforms whose rename error for crossing
// filesystems is not known here. Nothing returns it, so Move there reports
// the rename error as it is rather than guessing at a fallback.
var errCrossDevice = errors.New("cross-device rename")
