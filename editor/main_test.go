package editor

import (
	"os"
	"testing"

	"github.com/Borderliner/nem/sysopen"
)

// TestMain keeps every test in this package away from the real system
// clipboard, and from the system's app for opening files.
//
// New installs defaultClipboardReader, which shells out to wl-paste, xclip or
// pbpaste. A test that yanked through it would read whatever the developer last
// copied - and assert on it, and print it in a failure. newTestEditor installs
// its own fake as well, but runKeys and a few other tests build an editor with
// New directly, so the default itself is replaced for the whole binary.
//
// The opener likewise: a test that opened a binary file through the real one
// would start a viewer on the developer's desktop. With none available, such a
// file opens as text; tests of the feature install a recording fake.
func TestMain(m *testing.M) {
	defaultClipboardReader = noSystemClipboard
	systemOpen = func(string, func(error)) error { return sysopen.ErrUnavailable }
	systemAvailable = func() error { return sysopen.ErrUnavailable }
	os.Exit(m.Run())
}

// noSystemClipboard is a clipboard with no text on it.
func noSystemClipboard() (string, bool) { return "", false }
