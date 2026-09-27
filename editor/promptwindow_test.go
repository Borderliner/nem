package editor

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

// C-x b at a prompt shows the buffer in the window the prompt was opened
// from, as emacs's does from the minibuffer. It used to show it in the
// prompt's own window, where what was typed next went into the buffer - a
// file, perhaps - rather than the prompt.
func TestSwitchingBufferAtAPromptShowsItInTheTextWindow(t *testing.T) {
	e, scr := newTestEditor(t, "hello")
	other := e.NewBuffer("other")
	if err := other.Insert(text.Pos{}, []rune("other text")); err != nil {
		t.Fatal(err)
	}
	// goto-line, then C-x b other RET inside it, then a digit and RET.
	feed(t, scr, key(t, "C-x", "b"), txt("other"), key(t, "RET"), txt("1"), key(t, "RET"))
	press(t, e, "M-g", "M-g")

	if got := other.String(); got != "other text" {
		t.Errorf("other = %q; what was typed at the prompt went into it", got)
	}
	if e.active.Buf != other {
		t.Errorf("the text window shows %q, want other", e.BufferName(e.active.Buf))
	}
	if e.active.Pt != (text.Pos{}) {
		t.Errorf("point %v; want goto-line 1 to have run in other", e.active.Pt)
	}
}
