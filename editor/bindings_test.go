package editor

import (
	"testing"

	"github.com/hajianpour/nem/keymap"
)

// Every default binding must parse and install cleanly. This is the test that
// catches a typo in a key spec or a prefix conflict between two bindings, both
// of which would otherwise only surface at startup in front of a user.
func TestInstallDefaultBindings(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("InstallDefaultBindings: %v", err)
	}
}

// Each spec must parse on its own, so a failure names the offending entry
// rather than only the first one to break the whole install.
func TestEveryDefaultSpecParses(t *testing.T) {
	for _, b := range defaultBindings {
		if _, err := keymap.ParseSpec(b.Spec); err != nil {
			t.Errorf("ParseSpec(%q) for %s: %v", b.Spec, b.Command, err)
		}
	}
}

// Bindings are stored normalized, so the keys the decoder produces must find
// them. C-SPC arrives as NUL and C-/ arrives as C-_; if either were bound
// unnormalized, set-mark and undo would be silently unreachable.
func TestNormalizedKeysResolveToCommands(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, tc := range []struct{ spec, want string }{
		{"C-SPC", "set-mark-command"},
		{"C-/", "undo"},
		{"C-_", "undo"},
		{"C-f", "forward-char"},
		{"RET", "newline"},
		{"TAB", "indent-for-tab-command"},
		{"M-<", "beginning-of-buffer"},
		{"M->", "end-of-buffer"},
		{"M-%", "query-replace"},
	} {
		seq, err := keymap.ParseSpec(tc.spec)
		if err != nil {
			t.Errorf("ParseSpec(%q): %v", tc.spec, err)
			continue
		}
		for i := range seq {
			seq[i] = keymap.Normalize(seq[i])
		}
		got := m.Lookup(seq)
		if got.Kind != keymap.Found || got.Command != tc.want {
			t.Errorf("%s: got kind=%v command=%q, want Found %q", tc.spec, got.Kind, got.Command, tc.want)
		}
	}
}

// A prefix must report Pending so the event loop knows to wait for another key
// rather than reporting the sequence undefined.
func TestPrefixesReportPending(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, spec := range []string{"C-x", "M-g", "<f1>"} {
		seq, err := keymap.ParseSpec(spec)
		if err != nil {
			t.Fatalf("ParseSpec(%q): %v", spec, err)
		}
		for i := range seq {
			seq[i] = keymap.Normalize(seq[i])
		}
		if got := m.Lookup(seq); got.Kind != keymap.Pending {
			t.Errorf("%s: got %v, want Pending", spec, got.Kind)
		}
	}
}

// Commands with more than one binding must be discoverable from all of them,
// which is what describe-key and the help buffer rely on.
func TestWhereFindsAllBindings(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := m.Where("undo"); len(got) < 2 {
		t.Errorf("Where(undo) = %v, want at least two bindings", got)
	}
	if got := m.Where("forward-char"); len(got) != 2 {
		t.Errorf("Where(forward-char) = %v, want C-f and <right>", got)
	}
}
