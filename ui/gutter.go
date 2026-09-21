package ui

import (
	"strconv"

	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// The line-number gutter is drawn to the left of a pane's text, in columns the
// text area never sees.
//
// That is the whole design. Line numbers are not buffer content, so C-w and M-w
// - which work in text.Pos buffer positions - cannot copy them. And because
// drawLine is handed an x and a width that begin after the gutter, neither the
// region highlighter nor the bracket matcher can reach those columns either.
// Both properties fall out of the layout rather than from a rule anybody has to
// remember: there is no "skip the gutter" branch anywhere, and if one ever seems
// necessary the layout has gone wrong.

// minGutterText is how many columns of text a pane must keep for the gutter to
// be worth its space. Below this the gutter is dropped entirely: text is the
// point of a window and numbers are an aid to reading it, so when only one can
// fit it is not a close call.
const minGutterText = 4

// gutterWidth is the intrinsic width of win's gutter: the digits of its last
// line number plus one blank column separating the numbers from the text.
//
// It is derived from the buffer's line count rather than from the rows on
// screen, so the numbers do not shift sideways the moment you scroll past line
// 99.
func gutterWidth(win *view.Window, th Theme) int {
	if !th.LineNumbers || win == nil || win.Buf == nil {
		return 0
	}
	n := win.Buf.NumLines()
	if n < 1 {
		n = 1
	}
	return len(strconv.Itoa(n)) + 1
}

// gutterFor is the width the gutter actually gets in this pane, after the
// narrow-pane rule.
//
// Both drawWindow and placeCursor go through this rather than computing it
// separately. If they ever disagreed the cursor would sit a few columns away
// from the character it is on, which is the most visible bug this feature could
// have.
func gutterFor(rect view.Rect, win *view.Window, th Theme) int {
	gw := gutterWidth(win, th)
	if gw == 0 || rect.W-gw < minGutterText {
		return 0
	}
	return gw
}

// drawGutter writes the visible line numbers down the left of rect.
//
// Numbers are right-aligned so their digits line up as the count grows, and the
// separating column is left blank so the gutter reads as a margin rather than a
// border. Rows past the end of the buffer get no number, matching the blank text
// rows beside them.
func drawGutter(scr tcell.Screen, rect view.Rect, win *view.Window, textH int, active bool, th Theme) {
	gw := gutterFor(rect, win, th)
	if gw == 0 || textH <= 0 || rect.H <= 0 {
		return
	}

	// Only the active window marks its current line, for the same reason only
	// the active window shows a region or a bracket match: highlighting it in a
	// second window onto the same buffer would claim something the user did not
	// do there.
	cur := -1
	if active {
		cur = win.Buf.ClampPos(win.Pt).Line
	}

	digits := gw - 1 // the last column is the separator
	for i := 0; i < textH; i++ {
		ln := win.Top + i
		if ln >= win.Buf.NumLines() {
			break
		}

		style := th.LineNumber
		if ln == cur {
			style = th.LineNumberCurrent
		}

		s := strconv.Itoa(ln + 1)
		// A number wider than the gutter cannot happen - the width came from the
		// largest of them - but clipping beats drawing over the text if it ever
		// did.
		if len(s) > digits {
			s = s[len(s)-digits:]
		}
		pad := digits - len(s)
		for k, r := range s {
			scr.SetContent(rect.X+pad+k, rect.Y+i, r, nil, style)
		}
	}
}
