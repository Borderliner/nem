package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// --- the session, tested directly ----------------------------------------

// from builds a completion over a fixed candidate set.
func from(names ...string) *completion {
	return newCompletion(command.CompleteFrom(names), "")
}

func candidates(c *completion) []string {
	out := make([]string, 0, c.count())
	for _, r := range c.ranked {
		out = append(out, r.Candidate)
	}
	return out
}

// Typing narrows the list, and erasing widens it again. The second half is the
// one worth asserting: a filter that only ever narrowed would leave the user
// stuck with whatever their longest typo matched.
func TestCompletionNarrowsAndWidens(t *testing.T) {
	c := from("forward-char", "forward-word", "kill-line", "save-buffer")

	if got := c.count(); got != 4 {
		t.Fatalf("empty input matched %d candidates, want all 4", got)
	}

	c.refresh("forw")
	got := candidates(c)
	if len(got) != 2 || got[0] != "forward-char" && got[0] != "forward-word" {
		t.Errorf("input %q matched %q, want the two forward- commands", "forw", got)
	}

	c.refresh("forward-c")
	if got := candidates(c); len(got) != 1 || got[0] != "forward-char" {
		t.Errorf("input %q matched %q, want only forward-char", "forward-c", got)
	}

	// Backspaced back to "forw": the list must grow again.
	c.refresh("forw")
	if got := c.count(); got != 2 {
		t.Errorf("after erasing, %d candidates matched, want 2 again", got)
	}
}

// Fuzzy matching is the point: a subsequence finds a command no prefix would.
func TestCompletionMatchesASubsequence(t *testing.T) {
	c := from("forward-char", "backward-char", "kill-line")
	c.refresh("fwc")
	got := candidates(c)
	if len(got) == 0 || got[0] != "forward-char" {
		t.Errorf("fuzzy input %q ranked %q, want forward-char first", "fwc", got)
	}
}

// An exact match goes first even where the scorer prefers another. It rewards
// a camelCase boundary, so fooBar outscores foobar for the input "foobar";
// without the promotion RET would take fooBar, a name nobody typed.
func TestCompletionRanksAnExactMatchFirst(t *testing.T) {
	c := from("fooBar", "foobar", "foo_bar")
	c.refresh("foobar")
	if got, _ := c.selected(); got != "foobar" {
		t.Errorf("input %q highlights %q (ranked %q), want the exact match first",
			"foobar", got, candidates(c))
	}
	if got := c.count(); got != 3 {
		t.Errorf("promoting the exact match left %d candidates, want all 3", got)
	}
}

// A prefix every candidate shares is not scored. At find-file it is the
// directory just walked into, and scoring it tied every entry, so the length
// tie-break highlighted the shortest name - often a dotfile - where the listing
// order, the one RET is expected to take, puts alpha.txt first.
func TestCompletionIgnoresAPrefixEveryCandidateShares(t *testing.T) {
	c := from("d/alpha.txt", "d/b.go", "d/.x")

	c.refresh("d/")
	if got, _ := c.selected(); got != "d/alpha.txt" {
		t.Errorf("input %q highlights %q (ranked %q), want the listing's first entry",
			"d/", got, candidates(c))
	}

	// What follows the prefix still ranks, and the emphasised positions are
	// still offsets into the whole candidate.
	c.refresh("d/b")
	if got := candidates(c); len(got) != 1 || got[0] != "d/b.go" {
		t.Fatalf("input %q matched %q, want only d/b.go", "d/b", got)
	}
	if got := c.ranked[0].Match.Indices; len(got) != 1 || got[0] != 2 {
		t.Errorf("d/b.go emphasises runes %v, want [2]", got)
	}
}

// The shared prefix ends on a rune boundary: é and è share their first byte,
// and cutting there would hand the scorer half a character.
func TestSharedPrefixStopsOnARuneBoundary(t *testing.T) {
	for _, tc := range []struct {
		input string
		cands []string
		want  int
	}{
		{"é", []string{"éa", "èb"}, 0},
		{"xé", []string{"xéa", "xèb"}, 1},
		{"ab", []string{"abc", "abd"}, 2},
		{"ab", nil, 0},
	} {
		if got := sharedPrefixLen(tc.input, tc.cands); got != tc.want {
			t.Errorf("sharedPrefixLen(%q, %q) = %d, want %d", tc.input, tc.cands, got, tc.want)
		}
	}
}

// The selection resets to the best match whenever the list changes, and can
// never address a candidate that has stopped existing.
func TestCompletionSelectionIsClampedWhenTheListShrinks(t *testing.T) {
	c := from("alpha", "alpine", "alps", "beta")
	c.refresh("al")
	c.move(2)
	if c.sel != 2 {
		t.Fatalf("setup: selection = %d, want 2", c.sel)
	}

	// Narrow to a single candidate. A stale index would now be out of range and
	// selected() would either panic or name the wrong thing.
	c.refresh("alpha")
	if c.sel != 0 {
		t.Errorf("selection = %d after the list shrank, want 0", c.sel)
	}
	got, ok := c.selected()
	if !ok || got != "alpha" {
		t.Errorf("selected() = %q, %v; want \"alpha\", true", got, ok)
	}
}

// Wrapping is deliberate: these lists are short and C-p to reach the last entry
// is a gesture people use.
func TestCompletionSelectionWraps(t *testing.T) {
	c := from("one", "two", "three")
	if c.sel != 0 {
		t.Fatalf("setup: selection = %d, want 0", c.sel)
	}
	c.move(-1)
	if c.sel != 2 {
		t.Errorf("C-p from the first entry selected %d, want the last (2)", c.sel)
	}
	c.move(1)
	if c.sel != 0 {
		t.Errorf("C-n from the last entry selected %d, want the first (0)", c.sel)
	}
}

// An empty list must not leave a selection pointing at anything.
func TestCompletionWithNoMatchesHasNoSelection(t *testing.T) {
	c := from("alpha", "beta")
	c.refresh("zzzz")
	if c.count() != 0 {
		t.Fatalf("setup: %q matched something", "zzzz")
	}
	if _, ok := c.selected(); ok {
		t.Error("selected() reported a candidate with none matching")
	}
	c.move(1) // must not panic or invent an index
	if c.sel != 0 {
		t.Errorf("selection = %d on an empty list, want 0", c.sel)
	}
}

// The visible window follows the selection in a list longer than it.
func TestCompletionScrollsToKeepTheSelectionVisible(t *testing.T) {
	names := make([]string, 0, 30)
	for _, s := range []string{"a", "b", "c"} {
		for i := 0; i < 10; i++ {
			names = append(names, s+string(rune('0'+i)))
		}
	}
	c := newCompletion(command.CompleteFrom(names), "")
	c.rows = 5

	c.move(7)
	if c.sel < c.top || c.sel >= c.top+c.visibleRows() {
		t.Errorf("selection %d outside the visible window [%d,%d)", c.sel, c.top, c.top+c.visibleRows())
	}
	c.move(-1)
	if c.sel < c.top || c.sel >= c.top+c.visibleRows() {
		t.Errorf("after moving back, selection %d outside [%d,%d)", c.sel, c.top, c.top+c.visibleRows())
	}
}

// --- the panel -----------------------------------------------------------

// promptOn builds an editor with a live prompt, without entering the nested
// event loop, so the panel can be inspected directly.
func promptOn(t *testing.T, w, h int, opts command.ReadOpts, typed string) (*Editor, *miniState) {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(w, h)
	t.Cleanup(scr.Fini)

	e, err := New(scr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	buf := text.NewBuffer()
	if typed != "" {
		if err := buf.Insert(text.Pos{}, []rune(typed)); err != nil {
			t.Fatalf("seed prompt: %v", err)
		}
	}
	win := view.NewWindow(buf)
	win.Pt = buf.End()
	// Through the same constructor ReadString uses, so the session-creation rule
	// is exercised here rather than restated.
	ms := newMiniState(opts, buf, win)
	e.mini = ms
	return e, ms
}

func TestPanelShowsPromptCandidatesAndSelection(t *testing.T) {
	e, ms := promptOn(t, 80, 24, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"forward-char", "forward-word", "kill-line"}),
	}, "forw")

	p, cx, cy, ok := e.panelFor(ms)
	if !ok {
		t.Fatal("panelFor declined to build a panel in an 80x24 frame")
	}
	if len(p.Lines) < 2 {
		t.Fatalf("panel has %d lines, want the prompt plus candidates", len(p.Lines))
	}
	if got := p.Lines[0].Text; got != "M-x forw" {
		t.Errorf("first line = %q, want the prompt and its contents", got)
	}
	if !strings.Contains(p.Title, "/2") {
		t.Errorf("title = %q, want a count over the 2 matches", p.Title)
	}

	var selected int
	for _, ln := range p.Lines[1:] {
		if ln.Selected {
			selected++
		}
		if len(ln.Match) == 0 {
			t.Errorf("candidate %q carries no match indices, so nothing would be emphasised", ln.Text)
		}
	}
	if selected != 1 {
		t.Errorf("%d candidate rows marked selected, want exactly 1", selected)
	}

	// The cursor must land on the prompt row inside the panel, not on the echo
	// row: the panel is where the user is typing.
	if cy != p.Rect.Y+1 {
		t.Errorf("cursor row = %d, want the panel's first interior row %d", cy, p.Rect.Y+1)
	}
	if cx <= p.Rect.X || cx >= p.Rect.X+p.Rect.W {
		t.Errorf("cursor column %d is outside the panel %v", cx, p.Rect)
	}
}

// The popup is centred, not anchored to point: the same gesture must put the
// panel in the same place whatever line the user happens to be on.
func TestPopupPanelIsCentredRegardlessOfPoint(t *testing.T) {
	opts := command.ReadOpts{Prompt: "M-x ", Complete: command.CompleteFrom([]string{"one", "two"})}

	e, ms := promptOn(t, 80, 24, opts, "")
	first, _, _, ok := e.panelFor(ms)
	if !ok {
		t.Fatal("no panel")
	}

	// Move point far away in the text buffer and rebuild.
	e2, ms2 := promptOn(t, 80, 24, opts, "")
	tb := e2.tree.Windows()[0].Buf
	if err := tb.Insert(text.Pos{}, []rune("aaa\nbbb\nccc\nddd\neee\nfff")); err != nil {
		t.Fatal(err)
	}
	e2.tree.Windows()[0].Pt = text.Pos{Line: 5, Col: 3}
	second, _, _, ok := e2.panelFor(ms2)
	if !ok {
		t.Fatal("no panel")
	}

	if first.Rect != second.Rect {
		t.Errorf("panel moved with point: %v then %v; a centred popup must not", first.Rect, second.Rect)
	}
}

// The default is the emacs shape, as Vertico draws it: the prompt at the foot
// of the screen with the candidates listed beneath it, full width, the count
// at the prompt's right, and the window above shrunk to make room - its
// modeline intact directly over the prompt rather than covered by a panel.
func TestBottomStyleListsCandidatesBelowThePrompt(t *testing.T) {
	e, _ := promptOn(t, 60, 20, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"forward-char", "forward-word", "kill-line"}),
	}, "forw")
	if e.comp.style != completionBottom {
		t.Fatalf("default completion style is %v, want bottom", e.comp.style)
	}

	e.Redraw()

	// Two matches, so the prompt is three rows from the bottom.
	promptY := 20 - 3
	if got := screenRow(t, e.scr.(tcell.SimulationScreen), promptY); !strings.HasPrefix(got, "M-x forw") || !strings.HasSuffix(strings.TrimRight(got, " "), "1/2") {
		t.Errorf("prompt row = %q, want the prompt with the count at its right", got)
	}
	for i, want := range []string{"forward-char", "forward-word"} {
		if got := screenRow(t, e.scr.(tcell.SimulationScreen), promptY+1+i); !strings.HasPrefix(got, want) {
			t.Errorf("row %d = %q, want candidate %s", promptY+1+i, got, want)
		}
	}
	if got := screenRow(t, e.scr.(tcell.SimulationScreen), promptY-1); !strings.Contains(got, "*scratch*") {
		t.Errorf("row above the prompt = %q, want the window's modeline", got)
	}
	// The selected candidate is the bar, across the full width.
	cells, w, _ := e.scr.(tcell.SimulationScreen).GetContents()
	if _, _, attr := cells[(promptY+1)*w+w-1].Style.Decompose(); attr&tcell.AttrReverse == 0 {
		t.Error("the selected candidate's bar does not reach the right edge")
	}
	if x, y, _ := e.scr.(tcell.SimulationScreen).GetCursor(); y != promptY || x != len("M-x forw") {
		t.Errorf("cursor at (%d,%d), want the end of the prompt (%d,%d)", x, y, len("M-x forw"), promptY)
	}
}

// The list keeps the height it has reached while the prompt is open, as
// emacs's grow-only minibuffer does, so narrowing the list does not resize the
// windows under the user as they type.
func TestBottomStyleDoesNotShrinkWhileTyping(t *testing.T) {
	e, ms := promptOn(t, 60, 20, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"alpha", "beta", "gamma"}),
	}, "")
	if f := e.frame(); len(f.MiniRows) != 3 {
		t.Fatalf("%d rows for three candidates, want 3", len(f.MiniRows))
	}

	ms.comp.refresh("gam")
	f := e.frame()
	if len(f.MiniRows) != 3 {
		t.Fatalf("narrowed to one match, the list is %d rows; want it to stay 3", len(f.MiniRows))
	}
	if f.MiniRows[0].Text != "gamma" || f.MiniRows[1].Text != "" {
		t.Errorf("rows = %+v, want gamma then blanks", f.MiniRows)
	}
}

// On a short screen the list gives way: the windows keep a line of text and a
// modeline, and the selection stays in view.
func TestBottomStyleLeavesTheWindowsARow(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("cmd-%02d", i)
	}
	e, ms := promptOn(t, 40, 8, command.ReadOpts{Prompt: "M-x ", Complete: command.CompleteFrom(names)}, "")
	ms.comp.move(20)

	f := e.frame()
	if got := len(f.MiniRows); got != 8-3 {
		t.Fatalf("%d candidate rows on an 8-row screen, want 5", got)
	}
	var sel string
	for _, r := range f.MiniRows {
		if r.Selected {
			sel = r.Text
		}
	}
	if sel != "cmd-20" {
		t.Errorf("selected row shows %q, want cmd-20 scrolled into view", sel)
	}
}

// The centred popup is still there for those who prefer it.
func TestPopupStyleIsASetting(t *testing.T) {
	e, _ := promptOn(t, 80, 24, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"one", "two"}),
	}, "")
	e.SetCompletionStyle("popup")

	f := e.frame()
	if len(f.Panels) != 1 || len(f.MiniRows) != 0 {
		t.Errorf("popup style gave %d panels and %d bottom rows, want one panel", len(f.Panels), len(f.MiniRows))
	}
}

// A frame too small for a readable panel falls back to the echo-row prompt
// rather than drawing an unusable box.
func TestTinyFrameFallsBackToTheEchoRow(t *testing.T) {
	e, ms := promptOn(t, 4, 2, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"one", "two"}),
	}, "")

	if _, _, _, ok := e.panelFor(ms); ok {
		t.Error("built a panel in a 4x2 frame; want a refusal so the prompt falls back to the echo row")
	}

	// And the frame the renderer receives must still carry the prompt.
	f := e.frame()
	if !f.MiniOn {
		t.Error("frame does not show the prompt on the echo row")
	}
	if f.CursorSet {
		t.Error("cursor was overridden to a panel that was never placed")
	}
	if len(f.Panels) != 0 {
		t.Errorf("%d panels in the frame, want none", len(f.Panels))
	}
}

// An incremental search grows no candidate panel. C-s passes no Complete
// function, and that absence - not an inspection of command names - is what
// distinguishes a search prompt from a completing one.
func TestIsearchPromptHasNoPanel(t *testing.T) {
	e, ms := promptOn(t, 80, 24, command.ReadOpts{Prompt: "I-search: "}, "beta")
	if ms.comp != nil {
		t.Fatal("a prompt with no Complete function built a completion session")
	}
	if _, _, _, ok := e.panelFor(ms); ok {
		t.Error("panelFor built a panel for a prompt with no candidates")
	}
	f := e.frame()
	if len(f.Panels) != 0 {
		t.Errorf("%d panels for a search prompt, want none", len(f.Panels))
	}
	if !f.MiniOn {
		t.Error("the search prompt is not on the echo row")
	}
}

// --- RET, TAB and M-RET, driven through the real prompt -------------------

// Fuzzy matching plus RET-takes-the-selection, end to end: a subsequence no
// prefix completion could resolve reaches the command.
func TestFuzzyInputThenRetRunsTheSelectedCommand(t *testing.T) {
	e, scr := newTestEditor(t, "abcdef")
	feed(t, scr, txt("fwc"), key(t, "RET"))
	press(t, e, "M-x")
	// forward-char ran, so point advanced one grapheme.
	wantPt(t, e, 0, 1)
}

// C-n moves the selection, and RET then runs what is highlighted rather than
// what was typed.
func TestSelectionMovesAndRetRunsIt(t *testing.T) {
	e, scr := newTestEditor(t, "one two three")
	// "forward-" matches forward-char and forward-word. Whichever ranks first,
	// C-n moves to the other, so the two cases are distinguished by where point
	// ends up: one column for a char, four for the first word.
	feed(t, scr, txt("forward-"), key(t, "C-n", "RET"))
	press(t, e, "M-x")
	if got := e.Win().Pt.Col; got != 1 && got != 3 {
		t.Errorf("point column = %d after C-n then RET, want 1 (char) or 3 (word)", got)
	}
}

// TAB fills the line with the selected candidate, which is what makes it useful
// under fuzzy matching - the candidates rarely share a prefix with the input, so
// a common-prefix extension would usually do nothing.
func TestTabCompletesToTheSelection(t *testing.T) {
	e, scr := newTestEditor(t, "abcdef")
	feed(t, scr, txt("fwc"), key(t, "TAB", "RET"))
	press(t, e, "M-x")
	wantPt(t, e, 0, 1)
}

// With nothing highlighted and no match required, RET accepts exactly what was
// typed, so a name that does not exist yet can still be created. That is all
// RequireMatch decides now: a highlighted candidate wins either way.
func TestRetAcceptsTypedTextWhenNothingMatches(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	feed(t, scr, txt("brand-new-buffer"), key(t, "RET"))
	press(t, e, "C-x", "b")
	if got := e.BufferName(e.Buf()); got != "brand-new-buffer" {
		t.Errorf("visiting buffer %q, want the typed name brand-new-buffer", got)
	}
}

// M-RET forces the literal line at a prompt that allows one.
func TestMetaRetAcceptsLiteralInput(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	feed(t, scr, txt("literal-name"), key(t, "M-RET"))
	press(t, e, "C-x", "b")
	if got := e.BufferName(e.Buf()); got != "literal-name" {
		t.Errorf("visiting buffer %q, want literal-name", got)
	}
}

// M-RET must not be a back door around RequireMatch, or the flag is decorative.
func TestMetaRetIsRefusedWhenAMatchIsRequired(t *testing.T) {
	e, scr := newTestEditor(t, "abc")
	feed(t, scr, txt("no-such-command"), key(t, "M-RET", "C-g"))
	press(t, e, "M-x")
	wantEcho(t, e, "requires an existing match")
	wantPt(t, e, 0, 0)
}

// Descending only makes sense when it changes the input. A candidate that is
// already exactly what was typed is taken even if Descend says to walk into
// it; otherwise RET would leave the prompt exactly as it was, a key that does
// nothing.
func TestRetOnAnExactDescendCandidateAccepts(t *testing.T) {
	e, ms := promptOn(t, 80, 24, command.ReadOpts{
		Prompt:   "Find file: ",
		Complete: command.CompleteFrom([]string{"src/", "srv/"}),
		Descend:  func(string) bool { return true },
	}, "src/")

	ms.accept(e)
	if !ms.done {
		t.Error("RET on an exact candidate kept the prompt open")
	}
	if got := ms.contents(); got != "src/" {
		t.Errorf("accepted %q, want %q", got, "src/")
	}
}

// With nothing highlighted and a match required, RET refuses and keeps the
// prompt open with the text intact.
func TestRetRefusesWhenNothingMatchesAndAMatchIsRequired(t *testing.T) {
	e, ms := promptOn(t, 80, 24, command.ReadOpts{
		Prompt:       "M-x ",
		Complete:     command.CompleteFrom([]string{"alpha", "beta"}),
		RequireMatch: true,
	}, "zzz")

	ms.accept(e)
	if ms.done {
		t.Error("RET accepted text that matches nothing at a RequireMatch prompt")
	}
	wantEcho(t, e, "No match")
	if got := ms.contents(); got != "zzz" {
		t.Errorf("prompt holds %q, want the typed text kept", got)
	}
}

// The prompt is still a real buffer, so the ordinary editing commands work in it
// and the candidate list follows what they leave behind. C-a C-k empties the
// line, which must widen the list rather than leave it filtered by text that is
// no longer there.
func TestEditingThePromptRefiltersTheList(t *testing.T) {
	e, ms := promptOn(t, 80, 24, command.ReadOpts{
		Prompt:   "M-x ",
		Complete: command.CompleteFrom([]string{"alpha", "alpine", "beta"}),
	}, "")

	ms.replace(e, "alp")
	if got := ms.comp.count(); got != 2 {
		t.Fatalf("after typing %q, %d candidates matched, want 2", "alp", got)
	}
	ms.replace(e, "")
	if got := ms.comp.count(); got != 3 {
		t.Errorf("after clearing the prompt, %d candidates matched, want all 3", got)
	}
}
