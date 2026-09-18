package command_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
)

func noop(command.Env) error { return nil }

func mustRegister(t *testing.T, r *command.Registry, cs ...command.Command) {
	t.Helper()
	for _, c := range cs {
		if err := r.Register(c); err != nil {
			t.Fatalf("Register(%q): unexpected error: %v", c.Name, err)
		}
	}
}

func TestRegisterAndLookup(t *testing.T) {
	r := command.NewRegistry()
	want := command.Command{Name: "forward-char", Doc: "Move right.", Fn: noop, Interactive: true}
	mustRegister(t, r, want)

	got, ok := r.Lookup("forward-char")
	if !ok {
		t.Fatal("Lookup(forward-char): not found after Register")
	}
	if got.Name != want.Name || got.Doc != want.Doc || got.Interactive != want.Interactive {
		t.Errorf("Lookup returned %+v, want name/doc/interactive of %+v", got, want)
	}
	if _, ok := r.Lookup("no-such-command"); ok {
		t.Error("Lookup of an unregistered name reported found")
	}
}

func TestRegisterRejections(t *testing.T) {
	tests := []struct {
		name    string
		pre     []command.Command
		cmd     command.Command
		wantErr error
	}{
		{
			name:    "empty name",
			cmd:     command.Command{Name: "", Fn: noop},
			wantErr: command.ErrEmptyName,
		},
		{
			name:    "nil func",
			cmd:     command.Command{Name: "broken", Fn: nil},
			wantErr: command.ErrNilFunc,
		},
		{
			name:    "duplicate",
			pre:     []command.Command{{Name: "yank", Fn: noop}},
			cmd:     command.Command{Name: "yank", Fn: noop},
			wantErr: command.ErrDuplicateCommand,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := command.NewRegistry()
			mustRegister(t, r, tt.pre...)
			err := r.Register(tt.cmd)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register: error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegisterErrorNamesTheOffender(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r, command.Command{Name: "kill-line", Fn: noop})
	err := r.Register(command.Command{Name: "kill-line", Fn: noop})
	if err == nil {
		t.Fatal("duplicate Register returned nil error")
	}
	// A rejection a Lua author must act on is useless without the name in it.
	if !contains(err.Error(), "kill-line") {
		t.Errorf("error %q does not name the offending command", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestNamesIsSortedAndInteractiveOnly(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r,
		command.Command{Name: "yank", Fn: noop, Interactive: true},
		command.Command{Name: "forward-char", Fn: noop, Interactive: true},
		command.Command{Name: "self-insert-command", Fn: noop, Interactive: false},
		command.Command{Name: "kill-line", Fn: noop, Interactive: true},
	)

	got := r.Names()
	want := []string{"forward-char", "kill-line", "yank"}
	if !slices.Equal(got, want) {
		t.Errorf("Names() = %v, want %v (sorted, interactive only)", got, want)
	}
}

func TestNamesSortednessIsStable(t *testing.T) {
	// Go randomizes map iteration, so an unsorted Names would flake rather
	// than fail outright. Repeat to turn that into a deterministic failure.
	for range 50 {
		r := command.NewRegistry()
		mustRegister(t, r,
			command.Command{Name: "zebra", Fn: noop, Interactive: true},
			command.Command{Name: "alpha", Fn: noop, Interactive: true},
			command.Command{Name: "middle", Fn: noop, Interactive: true},
		)
		if got := r.Names(); !slices.IsSorted(got) {
			t.Fatalf("Names() = %v, not sorted", got)
		}
	}
}

func TestAllReturnsEveryCommand(t *testing.T) {
	r := command.NewRegistry()
	mustRegister(t, r,
		command.Command{Name: "a", Fn: noop, Interactive: true},
		command.Command{Name: "b", Fn: noop, Interactive: false},
	)
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("All() returned %d commands, want 2 (including non-interactive)", len(all))
	}
	names := []string{all[0].Name, all[1].Name}
	slices.Sort(names)
	if !slices.Equal(names, []string{"a", "b"}) {
		t.Errorf("All() names = %v, want [a b]", names)
	}
}

func TestRunInvokesTheCommand(t *testing.T) {
	r := command.NewRegistry()
	ran := 0
	mustRegister(t, r, command.Command{
		Name: "counter",
		Fn:   func(command.Env) error { ran++; return nil },
	})

	if err := r.Run("counter", nil); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if ran != 1 {
		t.Errorf("command ran %d times, want 1", ran)
	}
}

func TestRunPropagatesTheCommandError(t *testing.T) {
	r := command.NewRegistry()
	sentinel := errors.New("boom")
	mustRegister(t, r, command.Command{
		Name: "explode",
		Fn:   func(command.Env) error { return sentinel },
	})
	if err := r.Run("explode", nil); !errors.Is(err, sentinel) {
		t.Errorf("Run returned %v, want %v", err, sentinel)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	r := command.NewRegistry()
	err := r.Run("nope", nil)
	if !errors.Is(err, command.ErrUnknownCommand) {
		t.Fatalf("Run of unknown command returned %v, want ErrUnknownCommand", err)
	}
	if !contains(err.Error(), "nope") {
		t.Errorf("error %q does not name the missing command", err)
	}
}

func TestEmptyRegistry(t *testing.T) {
	r := command.NewRegistry()
	if got := r.Names(); len(got) != 0 {
		t.Errorf("Names() on empty registry = %v, want empty", got)
	}
	if got := r.All(); len(got) != 0 {
		t.Errorf("All() on empty registry = %v, want empty", got)
	}
}

// --- exercising the headless fake --------------------------------------
//
// The fake is what ~60 command tests will depend on, so its own behaviour is
// pinned here rather than taken on trust.

func TestFakeKillAndYankAreReal(t *testing.T) {
	f := commandtest.New("hello", "world")

	// Two consecutive kills must accumulate into one entry, because the fake
	// holds a real KillRing rather than a stub.
	f.KillForward("hello")
	f.KillForward("\nworld")

	got, err := f.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got != "hello\nworld" {
		t.Errorf("Yank() = %q, want %q (consecutive kills must accumulate)", got, "hello\nworld")
	}
	if n := f.Ring().Len(); n != 1 {
		t.Errorf("ring holds %d entries, want 1", n)
	}
}

func TestFakeTextAndPointReflectRealEdits(t *testing.T) {
	f := commandtest.New("abc")
	if got := f.Text(); got != "abc" {
		t.Fatalf("Text() = %q, want %q", got, "abc")
	}
	if err := f.Buf().Insert(text.Pos{Line: 0, Col: 3}, []rune("def")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := f.Text(); got != "abcdef" {
		t.Errorf("Text() after Insert = %q, want %q", got, "abcdef")
	}
	f.SetPoint(text.Pos{Line: 0, Col: 99}) // beyond the line; must clamp
	if got := f.Point(); got.Col != 6 {
		t.Errorf("SetPoint clamped to %v, want Col 6", got)
	}
}

func TestFakeReadStringDrivesOnChangePerKeystroke(t *testing.T) {
	f := commandtest.New("")
	f.Replies = []string{"foo"}

	var seen []string
	got, err := f.ReadString(command.ReadOpts{
		Prompt:   "I-search: ",
		OnChange: func(s string) { seen = append(seen, s) },
	})
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if got != "foo" {
		t.Errorf("ReadString = %q, want %q", got, "foo")
	}
	// Incremental search is only exercised if OnChange fires per keystroke.
	want := []string{"f", "fo", "foo"}
	if !slices.Equal(seen, want) {
		t.Errorf("OnChange saw %v, want %v", seen, want)
	}
	if !slices.Equal(f.Prompts, []string{"I-search: "}) {
		t.Errorf("recorded prompts = %v", f.Prompts)
	}
}

func TestFakeReadStringQuitSentinel(t *testing.T) {
	f := commandtest.New("")
	f.Replies = []string{commandtest.Quit}
	if _, err := f.ReadString(command.ReadOpts{Prompt: "Find file: "}); !errors.Is(err, command.ErrQuit) {
		t.Errorf("ReadString with Quit sentinel returned %v, want command.ErrQuit", err)
	}
}

func TestFakeReadStringWithoutReplyIsATestBugNotAQuit(t *testing.T) {
	f := commandtest.New("")
	_, err := f.ReadString(command.ReadOpts{Prompt: "Find file: "})
	if errors.Is(err, command.ErrQuit) {
		t.Fatal("exhausted replies reported ErrQuit; a test bug must not look like C-g")
	}
	if !errors.Is(err, commandtest.ErrNoReply) {
		t.Errorf("error = %v, want ErrNoReply", err)
	}
}

func TestFakeReadCharRejectsAnswerTheRealPromptWouldRefuse(t *testing.T) {
	f := commandtest.New("")
	f.Chars = []rune{'z'}
	if _, err := f.ReadChar("Replace? (y/n/!/q) ", []rune{'y', 'n', '!', 'q'}); err == nil {
		t.Error("ReadChar accepted 'z', which is not among the valid answers")
	}

	f.Chars = []rune{'y'}
	got, err := f.ReadChar("Replace? ", []rune{'y', 'n'})
	if err != nil || got != 'y' {
		t.Errorf("ReadChar = %q, %v; want 'y', nil", got, err)
	}
}

func TestFakeBufferList(t *testing.T) {
	f := commandtest.New("scratch contents")
	other := f.AddBuffer("notes.md", "a note")

	if n := len(f.Buffers()); n != 2 {
		t.Fatalf("Buffers() = %d, want 2", n)
	}
	if got := f.BufferName(other); got != "notes.md" {
		t.Errorf("BufferName = %q, want notes.md", got)
	}
	if b, ok := f.BufferByName("*scratch*"); !ok || b != f.Buf() {
		t.Error("BufferByName(*scratch*) did not return the active buffer")
	}
	if err := f.KillBuffer(other); err != nil {
		t.Fatalf("KillBuffer: %v", err)
	}
	if err := f.KillBuffer(f.Buf()); !errors.Is(err, commandtest.ErrLastBuffer) {
		t.Errorf("killing the sole buffer returned %v, want ErrLastBuffer", err)
	}
}

func TestFakeOpenFileUsesSeededContentNotDisk(t *testing.T) {
	f := commandtest.New("")
	f.Files["/etc/nonexistent/notes.md"] = "seeded\ncontent"

	b, err := f.OpenFile("/etc/nonexistent/notes.md")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := b.String(); got != "seeded\ncontent" {
		t.Errorf("OpenFile content = %q, want seeded content", got)
	}
	if b.Modified() {
		t.Error("freshly opened buffer reports modified")
	}
	// Reopening the same path must return the same buffer, not a duplicate.
	again, _ := f.OpenFile("/etc/nonexistent/notes.md")
	if again != b {
		t.Error("OpenFile created a second buffer for an already-open path")
	}
	// An unseeded path is a new empty buffer, as find-file does.
	fresh, err := f.OpenFile("/tmp/brand-new.txt")
	if err != nil || fresh.String() != "" {
		t.Errorf("OpenFile of unseeded path = %q, %v; want empty, nil", fresh.String(), err)
	}
}

func TestFakeRecordsWindowAndSessionCalls(t *testing.T) {
	f := commandtest.New("")
	_ = f.SplitWindow(true)
	_ = f.SplitWindow(false)
	f.OtherWindow(-1)
	_ = f.DeleteWindow()
	f.DeleteOtherWindows()
	_ = f.Quit(true)

	if !slices.Equal(f.Splits, []bool{true, false}) {
		t.Errorf("Splits = %v, want [true false]", f.Splits)
	}
	if !slices.Equal(f.OtherWindowArgs, []int{-1}) {
		t.Errorf("OtherWindowArgs = %v, want [-1]", f.OtherWindowArgs)
	}
	if f.DeleteWindowCalls != 1 || f.DeleteOtherWindowsCalls != 1 {
		t.Errorf("window deletion counts = %d, %d; want 1, 1", f.DeleteWindowCalls, f.DeleteOtherWindowsCalls)
	}
	if !slices.Equal(f.QuitArgs, []bool{true}) {
		t.Errorf("QuitArgs = %v, want [true]", f.QuitArgs)
	}
}

func TestFakeRunGoesThroughItsRegistry(t *testing.T) {
	f := commandtest.New("")
	ran := false
	if err := f.Reg.Register(command.Command{
		Name:        "toggle",
		Fn:          func(command.Env) error { ran = true; return nil },
		Interactive: true,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := f.Run("toggle"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran {
		t.Error("Run did not invoke the registered command")
	}
	if !slices.Equal(f.RunNames, []string{"toggle"}) {
		t.Errorf("RunNames = %v, want [toggle]", f.RunNames)
	}
	if !slices.Equal(f.CommandNames(), []string{"toggle"}) {
		t.Errorf("CommandNames = %v, want [toggle]", f.CommandNames())
	}
}

func TestFakeLastCommand(t *testing.T) {
	f := commandtest.New("")
	if f.LastCommand() != "" {
		t.Errorf("LastCommand on a fresh fake = %q, want empty", f.LastCommand())
	}
	f.SetLastCommand("yank")
	if f.LastCommand() != "yank" {
		t.Errorf("LastCommand = %q, want yank", f.LastCommand())
	}
}

func TestFakeSeqPointerIsStable(t *testing.T) {
	f := commandtest.New("")
	if f.Seq() != f.Seq() {
		t.Fatal("Seq() returned a different pointer on a second call")
	}
}

func TestFakeSeqPersistsAcrossDispatches(t *testing.T) {
	// The reason this matters: yank records the extent it inserted, and the
	// NEXT command (yank-pop) must see it. A fake that handed out a fresh Seq
	// per call would make every yank-pop test pass without testing anything.
	f := commandtest.New("hello")

	writer := command.Command{
		Name: "fake-yank",
		Fn: func(e command.Env) error {
			e.Seq().LastYankFrom = text.Pos{Line: 0, Col: 0}
			e.Seq().LastYankTo = text.Pos{Line: 0, Col: 5}
			e.Seq().HasLastYank = true
			e.Seq().RecenterCycle++
			return nil
		},
	}
	var sawFrom, sawTo text.Pos
	var sawFlag bool
	var sawCycle int
	reader := command.Command{
		Name: "fake-yank-pop",
		Fn: func(e command.Env) error {
			sawFrom, sawTo = e.Seq().LastYankFrom, e.Seq().LastYankTo
			sawFlag = e.Seq().HasLastYank
			sawCycle = e.Seq().RecenterCycle
			return nil
		},
	}
	if err := f.Reg.Register(writer); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := f.Reg.Register(reader); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := f.Run("fake-yank"); err != nil {
		t.Fatalf("Run fake-yank: %v", err)
	}
	if err := f.Run("fake-yank-pop"); err != nil {
		t.Fatalf("Run fake-yank-pop: %v", err)
	}

	if !sawFlag {
		t.Error("HasLastYank did not survive into the next dispatch")
	}
	if sawFrom != (text.Pos{Line: 0, Col: 0}) || sawTo != (text.Pos{Line: 0, Col: 5}) {
		t.Errorf("yank extent seen as %v..%v, want {0,0}..{0,5}", sawFrom, sawTo)
	}
	if sawCycle != 1 {
		t.Errorf("RecenterCycle seen as %d, want 1", sawCycle)
	}

	// And mutation through Env is visible on the fake itself.
	if got := f.Seq().RecenterCycle; got != 1 {
		t.Errorf("f.Seq().RecenterCycle = %d, want 1", got)
	}
}

func TestFakeWhereFindsEveryBinding(t *testing.T) {
	f := commandtest.New("")
	f.BindingMap = map[string]string{
		"C-_":   "undo",
		"C-x u": "undo",
		"C-y":   "yank",
	}
	if got := f.Where("undo"); !slices.Equal(got, []string{"C-_", "C-x u"}) {
		t.Errorf("Where(undo) = %v, want [C-_ C-x u] sorted", got)
	}
	if got := f.Where("no-such-command"); got != nil {
		t.Errorf("Where of unbound command = %v, want nil", got)
	}
}

// --- SaveBuffer and full ReadOpts recording -----------------------------

func TestFakeSaveBufferRecordsWhatWouldHitDisk(t *testing.T) {
	f := commandtest.New("hello")
	b := f.Buf()
	b.SetPath("/notes.md")
	if err := b.Insert(text.Pos{Line: 0, Col: 5}, []rune(" world")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !b.Modified() {
		t.Fatal("buffer not modified after Insert")
	}

	if err := f.SaveBuffer(b, ""); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if len(f.Saves) != 1 {
		t.Fatalf("Saves = %d, want 1", len(f.Saves))
	}
	got := f.Saves[0]
	if got.Buf != b || got.Path != "" || got.Content != "hello world" {
		t.Errorf("Saves[0] = %+v, want buf, empty path, %q", got, "hello world")
	}
	if b.Modified() {
		t.Error("buffer still reports modified after a successful save")
	}
}

func TestFakeSaveBufferWithPathAdoptsIt(t *testing.T) {
	f := commandtest.New("contents")
	b := f.Buf()

	if err := f.SaveBuffer(b, "/tmp/adopted.txt"); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	if b.Path() != "/tmp/adopted.txt" {
		t.Errorf("Path() = %q, want the adopted path (this is write-file)", b.Path())
	}
	if got := f.BufferName(b); got != "adopted.txt" {
		t.Errorf("BufferName = %q, want adopted.txt", got)
	}
}

func TestFakeSaveBufferWithoutAnyPathFails(t *testing.T) {
	f := commandtest.New("unnamed")
	if err := f.SaveBuffer(f.Buf(), ""); !errors.Is(err, text.ErrNoPath) {
		t.Errorf("SaveBuffer of a path-less buffer = %v, want text.ErrNoPath", err)
	}
	// Even a failed save is recorded: a test asserting "no save was attempted"
	// must be able to tell that apart from "a save was attempted and failed".
	if len(f.Saves) != 1 {
		t.Errorf("Saves = %d, want the attempt recorded", len(f.Saves))
	}
}

func TestFakeSaveErrIsOneShot(t *testing.T) {
	f := commandtest.New("data")
	b := f.Buf()
	b.SetPath("/x.txt")
	if err := b.Insert(text.Pos{Line: 0, Col: 4}, []rune("!")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if !b.Modified() {
		t.Fatal("precondition: buffer should be modified before the save")
	}

	boom := errors.New("disk full")
	f.SaveErr = boom

	if err := f.SaveBuffer(b, ""); !errors.Is(err, boom) {
		t.Fatalf("first save = %v, want the injected error", err)
	}
	if !b.Modified() {
		t.Error("buffer marked unmodified despite a failed save — that would lose the user's work silently")
	}
	// Cleared, so a command that retries can succeed.
	if err := f.SaveBuffer(b, ""); err != nil {
		t.Errorf("second save = %v, want nil (SaveErr must be one-shot)", err)
	}
}

func TestFakeSaveNeverTouchesTheFileSystemAndStaysSelfConsistent(t *testing.T) {
	f := commandtest.New("payload")
	b := f.Buf()

	// A path no test process could write to. A real save would fail here; the
	// fake must not care, because it never goes near the disk.
	const path = "/proc/definitely/not/writable/x.txt"
	if err := f.SaveBuffer(b, path); err != nil {
		t.Fatalf("SaveBuffer to an unwritable path = %v, want nil (fake must not touch disk)", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the fake created a real file")
	}

	// Saving then reopening the same path must agree.
	reopened, err := f.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if reopened.String() != "payload" {
		t.Errorf("reopened content = %q, want %q", reopened.String(), "payload")
	}
}

func TestFakeRecordsFullReadOptsIncludingCompletionCandidates(t *testing.T) {
	// This is the case that drove recording ReadOpts rather than just the
	// prompt: asserting that switch-to-buffer offers the right candidates.
	f := commandtest.New("scratch")
	f.AddBuffer("notes.md")
	f.AddBuffer("nem.go")
	f.Replies = []string{"notes.md"}

	// A stand-in for switch-to-buffer's prompt.
	err := f.Reg.Register(command.Command{
		Name: "fake-switch-to-buffer",
		Fn: func(e command.Env) error {
			_, err := e.ReadString(command.ReadOpts{
				Prompt:  "Switch to buffer: ",
				Initial: "*scratch*",
				Complete: func(prefix string) []string {
					var out []string
					for _, b := range e.Buffers() {
						if name := e.BufferName(b); strings.HasPrefix(name, prefix) {
							out = append(out, name)
						}
					}
					sort.Strings(out)
					return out
				},
			})
			return err
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := f.Run("fake-switch-to-buffer"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(f.Reads) != 1 {
		t.Fatalf("Reads = %d, want 1", len(f.Reads))
	}
	opts := f.Reads[0]
	if opts.Prompt != "Switch to buffer: " {
		t.Errorf("Prompt = %q", opts.Prompt)
	}
	if opts.Initial != "*scratch*" {
		t.Errorf("Initial = %q, want *scratch* (pre-fill is now assertable)", opts.Initial)
	}
	if opts.Complete == nil {
		t.Fatal("Complete was not recorded")
	}
	if got := opts.Complete("n"); !slices.Equal(got, []string{"nem.go", "notes.md"}) {
		t.Errorf("candidates for %q = %v, want [nem.go notes.md]", "n", got)
	}
	if got := opts.Complete("notes"); !slices.Equal(got, []string{"notes.md"}) {
		t.Errorf("candidates for %q = %v, want [notes.md]", "notes", got)
	}
	// The prompt-only recorder still works, since many tests only count.
	if !slices.Equal(f.Prompts, []string{"Switch to buffer: "}) {
		t.Errorf("Prompts = %v", f.Prompts)
	}
}

// --- Seq.LastRune and the shared boundary conditions --------------------

func TestFakeSeqLastRuneDrivesSelfInsert(t *testing.T) {
	// self-insert-command is the one command whose behaviour depends on which
	// key ran it. Seq.LastRune carries that, so it can be an ordinary
	// registered command rather than something only the event loop can call.
	f := commandtest.New("")

	err := f.Reg.Register(command.Command{
		Name:        "fake-self-insert",
		Interactive: true,
		Fn: func(e command.Env) error {
			r := e.Seq().LastRune
			if r == 0 {
				return errors.New("no triggering rune")
			}
			n, _ := e.Arg()
			w := e.Win()
			for range n {
				if err := w.Buf.Insert(w.Pt, []rune{r}); err != nil {
					return err
				}
				w.Pt.Col++
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The event loop's job, emulated: set the rune, then dispatch.
	f.Seq().LastRune = 'x'
	f.ArgN = 3
	if err := f.Run("fake-self-insert"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.Text(); got != "xxx" {
		t.Errorf("Text() = %q, want %q (C-u 3 x)", got, "xxx")
	}

	// A different key next time inserts that key, not the previous one.
	f.Seq().LastRune = 'y'
	f.ArgN = 1
	if err := f.Run("fake-self-insert"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := f.Text(); got != "xxxy" {
		t.Errorf("Text() = %q, want %q", got, "xxxy")
	}

	// And it is reachable by name, which is the point: M-x and Lua can bind it.
	if !slices.Contains(f.CommandNames(), "fake-self-insert") {
		t.Error("a self-inserting command is not reachable from M-x")
	}
}

func TestBoundaryConditionsAreDistinctAndMatchable(t *testing.T) {
	// These exist so the dispatcher can tell a harmless boundary from a real
	// failure via errors.Is, including through a wrap.
	if errors.Is(command.ErrEndOfBuffer, command.ErrBeginningOfBuffer) {
		t.Error("the two boundary conditions are not distinct")
	}
	wrapped := fmt.Errorf("next-line: %w", command.ErrEndOfBuffer)
	if !errors.Is(wrapped, command.ErrEndOfBuffer) {
		t.Error("ErrEndOfBuffer does not survive wrapping")
	}
	if errors.Is(wrapped, command.ErrQuit) {
		t.Error("a boundary condition matched ErrQuit")
	}
}
