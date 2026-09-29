package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// Only a terminal that reorders right-to-left text whatever it is told is left
// to do it. VTE's terminals and mintty are told not to, and nem lays the text
// out itself, as it does where the terminal never reorders.
func TestTerminalDoesBidi(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		want             bool
	}{
		{"konsole", "KONSOLE_VERSION", "230804", true},
		{"mlterm", "MLTERM", "3.9.3", true},
		{"macOS Terminal", "TERM_PROGRAM", "Apple_Terminal", true},
		{"GNOME Terminal", "VTE_VERSION", "7600", false},
		{"mintty", "TERM_PROGRAM", "mintty", false},
		{"ghostty", "TERM_PROGRAM", "ghostty", false},
		{"nothing said", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"KONSOLE_VERSION", "MLTERM", "TERM_PROGRAM", "VTE_VERSION"} {
				t.Setenv(k, "")
			}
			if tc.key != "" {
				t.Setenv(tc.key, tc.value)
			}
			if got := TerminalDoesBidi(); got != tc.want {
				t.Errorf("TerminalDoesBidi() = %v, want %v", got, tc.want)
			}
		})
	}
}

// recordingTty is a terminal that remembers what it was sent.
type recordingTty struct{ sent strings.Builder }

func (r *recordingTty) Start() error                          { return nil }
func (r *recordingTty) Stop() error                           { return nil }
func (r *recordingTty) Drain() error                          { return nil }
func (r *recordingTty) NotifyResize(func())                   {}
func (r *recordingTty) WindowSize() (tcell.WindowSize, error) { return tcell.WindowSize{}, nil }
func (r *recordingTty) Read(p []byte) (int, error)            { return 0, nil }
func (r *recordingTty) Write(p []byte) (int, error)           { return r.sent.Write(p) }
func (r *recordingTty) Close() error                          { return nil }

// ttyScreen is a simulated screen with a terminal to write to.
type ttyScreen struct {
	tcell.SimulationScreen
	tty *recordingTty
}

func (s ttyScreen) Tty() (tcell.Tty, bool) { return s.tty, true }

func newTTYScreen(t *testing.T) (*Screen, *recordingTty) {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	tty := &recordingTty{}
	return &Screen{Screen: ttyScreen{SimulationScreen: sim, tty: tty}}, tty
}

// While nem lays right-to-left text out, the terminal is told it need not:
// once, however often it is said, and told again that it must when nem is
// done.
func TestTakeBidiTellsTheTerminal(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	s, tty := newTTYScreen(t)
	s.TakeBidi(false)
	if got := tty.sent.String(); got != "" {
		t.Fatalf("giving back what was never taken sent %q", got)
	}
	s.TakeBidi(true)
	s.TakeBidi(true)
	if got := tty.sent.String(); got != "\x1b[8l" {
		t.Fatalf("taking the layout sent %q, want explicit mode once", got)
	}
	s.Close()
	if got := tty.sent.String(); got != "\x1b[8l\x1b[8h" {
		t.Errorf("after Close the terminal was sent %q, want its own layout back", got)
	}
}

// Inside tmux the switch is also passed through to the terminal outside,
// which is the one that draws.
func TestTakeBidiReachesTheTerminalOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	t.Setenv("STY", "")
	s, tty := newTTYScreen(t)
	s.TakeBidi(true)
	if got, want := tty.sent.String(), "\x1b[8l\x1bPtmux;\x1b\x1b[8l\x1b\\"; got != want {
		t.Errorf("inside tmux the terminal was sent %q, want %q", got, want)
	}
}
