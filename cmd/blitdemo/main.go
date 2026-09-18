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

// A small palette, kept in one place so the active and inactive treatments stay
// deliberate rather than accumulating ad hoc colours.
var (
	colText       = lipgloss.Color("252")
	colDim        = lipgloss.Color("244")
	colDivider    = lipgloss.Color("238")
	colAccent     = lipgloss.Color("170")
	colModeBgOn   = lipgloss.Color("53")
	colModeBgOff  = lipgloss.Color("236")
	colModeFgOn   = lipgloss.Color("225")
	colModeFgOff  = lipgloss.Color("245")
	colMiniPrompt = lipgloss.Color("178")
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

	body := lipgloss.NewStyle().Foreground(colText)
	gutter := lipgloss.NewStyle().Foreground(colDim)

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

// modeline builds the status bar for a pane, padded to exactly w cells.
func modeline(p pane, w int, isActive bool) string {
	fg, bg := colModeFgOff, colModeBgOff
	if isActive {
		fg, bg = colModeFgOn, colModeBgOn
	}

	flag := "--"
	if p.modified {
		flag = "**"
	}

	left := fmt.Sprintf(" %s  %s", flag, p.name)
	right := fmt.Sprintf("%d:%d ", p.line, p.col)

	// Pad the middle so the position sits flush right. Width is measured in
	// cells, not runes, which is the whole point of the exercise.
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	content := left + strings.Repeat(" ", gap) + right
	style := lipgloss.NewStyle().Foreground(fg).Background(bg)
	if isActive {
		style = style.Bold(true)
	}
	// MaxWidth guards against a pane too narrow for the content.
	return style.MaxWidth(w).Render(content)
}

// drawDivider draws the vertical rule between the two panes.
func drawDivider(scr tcell.Screen, x, y, h int) {
	rule := lipgloss.NewStyle().Foreground(colDivider).Render("│")
	for i := 0; i < h; i++ {
		blit.Draw(scr, x, y+i, 1, 1, rule)
	}
}

// drawMinibuffer draws the bottom line: a nano-style key hint strip.
func drawMinibuffer(scr tcell.Screen, x, y, w int) {
	key := lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	desc := lipgloss.NewStyle().Foreground(colDim)
	prompt := lipgloss.NewStyle().Foreground(colMiniPrompt)

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
