package keymap

import (
	"fmt"
	"strings"
)

// specialByName resolves an angle-bracketed name to its key. It holds every
// canonical name from specialNames plus the aliases emacs users expect.
var specialByName = func() map[string]SpecialKey {
	m := make(map[string]SpecialKey, len(specialNames)+8)
	for sk, name := range specialNames {
		m[name] = sk
	}
	// Aliases accepted on input but never emitted by String.
	m["<prior>"] = KeyPgUp // emacs' own name for Page Up
	m["<next>"] = KeyPgDn  // emacs' own name for Page Down
	m["<pageup>"] = KeyPgUp
	m["<pagedown>"] = KeyPgDn
	m["<deletechar>"] = KeyDelete
	m["<enter>"] = KeyEnter
	m["<esc>"] = KeyEscape
	return m
}()

// bareRuneNames are unbracketed names that stand for a character.
var bareRuneNames = map[string]rune{
	"SPC": ' ',
}

// bareSpecialNames are unbracketed names that stand for a special key.
//
// Note DEL: in emacs notation DEL is the *backspace* key (ASCII 0x7F), not
// forward-delete. Forward-delete is <delete> (emacs spells it <deletechar>).
// This trips people up constantly, so it is spelled out here.
var bareSpecialNames = map[string]SpecialKey{
	"RET": KeyEnter,
	"TAB": KeyTab,
	"ESC": KeyEscape,
	"DEL": KeyBackspace,
}

// ParseSpec parses emacs key notation into a key sequence. Keys are separated
// by whitespace, so "C-x C-f" is two keystrokes and "C-x" is one.
//
// It never panics; malformed input always comes back as an error.
func ParseSpec(spec string) ([]Key, error) {
	tokens := strings.Fields(spec)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("keymap: empty key specification")
	}
	keys := make([]Key, 0, len(tokens))
	for _, tok := range tokens {
		k, err := parseKey(tok)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// parseKey parses a single whitespace-free keystroke token.
func parseKey(tok string) (Key, error) {
	var k Key
	orig := tok

	// Consume leading modifier prefixes. A trailing "-" that is not followed by
	// another character is the base key itself, which is how "C--" works.
	for len(tok) >= 2 && tok[1] == '-' {
		var seen *bool
		switch tok[0] {
		case 'C':
			seen = &k.Ctrl
		case 'M':
			seen = &k.Meta
		case 'S':
			seen = &k.Shift
		}
		if seen == nil {
			break // not a modifier; the rest is the base key
		}
		if *seen {
			return Key{}, fmt.Errorf("keymap: duplicate %c- modifier in key %q", tok[0], orig)
		}
		*seen = true
		tok = tok[2:]
	}

	if tok == "" {
		return Key{}, fmt.Errorf("keymap: key %q has modifiers but no base key", orig)
	}

	// Angle-bracketed special key, e.g. <f5> or <up>.
	if strings.HasPrefix(tok, "<") {
		if !strings.HasSuffix(tok, ">") || len(tok) <= 2 {
			// A lone "<" is the less-than character; anything longer is a typo.
			if len(tok) == 1 {
				k.Rune = '<'
				return finishRune(k, orig)
			}
			return Key{}, fmt.Errorf("keymap: unterminated special key name %q", tok)
		}
		sk, ok := specialByName[strings.ToLower(tok)]
		if !ok {
			return Key{}, fmt.Errorf("keymap: unknown special key %q", tok)
		}
		k.Special = sk
		return k, nil
	}

	// Unbracketed names: SPC, RET, TAB, ESC, DEL.
	if r, ok := bareRuneNames[tok]; ok {
		k.Rune = r
		return finishRune(k, orig)
	}
	if sk, ok := bareSpecialNames[tok]; ok {
		k.Special = sk
		return k, nil
	}

	runes := []rune(tok)
	if len(runes) == 1 {
		k.Rune = runes[0]
		return finishRune(k, orig)
	}
	return Key{}, fmt.Errorf("keymap: unrecognized key %q", tok)
}

// finishRune validates a character-bearing key. Shift is rejected here because a
// character already encodes its own case: "S-a" is ambiguous where "A" is not.
func finishRune(k Key, orig string) (Key, error) {
	if k.Shift {
		return Key{}, fmt.Errorf("keymap: S- modifier in %q is only meaningful with special keys; write the shifted character itself", orig)
	}
	return k, nil
}
