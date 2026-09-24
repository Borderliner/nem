package editor

import (
	"fmt"
	"testing"
)

// C-u C-SPC returns to where a jump started: M->, a search, M-g M-g - and
// again goes further back.
func TestPopMarkReturnsFromJumps(t *testing.T) {
	var lines []string
	for i := range 10 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}

	e, _ := newTestEditor(t, lines...)
	press(t, e, "C-n", "C-n", "C-n")
	press(t, e, "M->")
	press(t, e, "C-u", "C-SPC")
	wantPt(t, e, 3, 0)

	e, scr := newTestEditor(t, lines...)
	press(t, e, "C-n")
	feed(t, scr, txt("line 7"), key(t, "RET"))
	press(t, e, "C-s")
	wantPt(t, e, 7, 6)
	wantEcho(t, e, "Mark saved where search started")
	press(t, e, "C-u", "C-SPC")
	wantPt(t, e, 1, 0)

	e, scr = newTestEditor(t, lines...)
	press(t, e, "C-n", "C-n")
	feed(t, scr, txt("9"), key(t, "RET"))
	press(t, e, "M-g", "M-g")
	wantPt(t, e, 8, 0)
	press(t, e, "C-u", "C-SPC")
	wantPt(t, e, 2, 0)
}

// With no mark ever set there is nowhere to go back to, and it says so.
func TestPopMarkWithNoMark(t *testing.T) {
	e, _ := newTestEditor(t, "text")
	press(t, e, "C-u", "C-SPC")
	wantPt(t, e, 0, 0)
	wantEcho(t, e, "no mark")
}
