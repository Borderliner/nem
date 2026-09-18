package view

// Minimum usable panel size.
//
// Two rows is the least that can show a prompt plus one candidate; a one-row
// panel says nothing the echo row does not already say better, so it is never
// worth drawing. Four columns is the least that can show a truncated candidate
// and still read as a list rather than as noise.
const (
	// Minimums include the one-cell border a panel always draws, so a placement
	// that reports ok can show at least one line of content in at least four
	// columns. Sizing these to the border alone would let PlacePanel succeed for
	// a panel that structurally cannot display anything - placement and drawing
	// would then disagree about what "usable" means.
	MinPanelWidth  = 6 // 4 content columns
	MinPanelHeight = 3 // 1 content row
)

// Anchor says where a panel wants to sit.
type Anchor int

const (
	// AnchorUnset is the zero value and is not a placement. PlacePanel refuses
	// it, so a caller that forgets to set Anchor gets no panel and a failing
	// test rather than a silently point-anchored one positioned at 0,0 because
	// PtX and PtY were also left unset.
	AnchorUnset Anchor = iota
	// AnchorPoint opens the panel next to point, which is where the eye already
	// is. Used by completion and by prefix-key discovery.
	AnchorPoint
	// AnchorBottom sits the panel flush to the bottom of the frame, which is the
	// emacs-shaped rendering selected by completion-style = "bottom".
	AnchorBottom
	// AnchorCenter centres the panel in the frame.
	AnchorCenter
)

// PanelReq is a request to place a panel of a preferred size.
//
// W and H include any border the caller intends to draw: placement deals in
// screen cells and has no opinion about what goes in them.
type PanelReq struct {
	W, H   int
	Anchor Anchor
	// Frame is the area a panel may occupy, which is the screen minus the echo
	// row. Excluding that row here rather than special-casing it below is what
	// makes "never cover the echo row" reduce to "stay inside Frame".
	Frame Rect
	// PtX and PtY are point's position on screen, used by AnchorPoint.
	PtX, PtY int
}

// PlacePanel returns where a panel should be drawn, and whether it should be
// drawn at all.
//
// Panels are not part of the split tree - they are drawn over the tiled frame -
// so this is the only geometry they need, and keeping it here rather than in ui
// means it is testable without a terminal alongside the rest of the layout
// arithmetic.
//
// The returned rect is always wholly inside Frame and never degenerate. A
// request larger than the frame is shrunk to it; a request smaller than a
// drawable panel is raised to the minimum, because handing back something
// unusable is worse than handing back slightly more than was asked for.
//
// ok is false only when the frame itself cannot hold a panel of the minimum
// size. The caller is then expected to render some other way rather than draw a
// box too small to read.
func PlacePanel(req PanelReq) (Rect, bool) {
	if req.Anchor == AnchorUnset {
		return Rect{}, false
	}

	f := req.Frame
	if f.W < MinPanelWidth || f.H < MinPanelHeight {
		return Rect{}, false
	}

	w := clampInt(req.W, MinPanelWidth, f.W)
	h := clampInt(req.H, MinPanelHeight, f.H)

	var x, y int
	switch req.Anchor {
	case AnchorBottom:
		x, y = centreOn(f.X, f.W, w), f.Y+f.H-h
	case AnchorCenter:
		x, y = centreOn(f.X, f.W, w), centreOn(f.Y, f.H, h)
	default: // AnchorPoint
		x, y, h = placeAtPoint(req, f, w, h)
	}
	return Rect{X: x, Y: y, W: w, H: h}, true
}

// placeAtPoint positions a panel beside point, returning a possibly reduced
// height.
//
// It prefers below point, flips above when the full height will not fit there,
// and takes the roomier side shrunk to fit when neither will hold it. Point's own
// row is never covered: a panel that sat on top of the line being edited would
// hide the thing the panel is about.
func placeAtPoint(req PanelReq, f Rect, w, h int) (x, y, height int) {
	// Clamp point into the frame before using it. A caller can hand over a
	// stale position - one captured before a resize shrank the frame - and
	// clamping here rather than guarding each result keeps every calculation
	// below sound. Without it a point above the frame makes the room below it
	// look enormous and the panel is placed off-screen.
	ptX := clampInt(req.PtX, f.X, f.X+f.W-1)
	ptY := clampInt(req.PtY, f.Y, f.Y+f.H-1)

	// Left edge at point, shifted left as far as needed to stay inside the
	// frame rather than overhanging it. No further left-edge guard is needed:
	// w is at most f.W, so f.X+f.W-w is never left of f.X.
	x = ptX
	if right := f.X + f.W; x+w > right {
		x = right - w
	}

	below := (f.Y + f.H) - (ptY + 1) // rows strictly below point
	above := ptY - f.Y               // rows strictly above point

	switch {
	case below >= h:
		return x, ptY + 1, h
	case above >= h:
		return x, ptY - h, h
	case below >= above && below >= MinPanelHeight:
		return x, ptY + 1, below
	case above >= MinPanelHeight:
		return x, ptY - above, above
	default:
		// Neither side of point can hold a drawable panel - point is boxed in by
		// a short frame. Usability beats anchoring, so ignore point and centre
		// in the frame instead of refusing to draw anything.
		return x, centreOn(f.Y, f.H, h), h
	}
}

// centreOn centres size within span starting at origin. Integer division
// truncates, so an odd leftover cell falls on the far side - the same direction
// the split layout gives its extra column.
func centreOn(origin, span, size int) int { return origin + (span-size)/2 }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
