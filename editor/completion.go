package editor

import (
	"fmt"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/fuzzy"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/ui"
	"github.com/Borderliner/nem/view"
)

// Completion shows the candidates for a prompt and narrows them as you type,
// which is the thing TAB-completes-a-common-prefix never was.
//
// The design that makes the two renderings cheap: prompt STATE and prompt
// RENDERING are independent. The minibuffer remains a real text.Buffer in a real
// view.Window — which is what makes C-a, C-k and the kill ring work inside a
// prompt — and where its contents are DRAWN is a separate decision. A floating
// panel and the echo row read the same buffer and the same candidate list, so
// completionStyle selects between them without a second state machine.
type completionStyle int

const (
	// completionPopup centres the prompt and its candidates in the frame, as a
	// command palette does. Centred rather than anchored under the current line:
	// find-file, switch-to-buffer and M-x are one gesture, and a panel that moved
	// about with point would be disorienting when the gesture did not change.
	completionPopup completionStyle = iota
	// completionBottom stacks them above the echo row, the emacs-shaped
	// rendering.
	completionBottom
)

const (
	// completionRows is how many candidates are visible at once.
	completionRows = 10
	// completionMinWidth keeps a panel readable when both the input and the
	// candidates are short, so it does not shrink to a few columns.
	completionMinWidth = 28
)

// completion is the candidate list for one prompt.
//
// It exists only when ReadOpts.Complete is non-nil. An incremental search has no
// candidates, so C-s grows no panel — the absence of a Complete function is what
// distinguishes the two, rather than the editor inspecting command names.
type completion struct {
	complete command.CompleteFunc

	// ranked is the filtered, ordered candidate list for the current input.
	ranked []fuzzy.Ranked
	// sel indexes ranked. It is reset on every refresh and clamped on every
	// change, so it can never address a candidate that has stopped existing.
	sel int
	// top is the first visible row, kept so sel stays on screen in a list
	// longer than rows.
	top int

	rows  int
	style completionStyle
}

func newCompletion(f command.CompleteFunc, input string) *completion {
	c := &completion{complete: f, rows: completionRows, style: completionPopup}
	c.refresh(input)
	return c
}

// refresh recomputes the candidates for input.
//
// The selection returns to the best match rather than trying to follow the
// previously selected candidate: after a keystroke the ranking has changed, and
// a selection that chased its old entry would land somewhere the user did not
// choose while the top of the list is what they are looking at.
func (c *completion) refresh(input string) {
	c.ranked = fuzzy.Rank(input, c.complete(input))
	c.sel, c.top = 0, 0
}

// count reports how many candidates match.
func (c *completion) count() int { return len(c.ranked) }

// selected returns the highlighted candidate.
func (c *completion) selected() (string, bool) {
	if c.sel < 0 || c.sel >= len(c.ranked) {
		return "", false
	}
	return c.ranked[c.sel].Candidate, true
}

// move steps the selection by delta, wrapping at both ends.
//
// Wrapping is deliberate: these lists are short, and C-p from the first entry to
// reach the last is a gesture people use. A selection that stopped dead at the
// ends would make the bottom of a ten-item list needlessly far away.
func (c *completion) move(delta int) {
	n := len(c.ranked)
	if n == 0 {
		c.sel, c.top = 0, 0
		return
	}
	c.sel = ((c.sel+delta)%n + n) % n
	c.scrollToSelection()
}

// scrollToSelection keeps sel inside the visible window.
func (c *completion) scrollToSelection() {
	rows := c.visibleRows()
	if rows <= 0 {
		c.top = 0
		return
	}
	if c.sel < c.top {
		c.top = c.sel
	}
	if c.sel >= c.top+rows {
		c.top = c.sel - rows + 1
	}
	if max := len(c.ranked) - rows; c.top > max {
		c.top = max
	}
	if c.top < 0 {
		c.top = 0
	}
}

// visibleRows is how many candidate rows are shown.
func (c *completion) visibleRows() int {
	if c.rows < len(c.ranked) {
		return c.rows
	}
	return len(c.ranked)
}

// isCandidate reports whether s is exactly one of the candidates.
//
// It searches the whole candidate set rather than the ranked list, because a
// value can be an exact candidate while ranking poorly, and RequireMatch asks
// whether the text names something real rather than whether it ranks well.
func (c *completion) isCandidate(s string) bool {
	for _, r := range c.ranked {
		if r.Candidate == s {
			return true
		}
	}
	return false
}

// panelFor builds the panel for a prompt, and reports where the cursor belongs.
//
// The prompt line is the panel's first row, so the text being typed sits with
// the candidates it is filtering rather than at the far side of the screen.
func (e *Editor) panelFor(ms *miniState) (ui.Panel, int, int, bool) {
	c := ms.comp
	if c == nil || e.scr == nil {
		return ui.Panel{}, 0, 0, false
	}

	sw, sh := e.scr.Size()
	// The frame excludes the echo row, which is what keeps a panel off it
	// without a special case.
	frame := view.Rect{X: 0, Y: 0, W: sw, H: sh - 1}

	promptLine := ms.line()
	rows := c.visibleRows()

	// Width is the widest thing that must fit, plus the border.
	wide := displayWidth(promptLine)
	for _, r := range c.ranked[c.top : c.top+rows] {
		if w := displayWidth(r.Candidate); w > wide {
			wide = w
		}
	}
	if wide < completionMinWidth {
		wide = completionMinWidth
	}

	// PtX and PtY are deliberately left unset: neither anchor consults them, so
	// point's screen position is not plumbed through here at all.
	anchor := view.AnchorCenter
	if c.style == completionBottom {
		anchor = view.AnchorBottom
	}

	rect, ok := view.PlacePanel(view.PanelReq{
		W:      wide + 2,     // the border
		H:      rows + 1 + 2, // candidates, the prompt row, the border
		Anchor: anchor,
		Frame:  frame,
	})
	if !ok {
		return ui.Panel{}, 0, 0, false
	}

	lines := make([]ui.PanelLine, 0, rows+1)
	lines = append(lines, ui.PanelLine{Text: promptLine})
	for i := c.top; i < c.top+rows; i++ {
		lines = append(lines, ui.PanelLine{
			Text:     c.ranked[i].Candidate,
			Match:    c.ranked[i].Match.Indices,
			Selected: i == c.sel,
		})
	}

	title := ""
	if n := len(c.ranked); n > 0 {
		title = fmt.Sprintf("%d/%d", c.sel+1, n)
	} else {
		title = "no match"
	}

	// The interior is the rect inset by the border, and the prompt is its first
	// row, so that is where the cursor goes.
	cx := rect.X + 1 + int(ms.cursorCol())
	cy := rect.Y + 1
	if maxX := rect.X + rect.W - 2; cx > maxX {
		cx = maxX
	}
	return ui.Panel{Rect: rect, Title: title, Lines: lines}, cx, cy, true
}

// displayWidth measures s in screen columns.
//
// It goes through text.Line rather than counting runes so a panel and the text
// area share one width implementation: two would eventually disagree about how
// many columns a CJK glyph or an emoji occupies.
func displayWidth(s string) int {
	l := text.NewLine([]rune(s))
	return int(l.Width())
}

// decorateWithCompletion adds the completion panel to a frame.
//
// This is the one line Redraw needs, and it is separated from the redraw itself
// precisely so that it can be: see redrawPrompt.
// completionPrefs is the config-settable part of completion.
type completionPrefs struct {
	style completionStyle
	rows  int
}

func defaultCompletionPrefs() completionPrefs {
	return completionPrefs{style: completionPopup, rows: completionRows}
}

// SetCompletionStyle selects the popup or the emacs-shaped bottom rendering.
// The value has already been validated by the config host; an unrecognised one
// here is a programming error rather than a user's typo, so it is ignored rather
// than reported to someone who cannot act on it.
func (e *Editor) SetCompletionStyle(name string) {
	switch name {
	case "popup":
		e.comp.style = completionPopup
	case "bottom":
		e.comp.style = completionBottom
	}
}

// SetCompletionRows sets how many candidates a panel shows at once.
func (e *Editor) SetCompletionRows(n int) {
	if n > 0 {
		e.comp.rows = n
	}
}

func (e *Editor) decorateWithCompletion(f *ui.Frame) {
	ms := e.mini
	if ms == nil || ms.comp == nil {
		return
	}
	// Preferences are the editor's, not the session's: a session built before a
	// config reload must still honour the new setting.
	ms.comp.style, ms.comp.rows = e.comp.style, e.comp.rows
	p, cx, cy, ok := e.panelFor(ms)
	if !ok {
		// The frame is too small for a readable panel. The prompt still works:
		// it falls back to the echo row, which is already in the frame.
		return
	}
	f.Panels = append(f.Panels, p)
	f.CursorX, f.CursorY, f.CursorSet = cx, cy, true
}
