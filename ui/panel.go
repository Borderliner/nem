package ui

import (
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// A panel is a box drawn over the tiled frame: the completion list, the
// prefix-key discovery popup. It is not part of the split tree - view.PlacePanel
// decides where it goes and this draws it there - so it cannot disturb the
// layout invariants the tree guarantees.
//
// Panels are drawn one cell at a time rather than through the Lip Gloss bridge.
// The chrome rule in theme.go sends borders and padding through Lip Gloss, but a
// panel needs per-cell control for two things Lip Gloss cannot express here:
// emphasising individual matched runes inside a candidate, and clipping a wide
// glyph at the border without splitting it.

// Box-drawing runes for a panel's frame. Rounded corners rather than square:
// the palette's reference is letterpress, and a hard-cornered box is the thing
// that reads as a dialog from 1994.
const (
	panelTopLeft     = '╭'
	panelTopRight    = '╮'
	panelBottomLeft  = '╰'
	panelBottomRight = '╯'
	panelHorizontal  = '─'
	panelVertical    = '│'
)

// PanelLine is one row of a panel's content.
type PanelLine struct {
	Text string
	// Match holds rune indices within Text to emphasise, ascending, as
	// fuzzy.Match.Indices reports them. They are rune indices and not columns:
	// after a wide glyph the two diverge, and emphasising by column would light
	// the wrong cell. Indices outside Text are ignored rather than fatal, since
	// they arrive from another package.
	Match []int
	// Selected marks the row the user is currently on, which is filled across
	// the whole interior so it reads as a bar rather than stopping at the end of
	// its text.
	Selected bool
	// Icon, when not zero, is drawn before Text with a space after it, in the
	// colour IconClass has in the syntax palette. It is not part of Text, so
	// Match indices and the candidate a prompt returns are unaffected.
	Icon      rune
	IconClass syntax.Class
	// Spans colour parts of Text by syntax class, for a row that is more than
	// one thing - a key and the command it runs. Rune indices, as Match.
	Spans []syntax.Span
}

// Panel is a box to draw over the frame.
//
// Rect includes the border, matching view.PanelReq, so the interior is the rect
// inset by one cell on every side. A rect too small to have an interior draws
// its border alone - which is what a panel at view.MinPanelWidth by
// MinPanelHeight amounts to.
type Panel struct {
	Rect view.Rect
	// Title is set into the top border when non-empty. It is styled as border
	// rather than as content, because it labels the panel; anything the user is
	// meant to read or edit belongs in a Line.
	Title string
	Lines []PanelLine
}

// drawPanel renders one panel.
//
// The first thing it does is clear its rect, and that is load-bearing: without
// it the buffer text underneath shows through every cell a glyph does not fill,
// which is unreadable.
//
// Clearing looks like a violation of the palette rule that nothing paints a
// background, and is not. The fill uses tcell.StyleDefault, which *is* the
// terminal's own background - so the panel becomes opaque without introducing a
// colour the palette does not own, and it stays correct on a light terminal and
// a dark one alike.
func drawPanel(scr tcell.Screen, p Panel, th Theme) {
	sw, sh := scr.Size()
	r, ok := clipToScreen(p.Rect, sw, sh)
	if !ok {
		return
	}

	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			scr.SetContent(x, y, ' ', nil, tcell.StyleDefault)
		}
	}

	drawPanelBorder(scr, r, p.Title, th)

	// The interior is the rect inset by the border. Both dimensions can vanish
	// on a small panel, which then shows its border and nothing else.
	ix, iy := r.X+1, r.Y+1
	iw, ih := r.W-2, r.H-2
	if iw <= 0 || ih <= 0 {
		return
	}
	for i, ln := range p.Lines {
		if i >= ih {
			break // a longer list is the caller's to scroll, not ours to spill
		}
		drawPanelLine(scr, ix, iy+i, iw, ln, th)
	}
}

// clipToScreen intersects r with the screen, reporting whether anything of it
// survives. A panel placed by view.PlacePanel is already inside the frame, but a
// caller can hand over a stale rect from before a resize, and a renderer must
// clip rather than panic.
func clipToScreen(r view.Rect, sw, sh int) (view.Rect, bool) {
	if r.W <= 0 || r.H <= 0 || sw <= 0 || sh <= 0 {
		return view.Rect{}, false
	}
	x0, y0 := max(r.X, 0), max(r.Y, 0)
	x1, y1 := min(r.X+r.W, sw), min(r.Y+r.H, sh)
	if x1 <= x0 || y1 <= y0 {
		return view.Rect{}, false
	}
	return view.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, true
}

// clampInt confines v to [lo, hi]. An inverted range yields lo, which keeps a
// zero-size screen from producing a negative coordinate.
func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

// drawPanelBorder outlines r and sets title into its top edge.
func drawPanelBorder(scr tcell.Screen, r view.Rect, title string, th Theme) {
	st := th.PanelBorder
	right, bottom := r.X+r.W-1, r.Y+r.H-1

	for x := r.X; x <= right; x++ {
		scr.SetContent(x, r.Y, panelHorizontal, nil, st)
		if bottom != r.Y {
			scr.SetContent(x, bottom, panelHorizontal, nil, st)
		}
	}
	for y := r.Y; y <= bottom; y++ {
		scr.SetContent(r.X, y, panelVertical, nil, st)
		if right != r.X {
			scr.SetContent(right, y, panelVertical, nil, st)
		}
	}

	// Corners last, so they win over the edges on a one-cell-thin panel.
	scr.SetContent(r.X, r.Y, panelTopLeft, nil, st)
	if right != r.X {
		scr.SetContent(right, r.Y, panelTopRight, nil, st)
	}
	if bottom != r.Y {
		scr.SetContent(r.X, bottom, panelBottomLeft, nil, st)
		if right != r.X {
			scr.SetContent(right, bottom, panelBottomRight, nil, st)
		}
	}

	if title != "" {
		// Inset two cells so the title clears both corners, and padded with a
		// space either side so the label reads as set into the rule rather than
		// running straight into it.
		drawPanelText(scr, r.X+2, r.Y, r.W-4, " "+title+" ", st, nil, nil, th)
	}
}

// drawPanelLine draws one content row, clipped to width columns.
func drawPanelLine(scr tcell.Screen, x, y, width int, ln PanelLine, th Theme) {
	base := th.Text
	if ln.Selected {
		base = overlay(base, th.PanelSelected)
		// Fill the full interior width first, so the selection reads as a bar
		// across the panel instead of ending where the candidate's text does.
		for k := 0; k < width; k++ {
			scr.SetContent(x+k, y, ' ', nil, base)
		}
	}
	if ln.Icon != 0 && width > 2 {
		// On the selected row the icon takes the bar's style like everything
		// else on it: coloured, the cell would invert to a block of colour.
		style := base
		if !ln.Selected {
			style = th.syntaxStyle(ln.IconClass)
		}
		scr.SetContent(x, y, ln.Icon, nil, style)
		x, width = x+2, width-2
	}
	drawPanelText(scr, x, y, width, ln.Text, base, ln.Match, ln.Spans, th)
}

// drawPanelText writes s at (x, y) clipped to width columns, emphasising the
// rune indices in match.
//
// Measuring goes through text.Line rather than counting runes, so a panel and
// the text area agree about how many columns a glyph occupies - they share one
// width implementation instead of two that can drift. A panel is a handful of
// short lines drawn once a frame, so building a Line per row costs nothing next
// to the text area it sits over.
func drawPanelText(scr tcell.Screen, x, y, width int, s string, base tcell.Style, match []int, spans []syntax.Span, th Theme) {
	if width <= 0 || s == "" {
		return
	}
	l := text.NewLine([]rune(s))

	// match is ascending, and so are the clusters, so one cursor walks both.
	// spans are walked the same way, by the draw loop's own walker.
	mi := 0
	syn := newLineSyntax(spans, &th)
	for c := range l.Clusters() {
		if int(c.Col)+int(c.Width) > width {
			// This cluster straddles the right edge; everything after it is
			// further out still, so stop rather than keep walking. Drawing it
			// would put half a wide glyph over the border.
			break
		}

		for mi < len(match) && text.RuneIdx(match[mi]) < c.Start {
			mi++ // an index before this cluster: out of range, or already used
		}
		style := base
		if len(spans) > 0 {
			// Laid over base rather than replacing it, so a coloured span on
			// a selected row keeps the bar.
			style = overlay(base, syn.styleAt(c.Start, base))
		}
		end := c.Start + text.RuneIdx(len(c.Runes))
		if mi < len(match) && text.RuneIdx(match[mi]) < end {
			style = overlay(style, th.PanelMatch)
			for mi < len(match) && text.RuneIdx(match[mi]) < end {
				mi++ // a cluster can hold several matched runes
			}
		}

		if c.Runes[0] == '\t' {
			// Writing a literal tab would let the terminal reinterpret it, so
			// the cluster's columns are spaced out by hand.
			for k := text.ColIdx(0); k < c.Width; k++ {
				scr.SetContent(x+int(c.Col+k), y, ' ', nil, style)
			}
			continue
		}
		scr.SetContent(x+int(c.Col), y, c.Runes[0], c.Runes[1:], style)
	}
}
