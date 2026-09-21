package editor

import (
	"strings"
	"testing"
)

// The welcome panel is the first thing anyone sees when nem starts with no
// file, and it must be gone the moment they start working.
func TestStartupPanelAppearsAndIsDismissed(t *testing.T) {
	e, _ := newTestEditor(t)

	if _, ok := e.startupPanel(); ok {
		t.Error("welcome panel offered before ShowStartup; it must be the caller's choice")
	}

	e.ShowStartup()
	if _, ok := e.startupPanel(); !ok {
		t.Fatal("no welcome panel after ShowStartup")
	}
	if len(e.frame().Panels) == 0 {
		t.Error("frame does not carry the welcome panel")
	}

	press(t, e, "a")
	if _, ok := e.startupPanel(); ok {
		t.Error("welcome panel survived a keystroke")
	}
}

// The dismissing keystroke must still do its job. A welcome screen that eats
// the first character someone types costs more than it is worth.
func TestStartupPanelDoesNotSwallowTheFirstKey(t *testing.T) {
	e, _ := newTestEditor(t)
	e.ShowStartup()

	press(t, e, "h", "i")
	if got := e.Buf().String(); got != "hi" {
		t.Errorf("buffer = %q, want %q - the first key was swallowed", got, "hi")
	}
}

// Once dismissed it must stay gone: nothing sets the flag again, and an empty
// scratch buffer is not a reason to greet someone a second time.
func TestStartupPanelNeverReturns(t *testing.T) {
	e, _ := newTestEditor(t)
	e.ShowStartup()
	press(t, e, "x")

	press(t, e, "<backspace>")
	if got := e.Buf().String(); got != "" {
		t.Fatalf("setup: buffer = %q, want it emptied", got)
	}
	if _, ok := e.startupPanel(); ok {
		t.Error("welcome panel came back once the buffer was empty again")
	}
}

// A file named on the command line means someone came to work, not to be
// greeted. main.go is what knows the difference, so the editor must not show it
// on its own.
func TestStartupPanelNotShownByDefault(t *testing.T) {
	e, _ := newTestEditor(t, "package main")
	if _, ok := e.startupPanel(); ok {
		t.Error("welcome panel shown without ShowStartup")
	}
	if len(e.frame().Panels) != 0 {
		t.Error("frame carries a panel nobody asked for")
	}
}

// The buffer underneath is an ordinary editable *scratch*, not a modal screen.
func TestBufferUnderTheStartupPanelIsEditable(t *testing.T) {
	e, _ := newTestEditor(t)
	e.ShowStartup()

	press(t, e, "p", "k", "g")
	if got := e.Buf().String(); got != "pkg" {
		t.Errorf("buffer = %q, want %q", got, "pkg")
	}
	if !strings.Contains(e.BufferName(e.Buf()), "scratch") {
		t.Errorf("buffer is %q, want the scratch buffer", e.BufferName(e.Buf()))
	}
}

// A screen too small for a panel shows none, rather than a broken box.
func TestStartupPanelSkippedOnATinyScreen(t *testing.T) {
	e, scr := newTestEditor(t)
	e.ShowStartup()
	scr.SetSize(4, 2)
	if _, ok := e.startupPanel(); ok {
		t.Error("welcome panel offered on a 4x2 screen")
	}
	e.Redraw() // must not panic
}
