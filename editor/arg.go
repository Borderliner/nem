package editor

import "github.com/hajianpour/nem/keymap"

// argState collects a universal argument between C-u and the command it
// modifies.
//
// Emacs accepts the same argument four ways and nem accepts all of them:
// repeated C-u multiplies by four (C-u C-u is 16), digits after C-u give the
// number outright (C-u 1 2 is 12), Meta'd digits do the same without C-u at all
// (M-1 M-2 is 12), and a minus makes it negative (C-u - is -1).
//
// It is consumed by the dispatch that follows, so a command always sees either
// the argument the user typed or the default of 1.
type argState struct {
	active bool // an argument is being collected
	digits bool // explicit digits have been typed
	neg    bool // a minus was typed
	n      int  // the accumulated value, ignoring sign
}

// value reports the argument for the command about to run. explicit is false
// when the user typed no argument, which some commands distinguish from C-u 1.
func (a argState) value() (int, bool) {
	if !a.active {
		return 1, false
	}
	n := a.n
	if a.neg && !a.digits {
		// A bare C-u - means -1, not -0. Emacs treats the minus alone as a
		// count of one in the negative direction.
		n = 1
	}
	if a.neg {
		n = -n
	}
	return n, true
}

// reset clears the argument, which dispatch does after every command.
func (a *argState) reset() { *a = argState{} }

// consumeUniversal handles C-u. Repeated presses multiply by four, but only
// before digits are typed: after C-u 1 2, a further C-u starts a fresh
// argument rather than multiplying 12.
func (a *argState) consumeUniversal() {
	switch {
	case !a.active:
		*a = argState{active: true, n: 4}
	case a.digits || a.neg:
		*a = argState{active: true, n: 4}
	default:
		a.n *= 4
	}
}

// consumeDigit accumulates a decimal digit.
func (a *argState) consumeDigit(d int) {
	if !a.active || !a.digits {
		a.active, a.digits, a.n = true, true, d
		return
	}
	a.n = a.n*10 + d
}

// consumeMinus records a negative argument. A minus after digits is not a sign
// but an ordinary character, so the caller checks digits first.
func (a *argState) consumeMinus() {
	if !a.active {
		*a = argState{active: true}
	}
	a.neg = true
}

// argKey classifies a key as part of an argument being typed.
//
// It returns handled true when the key was absorbed into the argument and must
// not be looked up in the keymap. The universal argument is resolved before
// keymap lookup precisely so that C-u 4 C-f reaches forward-char with an
// argument rather than looking up a four-key sequence.
func (e *Editor) argKey(k keymap.Key) bool {
	// C-u always starts or extends an argument.
	if k.Ctrl && !k.Meta && k.Rune == 'u' {
		e.arg.consumeUniversal()
		e.echoArg()
		return true
	}

	// Meta'd digits and minus start one without C-u: M-1 M-2 is 12.
	if k.Meta && !k.Ctrl {
		if d, ok := digit(k.Rune); ok {
			e.arg.consumeDigit(d)
			e.echoArg()
			return true
		}
		if k.Rune == '-' {
			e.arg.consumeMinus()
			e.echoArg()
			return true
		}
	}

	// Plain digits and minus continue an argument already begun, and only
	// then: otherwise typing "4" would never insert a 4.
	if e.arg.active && !k.Ctrl && !k.Meta && k.Special == keymap.SpecialNone {
		if d, ok := digit(k.Rune); ok {
			e.arg.consumeDigit(d)
			e.echoArg()
			return true
		}
		if k.Rune == '-' && !e.arg.digits {
			e.arg.consumeMinus()
			e.echoArg()
			return true
		}
	}
	return false
}

func digit(r rune) (int, bool) {
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	return 0, false
}

// echoArg shows the argument as it is typed, as emacs does, so a long count is
// visible before it takes effect.
func (e *Editor) echoArg() {
	n, _ := e.arg.value()
	e.Echo("C-u %d-", n)
}
