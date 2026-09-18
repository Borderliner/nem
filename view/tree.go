package view

import "errors"

// Minimum usable pane size. A window needs one row of text plus its modeline,
// and enough columns that a modeline is not pure ellipsis.
const (
	MinWindowHeight = 2
	MinWindowWidth  = 8
)

// DividerWidth is the width of the column drawn between side-by-side panes.
const DividerWidth = 1

var (
	// ErrTooSmall is returned by Split when the frame cannot accommodate two
	// panes of at least MinWindowWidth by MinWindowHeight.
	ErrTooSmall = errors.New("view: not enough room to split")
	// ErrNotFound is returned when a window is not present in the tree.
	ErrNotFound = errors.New("view: window not in tree")
	// ErrSoleWindow is returned by Delete when only one window remains.
	ErrSoleWindow = errors.New("view: cannot delete the sole window")
)

// Node is one position in the split tree: either a Leaf holding a window or a
// Split dividing its rectangle between two children. The interface is closed —
// only this package can implement it.
type Node interface{ isNode() }

// Leaf is a node holding a single window.
type Leaf struct{ Win *Window }

// Split divides its rectangle between two children.
//
// Vertical means the panes sit side by side, as C-x 3 makes them, with A on
// the left. A horizontal split stacks them, as C-x 2 makes them, with A above.
// Ratio is A's share of the space available after any divider, clamped to
// [0,1].
type Split struct {
	Vertical bool
	A, B     Node
	Ratio    float64
}

func (*Leaf) isNode()  {}
func (*Split) isNode() {}

// Rect is a rectangle of terminal cells.
type Rect struct{ X, Y, W, H int }

// Tree arranges windows over the frame.
//
// Layout records the frame size it was last given, and Split consults it to
// refuse a split that would produce an unusable pane. A tree that has never
// been laid out has no size to check against, so Split allows anything — that
// is the startup case, before the screen dimensions are known.
type Tree struct {
	Root Node

	lastW, lastH int
	sized        bool
}

// NewTree returns a tree holding w as its only window.
func NewTree(w *Window) *Tree { return &Tree{Root: &Leaf{Win: w}} }

// TextHeight returns the rows of r available for buffer text, which is all of
// them but the last: the modeline owns the bottom row of every window.
func TextHeight(r Rect) int {
	if r.H < 1 {
		return 0
	}
	return r.H - 1
}

// Layout assigns every window a rectangle within a frame of w by h cells, and
// records the size for Split's benefit.
//
// Windows and dividers together account for every cell of the frame exactly
// once; see Dividers.
func (t *Tree) Layout(w, h int) map[*Window]Rect {
	t.lastW, t.lastH, t.sized = w, h, true
	wins := make(map[*Window]Rect)
	walk(t.Root, Rect{0, 0, w, h}, func(win *Window, r Rect) { wins[win] = r }, nil)
	return wins
}

// Dividers returns the rectangles of the divider columns for a frame of w by h
// cells, one per vertical split.
//
// A vertical split reserves one column between its panes for the divider, so a
// pane's rectangle never includes it. A horizontal split needs no divider: the
// upper window's modeline already separates the two. Together with the window
// rectangles from Layout, these cover the frame exactly.
func (t *Tree) Dividers(w, h int) []Rect {
	var ds []Rect
	walk(t.Root, Rect{0, 0, w, h}, nil, func(r Rect) { ds = append(ds, r) })
	return ds
}

// walk assigns rectangles depth-first, reporting each window rect to onWin and
// each divider rect to onDiv. Either callback may be nil.
//
// Both Layout and Dividers go through here so the two can never disagree about
// where a divider sits.
func walk(n Node, r Rect, onWin func(*Window, Rect), onDiv func(Rect)) {
	switch n := n.(type) {
	case *Leaf:
		if onWin != nil {
			onWin(n.Win, r)
		}
	case *Split:
		ratio := n.Ratio
		if ratio < 0 {
			ratio = 0
		} else if ratio > 1 {
			ratio = 1
		}
		if n.Vertical {
			// A frame too narrow to hold a divider gets none, so the two panes
			// still account for every column and no rectangle goes negative.
			div := DividerWidth
			if r.W < DividerWidth {
				div = r.W
			}
			avail := r.W - div
			aW := int(float64(avail) * ratio)
			bW := avail - aW
			walk(n.A, Rect{r.X, r.Y, aW, r.H}, onWin, onDiv)
			if div > 0 && onDiv != nil {
				onDiv(Rect{r.X + aW, r.Y, div, r.H})
			}
			walk(n.B, Rect{r.X + aW + div, r.Y, bW, r.H}, onWin, onDiv)
			return
		}
		aH := int(float64(r.H) * ratio)
		bH := r.H - aH
		walk(n.A, Rect{r.X, r.Y, r.W, aH}, onWin, onDiv)
		walk(n.B, Rect{r.X, r.Y + aH, r.W, bH}, onWin, onDiv)
	}
}

// Windows returns every window in tree order, which for trees built by Split
// reads as the screen does: left to right, then top to bottom. This is the
// order other-window cycles through.
func (t *Tree) Windows() []*Window {
	var ws []*Window
	collect(t.Root, &ws)
	return ws
}

func collect(n Node, out *[]*Window) {
	switch n := n.(type) {
	case *Leaf:
		*out = append(*out, n.Win)
	case *Split:
		collect(n.A, out)
		collect(n.B, out)
	}
}

// Split divides target's pane in two and returns the new window, which shows
// the same buffer at the same point — that is how two views onto one buffer
// arise, and each then moves independently.
//
// It refuses rather than produce a pane too small to use, and leaves the tree
// untouched when it does.
func (t *Tree) Split(target *Window, vertical bool) (*Window, error) {
	if !t.contains(target) {
		return nil, ErrNotFound
	}
	if t.sized {
		r, ok := t.Layout(t.lastW, t.lastH)[target]
		if !ok {
			return nil, ErrNotFound
		}
		if !roomToSplit(r, vertical) {
			return nil, ErrTooSmall
		}
	}

	nw := &Window{Buf: target.Buf, Pt: target.Pt, Top: target.Top, GoalCol: target.GoalCol}
	replace(&t.Root, target, &Split{
		Vertical: vertical,
		A:        &Leaf{Win: target},
		B:        &Leaf{Win: nw},
		Ratio:    0.5,
	})
	return nw, nil
}

// roomToSplit reports whether r can be divided into two panes that both meet
// the minimum in the dimension being divided, counting the divider column a
// vertical split consumes.
//
// Only the divided dimension is checked. A horizontal split leaves width
// untouched, so refusing one because the frame is already narrow would block
// the operation for a condition it neither causes nor worsens.
func roomToSplit(r Rect, vertical bool) bool {
	if vertical {
		return r.W >= 2*MinWindowWidth+DividerWidth
	}
	return r.H >= 2*MinWindowHeight
}

// replace swaps the leaf holding target for repl, in place.
func replace(n *Node, target *Window, repl Node) bool {
	switch cur := (*n).(type) {
	case *Leaf:
		if cur.Win == target {
			*n = repl
			return true
		}
	case *Split:
		return replace(&cur.A, target, repl) || replace(&cur.B, target, repl)
	}
	return false
}

// Delete removes target's pane, giving its space to the sibling that shared
// the split. The sole remaining window cannot be deleted.
func (t *Tree) Delete(target *Window) error {
	if !t.contains(target) {
		return ErrNotFound
	}
	if _, isLeaf := t.Root.(*Leaf); isLeaf {
		return ErrSoleWindow
	}
	if !promoteSibling(&t.Root, target) {
		return ErrNotFound
	}
	return nil
}

// promoteSibling finds the split with target as one child and replaces that
// split with the other child.
func promoteSibling(n *Node, target *Window) bool {
	s, ok := (*n).(*Split)
	if !ok {
		return false
	}
	if l, ok := s.A.(*Leaf); ok && l.Win == target {
		*n = s.B
		return true
	}
	if l, ok := s.B.(*Leaf); ok && l.Win == target {
		*n = s.A
		return true
	}
	return promoteSibling(&s.A, target) || promoteSibling(&s.B, target)
}

// DeleteOthers makes keep the only window, filling the frame. A window not in
// the tree is ignored rather than allowed to empty it.
func (t *Tree) DeleteOthers(keep *Window) {
	if !t.contains(keep) {
		return
	}
	t.Root = &Leaf{Win: keep}
}

func (t *Tree) contains(w *Window) bool {
	for _, got := range t.Windows() {
		if got == w {
			return true
		}
	}
	return false
}
