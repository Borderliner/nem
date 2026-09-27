package editor

import (
	"slices"
	"testing"

	"github.com/Borderliner/nem/keymap"
	"github.com/gdamore/tcell/v2"
)

// With a Persian keyboard, the keys send Persian letters; bound keys still
// work, as the keys they sit on. C-ب is C-f, M-ب is M-f, C-ط ۲ is C-x 2.
func TestBindingsWorkOnAPersianKeyboard(t *testing.T) {
	e, _ := newTestEditor(t, "one two three")
	press(t, e, "C-ب")
	wantPt(t, e, 0, 1)
	press(t, e, "M-ب")
	wantPt(t, e, 0, 3)
	press(t, e, "C-ط", "۲")
	if n := len(e.tree.Windows()); n != 2 {
		t.Errorf("C-ط ۲ left %d windows, want C-x 2's two", n)
	}
	// C-u ۴ C-ب: a Persian digit is the argument's digit.
	e.active.Pt.Col = 0
	press(t, e, "C-u", "۴", "C-ب")
	wantPt(t, e, 0, 4)
}

// Letters typed on their own are text, in whatever script.
func TestPersianLettersStillType(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "س", "ل", "ا", "م")
	wantText(t, e, "سلام")
}

// C-g cancels on any keyboard - C-ل on a Persian one - and a question's
// answer is read as the key it sits on: y is غ.
func TestQuitAndAnswersOnAPersianKeyboard(t *testing.T) {
	e, scr := newTestEditor(t, "text")
	press(t, e, "C-ط")
	press(t, e, "C-ل")
	if len(e.pending) != 0 {
		t.Error("C-ل did not cancel the pending C-x")
	}
	wantEcho(t, e, "Quit")

	b := e.active.Buf
	press(t, e, "a")     // modified, so killing it asks
	e.NewBuffer("other") // something to show instead
	feed(t, scr, txt("غ"))
	press(t, e, "C-ط", "ن") // C-x k
	if slices.Contains(e.buffers, b) {
		t.Error("answering غ (y) did not kill the buffer")
	}
}

// In a listing, letters are its commands whatever the keyboard: د is n.
func TestListingLettersOnAPersianKeyboard(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "a.txt", "b.txt")
	e, _ := newTestEditor(t)
	listed(t, e, dir)
	goTo(t, e, "a.txt")
	press(t, e, "د")
	if got := atEntry(t, e); got != "b.txt" {
		t.Errorf("د moved to %q, want the next file", got)
	}
}

// A binding made for the letter itself wins, and with the reading off keys
// are what the terminal says.
func TestLayoutReadingCanBeOverruledAndTurnedOff(t *testing.T) {
	e, _ := newTestEditor(t, "abc")
	if err := bindSpec(e.keys, "C-ب", "end-of-buffer"); err != nil {
		t.Fatal(err)
	}
	press(t, e, "C-ب")
	wantPt(t, e, 0, 3)

	e2, _ := newTestEditor(t, "abc")
	e2.layout = keymap.LayoutOff
	press(t, e2, "C-ب")
	wantPt(t, e2, 0, 0)
	wantEcho(t, e2, "undefined")
}

// What a terminal sends for Ctrl and Alt on a Persian letter decodes to the
// letter with the modifier, for commandKey to read back.
func TestModifiedPersianLettersDecode(t *testing.T) {
	for _, tc := range []struct {
		ev   *tcell.EventKey
		want keymap.Key
	}{
		{tcell.NewEventKey(tcell.KeyRune, 'ط', tcell.ModCtrl), keymap.Key{Rune: 'ط', Ctrl: true}},
		{tcell.NewEventKey(tcell.KeyRune, 'ط', tcell.ModAlt), keymap.Key{Rune: 'ط', Meta: true}},
	} {
		if got := DecodeKey(tc.ev, false); got != tc.want {
			t.Errorf("decoded %+v, want %+v", got, tc.want)
		}
	}
}
