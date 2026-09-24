package editor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/gdamore/tcell/v2"
)

// Files that are not text, opened through a fake system opener: a test run
// must never start a real viewer.

// launches records what would have been handed to the system's app.
type launches struct {
	paths  []string
	failed func(error)
}

// fakeOpener installs a recording opener, available unless unavailable says
// otherwise, and returns the record.
func fakeOpener(e *Editor, unavailable error) *launches {
	l := &launches{}
	e.ext.open = func(path string, failed func(error)) error {
		l.paths = append(l.paths, path)
		l.failed = failed
		return nil
	}
	e.ext.available = func() error { return unavailable }
	return l
}

// binaryFiles writes a PNG-ish and a PDF-ish file and a text file into a
// fresh directory.
func binaryFiles(t *testing.T) (dir, png, pdf, txt string) {
	t.Helper()
	dir = t.TempDir()
	png, pdf, txt = filepath.Join(dir, "photo.png"), filepath.Join(dir, "paper.pdf"), filepath.Join(dir, "notes.txt")
	for p, data := range map[string]string{
		png: "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
		pdf: "%PDF-1.7\n%\xe2\xe3\xcf\xd3\n",
		txt: "plain words\n",
	} {
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, png, pdf, txt
}

// buffersFor lists the buffers visiting path.
func buffersFor(e *Editor, path string) int {
	n := 0
	for _, b := range e.Buffers() {
		if b.Path() == path {
			n++
		}
	}
	return n
}

// Asked, s hands the file to the system's app and makes no buffer; t opens it
// as text, as nem always used to.
func TestOpeningANonTextFileAsks(t *testing.T) {
	_, png, _, _ := binaryFiles(t)

	t.Run("s", func(t *testing.T) {
		e, scr := newTestEditor(t)
		l := fakeOpener(e, nil)
		feed(t, scr, txt("s"))
		b, err := e.OpenFile(png)
		if !errors.Is(err, command.ErrOpenedElsewhere) || b != nil {
			t.Fatalf("OpenFile = %v, %v; want ErrOpenedElsewhere and no buffer", b, err)
		}
		if !slices.Equal(l.paths, []string{png}) {
			t.Errorf("launched %q, want the photo", l.paths)
		}
		if buffersFor(e, png) != 0 {
			t.Error("a buffer was made for a file sent to the system app")
		}
		wantEcho(t, e, "Opened photo.png with the system app")
	})

	t.Run("t", func(t *testing.T) {
		e, scr := newTestEditor(t)
		l := fakeOpener(e, nil)
		feed(t, scr, txt("t"))
		b, err := e.OpenFile(png)
		if err != nil || b == nil || b.Path() != png {
			t.Fatalf("OpenFile = %v, %v; want a buffer on the photo", b, err)
		}
		if len(l.paths) != 0 {
			t.Errorf("launched %q after choosing text", l.paths)
		}
		if m := e.Message(); m != "" {
			t.Errorf("echo = %q after choosing text, want it clear", m)
		}
	})

	t.Run("C-g", func(t *testing.T) {
		e, scr := newTestEditor(t)
		l := fakeOpener(e, nil)
		feed(t, scr, key(t, "C-g"))
		if _, err := e.OpenFile(png); !errors.Is(err, command.ErrQuit) {
			t.Fatalf("OpenFile after C-g = %v, want ErrQuit", err)
		}
		if len(l.paths) != 0 || buffersFor(e, png) != 0 {
			t.Error("C-g still opened the file")
		}
	})
}

// S and T answer for every file of that type for the rest of the session, and
// only that type.
func TestTheAnswerCanBeRememberedPerExtension(t *testing.T) {
	dir, png, pdf, _ := binaryFiles(t)
	other := filepath.Join(dir, "second.png")
	if err := os.WriteFile(other, []byte("\x89PNG\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, scr := newTestEditor(t)
	l := fakeOpener(e, nil)

	feed(t, scr, txt("S"))
	_, _ = e.OpenFile(png)
	// No keys queued: a prompt here would block the test.
	if _, err := e.OpenFile(other); !errors.Is(err, command.ErrOpenedElsewhere) {
		t.Fatalf("second png: %v, want it sent without asking", err)
	}
	if len(l.paths) != 2 {
		t.Errorf("launched %q, want both photos", l.paths)
	}

	// A PDF is another question.
	feed(t, scr, txt("T"))
	if b, err := e.OpenFile(pdf); err != nil || b == nil {
		t.Fatalf("pdf: %v, %v; want it asked about and opened as text", b, err)
	}
	if len(l.paths) != 2 {
		t.Errorf("the pdf was launched too: %q", l.paths)
	}
	// The echo area says what was chosen, not what the last launch did.
	wantEcho(t, e, "Opening .pdf files as text")
}

// The open-binary setting answers without asking.
func TestOpenBinarySetting(t *testing.T) {
	_, png, _, _ := binaryFiles(t)

	e, _ := newTestEditor(t)
	l := fakeOpener(e, nil)
	e.SetOpenBinary("system")
	if _, err := e.OpenFile(png); !errors.Is(err, command.ErrOpenedElsewhere) || len(l.paths) != 1 {
		t.Errorf("open-binary system: err %v, launched %q", err, l.paths)
	}

	e2, _ := newTestEditor(t)
	l2 := fakeOpener(e2, nil)
	e2.SetOpenBinary("text")
	if b, err := e2.OpenFile(png); err != nil || b == nil || len(l2.paths) != 0 {
		t.Errorf("open-binary text: %v, %v, launched %q; want a buffer", b, err, l2.paths)
	}
}

// Text is never asked about, and with no system app to hand a file to - no
// display, say - it opens as text with a word about why.
func TestTextAndUnavailableOpenAsText(t *testing.T) {
	_, png, _, txtFile := binaryFiles(t)
	e, _ := newTestEditor(t)
	l := fakeOpener(e, errors.New("no graphical display"))

	if b, err := e.OpenFile(txtFile); err != nil || b == nil {
		t.Fatalf("text file: %v, %v", b, err)
	}
	if b, err := e.OpenFile(png); err != nil || b == nil {
		t.Fatalf("photo with no opener: %v, %v; want it opened as text", b, err)
	}
	if len(l.paths) != 0 {
		t.Errorf("launched %q with no opener available", l.paths)
	}
	wantEcho(t, e, "no graphical display")
}

// find-file and dired's RET both come through OpenFile, and a file sent away
// leaves the window where it was.
func TestFindFileAndDiredSendNonTextFilesAway(t *testing.T) {
	dir, png, _, _ := binaryFiles(t)

	e, scr := newTestEditor(t)
	l := fakeOpener(e, nil)
	scratch := e.active.Buf
	// One feeder, so the order holds: the path, M-RET to take it as typed,
	// then the answer to the question.
	feed(t, scr, txt(png), key(t, "M-RET"), txt("s"))
	press(t, e, "C-x", "C-f")
	if len(l.paths) != 1 || e.active.Buf != scratch {
		t.Errorf("find-file: launched %q, showing %q; want the photo launched and *scratch* kept",
			l.paths, e.BufferName(e.active.Buf))
	}

	e2, _ := newTestEditor(t)
	l2 := fakeOpener(e2, nil)
	e2.SetOpenBinary("system")
	b, _ := listed(t, e2, dir)
	goTo(t, e2, "photo.png")
	press(t, e2, "RET")
	if len(l2.paths) != 1 || e2.active.Buf != b {
		t.Errorf("dired RET: launched %q, showing %q; want the photo launched and the listing kept",
			l2.paths, e2.BufferName(e2.active.Buf))
	}
}

// E in dired hands the file at point to the system app even when it is text,
// and open-externally does the same for the current buffer - or, in a
// listing, the directory itself.
func TestOpeningExternallyOnPurpose(t *testing.T) {
	dir, _, _, txtFile := binaryFiles(t)
	e, _ := newTestEditor(t)
	l := fakeOpener(e, nil)
	listed(t, e, dir)
	goTo(t, e, "notes.txt")

	press(t, e, "E")
	if err := e.dispatch("open-externally"); err != nil {
		t.Fatal(err)
	}
	if want := []string{txtFile, dir}; !slices.Equal(l.paths, want) {
		t.Errorf("launched %q, want %q", l.paths, want)
	}
}

// A launch that fails after the fact is reported through the event loop, the
// only place allowed to touch editor state.
func TestALaunchFailureIsReported(t *testing.T) {
	_, png, _, _ := binaryFiles(t)
	e, scr := newTestEditor(t)
	l := fakeOpener(e, nil)
	e.SetOpenBinary("system")
	_, _ = e.OpenFile(png)

	l.failed(errors.New("xdg-open could not open photo.png (exit status 3)"))
	ev := scr.PollEvent()
	if _, ok := ev.(*tcell.EventInterrupt); !ok {
		t.Fatalf("got %T from the screen, want the posted interrupt", ev)
	}
	e.HandleEvent(ev)
	wantEcho(t, e, "could not open photo.png")
}
