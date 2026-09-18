// Command blitdemo renders a mock of nem's chrome so the Lip Gloss to tcell
// blitter can be judged by eye rather than only by test assertions.
//
// It draws two side-by-side window panes separated by a vertical divider, each
// with its own modeline, plus a minibuffer along the bottom. The left pane is
// styled as the active window. Sample text includes CJK and emoji so cell-width
// handling is visible: if the blitter miscounts a glyph, the right-hand border
// will not line up.
//
// Press q, Escape or Ctrl-C to quit. Tab switches the active pane.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/ui/blit"
)

// The palette mirrors ui.DefaultTheme so this mock previews the real chrome
// rather than drifting into a look nem does not have. Three colours, no
// background ever painted: the editor sits inside the terminal's own palette.
var (
	colQuiet = lipgloss.Color("#8a857c") // secondary text, inactive names
	colRule  = lipgloss.Color("#6b655b") // modeline hairline, pane divider
	colMark  = lipgloss.Color("#b4543a") // the one accent: unsaved changes
)

type pane struct {
	name     string
	modified bool
	lines    []string
	line     int
	col      int
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("blitdemo: visual check for the Lip Gloss to tcell blitter.")
		fmt.Println("keys: q/Esc/C-c quit, Tab switch active pane")
		return
	}

	scr, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("new screen: %v", err)
	}
	if err := scr.Init(); err != nil {
		log.Fatalf("init screen: %v", err)
	}
	defer scr.Fini()

	// Without this, Lip Gloss probes stdout - which tcell has taken over - and
	// may silently render everything with no colour at all.
	blit.SyncLipglossProfile(scr)

	scr.SetStyle(tcell.StyleDefault)
	scr.Clear()

	panes := []pane{
		{
			name:     "buffer.go",
			modified: true,
			lines: []string{
				"package main",
				"",
				"// ASCII renders at one cell per rune.",
				"func main() {",
				"    greet(\"world\")",
				"}",
			},
			line: 5, col: 18,
		},
		{
			name:     "notes.md",
			modified: false,
			lines: []string{
				"# width torture test",
				"",
				"CJK:      日本語のテキスト",
				"emoji:    🙂 🚀 ✨",
				"ZWJ fam:  👨‍👩‍👧‍👦 (one cluster)",
				"combining: ééé (e + U+0301)",
				"mixed:    ab日cd🙂ef",
			},
			line: 3, col: 11,
		},
	}
	active := 0

	for {
		draw(scr, panes, active)
		scr.Show()

		switch ev := scr.PollEvent().(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventKey:
			switch {
			case ev.Key() == tcell.KeyCtrlC, ev.Key() == tcell.KeyEscape:
				return
			case ev.Rune() == 'q':
				return
			case ev.Key() == tcell.KeyTab:
				active = (active + 1) % len(panes)
			}
		}
	}
}

// draw lays out the whole frame: two panes, a divider, and the minibuffer.
func draw(scr tcell.Screen, panes []pane, active int) {
	scr.Clear()
	w, h := scr.Size()
	if w < 20 || h < 6 {
		blit.Draw(scr, 0, 0, w, h, "terminal too small")
		return
	}

	// The minibuffer takes the last row; panes share everything above it.
	paneH := h - 1
	dividerX := w / 2
	leftW := dividerX
	rightW := w - dividerX - 1

	drawPane(scr, 0, 0, leftW, paneH, panes[0], active == 0)
	drawDivider(scr, dividerX, 0, paneH)
	drawPane(scr, dividerX+1, 0, rightW, paneH, panes[1], active == 1)
	drawMinibuffer(scr, 0, h-1, w)
}

// drawPane renders one window: its text area with a modeline pinned to the
// bottom row.
func drawPane(scr tcell.Screen, x, y, w, h int, p pane, isActive bool) {
	if w <= 0 || h <= 1 {
		return
	}
	textH := h - 1

	body := lipgloss.NewStyle() // terminal's own foreground
	gutter := lipgloss.NewStyle().Foreground(colQuiet)

	for i := 0; i < textH; i++ {
		var row string
		if i < len(p.lines) {
			row = gutter.Render(fmt.Sprintf("%3d ", i+1)) + body.Render(p.lines[i])
		} else {
			// Emacs marks rows past end-of-buffer with a tilde in the gutter.
			row = gutter.Render("  ~ ")
		}
		blit.Draw(scr, x, y+i, w, 1, row)
	}

	blit.Draw(scr, x, y+textH, w, 1, modeline(p, w, isActive))
}

// modeline builds the status line for a pane, padded to exactly w cells.
//
// A hairline rule with the buffer name set into it: the modified mark and name
// on the left, position flush right, rule filling the span between. Focus reads
// as weight, not as a bar of colour.
func modeline(p pane, w int, isActive bool) string {
	mark := " "
	if p.modified {
		mark = lipgloss.NewStyle().Foreground(colMark).Bold(true).Render("▍")
	}

	nameStyle := lipgloss.NewStyle().Foreground(colQuiet)
	if isActive {
		nameStyle = lipgloss.NewStyle().Bold(true)
	}
	pos := fmt.Sprintf("%d:%d", p.line, p.col)

	// Width is measured in cells, not runes, which is the whole point of the
	// exercise: a miscount here bleeds into the divider column.
	nameW, posW := lipgloss.Width(p.name), lipgloss.Width(pos)
	if gap := w - 4 - nameW - posW; gap >= 1 {
		return mark + nameStyle.Render(p.name) + " " +
			lipgloss.NewStyle().Foreground(colRule).Render(strings.Repeat("─", gap)) + " " +
			lipgloss.NewStyle().Foreground(colQuiet).Render(pos) + " "
	}
	if pad := w - 2 - nameW - posW; pad >= 1 {
		return mark + nameStyle.Render(p.name) + strings.Repeat(" ", pad) +
			lipgloss.NewStyle().Foreground(colQuiet).Render(pos) + " "
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(mark + p.name)
}

// drawDivider draws the vertical rule between the two panes.
func drawDivider(scr tcell.Screen, x, y, h int) {
	rule := lipgloss.NewStyle().Foreground(colRule).Render("│")
	for i := 0; i < h; i++ {
		blit.Draw(scr, x, y+i, 1, 1, rule)
	}
}

// drawMinibuffer draws the bottom line: a nano-style key hint strip.
func drawMinibuffer(scr tcell.Screen, x, y, w int) {
	key := lipgloss.NewStyle().Bold(true)
	desc := lipgloss.NewStyle().Foreground(colQuiet)
	prompt := lipgloss.NewStyle().Foreground(colMark)

	hints := []struct{ k, d string }{
		{"C-x C-s", "save"},
		{"C-x C-c", "exit"},
		{"M-x", "command"},
		{"TAB", "other window"},
	}

	var b strings.Builder
	b.WriteString(prompt.Render(" "))
	for i, hint := range hints {
		if i > 0 {
			b.WriteString(desc.Render("   "))
		}
		b.WriteString(key.Render(hint.k))
		b.WriteString(desc.Render(" " + hint.d))
	}
	blit.Draw(scr, x, y, w, 1, b.String())
}
