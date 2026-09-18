package keymap

import (
	"reflect"
	"testing"
)

// contSpecs renders continuations as canonical specs, which is how a failing
// test reads most clearly: the order and identity of keys is the thing under
// test, and Key.String() is exactly what a which-key panel would show.
func contSpecs(cs []Continuation) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Key.String()
	}
	return out
}

func TestContinuationsEmptyMap(t *testing.T) {
	if got := New().Continuations(nil); got != nil {
		t.Errorf("Continuations(nil) on an empty map = %v, want nil", got)
	}
}

// An empty sequence lists the global map's top-level keys, which is what makes
// the whole keymap browsable.
func TestContinuationsEmptySeqListsTopLevel(t *testing.T) {
	m := New()
	mustBind(t, m, "C-f", "forward-char")
	mustBind(t, m, "b", "self-insert")
	mustBind(t, m, "C-x C-s", "save-buffer")

	got := m.Continuations(nil)
	if want := []string{"b", "C-f", "C-x"}; !reflect.DeepEqual(contSpecs(got), want) {
		t.Fatalf("specs = %v, want %v", contSpecs(got), want)
	}

	byKey := map[string]Continuation{}
	for _, c := range got {
		byKey[c.Key.String()] = c
	}
	if c := byKey["C-f"]; c.IsPrefix || c.Command != "forward-char" || c.Count != 1 {
		t.Errorf("C-f = %+v, want a leaf bound to forward-char with Count 1", c)
	}
	if c := byKey["C-x"]; !c.IsPrefix || c.Command != "" || c.Count != 1 {
		t.Errorf("C-x = %+v, want a prefix with no command and Count 1", c)
	}
}

func TestContinuationsTwoLevelPrefix(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	mustBind(t, m, "C-x C-s", "save-buffer")
	mustBind(t, m, "C-x b", "switch-to-buffer")

	got := m.Continuations(mustParse(t, "C-x"))
	if want := []string{"b", "C-f", "C-s"}; !reflect.DeepEqual(contSpecs(got), want) {
		t.Fatalf("specs = %v, want %v", contSpecs(got), want)
	}
	for _, c := range got {
		if c.IsPrefix {
			t.Errorf("%s reported as a prefix, want a leaf", c.Key)
		}
		if c.Count != 1 {
			t.Errorf("%s Count = %d, want 1", c.Key, c.Count)
		}
	}
}

func TestContinuationsThreeLevelPrefix(t *testing.T) {
	m := New()
	mustBind(t, m, "M-g M-g", "goto-line")
	mustBind(t, m, "M-g c", "goto-char")

	got := m.Continuations(mustParse(t, "M-g"))
	if want := []string{"c", "M-g"}; !reflect.DeepEqual(contSpecs(got), want) {
		t.Errorf("specs = %v, want %v", contSpecs(got), want)
	}
}

// Walking to a complete binding yields nothing: a terminal node has no children,
// so there is nothing that can follow it. Not an error.
func TestContinuationsCompleteBindingReturnsNil(t *testing.T) {
	m := New()
	mustBind(t, m, "C-f", "forward-char")
	if got := m.Continuations(mustParse(t, "C-f")); got != nil {
		t.Errorf("Continuations(C-f) = %v, want nil for a complete binding", got)
	}
}

func TestContinuationsUndefinedReturnsNil(t *testing.T) {
	m := New()
	mustBind(t, m, "C-f", "forward-char")
	for _, spec := range []string{"C-q", "C-q C-q", "C-f C-f"} {
		if got := m.Continuations(mustParse(t, spec)); got != nil {
			t.Errorf("Continuations(%s) = %v, want nil", spec, got)
		}
	}
}

// Count is the total number of bindings beneath a prefix, however deep. A
// which-key panel showing "+register 3" is claiming three reachable commands,
// not three immediate children.
func TestContinuationsCountIsRecursive(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x r k", "kill-rectangle")
	mustBind(t, m, "C-x r y", "yank-rectangle")
	mustBind(t, m, "C-x r t", "string-rectangle")
	mustBind(t, m, "C-x C-f", "find-file")

	top := m.Continuations(nil)
	if len(top) != 1 {
		t.Fatalf("top level = %v, want just C-x", contSpecs(top))
	}
	if got := top[0].Count; got != 4 {
		t.Errorf("C-x Count = %d, want 4 (three rectangle commands plus find-file)", got)
	}

	under := m.Continuations(mustParse(t, "C-x"))
	byKey := map[string]Continuation{}
	for _, c := range under {
		byKey[c.Key.String()] = c
	}
	if c := byKey["r"]; !c.IsPrefix || c.Count != 3 {
		t.Errorf("C-x r = %+v, want a prefix with Count 3", c)
	}
	if c := byKey["C-f"]; c.IsPrefix || c.Count != 1 {
		t.Errorf("C-x C-f = %+v, want a leaf with Count 1", c)
	}
}

// The sequence is normalized exactly as Lookup normalizes it, so a caller can
// hand over raw decoded keys. C-X folds to C-x and NUL folds to C-@; if either
// were missed, a prefix panel would come up empty on a real keystroke.
func TestContinuationsNormalizesTheSequence(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	mustBind(t, m, "C-SPC C-a", "mark-then-home")

	for _, tc := range []struct {
		name string
		seq  []Key
	}{
		{"canonical C-x", []Key{{Rune: 'x', Ctrl: true}}},
		{"uppercase C-X folds to C-x", []Key{{Rune: 'X', Ctrl: true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := m.Continuations(tc.seq)
			if want := []string{"C-f"}; !reflect.DeepEqual(contSpecs(got), want) {
				t.Errorf("Continuations(%s) = %v, want %v", dbgSeq(tc.seq), contSpecs(got), want)
			}
		})
	}

	// NUL is how Ctrl+Space arrives from a terminal; it must reach the C-@ node.
	t.Run("NUL folds to C-@", func(t *testing.T) {
		got := m.Continuations([]Key{{Rune: 0, Ctrl: true}})
		if want := []string{"C-a"}; !reflect.DeepEqual(contSpecs(got), want) {
			t.Errorf("Continuations(NUL) = %v, want %v", contSpecs(got), want)
		}
	})
}

// The map's own opt-in fold must apply too, matching Lookup rather than adding a
// second rule.
func TestContinuationsHonoursTreatCtrlHAsBackspace(t *testing.T) {
	m := New()
	m.TreatCtrlHAsBackspace = true
	mustBind(t, m, "<backspace> x", "some-command")

	got := m.Continuations([]Key{{Rune: 'h', Ctrl: true}})
	if want := []string{"x"}; !reflect.DeepEqual(contSpecs(got), want) {
		t.Errorf("Continuations(C-h) with the fold on = %v, want %v", contSpecs(got), want)
	}
}

// Ordering is documented: plain runes, then modified runes, then named keys,
// ascending by canonical notation within each group.
func TestContinuationsOrdering(t *testing.T) {
	m := New()
	for _, b := range []struct{ spec, cmd string }{
		{"<up>", "previous-line"},
		{"M-g", "goto-line"},
		{"b", "switch-to-buffer"},
		{"<f1>", "help"},
		{"C-x", "exchange"},
		{"2", "split-below"},
		{"C-f", "forward-char"},
	} {
		mustBind(t, m, b.spec, b.cmd)
	}

	want := []string{"2", "b", "C-f", "C-x", "M-g", "<f1>", "<up>"}
	if got := contSpecs(m.Continuations(nil)); !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// Go randomizes map iteration, so an unsorted implementation flakes rather than
// fails. Building the same map repeatedly and demanding identical output is what
// turns that into a hard failure.
func TestContinuationsAreDeterministic(t *testing.T) {
	build := func() *Map {
		m := New()
		for _, b := range []struct{ spec, cmd string }{
			{"C-x C-f", "find-file"}, {"C-x C-s", "save-buffer"},
			{"C-x b", "switch-to-buffer"}, {"C-x k", "kill-buffer"},
			{"C-x 0", "delete-window"}, {"C-x 2", "split-below"},
			{"C-x o", "other-window"}, {"C-x r k", "kill-rectangle"},
			{"C-x <up>", "up-thing"}, {"C-x M-y", "meta-yank"},
		} {
			mustBind(t, m, b.spec, b.cmd)
		}
		return m
	}

	first := contSpecs(build().Continuations(mustParse(t, "C-x")))
	if want := []string{"0", "2", "b", "k", "o", "r", "C-f", "C-s", "M-y", "<up>"}; !reflect.DeepEqual(first, want) {
		t.Fatalf("specs = %v, want %v", first, want)
	}
	for i := 0; i < 50; i++ {
		if got := contSpecs(build().Continuations(mustParse(t, "C-x"))); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d order = %v, want %v (unstable ordering)", i, got, first)
		}
	}
}

// Bind refuses to let a key be both a command and a prefix, in either direction.
// Continuations therefore never has to represent that state, and this test
// records why rather than leaving the absence to be rediscovered.
func TestBindRefusesKeyAsBothCommandAndPrefix(t *testing.T) {
	t.Run("prefix already bound to a command", func(t *testing.T) {
		m := New()
		mustBind(t, m, "C-x", "some-command")
		if err := m.Bind(mustParse(t, "C-x C-f"), "find-file"); err == nil {
			t.Error("binding C-x C-f over a bound C-x succeeded, want a conflict error")
		}
	})
	t.Run("sequence is a prefix of an existing binding", func(t *testing.T) {
		m := New()
		mustBind(t, m, "C-x C-f", "find-file")
		if err := m.Bind(mustParse(t, "C-x"), "some-command"); err == nil {
			t.Error("binding C-x under an existing C-x C-f succeeded, want a conflict error")
		}
	})
}

// Unbind prunes empty prefix nodes, so a prefix that loses its last binding must
// stop appearing as a continuation.
func TestContinuationsReflectUnbind(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	mustBind(t, m, "C-x C-s", "save-buffer")

	if err := m.Unbind(mustParse(t, "C-x C-f")); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if got, want := contSpecs(m.Continuations(mustParse(t, "C-x"))), []string{"C-s"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after unbind = %v, want %v", got, want)
	}

	if err := m.Unbind(mustParse(t, "C-x C-s")); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if got := m.Continuations(nil); got != nil {
		t.Errorf("top level = %v, want nil once C-x is empty and pruned", contSpecs(got))
	}
}
