package editor

import (
	"github.com/Borderliner/nem/keymap"
	"github.com/gdamore/tcell/v2"
)

// tcellSpecials maps tcell's named keys onto keymap's. Only keys keymap knows
// appear here; anything absent decodes to the zero Key (see DecodeKey).
//
// The entries for 0x08/0x09/0x0D/0x1B/0x7F must be consulted before the C0
// fallback below, because tcell reports those five bytes as named keys rather
// than as control characters.
var tcellSpecials = map[tcell.Key]keymap.SpecialKey{
	tcell.KeyEnter:     keymap.KeyEnter,     // 0x0D
	tcell.KeyTab:       keymap.KeyTab,       // 0x09
	tcell.KeyBackspace: keymap.KeyBackspace, // 0x08
	tcell.KeyDEL:       keymap.KeyBackspace, // 0x7F, see DecodeKey's note on C-h
	tcell.KeyEscape:    keymap.KeyEscape,    // 0x1B
	tcell.KeyDelete:    keymap.KeyDelete,
	tcell.KeyUp:        keymap.KeyUp,
	tcell.KeyDown:      keymap.KeyDown,
	tcell.KeyLeft:      keymap.KeyLeft,
	tcell.KeyRight:     keymap.KeyRight,
	tcell.KeyHome:      keymap.KeyHome,
	tcell.KeyEnd:       keymap.KeyEnd,
	tcell.KeyPgUp:      keymap.KeyPgUp,
	tcell.KeyPgDn:      keymap.KeyPgDn,
	tcell.KeyF1:        keymap.KeyF1,
	tcell.KeyF2:        keymap.KeyF2,
	tcell.KeyF3:        keymap.KeyF3,
	tcell.KeyF4:        keymap.KeyF4,
	tcell.KeyF5:        keymap.KeyF5,
	tcell.KeyF6:        keymap.KeyF6,
	tcell.KeyF7:        keymap.KeyF7,
	tcell.KeyF8:        keymap.KeyF8,
	tcell.KeyF9:        keymap.KeyF9,
	tcell.KeyF10:       keymap.KeyF10,
	tcell.KeyF11:       keymap.KeyF11,
	tcell.KeyF12:       keymap.KeyF12,
}

// DecodeKey translates a tcell key event into a canonical [keymap.Key].
//
// This is the only place in nem where tcell meets keymap. The boundary is
// deliberate: keymap depends on nothing but the standard library and is
// therefore testable without a terminal, and every terminal-specific
// irregularity is absorbed here.
//
// The returned key is always already normalized, so it can be appended to a
// pending sequence and looked up directly.
//
// An event this function does not recognise — tcell's KeyInsert, mouse and
// paste keys, function keys past F12 — decodes to the zero Key, which
// [keymap.Normalize] can never produce. Callers should treat a zero Key as
// "no binding could name this" and ignore the event.
//
// # Escape versus Meta
//
// This function does NOT implement escape-sequence timing, because tcell
// already owns it. A terminal delivers Meta+x as ESC followed by x, which is
// indistinguishable from Escape then x except by arrival time. tcell's input
// parser resolves it with a 50ms expiry and a 60ms timer (input.go: it sets
// ip.expire to now+50ms and arms time.AfterFunc(60ms, ip.escTimeout)), then
// rewrites an ESC-prefixed key by adding ModAlt. So by the time an event
// reaches here the ambiguity is gone. Do not reimplement it.
//
// # Ctrl+H, and why treatCtrlHAsBackspace barely matters
//
// tcell merges the Backspace key and Ctrl+H in two independent places: its
// input parser maps both 0x08 and 0x7F to KeyBackspace ("case '\b', '\x7F'"),
// and NewEventKey rewrites KeyBackspace2 to KeyBackspace unconditionally. A
// real terminal therefore cannot deliver C-h as distinct from <backspace>,
// which means emacs's C-h help prefix is unreachable under tcell's legacy
// input path regardless of this flag.
//
// The flag is honoured for the one event that can still express the
// difference: an explicitly constructed KeyCtrlH (tcell numbers it 72, well
// clear of KeyBackspace at 8). When false that decodes to C-h; when true it
// folds to <backspace>.
func DecodeKey(ev *tcell.EventKey, treatCtrlHAsBackspace bool) keymap.Key {
	k := ev.Key()
	mod := ev.Modifiers()

	var out keymap.Key
	switch {
	case k == tcell.KeyRune:
		// ModCtrl is consulted here and nowhere else, because this is the only
		// branch where the Ctrl is not already encoded in the key constant.
		//
		// On the legacy input path tcell folds Ctrl into its KeyCtrlX constants,
		// so the modifier bit adds nothing. But tcell requests the advanced
		// keyboard protocols on any XTermLike terminal (tscreen.go sends
		// modifyOtherKeys, kitty CSI-u and win32-input-mode), and under CSI-u a
		// Ctrl'd character arrives as KeyRune plus ModCtrl instead. Dropping the
		// bit there silently turned C-SPC into a literal space and C-/ into a
		// literal slash, making set-mark and undo dead keys on exactly the
		// modern terminals that report the most detail.
		//
		// Applying it outside this branch would be wrong: tcell reports the C0
		// control codes with ModCtrl set, so a blanket rule turns <backspace>
		// into C-<backspace> and RET into C-RET.
		out = keymap.Key{Rune: ev.Rune(), Ctrl: mod&tcell.ModCtrl != 0}

	case k == tcell.KeyBacktab:
		// NewEventKey rewrites Shift+Tab to KeyBacktab and clears ModShift, so
		// the Shift has to be put back by hand.
		out = keymap.Key{Special: keymap.KeyTab, Shift: true}

	case k == tcell.KeyCtrlH:
		if treatCtrlHAsBackspace {
			out = keymap.Key{Special: keymap.KeyBackspace}
		} else {
			out = keymap.Key{Rune: 'h', Ctrl: true}
		}

	default:
		if sk, ok := tcellSpecials[k]; ok {
			out = keymap.Key{Special: sk}
			break
		}
		switch {
		case k >= tcell.KeyCtrlSpace && k <= tcell.KeyCtrlUnderscore:
			// tcell numbers these 64..95, which are exactly the ASCII codes of
			// '@' and 'A'..'Z' and '['..'_' — the characters whose Ctrl'd form
			// produces C0 bytes 0..31. So the constant doubles as the rune.
			// Normalize then case-folds 'A'..'Z' down to lower case.
			out = keymap.Key{Rune: rune(k), Ctrl: true}
		case k < 0x20:
			// A raw C0 byte, which is how tcell reports a control character
			// built as NewEventKey(KeyRune, ch). The inverse of the terminal's
			// "byte = char & 0x1F" is "char = byte | 0x40", giving '@'..'_'.
			// Note this is NOT tcell's own ch+0x60, which is correct only for
			// the 26 letters and would turn 0x1F into DEL instead of '_'.
			out = keymap.Key{Rune: rune(k) | 0x40, Ctrl: true}
		default:
			return keymap.Key{} // unrecognised; caller ignores it
		}
	}

	if mod&tcell.ModAlt != 0 {
		out.Meta = true
	}
	if mod&tcell.ModShift != 0 {
		// Meaningful only on special keys; Normalize clears it on runes, where
		// the character already carries its own case.
		out.Shift = true
	}
	return keymap.Normalize(out)
}
