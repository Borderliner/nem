package ui

import (
	"io"
	"os"
	"strings"

	"github.com/Borderliner/nem/ui/blit"
	"github.com/gdamore/tcell/v2"
)

// Screen owns the terminal. It is a thin wrapper over tcell.Screen whose jobs
// are making sure Lip Gloss is pointed at the right place, and telling the
// terminal who lays out right-to-left text.
type Screen struct {
	tcell.Screen
	// bidiTaken is set while the terminal is told nem lays right-to-left
	// text out itself. See TakeBidi.
	bidiTaken bool
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
	// Coming back to the terminal is when files changed elsewhere are looked
	// at again, where the terminal says so.
	scr.EnableFocus()
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
	s.TakeBidi(false)
	s.Screen.Fini()
}

// The BiDi recommendation for terminal emulators' switch, ECMA-48's BDSM:
// explicit mode, in which the application has laid right-to-left text out
// and the terminal shows it as it is sent; and implicit mode, the terminal's
// own default, in which it reorders each line itself.
const (
	bidiExplicit = "\x1b[8l"
	bidiImplicit = "\x1b[8h"
)

// TakeBidi says whether nem lays right-to-left text out itself. While it
// does, the terminal is told not to reorder it again: VTE's terminals and
// mintty do as they are told, and a terminal that does not know the switch
// ignores it. Close gives the terminal its own layout back.
//
// The switch travels with nem's output, so it reaches the terminal over ssh
// and under sudo, where nothing in the environment says what the terminal
// is. Inside tmux or screen it is also sent through to the terminal outside,
// which draws every pane: tmux passes it on only with allow-passthrough on.
func (s *Screen) TakeBidi(take bool) {
	if take == s.bidiTaken {
		return
	}
	seq := bidiImplicit
	if take {
		seq = bidiExplicit
	}
	tty, ok := s.Screen.Tty()
	if !ok || tty == nil {
		return
	}
	_, _ = io.WriteString(tty, seq)
	if wrapped := passThrough(seq); wrapped != "" {
		_, _ = io.WriteString(tty, wrapped)
	}
	s.bidiTaken = take
}

// passThrough wraps seq for the terminal outside a multiplexer nem runs in,
// or returns "" outside one. Both multiplexers take it as a device control
// string; tmux wants its name first, and every escape inside doubled.
func passThrough(seq string) string {
	switch {
	case os.Getenv("TMUX") != "":
		return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
	case os.Getenv("STY") != "":
		return "\x1bP" + seq + "\x1b\\"
	}
	return ""
}
