package editor

// System clipboard integration: out over OSC 52, back in through the desktop's
// own clipboard tools (see sysclip.go).
//
// tcell does the hard part: Screen.SetClipboard base64-encodes the payload and
// emits the escape under tcell's own lock, so it cannot interleave with a frame
// being drawn, and GetClipboard issues the query whose answer arrives as a
// *tcell.EventClipboard. Nothing here hand-rolls base64 or writes escapes.
//
// The escape travels in the terminal's own output stream rather than talking to a
// display server, which is why this works over SSH — the case a terminal editor
// most needs it for. Inside tmux the sequence only leaves the pane when
// allow-passthrough is on; without it the write silently goes nowhere, which is
// worth knowing before concluding nem is at fault.

// ClipboardMode selects whether kills also reach the system clipboard.
type ClipboardMode int

const (
	// ClipboardOSC52 mirrors every kill to the terminal's clipboard, and lets
	// C-y take in text copied elsewhere. The zero value, so this is on unless
	// something turns it off.
	ClipboardOSC52 ClipboardMode = iota
	// ClipboardOff leaves the system clipboard alone, in both directions.
	ClipboardOff
)

// clipboard tracks what has been sent to the system clipboard.
type clipboard struct {
	mode ClipboardMode

	// accum mirrors the kill-ring entry currently being built. The ring
	// accumulates consecutive kills into one entry, so C-k C-k has to put both
	// lines on the clipboard rather than only the second — and the ring exposes
	// no way to read its newest entry without disturbing the yank state that
	// M-y depends on. See noteKill for how the run is tracked.
	accum string

	// sent is the last payload handed to the terminal, so a reply quoting our
	// own text back at us is not mistaken for a new copy made elsewhere.
	sent string

	// seen is the last clipboard text taken into the ring. A C-y with the
	// clipboard unchanged since then yanks the ring's front instead of pushing
	// the same text again - otherwise every C-y would add a copy and M-y would
	// have to wade through duplicates to reach anything older.
	seen string

	// read fetches the system clipboard's text for C-y. New installs
	// defaultClipboardReader; tests install a fake.
	read clipboardReader
}

// SetClipboardMode chooses whether kills reach the system clipboard.
func (e *Editor) SetClipboardMode(m ClipboardMode) { e.clip.mode = m }

// ClipboardMode reports the current setting.
func (e *Editor) ClipboardMode() ClipboardMode { return e.clip.mode }

// noteKill mirrors the ring's accumulation so the clipboard receives the whole
// entry rather than the latest fragment.
//
// The ring decides for itself whether a kill starts a new entry or extends the
// current one, and does not say which. The entry count is the signal: it grows
// when a new entry is pushed and holds steady when one is extended. That is
// exact below capacity, which is where every real session lives.
//
// At capacity the ring evicts as it pushes, so the count cannot grow and the two
// cases become indistinguishable. This treats that as a new entry deliberately:
// a clipboard holding only the most recent kill is recoverable — the ring still
// has everything — whereas one holding two unrelated kills spliced together is
// simply wrong, and the user would not know.
func (e *Editor) noteKill(s string, backward bool) {
	if s == "" {
		return // the ring ignores an empty kill, so nothing accumulated
	}

	before, capacity := e.ring.Len(), e.ring.Capacity()
	if backward {
		e.ring.KillBackward(s)
	} else {
		e.ring.KillForward(s)
	}

	switch {
	case e.ring.Len() > before, before >= capacity:
		e.clip.accum = s
	case backward:
		e.clip.accum = s + e.clip.accum
	default:
		e.clip.accum += s
	}
	e.pushClipboard(e.clip.accum)
}

// pushClipboard hands text to the terminal's clipboard.
func (e *Editor) pushClipboard(s string) {
	if e.clip.mode != ClipboardOSC52 || e.scr == nil || s == "" {
		return
	}
	e.clip.sent = s
	e.scr.SetClipboard([]byte(s))
}

// RequestClipboard asks the terminal for its clipboard contents.
//
// The answer is asynchronous and may never come: OSC 52 reads are far less
// widely implemented than writes, and terminals that do support writing often
// refuse to read on the grounds that a program should not be able to exfiltrate
// whatever you last copied. So this cannot be made to look synchronous, and
// yanking does not wait on it — C-y reads through the desktop's clipboard tools
// instead (see takeSystemClipboard). When a reply does arrive,
// SetClipboardReply puts it at the front of the ring so the next C-y picks it
// up.
func (e *Editor) RequestClipboard() {
	if e.clip.mode != ClipboardOSC52 || e.scr == nil {
		e.Echo("System clipboard reads are off")
		return
	}
	e.scr.GetClipboard()
	e.Echo("Asked the terminal for its clipboard")
}

// SetClipboardReply accepts the terminal's answer to RequestClipboard. The event
// loop routes *tcell.EventClipboard here.
//
// The reply becomes the newest kill-ring entry, which is what makes it reachable
// by the ordinary C-y rather than needing a second yank command. Text nem itself
// put on the clipboard is ignored: terminals echo it straight back, and adding it
// again would duplicate an entry the ring already holds.
func (e *Editor) SetClipboardReply(data []byte) {
	if e.takeClipboardText(string(data)) {
		e.Echo("Clipboard contents added to the kill ring")
	}
}

// takeSystemClipboard puts text copied in another application at the front of
// the ring, so the C-y about to read the ring yanks it. This is emacs's
// interprogram-paste-function.
//
// It asks every time rather than watching for changes, because nothing tells a
// terminal program that the clipboard changed. When there is no text to be had
// - an image, no display, no tool installed - the ring is left as it was and C-y
// yanks from it as it always did.
func (e *Editor) takeSystemClipboard() {
	if e.clip.mode == ClipboardOff || e.clip.read == nil {
		return
	}
	if s, ok := e.clip.read(); ok {
		e.takeClipboardText(s)
	}
}

// takeClipboardText makes s the newest kill-ring entry, unless the ring already
// has it: text nem itself sent, which terminals and clipboards hand straight
// back, or text an earlier read already took. It reports whether s was added.
func (e *Editor) takeClipboardText(s string) bool {
	if s == "" || s == e.clip.sent || s == e.clip.seen {
		return false
	}
	// A fresh entry, not an extension of whatever kill run was open: this text
	// came from somewhere else entirely.
	e.ring.BreakRun()
	e.ring.KillForward(s)
	e.ring.BreakRun()
	e.clip.accum = ""
	e.clip.seen = s
	return true
}
