package editor

import "testing"

// C-x C-u and C-x C-l change the case of the selection, which then ends; with
// none they say so.
func TestCaseRegionKeys(t *testing.T) {
	e, _ := newTestEditor(t, "hello world")
	press(t, e, "C-SPC", "M-f", "C-x", "C-u")
	wantText(t, e, "HELLO world")
	wantPt(t, e, 0, 5)
	if e.Buf().MarkActive() {
		t.Error("the selection is still active after C-x C-u")
	}
	press(t, e, "C-x", "C-x", "C-x", "C-l")
	wantText(t, e, "hello world")

	e, _ = newTestEditor(t, "abc")
	press(t, e, "C-x", "C-u")
	wantText(t, e, "abc")
	wantEcho(t, e, "no region")
}

// The line commands are reached through M-x, and the region survives the
// prompt to be what they work on.
func TestSortLinesThroughMx(t *testing.T) {
	e, scr := newTestEditor(t, "pear", "apple", "fig")
	press(t, e, "C-x", "h")
	feed(t, scr, txt("sort-lines"), key(t, "RET"))
	press(t, e, "M-x")
	wantText(t, e, "apple\nfig\npear")

	press(t, e, "C-x", "h")
	feed(t, scr, txt("reverse-region"), key(t, "RET"))
	press(t, e, "M-x")
	wantText(t, e, "pear\nfig\napple")
}
