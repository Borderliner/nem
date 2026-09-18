package editor

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/keymap"
)

// dbg renders a Key by its fields. Key implements Stringer, so %+v would print
// canonical notation and hide exactly the field differences a failure is about.
func dbg(k keymap.Key) string {
	return fmt.Sprintf("{Rune:%q Special:%d Ctrl:%v Meta:%v Shift:%v}",
		k.Rune, k.Special, k.Ctrl, k.Meta, k.Shift)
}

func sp(s keymap.SpecialKey) keymap.Key { return keymap.Key{Special: s} }
func ctrl(r rune) keymap.Key            { return keymap.Key{Rune: r, Ctrl: true} }
func plain(r rune) keymap.Key           { return keymap.Key{Rune: r} }

func TestDecodeKeyNamedSpecials(t *testing.T) {
	tests := []struct {
		name string
		key  tcell.Key
		want keymap.Key
	}{
		{"up", tcell.KeyUp, sp(keymap.KeyUp)},
		{"down", tcell.KeyDown, sp(keymap.KeyDown)},
		{"left", tcell.KeyLeft, sp(keymap.KeyLeft)},
		{"right", tcell.KeyRight, sp(keymap.KeyRight)},
		{"home", tcell.KeyHome, sp(keymap.KeyHome)},
		{"end", tcell.KeyEnd, sp(keymap.KeyEnd)},
		{"pgup", tcell.KeyPgUp, sp(keymap.KeyPgUp)},
		{"pgdn", tcell.KeyPgDn, sp(keymap.KeyPgDn)},
		{"delete", tcell.KeyDelete, sp(keymap.KeyDelete)},
		{"insert-is-unmapped", tcell.KeyInsert, keymap.Key{}},
		{"f1", tcell.KeyF1, sp(keymap.KeyF1)},
		{"f5", tcell.KeyF5, sp(keymap.KeyF5)},
		{"f12", tcell.KeyF12, sp(keymap.KeyF12)},
		{"tab", tcell.KeyTab, sp(keymap.KeyTab)},
		{"enter", tcell.KeyEnter, sp(keymap.KeyEnter)},
		{"escape", tcell.KeyEscape, sp(keymap.KeyEscape)},
		{"backspace-0x08", tcell.KeyBackspace, sp(keymap.KeyBackspace)},
		{"backspace2-0x7F-collapses", tcell.KeyBackspace2, sp(keymap.KeyBackspace)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeKey(tcell.NewEventKey(tt.key, 0, tcell.ModNone), false)
			if got != tt.want {
				t.Errorf("DecodeKey(%s) = %s, want %s", tt.name, dbg(got), dbg(tt.want))
			}
		})
	}
}

// TestDecodeKeyCtrlLetters pins the whole KeyCtrlA..KeyCtrlZ range. tcell numbers
// these 65..90 (the ASCII codes of the uppercase letters), NOT the C0 control
// codes, so they do not collide with KeyTab/KeyEnter/KeyEscape the way raw bytes
// do. The emacs-equivalent folds still happen, but in keymap.Normalize.
func TestDecodeKeyCtrlLetters(t *testing.T) {
	for r := 'a'; r <= 'z'; r++ {
		k := tcell.KeyCtrlA + tcell.Key(r-'a')
		want := ctrl(r)
		switch r {
		case 'h': // KeyCtrlH is distinct from KeyBackspace in tcell's numbering
			want = ctrl('h')
		case 'i': // Normalize folds C-i to <tab>
			want = sp(keymap.KeyTab)
		case 'm': // Normalize folds C-m to <return>
			want = sp(keymap.KeyEnter)
		}
		got := DecodeKey(tcell.NewEventKey(k, 0, tcell.ModCtrl), false)
		if got != want {
			t.Errorf("DecodeKey(KeyCtrl%c) = %s, want %s", r-'a'+'A', dbg(got), dbg(want))
		}
	}
}

// TestDecodeKeyC0Punctuation covers the five KeyCtrl* constants above KeyCtrlZ.
// C-_ matters most: it is undo, and it is also what C-/ arrives as.
func TestDecodeKeyC0Punctuation(t *testing.T) {
	tests := []struct {
		name string
		key  tcell.Key
		want keymap.Key
	}{
		{"ctrl-leftsq-is-escape", tcell.KeyCtrlLeftSq, sp(keymap.KeyEscape)},
		{"ctrl-backslash", tcell.KeyCtrlBackslash, ctrl('\\')},
		{"ctrl-rightsq", tcell.KeyCtrlRightSq, ctrl(']')},
		{"ctrl-carat", tcell.KeyCtrlCarat, ctrl('^')},
		{"ctrl-underscore-is-undo", tcell.KeyCtrlUnderscore, ctrl('_')},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeKey(tcell.NewEventKey(tt.key, 0, tcell.ModCtrl), false)
			if got != tt.want {
				t.Errorf("= %s, want %s", dbg(got), dbg(tt.want))
			}
		})
	}
}

// TestDecodeKeySetMark: every encoding of Ctrl+Space must reach C-@, which is
// what set-mark-command binds to.
func TestDecodeKeySetMark(t *testing.T) {
	want := ctrl('@')
	cases := []struct {
		name string
		ev   *tcell.EventKey
	}{
		{"KeyCtrlSpace", tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModCtrl)},
		{"KeyNUL", tcell.NewEventKey(tcell.KeyNUL, 0, tcell.ModCtrl)},
		{"rune-0", tcell.NewEventKey(tcell.KeyRune, 0, tcell.ModCtrl)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DecodeKey(c.ev, false); got != want {
				t.Errorf("= %s, want %s", dbg(got), dbg(want))
			}
		})
	}
}

// TestDecodeKeyCtrlH is the one place the flag has any effect. tcell merges the
// Backspace key (0x7F) and Ctrl+H (0x08) into KeyBackspace in TWO independent
// places, so a real terminal cannot reach C-h at all; only an explicitly
// constructed KeyCtrlH can, which is what this covers.
func TestDecodeKeyCtrlH(t *testing.T) {
	tests := []struct {
		name string
		key  tcell.Key
		flag bool
		want keymap.Key
	}{
		{"KeyCtrlH flag off stays C-h", tcell.KeyCtrlH, false, ctrl('h')},
		{"KeyCtrlH flag on folds to backspace", tcell.KeyCtrlH, true, sp(keymap.KeyBackspace)},
		{"KeyBackspace flag off", tcell.KeyBackspace, false, sp(keymap.KeyBackspace)},
		{"KeyBackspace flag on", tcell.KeyBackspace, true, sp(keymap.KeyBackspace)},
		{"KeyBackspace2 flag off", tcell.KeyBackspace2, false, sp(keymap.KeyBackspace)},
		{"KeyBackspace2 flag on", tcell.KeyBackspace2, true, sp(keymap.KeyBackspace)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeKey(tcell.NewEventKey(tt.key, 0, tcell.ModCtrl), tt.flag)
			if got != tt.want {
				t.Errorf("= %s, want %s", dbg(got), dbg(tt.want))
			}
		})
	}
}

func TestDecodeKeyRunes(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		mod  tcell.ModMask
		want keymap.Key
	}{
		{"ascii", 'x', tcell.ModNone, plain('x')},
		{"uppercase-keeps-case", 'X', tcell.ModNone, plain('X')},
		{"space", ' ', tcell.ModNone, plain(' ')},
		{"latin1-combining-base", 'é', tcell.ModNone, plain('é')},
		{"cjk", '日', tcell.ModNone, plain('日')},
		{"emoji", '🚀', tcell.ModNone, plain('🚀')},
		{"meta-rune", 'x', tcell.ModAlt, keymap.Key{Rune: 'x', Meta: true}},
		{"meta-uppercase", 'X', tcell.ModAlt, keymap.Key{Rune: 'X', Meta: true}},
		{"meta-cjk", '日', tcell.ModAlt, keymap.Key{Rune: '日', Meta: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeKey(tcell.NewEventKey(tcell.KeyRune, tt.r, tt.mod), false)
			if got != tt.want {
				t.Errorf("= %s, want %s", dbg(got), dbg(tt.want))
			}
		})
	}
}

func TestDecodeKeyModifierCombinations(t *testing.T) {
	tests := []struct {
		name string
		ev   *tcell.EventKey
		want keymap.Key
	}{
		{"M-<up>", tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt),
			keymap.Key{Special: keymap.KeyUp, Meta: true}},
		{"S-<up>", tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModShift),
			keymap.Key{Special: keymap.KeyUp, Shift: true}},
		{"C-M-f", tcell.NewEventKey(tcell.KeyCtrlF, 0, tcell.ModCtrl|tcell.ModAlt),
			keymap.Key{Rune: 'f', Ctrl: true, Meta: true}},
		// Ctrl+Shift+x is indistinguishable from Ctrl+x on a VT-lineage terminal;
		// tcell hands back an uppercase rune and Normalize case-folds it.
		{"C-S-x folds to C-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModCtrl|tcell.ModShift),
			ctrl('x')},
		// NewEventKey rewrites Shift+Tab to KeyBacktab and clears ModShift.
		{"S-<tab> becomes backtab", tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModShift),
			keymap.Key{Special: keymap.KeyTab, Shift: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecodeKey(tt.ev, false); got != tt.want {
				t.Errorf("= %s, want %s", dbg(got), dbg(tt.want))
			}
		})
	}
}

// TestDecodeKeyRawC0 covers the other representation of a control key: when a
// caller builds NewEventKey(KeyRune, <C0 byte>), tcell rewrites it to the raw
// byte value rather than a KeyCtrl* constant, so both shapes reach DecodeKey.
func TestDecodeKeyRawC0(t *testing.T) {
	tests := []struct {
		name string
		ch   rune
		want keymap.Key
	}{
		{"0x18 is C-x", 0x18, ctrl('x')},
		{"0x01 is C-a", 0x01, ctrl('a')},
		{"0x0B is C-k", 0x0B, ctrl('k')},
		{"0x19 is C-y", 0x19, ctrl('y')},
		{"0x1F is C-_ (undo)", 0x1F, ctrl('_')},
		{"0x09 is tab", 0x09, sp(keymap.KeyTab)},
		{"0x0D is return", 0x0D, sp(keymap.KeyEnter)},
		{"0x1B is escape", 0x1B, sp(keymap.KeyEscape)},
		{"0x08 is backspace", 0x08, sp(keymap.KeyBackspace)},
		{"0x7F is backspace", 0x7F, sp(keymap.KeyBackspace)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeKey(tcell.NewEventKey(tcell.KeyRune, tt.ch, tcell.ModNone), false)
			if got != tt.want {
				t.Errorf("= %s, want %s", dbg(got), dbg(tt.want))
			}
		})
	}
}

// TestDecodeKeyAlwaysNormalized is the invariant that matters: whatever tcell
// hands us, the result is already in canonical form, so Map lookups match.
func TestDecodeKeyAlwaysNormalized(t *testing.T) {
	var evs []*tcell.EventKey
	for k := tcell.KeyCtrlSpace; k <= tcell.KeyCtrlUnderscore; k++ {
		evs = append(evs, tcell.NewEventKey(k, 0, tcell.ModCtrl))
	}
	for _, k := range []tcell.Key{
		tcell.KeyUp, tcell.KeyDown, tcell.KeyTab, tcell.KeyEnter, tcell.KeyEscape,
		tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyDelete, tcell.KeyF1, tcell.KeyF12,
	} {
		evs = append(evs, tcell.NewEventKey(k, 0, tcell.ModNone))
		evs = append(evs, tcell.NewEventKey(k, 0, tcell.ModAlt))
	}
	for _, r := range []rune{'a', 'Z', ' ', 'é', '日', '🚀'} {
		evs = append(evs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		evs = append(evs, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModAlt))
	}
	for _, flag := range []bool{false, true} {
		for _, ev := range evs {
			got := DecodeKey(ev, flag)
			if renorm := keymap.Normalize(got); renorm != got {
				t.Errorf("DecodeKey(%v, %v) = %s not canonical; Normalize gives %s",
					ev.Name(), flag, dbg(got), dbg(renorm))
			}
		}
	}
}

// TestDecodeKeyRoundTripsThroughSpec ties the decoder to the binding syntax a
// Lua config will use: a decoded key must render to notation that ParseSpec
// reads back as the same key, or a config could never name it.
func TestDecodeKeyRoundTripsThroughSpec(t *testing.T) {
	evs := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyCtrlUnderscore, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyCtrlSpace, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModAlt),
		tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt),
		tcell.NewEventKey(tcell.KeyF5, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone),
	}
	for _, ev := range evs {
		k := DecodeKey(ev, false)
		spec := k.String()
		back, err := keymap.ParseSpec(spec)
		if err != nil {
			t.Errorf("%s -> %q: ParseSpec: %v", ev.Name(), spec, err)
			continue
		}
		if len(back) != 1 || back[0] != k {
			t.Errorf("%s -> %q -> %v, want single %s", ev.Name(), spec, back, dbg(k))
		}
	}
}
