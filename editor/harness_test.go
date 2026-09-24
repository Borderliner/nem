package editor

import (
	"strings"
	"testing"
	"unicode"

	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// The harness drives the editor by emacs key spec, because that is the only
// form in which these behaviours can be stated the way a user experiences them.
// A test that reads press(e, "C-k", "C-k", "C-y") and asserts the buffer is a
// test of the editor; one that calls killLine twice and yank once is a test of
// three functions that happen to be adjacent.

// newTestEditor returns an editor on a simulation screen, its scratch buffer
// seeded with lines and point at the origin.
func newTestEditor(t *testing.T, lines ...string) (*Editor, tcell.SimulationScreen) {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatalf("screen init: %v", err)
	}
	scr.SetSize(80, 24)
	t.Cleanup(scr.Fini)

	e, err := New(scr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// New builds a backup store at the real $XDG_STATE_HOME/nem, so without this
	// any test that saves over an existing file would write into the developer's
	// home directory.
	e.SetBackupRoot(t.TempDir())
	// Likewise the system clipboard: an empty one unless a test installs
	// its own. See TestMain.
	e.clip.read = noSystemClipboard

	if len(lines) > 0 {
		b := e.Buf()
		if err := b.Insert(text.Pos{}, []rune(strings.Join(lines, "\n"))); err != nil {
			t.Fatalf("seeding buffer: %v", err)
		}
		b.BreakUndo()
		b.SetModified(false)
		e.Active().Pt = text.Pos{}
	}
	return e, scr
}

// specKeys parses an emacs key spec into keys, failing the test on a bad spec.
func specKeys(t *testing.T, spec string) []keymap.Key {
	t.Helper()
	seq, err := keymap.ParseSpec(spec)
	if err != nil {
		t.Fatalf("ParseSpec(%q): %v", spec, err)
	}
	for i := range seq {
		seq[i] = keymap.Normalize(seq[i])
	}
	return seq
}

// press feeds key specs straight into HandleKey, synchronously. A spec that
// opens a prompt blocks until the prompt's own keys are found, so those must be
// injected first with queue.
func press(t *testing.T, e *Editor, specs ...string) {
	t.Helper()
	for _, spec := range specs {
		for _, k := range specKeys(t, spec) {
			e.HandleKey(k)
		}
	}
}

// tcellSpecialFor is the reverse of decodekey's tcellSpecials, for injection.
var tcellSpecialFor = map[keymap.SpecialKey]tcell.Key{
	keymap.KeyEnter:     tcell.KeyEnter,
	keymap.KeyTab:       tcell.KeyTab,
	keymap.KeyBackspace: tcell.KeyBackspace,
	keymap.KeyDelete:    tcell.KeyDelete,
	keymap.KeyEscape:    tcell.KeyEscape,
	keymap.KeyUp:        tcell.KeyUp,
	keymap.KeyDown:      tcell.KeyDown,
	keymap.KeyLeft:      tcell.KeyLeft,
	keymap.KeyRight:     tcell.KeyRight,
	keymap.KeyHome:      tcell.KeyHome,
	keymap.KeyEnd:       tcell.KeyEnd,
	keymap.KeyPgUp:      tcell.KeyPgUp,
	keymap.KeyPgDn:      tcell.KeyPgDn,
	keymap.KeyF1:        tcell.KeyF1,
	keymap.KeyF3:        tcell.KeyF3,
	keymap.KeyF4:        tcell.KeyF4,
}

// tcell's simulation screen holds only 10 queued events and InjectKey BLOCKS
// once that fills, so input cannot simply be pushed in before the key that
// opens a prompt. It is fed from one goroutine instead, which keeps the order
// exact while the nested loop drains it. A feeder still blocked at the end of a
// test is released by scr.Fini, registered in newTestEditor's cleanup.

// injected is one terminal event, built ahead of time so the feeder goroutine
// never touches *testing.T.
type injected struct {
	key tcell.Key
	r   rune
	mod tcell.ModMask
}

// txt turns literal text into one keystroke per rune, as typing it would.
func txt(s string) []injected {
	out := make([]injected, 0, len(s))
	for _, r := range s {
		out = append(out, injected{tcell.KeyRune, r, tcell.ModNone})
	}
	return out
}

// key turns emacs key specs into keystrokes.
func key(t *testing.T, specs ...string) []injected {
	t.Helper()
	var out []injected
	for _, spec := range specs {
		for _, k := range specKeys(t, spec) {
			kk, r, mod := injection(t, k)
			out = append(out, injected{kk, r, mod})
		}
	}
	return out
}

// feed delivers groups of keystrokes, in order, from a single goroutine.
func feed(t *testing.T, scr tcell.SimulationScreen, groups ...[]injected) {
	t.Helper()
	var all []injected
	for _, g := range groups {
		all = append(all, g...)
	}
	go func() {
		for _, ev := range all {
			scr.InjectKey(ev.key, ev.r, ev.mod)
		}
	}()
}

// injection converts a keymap.Key back into the tcell event that produces it,
// so that DecodeKey round-trips it.
func injection(t *testing.T, k keymap.Key) (tcell.Key, rune, tcell.ModMask) {
	t.Helper()
	var mod tcell.ModMask
	if k.Meta {
		mod |= tcell.ModAlt
	}
	if k.Special != keymap.SpecialNone {
		tk, ok := tcellSpecialFor[k.Special]
		if !ok {
			t.Fatalf("no tcell key for special %v", k.Special)
		}
		if k.Shift {
			mod |= tcell.ModShift
		}
		return tk, 0, mod
	}
	if k.Ctrl {
		// tcell numbers Ctrl'd keys 64..95, the ASCII codes of '@', 'A'..'Z'
		// and '['..'_' — so the uppercase form of the rune is the constant.
		up := unicode.ToUpper(k.Rune)
		if up < 64 || up > 95 {
			t.Fatalf("cannot inject Ctrl+%q", k.Rune)
		}
		return tcell.Key(up), 0, mod | tcell.ModCtrl
	}
	return tcell.KeyRune, k.Rune, mod
}

// --- assertions ----------------------------------------------------------

func wantText(t *testing.T, e *Editor, want string) {
	t.Helper()
	if got := e.Buf().String(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func wantPt(t *testing.T, e *Editor, line int, col text.RuneIdx) {
	t.Helper()
	if got, w := e.Win().Pt, (text.Pos{Line: line, Col: col}); got != w {
		t.Errorf("point = %v, want %v", got, w)
	}
}

func wantEcho(t *testing.T, e *Editor, substr string) {
	t.Helper()
	if !strings.Contains(e.Message(), substr) {
		t.Errorf("echo = %q, want it to contain %q", e.Message(), substr)
	}
}
