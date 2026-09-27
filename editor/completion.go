package editor

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/fuzzy"
	"github.com/Borderliner/nem/icons"
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
// prompt — and where its contents are DRAWN is a separate decision. The bottom
// of the screen and a floating panel read the same buffer and the same
// candidate list, so completionStyle selects between them without a second
// state machine.
type completionStyle int

const (
	// completionBottom is the emacs shape, and Vertico's: the prompt at the
	// foot of the screen with its candidates listed below it, full width, the
	// windows shrinking to make room. It is the default because it is where
	// an emacs user's eyes already go, and it covers none of the buffer.
	completionBottom completionStyle = iota
	// completionPopup centres the prompt and its candidates in the frame, as a
	// command palette does. Centred rather than anchored under the current line:
	// find-file, switch-to-buffer and M-x are one gesture, and a panel that moved
	// about with point would be disorienting when the gesture did not change.
	completionPopup
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
	// icon gives each candidate its icon, when the prompt offers one.
	icon func(string) icons.Icon
	// annotate gives each candidate its note, when the prompt offers one.
	annotate func(string) string
	// history, when set, puts the candidates entered before first, most
	// recent first, while nothing is typed. See ReadOpts.HistoryFirst.
	history []string
	// peak is the most candidate rows this prompt has shown. At the bottom of
	// the screen the list keeps that height as it narrows, as emacs's
	// grow-only minibuffer does: were it to shrink with every keystroke, the
	// windows above would change size under the user as they typed.
	peak int

	// keepOrder lists the matches in the candidates' own order. See
	// ReadOpts.KeepOrder.
	keepOrder bool

	// The last ranking, for the next keystroke's to narrow, as fzf does: the
	// list Complete gave, the input and how much of it was cut, the
	// candidates that matched, in the list's order, and how they ranked.
	// While the list is the same and what is typed narrows what was - one
	// more letter, one more word - only those are ranked, not the whole list
	// again: the first keystroke in a large project ranks every file, the
	// next only the files that one left. And a keystroke that leaves the
	// query as it was, a space or a ' before the next word, ranks nothing.
	lastCands  []string
	lastInput  string
	lastCut    int
	lastPool   []string
	lastRanked []fuzzy.Ranked

	// A live list's candidates are a background search's results - see
	// ReadOpts.Search. gen counts the searches started, so results that
	// arrive after a newer search began are known for stale; stop stops the
	// one running; pending says it has not come back yet.
	live    bool
	gen     int
	stop    *atomic.Bool
	pending bool
}

// newLiveCompletion is a list whose candidates a search will supply.
func newLiveCompletion() *completion {
	return &completion{rows: completionRows, style: completionBottom, live: true}
}

// setFound makes a live search's results the candidates, in its order, the
// parts that matched highlighted.
func (c *completion) setFound(found []command.Found) {
	c.ranked = c.ranked[:0]
	for _, f := range found {
		var idx []int
		for _, sp := range f.Spans {
			for i := sp[0]; i < sp[1]; i++ {
				idx = append(idx, i)
			}
		}
		c.ranked = append(c.ranked, fuzzy.Ranked{Candidate: f.Text, Match: fuzzy.Match{Indices: idx}})
	}
	c.sel, c.top, c.pending = 0, 0, false
}

func newCompletion(f command.CompleteFunc, input string) *completion {
	c := &completion{complete: f, rows: completionRows, style: completionBottom}
	c.refresh(input)
	return c
}

// refresh recomputes the candidates for input.
//
// The selection returns to the best match rather than trying to follow the
// previously selected candidate: after a keystroke the ranking has changed, and
// a selection that chased its old entry would land somewhere the user did not
// choose while the top of the list is what they are looking at.
//
// A candidate equal to the input goes first, whatever it scored. The scorer
// rewards word boundaries, so for "foobar" it ranks fooBar above foobar itself,
// and since RET takes the highlight, a name typed out in full would otherwise
// open a different one. Promoting it here rather than special-casing it at RET
// keeps the two in agreement: what is highlighted is what RET takes, and C-n
// off the exact match is still honoured.
func (c *completion) refresh(input string) {
	if c.live {
		return // the search's results come in their own time; see setFound
	}
	cands := c.complete(input)
	cut := cutLen(input, cands)
	c.sel, c.top = 0, 0
	sameCands := sameList(cands, c.lastCands) && c.lastPool != nil
	if sameCands && input != "" && c.lastInput != "" && cut == c.lastCut && fuzzy.Same(c.lastInput[cut:], input[cut:]) {
		// The ranking is the last one, and so is what matched. It was
		// only ever rearranged in place into the list's order, which is
		// where keepOrder leaves it again; the history, which it is not,
		// is only for an empty input.
		c.ranked = c.lastRanked
	} else {
		pool := cands
		if sameCands && fuzzy.Narrows(c.lastInput, input) {
			pool = c.lastPool
		}
		c.ranked = rankPast(input, pool, cut)
		// What matched, in the list's order, for the next keystroke.
		// Ranked's Index is a place in the pool, which is in the list's
		// order itself.
		matched := make([]bool, len(pool))
		for _, r := range c.ranked {
			matched[r.Index] = true
		}
		next := make([]string, 0, len(c.ranked))
		for i, ok := range matched {
			if ok {
				next = append(next, pool[i])
			}
		}
		c.lastPool = next
	}
	c.lastCands, c.lastInput, c.lastCut, c.lastRanked = cands, input, cut, c.ranked
	if c.keepOrder && input != "" {
		slices.SortFunc(c.ranked, func(a, b fuzzy.Ranked) int { return a.Index - b.Index })
		return
	}
	if input == "" && len(c.history) > 0 {
		at := make(map[string]int, len(c.history))
		for i, h := range c.history {
			if _, dup := at[h]; !dup {
				at[h] = i
			}
		}
		slices.SortStableFunc(c.ranked, func(a, b fuzzy.Ranked) int {
			ia, oka := at[a.Candidate]
			ib, okb := at[b.Candidate]
			switch {
			case oka && okb:
				return ia - ib
			case oka:
				return -1
			case okb:
				return 1
			}
			return 0
		})
	}
	for i, r := range c.ranked {
		if r.Candidate == input {
			if i > 0 {
				// Into a copy: the ranking is kept for the next
				// keystroke, which may not type this candidate.
				promoted := make([]fuzzy.Ranked, 0, len(c.ranked))
				promoted = append(promoted, r)
				promoted = append(promoted, c.ranked[:i]...)
				c.ranked = append(promoted, c.ranked[i+1:]...)
			}
			break
		}
	}
}

// sameList reports whether a and b are the one list: Complete giving back the
// list it gave before, as a fixed list's does, rather than a new one.
func sameList(a, b []string) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// rankPastSharedPrefix ranks cands against input, leaving out the longest
// prefix of input that every candidate also begins with.
//
// Such a prefix says nothing about which candidate is wanted - at find-file it
// is the directory being listed - but the scorer would still weigh it, and it
// breaks ties by length. So walking into a directory highlighted its shortest
// entry, a dotfile as often as not, when RET should find the first. Ranked on
// what follows it, a directory just entered keeps its listing order, as a
// prompt does before the first keystroke. Which candidates match is unchanged,
// since the prefix is cut only where cutting it changes nothing else: not from
// a$ over candidates all starting with a, which would be left as $, and match
// every one of them. cutLen has the rule.
func rankPastSharedPrefix(input string, cands []string) []fuzzy.Ranked {
	return rankPast(input, cands, cutLen(input, cands))
}

// cutLen is how much of input rankPastSharedPrefix cuts off: the prefix every
// candidate shares with it, where cutting it leaves the query matching what it
// matched whole.
//
// That holds for a query of one plain fuzzy word, cut anywhere that does not
// leave the rest of it starting with a marker: the shared prefix matches in
// place, so the candidates matching the word are those whose rest matches the
// rest of it. And it holds for plain words cut whole, all of them: each is in
// the prefix, and so matches every candidate, as the nothing left does. A
// directory just entered is that, even with spaces in its path. Anything
// else - a word marked ', ^, ! or $, a |, an escaped space, or a word cut in
// two when there are others - could match otherwise, and is not cut. A
// backslash is only an escape before a space, and a Windows path is cut as
// any other.
func cutLen(input string, cands []string) int {
	n := sharedPrefixLen(input, cands)
	if n == 0 || !plainWords(input) {
		return 0
	}
	if n == len(input) {
		return n
	}
	if rest := input[n:]; strings.Contains(input, " ") || strings.ContainsRune("'^!", rune(rest[0])) || rest == "|" {
		return 0
	}
	return n
}

// plainWords reports whether every word of input is a plain fuzzy term in
// fzf's syntax, as Rank reads it: none marked ', ^, ! or $, none a | between
// others, and no space escaped into one.
func plainWords(input string) bool {
	if strings.Contains(input, "\\ ") {
		return false
	}
	for _, w := range strings.Split(input, " ") {
		if w == "|" || w != "" && (strings.ContainsRune("'^!", rune(w[0])) || strings.HasSuffix(w, "$")) {
			return false
		}
	}
	return true
}

// rankPast ranks cands against input past its first n bytes, which every
// candidate starts with. n comes from the whole list, not from cands, when
// cands are what the last keystroke left: those may share more, and cutting
// more would score them otherwise than the whole list's ranking does.
func rankPast(input string, cands []string, n int) []fuzzy.Ranked {
	if n == 0 {
		return fuzzy.Rank(input, cands)
	}
	tails := make([]string, len(cands))
	for i, cand := range cands {
		tails[i] = cand[n:]
	}
	ranked := fuzzy.Rank(input[n:], tails)
	head, shift := input[:n], utf8.RuneCountInString(input[:n])
	for i := range ranked {
		ranked[i].Candidate = head + ranked[i].Candidate
		for j := range ranked[i].Match.Indices {
			ranked[i].Match.Indices[j] += shift
		}
	}
	return ranked
}

// sharedPrefixLen returns the length in bytes of the longest prefix of input
// that every candidate begins with, ending on a rune boundary.
func sharedPrefixLen(input string, cands []string) int {
	if len(cands) == 0 {
		return 0
	}
	n := len(input)
	for _, cand := range cands {
		m := 0
		for m < n && m < len(cand) && cand[m] == input[m] {
			m++
		}
		if n = m; n == 0 {
			return 0
		}
	}
	for n < len(input) && n > 0 && !utf8.RuneStart(input[n]) {
		n--
	}
	return n
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

// page moves the selection d screenfuls down the list, up for a negative d,
// and the list with it, so the selection keeps its row as point keeps its
// line when C-v scrolls a window. Unlike move it stops at the ends: a page
// past the last candidate lands on the last.
func (c *completion) page(d int) {
	n, rows := len(c.ranked), c.visibleRows()
	if n == 0 || rows <= 0 {
		return
	}
	c.sel = min(max(c.sel+d*rows, 0), n-1)
	c.top += d * rows
	c.scrollToSelection()
}

// goTo selects candidate i, the first or the last.
func (c *completion) goTo(i int) {
	if n := len(c.ranked); n > 0 {
		c.sel = min(max(i, 0), n-1)
		c.scrollToSelection()
	}
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

	cands := make([]ui.PanelLine, 0, rows)
	for i := c.top; i < c.top+rows; i++ {
		cands = append(cands, e.candidateLine(c, i))
	}
	alignNotes(cands)

	// Width is the widest thing that must fit, plus the border: the prompt,
	// or a candidate with its icon before it and its note after.
	wide := displayWidth(promptLine)
	for _, ln := range cands {
		w := displayWidth(ln.Text)
		if ln.Note != "" {
			w = ln.NoteCol + displayWidth(ln.Note)
		}
		if ln.Icon != 0 {
			w += 2
		}
		wide = max(wide, w)
	}
	if wide < completionMinWidth {
		wide = completionMinWidth
	}

	// PtX and PtY are deliberately left unset: the centre anchor does not
	// consult them, so point's screen position is not plumbed through here.
	rect, ok := view.PlacePanel(view.PanelReq{
		W:      wide + 2,     // the border
		H:      rows + 1 + 2, // candidates, the prompt row, the border
		Anchor: view.AnchorCenter,
		Frame:  frame,
	})
	if !ok {
		return ui.Panel{}, 0, 0, false
	}

	lines := make([]ui.PanelLine, 0, rows+1)
	lines = append(lines, ui.PanelLine{Text: promptLine})
	lines = append(lines, cands...)

	title := c.countNote()

	// The interior is the rect inset by the border, and the prompt is its first
	// row, so that is where the cursor goes.
	cx := rect.X + 1 + int(ms.cursorCol())
	cy := rect.Y + 1
	if maxX := rect.X + rect.W - 2; cx > maxX {
		cx = maxX
	}
	return ui.Panel{Rect: rect, Title: title, Lines: lines}, cx, cy, true
}

// countNote is the position readout: which candidate is selected, of how many.
func (c *completion) countNote() string {
	switch n := len(c.ranked); {
	case n > 0 && c.pending:
		return fmt.Sprintf("%d/%d …", c.sel+1, n)
	case n > 0:
		return fmt.Sprintf("%d/%d", c.sel+1, n)
	case c.pending:
		return "searching …"
	}
	return "no match"
}

// bottomRows lays the candidates out beneath the prompt for the bottom style,
// in a screen sh rows tall, reporting false when there is no room for any.
func (e *Editor) bottomRows(c *completion, sh int) ([]ui.PanelLine, bool) {
	// The prompt takes a row and the windows keep a text row and a modeline;
	// the list gets what is left, up to the configured count.
	room := sh - 1 - 2
	if room < 1 {
		return nil, false
	}
	if c.rows > room {
		c.rows = room
		c.scrollToSelection()
	}
	shown := c.visibleRows()
	c.peak = max(c.peak, shown)
	lines := make([]ui.PanelLine, min(c.peak, c.rows))
	for i := range shown {
		lines[i] = e.candidateLine(c, c.top+i)
	}
	alignNotes(lines[:shown])
	return lines, true
}

// candidateLine is the row for candidate idx, with its icon when the prompt
// has them and they are switched on.
func (e *Editor) candidateLine(c *completion, idx int) ui.PanelLine {
	ln := ui.PanelLine{
		Text:     c.ranked[idx].Candidate,
		Match:    c.ranked[idx].Match.Indices,
		Selected: idx == c.sel,
	}
	if e.icons && c.icon != nil {
		ic := c.icon(ln.Text)
		ln.Icon, ln.IconClass = ic.Glyph, ic.Class
	}
	if c.annotate != nil {
		ln.Note = c.annotate(ln.Text)
	}
	return ln
}

// alignNotes puts the notes of the rows shown in one column, two past the
// widest candidate among them, so they read as a column rather than a ragged
// edge. The widest on screen rather than of them all: a long name scrolled
// out of view should not push the notes of what is visible away from it.
func alignNotes(lines []ui.PanelLine) {
	col := 0
	for _, ln := range lines {
		col = max(col, displayWidth(ln.Text))
	}
	for i := range lines {
		lines[i].NoteCol = col + 2
	}
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
	return completionPrefs{style: completionBottom, rows: completionRows}
}

// SetCompletionStyle selects the emacs-shaped bottom rendering or the popup.
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
	if ms.comp.style == completionBottom {
		if e.scr == nil {
			return
		}
		_, sh := e.scr.Size()
		// No room means the prompt alone on the echo row, which the frame
		// already has; the cursor stays where the renderer puts it for a
		// prompt, on that row.
		if rows, ok := e.bottomRows(ms.comp, sh); ok {
			f.MiniRows, f.MiniNote = rows, ms.comp.countNote()
		}
		return
	}
	p, cx, cy, ok := e.panelFor(ms)
	if !ok {
		// The frame is too small for a readable panel. The prompt still works:
		// it falls back to the echo row, which is already in the frame.
		return
	}
	f.Panels = append(f.Panels, p)
	f.CursorX, f.CursorY, f.CursorSet = cx, cy, true
}
