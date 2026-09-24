package view

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

// bufLines returns a buffer holding n lines named line0..line(n-1).
func bufLines(t *testing.T, n int) *text.Buffer {
	t.Helper()
	b := text.NewBuffer()
	rs := []rune{}
	for i := 0; i < n; i++ {
		if i > 0 {
			rs = append(rs, '\n')
		}
		rs = append(rs, []rune("line")...)
		rs = append(rs, []rune(itoa(i))...)
	}
	if err := b.Insert(text.Pos{}, rs); err != nil {
		t.Fatalf("seeding buffer: %v", err)
	}
	if got := b.NumLines(); got != n {
		t.Fatalf("seeded buffer has %d lines, want %d", got, n)
	}
	b.SetModified(false)
	return b
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for i > 0 {
		d = append([]byte{byte('0' + i%10)}, d...)
		i /= 10
	}
	return string(d)
}

func TestNewWindowLeavesGoalColUnset(t *testing.T) {
	w := NewWindow(bufLines(t, 3))
	if w.GoalCol != GoalColUnset {
		t.Errorf("GoalCol = %d, want %d (unset)", w.GoalCol, GoalColUnset)
	}
	if w.Pt != (text.Pos{}) {
		t.Errorf("Pt = %+v, want origin", w.Pt)
	}
	if w.Top != 0 {
		t.Errorf("Top = %d, want 0", w.Top)
	}
}

func TestVisitSavesAndRestoresPointPerBuffer(t *testing.T) {
	a, b := bufLines(t, 50), bufLines(t, 50)
	w := NewWindow(a)

	w.Pt = text.Pos{Line: 10, Col: 2}
	w.Visit(b)
	if !w.Pt.Equal(text.Pos{}) {
		t.Errorf("point in freshly visited buffer = %+v, want origin", w.Pt)
	}

	w.Pt = text.Pos{Line: 30, Col: 1}
	w.Visit(a)
	if want := (text.Pos{Line: 10, Col: 2}); !w.Pt.Equal(want) {
		t.Errorf("point restored for a = %+v, want %+v", w.Pt, want)
	}

	w.Visit(b)
	if want := (text.Pos{Line: 30, Col: 1}); !w.Pt.Equal(want) {
		t.Errorf("point restored for b = %+v, want %+v", w.Pt, want)
	}
}

func TestVisitResetsGoalCol(t *testing.T) {
	a, b := bufLines(t, 5), bufLines(t, 5)
	w := NewWindow(a)
	w.GoalCol = 7
	w.Visit(b)
	if w.GoalCol != GoalColUnset {
		t.Errorf("GoalCol = %d after Visit, want unset", w.GoalCol)
	}
}

func TestVisitSameBufferIsNoOp(t *testing.T) {
	a := bufLines(t, 20)
	w := NewWindow(a)
	w.Pt = text.Pos{Line: 5}
	w.Top = 3
	w.GoalCol = 4
	w.Visit(a)
	if w.Pt.Line != 5 || w.Top != 3 || w.GoalCol != 4 {
		t.Errorf("Visit(same buffer) disturbed state: Pt=%+v Top=%d GoalCol=%d", w.Pt, w.Top, w.GoalCol)
	}
}

func TestVisitClampsStalePoint(t *testing.T) {
	a, b := bufLines(t, 5), bufLines(t, 40)
	w := NewWindow(b)
	w.Pt = text.Pos{Line: 35}
	w.Visit(a) // a's save point is origin, fine
	w.Visit(b) // b's save point is line 35, still valid
	if w.Pt.Line != 35 {
		t.Fatalf("Pt.Line = %d, want 35", w.Pt.Line)
	}
	// Shrink b below the saved point, then revisit.
	if err := b.Delete(text.Pos{Line: 2}, b.End()); err != nil {
		t.Fatalf("shrinking buffer: %v", err)
	}
	w.Visit(a)
	w.Visit(b)
	if w.Pt.Line >= b.NumLines() {
		t.Errorf("Pt.Line = %d, outside buffer of %d lines", w.Pt.Line, b.NumLines())
	}
}

func TestVisitLeavesPointVisible(t *testing.T) {
	a, b := bufLines(t, 5), bufLines(t, 100)
	w := NewWindow(b)
	w.Pt = text.Pos{Line: 80}
	w.Visit(a)
	w.Visit(b)
	if w.Top > w.Pt.Line {
		t.Errorf("Top = %d is past Pt.Line = %d; point not visible", w.Top, w.Pt.Line)
	}
}

func TestScrollToPoint(t *testing.T) {
	tests := []struct {
		name       string
		lines      int
		ptLine     int
		top        int
		textHeight int
		margin     int
		wantTop    int
	}{
		{"already visible leaves top alone", 100, 5, 0, 10, 0, 0},
		{"point below viewport scrolls down", 100, 50, 0, 10, 0, 41},
		{"point above viewport scrolls up", 100, 5, 20, 10, 0, 5},
		{"margin honoured scrolling down", 100, 50, 0, 10, 2, 43},
		{"margin honoured scrolling up", 100, 5, 20, 10, 2, 3},
		{"buffer shorter than viewport pins to top", 3, 1, 0, 10, 2, 0},
		{"buffer exactly viewport height pins to top", 10, 9, 0, 10, 2, 0},
		{"cannot scroll past end", 100, 99, 0, 10, 2, 90},
		{"cannot scroll before start", 100, 1, 5, 10, 2, 0},
		// margin 50 clamps to (10-1)/2 = 4, so top = 50-10+1+4.
		{"oversized margin is clamped not thrashing", 100, 50, 0, 10, 50, 45},
		{"negative margin treated as zero", 100, 50, 0, 10, -3, 41},
		{"height of one still works", 100, 40, 0, 1, 0, 40},
		{"zero height treated as one", 100, 40, 0, 0, 0, 40},
		{"point on first line", 100, 0, 40, 10, 2, 0},
		{"point on last line no margin", 100, 99, 0, 10, 0, 90},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWindow(bufLines(t, tc.lines))
			w.Pt = text.Pos{Line: tc.ptLine}
			w.Top = tc.top
			w.ScrollToPoint(tc.textHeight, tc.margin)
			if w.Top != tc.wantTop {
				t.Errorf("Top = %d, want %d", w.Top, tc.wantTop)
			}
		})
	}
}

// ScrollToPoint must always leave point visible and Top in range, whatever it
// is handed. This is the invariant the render path depends on.
func TestScrollToPointAlwaysLeavesPointVisible(t *testing.T) {
	for _, lines := range []int{1, 2, 7, 50, 100} {
		b := bufLines(t, lines)
		for ptLine := 0; ptLine < lines; ptLine++ {
			for _, h := range []int{1, 2, 5, 10, 200} {
				for _, margin := range []int{0, 1, 3, 100} {
					for _, top := range []int{0, 1, lines - 1, 1000} {
						w := NewWindow(b)
						w.Pt = text.Pos{Line: ptLine}
						w.Top = top
						w.ScrollToPoint(h, margin)
						eff := h
						if eff < 1 {
							eff = 1
						}
						if w.Top < 0 {
							t.Fatalf("Top = %d < 0 (lines=%d pt=%d h=%d margin=%d top=%d)", w.Top, lines, ptLine, h, margin, top)
						}
						if w.Top > ptLine || ptLine >= w.Top+eff {
							t.Fatalf("point %d not visible in [%d,%d) (lines=%d h=%d margin=%d top=%d)",
								ptLine, w.Top, w.Top+eff, lines, h, margin, top)
						}
					}
				}
			}
		}
	}
}

// Coming back to a buffer shows it as it was left - the same first line on
// screen - rather than with point's line moved to the top.
func TestVisitRestoresTheView(t *testing.T) {
	a, b := text.NewBuffer(), text.NewBuffer()
	lines := make([]rune, 0, 200)
	for range 100 {
		lines = append(lines, 'x', '\n')
	}
	if err := a.Insert(text.Pos{}, lines); err != nil {
		t.Fatal(err)
	}
	w := NewWindow(a)
	w.Pt, w.Top = text.Pos{Line: 50}, 40

	w.Visit(b)
	w.Visit(a)

	if w.Pt.Line != 50 || w.Top != 40 {
		t.Errorf("back in a: point line %d, top %d; want 50 and 40", w.Pt.Line, w.Top)
	}
	// A buffer never shown starts at its top.
	c := text.NewBuffer()
	w.Visit(c)
	if w.Top != 0 {
		t.Errorf("a fresh buffer shows from line %d, want 0", w.Top)
	}
}
