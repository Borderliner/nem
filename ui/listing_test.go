package ui

import (
	"testing"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
	"github.com/gdamore/tcell/v2"
)

// listingFrame is a frame whose only window shows lines as a listing, with
// point on row pt. Every line's first four runes are classified Function, so a
// test can tell whether the row kept its colours.
func listingFrame(t *testing.T, pt int, lines ...string) (Frame, *view.Window) {
	t.Helper()
	f, w := singleFrame(t, lines...)
	w.Pt = text.Pos{Line: pt}
	f.ListingOf = func(b *text.Buffer) bool { return b == w.Buf }
	f.SpansOf = func(*text.Buffer, int) []syntax.Span {
		return []syntax.Span{{Start: 0, End: 4, Class: syntax.Function}}
	}
	return f, w
}

func reversed(c tcell.SimCell) bool {
	_, _, attr := c.Style.Decompose()
	return attr&tcell.AttrReverse != 0
}

// A listing has no gutter even with line numbers on: they would count rows of
// a directory, which nobody reads by number. The text starts in the first
// column and the cursor is placed to match.
func TestListingHasNoGutter(t *testing.T) {
	f, w := listingFrame(t, 1, "alpha", "beta")
	w.Pt.Col = 2

	scr := draw(t, 30, 6, f)

	if got := rowText(t, scr, 0); got[:5] != "alpha" {
		t.Errorf("row 0 = %q, want the listing from column 0 with no numbers", got)
	}
	if x, y, _ := scr.GetCursor(); x != 2 || y != 1 {
		t.Errorf("cursor at (%d,%d), want (2,1)", x, y)
	}
}

// The row point is on is one flat bar to the window's edge; every other row is
// drawn normally, colours included.
func TestListingDrawsTheCurrentRowAsABar(t *testing.T) {
	f, _ := listingFrame(t, 1, "alpha", "beta", "gamma")

	scr := draw(t, 20, 6, f)

	for x := 0; x < 20; x++ {
		if c := cellAt(t, scr, x, 1); !reversed(c) {
			t.Fatalf("cell (%d,1) is not in the bar; the bar must run to the edge", x)
		}
	}
	if fg, _, _ := cellAt(t, scr, 0, 1).Style.Decompose(); fg != tcell.ColorDefault {
		t.Errorf("bar keeps foreground %v; the row's own colours should be dropped", fg)
	}
	for _, y := range []int{0, 2} {
		c := cellAt(t, scr, 0, y)
		if reversed(c) {
			t.Errorf("row %d is drawn as the bar too", y)
		}
		if fg, _, _ := c.Style.Decompose(); fg == tcell.ColorDefault {
			t.Errorf("row %d lost its colour", y)
		}
	}
}

// Only the active window shows the bar, as only it shows a selection.
func TestListingBarIsOnlyInTheActiveWindow(t *testing.T) {
	f, w := listingFrame(t, 0, "alpha", "beta")
	other := view.NewWindow(bufferOf(t, "text"))
	f.Active = other

	scr := draw(t, 20, 6, Frame{
		Tree: view.NewTree(w), Active: other,
		ListingOf: f.ListingOf, SpansOf: f.SpansOf,
	})

	if reversed(cellAt(t, scr, 0, 0)) {
		t.Error("an inactive listing window draws the bar")
	}
}

// A buffer that is not a listing keeps its gutter and gets no bar.
func TestTextBufferIsNotDrawnAsAListing(t *testing.T) {
	f, _ := singleFrame(t, "alpha", "beta")
	f.ListingOf = func(*text.Buffer) bool { return false }

	scr := draw(t, 20, 6, f)

	if got := rowText(t, scr, 0); got[:1] != "1" {
		t.Errorf("row 0 = %q, want a line number first", got)
	}
	if reversed(cellAt(t, scr, 5, 0)) {
		t.Error("a text buffer's current row is drawn as a bar")
	}
}

// A listing being edited as text keeps its layout, with no gutter to shift
// its columns, but has no bar: the row keeps its colours and point is a
// cursor.
func TestEditedListingHasNoBar(t *testing.T) {
	f, w := listingFrame(t, 1, "alpha", "beta")
	w.Pt.Col = 2
	f.EditingOf = func(b *text.Buffer) bool { return b == w.Buf }

	scr := draw(t, 20, 6, f)

	if got := rowText(t, scr, 1); got[:4] != "beta" {
		t.Errorf("row 1 = %q, want the text from column 0 with no numbers", got)
	}
	if c := cellAt(t, scr, 0, 1); reversed(c) {
		t.Error("the row point is on is drawn as a bar")
	}
	if fg, _, _ := cellAt(t, scr, 0, 1).Style.Decompose(); fg == tcell.ColorDefault {
		t.Error("the row point is on lost its colours")
	}
	if x, y, _ := scr.GetCursor(); x != 2 || y != 1 {
		t.Errorf("cursor at (%d,%d), want (2,1)", x, y)
	}
}
