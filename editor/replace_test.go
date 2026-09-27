package editor

import "testing"

// C-M-% replaces a regexp, answering in the prompts as query-replace does.
// Terminals rarely send C-M-%, so it is pressed as the decoded key here, and
// M-x is the way that always works.
func TestQueryReplaceRegexpKeys(t *testing.T) {
	e, scr := newTestEditor(t, "width=10 height=20")
	feed(t, scr, txt(`(\w+)=(\d+)`), key(t, "RET"), txt(`\2 \1`), key(t, "RET"), txt("yn"))
	press(t, e, "C-M-%")
	wantText(t, e, "10 width height=20")
	wantEcho(t, e, "Replaced 1 occurrence")
}

// replace-string asks nothing, and keeps to the selection when there is one.
func TestReplaceStringInTheSelection(t *testing.T) {
	e, scr := newTestEditor(t, "x x", "x x", "x x")
	press(t, e, "C-n", "C-SPC", "C-n")
	feed(t, scr,
		txt("replace-string"), key(t, "RET"),
		txt("x"), key(t, "RET"), txt("y"), key(t, "RET"))
	press(t, e, "M-x")
	wantText(t, e, "x x\ny y\nx x")
	wantEcho(t, e, "Replaced 2 occurrences")

	press(t, e, "C-/")
	wantText(t, e, "x x\nx x\nx x")
}

func TestQueryReplaceRegexpThroughMx(t *testing.T) {
	e, scr := newTestEditor(t, "a1 b2")
	feed(t, scr,
		txt("query-replace-regexp"), key(t, "RET"),
		txt(`[0-9]`), key(t, "RET"), txt(`<\&>`), key(t, "RET"), txt("!"))
	press(t, e, "M-x")
	wantText(t, e, "a<1> b<2>")
}
