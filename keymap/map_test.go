package keymap

import (
	"slices"
	"strings"
	"testing"
)

func mustBind(t *testing.T, m *Map, spec, command string) {
	t.Helper()
	if err := m.Bind(mustParse(t, spec), command); err != nil {
		t.Fatalf("Bind(%q, %q) returned error: %v", spec, command, err)
	}
}

func lookup(t *testing.T, m *Map, spec string) Result {
	t.Helper()
	return m.Lookup(mustParse(t, spec))
}

func TestEmptyMapLooksUpUndefined(t *testing.T) {
	m := New()
	if got := lookup(t, m, "C-x"); got.Kind != Undefined {
		t.Errorf("Lookup on empty map = %v, want Undefined", got.Kind)
	}
}

func TestBindThenLookupExactSequence(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	got := lookup(t, m, "C-x C-f")
	if got.Kind != Found {
		t.Fatalf("Lookup(C-x C-f).Kind = %v, want Found", got.Kind)
	}
	if got.Command != "find-file" {
		t.Errorf("Lookup(C-x C-f).Command = %q, want %q", got.Command, "find-file")
	}
}

func TestLookupPrefixIsPending(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	got := lookup(t, m, "C-x")
	if got.Kind != Pending {
		t.Errorf("Lookup(C-x) = %v, want Pending (C-x is a prefix)", got.Kind)
	}
	if got.Command != "" {
		t.Errorf("Pending result carried Command %q, want empty", got.Command)
	}
}

func TestLookupUnboundSuffixIsUndefined(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	if got := lookup(t, m, "C-x C-q"); got.Kind != Undefined {
		t.Errorf("Lookup(C-x C-q) = %v, want Undefined", got.Kind)
	}
}

func TestLookupPastATerminalBindingIsUndefined(t *testing.T) {
	m := New()
	mustBind(t, m, "C-a", "move-beginning-of-line")
	if got := lookup(t, m, "C-a C-b"); got.Kind != Undefined {
		t.Errorf("Lookup(C-a C-b) = %v, want Undefined", got.Kind)
	}
}

func TestBindOverPrefixReportsBothBindings(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x", "some-command")
	err := m.Bind(mustParse(t, "C-x C-f"), "find-file")
	if err == nil {
		t.Fatal("Bind(C-x C-f) over terminal C-x = nil error, want a conflict error")
	}
	msg := err.Error()
	for _, want := range []string{"C-x C-f", "C-x", "some-command"} {
		if !strings.Contains(msg, want) {
			t.Errorf("conflict error %q does not mention %q", msg, want)
		}
	}
}

func TestBindOverExistingPrefixReportsBothBindings(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	err := m.Bind(mustParse(t, "C-x"), "some-command")
	if err == nil {
		t.Fatal("Bind(C-x) where C-x C-f exists = nil error, want a conflict error")
	}
	msg := err.Error()
	for _, want := range []string{"C-x", "C-x C-f", "find-file"} {
		if !strings.Contains(msg, want) {
			t.Errorf("conflict error %q does not mention %q", msg, want)
		}
	}
}

func TestRebindingExactSequenceReplacesSilently(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-s", "save-buffer")
	mustBind(t, m, "C-x C-s", "save-some-buffers") // the Lua config needs this
	got := lookup(t, m, "C-x C-s")
	if got.Command != "save-some-buffers" {
		t.Errorf("after rebind, Command = %q, want %q", got.Command, "save-some-buffers")
	}
}

func TestBindEmptySequenceIsAnError(t *testing.T) {
	m := New()
	if err := m.Bind(nil, "nope"); err == nil {
		t.Error("Bind(nil) = nil error, want an error")
	}
}

func TestUnbindRemovesBinding(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	if err := m.Unbind(mustParse(t, "C-x C-f")); err != nil {
		t.Fatalf("Unbind returned error: %v", err)
	}
	if got := lookup(t, m, "C-x C-f"); got.Kind != Undefined {
		t.Errorf("after Unbind, Lookup = %v, want Undefined", got.Kind)
	}
}

func TestUnbindPrunesEmptyPrefixes(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	if err := m.Unbind(mustParse(t, "C-x C-f")); err != nil {
		t.Fatalf("Unbind returned error: %v", err)
	}
	// C-x has no surviving children, so it must stop being a pending prefix.
	if got := lookup(t, m, "C-x"); got.Kind != Undefined {
		t.Errorf("after pruning, Lookup(C-x) = %v, want Undefined", got.Kind)
	}
}

func TestUnbindKeepsSiblingBindings(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	mustBind(t, m, "C-x C-s", "save-buffer")
	if err := m.Unbind(mustParse(t, "C-x C-f")); err != nil {
		t.Fatalf("Unbind returned error: %v", err)
	}
	if got := lookup(t, m, "C-x C-s"); got.Kind != Found {
		t.Errorf("sibling binding lost: Lookup(C-x C-s) = %v, want Found", got.Kind)
	}
	if got := lookup(t, m, "C-x"); got.Kind != Pending {
		t.Errorf("Lookup(C-x) = %v, want Pending (C-x C-s survives)", got.Kind)
	}
}

func TestUnbindUnboundSequenceIsAnError(t *testing.T) {
	m := New()
	if err := m.Unbind(mustParse(t, "C-x C-f")); err == nil {
		t.Error("Unbind of an unbound sequence = nil error, want an error")
	}
}

func TestBindingsReturnsCanonicalSpecs(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	mustBind(t, m, "M-x", "execute-extended-command")
	mustBind(t, m, "<f5>", "revert-buffer")
	got := m.Bindings()
	want := map[string]string{
		"C-x C-f": "find-file",
		"M-x":     "execute-extended-command",
		"<f5>":    "revert-buffer",
	}
	if len(got) != len(want) {
		t.Fatalf("Bindings() has %d entries, want %d: %v", len(got), len(want), got)
	}
	for spec, cmd := range want {
		if got[spec] != cmd {
			t.Errorf("Bindings()[%q] = %q, want %q", spec, got[spec], cmd)
		}
	}
}

func TestMapNormalizesOnBindAndLookup(t *testing.T) {
	// C-i and <tab> are the same byte, so binding one must match the other.
	m := New()
	mustBind(t, m, "C-i", "indent-for-tab-command")
	if got := lookup(t, m, "<tab>"); got.Kind != Found || got.Command != "indent-for-tab-command" {
		t.Errorf("Lookup(<tab>) after Bind(C-i) = %v/%q, want Found/indent-for-tab-command", got.Kind, got.Command)
	}
	// And C-SPC must reach a C-@ binding.
	mustBind(t, m, "C-@", "set-mark-command")
	if got := lookup(t, m, "C-SPC"); got.Kind != Found || got.Command != "set-mark-command" {
		t.Errorf("Lookup(C-SPC) after Bind(C-@) = %v/%q, want Found/set-mark-command", got.Kind, got.Command)
	}
}

func TestCtrlHIsNotBackspaceByDefault(t *testing.T) {
	m := New()
	mustBind(t, m, "<backspace>", "delete-backward-char")
	mustBind(t, m, "C-h", "help-command")
	if got := lookup(t, m, "C-h"); got.Command != "help-command" {
		t.Errorf("Lookup(C-h) = %q, want help-command", got.Command)
	}
}

func TestTreatCtrlHAsBackspaceFoldsCtrlH(t *testing.T) {
	m := New()
	m.TreatCtrlHAsBackspace = true
	mustBind(t, m, "<backspace>", "delete-backward-char")
	if got := lookup(t, m, "C-h"); got.Kind != Found || got.Command != "delete-backward-char" {
		t.Errorf("with TreatCtrlHAsBackspace, Lookup(C-h) = %v/%q, want Found/delete-backward-char", got.Kind, got.Command)
	}
}

func TestFailedBindLeavesMapUnchanged(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	before := m.Bindings()

	if err := m.Bind(mustParse(t, "C-x"), "clobber"); err == nil {
		t.Fatal("expected a conflict error")
	}
	if err := m.Bind(mustParse(t, "C-x C-f C-g"), "clobber"); err == nil {
		t.Fatal("expected a conflict error")
	}

	after := m.Bindings()
	if len(after) != len(before) {
		t.Errorf("failed Bind changed the map: %v -> %v", before, after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("failed Bind altered %q: %q -> %q", k, v, after[k])
		}
	}
	// A failed bind must not leave a dangling prefix behind.
	if got := lookup(t, m, "C-x C-f C-g"); got.Kind != Undefined {
		t.Errorf("Lookup(C-x C-f C-g) = %v, want Undefined", got.Kind)
	}
}

func TestDeepPrefixChainsResolve(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x 4 f", "find-file-other-window")
	mustBind(t, m, "C-x 4 b", "switch-to-buffer-other-window")
	if got := lookup(t, m, "C-x"); got.Kind != Pending {
		t.Errorf("Lookup(C-x) = %v, want Pending", got.Kind)
	}
	if got := lookup(t, m, "C-x 4"); got.Kind != Pending {
		t.Errorf("Lookup(C-x 4) = %v, want Pending", got.Kind)
	}
	if got := lookup(t, m, "C-x 4 f"); got.Command != "find-file-other-window" {
		t.Errorf("Lookup(C-x 4 f) = %q, want find-file-other-window", got.Command)
	}
}

func TestWhereFindsEveryBindingForACommand(t *testing.T) {
	m := New()
	// undo legitimately has two bindings, which is the whole reason Where exists.
	mustBind(t, m, "C-_", "undo")
	mustBind(t, m, "C-x u", "undo")
	mustBind(t, m, "C-y", "yank")

	got := m.Where("undo")
	want := []string{"C-_", "C-x u"}
	if len(got) != len(want) {
		t.Fatalf("Where(\"undo\") = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Where(\"undo\")[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWhereFindsSingleBinding(t *testing.T) {
	m := New()
	mustBind(t, m, "C-x C-f", "find-file")
	got := m.Where("find-file")
	if len(got) != 1 || got[0] != "C-x C-f" {
		t.Errorf("Where(\"find-file\") = %v, want [\"C-x C-f\"]", got)
	}
}

func TestWhereReturnsNilForUnknownCommand(t *testing.T) {
	m := New()
	mustBind(t, m, "C-y", "yank")
	if got := m.Where("no-such-command"); got != nil {
		t.Errorf("Where(\"no-such-command\") = %v, want nil", got)
	}
}

func TestWhereOnEmptyMapReturnsNil(t *testing.T) {
	if got := New().Where("undo"); got != nil {
		t.Errorf("Where on empty map = %v, want nil", got)
	}
}

func TestWhereIsSortedDeterministically(t *testing.T) {
	// Map iteration order is randomized, so an unsorted Where would flake.
	specs := []string{"C-x u", "C-_", "M-u", "C-c C-u", "<f7>"}
	for range 50 {
		m := New()
		for _, s := range specs {
			mustBind(t, m, s, "undo")
		}
		got := m.Where("undo")
		if !slices.IsSorted(got) {
			t.Fatalf("Where returned unsorted result: %v", got)
		}
		if len(got) != len(specs) {
			t.Fatalf("Where returned %d bindings, want %d: %v", len(got), len(specs), got)
		}
	}
}

func TestWhereReflectsUnbind(t *testing.T) {
	m := New()
	mustBind(t, m, "C-_", "undo")
	mustBind(t, m, "C-x u", "undo")
	if err := m.Unbind(mustParse(t, "C-_")); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	got := m.Where("undo")
	if len(got) != 1 || got[0] != "C-x u" {
		t.Errorf("after Unbind, Where(\"undo\") = %v, want [\"C-x u\"]", got)
	}
}
