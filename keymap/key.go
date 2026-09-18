// Package keymap turns emacs key notation into structured keystrokes and
// resolves sequences of them against a prefix tree of bindings.
//
// This package depends only on the standard library. Translating a terminal
// library's event type into a [Key] is deliberately somebody else's job, which
// is what keeps every rule in here unit-testable without a terminal.
package keymap

import "strings"

// Key is a single keystroke.
type Key struct {
	Rune    rune       // base character; 0 when Special != SpecialNone
	Special SpecialKey // non-None for keys that have no character
	Ctrl    bool
	Meta    bool // emacs Meta: Alt, or an ESC prefix
	// Shift is only meaningful when Special != SpecialNone: a rune carries its own
	// case, so S- on a character is meaningless. Normalize enforces this by
	// clearing Shift on any rune key, and ParseSpec rejects "S-a" outright.
	Shift bool
}

// SpecialKey identifies a key that produces no character.
type SpecialKey int

// The special keys nem understands.
const (
	SpecialNone SpecialKey = iota
	KeyEnter
	KeyTab
	KeyBackspace
	KeyDelete
	KeyEscape
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// specialNames maps each special key to its canonical notation. Parsing accepts
// additional aliases (see specialAliases); this table is what String emits.
var specialNames = map[SpecialKey]string{
	KeyEnter:     "<return>",
	KeyTab:       "<tab>",
	KeyBackspace: "<backspace>",
	KeyDelete:    "<delete>",
	KeyEscape:    "<escape>",
	KeyUp:        "<up>",
	KeyDown:      "<down>",
	KeyLeft:      "<left>",
	KeyRight:     "<right>",
	KeyHome:      "<home>",
	KeyEnd:       "<end>",
	KeyPgUp:      "<pgup>",
	KeyPgDn:      "<pgdn>",
	KeyF1:        "<f1>",
	KeyF2:        "<f2>",
	KeyF3:        "<f3>",
	KeyF4:        "<f4>",
	KeyF5:        "<f5>",
	KeyF6:        "<f6>",
	KeyF7:        "<f7>",
	KeyF8:        "<f8>",
	KeyF9:        "<f9>",
	KeyF10:       "<f10>",
	KeyF11:       "<f11>",
	KeyF12:       "<f12>",
}

// String renders the key in canonical emacs notation. The result always parses
// back to the same key via [ParseSpec], for every key this package can produce.
//
// Modifiers are emitted in the fixed order C- M- S-. Shift is emitted only for
// special keys: a rune already carries its own case.
func (k Key) String() string {
	var b strings.Builder
	if k.Ctrl {
		b.WriteString("C-")
	}
	if k.Meta {
		b.WriteString("M-")
	}
	if k.Special != SpecialNone {
		if k.Shift {
			b.WriteString("S-")
		}
		b.WriteString(specialNames[k.Special])
		return b.String()
	}
	if k.Rune == ' ' {
		b.WriteString("SPC")
		return b.String()
	}
	b.WriteRune(k.Rune)
	return b.String()
}
