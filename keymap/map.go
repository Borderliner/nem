package keymap

import (
	"fmt"
	"sort"
	"strings"
)

// ResultKind says what a lookup found.
type ResultKind int

const (
	// Undefined means the sequence matches no binding and cannot become one.
	Undefined ResultKind = iota
	// Pending means the sequence is a proper prefix of at least one binding, so
	// the caller should read another key. This is how C-x waits.
	Pending
	// Found means the sequence resolves to a command.
	Found
)

func (r ResultKind) String() string {
	switch r {
	case Undefined:
		return "Undefined"
	case Pending:
		return "Pending"
	case Found:
		return "Found"
	}
	return fmt.Sprintf("ResultKind(%d)", int(r))
}

// Result is the outcome of a [Map.Lookup].
type Result struct {
	Kind    ResultKind
	Command string // set only when Kind is Found
}

// node is one level of the prefix tree. A node is either terminal (command set,
// no children) or internal (children, no command); Bind enforces that, which is
// what makes an ambiguous binding an error rather than a silent shadowing.
type node struct {
	command  string
	children map[Key]*node
}

func (n *node) terminal() bool { return n.command != "" }

// Map resolves key sequences to command names.
//
// The zero value is not usable; call [New].
type Map struct {
	// TreatCtrlHAsBackspace folds C-h into <backspace>. Off by default, because
	// in emacs C-h is the help prefix; turn it on for terminals whose erase
	// character is ^H. See the comment in Map.normalize.
	TreatCtrlHAsBackspace bool

	root *node
}

// New returns an empty Map.
func New() *Map {
	return &Map{root: &node{children: map[Key]*node{}}}
}

// normalize applies [Normalize] plus this map's opt-in folds.
func (m *Map) normalize(k Key) Key {
	k = Normalize(k)
	// Ctrl+h -> 0x08 (BS). Whether that means "backspace" depends on the
	// terminal's erase character: most modern terminals send 0x7F (DEL) for the
	// Backspace key and reserve 0x08 for Ctrl+h, which is why emacs can keep
	// C-h as its help prefix. Terminals configured with `stty erase ^H`, and
	// some older hardware, send 0x08 for Backspace and make the two keys
	// genuinely inseparable — that is who this flag is for.
	if m.TreatCtrlHAsBackspace && k.Special == SpecialNone && k.Ctrl && k.Rune == 'h' {
		return Key{Special: KeyBackspace, Meta: k.Meta}
	}
	return k
}

func (m *Map) normalizeSeq(seq []Key) []Key {
	out := make([]Key, len(seq))
	for i, k := range seq {
		out[i] = m.normalize(k)
	}
	return out
}

// SpecString renders a key sequence in canonical notation, the form used by
// config files and describe-bindings.
func SpecString(seq []Key) string {
	parts := make([]string, len(seq))
	for i, k := range seq {
		parts[i] = k.String()
	}
	return strings.Join(parts, " ")
}

// Bind binds a key sequence to a command name, creating intermediate prefix
// maps as needed. Rebinding an existing sequence replaces it silently.
//
// It fails if the sequence would shadow, or be shadowed by, an existing
// binding: binding C-x C-f when C-x is already a command, or binding C-x when
// C-x C-f exists. The error names both bindings.
func (m *Map) Bind(seq []Key, command string) error {
	if len(seq) == 0 {
		return fmt.Errorf("keymap: cannot bind an empty key sequence to %q", command)
	}
	if command == "" {
		return fmt.Errorf("keymap: cannot bind %s to an empty command name", SpecString(seq))
	}
	seq = m.normalizeSeq(seq)

	cur := m.root
	for i, k := range seq {
		next, ok := cur.children[k]
		if !ok {
			next = &node{children: map[Key]*node{}}
			cur.children[k] = next
		}
		if next.terminal() && i < len(seq)-1 {
			return fmt.Errorf("keymap: cannot bind %q: its prefix %q is already bound to %q",
				SpecString(seq), SpecString(seq[:i+1]), next.command)
		}
		cur = next
	}
	if len(cur.children) > 0 {
		example, cmd := cur.anyDescendant(seq)
		return fmt.Errorf("keymap: cannot bind %q: it is a prefix of the existing binding %q (bound to %q)",
			SpecString(seq), example, cmd)
	}
	cur.command = command
	return nil
}

// anyDescendant returns the spec and command of some binding beneath n, for use
// in a conflict message. Deterministic order is not needed; any witness will do,
// but sorting keeps the message stable across runs.
func (n *node) anyDescendant(prefix []Key) (string, string) {
	if n.terminal() {
		return SpecString(prefix), n.command
	}
	keys := make([]Key, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sortKeys(keys)
	for _, k := range keys {
		next := append(append([]Key(nil), prefix...), k)
		if spec, cmd := n.children[k].anyDescendant(next); cmd != "" {
			return spec, cmd
		}
	}
	return SpecString(prefix), ""
}

// Unbind removes the binding for a sequence and prunes any prefix nodes left
// empty, so a former prefix stops reporting Pending.
func (m *Map) Unbind(seq []Key) error {
	if len(seq) == 0 {
		return fmt.Errorf("keymap: cannot unbind an empty key sequence")
	}
	seq = m.normalizeSeq(seq)
	if !unbind(m.root, seq) {
		return fmt.Errorf("keymap: %q is not bound", SpecString(seq))
	}
	return nil
}

// unbind removes seq beneath n, reporting whether it was bound. It prunes
// children that end up holding neither a command nor descendants.
func unbind(n *node, seq []Key) bool {
	child, ok := n.children[seq[0]]
	if !ok {
		return false
	}
	if len(seq) == 1 {
		if !child.terminal() {
			return false
		}
		delete(n.children, seq[0])
		return true
	}
	if !unbind(child, seq[1:]) {
		return false
	}
	if !child.terminal() && len(child.children) == 0 {
		delete(n.children, seq[0])
	}
	return true
}

// Lookup resolves a key sequence.
func (m *Map) Lookup(seq []Key) Result {
	cur := m.root
	for _, k := range seq {
		next, ok := cur.children[m.normalize(k)]
		if !ok {
			return Result{Kind: Undefined}
		}
		cur = next
	}
	if cur.terminal() {
		return Result{Kind: Found, Command: cur.command}
	}
	if len(cur.children) > 0 {
		return Result{Kind: Pending}
	}
	return Result{Kind: Undefined}
}

// Bindings returns every binding as canonical spec string to command name,
// for describe-bindings and for M-x to show where a command lives.
func (m *Map) Bindings() map[string]string {
	out := map[string]string{}
	collect(m.root, nil, out)
	return out
}

func collect(n *node, prefix []Key, out map[string]string) {
	if n.terminal() {
		out[SpecString(prefix)] = n.command
		return
	}
	for k, child := range n.children {
		collect(child, append(append([]Key(nil), prefix...), k), out)
	}
}

// Where returns every key sequence bound to command, in canonical notation,
// sorted for determinism. It returns nil when the command has no bindings.
//
// A command legitimately has several bindings — undo answers to both C-_ and
// C-x u — so this is the reverse index of [Map.Bindings], which callers would
// otherwise have to invert themselves on every lookup.
func (m *Map) Where(command string) []string {
	if command == "" {
		return nil
	}
	var out []string
	for spec, cmd := range m.Bindings() {
		if cmd == command {
			out = append(out, spec)
		}
	}
	// Bindings walks a map, so iteration order is randomized; sort or callers
	// see a different order on every run.
	sort.Strings(out)
	return out
}

// sortKeys orders keys deterministically by their canonical notation, so a
// conflict message names the same witness binding on every run.
func sortKeys(keys []Key) {
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
}
