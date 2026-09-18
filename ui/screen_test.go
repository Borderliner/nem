package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Wrap must point Lip Gloss at the terminal tcell opened. Without it, Lip Gloss
// probes os.Stdout - meaningless once tcell owns the terminal - and can render
// the whole chrome with no colour, a failure that looks exactly like a blitter
// bug and is not.
func TestWrapSyncsLipglossProfile(t *testing.T) {
	saved := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	lipgloss.SetColorProfile(termenv.Ascii)
	scr := sim(t, 20, 6)
	Wrap(scr)

	if got := lipgloss.ColorProfile(); got == termenv.Ascii {
		t.Error("colour profile still Ascii after Wrap; chrome would render unstyled")
	}
}

func TestResyncKeepsProfileSynced(t *testing.T) {
	saved := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	scr := sim(t, 20, 6)
	s := Wrap(scr)

	// A suspend/resume cycle reinitialises the screen, and Lip Gloss must be
	// pointed at it again.
	lipgloss.SetColorProfile(termenv.Ascii)
	s.Resync()

	if got := lipgloss.ColorProfile(); got == termenv.Ascii {
		t.Error("colour profile still Ascii after Resync")
	}
}

func TestScreenExposesUnderlyingSize(t *testing.T) {
	s := Wrap(sim(t, 24, 9))
	if w, h := s.Size(); w != 24 || h != 9 {
		t.Errorf("Size() = (%d,%d), want (24,9)", w, h)
	}
}
