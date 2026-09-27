package editor

import (
	"slices"

	"github.com/Borderliner/nem/keymap"
)

// Keyboard layouts: key bindings that work whatever layout is switched on.
// With a Persian keyboard, Ctrl on the x key sends C-ط; read back as the
// Latin key it sits on, it is the C-x it was meant as. See keymap/layout.go.

// commandKey is k as the key it is meant as. A letter of another layout is
// read as the Latin key in its place wherever it must be a command rather
// than text: with Ctrl or Meta held, continuing a prefix (C-x ب is C-x f),
// as a digit of C-u's argument, after C-x e where e plays the macro again,
// and in a listing, where letters are its commands. Typed into a buffer or a
// prompt on its own, a letter is the letter.
//
// A binding made for the letter itself - C-ط in someone's config - wins.
func (e *Editor) commandKey(k keymap.Key) keymap.Key {
	if k.Special != keymap.SpecialNone {
		return k
	}
	latin, ok := keymap.LatinKey(k.Rune, e.layout)
	if !ok {
		return k
	}
	digit := latin >= '0' && latin <= '9'
	if !(k.Ctrl || k.Meta || len(e.pending) > 0 || e.km.repeatE || (e.arg.active && digit) || e.lettersAreCommands()) {
		return k
	}
	if res, _ := e.lookup(append(slices.Clone(e.pending), k)); res.Kind != keymap.Undefined {
		return k
	}
	k.Rune = latin
	return k
}

// lettersAreCommands reports whether a bare letter typed now runs a command
// rather than typing itself: in a read-only listing with keys of its own,
// such as dired.
func (e *Editor) lettersAreCommands() bool {
	return e.mini == nil && e.modeKeys(e.active.Buf) != nil && e.active.Buf.ReadOnly()
}

// answerKey is k read as the answer to a question with the answers valid: a
// letter of another layout is its Latin key when it is not an answer as it
// stands - y is غ on a Persian keyboard - and with Ctrl held, so C-g still
// cancels.
func (e *Editor) answerKey(k keymap.Key, valid []rune) keymap.Key {
	if k.Special != keymap.SpecialNone || (len(valid) > 0 && containsRune(valid, k.Rune) && !k.Ctrl && !k.Meta) {
		return k
	}
	if latin, ok := keymap.LatinKey(k.Rune, e.layout); ok && (k.Ctrl || k.Meta || len(valid) > 0) {
		k.Rune = latin
	}
	return k
}
