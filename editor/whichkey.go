package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/hajianpour/nem/keymap"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/ui"
	"github.com/hajianpour/nem/view"
)

// Prefix-key discovery: press C-x, pause, and a panel lists what can follow.
//
// This is nem going past emacs rather than imitating it. The largest
// discoverability gap in emacs is that a prefix key gives no indication of what
// it leads to, so the bindings you do not already know stay invisible.
//
// The feature must cost a fluent user nothing, which shapes the whole design:
// the panel appears only after a delay, and any real key both dismisses it and
// is acted on normally. Someone who types C-x u without pausing never sees it.
//
// Timing and the decision to show are deliberately separate. The event loop owns
// the clock and calls fireWhichKey when a timer expires; everything about
// whether and what to show lives here, where it can be tested without one.

// whichKeyDefaultDelay is how long a prefix must sit pending before the panel
// appears. Long enough that deliberate two-key sequences never trigger it,
// short enough that a pause means the panel arrives before you go looking.
const whichKeyDefaultDelay = 300 * time.Millisecond

// whichKeyGutter separates columns in a multi-column panel.
const whichKeyGutter = 2

// whichKeyState is which-key's whole state. It is a struct rather than loose
// fields so that Editor carries one field for the feature.
//
// The zero value means "enabled at the default delay", which is why delaySet
// exists: an explicit delay of 0 disables the feature, and that has to be
// distinguishable from never having set one.
type whichKeyState struct {
	delay    time.Duration
	delaySet bool

	// panel is non-nil exactly while the panel is showing.
	panel *ui.Panel

	// shows counts panels built. It exists so a test driving the real event loop
	// can prove the timer is wired without reading editor state while the loop
	// owns it.
	shows int
}

// SetWhichKeyDelay sets how long a prefix sits pending before the panel
// appears. Zero disables prefix-key discovery entirely.
func (e *Editor) SetWhichKeyDelay(d time.Duration) {
	if d < 0 {
		d = 0
	}
	e.wk.delay, e.wk.delaySet = d, true
}

// whichKeyDelay is the delay in effect, defaulting when none was set.
func (e *Editor) whichKeyDelay() time.Duration {
	if e.wk.delaySet {
		return e.wk.delay
	}
	return whichKeyDefaultDelay
}

// whichKeyArmed reports whether the event loop should be running a timer.
//
// Not while a prompt is open: a prompt is what the user is looking at, and a
// panel describing a prefix would cover it. Not while the panel is already
// showing either, or the timer would re-fire on every frame.
func (e *Editor) whichKeyArmed() bool {
	return e.whichKeyDelay() > 0 &&
		len(e.pending) > 0 &&
		e.mini == nil &&
		e.wk.panel == nil
}

// dismissWhichKey hides the panel. Called for every key, before the key is
// resolved, so dismissal never costs the keystroke that caused it.
func (e *Editor) dismissWhichKey() { e.wk.panel = nil }

// whichKeyPanel is the panel to draw, or nil.
func (e *Editor) whichKeyPanel() *ui.Panel { return e.wk.panel }

// fireWhichKey builds the panel, and is what the loop calls when the timer
// expires. It re-checks its preconditions rather than trusting them: the timer
// was armed one iteration ago and the prefix may have been completed or
// cancelled since.
func (e *Editor) fireWhichKey() {
	if e.whichKeyDelay() == 0 || len(e.pending) == 0 || e.mini != nil {
		return
	}
	cs := e.keys.Continuations(e.pending)
	if len(cs) == 0 {
		return
	}
	p, ok := e.buildWhichKeyPanel(cs)
	if !ok {
		return
	}
	e.wk.panel = &p
	e.wk.shows++
}

// buildWhichKeyPanel lays the continuations out and places the panel.
//
// Rows are laid in columns when the frame allows, because C-x has fifteen
// continuations and a single tall column is unreadable in a short frame - and in
// a frame short enough to force truncation, columns are what decide whether the
// list fits at all.
func (e *Editor) buildWhichKeyPanel(cs []keymap.Continuation) (ui.Panel, bool) {
	if e.scr == nil {
		return ui.Panel{}, false
	}
	sw, sh := e.scr.Size()
	// The echo row is excluded here rather than guarded downstream, which is
	// what makes "never cover the echo row" reduce to "stay inside Frame".
	frame := view.Rect{W: sw, H: sh - 1}

	cells := whichKeyCells(cs)
	cellW := 0
	for _, s := range cells {
		cellW = max(cellW, wkWidth(s))
	}

	cols, rows := whichKeyGrid(len(cells), cellW, frame)
	if cols == 0 || rows == 0 {
		return ui.Panel{}, false
	}

	// A frame too short for every continuation drops some, and dropping them
	// silently would let the panel claim to be the whole answer. One row is
	// given up to say how many are missing, so the list is either complete or
	// admits that it is not.
	dropped := len(cells) - cols*rows
	if dropped > 0 && rows > 1 {
		rows--
		dropped = len(cells) - cols*rows
	}

	lines := make([]ui.PanelLine, 0, rows+1)
	for r := 0; r < rows; r++ {
		var b strings.Builder
		for c := 0; c < cols; c++ {
			// Column-major fill, as which-key does: reading down a column keeps
			// the sorted order contiguous, where reading across rows scatters it.
			i := c*rows + r
			if i >= len(cells) {
				break
			}
			if c > 0 {
				b.WriteString(strings.Repeat(" ", whichKeyGutter))
			}
			b.WriteString(wkPad(cells[i], cellW))
		}
		if s := strings.TrimRight(b.String(), " "); s != "" {
			lines = append(lines, ui.PanelLine{Text: s})
		}
	}

	if dropped > 0 {
		lines = append(lines, ui.PanelLine{Text: fmt.Sprintf("… %d more", dropped)})
	}

	wantW := cols*cellW + (cols-1)*whichKeyGutter + 2 // +2 for the border
	rect, ok := view.PlacePanel(view.PanelReq{
		W: wantW, H: len(lines) + 2,
		// Bottom, not at point: which-key is describing the keyboard rather than
		// the text, and a box over the line you are editing is in the way.
		Anchor: view.AnchorBottom,
		Frame:  frame,
	})
	if !ok {
		return ui.Panel{}, false
	}

	// PlacePanel may have shrunk the request, and drawPanel silently drops rows
	// past the interior. Trim here instead so the panel reports what it shows.
	if n := rect.H - 2; n >= 0 && len(lines) > n {
		lines = lines[:n]
	}
	return ui.Panel{Rect: rect, Title: keymap.SpecString(e.pending), Lines: lines}, true
}

// whichKeyGrid chooses a column count and row count for n cells of cellW
// columns each, within frame.
//
// Width sets the ceiling on columns; height sets the floor, since more columns
// is how a long list is made short enough to fit. Where the two conflict the
// list is truncated, which is a better degradation than not appearing.
func whichKeyGrid(n, cellW int, frame view.Rect) (cols, rows int) {
	if n == 0 || cellW <= 0 {
		return 0, 0
	}
	interiorW := frame.W - 2
	availRows := frame.H - 2
	if interiorW < 1 || availRows < 1 {
		return 0, 0
	}

	maxCols := (interiorW + whichKeyGutter) / (cellW + whichKeyGutter)
	if maxCols < 1 {
		maxCols = 1 // one column, truncated by the renderer, beats no panel
	}
	needCols := (n + availRows - 1) / availRows
	cols = min(max(needCols, 1), maxCols)
	rows = (n + cols - 1) / cols
	return cols, min(rows, availRows)
}

// whichKeyCells renders one label per continuation, key column padded so the
// descriptions line up.
func whichKeyCells(cs []keymap.Continuation) []string {
	keyW := 0
	for _, c := range cs {
		keyW = max(keyW, wkWidth(c.Key.String()))
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		desc := c.Command
		if c.IsPrefix {
			// A prefix has no command to name, so it reports that it leads
			// somewhere and how much is reachable beneath it.
			desc = fmt.Sprintf("+prefix (%d)", c.Count)
		}
		out = append(out, wkPad(c.Key.String(), keyW)+"  "+desc)
	}
	return out
}

// wkWidth measures s in display columns.
//
// Through text.Line rather than counting runes, for the same reason ui does it:
// one width implementation shared with the text area cannot drift from it, and a
// key label can hold a multi-byte rune.
func wkWidth(s string) int {
	l := text.NewLine([]rune(s))
	return int(l.Width())
}

// wkPad right-pads s to w display columns.
func wkPad(s string, w int) string {
	if d := w - wkWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
