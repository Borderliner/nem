package keymap

import "testing"

// representativeKeys enumerates every key this package can construct, across
// every legal modifier combination. Runes never carry Shift (a character
// encodes its own case), so that axis is only applied to special keys.
func representativeKeys() []Key {
	var keys []Key

	runes := []rune{
		'a', 'z', 'A', 'Z', '0', '9',
		' ', '-', '<', '>', '/', '_', '@', '[', ']', '\\', '?', '.', ',', ';',
		'C', 'M', 'S', // letters that are also modifier prefixes
		'é', '中', '👍', // multi-byte
	}
	for _, r := range runes {
		for _, ctrl := range []bool{false, true} {
			for _, meta := range []bool{false, true} {
				keys = append(keys, Key{Rune: r, Ctrl: ctrl, Meta: meta})
			}
		}
	}

	for sk := range specialNames {
		for _, ctrl := range []bool{false, true} {
			for _, meta := range []bool{false, true} {
				for _, shift := range []bool{false, true} {
					keys = append(keys, Key{Special: sk, Ctrl: ctrl, Meta: meta, Shift: shift})
				}
			}
		}
	}
	return keys
}

func TestStringParseSpecRoundTrip(t *testing.T) {
	for _, want := range representativeKeys() {
		spec := want.String()
		got, err := ParseSpec(spec)
		if err != nil {
			t.Errorf("ParseSpec(%s.String() = %q) errored: %v", dbg(want), spec, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("ParseSpec(%q) returned %d keys, want 1", spec, len(got))
			continue
		}
		if got[0] != want {
			t.Errorf("round-trip of %q: got %s, want %s", spec, dbg(got[0]), dbg(want))
		}
	}
}

func TestStringIsStableAcrossRoundTrip(t *testing.T) {
	// The canonical form must be a fixed point: rendering a parsed key returns
	// the identical string, so config files and describe-bindings agree.
	for _, k := range representativeKeys() {
		once := k.String()
		parsed, err := ParseSpec(once)
		if err != nil {
			t.Fatalf("ParseSpec(%q): %v", once, err)
		}
		if twice := parsed[0].String(); twice != once {
			t.Errorf("String not stable: %q -> %q", once, twice)
		}
	}
}
