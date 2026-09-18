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

// The keys the decoder actually produces must resolve, looked up RAW with no
// Normalize call here. keymap.Map owns that rule on both Bind and Lookup, so
// testing it this way catches a regression in keymap at the layer a user
// notices: C-SPC arrives as NUL and C-/ arrives as C-_, and if either stopped
// resolving, set-mark and undo would be silently dead keys.
func TestRawDecodedKeysResolveToCommands(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, tc := range []struct {
		name string
		key  keymap.Key
		want string
	}{
		{"NUL is C-SPC", keymap.Key{Rune: 0, Ctrl: true}, "set-mark-command"},
		{"C-/ arrives as C-_", keymap.Key{Rune: '_', Ctrl: true}, "undo"},
		{"C-f", keymap.Key{Rune: 'f', Ctrl: true}, "forward-char"},
		{"RET", keymap.Key{Special: keymap.KeyEnter}, "newline"},
		{"TAB", keymap.Key{Special: keymap.KeyTab}, "indent-for-tab-command"},
		{"M-<", keymap.Key{Rune: '<', Meta: true}, "beginning-of-buffer"},
		{"M-%", keymap.Key{Rune: '%', Meta: true}, "query-replace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := m.Lookup([]keymap.Key{tc.key})
			if got.Kind != keymap.Found || got.Command != tc.want {
				t.Errorf("got kind=%v command=%q, want Found %q", got.Kind, got.Command, tc.want)
			}
		})
	}
}

// A prefix must report Pending so the event loop waits for another key rather
// than reporting the sequence undefined. Looked up raw, for the same reason.
func TestPrefixesReportPending(t *testing.T) {
	m := keymap.New()
	if err := InstallDefaultBindings(m); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, tc := range []struct {
		name string
		key  keymap.Key
	}{
		{"C-x", keymap.Key{Rune: 'x', Ctrl: true}},
		{"M-g", keymap.Key{Rune: 'g', Meta: true}},
		{"<f1>", keymap.Key{Special: keymap.KeyF1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.Lookup([]keymap.Key{tc.key}); got.Kind != keymap.Pending {
				t.Errorf("got %v, want Pending", got.Kind)
			}
		})
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
