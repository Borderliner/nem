package keymap

import "testing"

func mustParse(t *testing.T, spec string) []Key {
	t.Helper()
	keys, err := ParseSpec(spec)
	if err != nil {
		t.Fatalf("ParseSpec(%q) returned error: %v", spec, err)
	}
	return keys
}

func TestParseSpecBareRune(t *testing.T) {
	keys := mustParse(t, "a")
	want := []Key{{Rune: 'a'}}
	if len(keys) != 1 || keys[0] != want[0] {
		t.Errorf("ParseSpec(\"a\") = %s, want %s", dbgSeq(keys), dbgSeq(want))
	}
}

func TestParseSpecCtrlAndMeta(t *testing.T) {
	cases := []struct {
		spec string
		want Key
	}{
		{"C-x", Key{Rune: 'x', Ctrl: true}},
		{"M-x", Key{Rune: 'x', Meta: true}},
		{"C-M-f", Key{Rune: 'f', Ctrl: true, Meta: true}},
		{"C--", Key{Rune: '-', Ctrl: true}},
		{"M-<", Key{Rune: '<', Meta: true}},
	}
	for _, c := range cases {
		keys := mustParse(t, c.spec)
		if len(keys) != 1 || keys[0] != c.want {
			t.Errorf("ParseSpec(%q) = %s, want [%s]", c.spec, dbgSeq(keys), dbg(c.want))
		}
	}
}

func TestParseSpecSequenceSplitsOnSpaces(t *testing.T) {
	keys := mustParse(t, "C-x C-f")
	want := []Key{{Rune: 'x', Ctrl: true}, {Rune: 'f', Ctrl: true}}
	if len(keys) != 2 || keys[0] != want[0] || keys[1] != want[1] {
		t.Errorf("ParseSpec(\"C-x C-f\") = %s, want %s", dbgSeq(keys), dbgSeq(want))
	}
}

func TestParseSpecAngleBracketSpecials(t *testing.T) {
	cases := []struct {
		spec string
		want Key
	}{
		{"<f5>", Key{Special: KeyF5}},
		{"<return>", Key{Special: KeyEnter}},
		{"<tab>", Key{Special: KeyTab}},
		{"<backspace>", Key{Special: KeyBackspace}},
		{"<delete>", Key{Special: KeyDelete}},
		{"<escape>", Key{Special: KeyEscape}},
		{"<up>", Key{Special: KeyUp}},
		{"<pgup>", Key{Special: KeyPgUp}},
		{"<prior>", Key{Special: KeyPgUp}}, // emacs alias
		{"<next>", Key{Special: KeyPgDn}},  // emacs alias
	}
	for _, c := range cases {
		keys := mustParse(t, c.spec)
		if len(keys) != 1 || keys[0] != c.want {
			t.Errorf("ParseSpec(%q) = %s, want [%s]", c.spec, dbgSeq(keys), dbg(c.want))
		}
	}
}

func TestParseSpecShiftOnSpecial(t *testing.T) {
	keys := mustParse(t, "C-S-<up>")
	want := Key{Special: KeyUp, Ctrl: true, Shift: true}
	if len(keys) != 1 || keys[0] != want {
		t.Errorf("ParseSpec(\"C-S-<up>\") = %s, want [%s]", dbgSeq(keys), dbg(want))
	}
}

func TestParseSpecBareNameAliases(t *testing.T) {
	cases := []struct {
		spec string
		want Key
	}{
		{"SPC", Key{Rune: ' '}},
		{"RET", Key{Special: KeyEnter}},
		{"TAB", Key{Special: KeyTab}},
		{"ESC", Key{Special: KeyEscape}},
		// emacs DEL is the backspace key, not forward-delete. <delete> is forward.
		{"DEL", Key{Special: KeyBackspace}},
		{"C-SPC", Key{Rune: ' ', Ctrl: true}},
		{"M-RET", Key{Special: KeyEnter, Meta: true}},
	}
	for _, c := range cases {
		keys := mustParse(t, c.spec)
		if len(keys) != 1 || keys[0] != c.want {
			t.Errorf("ParseSpec(%q) = %s, want [%s]", c.spec, dbgSeq(keys), dbg(c.want))
		}
	}
}

func TestParseSpecRejectsMalformed(t *testing.T) {
	bad := []string{
		"",            // empty
		"   ",         // whitespace only
		"C-",          // dangling modifier
		"M-",          // dangling modifier
		"<nosuchkey>", // unknown special
		"<up",         // unterminated
		"up>",         // unopened
		"abc",         // multi-rune base that is not a known name
		"S-a",         // shift on a rune is meaningless
		"C-C-x",       // duplicate modifier
	}
	for _, spec := range bad {
		if _, err := ParseSpec(spec); err == nil {
			t.Errorf("ParseSpec(%q) = nil error, want an error", spec)
		}
	}
}

func TestParseSpecNeverPanics(t *testing.T) {
	inputs := []string{"-", "--", "C--", "<", ">", "<>", "C-<", "S-", "C-M-S-", "\t\n"}
	for _, spec := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ParseSpec(%q) panicked: %v", spec, r)
				}
			}()
			_, _ = ParseSpec(spec)
		}()
	}
}
