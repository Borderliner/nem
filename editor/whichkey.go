package editor

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui"
	"github.com/Borderliner/nem/view"
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
// appears. A second, as emacs's which-key waits: long enough that a pause to
// think about the second key does not throw a panel at you, short enough that
// it arrives when you are plainly stuck.
const whichKeyDefaultDelay = time.Second

// whichKeyGutter separates columns in a multi-column panel.
const whichKeyGutter = 3

// whichKeyState is which-key's whole state. It is a struct rather than loose
// fields so that Editor carries one field for the feature.
//
// The zero value means "enabled at the default delay", which is why delaySet
// exists: an explicit delay of 0 disables the feature, and that has to be
// distinguishable from never having set one.
type whichKeyState struct {
	delay    time.Duration
	delaySet bool

	// view is non-nil exactly while the panel is showing.
	view *whichKeyView

	// shows counts panels built. It exists so a test driving the real event loop
	// can prove the timer is wired without reading editor state while the loop
	// owns it.
	shows int
}

// whichKeyView is the panel as laid out when it was shown: the rows, and, in
// the popup style, the box they are drawn in. With no box the rows go at the
// foot of the screen, under the echo row that already says "C-x-", where the
// bottom completion style puts M-x's candidates.
type whichKeyView struct {
	lines []ui.PanelLine
	popup *ui.Panel
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
		e.wk.view == nil
}

// dismissWhichKey hides the panel. Called for every key, before the key is
// resolved, so dismissal never costs the keystroke that caused it.
func (e *Editor) dismissWhichKey() { e.wk.view = nil }

// decorateWithWhichKey adds the panel to a frame, in whichever style it was
// laid out for.
func (e *Editor) decorateWithWhichKey(f *ui.Frame) {
	v := e.wk.view
	switch {
	case v == nil:
	case v.popup != nil:
		f.Panels = append(f.Panels, *v.popup)
	default:
		f.MiniRows = v.lines
	}
}

// fireWhichKey builds the panel, and is what the loop calls when the timer
// expires. It re-checks its preconditions rather than trusting them: the timer
// was armed one iteration ago and the prefix may have been completed or
// cancelled since.
func (e *Editor) fireWhichKey() {
	if e.whichKeyDelay() == 0 || len(e.pending) == 0 || e.mini != nil {
		return
	}
	cs := e.continuations(e.pending)
	if len(cs) == 0 {
		return
	}
	v, ok := e.buildWhichKey(cs)
	if !ok {
		return
	}
	e.wk.view = v
	e.wk.shows++
}

// continuations is what can follow seq in every keymap in force, as the keys
// resolve: a mode's keymap comes first and shadows the global one, so in a
// listing C-x offers dired's C-x C-q beside the global C-x keys, and where a
// mode rebinds a key the panel names what that key does there.
func (e *Editor) continuations(seq []keymap.Key) []keymap.Continuation {
	stack := e.keymapStack()
	if len(stack) == 1 {
		return stack[0].Continuations(seq)
	}
	var out []keymap.Continuation
	seen := map[string]bool{}
	for _, m := range stack {
		for _, c := range m.Continuations(seq) {
			if k := c.Key.String(); !seen[k] {
				seen[k] = true
				out = append(out, c)
			}
		}
	}
	keymap.SortContinuations(out)
	return out
}

// buildWhichKey lays the continuations out for the completion style in force:
// full width at the foot of the screen, or in a box in the middle of it.
//
// Rows are laid in columns when there is room, because C-x has twenty
// continuations and a single tall column is unreadable in a short frame - and
// in a frame short enough to force truncation, columns are what decide
// whether the list fits at all.
func (e *Editor) buildWhichKey(cs []keymap.Continuation) (*whichKeyView, bool) {
	if e.scr == nil {
		return nil, false
	}
	sw, sh := e.scr.Size()
	bottom := e.comp.style == completionBottom

	// At the bottom the rows take the full width, and at most half the height
	// so the buffer stays in view; the windows always keep a text row and a
	// modeline. In the popup the border costs two of each, and the echo row is
	// never covered.
	width, maxRows := sw-2, sh-1-2
	if bottom {
		width, maxRows = sw, min(sh-1-2, max(3, sh/2))
	}

	cells := whichKeyCells(cs)
	cellW := 0
	for _, c := range cells {
		cellW = max(cellW, wkWidth(c.text))
	}
	cols, rows := whichKeyGrid(len(cells), cellW, width, maxRows)
	if cols == 0 || rows == 0 {
		return nil, false
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
		var ln ui.PanelLine
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
			at := utf8.RuneCountInString(b.String())
			for _, sp := range cells[i].spans {
				ln.Spans = append(ln.Spans, syntax.Span{Start: at + sp.Start, End: at + sp.End, Class: sp.Class})
			}
			b.WriteString(wkPad(cells[i].text, cellW))
		}
		if ln.Text = strings.TrimRight(b.String(), " "); ln.Text != "" {
			lines = append(lines, ln)
		}
	}
	if dropped > 0 {
		more := fmt.Sprintf("… %d more", dropped)
		lines = append(lines, ui.PanelLine{
			Text:  more,
			Spans: []syntax.Span{{Start: 0, End: utf8.RuneCountInString(more), Class: syntax.Comment}},
		})
	}

	if bottom {
		// Only a frame too short to spare a row for it can overflow here, and
		// there the list is cut rather than the windows squeezed out.
		return &whichKeyView{lines: lines[:min(len(lines), maxRows)]}, true
	}
	rect, ok := view.PlacePanel(view.PanelReq{
		W:      cols*cellW + (cols-1)*whichKeyGutter + 2, // +2 for the border
		H:      len(lines) + 2,
		Anchor: view.AnchorCenter,
		Frame:  view.Rect{W: sw, H: sh - 1},
	})
	if !ok {
		return nil, false
	}
	// PlacePanel may have shrunk the request, and drawPanel silently drops rows
	// past the interior. Trim here instead so the panel reports what it shows.
	if n := rect.H - 2; n >= 0 && len(lines) > n {
		lines = lines[:n]
	}
	return &whichKeyView{
		lines: lines,
		popup: &ui.Panel{Rect: rect, Title: keymap.SpecString(e.pending), Lines: lines},
	}, true
}

// whichKeyGrid chooses a column count and row count for n cells of cellW
// columns each, in width columns and at most maxRows rows.
//
// Width sets the ceiling on columns; height sets the floor, since more columns
// is how a long list is made short enough to fit. Where the two conflict the
// list is truncated, which is a better degradation than not appearing.
func whichKeyGrid(n, cellW, width, maxRows int) (cols, rows int) {
	if n == 0 || cellW <= 0 || width < 1 || maxRows < 1 {
		return 0, 0
	}
	maxCols := (width + whichKeyGutter) / (cellW + whichKeyGutter)
	if maxCols < 1 {
		maxCols = 1 // one column, truncated by the renderer, beats no panel
	}
	needCols := (n + maxRows - 1) / maxRows
	cols = min(max(needCols, 1), maxCols)
	rows = (n + cols - 1) / cols
	return cols, min(rows, maxRows)
}

// wkCell is one continuation's label and how to colour it.
type wkCell struct {
	text  string
	spans []syntax.Span
}

// whichKeyCells renders one label per continuation - the key, an arrow, and
// what it runs - with the key column padded so the arrows line up.
//
// Coloured as emacs's which-key colours them: the key as a constant, the
// arrow quiet, a command as a function, and a prefix, which leads to more
// keys rather than running anything, as a keyword.
func whichKeyCells(cs []keymap.Continuation) []wkCell {
	keyW := 0
	for _, c := range cs {
		keyW = max(keyW, wkWidth(c.Key.String()))
	}
	out := make([]wkCell, 0, len(cs))
	for _, c := range cs {
		key := c.Key.String()
		desc, class := c.Command, syntax.Function
		if c.IsPrefix {
			// A prefix has no command to name, so it reports that it leads
			// somewhere and how much is reachable beneath it.
			desc, class = fmt.Sprintf("+prefix (%d)", c.Count), syntax.Keyword
		}
		padded := wkPad(key, keyW)
		arrow := utf8.RuneCountInString(padded) + 1
		out = append(out, wkCell{
			text: padded + " → " + desc,
			spans: []syntax.Span{
				{Start: 0, End: utf8.RuneCountInString(key), Class: syntax.Constant},
				{Start: arrow, End: arrow + 1, Class: syntax.Comment},
				{Start: arrow + 2, End: arrow + 2 + utf8.RuneCountInString(desc), Class: class},
			},
		})
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
