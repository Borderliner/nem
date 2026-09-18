package keymap

// Normalize folds the ambiguities that terminals impose on control keys, so a
// binding matches no matter which encoding the terminal happened to send.
//
// The folds exist because a terminal does not transmit "Ctrl" as a modifier
// bit for control characters: it transmits a single byte in the C0 range, and
// several distinct keystrokes collapse onto the same byte before the program
// ever sees them. Folding to one canonical spelling is therefore recovering
// information the terminal already destroyed, not a policy choice.
//
// Every fold here is unconditional. The one genuinely contentious case, C-h,
// is opt-in per [Map] via Map.TreatCtrlHAsBackspace.
func Normalize(k Key) Key {
	// A character encodes its own case, so Shift carries no information on a rune
	// key: "S-a" is ambiguous where "A" is not. Clearing it here is what turns the
	// canonical form from a convention into a guarantee — every constructible key
	// normalizes to one that String can round-trip. Special keys keep Shift,
	// because S-<up> is a genuinely distinct keystroke.
	if k.Special == SpecialNone {
		k.Shift = false
	}

	// A bare NUL byte. xterm, foot, kitty and the Linux console all send 0x00
	// for Ctrl+Space, and there is no separate encoding for Ctrl+@. Emacs calls
	// this key C-@ and binds set-mark-command to it.
	if k.Special == SpecialNone && k.Rune == 0 {
		k.Rune = '@'
		k.Ctrl = true
		return k
	}

	if !k.Ctrl || k.Special != SpecialNone {
		return k
	}

	// Ctrl+letter case-folds: the terminal computes byte = char & 0x1F, which
	// is identical for 'x' (0x78) and 'X' (0x58). Ctrl+Shift+x is therefore
	// indistinguishable from Ctrl+x on every VT-lineage terminal.
	if k.Rune >= 'A' && k.Rune <= 'Z' {
		k.Rune = k.Rune - 'A' + 'a'
	}

	switch k.Rune {
	case ' ':
		// Ctrl+Space -> 0x00, the same byte as C-@. See above.
		k.Rune = '@'
	case '/':
		// Ctrl+/ -> 0x1F on xterm, foot, alacritty and kitty, which is the same
		// byte as Ctrl+_ ('_' is 0x5F, and 0x5F & 0x1F == 0x1F). Emacs documents
		// undo as C-/ but receives C-_; both spellings must reach one binding.
		k.Rune = '_'
	case 'i':
		// Ctrl+i -> 0x09, indistinguishable from the Tab key. Universal across
		// terminals; only kitty's extended protocol can separate them.
		return Key{Special: KeyTab, Meta: k.Meta}
	case 'm':
		// Ctrl+m -> 0x0D, indistinguishable from Return. Universal.
		return Key{Special: KeyEnter, Meta: k.Meta}
	case '[':
		// Ctrl+[ -> 0x1B, indistinguishable from Escape. Universal, and the
		// reason a lone ESC is ambiguous with the start of an escape sequence.
		return Key{Special: KeyEscape, Meta: k.Meta}
	}
	return k
}
