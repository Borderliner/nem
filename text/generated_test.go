package text

import "testing"

func TestRegenerateTail(t *testing.T) {
	b := NewBuffer()
	b.Regenerate([]rune("header\n\npartial"))
	b.SetReadOnly(true)

	b.RegenerateTail(2, []rune("partial line\nnext"))
	if got := b.String(); got != "header\n\npartial line\nnext" {
		t.Errorf("text = %q", got)
	}
	if b.Modified() {
		t.Error("the buffer reads as modified")
	}
	if _, ok := b.Undo(); ok {
		t.Error("there is something to undo")
	}
	b.RegenerateTail(99, []rune("last"))
	if got := b.String(); got != "header\n\npartial line\nlast" {
		t.Errorf("a line past the end rewrote %q", got)
	}
}
