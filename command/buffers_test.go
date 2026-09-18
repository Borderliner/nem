package command_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
)

// recEnv wraps a Fake to capture the ReadOpts each prompt was built with.
//
// commandtest.Fake records only opts.Prompt, so completion candidates and
// pre-filled text are otherwise invisible to a test. Embedding and overriding
// ReadString keeps that assertable without modifying the shared fake.
type recEnv struct {
	*commandtest.Fake
	opts []command.ReadOpts
}

func (r *recEnv) ReadString(opts command.ReadOpts) (string, error) {
	r.opts = append(r.opts, opts)
	return r.Fake.ReadString(opts)
}

// lastOpts returns the ReadOpts of the most recent prompt.
func (r *recEnv) lastOpts(t *testing.T) command.ReadOpts {
	t.Helper()
	if len(r.opts) == 0 {
		t.Fatal("no prompt was issued")
	}
	return r.opts[len(r.opts)-1]
}

func newEnv(lines ...string) *recEnv {
	return &recEnv{Fake: commandtest.New(lines...)}
}

// runCmd dispatches name through a registry holding only the buffer commands.
func runCmd(t *testing.T, e command.Env, name string) error {
	t.Helper()
	r := command.NewRegistry()
	if err := command.RegisterBuffers(r); err != nil {
		t.Fatalf("RegisterBuffers: %v", err)
	}
	return r.Run(name, e)
}

func mustRun(t *testing.T, e command.Env, name string) {
	t.Helper()
	if err := runCmd(t, e, name); err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
}

// --- registration --------------------------------------------------------

func TestRegisterBuffersRegistersEveryCommand(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterBuffers(r); err != nil {
		t.Fatalf("RegisterBuffers: %v", err)
	}
	want := []string{
		"find-file", "save-buffer", "write-file", "save-some-buffers",
		"switch-to-buffer", "kill-buffer", "list-buffers",
		"split-window-below", "split-window-right", "delete-other-windows",
		"delete-window", "other-window",
		"keyboard-quit", "save-buffers-kill-terminal",
	}
	for _, name := range want {
		c, ok := r.Lookup(name)
		if !ok {
			t.Errorf("%s not registered", name)
			continue
		}
		if c.Doc == "" {
			t.Errorf("%s has no doc string", name)
		}
		if !c.Interactive {
			t.Errorf("%s should be interactive so M-x finds it", name)
		}
	}
	if got := len(r.Names()); got != len(want) {
		t.Errorf("registry holds %d interactive commands, want %d", got, len(want))
	}
}

func TestRegisterBuffersTwiceIsAnError(t *testing.T) {
	r := command.NewRegistry()
	if err := command.RegisterBuffers(r); err != nil {
		t.Fatalf("first RegisterBuffers: %v", err)
	}
	if err := command.RegisterBuffers(r); !errors.Is(err, command.ErrDuplicateCommand) {
		t.Errorf("second RegisterBuffers returned %v, want ErrDuplicateCommand", err)
	}
}

// --- find-file -----------------------------------------------------------

func TestFindFileOpensSeededFile(t *testing.T) {
	e := newEnv("original")
	e.Files["/proj/main.go"] = "package main"
	e.Replies = []string{"/proj/main.go"}

	mustRun(t, e, "find-file")

	if got := e.Text(); got != "package main" {
		t.Errorf("visited buffer holds %q, want %q", got, "package main")
	}
	if got := e.Buf().Path(); got != "/proj/main.go" {
		t.Errorf("path is %q, want %q", got, "/proj/main.go")
	}
}

// A path that does not exist is how a new file is created, not an error.
func TestFindFileOnMissingPathOpensEmptyBufferWithPath(t *testing.T) {
	e := newEnv("original")
	e.Replies = []string{"/proj/brand-new.txt"}

	mustRun(t, e, "find-file")

	if got := e.Text(); got != "" {
		t.Errorf("new-file buffer holds %q, want empty", got)
	}
	if got := e.Buf().Path(); got != "/proj/brand-new.txt" {
		t.Errorf("path is %q, want the requested path", got)
	}
	if e.Buf().Modified() {
		t.Error("a freshly opened new file should not be marked modified")
	}
}

func TestFindFileQuitLeavesEverythingAlone(t *testing.T) {
	e := newEnv("original")
	before := e.Buf()
	e.Replies = []string{commandtest.Quit}

	err := runCmd(t, e, "find-file")

	if !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if e.Buf() != before {
		t.Error("C-g at the prompt still switched buffers")
	}
	if got := e.Text(); got != "original" {
		t.Errorf("buffer text became %q, want it untouched", got)
	}
}

// A genuine failure to open — unreadable, not merely absent — is an error the
// user must see, unlike a missing path which creates a new file.
func TestFindFileReportsARealOpenFailure(t *testing.T) {
	e := newEnv("original")
	before := e.Buf()
	wantErr := errors.New("permission denied")
	e.OpenFileErr = wantErr
	e.Replies = []string{"/root/secret"}

	err := runCmd(t, e, "find-file")

	if !errors.Is(err, wantErr) {
		t.Errorf("returned %v, want the open error", err)
	}
	if e.Buf() != before {
		t.Error("a failed open still switched buffers")
	}
}

func TestFindFileEmptyReplyIsANoOp(t *testing.T) {
	e := newEnv("original")
	before := e.Buf()
	e.Replies = []string{""}

	mustRun(t, e, "find-file")

	if e.Buf() != before {
		t.Error("an empty filename still switched buffers")
	}
}

func TestFindFilePromptOffersCompletion(t *testing.T) {
	e := newEnv()
	e.Replies = []string{""}
	mustRun(t, e, "find-file")

	if e.lastOpts(t).Complete == nil {
		t.Error("find-file prompt has no completion function")
	}
}

// Filename completion is unexported, so it is exercised through the Complete
// function the prompt was actually built with.
func TestFilenameCompletionListsMatchingEntries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"album.txt", "alpha.txt", "zeta.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "alpine"), 0o755); err != nil {
		t.Fatal(err)
	}

	e := newEnv()
	e.Replies = []string{""}
	mustRun(t, e, "find-file")
	complete := e.lastOpts(t).Complete

	got := complete(filepath.Join(dir, "al"))
	want := []string{
		filepath.Join(dir, "album.txt"),
		filepath.Join(dir, "alpha.txt"),
		// A directory keeps its separator so repeated completion can descend
		// into it instead of stalling.
		filepath.Join(dir, "alpine") + string(filepath.Separator),
	}
	if !slices.Equal(got, want) {
		t.Errorf("completing %q gave\n  %q\nwant\n  %q", "al", got, want)
	}
	for _, g := range got {
		if strings.Contains(g, "zeta") {
			t.Errorf("completion returned a non-matching entry %q", g)
		}
	}
	// A trailing separator means "everything in this directory".
	if got := complete(dir + string(filepath.Separator)); len(got) != 4 {
		t.Errorf("listing the directory completed to %q, want all four entries", got)
	}
	if got := complete("/no/such/directory/anywhere/x"); got != nil {
		t.Errorf("unreadable directory returned %q, want nil", got)
	}
}

// --- save-buffer and write-file ------------------------------------------

func TestSaveBufferWritesToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	e := newEnv("line one", "line two")
	e.Buf().SetPath(path)
	e.Buf().SetModified(true)

	mustRun(t, e, "save-buffer")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "line one\nline two" {
		t.Errorf("file holds %q", string(got))
	}
	if e.Buf().Modified() {
		t.Error("buffer still marked modified after a successful save")
	}
	if len(e.Echoes) == 0 || !strings.Contains(e.Echoes[len(e.Echoes)-1], path) {
		t.Errorf("echoes %q, want a message naming the file", e.Echoes)
	}
}

// A pathless buffer must prompt rather than fail with ErrNoPath.
func TestSaveBufferWithNoPathPrompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "named.txt")
	e := newEnv("content")
	e.Buf().SetModified(true)
	e.Replies = []string{path}

	mustRun(t, e, "save-buffer")

	if len(e.Prompts) != 1 {
		t.Fatalf("prompts %q, want exactly one", e.Prompts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "content" {
		t.Errorf("file holds %q, want %q", string(got), "content")
	}
	if e.Buf().Path() != path {
		t.Errorf("buffer path is %q, want %q", e.Buf().Path(), path)
	}
}

func TestSaveBufferWithNoPathQuitWritesNothing(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("content")
	e.Buf().SetModified(true)
	e.Replies = []string{commandtest.Quit}

	if err := runCmd(t, e, "save-buffer"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("C-g still wrote %d file(s)", len(entries))
	}
	if !e.Buf().Modified() {
		t.Error("abandoning the save cleared the modified flag")
	}
}

func TestSaveBufferUnmodifiedDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "untouched.txt")
	e := newEnv("content")
	e.Buf().SetPath(path)
	e.Buf().SetModified(false)

	mustRun(t, e, "save-buffer")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an unmodified buffer was written to disk anyway")
	}
	if len(e.Echoes) == 0 {
		t.Error("expected a message saying there was nothing to save")
	}
}

func TestWriteFileSavesUnderNewPathAndPrefillsCurrent(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	newPath := filepath.Join(dir, "new.txt")
	e := newEnv("body")
	e.Buf().SetPath(oldPath)
	e.Replies = []string{newPath}

	mustRun(t, e, "write-file")

	if got := e.lastOpts(t).Initial; got != oldPath {
		t.Errorf("prompt pre-filled with %q, want the current path %q", got, oldPath)
	}
	if got, err := os.ReadFile(newPath); err != nil || string(got) != "body" {
		t.Errorf("new file: %q, err %v", string(got), err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("write-file also wrote the old path")
	}
	if e.Buf().Path() != newPath {
		t.Errorf("buffer path is %q, want %q", e.Buf().Path(), newPath)
	}
}

func TestWriteFileEmptyReplyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("body")
	e.Replies = []string{""}

	mustRun(t, e, "write-file")

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("an empty filename still wrote %d file(s)", len(entries))
	}
	if e.Buf().Path() != "" {
		t.Errorf("buffer path became %q, want it unset", e.Buf().Path())
	}
}

func TestWriteFileQuitWritesNothing(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("body")
	e.Replies = []string{commandtest.Quit}

	if err := runCmd(t, e, "write-file"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("C-g still wrote %d file(s)", len(entries))
	}
}

// --- save-some-buffers ---------------------------------------------------

// modifiedIn seeds n extra buffers with paths in dir, all modified.
func modifiedIn(t *testing.T, e *recEnv, dir string, names ...string) []*text.Buffer {
	t.Helper()
	var out []*text.Buffer
	for _, n := range names {
		b := e.AddBuffer(n, "body of "+n)
		b.SetPath(filepath.Join(dir, n))
		b.SetModified(true)
		out = append(out, b)
	}
	return out
}

func TestSaveSomeBuffersBangStopsPrompting(t *testing.T) {
	dir := t.TempDir()
	e := newEnv()
	bufs := modifiedIn(t, e, dir, "a.txt", "b.txt", "c.txt")
	e.Chars = []rune{'!'}

	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 1 {
		t.Errorf("prompted %d times, want 1 — ! must stop asking", len(e.CharPrompts))
	}
	for _, b := range bufs {
		if b.Modified() {
			t.Errorf("%s still modified after !", b.Path())
		}
		if _, err := os.Stat(b.Path()); err != nil {
			t.Errorf("%s was not written: %v", b.Path(), err)
		}
	}
}

func TestSaveSomeBuffersDeclineLeavesBufferModified(t *testing.T) {
	dir := t.TempDir()
	e := newEnv()
	bufs := modifiedIn(t, e, dir, "keep.txt")
	e.Chars = []rune{'n'}

	mustRun(t, e, "save-some-buffers")

	if !bufs[0].Modified() {
		t.Error("declining still saved the buffer")
	}
	if _, err := os.Stat(bufs[0].Path()); !os.IsNotExist(err) {
		t.Error("declining still wrote the file")
	}
}

func TestSaveSomeBuffersQuitStopsImmediately(t *testing.T) {
	dir := t.TempDir()
	e := newEnv()
	bufs := modifiedIn(t, e, dir, "a.txt", "b.txt")
	e.Chars = []rune{'q'}

	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 1 {
		t.Errorf("prompted %d times, want 1 — q must stop the walk", len(e.CharPrompts))
	}
	for _, b := range bufs {
		if !b.Modified() {
			t.Errorf("%s was saved despite q", b.Path())
		}
	}
}

// "Saved 1 file", not "Saved 1 files".
func TestSaveSomeBuffersReportsASingleFileInTheSingular(t *testing.T) {
	dir := t.TempDir()
	e := newEnv()
	modifiedIn(t, e, dir, "only.txt")
	e.Chars = []rune{'y'}

	mustRun(t, e, "save-some-buffers")

	last := e.Echoes[len(e.Echoes)-1]
	if !strings.Contains(last, "1 file") || strings.Contains(last, "1 files") {
		t.Errorf("echoed %q, want the singular form", last)
	}
}

func TestSaveSomeBuffersReportsSeveralFilesInThePlural(t *testing.T) {
	dir := t.TempDir()
	e := newEnv()
	modifiedIn(t, e, dir, "a.txt", "b.txt")
	e.Chars = []rune{'!'}

	mustRun(t, e, "save-some-buffers")

	last := e.Echoes[len(e.Echoes)-1]
	if !strings.Contains(last, "2 files") {
		t.Errorf("echoed %q, want the plural form", last)
	}
}

func TestSaveSomeBuffersWithNothingToSaveSaysSo(t *testing.T) {
	e := newEnv("clean")
	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 0 {
		t.Errorf("prompted %q with no modified buffers", e.CharPrompts)
	}
	if len(e.Echoes) == 0 {
		t.Error("expected a message saying no files need saving")
	}
}

func TestSaveSomeBuffersSkipsPathlessBuffers(t *testing.T) {
	e := newEnv("scratch text")
	e.Buf().SetModified(true)

	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 0 {
		t.Errorf("prompted %q for a buffer with no file", e.CharPrompts)
	}
	if !e.Buf().Modified() {
		t.Error("a pathless buffer was somehow saved")
	}
}

// --- switch-to-buffer ----------------------------------------------------

func TestSwitchToBufferVisitsExisting(t *testing.T) {
	e := newEnv("scratch")
	other := e.AddBuffer("notes.md", "my notes")
	e.Replies = []string{"notes.md"}

	mustRun(t, e, "switch-to-buffer")

	if e.Buf() != other {
		t.Error("did not switch to the named buffer")
	}
	if got := e.Text(); got != "my notes" {
		t.Errorf("active buffer holds %q", got)
	}
}

func TestSwitchToBufferUnknownNameCreatesIt(t *testing.T) {
	e := newEnv("scratch")
	before := len(e.Buffers())
	e.Replies = []string{"fresh"}

	mustRun(t, e, "switch-to-buffer")

	if got := len(e.Buffers()); got != before+1 {
		t.Errorf("buffer count %d, want %d", got, before+1)
	}
	if got := e.BufferName(e.Buf()); got != "fresh" {
		t.Errorf("active buffer is named %q, want %q", got, "fresh")
	}
	if got := e.Text(); got != "" {
		t.Errorf("new buffer holds %q, want empty", got)
	}
}

func TestSwitchToBufferCompletesOverBufferNames(t *testing.T) {
	e := newEnv("scratch")
	e.AddBuffer("notes.md")
	e.AddBuffer("nginx.conf")
	e.Replies = []string{""}

	mustRun(t, e, "switch-to-buffer")

	complete := e.lastOpts(t).Complete
	if complete == nil {
		t.Fatal("switch-to-buffer offers no completion")
	}
	got := complete("n")
	want := []string{"nginx.conf", "notes.md"}
	if !slices.Equal(got, want) {
		t.Errorf("completing %q gave %q, want %q", "n", got, want)
	}
	if all := complete(""); len(all) != 3 {
		t.Errorf("empty prefix completed to %q, want all three buffers", all)
	}
}

func TestSwitchToBufferQuitStaysPut(t *testing.T) {
	e := newEnv("scratch")
	before := e.Buf()
	e.AddBuffer("notes.md")
	e.Replies = []string{commandtest.Quit}

	if err := runCmd(t, e, "switch-to-buffer"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if e.Buf() != before {
		t.Error("C-g still switched buffers")
	}
}

// --- kill-buffer ---------------------------------------------------------

func TestKillBufferUnmodifiedDoesNotPrompt(t *testing.T) {
	e := newEnv("clean")
	e.AddBuffer("other.txt", "keep me")
	victim := e.Buf()

	mustRun(t, e, "kill-buffer")

	if len(e.CharPrompts) != 0 {
		t.Errorf("prompted %q to kill an unmodified buffer", e.CharPrompts)
	}
	if slices.Contains(e.Buffers(), victim) {
		t.Error("buffer was not killed")
	}
	if e.Buf() == victim {
		t.Error("the window is still showing the killed buffer")
	}
}

// Silently discarding a user's unsaved text is the worst bug this file could
// have, so declining must genuinely abort.
func TestKillBufferDeclineKeepsBufferAndContents(t *testing.T) {
	e := newEnv("precious")
	e.AddBuffer("other.txt")
	victim := e.Buf()
	victim.SetModified(true)
	e.Chars = []rune{'n'}

	mustRun(t, e, "kill-buffer")

	if len(e.CharPrompts) != 1 {
		t.Errorf("prompted %d times, want 1", len(e.CharPrompts))
	}
	if !slices.Contains(e.Buffers(), victim) {
		t.Fatal("declining still killed the buffer")
	}
	if !victim.Modified() {
		t.Error("declining cleared the modified flag")
	}
	if got := victim.String(); got != "precious" {
		t.Errorf("buffer text is %q, want it intact", got)
	}
}

func TestKillBufferAcceptKillsModifiedBuffer(t *testing.T) {
	e := newEnv("throwaway")
	e.AddBuffer("other.txt")
	victim := e.Buf()
	victim.SetModified(true)
	e.Chars = []rune{'y'}

	mustRun(t, e, "kill-buffer")

	if slices.Contains(e.Buffers(), victim) {
		t.Error("answering y did not kill the buffer")
	}
}

func TestKillBufferQuitAborts(t *testing.T) {
	e := newEnv("precious")
	e.AddBuffer("other.txt")
	victim := e.Buf()
	victim.SetModified(true)
	e.Chars = []rune{commandtest.QuitChar}

	if err := runCmd(t, e, "kill-buffer"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if !slices.Contains(e.Buffers(), victim) {
		t.Error("C-g still killed the buffer")
	}
}

func TestKillBufferRefusesTheLastBuffer(t *testing.T) {
	e := newEnv("only one")

	err := runCmd(t, e, "kill-buffer")

	if err == nil && len(e.Echoes) == 0 {
		t.Error("killing the sole buffer neither failed nor reported anything")
	}
	if len(e.Buffers()) != 1 {
		t.Errorf("buffer count %d, want the sole buffer kept", len(e.Buffers()))
	}
}

// --- list-buffers --------------------------------------------------------

func TestListBuffersRendersIntoBufferListBuffer(t *testing.T) {
	e := newEnv("scratch")
	e.AddBuffer("notes.md", "notes")
	e.AddBuffer("nginx.conf", "conf")

	mustRun(t, e, "list-buffers")

	if got := e.BufferName(e.Buf()); got != "*Buffer List*" {
		t.Fatalf("active buffer is %q, want *Buffer List*", got)
	}
	body := e.Text()
	for _, want := range []string{"notes.md", "nginx.conf", "*scratch*"} {
		if !strings.Contains(body, want) {
			t.Errorf("listing omits %q:\n%s", want, body)
		}
	}
	if e.Buf().Modified() {
		t.Error("the listing buffer should not start out modified")
	}
	if e.Buf().Path() != "" {
		t.Errorf("listing buffer has path %q, want none", e.Buf().Path())
	}
}

func TestListBuffersMarksModifiedBuffers(t *testing.T) {
	e := newEnv("scratch")
	dirty := e.AddBuffer("dirty.txt", "x")
	dirty.SetModified(true)

	mustRun(t, e, "list-buffers")

	var line string
	for _, l := range strings.Split(e.Text(), "\n") {
		if strings.Contains(l, "dirty.txt") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no line for dirty.txt in:\n%s", e.Text())
	}
	if !strings.Contains(line, "*") {
		t.Errorf("modified buffer line lacks a marker: %q", line)
	}
}

// Running it twice must reuse the listing buffer, not accumulate copies.
func TestListBuffersReusesTheExistingListing(t *testing.T) {
	e := newEnv("scratch")
	e.AddBuffer("notes.md")

	mustRun(t, e, "list-buffers")
	first := e.Buf()
	countAfterFirst := len(e.Buffers())

	mustRun(t, e, "list-buffers")

	if e.Buf() != first {
		t.Error("second run created a different listing buffer")
	}
	if got := len(e.Buffers()); got != countAfterFirst {
		t.Errorf("buffer count grew to %d, want %d", got, countAfterFirst)
	}
	if n := strings.Count(e.Text(), "notes.md"); n != 1 {
		t.Errorf("notes.md appears %d times — the listing was appended, not replaced", n)
	}
}

// --- windows -------------------------------------------------------------

func TestSplitCommandsPassTheRightOrientation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		vertical bool
	}{
		{"split-window-below", false},
		{"split-window-right", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv("text")
			mustRun(t, e, tc.name)
			if !slices.Equal(e.Splits, []bool{tc.vertical}) {
				t.Errorf("splits %v, want [%v]", e.Splits, tc.vertical)
			}
		})
	}
}

// A frame too small to split is a message for the user, not a command failure.
func TestSplitTooSmallEchoesInsteadOfFailing(t *testing.T) {
	e := newEnv("text")
	e.SplitErr = errors.New("view: not enough room to split")

	if err := runCmd(t, e, "split-window-below"); err != nil {
		t.Errorf("returned %v, want nil with an echoed message", err)
	}
	if len(e.Echoes) == 0 {
		t.Fatal("no message was echoed")
	}
	if !strings.Contains(e.Echoes[0], "room") {
		t.Errorf("echoed %q, want it to carry the reason", e.Echoes[0])
	}
}

func TestDeleteWindowSoleWindowEchoesRefusal(t *testing.T) {
	e := newEnv("text")
	e.DeleteWindowErr = errors.New("view: cannot delete the sole window")

	if err := runCmd(t, e, "delete-window"); err != nil {
		t.Errorf("returned %v, want nil with an echoed message", err)
	}
	if len(e.Echoes) == 0 || !strings.Contains(e.Echoes[0], "sole window") {
		t.Errorf("echoes %q, want the refusal reported", e.Echoes)
	}
}

func TestDeleteWindowAndDeleteOtherWindows(t *testing.T) {
	e := newEnv("text")
	mustRun(t, e, "delete-window")
	mustRun(t, e, "delete-other-windows")

	if e.DeleteWindowCalls != 1 {
		t.Errorf("DeleteWindow called %d times, want 1", e.DeleteWindowCalls)
	}
	if e.DeleteOtherWindowsCalls != 1 {
		t.Errorf("DeleteOtherWindows called %d times, want 1", e.DeleteOtherWindowsCalls)
	}
}

func TestOtherWindowHonoursTheUniversalArgument(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  int
		want int
	}{
		{"no argument moves one", 1, 1},
		{"C-u 2 moves two", 2, 2},
		{"negative moves backward", -1, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv("text")
			e.ArgN = tc.arg
			e.ArgExplicit = tc.arg != 1
			mustRun(t, e, "other-window")
			if !slices.Equal(e.OtherWindowArgs, []int{tc.want}) {
				t.Errorf("OtherWindow got %v, want [%d]", e.OtherWindowArgs, tc.want)
			}
		})
	}
}

// --- session -------------------------------------------------------------

// C-g must always report something; a silent C-g leaves the user unsure
// whether the editor is wedged.
func TestKeyboardQuitAlwaysReports(t *testing.T) {
	e := newEnv("text")
	mustRun(t, e, "keyboard-quit")

	if len(e.Echoes) == 0 {
		t.Fatal("keyboard-quit echoed nothing")
	}
	if !strings.Contains(e.Echoes[0], "Quit") {
		t.Errorf("echoed %q, want it to say Quit", e.Echoes[0])
	}
}

// C-g deactivates the region, and must do so without disturbing anything else.
// A keyboard-quit that quietly moved point would be maddening to use and no
// other test would catch it.
func TestKeyboardQuitDeactivatesTheMarkAndTouchesNothingElse(t *testing.T) {
	e := newEnv("first line", "second line", "third line")
	e.SetPoint(text.Pos{Line: 1, Col: 4})
	e.Buf().SetMark(text.Pos{Line: 2, Col: 2})
	if !e.Buf().HasMark() {
		t.Fatal("precondition: mark was not set")
	}
	textBefore := e.Text()
	pointBefore := e.Point()

	mustRun(t, e, "keyboard-quit")

	if e.Buf().HasMark() {
		t.Error("the mark is still active after C-g")
	}
	if got := e.Text(); got != textBefore {
		t.Errorf("buffer text changed to %q, want %q", got, textBefore)
	}
	if got := e.Point(); got != pointBefore {
		t.Errorf("point moved to %+v, want it left at %+v", got, pointBefore)
	}
}

// A mark deliberately set at the origin is still a mark, so C-g must clear it.
// This is the case a bare Mark() == Pos{} check gets wrong.
func TestKeyboardQuitClearsAMarkAtTheOrigin(t *testing.T) {
	e := newEnv("text")
	e.Buf().SetMark(text.Pos{})
	if !e.Buf().HasMark() {
		t.Fatal("precondition: a mark at the origin should count as set")
	}

	mustRun(t, e, "keyboard-quit")

	if e.Buf().HasMark() {
		t.Error("a mark at the origin survived C-g")
	}
}

// With no mark set, C-g is still not a no-op: it reports.
func TestKeyboardQuitWithNoMarkStillReports(t *testing.T) {
	e := newEnv("text")
	if e.Buf().HasMark() {
		t.Fatal("precondition: expected no mark on a fresh buffer")
	}

	mustRun(t, e, "keyboard-quit")

	if e.Buf().HasMark() {
		t.Error("C-g somehow created a mark")
	}
	if len(e.Echoes) == 0 {
		t.Error("C-g reported nothing when there was no mark to clear")
	}
}

func TestKillTerminalWithNothingModifiedQuitsAtOnce(t *testing.T) {
	e := newEnv("clean")

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.CharPrompts) != 0 {
		t.Errorf("prompted %q with nothing to save", e.CharPrompts)
	}
	if len(e.QuitArgs) != 1 {
		t.Fatalf("Quit called %d times, want 1", len(e.QuitArgs))
	}
}

func TestKillTerminalSavesThenQuits(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("clean")
	bufs := modifiedIn(t, e, dir, "work.txt")
	e.Chars = []rune{'y'}

	mustRun(t, e, "save-buffers-kill-terminal")

	if bufs[0].Modified() {
		t.Error("buffer still modified after answering y")
	}
	if _, err := os.Stat(bufs[0].Path()); err != nil {
		t.Errorf("file was not written: %v", err)
	}
	if len(e.QuitArgs) != 1 {
		t.Errorf("Quit called %d times, want 1", len(e.QuitArgs))
	}
}

// Declining the final confirmation must not quit — this is the last line of
// defence against losing unsaved work.
func TestKillTerminalDecliningFinalConfirmationAborts(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("clean")
	modifiedIn(t, e, dir, "work.txt")
	e.Chars = []rune{'n', 'n'} // don't save, then don't exit

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.QuitArgs) != 0 {
		t.Errorf("Quit was called %v despite declining", e.QuitArgs)
	}
}

func TestKillTerminalConfirmingExitWithUnsavedChangesQuits(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("clean")
	modifiedIn(t, e, dir, "work.txt")
	e.Chars = []rune{'n', 'y'} // don't save, but do exit

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.QuitArgs) != 1 {
		t.Fatalf("Quit called %d times, want 1", len(e.QuitArgs))
	}
	if !e.QuitArgs[0] {
		t.Error("Quit was not forced after the user confirmed")
	}
}

// C-g at the final confirmation must abort the exit, not fall through to it.
func TestKillTerminalCtrlGAtConfirmationAborts(t *testing.T) {
	e := newEnv("unsaved scratch")
	e.Buf().SetModified(true)
	e.Chars = []rune{commandtest.QuitChar}

	if err := runCmd(t, e, "save-buffers-kill-terminal"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if len(e.QuitArgs) != 0 {
		t.Error("C-g at the confirmation still quit the editor")
	}
}

func TestKillTerminalQuitCharAborts(t *testing.T) {
	dir := t.TempDir()
	e := newEnv("clean")
	modifiedIn(t, e, dir, "work.txt")
	e.Chars = []rune{'q'}

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.QuitArgs) != 0 {
		t.Errorf("q at the save prompt still quit: %v", e.QuitArgs)
	}
}

// A modified pathless buffer cannot be saved without a name, so it must still
// trigger the exit confirmation rather than being silently discarded.
func TestKillTerminalConfirmsForModifiedPathlessBuffer(t *testing.T) {
	e := newEnv("unsaved scratch")
	e.Buf().SetModified(true)
	e.Chars = []rune{'n'}

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.CharPrompts) != 1 {
		t.Errorf("char prompts %q, want one exit confirmation", e.CharPrompts)
	}
	if len(e.QuitArgs) != 0 {
		t.Error("declining the confirmation still quit")
	}
}
