package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// parenAt runs the matcher over lines with point at (line, col).
func parenAt(t *testing.T, line int, col text.RuneIdx, lines ...string) parenHL {
	t.Helper()
	b := bufferOf(t, lines...)
	return matchParenAt(b, text.Pos{Line: line, Col: col}, DefaultTheme())
}

func TestMatchParen(t *testing.T) {
	th := DefaultTheme()
	for _, tc := range []struct {
		name  string
		lines []string
		line  int
		col   text.RuneIdx
		want  []text.Pos // empty means no highlight
		style bool       // true = matched, false = mismatched
	}{
		{
			name: "point on an opener", lines: []string{"(x)"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}, {Line: 0, Col: 2}}, style: true,
		},
		{
			name: "point on a closer", lines: []string{"(x)"}, col: 2,
			want: []text.Pos{{Line: 0, Col: 2}, {Line: 0, Col: 0}}, style: true,
		},
		{
			// Just after typing the closer is when you most want to see the match.
			name: "point just after a closer", lines: []string{"(x)"}, col: 3,
			want: []text.Pos{{Line: 0, Col: 2}, {Line: 0, Col: 0}}, style: true,
		},
		{
			name: "point just after an opener", lines: []string{"(x)"}, col: 1,
			want: []text.Pos{{Line: 0, Col: 0}, {Line: 0, Col: 2}}, style: true,
		},
		{
			name: "nested outer", lines: []string{"((x))"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}, {Line: 0, Col: 4}}, style: true,
		},
		{
			name: "nested inner", lines: []string{"((x))"}, col: 1,
			want: []text.Pos{{Line: 0, Col: 1}, {Line: 0, Col: 3}}, style: true,
		},
		{
			name: "across several lines", lines: []string{"foo(", "  bar", ")"}, col: 3,
			want: []text.Pos{{Line: 0, Col: 3}, {Line: 2, Col: 0}}, style: true,
		},
		{
			name: "backward across several lines", lines: []string{"foo(", "  bar", ")"},
			line: 2, col: 0,
			want: []text.Pos{{Line: 2, Col: 0}, {Line: 0, Col: 3}}, style: true,
		},
		{
			name: "mixed kinds nest correctly", lines: []string{"(a[b]c)"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}, {Line: 0, Col: 6}}, style: true,
		},
		{
			name: "braces", lines: []string{"{a}"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}, {Line: 0, Col: 2}}, style: true,
		},
		{
			name: "unmatched opener", lines: []string{"(x"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}}, style: false,
		},
		{
			name: "unmatched closer", lines: []string{"x)"}, col: 1,
			want: []text.Pos{{Line: 0, Col: 1}}, style: false,
		},
		{
			// "([)" is not a legal nesting; reporting a match would hide a real bug
			// in the user's text.
			name: "crossed kinds are a mismatch", lines: []string{"([)"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}}, style: false,
		},
		{
			// The discriminating case for kind checking: the wrong closer is the
			// FIRST one reached at depth zero, so a matcher that accepts any
			// closer reports a false match here. "([)" does not catch that,
			// because its ')' is consumed as the partner of '['.
			name: "wrong closer at depth zero", lines: []string{"(]"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}}, style: false,
		},
		{
			name: "wrong closer at depth zero, bracket", lines: []string{"[)"}, col: 0,
			want: []text.Pos{{Line: 0, Col: 0}}, style: false,
		},
		{
			// The discriminating case for scanning backward in the right order:
			// three brackets precede point on this line, and the nearest opener
			// at depth zero is the third, not the first. A matcher that walks a
			// line's brackets left-to-right returns column 0.
			name: "backward picks the nearest opener", lines: []string{"(a)(b)"}, col: 5,
			want: []text.Pos{{Line: 0, Col: 5}, {Line: 0, Col: 3}}, style: true,
		},
		{
			name: "backward skips a completed pair", lines: []string{"([a])b)"}, col: 6,
			want: []text.Pos{{Line: 0, Col: 6}}, style: false,
		},
		{
			// The backward mirror of "wrong closer at depth zero": the nearest
			// opener at depth zero is the wrong kind, so a matcher that accepts
			// any opener reports a false match. The completed-pair case above
			// never reaches this branch, so it cannot catch it.
			name: "wrong opener at depth zero", lines: []string{"[a)"}, col: 2,
			want: []text.Pos{{Line: 0, Col: 2}}, style: false,
		},
		{
			name: "wrong opener at depth zero, brace", lines: []string{"{a)"}, col: 2,
			want: []text.Pos{{Line: 0, Col: 2}}, style: false,
		},
		{
			name: "no bracket near point", lines: []string{"abc"}, col: 1,
			want: nil,
		},
		{
			name: "empty line", lines: []string{""}, col: 0,
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parenAt(t, tc.line, tc.col, tc.lines...)
			if got.n != len(tc.want) {
				t.Fatalf("highlighted %d positions %v, want %d %v",
					got.n, got.pos[:got.n], len(tc.want), tc.want)
			}
			for i, want := range tc.want {
				if got.pos[i] != want {
					t.Errorf("position %d = %v, want %v", i, got.pos[i], want)
				}
			}
			if len(tc.want) == 0 {
				return
			}
			wantStyle := th.ParenMismatch
			if tc.style {
				wantStyle = th.ParenMatch
			}
			for i := 0; i < got.n; i++ {
				if got.style[i] != wantStyle {
					t.Errorf("position %d has the wrong style (matched=%v)", i, tc.style)
				}
			}
		})
	}
}

// A bracket at point takes precedence over one before point, so moving onto a
// bracket matches the one you are on rather than the one you just left.
func TestBracketAtPointBeatsBracketBeforeIt(t *testing.T) {
	// "()" with point at col 1: the rune at point is ')', the rune before is '('.
	// Both are brackets; the one at point wins, so the pair is reported closer
	// first.
	got := parenAt(t, 0, 1, "()")
	if got.n != 2 {
		t.Fatalf("highlighted %d positions, want 2", got.n)
	}
	if got.pos[0] != (text.Pos{Line: 0, Col: 1}) {
		t.Errorf("first position = %v, want the bracket at point {0 1}", got.pos[0])
	}
}

// Bracket matching must not scan an unbounded distance: it runs on every frame
// in which point sits next to a bracket.
func TestScanIsCappedOnUnbalancedText(t *testing.T) {
	lines := make([]string, 400)
	for i := range lines {
		lines[i] = strings.Repeat("(", 100) // 40,000 runes, all openers
	}
	b := bufferOf(t, lines...)

	start := time.Now()
	got := matchParenAt(b, text.Pos{}, DefaultTheme())
	elapsed := time.Since(start)

	if got.n != 1 {
		t.Errorf("highlighted %d positions, want 1 (an unmatched opener)", got.n)
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("scan took %v; the cap is meant to keep this off the frame budget", elapsed)
	}
}

func TestParenHLOnLineSelectsOnlyThatLine(t *testing.T) {
	p := parenAt(t, 0, 3, "foo(", "  bar", ")")
	if p.n != 2 {
		t.Fatalf("highlighted %d positions, want 2", p.n)
	}
	if got := p.onLine(0); got.n != 1 || got.idx[0] != 3 {
		t.Errorf("onLine(0) = %+v, want one entry at rune 3", got)
	}
	if got := p.onLine(2); got.n != 1 || got.idx[0] != 0 {
		t.Errorf("onLine(2) = %+v, want one entry at rune 0", got)
	}
	if got := p.onLine(1); got.n != 0 {
		t.Errorf("onLine(1) = %+v, want no entries", got)
	}
}

func TestLineHLStyleForFallsBackToBase(t *testing.T) {
	th := DefaultTheme()
	h := lineHL{idx: [2]text.RuneIdx{2, 7}, style: [2]tcell.Style{th.ParenMatch, th.ParenMatch}, n: 2}
	if got := h.styleFor(2, th.Text); got != th.ParenMatch {
		t.Error("styleFor(2) did not return the override")
	}
	if got := h.styleFor(3, th.Text); got != th.Text {
		t.Error("styleFor(3) did not fall back to the base style")
	}
}

// --- render-level ---

func TestMatchedBracketsAreStyledOnScreen(t *testing.T) {
	th := DefaultTheme()
	f, w := singleFrame(t, "(x)")
	w.Pt = text.Pos{Line: 0, Col: 0}
	scr := sim(t, 20, 6)
	Render(scr, f, th)
	scr.Show()

	if got := cellAt(t, scr, 0, 0).Style; got != th.ParenMatch {
		t.Error("the opening bracket is not styled as a match")
	}
	if got := cellAt(t, scr, 2, 0).Style; got != th.ParenMatch {
		t.Error("the closing bracket is not styled as a match")
	}
	if got := cellAt(t, scr, 1, 0).Style; got != th.Text {
		t.Error("the character between the brackets should carry the plain text style")
	}
}

func TestUnmatchedBracketIsStyledAsMismatch(t *testing.T) {
	th := DefaultTheme()
	f, w := singleFrame(t, "(x")
	w.Pt = text.Pos{Line: 0, Col: 0}
	scr := sim(t, 20, 6)
	Render(scr, f, th)
	scr.Show()

	if got := cellAt(t, scr, 0, 0).Style; got != th.ParenMismatch {
		t.Error("an unmatched bracket should carry the mismatch style")
	}
}

// The partner may be scrolled out of view. The match must still be found, the
// visible half must still be styled, and drawing must not reach for a row that
// is not on screen.
func TestMatchFoundWhenPartnerIsScrolledOffScreen(t *testing.T) {
	th := DefaultTheme()
	lines := make([]string, 40)
	lines[0] = "("
	for i := 1; i < 39; i++ {
		lines[i] = "body"
	}
	lines[39] = ")"

	f, w := singleFrame(t, lines...)
	w.Pt = text.Pos{Line: 39, Col: 0}
	scr := sim(t, 20, 8)
	Render(scr, f, th) // must not panic
	scr.Show()

	if w.Top == 0 {
		t.Fatal("expected the view to have scrolled, leaving line 0 off screen")
	}
	// The matcher itself must still find the opener, even though it is not drawn.
	got := matchParenAt(w.Buf, w.Pt, th)
	if got.n != 2 {
		t.Fatalf("highlighted %d positions, want 2 across a scrolled viewport", got.n)
	}
	if got.pos[1] != (text.Pos{Line: 0, Col: 0}) {
		t.Errorf("partner = %v, want {0 0}", got.pos[1])
	}

	// The visible half is on the last text row.
	y := 39 - w.Top
	if cell := cellAt(t, scr, 0, y).Style; cell != th.ParenMatch {
		t.Error("the visible half of the pair is not styled as a match")
	}
}

// Only the window holding point shows a bracket highlight, as in emacs.
func TestInactiveWindowShowsNoParenHighlight(t *testing.T) {
	th := DefaultTheme()
	left := view.NewWindow(bufferOf(t, "(x)"))
	tree := view.NewTree(left)
	right, err := tree.Split(left, true)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	right.Visit(bufferOf(t, "(y)"))
	right.Pt = text.Pos{Line: 0, Col: 0}

	scr := sim(t, 40, 8)
	Render(scr, Frame{Tree: tree, Active: left}, th)
	scr.Show()

	dx := tree.Dividers(40, 7)[0].X
	// The right pane's opening bracket sits just past the divider.
	if got := cellAt(t, scr, dx+1, 0).Style; got == th.ParenMatch {
		t.Error("an inactive window highlighted its brackets")
	}
}

// A highlight must not disturb glyph placement: the styled cell is still one
// cluster, wide or not.
func TestHighlightDoesNotDisturbWideGlyphs(t *testing.T) {
	th := DefaultTheme()
	f, w := singleFrame(t, "(日)")
	w.Pt = text.Pos{Line: 0, Col: 0}
	scr := sim(t, 20, 6)
	Render(scr, f, th)
	scr.Show()

	for i, want := range []string{"(", "日", "", ")"} {
		if got := string(cellAt(t, scr, i, 0).Runes); got != want {
			t.Errorf("cell %d = %q, want %q", i, got, want)
		}
	}
	if got := cellAt(t, scr, 3, 0).Style; got != th.ParenMatch {
		t.Error("the closer after a wide glyph is not styled as a match")
	}
}
