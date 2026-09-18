package keymap

import "sort"

// Continuation is one key that may follow a prefix.
//
// A node in the tree is either terminal or internal — [Map.Bind] refuses to let a
// key be both a command and a prefix — so exactly one of Command and IsPrefix is
// meaningful on any given Continuation.
type Continuation struct {
	// Key is the next keystroke, normalized as the map stores it.
	Key Key
	// Command is the bound command name. Empty when IsPrefix.
	Command string
	// IsPrefix reports that this key leads to a further keymap rather than to a
	// command, so pressing it waits for another key.
	IsPrefix bool
	// Count is how many bindings this key leads to: the recursive total beneath
	// a prefix, and 1 for a leaf. A panel reporting "+register 3" is claiming
	// three reachable commands, so counting only immediate children would lie
	// about any nesting deeper than one level.
	Count int
}

// Continuations lists the keys that may follow seq.
//
// This is what a which-key panel is built from: after a pending prefix, it
// answers "what can I press now, and what will it do".
//
// seq is normalized the same way [Map.Lookup] normalizes it, including this map's
// opt-in folds, so a caller may pass keys straight from the terminal decoder. It
// returns nil when seq resolves to a complete binding (a terminal node has no
// children, so nothing can follow it) and when seq matches nothing at all —
// neither case is an error.
//
// The result is ordered for a human reading a panel: plain runes first, since
// those are the quickest to press and read as menu letters; then runes carrying
// Ctrl or Meta; then named keys such as <f1> and <up>. Within each group, keys are
// sorted ascending by canonical notation. The order is total and stable, which
// matters because Go randomizes map iteration and an unsorted implementation
// would reshuffle a panel between one keystroke and the next.
func (m *Map) Continuations(seq []Key) []Continuation {
	cur := m.root
	for _, k := range seq {
		next, ok := cur.children[m.normalize(k)]
		if !ok {
			return nil
		}
		cur = next
	}

	if len(cur.children) == 0 {
		return nil
	}

	out := make([]Continuation, 0, len(cur.children))
	for k, child := range cur.children {
		c := Continuation{Key: k, Count: child.bindingCount()}
		if child.terminal() {
			c.Command = child.command
		} else {
			c.IsPrefix = true
		}
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool {
		if gi, gj := keyGroup(out[i].Key), keyGroup(out[j].Key); gi != gj {
			return gi < gj
		}
		return out[i].Key.String() < out[j].Key.String()
	})
	return out
}

// bindingCount is the number of commands reachable at or beneath n.
func (n *node) bindingCount() int {
	if n.terminal() {
		return 1
	}
	total := 0
	for _, child := range n.children {
		total += child.bindingCount()
	}
	return total
}

// keyGroup ranks a key for display: plain runes, then modified runes, then named
// keys. Shift is not considered, because [Normalize] clears it on runes and so it
// only ever appears on a named key, which is already in the last group.
func keyGroup(k Key) int {
	switch {
	case k.Special != SpecialNone:
		return 2
	case k.Ctrl || k.Meta:
		return 1
	default:
		return 0
	}
}
