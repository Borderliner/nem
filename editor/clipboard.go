package editor

// System clipboard integration over OSC 52.
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
	// ClipboardOSC52 mirrors every kill to the terminal's clipboard. The zero
	// value, so this is on unless something turns it off.
	ClipboardOSC52 ClipboardMode = iota
	// ClipboardOff leaves the system clipboard alone.
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
// yanking does not wait on it — C-y takes nem's own kill ring, which always
// works. When a reply does arrive, SetClipboardReply puts it at the front of the
// ring so the next C-y picks it up.
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
	s := string(data)
	if s == "" || s == e.clip.sent {
		return
	}
	// A fresh entry, not an extension of whatever kill run was open: this text
	// came from somewhere else entirely.
	e.ring.BreakRun()
	e.ring.KillForward(s)
	e.ring.BreakRun()
	e.clip.accum = ""
	e.Echo("Clipboard contents added to the kill ring")
}
