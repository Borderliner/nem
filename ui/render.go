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

// Render draws f onto scr using th. It does not call Show; the caller decides
// when to flush, so a frame and a cursor move are one update rather than two.
//
// Rendering is a pure function of the frame, the theme and the screen: every
// piece of per-window state it needs - which line is at the top, which column is
// at the left - lives on the view.Window it is drawing. That is why this is a
// function and not a method on a renderer object.
func Render(scr tcell.Screen, f Frame, th Theme) {
	if th.ScrollMargin < 0 {
		th.ScrollMargin = 0
	}
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
			drawWindow(scr, rect, win, win == f.Active, th)
		}
		for _, d := range f.Tree.Dividers(w, treeH) {
			drawDivider(scr, d, th)
		}
	}

	drawEcho(scr, echoY, w, f, th)
	placeCursor(scr, w, h, echoY, rects, f)
}

// drawWindow draws one pane: its visible buffer text, then its modeline.
func drawWindow(scr tcell.Screen, rect view.Rect, win *view.Window, active bool, th Theme) {
	if rect.W <= 0 || rect.H <= 0 || win == nil || win.Buf == nil {
		return
	}

	textH := view.TextHeight(rect)
	if textH > 0 {
		// Scroll both axes first: afterwards point is guaranteed to lie within
		// [Top, Top+textH) and [LeftCol, LeftCol+rect.W), which the cursor
		// placement below relies on.
		win.ScrollToPoint(textH, th.ScrollMargin)
		win.ScrollToPointHorizontally(rect.W)

		// Bracket matching is computed here, from point, at draw time. A command
		// could not do it: Env cannot reach the screen by design, so it has
		// nowhere to report a highlight to. Only the active window shows it,
		// as in emacs.
		var paren parenHL
		if active {
			paren = matchParenAt(win.Buf, win.Pt, th)
		}

		for i := 0; i < textH; i++ {
			ln := win.Top + i
			if ln >= win.Buf.NumLines() {
				break // rows past the end of the buffer stay blank, as in emacs
			}
			drawLine(scr, rect.X, rect.Y+i, rect.W,
				win.Buf.Line(ln), win.LeftCol, th, paren.onLine(ln))
		}
	}

	// The modeline owns the bottom row of the pane, so a pane one row tall is
	// all modeline and no text.
	blit.Draw(scr, rect.X, rect.Y+rect.H-1, rect.W, 1,
		modelineString(th, win, rect.W, active))
}

// drawLine writes one buffer line into the cells at y, starting from display
// column left and clipped to width cells.
//
// It walks grapheme clusters, not runes: a cluster is the smallest thing that
// occupies a cell, and passing the whole cluster to SetContent lets tcell place
// its combining marks and blank the trailing cell of a wide glyph itself. That
// matters because tcell measures width with the same uniseg segmenter that text
// does, so the two cannot disagree about how many columns a glyph takes.
//
// hl carries any bracket-match highlight falling on this line.
func drawLine(scr tcell.Screen, x, y, width int, l *text.Line, left text.ColIdx, th Theme, hl lineHL) {
	lineW := l.Width()
	truncated := lineW-left > text.ColIdx(width)

	// A truncated line gives up its last column to the marker.
	avail := text.ColIdx(width)
	if truncated {
		avail--
	}

	if avail > 0 {
	walk:
		for c := range l.Clusters() {
			sx := c.Col - left
			style := hl.styleFor(c.Start, th.Text)

			switch {
			case sx+c.Width <= 0:
				// Entirely scrolled off to the left.
			case sx < 0:
				// Straddles the left edge. Drop it whole rather than draw half
				// a glyph; emacs shows nothing there either.
			case sx+c.Width > avail:
				// Straddles the right edge. Everything after it is off-screen
				// too, so stop rather than keep walking the line.
				break walk
			case c.Runes[0] == '\t':
				// A tab is one cluster spanning several columns, and writing a
				// literal tab would let the terminal reinterpret it.
				for k := text.ColIdx(0); k < c.Width; k++ {
					scr.SetContent(x+int(sx+k), y, ' ', nil, style)
				}
			default:
				scr.SetContent(x+int(sx), y, c.Runes[0], c.Runes[1:], style)
			}
		}
	}

	if truncated {
		scr.SetContent(x+width-1, y, TruncMarker, nil, th.Trunc)
	}
}

// drawDivider fills the column between two side-by-side panes. Horizontal
// splits produce none: the upper window's modeline already separates them.
func drawDivider(scr tcell.Screen, d view.Rect, th Theme) {
	if d.W <= 0 || d.H <= 0 {
		return
	}
	one := th.Divider.Render(string(DividerRune))
	col := strings.TrimSuffix(strings.Repeat(one+"\n", d.H), "\n")
	blit.Draw(scr, d.X, d.Y, d.W, d.H, col)
}

// drawEcho draws the bottom row: a minibuffer prompt when one is active,
// otherwise whatever message the editor wants to show.
func drawEcho(scr tcell.Screen, y, width int, f Frame, th Theme) {
	if y < 0 || f.Echo == "" {
		return
	}
	// A minibuffer being typed into is ordinary text and must not be dimmed
	// like a transient message.
	style := th.Echo
	if f.MiniOn {
		style = th.Mini
	}
	blit.Draw(scr, 0, y, width, 1, style.Render(f.Echo))
}

// placeCursor puts the real hardware cursor at point.
//
// This is the whole reason nem draws through tcell rather than a framework that
// owns the screen: a styled cell pretending to be a cursor is visibly wrong to
// look at all day, and it is invisible to an IME and to a screen reader.
func placeCursor(scr tcell.Screen, w, h, echoY int, rects map[*view.Window]view.Rect, f Frame) {
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

	sx := int(col - f.Active.LeftCol)
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
