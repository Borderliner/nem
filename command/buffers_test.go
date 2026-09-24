package command_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

func newEnv(lines ...string) *commandtest.Fake { return commandtest.New(lines...) }

// lastOpts returns the ReadOpts of the most recent prompt, so completion
// candidates and pre-filled text can be asserted on directly.
func lastOpts(t *testing.T, e *commandtest.Fake) command.ReadOpts {
	t.Helper()
	if len(e.Reads) == 0 {
		t.Fatal("no prompt was issued")
	}
	return e.Reads[len(e.Reads)-1]
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

	if lastOpts(t, e).Complete == nil {
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
	complete := lastOpts(t, e).Complete

	// The input selects the DIRECTORY; it does not narrow within it. Narrowing
	// is the minibuffer's job now, by fuzzy rank, and a CompleteFunc that
	// pre-filtered by prefix would defeat it - typing "al" to reach album.txt is
	// a prefix match, but typing "abm" is not, and only fuzzy finds that.
	got := complete(filepath.Join(dir, "al"))
	want := []string{
		// The directory itself leads, so RET can take it and list it.
		dir + string(filepath.Separator),
		filepath.Join(dir, "album.txt"),
		filepath.Join(dir, "alpha.txt"),
		// A directory keeps its separator so repeated completion can descend
		// into it instead of stalling.
		filepath.Join(dir, "alpine") + string(filepath.Separator),
		filepath.Join(dir, "zeta.txt"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("completing %q gave\n  %q\nwant\n  %q", "al", got, want)
	}
	// A trailing separator means "everything in this directory".
	if got := complete(dir + string(filepath.Separator)); len(got) != 5 {
		t.Errorf("listing the directory completed to %q, want it and all four entries", got)
	}
	if got := complete("/no/such/directory/anywhere/x"); got != nil {
		t.Errorf("unreadable directory returned %q, want nil", got)
	}
}

// Dotfiles come after everything else. With nothing typed RET takes the first
// candidate, and a plain name sort put .git/ there in every repository.
func TestFilenameCompletionListsHiddenEntriesLast(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".env", "Makefile", "main.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{".git", "cmd"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	e := newEnv()
	e.Replies = []string{""}
	mustRun(t, e, "find-file")
	complete := lastOpts(t, e).Complete

	sep := string(filepath.Separator)
	in := dir + sep
	want := []string{
		in,
		in + "Makefile",
		in + "cmd" + sep,
		in + "main.go",
		in + ".env",
		in + ".git" + sep,
	}
	if got := complete(in); !slices.Equal(got, want) {
		t.Errorf("listing gave\n  %q\nwant\n  %q", got, want)
	}
}

// With nothing typed the directory itself is offered as ./, first, and RET
// takes it rather than walking into it: C-x C-f RET lists the working
// directory, as in emacs.
func TestFindFileOffersTheWorkingDirectoryFirst(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	e := newEnv()
	e.Replies = []string{""}
	mustRun(t, e, "find-file")
	opts := lastOpts(t, e)

	sep := string(filepath.Separator)
	got := opts.Complete("")
	if want := []string{"." + sep, "a.txt"}; !slices.Equal(got, want) {
		t.Errorf("completing nothing gave %q, want %q", got, want)
	}
	if opts.Descend("." + sep) {
		t.Error("./ is walked into; RET on it should answer with the directory")
	}
}

// Dired's prompts complete directories only, led by the one typed. A link to a
// directory is a directory for this purpose.
func TestCompleteDirectoryListsOnlyDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"src", ".git"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sep := string(filepath.Separator)
	in := dir + sep
	want := []string{in, in + "src" + sep, in + ".git" + sep}
	if err := os.Symlink(filepath.Join(dir, "src"), filepath.Join(dir, "link")); err == nil {
		want = []string{in, in + "link" + sep, in + "src" + sep, in + ".git" + sep}
	}

	if got := command.CompleteDirectory(in); !slices.Equal(got, want) {
		t.Errorf("CompleteDirectory(%q) =\n  %q\nwant\n  %q", in, got, want)
	}
}

// RET on a directory walks into it at the filename prompts. The hook is judged
// against the candidates completion really returns, where a directory keeps
// its trailing separator, rather than against strings written to suit it.
func TestFilenamePromptsDescendIntoDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sep := string(filepath.Separator)

	for _, tc := range []struct {
		cmd   string
		cands int
	}{
		// find-file also offers the directory itself; write-file cannot write
		// to one.
		{"find-file", 3},
		{"write-file", 2},
	} {
		cmd := tc.cmd
		t.Run(cmd, func(t *testing.T) {
			e := newEnv("body")
			e.Replies = []string{""}
			mustRun(t, e, cmd)

			opts := lastOpts(t, e)
			if opts.Descend == nil {
				t.Fatalf("%s sets no Descend hook, so RET on a directory tries to visit it", cmd)
			}
			cands := opts.Complete(dir + sep)
			if len(cands) != tc.cands {
				t.Fatalf("setup: completion gave %q, want %d candidates", cands, tc.cands)
			}
			for _, c := range cands {
				if got, want := opts.Descend(c), strings.HasSuffix(c, sep); got != want {
					t.Errorf("Descend(%q) = %v, want %v", c, got, want)
				}
			}
		})
	}
}

// A buffer name ending in a slash is only a name. Descending is a filename
// prompt's business, and the other prompts must not acquire it.
func TestSwitchToBufferDoesNotDescend(t *testing.T) {
	e := newEnv("scratch")
	e.Replies = []string{""}
	mustRun(t, e, "switch-to-buffer")
	if lastOpts(t, e).Descend != nil {
		t.Error("switch-to-buffer sets a Descend hook")
	}
}

// --- save-buffer and write-file ------------------------------------------

func TestSaveBufferSavesThroughEnv(t *testing.T) {
	e := newEnv("line one", "line two")
	e.Buf().SetPath("/proj/out.txt")
	e.Buf().SetModified(true)

	mustRun(t, e, "save-buffer")

	if got := savedPaths(e); !slices.Equal(got, []string{"/proj/out.txt"}) {
		t.Errorf("saved %q, want one save of the buffer's own path", got)
	}
	if got := e.Saves[0].Path; got != "" {
		t.Errorf("save-buffer passed path %q, want empty so the buffer keeps its own", got)
	}
	if got := e.Saves[0].Content; got != "line one\nline two" {
		t.Errorf("saved content %q", got)
	}
	if e.Buf().Modified() {
		t.Error("buffer still marked modified after a successful save")
	}
	if len(e.Echoes) == 0 || !strings.Contains(e.Echoes[len(e.Echoes)-1], "/proj/out.txt") {
		t.Errorf("echoes %q, want a message naming the file", e.Echoes)
	}
}

// A pathless buffer must prompt rather than fail with ErrNoPath.
func TestSaveBufferWithNoPathPrompts(t *testing.T) {
	e := newEnv("content")
	e.Buf().SetModified(true)
	e.Replies = []string{"/proj/named.txt"}

	mustRun(t, e, "save-buffer")

	if len(e.Prompts) != 1 {
		t.Fatalf("prompts %q, want exactly one", e.Prompts)
	}
	if got := savedPaths(e); !slices.Equal(got, []string{"/proj/named.txt"}) {
		t.Errorf("saved %q, want the name the user supplied", got)
	}
	if got := e.Buf().Path(); got != "/proj/named.txt" {
		t.Errorf("buffer path is %q, want the new name adopted", got)
	}
	if got := e.Files["/proj/named.txt"]; got != "content" {
		t.Errorf("stored content %q, want %q", got, "content")
	}
}

func TestSaveBufferWithNoPathQuitWritesNothing(t *testing.T) {
	e := newEnv("content")
	e.Buf().SetModified(true)
	e.Replies = []string{commandtest.Quit}

	if err := runCmd(t, e, "save-buffer"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if len(e.Saves) != 0 {
		t.Errorf("C-g still attempted %d save(s)", len(e.Saves))
	}
	if !e.Buf().Modified() {
		t.Error("abandoning the save cleared the modified flag")
	}
}

func TestSaveBufferUnmodifiedDoesNotWrite(t *testing.T) {
	e := newEnv("content")
	e.Buf().SetPath("/proj/untouched.txt")
	e.Buf().SetModified(false)

	mustRun(t, e, "save-buffer")

	if len(e.Saves) != 0 {
		t.Errorf("an unmodified buffer was saved anyway: %q", savedPaths(e))
	}
	if len(e.Echoes) == 0 {
		t.Error("expected a message saying there was nothing to save")
	}
}

// A failed write must leave the buffer marked modified. Clearing the flag would
// tell the user their work is safe when it is not, and they would then close
// the editor and lose it.
func TestSaveBufferFailedWriteKeepsBufferModified(t *testing.T) {
	e := newEnv("precious")
	e.Buf().SetPath("/proj/out.txt")
	e.Buf().SetModified(true)
	wantErr := errors.New("no space left on device")
	e.SaveErr = wantErr

	err := runCmd(t, e, "save-buffer")

	if !errors.Is(err, wantErr) {
		t.Errorf("returned %v, want the write error", err)
	}
	if !e.Buf().Modified() {
		t.Error("a failed save cleared the modified flag — the user would believe this was written")
	}
	if len(e.Saves) != 1 {
		t.Errorf("recorded %d attempts, want 1: attempted and failed, not skipped", len(e.Saves))
	}
	for _, msg := range e.Echoes {
		if strings.Contains(msg, "Wrote") {
			t.Errorf("reported success after a failed write: %q", msg)
		}
	}
}

func TestWriteFileSavesUnderNewPathAndPrefillsCurrent(t *testing.T) {
	e := newEnv("body")
	e.Buf().SetPath("/proj/old.txt")
	e.Replies = []string{"/proj/new.txt"}

	mustRun(t, e, "write-file")

	if got := lastOpts(t, e).Initial; got != "/proj/old.txt" {
		t.Errorf("prompt pre-filled with %q, want the current path", got)
	}
	if got := savedPaths(e); !slices.Equal(got, []string{"/proj/new.txt"}) {
		t.Errorf("saved %q, want only the new path", got)
	}
	if got := e.Files["/proj/new.txt"]; got != "body" {
		t.Errorf("new path holds %q, want %q", got, "body")
	}
	if _, wrote := e.Files["/proj/old.txt"]; wrote {
		t.Error("write-file also wrote the old path")
	}
	if got := e.Buf().Path(); got != "/proj/new.txt" {
		t.Errorf("buffer path is %q, want the new one", got)
	}
}

// The path is adopted only on success. A failed write must not repoint the
// buffer at a file that does not hold its contents.
func TestWriteFileFailedWriteDoesNotAdoptThePath(t *testing.T) {
	e := newEnv("body")
	e.Buf().SetPath("/proj/old.txt")
	wantErr := errors.New("read-only file system")
	e.SaveErr = wantErr
	e.Replies = []string{"/proj/new.txt"}

	err := runCmd(t, e, "write-file")

	if !errors.Is(err, wantErr) {
		t.Errorf("returned %v, want the write error", err)
	}
	if got := e.Buf().Path(); got != "/proj/old.txt" {
		t.Errorf("buffer path became %q after a failed write, want %q", got, "/proj/old.txt")
	}
	if got := savedPaths(e); !slices.Equal(got, []string{"/proj/new.txt"}) {
		t.Errorf("attempts %q, want exactly one at the new path", got)
	}
	if _, wrote := e.Files["/proj/new.txt"]; wrote {
		t.Error("a failed write still stored content at the new path")
	}
}

func TestWriteFileEmptyReplyWritesNothing(t *testing.T) {
	e := newEnv("body")
	e.Replies = []string{""}

	mustRun(t, e, "write-file")

	if len(e.Saves) != 0 {
		t.Errorf("an empty filename still attempted %d save(s)", len(e.Saves))
	}
	if e.Buf().Path() != "" {
		t.Errorf("buffer path became %q, want it unset", e.Buf().Path())
	}
}

func TestWriteFileQuitWritesNothing(t *testing.T) {
	e := newEnv("body")
	e.Replies = []string{commandtest.Quit}

	if err := runCmd(t, e, "write-file"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("returned %v, want ErrQuit", err)
	}
	if len(e.Saves) != 0 {
		t.Errorf("C-g still attempted %d save(s)", len(e.Saves))
	}
}

// RET at the prompt takes the highlighted file, so a new name that fuzzy-
// matches an existing one arrives here as that file. write-file asks before
// replacing it: n and C-g leave it untouched, y writes.
func TestWriteFileAsksBeforeReplacingAnotherFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  rune
		wantErr error
		saved   bool
	}{
		{"no", 'n', nil, false},
		{"quit", commandtest.QuitChar, command.ErrQuit, false},
		{"yes", 'y', nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			mine, other := filepath.Join(dir, "mine.txt"), filepath.Join(dir, "other.txt")
			if err := os.WriteFile(other, []byte("precious"), 0o644); err != nil {
				t.Fatal(err)
			}
			e := newEnv("body")
			e.Buf().SetPath(mine)
			e.Replies = []string{other}
			e.Chars = []rune{tc.answer}

			if err := runCmd(t, e, "write-file"); !errors.Is(err, tc.wantErr) {
				t.Fatalf("returned %v, want %v", err, tc.wantErr)
			}
			if len(e.CharPrompts) != 1 || !strings.Contains(e.CharPrompts[0], "exists") {
				t.Errorf("questions %q, want one asking about the existing file", e.CharPrompts)
			}
			if tc.saved {
				if got := savedPaths(e); !slices.Equal(got, []string{other}) {
					t.Errorf("saved %q, want %q after y", got, other)
				}
				return
			}
			if len(e.Saves) != 0 {
				t.Errorf("wrote %q after %q", savedPaths(e), tc.answer)
			}
			if got := e.Buf().Path(); got != mine {
				t.Errorf("buffer path became %q, want it left at %q", got, mine)
			}
		})
	}
}

// Writing a buffer back to its own file is a save, not a replacement, and a
// path with nothing there has nothing to lose. Neither asks. The fake has no
// answer queued, so a question would fail the command.
func TestWriteFileDoesNotAskWhenNothingIsReplaced(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "mine.txt")
	if err := os.WriteFile(mine, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{mine, filepath.Join(dir, "new.txt")} {
		e := newEnv("body")
		e.Buf().SetPath(mine)
		e.Replies = []string{target}

		mustRun(t, e, "write-file")

		if len(e.CharPrompts) != 0 {
			t.Errorf("writing to %q asked %q", target, e.CharPrompts)
		}
		if got := savedPaths(e); !slices.Equal(got, []string{target}) {
			t.Errorf("saved %q, want %q", got, target)
		}
	}
}

// --- save-some-buffers ---------------------------------------------------

// modified seeds extra buffers that each have a path and unsaved changes.
func modified(t *testing.T, e *commandtest.Fake, names ...string) []*text.Buffer {
	t.Helper()
	var out []*text.Buffer
	for _, n := range names {
		b := e.AddBuffer(n, "body of "+n)
		b.SetPath("/proj/" + n)
		b.SetModified(true)
		out = append(out, b)
	}
	return out
}

// savedPaths lists the target path of every recorded save attempt, resolving
// an empty Path to the buffer's own.
func savedPaths(e *commandtest.Fake) []string {
	var out []string
	for _, s := range e.Saves {
		if s.Path != "" {
			out = append(out, s.Path)
		} else {
			out = append(out, s.Buf.Path())
		}
	}
	return out
}

func TestSaveSomeBuffersBangStopsPrompting(t *testing.T) {
	e := newEnv()
	bufs := modified(t, e, "a.txt", "b.txt", "c.txt")
	e.Chars = []rune{'!'}

	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 1 {
		t.Errorf("prompted %d times, want 1 — ! must stop asking", len(e.CharPrompts))
	}
	if got := len(e.Saves); got != 3 {
		t.Errorf("attempted %d saves, want 3", got)
	}
	for _, b := range bufs {
		if b.Modified() {
			t.Errorf("%s still modified after !", b.Path())
		}
	}
}

func TestSaveSomeBuffersDeclineLeavesBufferModified(t *testing.T) {
	e := newEnv()
	bufs := modified(t, e, "keep.txt")
	e.Chars = []rune{'n'}

	mustRun(t, e, "save-some-buffers")

	if !bufs[0].Modified() {
		t.Error("declining still saved the buffer")
	}
	if len(e.Saves) != 0 {
		t.Errorf("declining still attempted %d save(s)", len(e.Saves))
	}
}

func TestSaveSomeBuffersQuitStopsImmediately(t *testing.T) {
	e := newEnv()
	bufs := modified(t, e, "a.txt", "b.txt")
	e.Chars = []rune{'q'}

	mustRun(t, e, "save-some-buffers")

	if len(e.CharPrompts) != 1 {
		t.Errorf("prompted %d times, want 1 — q must stop the walk", len(e.CharPrompts))
	}
	if len(e.Saves) != 0 {
		t.Errorf("q still attempted %d save(s)", len(e.Saves))
	}
	for _, b := range bufs {
		if !b.Modified() {
			t.Errorf("%s was saved despite q", b.Path())
		}
	}
}

// A write that fails part-way through the walk aborts rather than continuing.
// Carrying on would let the closing summary claim files were saved when they
// were not, and whatever broke the first write will usually break the rest.
func TestSaveSomeBuffersAbortsWhenAWriteFails(t *testing.T) {
	e := newEnv()
	bufs := modified(t, e, "first.txt", "second.txt")
	wantErr := errors.New("no space left on device")
	e.SaveErr = wantErr
	e.Chars = []rune{'!'}

	err := runCmd(t, e, "save-some-buffers")

	if !errors.Is(err, wantErr) {
		t.Fatalf("returned %v, want the write error", err)
	}
	if !strings.Contains(err.Error(), "first.txt") {
		t.Errorf("error %q does not name the buffer that failed", err)
	}
	if got := len(e.Saves); got != 1 {
		t.Errorf("attempted %d saves, want 1 — the walk must stop at the failure", got)
	}
	for _, b := range bufs {
		if !b.Modified() {
			t.Errorf("%s reports unmodified though nothing was written", b.Path())
		}
	}
	for _, msg := range e.Echoes {
		if strings.Contains(msg, "Saved") {
			t.Errorf("claimed success after a failure: %q", msg)
		}
	}
}

// "Saved 1 file", not "Saved 1 files".
func TestSaveSomeBuffersReportsASingleFileInTheSingular(t *testing.T) {
	e := newEnv()
	modified(t, e, "only.txt")
	e.Chars = []rune{'y'}

	mustRun(t, e, "save-some-buffers")

	last := e.Echoes[len(e.Echoes)-1]
	if !strings.Contains(last, "1 file") || strings.Contains(last, "1 files") {
		t.Errorf("echoed %q, want the singular form", last)
	}
}

func TestSaveSomeBuffersReportsSeveralFilesInThePlural(t *testing.T) {
	e := newEnv()
	modified(t, e, "a.txt", "b.txt")
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

	complete := lastOpts(t, e).Complete
	if complete == nil {
		t.Fatal("switch-to-buffer offers no completion")
	}
	// Every buffer, whatever has been typed: the minibuffer narrows by fuzzy
	// rank, so narrowing here too would hide candidates fuzzy could have found.
	// In Buffers() order, which is most recent first, except that the current
	// buffer goes last so RET alone switches away from it.
	want := []string{"notes.md", "nginx.conf", "*scratch*"}
	if got := complete("n"); !slices.Equal(got, want) {
		t.Errorf("completing %q gave %q, want every buffer %q", "n", got, want)
	}
	if got := complete(""); !slices.Equal(got, want) {
		t.Errorf("completing %q gave %q, want every buffer %q", "", got, want)
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
	e.Buf().ActivateMark()
	if !e.Buf().MarkActive() {
		t.Fatal("precondition: region was not active")
	}
	textBefore := e.Text()
	pointBefore := e.Point()

	mustRun(t, e, "keyboard-quit")

	if e.Buf().MarkActive() {
		t.Error("the region is still active after C-g")
	}
	// C-g ends the selection but keeps the mark, so C-x C-x can still return to
	// it. Clearing it would discard a position the user set deliberately.
	if !e.Buf().HasMark() {
		t.Error("C-g discarded the mark instead of only deactivating the region")
	}
	if got, want := e.Buf().Mark(), (text.Pos{Line: 2, Col: 2}); got != want {
		t.Errorf("mark moved to %v, want it kept at %v", got, want)
	}
	if got := e.Text(); got != textBefore {
		t.Errorf("buffer text changed to %q, want %q", got, textBefore)
	}
	if got := e.Point(); got != pointBefore {
		t.Errorf("point moved to %+v, want it left at %+v", got, pointBefore)
	}
}

// A region anchored at the origin is still a region, so C-g must deactivate it.
// This is the case a bare Mark() == Pos{} check gets wrong.
func TestKeyboardQuitDeactivatesARegionAtTheOrigin(t *testing.T) {
	e := newEnv("text")
	e.Buf().SetMark(text.Pos{})
	e.Buf().ActivateMark()
	if !e.Buf().MarkActive() {
		t.Fatal("precondition: a region anchored at the origin should be active")
	}

	mustRun(t, e, "keyboard-quit")

	if e.Buf().MarkActive() {
		t.Error("a region anchored at the origin survived C-g")
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
	e := newEnv("clean")
	bufs := modified(t, e, "work.txt")
	e.Chars = []rune{'y'}

	mustRun(t, e, "save-buffers-kill-terminal")

	if bufs[0].Modified() {
		t.Error("buffer still modified after answering y")
	}
	if got := savedPaths(e); !slices.Equal(got, []string{"/proj/work.txt"}) {
		t.Errorf("saved %q, want the one modified buffer", got)
	}
	if len(e.QuitArgs) != 1 {
		t.Errorf("Quit called %d times, want 1", len(e.QuitArgs))
	}
}

// The last line of defence: a failed save must abort before Quit is reached, so
// the user is never dropped out of the editor having just been told their file
// could not be written.
func TestKillTerminalFailedSaveDoesNotQuit(t *testing.T) {
	e := newEnv("clean")
	bufs := modified(t, e, "work.txt")
	wantErr := errors.New("input/output error")
	e.SaveErr = wantErr
	e.Chars = []rune{'y'}

	err := runCmd(t, e, "save-buffers-kill-terminal")

	if !errors.Is(err, wantErr) {
		t.Errorf("returned %v, want the write error", err)
	}
	if len(e.QuitArgs) != 0 {
		t.Fatalf("the editor quit despite a failed save: %v", e.QuitArgs)
	}
	if !bufs[0].Modified() {
		t.Error("a failed save cleared the modified flag")
	}
}

// Declining the final confirmation must not quit — this is the last guard
// against losing unsaved work.
func TestKillTerminalDecliningFinalConfirmationAborts(t *testing.T) {
	e := newEnv("clean")
	modified(t, e, "work.txt")
	e.Chars = []rune{'n', 'n'} // don't save, then don't exit

	mustRun(t, e, "save-buffers-kill-terminal")

	if len(e.QuitArgs) != 0 {
		t.Errorf("Quit was called %v despite declining", e.QuitArgs)
	}
}

func TestKillTerminalConfirmingExitWithUnsavedChangesQuits(t *testing.T) {
	e := newEnv("clean")
	modified(t, e, "work.txt")
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
	e := newEnv("clean")
	modified(t, e, "work.txt")
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
