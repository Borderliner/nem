package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/ui/blit"
	"github.com/hajianpour/nem/view"
)

// Frame is everything the renderer needs to draw one frame. It is a snapshot
// owned by the caller: Render reads it and does not retain it.
type Frame struct {
	// Tree arranges the windows. Required.
	Tree *view.Tree
	// Active is the window holding the cursor, unless the minibuffer does.
	Active *view.Window
	// Echo is the bottom row: an echo-area message, or the minibuffer's prompt
	// and contents when MiniOn.
	Echo string
	// MiniPt is the cursor's display column within the echo row, counted from
	// the left edge of the screen, so the caller decides how the prompt and the
	// text it has typed combine.
	MiniPt text.ColIdx
	// MiniOn reports whether a minibuffer prompt is active, which is what moves
	// the cursor to the echo row.
	MiniOn bool
}

// Renderer draws frames onto a screen.
//
// It holds one piece of state: each window's horizontal scroll offset. That
// belongs beside Top on view.Window - vertical and horizontal scroll are the
// same kind of thing - but view has no field for it, so it lives here rather
// than in a global. See the note on hscroll.
type Renderer struct {
	th Theme
	// hscroll is the leftmost visible display column per window. Vertical
	// scroll lives on view.Window as Top; this is its horizontal twin and
	// should move there if view ever grows the field.
	hscroll map[*view.Window]text.ColIdx
}

// NewRenderer returns a renderer using th.
func NewRenderer(th Theme) *Renderer {
	if th.ScrollMargin < 0 {
		th.ScrollMargin = 0
	}
	return &Renderer{th: th, hscroll: make(map[*view.Window]text.ColIdx)}
}

// Render draws f onto scr. It does not call Show; the caller decides when to
// flush, so a frame and a cursor move are one update rather than two.
func (r *Renderer) Render(scr tcell.Screen, f Frame) {
	scr.Clear()

	w, h := scr.Size()
	if w <= 0 || h <= 0 || f.Tree == nil {
		scr.HideCursor()
		return
	}

	// The echo area is a single row pinned to the bottom of the screen and is
	// not part of the split tree, exactly as emacs's minibuffer window is not.
	echoY := h - 1
	treeH := h - 1

	rects := map[*view.Window]view.Rect{}
	if treeH > 0 {
		rects = f.Tree.Layout(w, treeH)
		for win, rect := range rects {
			r.drawWindow(scr, rect, win, win == f.Active)
		}
		for _, d := range f.Tree.Dividers(w, treeH) {
			r.drawDivider(scr, d)
		}
	}
	r.forgetClosedWindows(rects)

	r.drawEcho(scr, echoY, w, f)
	r.placeCursor(scr, w, h, echoY, rects, f)
}

// drawWindow draws one pane: its visible buffer text, then its modeline.
func (r *Renderer) drawWindow(scr tcell.Screen, rect view.Rect, win *view.Window, active bool) {
	if rect.W <= 0 || rect.H <= 0 || win == nil || win.Buf == nil {
		return
	}

	textH := view.TextHeight(rect)
	if textH > 0 {
		// Vertical scroll first: afterwards point is guaranteed to lie within
		// [Top, Top+textH), which the cursor placement below relies on.
		win.ScrollToPoint(textH, r.th.ScrollMargin)
		left := r.scrollLeftFor(win, rect.W)

		for i := 0; i < textH; i++ {
			ln := win.Top + i
			if ln >= win.Buf.NumLines() {
				break // rows past the end of the buffer stay blank, as in emacs
			}
			r.drawLine(scr, rect.X, rect.Y+i, rect.W, win.Buf.Line(ln), left)
		}
	}

	// The modeline owns the bottom row of the pane, so a pane one row tall is
	// all modeline and no text.
	blit.Draw(scr, rect.X, rect.Y+rect.H-1, rect.W, 1,
		modelineString(r.th, win, rect.W, active))
}

// drawLine writes one buffer line into the cells at y, starting from display
// column left and clipped to width cells.
//
// It walks grapheme clusters, not runes: a cluster is the smallest thing that
// occupies a cell, and passing the whole cluster to SetContent lets tcell place
// its combining marks and blank the trailing cell of a wide glyph itself. That
// matters because tcell measures width with the same uniseg segmenter that text
// does, so the two cannot disagree about how many columns a glyph takes.
func (r *Renderer) drawLine(scr tcell.Screen, x, y, width int, l *text.Line, left text.ColIdx) {
	lineW := l.Width()
	truncated := lineW-left > text.ColIdx(width)

	// A truncated line gives up its last column to the marker.
	avail := text.ColIdx(width)
	if truncated {
		avail--
	}

	if avail > 0 {
		runes := l.Runes()
		for i := text.RuneIdx(0); i < l.Len(); {
			n := l.NextGrapheme(i)
			if n <= i {
				break // defensive: a non-advancing walk would spin
			}
			start := l.DisplayCol(i)
			w := l.DisplayCol(n) - start // DisplayCol(Len()) is the line width
			sx := start - left

			switch {
			case sx+w <= 0:
				// Entirely scrolled off to the left.
			case sx < 0:
				// Straddles the left edge. Drop it whole rather than draw half
				// a glyph; emacs shows nothing there either.
			case sx+w > avail:
				// Straddles the right edge. Everything after it is off-screen
				// too, so stop rather than keep walking the line.
				i = l.Len()
				continue
			case runes[i] == '\t':
				// A tab is one cluster spanning several columns, and writing a
				// literal tab would let the terminal reinterpret it.
				for c := text.ColIdx(0); c < w; c++ {
					scr.SetContent(x+int(sx+c), y, ' ', nil, r.th.Text)
				}
			default:
				scr.SetContent(x+int(sx), y, runes[i], runes[i+1:n], r.th.Text)
			}
			i = n
		}
	}

	if truncated {
		scr.SetContent(x+width-1, y, TruncMarker, nil, r.th.Trunc)
	}
}

// scrollLeftFor returns the leftmost visible column for win, having adjusted it
// so point is in view, and records it.
func (r *Renderer) scrollLeftFor(win *view.Window, width int) text.ColIdx {
	left := r.hscroll[win]
	pt := win.Buf.ClampPos(win.Pt)
	l := win.Buf.Line(pt.Line)
	ptCol := l.DisplayCol(pt.Col)

	// Two passes: how much room there is depends on whether the line overflows,
	// which depends on where we scrolled to. One correction settles it.
	for pass := 0; pass < 2; pass++ {
		usable := text.ColIdx(width)
		if l.Width()-left > text.ColIdx(width) {
			usable-- // the truncation marker takes the last column
		}
		if usable < 1 {
			usable = 1
		}
		if ptCol < left {
			left = ptCol
		}
		if ptCol > left+usable-1 {
			left = ptCol - usable + 1
		}
		if left < 0 {
			left = 0
		}
	}

	r.hscroll[win] = left
	return left
}

// forgetClosedWindows drops scroll state for windows no longer on screen, so a
// long session deleting and creating windows does not grow the map forever.
func (r *Renderer) forgetClosedWindows(live map[*view.Window]view.Rect) {
	for win := range r.hscroll {
		if _, ok := live[win]; !ok {
			delete(r.hscroll, win)
		}
	}
}

// drawDivider fills the column between two side-by-side panes. Horizontal
// splits produce none: the upper window's modeline already separates them.
func (r *Renderer) drawDivider(scr tcell.Screen, d view.Rect) {
	if d.W <= 0 || d.H <= 0 {
		return
	}
	one := r.th.Divider.Render(string(DividerRune))
	col := strings.TrimSuffix(strings.Repeat(one+"\n", d.H), "\n")
	blit.Draw(scr, d.X, d.Y, d.W, d.H, col)
}

// drawEcho draws the bottom row: a minibuffer prompt when one is active,
// otherwise whatever message the editor wants to show.
func (r *Renderer) drawEcho(scr tcell.Screen, y, width int, f Frame) {
	if y < 0 || f.Echo == "" {
		return
	}
	// A minibuffer being typed into is ordinary text and must not be dimmed
	// like a transient message.
	style := r.th.Echo
	if f.MiniOn {
		style = r.th.Mini
	}
	blit.Draw(scr, 0, y, width, 1, style.Render(f.Echo))
}

// placeCursor puts the real hardware cursor at point.
//
// This is the whole reason nem draws through tcell rather than a framework that
// owns the screen: a styled cell pretending to be a cursor is visibly wrong to
// look at all day, and it is invisible to an IME and to a screen reader.
func (r *Renderer) placeCursor(scr tcell.Screen, w, h, echoY int, rects map[*view.Window]view.Rect, f Frame) {
	if f.MiniOn {
		x := int(f.MiniPt)
		if x < 0 {
			x = 0
		}
		if x > w-1 {
			x = w - 1
		}
		scr.ShowCursor(x, echoY)
		return
	}

	rect, ok := rects[f.Active]
	if !ok || f.Active == nil || f.Active.Buf == nil {
		scr.HideCursor()
		return
	}
	textH := view.TextHeight(rect)
	if textH <= 0 || rect.W <= 0 {
		scr.HideCursor()
		return
	}

	pt := f.Active.Buf.ClampPos(f.Active.Pt)
	col := f.Active.Buf.Line(pt.Line).DisplayCol(pt.Col)

	sx := int(col - r.hscroll[f.Active])
	sy := pt.Line - f.Active.Top
	if sx < 0 {
		sx = 0
	}
	if sx > rect.W-1 {
		sx = rect.W - 1
	}
	if sy < 0 {
		sy = 0
	}
	if sy > textH-1 {
		sy = textH - 1
	}
	scr.ShowCursor(rect.X+sx, rect.Y+sy)
}
