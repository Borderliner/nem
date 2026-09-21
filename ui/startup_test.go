package ui

import (
	"strings"
	"testing"

	"github.com/hajianpour/nem/view"
)

func TestStartupPanelShowsTheEssentialKeys(t *testing.T) {
	p, ok := StartupPanel(view.Rect{W: 80, H: 23}, DefaultTheme())
	if !ok {
		t.Fatal("StartupPanel refused an 80x23 frame")
	}
	if p.Title != startupTitle {
		t.Errorf("title = %q, want %q", p.Title, startupTitle)
	}

	var all strings.Builder
	for _, ln := range p.Lines {
		all.WriteString(ln.Text)
		all.WriteByte('\n')
	}
	// These five are the whole point: open, save, quit, and the two keys that
	// make everything else discoverable.
	for _, want := range []string{"C-x C-f", "C-x C-s", "C-x C-c", "M-x", "<f1> b"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("startup panel does not mention %q:\n%s", want, all.String())
		}
	}
}

// The panel must fit inside the frame it was given, or it draws over the echo
// row and clips at the screen edge.
func TestStartupPanelFitsItsFrame(t *testing.T) {
	for _, frame := range []view.Rect{
		{W: 80, H: 23}, {W: 120, H: 40}, {W: 40, H: 12}, {W: 200, H: 60},
	} {
		p, ok := StartupPanel(frame, DefaultTheme())
		if !ok {
			continue
		}
		if p.Rect.X < frame.X || p.Rect.Y < frame.Y ||
			p.Rect.X+p.Rect.W > frame.X+frame.W ||
			p.Rect.Y+p.Rect.H > frame.Y+frame.H {
			t.Errorf("frame %v: panel %v escapes it", frame, p.Rect)
		}
	}
}

// A frame with no room returns false rather than a squeezed box: the buffer
// underneath is perfectly usable, and half a panel over it is worse than none.
func TestStartupPanelRefusesATinyFrame(t *testing.T) {
	for _, frame := range []view.Rect{{W: 4, H: 2}, {W: 1, H: 1}, {W: 0, H: 0}} {
		if _, ok := StartupPanel(frame, DefaultTheme()); ok {
			t.Errorf("frame %v: panel accepted, want refusal", frame)
		}
	}
}

// It is drawn over an empty buffer, so it has to occlude - the panel layer
// clears its rect, and this is the end-to-end check that it does.
func TestStartupPanelDrawsOverTheBuffer(t *testing.T) {
	b := bufferOf(t, strings.Repeat("x", 70), strings.Repeat("x", 70))
	w := view.NewWindow(b)
	tree := view.NewTree(w)

	p, ok := StartupPanel(view.Rect{W: 80, H: 23}, DefaultTheme())
	if !ok {
		t.Fatal("StartupPanel refused an 80x23 frame")
	}
	scr := draw(t, 80, 24, Frame{Tree: tree, Active: w, Panels: []Panel{p}})

	row := rowText(t, scr, p.Rect.Y+1)
	inside := row[p.Rect.X : p.Rect.X+p.Rect.W]
	if strings.Contains(inside, "xxx") {
		t.Errorf("buffer text shows through the panel: %q", inside)
	}
}
