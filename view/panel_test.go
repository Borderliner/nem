package view

import (
	"fmt"
	"testing"
)

func inside(r, frame Rect) bool {
	return r.X >= frame.X && r.Y >= frame.Y &&
		r.X+r.W <= frame.X+frame.W && r.Y+r.H <= frame.Y+frame.H
}

// The invariant that catches every clamping and flipping mistake at once: over a
// sweep of frames, sizes, anchors and point positions, a placed panel is always
// wholly inside the frame and never degenerate.
func TestPlacedPanelIsAlwaysInsideTheFrame(t *testing.T) {
	anchors := []Anchor{AnchorPoint, AnchorBottom, AnchorCenter}
	checked := 0
	for _, fw := range []int{0, 1, 3, 4, 5, 8, 17, 40, 80} {
		for _, fh := range []int{0, 1, 2, 3, 6, 11, 24} {
			frame := Rect{X: 2, Y: 1, W: fw, H: fh}
			for _, pw := range []int{-3, 0, 1, 4, 9, 20, 100} {
				for _, ph := range []int{-2, 0, 1, 2, 5, 12, 50} {
					for _, a := range anchors {
						for _, pty := range []int{frame.Y, frame.Y + fh/2, frame.Y + fh - 1} {
							for _, ptx := range []int{frame.X, frame.X + fw - 1} {
								req := PanelReq{W: pw, H: ph, Anchor: a, Frame: frame, PtX: ptx, PtY: pty}
								got, ok := PlacePanel(req)
								if !ok {
									continue
								}
								checked++
								if got.W <= 0 || got.H <= 0 {
									t.Fatalf("%+v -> degenerate %+v", req, got)
								}
								if !inside(got, frame) {
									t.Fatalf("%+v -> %+v escapes frame %+v", req, got, frame)
								}
							}
						}
					}
				}
			}
		}
	}
	if checked < 500 {
		t.Fatalf("only %d placements exercised; the sweep is not covering enough", checked)
	}
	t.Logf("%d placements checked", checked)
}

// A frame too small for a usable panel must be refused, so the caller can render
// some other way instead of drawing an unusable box.
func TestTinyFrameIsRefused(t *testing.T) {
	for _, frame := range []Rect{
		{W: 0, H: 0}, {W: 1, H: 1}, {W: 3, H: 5}, {W: 40, H: 1}, {W: -4, H: -4},
	} {
		if got, ok := PlacePanel(PanelReq{W: 10, H: 4, Frame: frame, Anchor: AnchorCenter}); ok {
			t.Errorf("frame %+v accepted, returned %+v; want refusal", frame, got)
		}
	}
}

func TestFrameExactlyAtTheMinimumIsAccepted(t *testing.T) {
	frame := Rect{W: MinPanelWidth, H: MinPanelHeight}
	got, ok := PlacePanel(PanelReq{W: 10, H: 10, Frame: frame, Anchor: AnchorCenter})
	if !ok {
		t.Fatalf("frame at exactly the minimum was refused")
	}
	if got.W != MinPanelWidth || got.H != MinPanelHeight {
		t.Errorf("got %+v, want the whole frame %+v", got, frame)
	}
}

// Point-anchored panels open below point, which is where the eye already is.
func TestAnchorPointOpensBelowPoint(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 40, H: 20}
	got, ok := PlacePanel(PanelReq{W: 12, H: 5, Anchor: AnchorPoint, Frame: frame, PtX: 3, PtY: 4})
	if !ok {
		t.Fatal("refused")
	}
	if want := (Rect{X: 3, Y: 5, W: 12, H: 5}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// With no room below, the panel flips above point rather than being clamped into
// overlapping the line the user is editing.
func TestAnchorPointFlipsAboveWhenBelowIsFull(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 40, H: 20}
	got, ok := PlacePanel(PanelReq{W: 12, H: 6, Anchor: AnchorPoint, Frame: frame, PtX: 3, PtY: 18})
	if !ok {
		t.Fatal("refused")
	}
	if want := (Rect{X: 3, Y: 12, W: 12, H: 6}); got != want {
		t.Errorf("got %+v, want %+v (flipped above point)", got, want)
	}
	if got.Y+got.H > 18 {
		t.Errorf("panel %+v covers point's row 18", got)
	}
}

func TestAnchorPointOnTheBottomRowFlipsAbove(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 30, H: 10}
	got, ok := PlacePanel(PanelReq{W: 8, H: 4, Anchor: AnchorPoint, Frame: frame, PtX: 0, PtY: 9})
	if !ok {
		t.Fatal("refused")
	}
	if got.Y+got.H > 9 {
		t.Errorf("got %+v, want it entirely above row 9", got)
	}
}

// Neither side can hold the full height: the roomier side wins and the panel
// shrinks to fit it, rather than overhanging or vanishing.
func TestAnchorPointShrinksToTheRoomierSide(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 30, H: 10}
	// Point at row 6: 3 rows below (7,8,9), 6 rows above (0..5).
	got, ok := PlacePanel(PanelReq{W: 8, H: 9, Anchor: AnchorPoint, Frame: frame, PtX: 0, PtY: 6})
	if !ok {
		t.Fatal("refused")
	}
	if got.Y != 0 || got.H != 6 {
		t.Errorf("got %+v, want the six rows above point (Y=0,H=6)", got)
	}
}

// A panel wider than its point allows is shifted left, not left to overhang.
func TestAnchorPointShiftsLeftToStayInside(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 20, H: 10}
	got, ok := PlacePanel(PanelReq{W: 14, H: 3, Anchor: AnchorPoint, Frame: frame, PtX: 18, PtY: 2})
	if !ok {
		t.Fatal("refused")
	}
	if want := (Rect{X: 6, Y: 3, W: 14, H: 3}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAnchorBottomSitsFlushAndCentred(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 20, H: 10}
	got, ok := PlacePanel(PanelReq{W: 10, H: 3, Anchor: AnchorBottom, Frame: frame})
	if !ok {
		t.Fatal("refused")
	}
	if want := (Rect{X: 5, Y: 7, W: 10, H: 3}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAnchorCenterIsCentredBothWays(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 20, H: 10}
	got, ok := PlacePanel(PanelReq{W: 10, H: 4, Anchor: AnchorCenter, Frame: frame})
	if !ok {
		t.Fatal("refused")
	}
	if want := (Rect{X: 5, Y: 3, W: 10, H: 4}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Centring truncates, so an odd leftover column goes to the right - the same
// direction the split layout gives its extra cell.
func TestOddCentringPutsTheExtraColumnRight(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 21, H: 10}
	got, _ := PlacePanel(PanelReq{W: 10, H: 3, Anchor: AnchorCenter, Frame: frame})
	left := got.X - frame.X
	right := (frame.X + frame.W) - (got.X + got.W)
	if left != 5 || right != 6 {
		t.Errorf("got %+v: %d columns left, %d right; want 5 and 6", got, left, right)
	}
}

// An oversized request is shrunk to the frame, not refused.
func TestOversizedRequestShrinksToTheFrame(t *testing.T) {
	frame := Rect{X: 1, Y: 1, W: 12, H: 6}
	for _, a := range []Anchor{AnchorBottom, AnchorCenter} {
		got, ok := PlacePanel(PanelReq{W: 100, H: 100, Anchor: a, Frame: frame})
		if !ok {
			t.Fatalf("anchor %v refused an oversized request", a)
		}
		if got.W != frame.W || got.H != frame.H {
			t.Errorf("anchor %v: got %+v, want the whole frame %+v", a, got, frame)
		}
	}
}

// A point-anchored panel cannot fill the frame, because point's own row stays
// uncovered - the panel is about the line being edited, so hiding it would be
// self-defeating. It takes the full width and as much height as one side of
// point allows.
func TestOversizedPointAnchoredRequestLeavesPointVisible(t *testing.T) {
	frame := Rect{X: 1, Y: 1, W: 12, H: 6}
	const ptY = 2
	got, ok := PlacePanel(PanelReq{W: 100, H: 100, Anchor: AnchorPoint, Frame: frame, PtX: 2, PtY: ptY})
	if !ok {
		t.Fatal("refused an oversized request")
	}
	if got.W != frame.W {
		t.Errorf("got width %d, want the full frame width %d", got.W, frame.W)
	}
	if !inside(got, frame) {
		t.Errorf("got %+v escapes %+v", got, frame)
	}
	if got.Y <= ptY && ptY < got.Y+got.H {
		t.Errorf("panel %+v covers point's row %d", got, ptY)
	}
	if got.H < MinPanelHeight {
		t.Errorf("panel %+v is below the drawable minimum", got)
	}
}

// Asking for less than a drawable panel yields the minimum rather than something
// unusable.
func TestUndersizedRequestIsRaisedToTheMinimum(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 30, H: 10}
	got, ok := PlacePanel(PanelReq{W: -5, H: 0, Anchor: AnchorCenter, Frame: frame})
	if !ok {
		t.Fatal("refused")
	}
	if got.W != MinPanelWidth || got.H != MinPanelHeight {
		t.Errorf("got %+v, want %dx%d", got, MinPanelWidth, MinPanelHeight)
	}
}

func TestPlacementIsDeterministic(t *testing.T) {
	req := PanelReq{W: 13, H: 5, Anchor: AnchorPoint, Frame: Rect{X: 2, Y: 3, W: 37, H: 15}, PtX: 30, PtY: 12}
	first, ok := PlacePanel(req)
	if !ok {
		t.Fatal("refused")
	}
	for i := 0; i < 50; i++ {
		got, _ := PlacePanel(req)
		if got != first {
			t.Fatalf("call %d returned %+v, first returned %+v", i, got, first)
		}
	}
}

// Point at each corner must place something sane.
func TestAnchorPointAtEveryCorner(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 24, H: 12}
	for _, pt := range []struct{ x, y int }{{0, 0}, {23, 0}, {0, 11}, {23, 11}} {
		t.Run(fmt.Sprintf("%d,%d", pt.x, pt.y), func(t *testing.T) {
			got, ok := PlacePanel(PanelReq{W: 10, H: 4, Anchor: AnchorPoint, Frame: frame, PtX: pt.x, PtY: pt.y})
			if !ok {
				t.Fatal("refused")
			}
			if !inside(got, frame) {
				t.Errorf("got %+v escapes %+v", got, frame)
			}
		})
	}
}

// A caller can hand over a stale point - a position captured before a resize
// shrank the frame. Placement clamps it into the frame rather than drawing
// off-screen, so a race between resize and redraw degrades instead of corrupting.
func TestPointOutsideTheFrameIsClamped(t *testing.T) {
	frame := Rect{X: 10, Y: 5, W: 20, H: 10}
	for _, pt := range []struct {
		name string
		x, y int
	}{
		{"left of frame", 0, 7},
		{"above frame", 12, 0},
		{"right of frame", 99, 7},
		{"below frame", 12, 99},
		{"far outside both", -50, -50},
	} {
		t.Run(pt.name, func(t *testing.T) {
			got, ok := PlacePanel(PanelReq{W: 8, H: 3, Anchor: AnchorPoint, Frame: frame, PtX: pt.x, PtY: pt.y})
			if !ok {
				t.Fatal("refused")
			}
			if !inside(got, frame) {
				t.Errorf("got %+v escapes frame %+v", got, frame)
			}
		})
	}
}

// A zero PanelReq must produce no panel. AnchorPoint used to be the zero value,
// so forgetting to set Anchor silently point-anchored at 0,0 - a panel in the
// top-left corner that looked like a placement bug rather than a missing field.
// Refusing the zero value turns that into a failing test at the call site.
func TestUnsetAnchorIsRefused(t *testing.T) {
	if _, ok := PlacePanel(PanelReq{
		W: 30, H: 8,
		Frame: Rect{X: 0, Y: 0, W: 80, H: 23},
	}); ok {
		t.Error("PlacePanel with no Anchor returned ok; want refusal so a forgotten field is caught")
	}
}
