package keymap

import "testing"

func TestKeyStringPlainRune(t *testing.T) {
	got := Key{Rune: 'a'}.String()
	if got != "a" {
		t.Errorf("Key{Rune:'a'}.String() = %q, want %q", got, "a")
	}
}

func TestKeyStringCtrlRune(t *testing.T) {
	got := Key{Rune: 'x', Ctrl: true}.String()
	if got != "C-x" {
		t.Errorf("Key{Rune:'x',Ctrl:true}.String() = %q, want %q", got, "C-x")
	}
}

func TestKeyStringMetaRune(t *testing.T) {
	got := Key{Rune: 'x', Meta: true}.String()
	if got != "M-x" {
		t.Errorf("Key{Rune:'x',Meta:true}.String() = %q, want %q", got, "M-x")
	}
}

func TestKeyStringCtrlMetaOrdersCtrlFirst(t *testing.T) {
	got := Key{Rune: 'f', Ctrl: true, Meta: true}.String()
	if got != "C-M-f" {
		t.Errorf("Key{Rune:'f',Ctrl,Meta}.String() = %q, want %q", got, "C-M-f")
	}
}

func TestKeyStringSpaceIsSPC(t *testing.T) {
	got := Key{Rune: ' '}.String()
	if got != "SPC" {
		t.Errorf("Key{Rune:' '}.String() = %q, want %q", got, "SPC")
	}
}

func TestKeyStringSpecialKeys(t *testing.T) {
	cases := []struct {
		key  Key
		want string
	}{
		{Key{Special: KeyEnter}, "<return>"},
		{Key{Special: KeyTab}, "<tab>"},
		{Key{Special: KeyBackspace}, "<backspace>"},
		{Key{Special: KeyDelete}, "<delete>"},
		{Key{Special: KeyEscape}, "<escape>"},
		{Key{Special: KeyUp}, "<up>"},
		{Key{Special: KeyDown}, "<down>"},
		{Key{Special: KeyLeft}, "<left>"},
		{Key{Special: KeyRight}, "<right>"},
		{Key{Special: KeyHome}, "<home>"},
		{Key{Special: KeyEnd}, "<end>"},
		{Key{Special: KeyPgUp}, "<pgup>"},
		{Key{Special: KeyPgDn}, "<pgdn>"},
		{Key{Special: KeyF1}, "<f1>"},
		{Key{Special: KeyF12}, "<f12>"},
	}
	for _, c := range cases {
		if got := c.key.String(); got != c.want {
			t.Errorf("Key{Special:%d}.String() = %q, want %q", c.key.Special, got, c.want)
		}
	}
}

func TestKeyStringShiftOnSpecialOrdersAfterCtrlMeta(t *testing.T) {
	got := Key{Special: KeyUp, Ctrl: true, Shift: true}.String()
	if got != "C-S-<up>" {
		t.Errorf("got %q, want %q", got, "C-S-<up>")
	}
}

func TestKeyStringShiftIgnoredOnRune(t *testing.T) {
	// A rune carries its own case, so S- is never emitted for one.
	got := Key{Rune: 'a', Shift: true}.String()
	if got != "a" {
		t.Errorf("got %q, want %q", got, "a")
	}
}
