package ui

import (
	"github.com/Borderliner/nem/ui/blit"
	"github.com/gdamore/tcell/v2"
)

// Screen owns the terminal. It is a thin wrapper over tcell.Screen whose only
// real job is making sure Lip Gloss is pointed at the right place.
type Screen struct {
	tcell.Screen
}

// NewScreen opens and initialises the terminal.
func NewScreen() (*Screen, error) {
	scr, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := scr.Init(); err != nil {
		return nil, err
	}
	// Bracketed paste makes the terminal mark where a paste begins and ends.
	// Without it pasted text is indistinguishable from typing and runs through
	// the keymap a character at a time: auto-indent re-indents every pasted
	// line, and every character costs a redraw. Fini turns it off again.
	scr.EnablePaste()
	return Wrap(scr), nil
}

// Wrap adopts an already-initialised screen. Tests pass a simulation screen
// here; NewScreen passes a real one.
//
// It syncs the Lip Gloss colour profile, which must happen after Init and is
// not optional. Lip Gloss decides how much colour to emit by probing
// os.Stdout, and that probe is meaningless once tcell owns the terminal: it can
// silently degrade to rendering every style with no colour at all. tcell has
// already done real terminfo negotiation, so its answer is the authoritative
// one. Skipping this produces unstyled chrome that looks exactly like a bug in
// the blitter and is not.
func Wrap(scr tcell.Screen) *Screen {
	blit.SyncLipglossProfile(scr)
	scr.SetStyle(tcell.StyleDefault)
	return &Screen{Screen: scr}
}

// Resync redraws the terminal from scratch and re-syncs the colour profile.
//
// Call it after anything that may have reinitialised the screen or left another
// process's output on it: a suspend/resume cycle, or a shell command run in the
// foreground. A plain resize does not need the profile sync, but doing it here
// too costs nothing and means there is one method to reach for rather than two
// that differ subtly.
func (s *Screen) Resync() {
	blit.SyncLipglossProfile(s.Screen)
	s.Screen.Sync()
}

// Close restores the terminal.
func (s *Screen) Close() {
	s.Screen.Fini()
}
