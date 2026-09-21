package ui

import (
	"github.com/Borderliner/nem/view"
	"github.com/charmbracelet/lipgloss"
)

// Starting nem with no file used to drop you straight into an empty *scratch*:
// a blank screen, a modeline, and no indication of what to press. This is the
// first thing anyone sees, so it says what the editor is and the five keys that
// get you moving, and then gets out of the way on the first keystroke.
//
// It is deliberately just a panel. The panel layer already places, clips and
// occludes correctly, so a welcome screen costs a list of strings rather than a
// second rendering path with its own bugs.

// startupTitle labels the panel.
const startupTitle = "nem"

// startupLines is the welcome text.
//
// Short on purpose. A first screen that has to be read is a first screen that
// gets dismissed unread, so this is one line of what nem is and the smallest
// set of keys that makes it usable: open, save, quit, find any command, and
// find every key. Everything else is discoverable from M-x and <f1> b, which is
// exactly why those two are here.
//
// No ASCII art. The border and the palette are the presentation, and a banner
// would only push the keys further down the panel.
var startupLines = []string{
	"a terminal editor with emacs keys",
	"",
	"  C-x C-f    open a file",
	"  C-x C-s    save",
	"  C-x C-c    exit",
	"  M-x        run a command by name",
	"  <f1> b     show every binding",
	"",
	"press any key to begin",
}

// StartupPanel builds the welcome panel for frame, reporting false when there
// is no room for it.
//
// A frame too small returns false rather than a squeezed box: the buffer
// underneath is perfectly usable, and half a panel over it is worse than none.
func StartupPanel(frame view.Rect, th Theme) (Panel, bool) {
	w := lipgloss.Width(startupTitle) + 4 // the title is set into the top border
	for _, ln := range startupLines {
		if n := lipgloss.Width(ln); n > w {
			w = n
		}
	}
	// Two cells of border plus a column of padding on each side, so the text
	// does not sit against the frame.
	w += 4
	h := len(startupLines) + 2

	rect, ok := view.PlacePanel(view.PanelReq{
		W: w, H: h, Anchor: view.AnchorCenter, Frame: frame,
	})
	if !ok {
		return Panel{}, false
	}

	lines := make([]PanelLine, 0, len(startupLines))
	for _, ln := range startupLines {
		if ln == "" {
			lines = append(lines, PanelLine{})
			continue
		}
		lines = append(lines, PanelLine{Text: " " + ln})
	}
	return Panel{Rect: rect, Title: startupTitle, Lines: lines}, true
}
