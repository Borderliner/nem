package editor

import "testing"

// Every way of editing is refused in a read-only buffer, and says so. The
// refusal lives in the text primitives, so this is the check that no command
// reaches around them: typing, deleting, killing, yanking, a selection being
// replaced, and undo.
func TestReadOnlyBufferRefusesEveryEdit(t *testing.T) {
	for _, keys := range [][]string{
		{"x"},
		{"C-d"},
		{"DEL"},
		{"C-k"},
		{"RET"},
		{"C-y"},
		{"C-x", "h", "x"},
		{"C-x", "u"},
	} {
		e, _ := newTestEditor(t, "listing", "entries")
		e.ring.KillForward("pasted")
		e.ring.BreakRun()
		e.Buf().SetReadOnly(true)
		e.Active().Pt.Col = 3

		press(t, e, keys...)

		wantText(t, e, "listing\nentries")
		wantEcho(t, e, "read-only")
	}
}
