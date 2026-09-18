package keymap

import "testing"

func TestNormalizeFolds(t *testing.T) {
	cases := []struct {
		name string
		in   Key
		want Key
	}{
		{"NUL byte is C-@", Key{Rune: 0}, Key{Rune: '@', Ctrl: true}},
		{"C-SPC arrives as NUL, i.e. C-@", Key{Rune: ' ', Ctrl: true}, Key{Rune: '@', Ctrl: true}},
		{"C-/ is C-_ (both 0x1F)", Key{Rune: '/', Ctrl: true}, Key{Rune: '_', Ctrl: true}},
		{"C-i is TAB (0x09)", Key{Rune: 'i', Ctrl: true}, Key{Special: KeyTab}},
		{"C-m is RET (0x0D)", Key{Rune: 'm', Ctrl: true}, Key{Special: KeyEnter}},
		{"C-[ is ESC (0x1B)", Key{Rune: '[', Ctrl: true}, Key{Special: KeyEscape}},
		{"ctrl letters case-fold", Key{Rune: 'X', Ctrl: true}, Key{Rune: 'x', Ctrl: true}},
		{"case-fold then alias-fold", Key{Rune: 'I', Ctrl: true}, Key{Special: KeyTab}},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("%s: Normalize(%s) = %s, want %s", c.name, dbg(c.in), dbg(got), dbg(c.want))
		}
	}
}

func TestNormalizePreservesMeta(t *testing.T) {
	// ESC C-i must fold to M-<tab>, not lose the Meta flag.
	got := Normalize(Key{Rune: 'i', Ctrl: true, Meta: true})
	want := Key{Special: KeyTab, Meta: true}
	if got != want {
		t.Errorf("Normalize(%s) = %s, want %s", dbg(Key{Rune: 'i', Ctrl: true, Meta: true}), dbg(got), dbg(want))
	}
}

func TestNormalizeLeavesUnrelatedKeysAlone(t *testing.T) {
	untouched := []Key{
		{Rune: 'i'},                  // plain i is not TAB
		{Rune: 'm'},                  // plain m is not RET
		{Rune: '['},                  // plain [ is not ESC
		{Rune: '/'},                  // plain / is not C-_
		{Rune: ' '},                  // plain space is not C-@
		{Rune: 'X'},                  // case matters without Ctrl
		{Rune: 'x', Ctrl: true},      // ordinary ctrl key
		{Special: KeyUp, Ctrl: true}, // specials pass through
	}
	for _, k := range untouched {
		if got := Normalize(k); got != k {
			t.Errorf("Normalize(%s) = %s, want it unchanged", dbg(k), dbg(got))
		}
	}
}

func TestNormalizeDoesNotFoldCtrlHByDefault(t *testing.T) {
	// C-h is backspace on many terminals but is help-prefix in emacs. Folding it
	// is opt-in per Map, never global.
	in := Key{Rune: 'h', Ctrl: true}
	if got := Normalize(in); got != in {
		t.Errorf("Normalize(C-h) = %s, want it unchanged", dbg(got))
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	for _, k := range representativeKeys() {
		once := Normalize(k)
		if twice := Normalize(once); twice != once {
			t.Errorf("Normalize not idempotent for %s: %s -> %s", dbg(k), dbg(once), dbg(twice))
		}
	}
}

func TestNormalizedKeysStillRoundTrip(t *testing.T) {
	// A folded key must still render and re-parse, or config files break.
	for _, k := range representativeKeys() {
		n := Normalize(k)
		parsed, err := ParseSpec(n.String())
		if err != nil {
			t.Errorf("Normalize(%s).String() = %q does not parse: %v", dbg(k), n.String(), err)
			continue
		}
		if parsed[0] != n {
			t.Errorf("normalized key %s does not round-trip: got %s", dbg(n), dbg(parsed[0]))
		}
	}
}
