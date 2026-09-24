package ui

import (
	"strings"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui/blit"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
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
	// Panels are drawn over the tiled frame, in slice order, so a later panel
	// overlaps an earlier one. They are not part of the split tree and so cannot
	// disturb its layout; view.PlacePanel decides where each one sits.
	Panels []Panel
	// CursorSet moves the cursor to CursorX, CursorY in absolute screen
	// coordinates, overriding both the active window and the echo row.
	//
	// It exists because a prompt can render inside a panel, and neither of the
	// other two can express that: MiniPt addresses a column of the echo row, and
	// the window path derives its position from point in a buffer. The caller
	// that drew the prompt into a panel already knows the exact cell, so it says
	// so rather than the renderer inferring it.
	CursorX, CursorY int
	CursorSet        bool
	// NameOf reports what each buffer is called, for the modeline. Optional: a
	// nil NameOf falls back to naming a buffer after its file, which is all the
	// renderer can work out on its own. See NameFunc.
	NameOf NameFunc
	// SpansOf reports how each line is classified, for syntax colour. Optional:
	// a nil SpansOf draws the text uncoloured. See SpansFunc.
	SpansOf SpansFunc
	// TypeOf reports each buffer's file type, and BranchOf the git branch its
	// file sits on, for the modeline's segments. Both optional: a nil source
	// simply contributes no segment. See TypeFunc and BranchFunc.
	TypeOf   TypeFunc
	BranchOf BranchFunc
	// ListingOf reports which buffers are listings rather than text. Optional:
	// a nil ListingOf treats every buffer as text. See ListingFunc.
	ListingOf ListingFunc

	// MiniRows lays a prompt's candidates out the emacs way, as Vertico does:
	// the prompt on its row and the candidates on the rows below it, down to
	// the bottom of the screen, full width. The windows give up the rows this
	// takes rather than being drawn over, so every window and its modeline stay
	// whole above the minibuffer. Empty means the minibuffer is the single echo
	// row. See also MiniNote.
	MiniRows []PanelLine
	// MiniNote is drawn quietly at the right-hand end of the prompt row, when
	// it fits beside what is typed: the candidate count.
	MiniNote string
}

// minTreeRows is what the windows keep however many candidates want the rows:
// one line of text and a modeline. A minibuffer that would take those is cut
// short instead - a list with no view of the buffer it was opened from is a
// worse trade than a shorter list.
const minTreeRows = 2

// ListingFunc reports whether a buffer is a listing - a directory, say - whose
// lines are items rather than text.
//
// A listing is drawn differently in two ways. It has no line numbers, which
// would count nothing a reader cares about. And the row point is on is drawn
// as a bar across the window, as the selected row of a completion panel is,
// because in a list the row is what point selects; a lone cursor cell in a
// column of names is easy to lose.
type ListingFunc func(*text.Buffer) bool

// isListing asks f.ListingOf, treating a missing function as "no".
func (f Frame) isListing(b *text.Buffer) bool {
	return f.ListingOf != nil && b != nil && f.ListingOf(b)
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

	// The echo area is pinned to the bottom of the screen and is not part of
	// the split tree, exactly as emacs's minibuffer window is not. It is one
	// row, or more while a prompt lists its candidates beneath it.
	miniRows := f.MiniRows
	if room := h - 1 - minTreeRows; len(miniRows) > room {
		miniRows = miniRows[:max(room, 0)]
	}
	echoY := h - 1 - len(miniRows)
	treeH := echoY

	rects := map[*view.Window]view.Rect{}
	if treeH > 0 {
		rects = f.Tree.Layout(w, treeH)
		for win, rect := range rects {
			info := modelineInfo{Name: f.NameOf, Type: f.TypeOf, Branch: f.BranchOf}
			drawWindow(scr, rect, win, win == f.Active, f.isListing(win.Buf), th, info, f.SpansOf)
		}
		for _, d := range f.Tree.Dividers(w, treeH) {
			drawDivider(scr, d, th)
		}
	}

	drawEcho(scr, echoY, w, f, th)
	drawMiniRows(scr, echoY, w, miniRows, f, th)

	// Panels last, over everything the tree drew, and before the cursor is
	// placed: a prompt rendered inside a panel needs the hardware cursor to land
	// on it, which means the panel has to exist on screen first.
	for _, p := range f.Panels {
		drawPanel(scr, p, th)
	}

	placeCursor(scr, w, h, echoY, rects, f, th)
}

// drawWindow draws one pane: its visible buffer text, then its modeline.
func drawWindow(scr tcell.Screen, rect view.Rect, win *view.Window, active, listing bool, th Theme, info modelineInfo, spansOf SpansFunc) {
	if rect.W <= 0 || rect.H <= 0 || win == nil || win.Buf == nil {
		return
	}

	textH := view.TextHeight(rect)
	if textH > 0 {
		// The gutter takes columns off the left of the pane, so everything below
		// works in the narrowed text area: textX is where the text begins and
		// textW how much of it there is. Handing drawLine those two is what puts
		// the line numbers beyond the reach of the region and the bracket match.
		gw := gutterFor(rect, win, listing, th)
		textX, textW := rect.X+gw, rect.W-gw

		// Scroll both axes first: afterwards point is guaranteed to lie within
		// [Top, Top+textH) and [LeftCol, LeftCol+textW), which the cursor
		// placement below relies on. Horizontal scrolling is given the narrowed
		// width - given the pane's full width it would believe point was visible
		// while it sat off the right edge by the width of the gutter.
		win.ScrollToPoint(textH, th.ScrollMargin)
		win.ScrollToPointHorizontally(textW)

		drawGutter(scr, rect, win, textH, active, listing, th)

		// Bracket matching is computed here, from point, at draw time. A command
		// could not do it: Env cannot reach the screen by design, so it has
		// nowhere to report a highlight to. Only the active window shows it,
		// as in emacs.
		var paren parenHL
		if active {
			paren = matchParenAt(win.Buf, win.Pt, th)
		}
		// The selection is computed the same way and in the same place, and is
		// likewise shown only in the active window. regionFor returns nothing
		// when the window is inactive or the buffer has no mark.
		region := regionFor(win.Buf, win.Pt, active, th)

		for i := 0; i < textH; i++ {
			ln := win.Top + i
			if ln >= win.Buf.NumLines() {
				break // rows past the end of the buffer stay blank, as in emacs
			}
			l := win.Buf.Line(ln)
			var spans []syntax.Span
			if spansOf != nil {
				spans = spansOf(win.Buf, ln)
			}
			reg := region.onLine(ln, l)
			if listing && active && ln == win.Pt.Line {
				// One flat bar, drawn like a selection to the window's edge.
				// The row's own colours are dropped rather than inverted:
				// inverted, each coloured field becomes a block of a different
				// colour and the bar reads as a patchwork.
				spans = nil
				reg = regionHL{on: true, to: l.Len(), toEOL: true, style: th.ListCursor}
			}
			drawLine(scr, textX, rect.Y+i, textW,
				l, win.LeftCol, th, paren.onLine(ln), reg, spans)
		}
	}

	// The modeline owns the bottom row of the pane, so a pane one row tall is
	// all modeline and no text.
	blit.Draw(scr, rect.X, rect.Y+rect.H-1, rect.W, 1,
		modelineString(th, win, rect.W, active, info))
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
// hl carries any bracket-match highlight falling on this line, reg any part of
// the selection. Where both fall on a cell they compose: the region contributes
// the inverted background and the bracket keeps its weight and underline.
func drawLine(scr tcell.Screen, x, y, width int, l *text.Line, left text.ColIdx, th Theme, hl lineHL, reg regionHL, spans []syntax.Span) {
	lineW := l.Width()
	truncated := lineW-left > text.ColIdx(width)

	// A truncated line gives up its last column to the marker.
	avail := text.ColIdx(width)
	if truncated {
		avail--
	}

	if avail > 0 {
		syn := newLineSyntax(spans, &th)
	walk:
		for c := range l.Clusters() {
			sx := c.Col - left
			// Order matters and each step only adds: syntax sets the
			// foreground, a matched bracket adds weight and an underline over
			// whatever colour that is, and the region inverts the result.
			style := syn.styleAt(c.Start, th.Text)
			style = parenOver(hl, c.Start, style, th.Text)
			if reg.covers(c.Start, c.Start+text.RuneIdx(len(c.Runes))) {
				style = overlay(style, reg.style)
			}

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

	// A selection running on to the next line is filled past this line's text to
	// the window's right edge, so a multi-line region reads as one block instead
	// of following the ragged ends of the lines. An empty line inside a
	// selection is only visible at all because of this.
	//
	// It starts at the line's full display width, so it can never overwrite the
	// trailing cell of a wide glyph. The truncation marker is deliberately left
	// out: it is chrome reporting that text continues off screen, not content,
	// and drawing it selected would claim it is part of what C-w would kill.
	if reg.on && reg.toEOL && avail > 0 {
		for sx := max(l.Width()-left, 0); sx < avail; sx++ {
			scr.SetContent(x+int(sx), y, ' ', nil, reg.style)
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

// drawMiniRows draws the candidates beneath the prompt row at y, and the
// count at the prompt row's right-hand end.
//
// Candidates are drawn as the completion panel draws its rows, match emphasis
// and selection bar included, so the two styles differ only in where the list
// sits.
func drawMiniRows(scr tcell.Screen, y, width int, rows []PanelLine, f Frame, th Theme) {
	for i, ln := range rows {
		drawPanelLine(scr, 0, y+1+i, width, ln, th)
	}
	if f.MiniNote == "" {
		return
	}
	// Only where it clears what is typed by a gap: the prompt is the thing
	// being edited and the count is a footnote to it.
	used, note := cellWidth(f.Echo), cellWidth(f.MiniNote)
	if x := width - note - 1; x >= used+2 {
		drawPanelText(scr, x, y, note, f.MiniNote, th.MiniNote, nil, nil, th)
	}
}

// cellWidth measures s in screen columns, through text.Line so it agrees with
// the text area about wide glyphs.
func cellWidth(s string) int {
	l := text.NewLine([]rune(s))
	return int(l.Width())
}

// placeCursor puts the real hardware cursor at point.
//
// This is the whole reason nem draws through tcell rather than a framework that
// owns the screen: a styled cell pretending to be a cursor is visibly wrong to
// look at all day, and it is invisible to an IME and to a screen reader.
func placeCursor(scr tcell.Screen, w, h, echoY int, rects map[*view.Window]view.Rect, f Frame, th Theme) {
	// An explicit override wins over everything, including an active prompt: when
	// completion renders the prompt inside a panel, MiniOn is still set but the
	// echo row is not where the user is typing.
	if f.CursorSet {
		scr.ShowCursor(clampInt(f.CursorX, 0, w-1), clampInt(f.CursorY, 0, h-1))
		return
	}
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

	// The same gutter width drawWindow used, from the same function, because a
	// disagreement here puts the cursor beside the character it is on.
	gw := gutterFor(rect, f.Active, f.isListing(f.Active.Buf), th)
	textW := rect.W - gw

	pt := f.Active.Buf.ClampPos(f.Active.Pt)
	col := f.Active.Buf.Line(pt.Line).DisplayCol(pt.Col)

	sx := int(col - f.Active.LeftCol)
	sy := pt.Line - f.Active.Top
	if sx < 0 {
		sx = 0
	}
	if sx > textW-1 {
		sx = textW - 1
	}
	if sy < 0 {
		sy = 0
	}
	if sy > textH-1 {
		sy = textH - 1
	}
	scr.ShowCursor(rect.X+gw+sx, rect.Y+sy)
}
