package editor

import (
	"testing"

	"github.com/Borderliner/nem/command"
)

// A quote or bracket typed over a selection goes around it, and point ends
// after the closer, ready to go on typing.
func TestAutoPairWrapsTheSelection(t *testing.T) {
	for _, tc := range []struct{ typed, want string }{
		{`"`, `"word" here`},
		{"(", "(word) here"},
	} {
		e, _ := newTestEditor(t, "word here")
		press(t, e, "C-SPC", "M-f", tc.typed)
		wantText(t, e, tc.want)
		wantPt(t, e, 0, 6)
		if e.Buf().MarkActive() {
			t.Error("the selection is still active after wrapping it")
		}
		press(t, e, "C-/")
		wantText(t, e, "word here") // one undo takes the whole wrap back
	}
}

// With auto-pair off, typing over a selection replaces it, as before.
func TestAutoPairOffReplacesTheSelection(t *testing.T) {
	command.AutoPair = false
	t.Cleanup(func() { command.AutoPair = true })
	e, _ := newTestEditor(t, "word here")
	press(t, e, "C-SPC", "M-f", "(")
	wantText(t, e, "( here")
}

// Nothing is paired in a prompt: a search for ( is a search for (.
func TestAutoPairLeavesPromptsAlone(t *testing.T) {
	e, scr := newTestEditor(t, "text")
	feed(t, scr, txt(`("x`), key(t, "RET"))
	got, err := e.ReadString(command.ReadOpts{Prompt: "Search: "})
	if err != nil {
		t.Fatal(err)
	}
	if got != `("x` {
		t.Errorf("prompt read %q, want exactly what was typed", got)
	}
}
