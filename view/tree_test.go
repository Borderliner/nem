package view

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/hajianpour/nem/text"
)

func TestTextHeight(t *testing.T) {
	tests := []struct {
		r    Rect
		want int
	}{
		{Rect{H: 24}, 23},
		{Rect{H: 2}, 1},
		{Rect{H: 1}, 0},
		{Rect{H: 0}, 0},
	}
	for _, tc := range tests {
		if got := TextHeight(tc.r); got != tc.want {
			t.Errorf("TextHeight(%+v) = %d, want %d", tc.r, got, tc.want)
		}
	}
}

func TestSingleWindowFillsFrame(t *testing.T) {
	w := NewWindow(bufLines(t, 5))
	tr := NewTree(w)
	got := tr.Layout(80, 24)
	if len(got) != 1 {
		t.Fatalf("got %d rects, want 1", len(got))
	}
	if want := (Rect{0, 0, 80, 24}); got[w] != want {
		t.Errorf("rect = %+v, want %+v", got[w], want)
	}
	if d := tr.Dividers(80, 24); len(d) != 0 {
		t.Errorf("sole window produced %d dividers, want 0", len(d))
	}
}

// A vertical split puts the panes side by side with a one-column divider
// between them, and the three together account for every column.
func TestVerticalSplitReservesDividerColumn(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, err := tr.Split(a, true)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	r := tr.Layout(81, 24)
	ra, rb := r[a], r[b]
	if want := (Rect{0, 0, 40, 24}); ra != want {
		t.Errorf("left = %+v, want %+v", ra, want)
	}
	if want := (Rect{41, 0, 40, 24}); rb != want {
		t.Errorf("right = %+v, want %+v", rb, want)
	}
	d := tr.Dividers(81, 24)
	if len(d) != 1 {
		t.Fatalf("got %d dividers, want 1", len(d))
	}
	if want := (Rect{40, 0, 1, 24}); d[0] != want {
		t.Errorf("divider = %+v, want %+v", d[0], want)
	}
	if ra.W+d[0].W+rb.W != 81 {
		t.Errorf("widths %d+%d+%d do not account for 81 columns", ra.W, d[0].W, rb.W)
	}
}

// A stacked split needs no divider: the upper window's modeline already
// separates the two.
func TestHorizontalSplitNeedsNoDivider(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, err := tr.Split(a, false)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	r := tr.Layout(80, 24)
	if want := (Rect{0, 0, 80, 12}); r[a] != want {
		t.Errorf("top = %+v, want %+v", r[a], want)
	}
	if want := (Rect{0, 12, 80, 12}); r[b] != want {
		t.Errorf("bottom = %+v, want %+v", r[b], want)
	}
	if d := tr.Dividers(80, 24); len(d) != 0 {
		t.Errorf("stacked split produced %d dividers, want 0", len(d))
	}
}

// With an odd number of columns or rows to share, the extra one goes to the
// second pane: the right one, or the lower one.
func TestOddSplitGivesExtraToSecondPane(t *testing.T) {
	t.Run("vertical", func(t *testing.T) {
		a := NewWindow(bufLines(t, 5))
		tr := NewTree(a)
		b, _ := tr.Split(a, true)
		r := tr.Layout(80, 24) // 79 usable after the divider: 39 | 40
		if r[a].W != 39 || r[b].W != 40 {
			t.Errorf("widths = %d | %d, want 39 | 40", r[a].W, r[b].W)
		}
	})
	t.Run("horizontal", func(t *testing.T) {
		a := NewWindow(bufLines(t, 5))
		tr := NewTree(a)
		b, _ := tr.Split(a, false)
		r := tr.Layout(80, 25)
		if r[a].H != 12 || r[b].H != 13 {
			t.Errorf("heights = %d / %d, want 12 / 13", r[a].H, r[b].H)
		}
	})
}

// This is the case the whole architecture exists for: two windows onto one
// buffer, sharing text but not cursors.
func TestSplitSharesBufferButNotPoint(t *testing.T) {
	buf := bufLines(t, 50)
	a := NewWindow(buf)
	a.Pt = text.Pos{Line: 10, Col: 2}
	tr := NewTree(a)

	b, err := tr.Split(a, false)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if b.Buf != a.Buf {
		t.Error("new window does not share the buffer")
	}
	if !b.Pt.Equal(a.Pt) {
		t.Errorf("new window point = %+v, want %+v", b.Pt, a.Pt)
	}

	// Edit through one window; the other sees the text, keeps its own point.
	beforeB := b.Pt
	if err := a.Buf.Insert(text.Pos{Line: 0}, []rune("XY")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := b.Buf.Line(0).String(); got != "XYline0" {
		t.Errorf("other window sees %q, want %q", got, "XYline0")
	}
	if !b.Pt.Equal(beforeB) {
		t.Errorf("other window point moved to %+v, want %+v", b.Pt, beforeB)
	}
	a.Pt = text.Pos{Line: 40}
	if b.Pt.Line == 40 {
		t.Error("windows are sharing a point; they must be independent")
	}
}

func TestSplitRefusesWhenPanesWouldBeTooSmall(t *testing.T) {
	tests := []struct {
		name     string
		w, h     int
		vertical bool
		wantErr  bool
	}{
		{"wide enough to split vertically", 17, 24, true, false},
		{"one column too narrow", 16, 24, true, true},
		{"tall enough to split horizontally", 4, 4, false, false},
		{"one row too short", 80, 3, false, true},
		{"comfortable frame", 80, 24, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewWindow(bufLines(t, 5))
			tr := NewTree(a)
			tr.Layout(tc.w, tc.h)
			_, err := tr.Split(a, tc.vertical)
			if tc.wantErr {
				if !errors.Is(err, ErrTooSmall) {
					t.Errorf("err = %v, want ErrTooSmall", err)
				}
				if n := len(tr.Windows()); n != 1 {
					t.Errorf("failed split left %d windows, want 1", n)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSplitBeforeAnyLayoutIsAllowed(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	if _, err := tr.Split(a, true); err != nil {
		t.Errorf("split with no recorded size: %v", err)
	}
}

func TestSplitUnknownWindow(t *testing.T) {
	tr := NewTree(NewWindow(bufLines(t, 5)))
	stranger := NewWindow(bufLines(t, 5))
	if _, err := tr.Split(stranger, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteSoleWindowRefused(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	if err := tr.Delete(a); !errors.Is(err, ErrSoleWindow) {
		t.Errorf("err = %v, want ErrSoleWindow", err)
	}
	if n := len(tr.Windows()); n != 1 {
		t.Errorf("window count = %d, want 1", n)
	}
}

func TestDeleteReplacesSplitWithSibling(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, _ := tr.Split(a, false)
	c, _ := tr.Split(b, true)

	if err := tr.Delete(b); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got := tr.Windows()
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Errorf("windows = %v, want [a c]", got)
	}
	// The survivor must take the whole of the departed split's space.
	r := tr.Layout(80, 24)
	if r[a].H+r[c].H != 24 {
		t.Errorf("heights %d + %d do not fill 24", r[a].H, r[c].H)
	}
}

func TestDeleteDownToOneThenRefuse(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, _ := tr.Split(a, true)
	if err := tr.Delete(b); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := tr.Delete(a); !errors.Is(err, ErrSoleWindow) {
		t.Errorf("err = %v, want ErrSoleWindow", err)
	}
}

func TestDeleteUnknownWindow(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	tr.Split(a, true)
	if err := tr.Delete(NewWindow(bufLines(t, 5))); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteOthers(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, _ := tr.Split(a, true)
	tr.Split(b, false)

	tr.DeleteOthers(b)
	got := tr.Windows()
	if len(got) != 1 || got[0] != b {
		t.Fatalf("windows = %v, want [b]", got)
	}
	if want := (Rect{0, 0, 80, 24}); tr.Layout(80, 24)[b] != want {
		t.Errorf("survivor rect = %+v, want %+v", tr.Layout(80, 24)[b], want)
	}
}

func TestDeleteOthersWithUnknownWindowIsNoOp(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	tr.Split(a, true)
	before := len(tr.Windows())
	tr.DeleteOthers(NewWindow(bufLines(t, 5)))
	if after := len(tr.Windows()); after != before {
		t.Errorf("window count %d, want %d unchanged", after, before)
	}
}

// Windows() drives other-window cycling, so its order must read the way the
// screen does: left to right, then top to bottom.
func TestWindowsCycleOrder(t *testing.T) {
	t.Run("top row split, then bottom", func(t *testing.T) {
		top := NewWindow(bufLines(t, 5))
		tr := NewTree(top)
		bottom, _ := tr.Split(top, false)  // top / bottom
		topRight, _ := tr.Split(top, true) // top | topRight
		want := []*Window{top, topRight, bottom}
		got := tr.Windows()
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("order wrong: got %v, want %v", got, want)
		}
	})
	t.Run("left column, then right stacked", func(t *testing.T) {
		left := NewWindow(bufLines(t, 5))
		tr := NewTree(left)
		right, _ := tr.Split(left, true)
		rightLower, _ := tr.Split(right, false)
		want := []*Window{left, right, rightLower}
		got := tr.Windows()
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("order wrong: got %v, want %v", got, want)
		}
	})
}

func TestWindowsOrderIsStableAcrossCalls(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	b, _ := tr.Split(a, true)
	tr.Split(b, false)
	first := tr.Windows()
	for i := 0; i < 5; i++ {
		next := tr.Windows()
		for j := range first {
			if first[j] != next[j] {
				t.Fatalf("order changed between calls at index %d", j)
			}
		}
	}
}

// randTree builds a tree by repeatedly splitting a random existing window,
// which is how one arises in use.
func randTree(t *testing.T, rng *rand.Rand, leaves int) *Tree {
	t.Helper()
	tr := NewTree(NewWindow(bufLines(t, 5)))
	for i := 1; i < leaves; i++ {
		ws := tr.Windows()
		target := ws[rng.Intn(len(ws))]
		nw, err := tr.Split(target, rng.Intn(2) == 0)
		if err != nil {
			t.Fatalf("building random tree: %v", err)
		}
		_ = nw
		setRatios(tr.Root, rng)
	}
	return tr
}

func setRatios(n Node, rng *rand.Rand) {
	if s, ok := n.(*Split); ok {
		s.Ratio = 0.2 + rng.Float64()*0.6
		setRatios(s.A, rng)
		setRatios(s.B, rng)
	}
}

// The most valuable test here: windows plus dividers must cover every cell of
// the frame exactly once, at any tree shape and any frame size. A gap means
// stale pixels on screen; an overlap means two windows fighting over a cell.
func TestLayoutTilesFrameExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 400; trial++ {
		leaves := 1 + rng.Intn(6)
		tr := randTree(t, rng, leaves)
		w := rng.Intn(120)
		h := rng.Intn(50)

		cover := make([]int, w*h)
		mark := func(r Rect, what string) {
			if r.W < 0 || r.H < 0 {
				t.Fatalf("trial %d: %s has negative size %+v", trial, what, r)
			}
			if r.X < 0 || r.Y < 0 || r.X+r.W > w || r.Y+r.H > h {
				t.Fatalf("trial %d: %s %+v escapes frame %dx%d", trial, what, r, w, h)
			}
			for y := r.Y; y < r.Y+r.H; y++ {
				for x := r.X; x < r.X+r.W; x++ {
					cover[y*w+x]++
				}
			}
		}
		for _, r := range tr.Layout(w, h) {
			mark(r, "window")
		}
		for _, r := range tr.Dividers(w, h) {
			mark(r, "divider")
		}
		for i, c := range cover {
			if c != 1 {
				t.Fatalf("trial %d (%d leaves, %dx%d): cell (%d,%d) covered %d times, want 1",
					trial, leaves, w, h, i%w, i/w, c)
			}
		}
	}
}

// Shrinking the terminal below the point where panes are usable must still
// produce valid rects rather than negative sizes or a panic.
func TestLayoutSurvivesTinyFrames(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for leaves := 1; leaves <= 5; leaves++ {
		tr := randTree(t, rng, leaves)
		for w := 0; w <= 20; w++ {
			for h := 0; h <= 10; h++ {
				for win, r := range tr.Layout(w, h) {
					if r.W < 0 || r.H < 0 {
						t.Fatalf("%dx%d: window rect %+v negative", w, h, r)
					}
					if r.X+r.W > w || r.Y+r.H > h {
						t.Fatalf("%dx%d: window rect %+v escapes frame", w, h, r)
					}
					_ = win
				}
			}
		}
	}
}

func TestLayoutRecordsSizeForSplitValidation(t *testing.T) {
	a := NewWindow(bufLines(t, 5))
	tr := NewTree(a)
	tr.Layout(80, 24)
	if _, err := tr.Split(a, true); err != nil {
		t.Fatalf("split at 80x24: %v", err)
	}
	// Shrinking the frame must make a further vertical split of a 40-column
	// pane impossible.
	tr.Layout(20, 24)
	ws := tr.Windows()
	if _, err := tr.Split(ws[0], true); !errors.Is(err, ErrTooSmall) {
		t.Errorf("err = %v, want ErrTooSmall after shrink", err)
	}
}

func TestDividersAndLayoutAgree(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 50; trial++ {
		tr := randTree(t, rng, 1+rng.Intn(5))
		w, h := 40+rng.Intn(80), 10+rng.Intn(30)
		wantDividers := countVerticalSplits(tr.Root)
		if got := len(tr.Dividers(w, h)); got != wantDividers {
			t.Errorf("trial %d: %d dividers, want %d", trial, got, wantDividers)
		}
	}
}

func countVerticalSplits(n Node) int {
	s, ok := n.(*Split)
	if !ok {
		return 0
	}
	c := countVerticalSplits(s.A) + countVerticalSplits(s.B)
	if s.Vertical {
		c++
	}
	return c
}
