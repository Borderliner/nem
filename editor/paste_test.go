package editor

import (
	"testing"
	"time"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// terminalBytes turns text into the key events tcell's input parser produces
// for those bytes (see input.go's scan): CR is KeyEnter, LF is Ctrl-J, TAB is
// KeyTab, DEL is Backspace and any other control byte is its Ctrl key. Building
// events this way, rather than as KeyRune for everything, is what makes these
// tests exercise the encoding a real terminal delivers - including the fact that
// a pasted newline would otherwise look to the keymap exactly like C-j.
func terminalBytes(s string) []tcell.Event {
	var out []tcell.Event
	for _, r := range s {
		switch {
		case r == '\r':
			out = append(out, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		case r == '\t':
			out = append(out, tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
		case r == 0x7f:
			out = append(out, tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone))
		case r < ' ':
			out = append(out, tcell.NewEventKey(tcell.KeyCtrlSpace+tcell.Key(r), 0, tcell.ModCtrl))
		default:
			out = append(out, tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		}
	}
	return out
}

// bracketed wraps text in the paste markers a terminal sends when bracketed
// paste mode is on.
func bracketed(s string) []tcell.Event {
	evs := []tcell.Event{tcell.NewEventPaste(true)}
	evs = append(evs, terminalBytes(s)...)
	return append(evs, tcell.NewEventPaste(false))
}

func deliver(e *Editor, evs []tcell.Event) {
	for _, ev := range evs {
		e.HandleEvent(ev)
	}
}

// The reported bug. Indented code pasted with Ctrl-Shift-V came out with its
// indentation compounded, because each newline ran newline - which copies the
// previous line's indent - and then the pasted text's own indent landed on top.
func TestPastedIndentedCodeArrivesVerbatim(t *testing.T) {
	code := "func main() {\n\tif ok {\n\t\trun()\n\t}\n    spaces()\n}"

	for _, tc := range []struct {
		name string
		wire string // what the terminal actually sends between the markers
	}{
		{"LF newlines", code},
		{"CR newlines", replaceAll(code, "\n", "\r")},
		{"CRLF newlines", replaceAll(code, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTestEditor(t)
			deliver(e, bracketed(tc.wire))
			wantText(t, e, code)
		})
	}
}

// The heart of the formatting bug, isolated: point sits at the end of an
// indented line, which is exactly where auto-indent fires.
func TestPasteBypassesAutoIndent(t *testing.T) {
	e, _ := newTestEditor(t, "    indented")
	e.Win().Pt = text.Pos{Line: 0, Col: 12}

	deliver(e, bracketed("\nflush"))

	wantText(t, e, "    indented\nflush")
}

func TestPastedTabIsATabNotIndentCommand(t *testing.T) {
	e, _ := newTestEditor(t)
	deliver(e, bracketed("a\tb"))
	wantText(t, e, "a\tb")
}

// Nothing between the markers may reach the keymap. These bytes are C-x C-c:
// dispatched as keys they would quit the editor. A pasted control character is
// dropped deliberately - it is invisible junk in a text file - and ordinary text
// around it survives.
func TestPastedBytesNeverReachTheKeymap(t *testing.T) {
	e, scr := newTestEditor(t)
	// Were C-x C-c dispatched, it would ask whether to leave the modified
	// buffer and wait for an answer forever. Queue one, so that failure is a
	// quick one rather than a hang. A correct editor never reads it.
	scr.InjectKey(tcell.KeyRune, 'y', tcell.ModNone)

	deliver(e, bracketed("keep\x18\x03this\x0b"))

	if e.quit {
		t.Fatal("pasted C-x C-c quit the editor: paste bytes were dispatched as keys")
	}
	wantText(t, e, "keepthis")
}

// One C-/ removes the whole paste, whatever it contained.
func TestOneUndoRemovesAWholePaste(t *testing.T) {
	e, _ := newTestEditor(t, "before")
	e.Win().Pt = text.Pos{Line: 0, Col: 6}

	deliver(e, bracketed(" one\ntwo\nthree"))
	wantText(t, e, "before one\ntwo\nthree")

	press(t, e, "C-/")
	wantText(t, e, "before")
}

// A single-character paste must not coalesce with typing before it: the undo
// log merges consecutive single-rune inserts, and a paste is not typing.
func TestSingleCharacterPasteIsItsOwnUndoUnit(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "a", "b")
	deliver(e, bracketed("c"))
	wantText(t, e, "abc")

	press(t, e, "C-/")
	wantText(t, e, "ab")
}

func TestPasteLeavesPointAtTheEnd(t *testing.T) {
	e, _ := newTestEditor(t, "xy")
	e.Win().Pt = text.Pos{Line: 0, Col: 1}

	deliver(e, bracketed("12\n34"))

	wantText(t, e, "x12\n34y")
	wantPt(t, e, 1, 2)
}

// With delete-selection on, pasting over a selection replaces it, exactly as
// typing does - and the replacement is still one undo step.
func TestPasteReplacesAnActiveSelection(t *testing.T) {
	e, _ := newTestEditor(t, "hello world")
	press(t, e, "C-x", "h")

	deliver(e, bracketed("bye"))
	wantText(t, e, "bye")

	press(t, e, "C-/")
	wantText(t, e, "hello world")
}

func TestPasteWithDeleteSelectionOffInsertsBesideIt(t *testing.T) {
	e, _ := newTestEditor(t, "hello")
	e.SetDeleteSelection(false)
	press(t, e, "C-x", "h") // point at the start, mark at the end

	deliver(e, bracketed(">"))
	wantText(t, e, ">hello")
}

// A half-typed prefix does not apply to pasted text: C-x then a paste must not
// look the first pasted character up as the second key of C-x.
func TestPasteDiscardsAPendingPrefix(t *testing.T) {
	e, _ := newTestEditor(t)
	press(t, e, "C-x")

	deliver(e, bracketed("o"))

	wantText(t, e, "o")
	if len(e.pending) != 0 {
		t.Errorf("pending = %v after a paste, want it cleared", e.pending)
	}
}

// An end marker without a start is ignored rather than inserting anything or
// confusing the next paste.
func TestStrayPasteEndIsIgnored(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	e.HandleEvent(tcell.NewEventPaste(false))
	press(t, e, "a")
	wantText(t, e, "ax")
}

// A terminal that sends a start marker and never an end would otherwise wedge
// the editor: every keystroke swallowed into a paste that never lands. Once
// pasted keys stop arriving for longer than a real paste ever pauses, the paste
// is taken as finished, what arrived is inserted, and the key that broke the
// silence is handled as the ordinary keystroke it is.
func TestAStalledPasteIsFlushedAndTheNextKeyTypedNormally(t *testing.T) {
	e, _ := newTestEditor(t)

	e.HandleEvent(tcell.NewEventPaste(true))
	deliver(e, terminalBytes("pasted"))

	// Pretend the end marker never came and the user has been waiting.
	e.paste.last = time.Now().Add(-2 * pasteStall)

	// A real keystroke, long after the paste went quiet. It goes through
	// HandleEvent, as a terminal key does: press would call HandleKey and skip
	// the layer a paste lives in.
	e.HandleEvent(tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModCtrl))
	if e.paste.active {
		t.Fatal("still pasting after the stall; the editor would be wedged")
	}
	wantText(t, e, "pasted")
	wantPt(t, e, 0, 0) // C-a ran as a command, not as pasted text
}

// The loop does not wait for a key to notice that a paste has stalled: once it
// has been quiet for pasteStall the text lands on its own, so it does not sit
// invisible until the user happens to press something.
func TestTheLoopLandsAStalledPasteWithoutAKey(t *testing.T) {
	e, scr := newTestEditor(t)
	e.HandleEvent(tcell.NewEventPaste(true))
	deliver(e, terminalBytes("stalled"))
	e.paste.last = time.Now().Add(-2 * pasteStall)

	// The hook runs on the loop's goroutine, so this channel is the only thing
	// the two goroutines share.
	landed := make(chan string, 1)
	e.AfterCommand(pasteCommand, func() { landed <- e.Buf().String() })

	done := make(chan struct{})
	go func() { _ = e.Loop(); close(done) }()
	defer func() {
		postAll(scr, []tcell.Event{
			tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModCtrl),
			tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl),
			tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone),
		})
		<-done
	}()

	select {
	case got := <-landed:
		if got != "stalled" {
			t.Errorf("buffer = %q when the stalled paste landed, want %q", got, "stalled")
		}
	case <-time.After(pasteStall):
		t.Fatal("a stalled paste never landed without another key")
	}
}

// A pasted block into a prompt lands in the prompt and fires its OnChange once,
// with the whole block - incremental search and completion filtering then see
// the pasted text, not a character at a time.
func TestPasteIntoAPromptFiresOnChangeOnce(t *testing.T) {
	e, scr := newTestEditor(t)

	var seen []string
	go func() {
		postAll(scr, bracketed("abc"))
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}()

	got, err := e.ReadString(readOpts("Pattern: ", func(s string) { seen = append(seen, s) }))
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if got != "abc" {
		t.Errorf("ReadString = %q, want %q", got, "abc")
	}
	if len(seen) != 1 || seen[0] != "abc" {
		t.Errorf("OnChange saw %q, want exactly one call with %q", seen, "abc")
	}
}

// A prompt is a single line. A trailing newline - which comes along whenever a
// whole line is copied - is dropped rather than becoming part of a file name,
// and any newline inside the paste becomes a space.
func TestPasteIntoAPromptFlattensNewlines(t *testing.T) {
	e, scr := newTestEditor(t)
	go func() {
		postAll(scr, bracketed("/tmp/some file\r"))
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}()

	got, err := e.ReadString(readOpts("Path: ", nil))
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if got != "/tmp/some file" {
		t.Errorf("ReadString = %q, want %q", got, "/tmp/some file")
	}

	go func() {
		postAll(scr, bracketed("a\nb"))
		scr.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	}()
	got, err = e.ReadString(readOpts("Path: ", nil))
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if got != "a b" {
		t.Errorf("ReadString = %q, want %q", got, "a b")
	}
}

// A prompt that reads one key - query-replace's y/n/!/q - is not a text field.
// Pasted text must be swallowed whole there, never taken as answers: pasting
// "yyyy" at a replace prompt must not replace four matches.
func TestPasteAtASingleKeyPromptIsIgnored(t *testing.T) {
	e, scr := newTestEditor(t)
	go func() {
		postAll(scr, bracketed("yyyy"))
		scr.InjectKey(tcell.KeyRune, 'n', tcell.ModNone)
	}()

	r, err := e.ReadChar("Replace? ", []rune("yn"))
	if err != nil {
		t.Fatalf("ReadChar: %v", err)
	}
	if r != 'n' {
		t.Errorf("ReadChar = %q, want the typed 'n' - the pasted y's were taken as answers", r)
	}
}

func TestPasteGateLetsKeysThroughAfterAStall(t *testing.T) {
	var g pasteGate
	now := time.Now()
	if !g.swallow(tcell.NewEventPaste(true), now) {
		t.Fatal("paste start not swallowed")
	}
	key := tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone)
	if !g.swallow(key, now.Add(time.Millisecond)) {
		t.Fatal("pasted key not swallowed")
	}
	if g.swallow(key, now.Add(2*pasteStall)) {
		t.Fatal("a key long after a paste that never ended was swallowed; the prompt would be wedged")
	}
}

// replaceAll avoids importing strings for one call.
func replaceAll(s, old, new string) string {
	out := []rune{}
	rs, o := []rune(s), []rune(old)
	for i := 0; i < len(rs); {
		if i+len(o) <= len(rs) && string(rs[i:i+len(o)]) == old {
			out = append(out, []rune(new)...)
			i += len(o)
			continue
		}
		out = append(out, rs[i])
		i++
	}
	return string(out)
}

// readOpts builds prompt options for the paste-into-prompt tests.
func readOpts(prompt string, onChange func(string)) command.ReadOpts {
	return command.ReadOpts{Prompt: prompt, OnChange: onChange}
}

// postAll delivers events in order. PostEvent refuses rather than blocks when
// the simulation screen's small queue is full, so it is retried until the
// consumer has made room.
func postAll(scr tcell.SimulationScreen, evs []tcell.Event) {
	for _, ev := range evs {
		for scr.PostEvent(ev) != nil {
			time.Sleep(time.Millisecond)
		}
	}
}

// The slowness half of the bug: every pasted character was a full dispatch and a
// full redraw. The loop now skips drawing while a paste is in progress and draws
// once at the end.
//
// Frames are counted rather than timed. A wall-clock assertion would be flaky on
// a loaded machine; a frame count is exact, and a redraw per character shows up
// as thousands of frames instead of a handful.
func TestAPasteRedrawsOnceNotPerCharacter(t *testing.T) {
	const n = 2000
	payload := make([]rune, n)
	for i := range payload {
		payload[i] = 'a' + rune(i%26)
	}

	// Driven through the real loop, which is where the redraws happen. Not
	// runKeys: the paste leaves *scratch* modified, so C-x C-c asks before
	// leaving, and the answer has to follow in the same stream.
	e, scr := newTestEditor(t)
	e.SetWhichKeyDelay(0)
	done := make(chan struct{})
	go func() { _ = e.Loop(); close(done) }()

	postAll(scr, bracketed(string(payload)))
	postAll(scr, []tcell.Event{
		tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModCtrl),
		tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone),
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the editor loop did not exit")
	}

	if got := e.Buf().String(); got != string(payload) {
		t.Fatalf("buffer holds %d runes, want the %d pasted", len([]rune(got)), n)
	}
	// Startup, the paste, and the quit keys account for a handful of frames.
	if e.frames > 50 {
		t.Errorf("drew %d frames for a %d-character paste; it is redrawing per character", e.frames, n)
	}
}
