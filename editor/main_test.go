package editor

import (
	"os"
	"testing"
)

// TestMain keeps every test in this package away from the real system
// clipboard.
//
// New installs defaultClipboardReader, which shells out to wl-paste, xclip or
// pbpaste. A test that yanked through it would read whatever the developer last
// copied - and assert on it, and print it in a failure. newTestEditor installs
// its own fake as well, but runKeys and a few other tests build an editor with
// New directly, so the default itself is replaced for the whole binary.
func TestMain(m *testing.M) {
	defaultClipboardReader = noSystemClipboard
	os.Exit(m.Run())
}

// noSystemClipboard is a clipboard with no text on it.
func noSystemClipboard() (string, bool) { return "", false }
