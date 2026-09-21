package lua_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/keymap"
	nemlua "github.com/Borderliner/nem/lua"
	"github.com/Borderliner/nem/text"
)

// writeScript puts a config script in a scratch file and returns its path, so
// no test touches a real ~/.config.
func writeScript(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "init.lua")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

// hostOver builds a host whose buffer holds the given lines and whose config
// defines a command named "t" wrapping body. Returning the fake lets a test run
// "t" and then assert on the buffer.
func hostOver(t *testing.T, body string, lines ...string) (*nemlua.Host, *commandtest.Fake) {
	t.Helper()
	f := commandtest.New(lines...)
	h, err := nemlua.New(nemlua.Options{
		Registry:   f.Reg,
		Keymap:     keymap.New(),
		ConfigPath: writeScript(t, "nem.command('t', 'test command', function()\n"+body+"\nend)"),
	})
	if err != nil {
		t.Fatalf("lua.New: %v", err)
	}
	t.Cleanup(h.Close)
	if err := h.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return h, f
}

// runT invokes the wrapped body and requires it to succeed.
func runT(t *testing.T, f *commandtest.Fake) {
	t.Helper()
	if err := f.Reg.Run("t", f); err != nil {
		t.Fatalf("running the script: %v", err)
	}
}

// runTErr invokes the wrapped body and requires it to fail.
func runTErr(t *testing.T, f *commandtest.Fake) error {
	t.Helper()
	err := f.Reg.Run("t", f)
	if err == nil {
		t.Fatal("the script succeeded, want an error")
	}
	return err
}

// --- reading ----------------------------------------------------------------

func TestLineCount(t *testing.T) {
	_, f := hostOver(t, `nem.buf.replace_line("n=" .. nem.buf.line_count())`, "a", "b", "c")
	runT(t, f)
	if got := f.Buf().Line(0).String(); got != "n=3" {
		t.Errorf("line 1 = %q, want %q", got, "n=3")
	}
}

// get_line is 1-based. An off-by-one here is the obvious trap, so it is pinned
// at both ends of the buffer rather than only in the middle.
func TestGetLineIsOneBased(t *testing.T) {
	for _, tc := range []struct{ n, want string }{
		{"1", "alpha"},
		{"2", "beta"},
		{"3", "gamma"},
	} {
		t.Run("line "+tc.n, func(t *testing.T) {
			_, f := hostOver(t, `nem.buf.set_line(1, nem.buf.get_line(`+tc.n+`))`,
				"alpha", "beta", "gamma")
			runT(t, f)
			if got := f.Buf().Line(0).String(); got != tc.want {
				t.Errorf("get_line(%s) wrote %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}

func TestGetLineOutOfRangeErrors(t *testing.T) {
	for _, n := range []string{"0", "4", "-1", "999"} {
		t.Run("line "+n, func(t *testing.T) {
			_, f := hostOver(t, `nem.buf.get_line(`+n+`)`, "a", "b", "c")
			err := runTErr(t, f)
			if !strings.Contains(err.Error(), "out of range") {
				t.Errorf("error = %q, want it to say the line is out of range", err)
			}
			if !strings.Contains(err.Error(), "3 lines") {
				t.Errorf("error = %q, want it to report the buffer's line count", err)
			}
		})
	}
}

// --- writing ----------------------------------------------------------------

func TestSetLine(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_line(2, "CHANGED")`, "a", "b", "c")
	runT(t, f)
	if got, want := f.Text(), "a\nCHANGED\nc"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

func TestSetLineOutOfRangeErrors(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_line(9, "x")`, "a", "b")
	before := f.Text()
	runTErr(t, f)
	if got := f.Text(); got != before {
		t.Errorf("buffer = %q after a failed set_line, want it untouched (%q)", got, before)
	}
}

func TestSetLineRejectsNewline(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_line(1, "a\nb")`, "x")
	err := runTErr(t, f)
	if !strings.Contains(err.Error(), "newline") {
		t.Errorf("error = %q, want it to mention the newline", err)
	}
}

func TestInsertLine(t *testing.T) {
	for _, tc := range []struct {
		name, call, want string
	}{
		{"at the top", `nem.buf.insert_line(1, "new")`, "new\na\nb\nc"},
		{"in the middle", `nem.buf.insert_line(2, "new")`, "a\nnew\nb\nc"},
		{"before the last", `nem.buf.insert_line(3, "new")`, "a\nb\nnew\nc"},
		{"appending past the end", `nem.buf.insert_line(4, "new")`, "a\nb\nc\nnew"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, f := hostOver(t, tc.call, "a", "b", "c")
			runT(t, f)
			if got := f.Text(); got != tc.want {
				t.Errorf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInsertLineOutOfRangeErrors(t *testing.T) {
	for _, n := range []string{"0", "5", "-2"} {
		t.Run("line "+n, func(t *testing.T) {
			_, f := hostOver(t, `nem.buf.insert_line(`+n+`, "x")`, "a", "b", "c")
			before := f.Text()
			runTErr(t, f)
			if got := f.Text(); got != before {
				t.Errorf("buffer = %q, want it untouched", got)
			}
		})
	}
}

func TestRemoveLine(t *testing.T) {
	for _, tc := range []struct {
		name, call, want string
	}{
		{"the first", `nem.buf.remove_line(1)`, "b\nc"},
		{"a middle one", `nem.buf.remove_line(2)`, "a\nc"},
		{"the last", `nem.buf.remove_line(3)`, "a\nb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, f := hostOver(t, tc.call, "a", "b", "c")
			runT(t, f)
			if got := f.Text(); got != tc.want {
				t.Errorf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}

// A buffer always holds at least one line, so removing the only line clears it
// rather than leaving a buffer with none — which would make End() read out of
// range on the next redraw.
func TestRemoveOnlyLineClearsIt(t *testing.T) {
	_, f := hostOver(t, `nem.buf.remove_line(1)`, "only")
	runT(t, f)
	if got := f.Text(); got != "" {
		t.Errorf("buffer = %q, want it empty", got)
	}
	if got := f.Buf().NumLines(); got != 1 {
		t.Errorf("NumLines() = %d, want 1", got)
	}
}

// --- set_text ---------------------------------------------------------------

func TestSetText(t *testing.T) {
	for _, tc := range []struct {
		name, call, want string
	}{
		{"same line count", `nem.buf.set_text("x\ny\nz")`, "x\ny\nz"},
		{"more lines", `nem.buf.set_text("a\nb\nc\nd\ne")`, "a\nb\nc\nd\ne"},
		{"fewer lines", `nem.buf.set_text("only")`, "only"},
		{"emptied", `nem.buf.set_text("")`, ""},
		{"first line only", `nem.buf.set_text("A\nb\nc")`, "A\nb\nc"},
		{"last line only", `nem.buf.set_text("a\nb\nC")`, "a\nb\nC"},
		{"middle line only", `nem.buf.set_text("a\nB\nc")`, "a\nB\nc"},
		{"trailing blank line added", `nem.buf.set_text("a\nb\nc\n")`, "a\nb\nc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, f := hostOver(t, tc.call, "a", "b", "c")
			runT(t, f)
			if got := f.Text(); got != tc.want {
				t.Errorf("buffer = %q, want %q", got, tc.want)
			}
		})
	}
}

// The round trip a formatter hook performs: read the whole buffer, transform the
// string, write it back.
//
// Note the local on the first line. Concatenating the result of a Go-backed
// function and then calling a method on it — (nem.buf.text() .. "\n"):gmatch(…) —
// crashes gopher-lua's VM, so the string has to land in a local first. See
// TestConcatenatedGoResultAsMethodReceiver for the pinned reproduction.
func TestSetTextRoundTripsAWholeBufferTransform(t *testing.T) {
	_, f := hostOver(t, `
	  local src = nem.buf.text() .. "\n"
	  local out = {}
	  for line in src:gmatch("([^\n]*)\n") do
	    out[#out + 1] = (line:gsub("%s+$", ""))
	  end
	  nem.buf.set_text(table.concat(out, "\n"))
	`, "keep   ", "clean", "  both  ")
	runT(t, f)
	if got, want := f.Text(), "keep\nclean\n  both"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}

// An upstream gopher-lua defect, pinned here because it shapes what the config
// documentation can recommend.
//
// Using a Go-backed function's result inside a concatenation, and then calling a
// method on that result, dereferences a nil pointer inside the VM:
//
//	(nem.buf.text() .. "\n"):gmatch("([^\n]*)\n")   -- crashes
//	local s = nem.buf.text() .. "\n"; s:gmatch(…)   -- fine
//	nem.buf.text():gmatch(…)                        -- fine, no concatenation
//
// It reproduces against a bare gopher-lua state with no nem code involved, so
// there is nothing to fix on this side. What matters is that the error boundary
// holds: it surfaces as a script error the editor reports, not as a panic that
// would cost the user their unsaved work.
//
// If this test starts failing because the call now succeeds, upstream has fixed
// it and the workaround note in docs/config.md can go.
func TestConcatenatedGoResultAsMethodReceiver(t *testing.T) {
	_, f := hostOver(t, `
	  local n = 0
	  for line in (nem.buf.text() .. "\n"):gmatch("([^\n]*)\n") do n = n + 1 end
	`, "a", "b")
	err := runTErr(t, f)
	if !strings.Contains(err.Error(), "nil pointer") {
		t.Errorf("error = %q; expected the upstream nil-pointer crash. If upstream fixed this, drop the workaround note in docs/config.md", err)
	}

	// The documented workaround must work.
	_, g := hostOver(t, `
	  local src = nem.buf.text() .. "\n"
	  local n = 0
	  for line in src:gmatch("([^\n]*)\n") do n = n + 1 end
	  nem.buf.set_line(1, "n=" .. n)
	`, "a", "b")
	runT(t, g)
	if got := g.Buf().Line(0).String(); got != "n=2" {
		t.Errorf("workaround wrote %q, want %q", got, "n=2")
	}
}

// Writing back identical text must record nothing: a no-op hook should not mark
// the buffer modified, and should not cost the user an undo step.
//
// Modified() is the proof, and it is exact rather than circumstantial. It is
// derived from the undo log's position against the last saved position, so
// clearing the flag first and finding it still clear afterwards means no undo
// entry was recorded at all — there is no way to record an edit without moving
// that position.
func TestSetTextWithIdenticalTextRecordsNothing(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_text(nem.buf.text())`, "a", "b", "c")
	before := f.Text()
	f.Buf().SetModified(false)
	runT(t, f)
	if f.Buf().Modified() {
		t.Error("Modified() = true after writing back identical text; an edit was recorded")
	}
	if got := f.Text(); got != before {
		t.Errorf("buffer = %q, want it unchanged (%q)", got, before)
	}
}

// --- point survival --------------------------------------------------------

// Point must remain a valid position after a script edits lines, wherever the
// edit lands relative to it.
func TestPointSurvivesEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"line above point removed", `nem.buf.remove_line(1)`},
		{"line below point removed", `nem.buf.remove_line(3)`},
		{"point's own line shortened", `nem.buf.set_line(2, "")`},
		{"line above point inserted", `nem.buf.insert_line(1, "new")`},
		{"buffer shrunk to one line", `nem.buf.set_text("x")`},
		{"buffer emptied", `nem.buf.set_text("")`},
		{"every line removed but one", `nem.buf.remove_line(3) nem.buf.remove_line(1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, f := hostOver(t, tc.body, "alpha", "bravo", "charlie")
			f.SetPoint(text.Pos{Line: 1, Col: 5})
			runT(t, f)

			b, pt := f.Buf(), f.Point()
			if pt.Line < 0 || pt.Line >= b.NumLines() {
				t.Fatalf("point line %d is outside a %d-line buffer", pt.Line, b.NumLines())
			}
			if n := b.Line(pt.Line).Len(); pt.Col < 0 || pt.Col > n {
				t.Errorf("point col %d is outside a line of %d runes", pt.Col, n)
			}
		})
	}
}

// --- undo -------------------------------------------------------------------

// undoStepsToRestore reports how many undo steps return the buffer to before.
func undoStepsToRestore(t *testing.T, b *text.Buffer, before string) int {
	t.Helper()
	for i := 1; i <= 12; i++ {
		if _, ok := b.Undo(); !ok {
			t.Fatalf("undo history ran out after %d steps without restoring %q", i-1, before)
		}
		if b.String() == before {
			return i
		}
	}
	t.Fatalf("more than 12 undo steps without restoring %q", before)
	return 0
}

// A trim is one undo step, because rewriting "foo   " to "foo" is recorded as a
// delete of the trailing runes rather than as a replacement of the line.
func TestTrimmingOneLineIsASingleUndoStep(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_text("a\nb")`, "a", "b   ")
	before := f.Text()
	runT(t, f)
	if got := f.Text(); got != "a\nb" {
		t.Fatalf("buffer = %q, want %q", got, "a\nb")
	}
	if n := undoStepsToRestore(t, f.Buf(), before); n != 1 {
		t.Errorf("restoring took %d undo steps, want 1", n)
	}
}

// Adding lines without changing any existing ones is one step too: there is
// nothing to delete, so only the insert is recorded.
func TestPureInsertionIsASingleUndoStep(t *testing.T) {
	_, f := hostOver(t, `nem.buf.insert_line(2, "new")`, "a", "b")
	before := f.Text()
	runT(t, f)
	if n := undoStepsToRestore(t, f.Buf(), before); n != 1 {
		t.Errorf("restoring took %d undo steps, want 1", n)
	}
}

func TestPureDeletionIsASingleUndoStep(t *testing.T) {
	_, f := hostOver(t, `nem.buf.remove_line(2)`, "a", "b", "c")
	before := f.Text()
	runT(t, f)
	if n := undoStepsToRestore(t, f.Buf(), before); n != 1 {
		t.Errorf("restoring took %d undo steps, want 1", n)
	}
}

// A replacement is two steps — one delete, one insert — because the undo log
// records one entry per Insert/Delete call and has no way to group them. This
// pins the honest number rather than the number we would prefer.
func TestReplacementIsTwoUndoSteps(t *testing.T) {
	_, f := hostOver(t, `nem.buf.set_text("x\ny\nz\nw")`, "a", "b", "c")
	before := f.Text()
	runT(t, f)
	if n := undoStepsToRestore(t, f.Buf(), before); n != 2 {
		t.Errorf("restoring took %d undo steps, want 2 (one delete, one insert)", n)
	}
}

// --- error boundary ---------------------------------------------------------

func TestNewBufFunctionsNeedAnEditorContext(t *testing.T) {
	for _, call := range []string{
		"nem.buf.line_count()",
		"nem.buf.get_line(1)",
		"nem.buf.set_line(1, 'x')",
		"nem.buf.insert_line(1, 'x')",
		"nem.buf.remove_line(1)",
		"nem.buf.set_text('x')",
	} {
		t.Run(call, func(t *testing.T) {
			h, _, _ := newHost(t, call)
			err := h.LoadConfig()
			if err == nil {
				t.Fatalf("%s at load time succeeded, want an error", call)
			}
			if !strings.Contains(err.Error(), "no editor context") {
				t.Errorf("error = %q, want it to explain there is no editor context", err)
			}
		})
	}
}

func TestBadArgumentsAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"get_line with a string", `nem.buf.get_line("two")`},
		{"get_line with nothing", `nem.buf.get_line()`},
		{"set_line with no text", `nem.buf.set_line(1)`},
		{"set_line with a table", `nem.buf.set_line(1, {})`},
		{"insert_line with a nil", `nem.buf.insert_line(nil, "x")`},
		{"remove_line with a bool", `nem.buf.remove_line(true)`},
		{"set_text with a number", `nem.buf.set_text({})`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, f := hostOver(t, tc.body, "a", "b")
			before := f.Text()
			runTErr(t, f)
			if got := f.Text(); got != before {
				t.Errorf("buffer = %q, want it untouched (%q)", got, before)
			}
		})
	}
}

// A script that throws partway through a transform leaves the buffer in whatever
// coherent state it reached — not corrupt, and the host still usable afterwards.
func TestThrowingMidTransformLeavesACoherentBuffer(t *testing.T) {
	_, f := hostOver(t, `
	  nem.buf.set_line(1, "first done")
	  error("stopping here")
	`, "a", "b", "c")
	runTErr(t, f)

	b := f.Buf()
	if got, want := f.Text(), "first done\nb\nc"; got != want {
		t.Errorf("buffer = %q, want %q — the completed edit should stand", got, want)
	}
	if got := b.NumLines(); got != 3 {
		t.Errorf("NumLines() = %d, want 3", got)
	}
	// Still usable: a second run must work.
	runTErr(t, f)
	if got, want := f.Text(), "first done\nb\nc"; got != want {
		t.Errorf("buffer = %q after a second run, want %q", got, want)
	}
}

// --- the documented hook ----------------------------------------------------

// The strip-trailing-whitespace hook docs/config.md publishes must actually run
// and actually strip.
func TestDocumentedStripTrailingWhitespaceHook(t *testing.T) {
	f := commandtest.New("keep   ", "clean", "trailing\t")
	h, err := nemlua.New(nemlua.Options{
		Registry: f.Reg,
		Keymap:   keymap.New(),
		ConfigPath: writeScript(t, `
			nem.hook("before-save", function(buf)
			  for i = 1, nem.buf.line_count() do
			    nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
			  end
			end)
		`),
	})
	if err != nil {
		t.Fatalf("lua.New: %v", err)
	}
	t.Cleanup(h.Close)
	if err := h.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := h.FireHook("before-save", f, f.Buf()); err != nil {
		t.Fatalf("FireHook: %v", err)
	}
	if got, want := f.Text(), "keep\nclean\ntrailing"; got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
}
