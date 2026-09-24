package editor

import (
	"fmt"
	"path/filepath"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/sysopen"
	"github.com/gdamore/tcell/v2"
)

// A PDF, a photo or a song is not read into a buffer: a page of mojibake helps
// nobody, and a video read as text can be gigabytes. OpenFile hands such files
// to the system's own app instead - asking first, by default, since sometimes
// looking at the bytes is exactly the point.
//
// The answer can be remembered for the rest of the session, per extension:
// wanting PDFs in the viewer says nothing about wanting .bin files there. The
// open-binary setting answers without asking at all.

// binaryMode is what to do with a file that is not text.
type binaryMode int

const (
	binaryAsk binaryMode = iota
	binarySystem
	binaryText
)

// externalState is how non-text files are opened.
type externalState struct {
	// mode is the configured default.
	mode binaryMode
	// remembered holds this session's "always" answers, by lower-cased
	// extension; "" is files with none.
	remembered map[string]binaryMode
	// open and available are sysopen's, replaced in tests so a test run never
	// starts a real viewer.
	open      func(path string, failed func(error)) error
	available func() error
}

// systemOpen and systemAvailable are what New installs. They are variables so
// the test binary can replace them for every editor it builds; see TestMain.
var (
	systemOpen      = sysopen.Open
	systemAvailable = sysopen.Available
)

func newExternalState() externalState {
	return externalState{
		remembered: map[string]binaryMode{},
		open:       systemOpen,
		available:  systemAvailable,
	}
}

// SetOpenBinary sets what happens to a file that is not text: "ask", "system"
// or "text". The config host has validated the value; anything else is ignored.
func (e *Editor) SetOpenBinary(name string) {
	switch name {
	case "ask":
		e.ext.mode = binaryAsk
	case "system":
		e.ext.mode = binarySystem
	case "text":
		e.ext.mode = binaryText
	}
}

// openFailure carries a failed launch from the opener's goroutine to the event
// loop, which is the only place editor state may be touched.
type openFailure struct{ err error }

// openElsewhere decides whether the file at abs, which exists and is regular,
// goes to the system's app instead of a buffer, and sends it there if so. It
// reports true when the file was handed over.
func (e *Editor) openElsewhere(abs string) (bool, error) {
	isText, err := sysopen.IsText(abs)
	if err != nil || isText {
		// Unreadable is for LoadFile to report, with its own error.
		return false, nil
	}

	name, ext := filepath.Base(abs), sysopen.Ext(abs)
	mode, ok := e.ext.remembered[ext]
	if !ok {
		mode = e.ext.mode
	}
	if mode == binaryText {
		return false, nil
	}
	if err := e.ext.available(); err != nil {
		// Nowhere to send it, so the old behaviour - and say why, so the user
		// is not left wondering why the setting did nothing.
		e.Echo("%s is not text; opened as text (%v)", name, err)
		return false, nil
	}

	if mode == binaryAsk {
		label := "." + ext + " files"
		if ext == "" {
			label = "files like it"
		}
		// Short enough for an 80-column echo row with a typical name: cut
		// off, the S/T half is the part that would go.
		c, err := e.ReadChar(
			fmt.Sprintf("%s isn't text. s: system app, t: as text, S/T: always for %s ", name, label),
			[]rune{'s', 't', 'S', 'T'},
		)
		if err != nil {
			return false, err
		}
		// ReadChar puts back whatever the echo area said before the question,
		// which is news about something else by now.
		e.Echo("")
		switch c {
		case 'S':
			e.ext.remembered[ext] = binarySystem
		case 'T':
			e.ext.remembered[ext] = binaryText
			e.Echo("Opening %s as text for the rest of this session", label)
		}
		if c == 't' || c == 'T' {
			return false, nil
		}
	}

	if err := e.launch(abs); err != nil {
		return false, err
	}
	return true, nil
}

// launch hands path to the system's app and says so.
func (e *Editor) launch(path string) error {
	// The failure callback runs on the opener's goroutine, so it only posts an
	// event; the loop turns that into a message. scr is captured here rather
	// than read from e over there.
	scr := e.scr
	failed := func(err error) {
		if scr != nil {
			_ = scr.PostEvent(tcell.NewEventInterrupt(openFailure{err}))
		}
	}
	if err := e.ext.open(path, failed); err != nil {
		return err
	}
	e.Echo("Opened %s with the system app", filepath.Base(path))
	return nil
}

// registerExternalCommands adds the commands that open files outside nem.
func registerExternalCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{
			Name: "open-externally",
			Doc:  "Open this buffer's file, or the directory a listing shows, with the system app.",
			Fn: func(command.Env) error {
				path := e.active.Buf.Path()
				if st := e.diredOf(e.active.Buf); st != nil {
					path = st.dir
				}
				if path == "" {
					e.Echo("This buffer has no file")
					return nil
				}
				return e.launch(path)
			},
		},
		{
			Name: "dired-do-open",
			Doc:  "Open the marked files, or the file at point, with the system app.",
			Fn: func(command.Env) error {
				_, st, err := e.here()
				if err != nil {
					return err
				}
				ts := e.targets(st, '*')
				if len(ts) == 0 {
					e.noTarget(st)
					return nil
				}
				for _, en := range ts {
					if err := e.launch(st.pathOf(en)); err != nil {
						return err
					}
				}
				if len(ts) > 1 {
					e.Echo("Opened %s with the system app", describeCount(len(ts)))
				}
				return nil
			},
		},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}
