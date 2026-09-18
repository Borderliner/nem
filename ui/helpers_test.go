package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
	"github.com/muesli/termenv"
)

// TestMain pins the Lip Gloss colour profile to truecolor.
//
// Under `go test` stdout is a pipe, so Lip Gloss degrades to its Ascii profile
// and renders every style with no colour at all - which would make the style
// assertions below pass vacuously against a renderer that dropped colour
// entirely. The editor has the same problem for a different reason, which is
// what blit.SyncLipglossProfile exists to fix.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// bufferOf returns a buffer holding lines, one per element.
func bufferOf(t *testing.T, lines ...string) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	if len(lines) == 0 {
		return b
	}
	if err := b.Insert(text.Pos{}, []rune(strings.Join(lines, "\n"))); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Building the fixture is an edit, which legitimately marks the buffer
	// modified. Tests want a buffer that looks freshly loaded, so clear it.
	b.SetModified(false)
	return b
}

// sim returns an initialised w by h simulation screen.
func sim(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(scr.Fini)
	scr.SetSize(w, h)
	scr.Clear()
	return scr
}

// draw renders f into a fresh w by h screen and returns it. Show is called so
// the front buffer - what a terminal would display - is what gets asserted on.
func draw(t *testing.T, w, h int, f Frame) tcell.SimulationScreen {
	t.Helper()
	scr := sim(t, w, h)
	Render(scr, f, DefaultTheme())
	scr.Show()
	return scr
}

// cellAt returns the front-buffer cell at (x, y).
func cellAt(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.SimCell {
	t.Helper()
	cells, w, h := scr.GetContents()
	if x < 0 || y < 0 || x >= w || y >= h {
		t.Fatalf("cell (%d,%d) out of bounds %dx%d", x, y, w, h)
	}
	return cells[y*w+x]
}

// rowText reads row y as a string, rendering empty cells as spaces. A cell
// holding no runes is the trailing half of a wide glyph or was never written;
// both read as a blank, which is what a terminal shows.
func rowText(t *testing.T, scr tcell.SimulationScreen, y int) string {
	t.Helper()
	cells, w, _ := scr.GetContents()
	var b strings.Builder
	for x := 0; x < w; x++ {
		rs := cells[y*w+x].Runes
		if len(rs) == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteString(string(rs))
	}
	return b.String()
}

// colText reads column x down the screen as a string.
func colText(t *testing.T, scr tcell.SimulationScreen, x int) string {
	t.Helper()
	cells, w, h := scr.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		rs := cells[y*w+x].Runes
		if len(rs) == 0 {
			b.WriteRune(' ')
			continue
		}
		b.WriteString(string(rs))
	}
	return b.String()
}

// linesOf returns n lines named line0..line(n-1).
func linesOf(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "line" + strconv.Itoa(i)
	}
	return out
}

// singleFrame returns a frame with one window over the given lines.
func singleFrame(t *testing.T, lines ...string) (Frame, *view.Window) {
	t.Helper()
	w := view.NewWindow(bufferOf(t, lines...))
	return Frame{Tree: view.NewTree(w), Active: w}, w
}
