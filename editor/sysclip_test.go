package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"
)

// --- choosing a tool -------------------------------------------------------

// clipboardCommands picks tools from the session, not from what happens to be
// installed: wl-paste answers for Wayland, xclip or xsel for X11, pbpaste on
// macOS. XWayland sessions set both WAYLAND_DISPLAY and DISPLAY, and there the
// native Wayland tool comes first.
func TestClipboardCommandsFollowTheSession(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}
	names := func(cmds [][]string) []string {
		var out []string
		for _, c := range cmds {
			out = append(out, c[0])
		}
		return out
	}

	for _, tc := range []struct {
		name string
		goos string
		env  map[string]string
		want []string
	}{
		{"wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-1"}, []string{"wl-paste"}},
		{"x11", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xclip", "xsel"}},
		{"xwayland prefers wayland", "linux",
			map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0"},
			[]string{"wl-paste", "xclip", "xsel"}},
		{"macOS", "darwin", nil, []string{"pbpaste"}},
		{"ssh with no display", "linux", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := names(clipboardCommands(tc.goos, env(tc.env)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("tools = %v, want %v", got, tc.want)
			}
		})
	}
}

// Every tool is asked for text specifically. An image on the clipboard must be
// refused by the tool, not handed over as bytes to insert.
func TestClipboardCommandsAskForTextOnly(t *testing.T) {
	cmds := clipboardCommands("linux", func(k string) string {
		return map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"}[k]
	})
	for _, c := range cmds {
		switch c[0] {
		case "wl-paste":
			if !slices.Contains(c, "--type") || !slices.Contains(c, "--no-newline") {
				t.Errorf("wl-paste args %v: want --type text and --no-newline", c)
			}
		case "xclip":
			if !slices.Contains(c, "UTF8_STRING") {
				t.Errorf("xclip args %v: want the UTF8_STRING target", c)
			}
		}
	}
}

// --- running a tool --------------------------------------------------------

// fakeTool puts an executable shell script called name first on PATH.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("writing fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestReadClipboardReturnsStdoutExactly(t *testing.T) {
	fakeTool(t, "wl-paste", `printf 'line one\n    indented\n\ttabbed'`)
	got, ok := runClipboardCommand([]string{"wl-paste"}, time.Second)
	if !ok {
		t.Fatal("no text read from a tool that printed text and exited 0")
	}
	if want := "line one\n    indented\n\ttabbed"; got != want {
		t.Errorf("read %q, want %q", got, want)
	}
}

// The trap, as measured on the user's machine: with an image on the clipboard,
// wl-paste --type text exits 1, prints nothing on stdout, and explains itself on
// stderr. Merging the streams, or ignoring the exit code, would paste the
// explanation into the user's buffer.
func TestReadClipboardRejectsAFailingTool(t *testing.T) {
	fakeTool(t, "wl-paste",
		`echo 'Clipboard content is not available as requested type "text"' >&2; exit 1`)
	got, ok := runClipboardCommand([]string{"wl-paste"}, time.Second)
	if ok || got != "" {
		t.Fatalf("read (%q, %v) from a tool that exited 1; want no text", got, ok)
	}
}

// A tool that fails partway has not produced the clipboard's text, whatever it
// managed to print first. The exit code decides, not the presence of output.
func TestReadClipboardRejectsPartialOutputFromAFailingTool(t *testing.T) {
	fakeTool(t, "xclip", `printf 'half a'; exit 1`)
	if got, ok := runClipboardCommand([]string{"xclip"}, time.Second); ok {
		t.Errorf("read %q from a tool that exited 1; want no text", got)
	}
}

// Stderr is never part of the result, even when the tool succeeds.
func TestReadClipboardIgnoresStderrOnSuccess(t *testing.T) {
	fakeTool(t, "wl-paste", `echo 'a warning' >&2; printf 'payload'`)
	got, ok := runClipboardCommand([]string{"wl-paste"}, time.Second)
	if !ok || got != "payload" {
		t.Errorf("read (%q, %v), want (%q, true)", got, ok, "payload")
	}
}

// Bytes that are not UTF-8 are not text, whatever the tool claims.
func TestReadClipboardRejectsInvalidUTF8(t *testing.T) {
	fakeTool(t, "wl-paste", `printf '\211PNG\r\n\032\n'`)
	if got, ok := runClipboardCommand([]string{"wl-paste"}, time.Second); ok {
		t.Errorf("read %q as text; invalid UTF-8 must be refused", got)
	}
}

func TestReadClipboardTreatsEmptyAsNothing(t *testing.T) {
	fakeTool(t, "wl-paste", `exit 0`)
	if _, ok := runClipboardCommand([]string{"wl-paste"}, time.Second); ok {
		t.Error("an empty clipboard was reported as text")
	}
}

// The tools are optional. One that is not installed is simply not there.
func TestReadClipboardWithAMissingToolIsSilent(t *testing.T) {
	if _, ok := runClipboardCommand([]string{"nem-no-such-clipboard-tool"}, time.Second); ok {
		t.Error("a missing tool produced text")
	}
}

// A stuck tool must never hang the editor. sleep runs as a child of the shell,
// so killing the shell at the deadline leaves a grandchild still holding stdout
// open: without WaitDelay, reading would wait the full ten seconds for it.
func TestReadClipboardTimesOutAStuckTool(t *testing.T) {
	fakeTool(t, "wl-paste", `sleep 10`)
	start := time.Now()
	if _, ok := runClipboardCommand([]string{"wl-paste"}, 100*time.Millisecond); ok {
		t.Error("a tool that never answered produced text")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("reading took %v; a stuck tool must not stall the editor", d)
	}
}

// The first tool that yields text wins; a failing one falls through.
func TestReadSystemClipboardFallsThroughToTheNextTool(t *testing.T) {
	fakeTool(t, "xclip", `exit 1`)
	fakeTool(t, "xsel", `printf 'from xsel'`)
	got, ok := readClipboardFrom([][]string{{"xclip"}, {"xsel"}}, time.Second)
	if !ok || got != "from xsel" {
		t.Errorf("read (%q, %v), want (%q, true)", got, ok, "from xsel")
	}
}

// --- the test binary never touches the real clipboard ---------------------

// The editor's default reader shells out to the system clipboard. A test that
// used it would read - and, through a paste-and-copy round trip, could clobber -
// whatever the developer last copied. TestMain replaces the default for this
// whole binary, and newTestEditor installs its own fake besides; this asserts
// both, without ever calling the real one.
func TestTestEditorsNeverUseTheRealClipboard(t *testing.T) {
	real := reflect.ValueOf(readSystemClipboard).Pointer()

	if reflect.ValueOf(defaultClipboardReader).Pointer() == real {
		t.Fatal("the package default is the real clipboard reader inside the test binary")
	}
	e, _ := newTestEditor(t)
	if e.clip.read == nil {
		t.Fatal("newTestEditor left no clipboard reader installed")
	}
	if reflect.ValueOf(e.clip.read).Pointer() == real {
		t.Fatal("newTestEditor uses the real clipboard reader")
	}
}
