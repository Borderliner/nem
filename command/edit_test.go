// Tests live in command_test rather than command because they use
// commandtest.Fake, and commandtest imports command: an in-package test file
// importing it would be an import cycle.
package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
)

// editReg returns a registry holding only the editing commands.
func editReg(t *testing.T) *command.Registry {
	t.Helper()
	r := command.NewRegistry()
	if err := command.RegisterEdit(r); err != nil {
		t.Fatalf("RegisterEdit: %v", err)
	}
	return r
}

// run dispatches name against f the way the event loop would, by name.
func edRun(t *testing.T, f *commandtest.Fake, name string) error {
	t.Helper()
	return editReg(t).Run(name, f)
}

// mustRun fails the test if the command reports an error.
func edMustRun(t *testing.T, f *commandtest.Fake, name string) {
	t.Helper()
	if err := edRun(t, f, name); err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
}

func edAt(line int, col text.RuneIdx) text.Pos {
	return text.Pos{Line: line, Col: col}
}

func edWantText(t *testing.T, f *commandtest.Fake, want string) {
	t.Helper()
	if got := f.Text(); got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func edWantPoint(t *testing.T, f *commandtest.Fake, want text.Pos) {
	t.Helper()
	if got := f.Point(); !got.Equal(want) {
		t.Errorf("point = %d:%d, want %d:%d", got.Line, got.Col, want.Line, want.Col)
	}
}

func edWantYank(t *testing.T, f *commandtest.Fake, want string) {
	t.Helper()
	got, err := f.Yank()
	if err != nil {
		t.Fatalf("Yank: %v", err)
	}
	if got != want {
		t.Errorf("yank = %q, want %q", got, want)
	}
}

// edType drives self-insert-command the way the event loop does: record the
// rune that triggered the command, then dispatch by name.
func edType(t *testing.T, f *commandtest.Fake, r rune) {
	t.Helper()
	f.Seq().LastRune = r
	edMustRun(t, f, "self-insert-command")
}

// arg sets the prefix argument as C-u would.
func edArg(f *commandtest.Fake, n int) {
	f.ArgN = n
	f.ArgExplicit = true
}

// --- registration ----------------------------------------------------------

func TestRegisterEditRegistersEveryCommand(t *testing.T) {
	r := editReg(t)
	for _, name := range []string{
		"delete-char", "delete-backward-char", "kill-word", "backward-kill-word",
		"kill-line", "open-line", "transpose-chars", "transpose-words", "newline",
		"indent-for-tab-command", "upcase-word", "downcase-word", "capitalize-word",
		"self-insert-command",
	} {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("%s is not registered", name)
		}
	}
}

func TestSelfInsertIsInteractive(t *testing.T) {
	// It reads its rune from Seq, so it is an ordinary command rather than a
	// stub only the event loop can reach.
	var found bool
	for _, name := range editReg(t).Names() {
		if name == "self-insert-command" {
			found = true
		}
	}
	if !found {
		t.Error("self-insert-command is missing from M-x completion")
	}
}

func TestRegisterEditTwiceIsAnError(t *testing.T) {
	r := editReg(t)
	if err := command.RegisterEdit(r); !errors.Is(err, command.ErrDuplicateCommand) {
		t.Errorf("second RegisterEdit error = %v, want ErrDuplicateCommand", err)
	}
}

// --- kill-line: the three cases -------------------------------------------

func TestKillLineToEndOfLineLeavesTheNewline(t *testing.T) {
	f := commandtest.New("hello", "world")
	edMustRun(t, f, "kill-line")
	edWantText(t, f, "\nworld")
	edWantPoint(t, f, edAt(0, 0))
	edWantYank(t, f, "hello")
}

func TestKillLineOnEmptyLineKillsTheNewline(t *testing.T) {
	f := commandtest.New("hello", "world")
	edMustRun(t, f, "kill-line") // kills "hello"
	edMustRun(t, f, "kill-line") // now at an empty line: kills the newline
	edWantText(t, f, "world")
	edWantPoint(t, f, edAt(0, 0))
	// Both kills accumulated into one entry.
	edWantYank(t, f, "hello\n")
	if n := f.Ring().Len(); n != 1 {
		t.Errorf("ring holds %d entries, want 1", n)
	}
}

func TestKillLineWithArgKillsWholeLinesIncludingNewlines(t *testing.T) {
	f := commandtest.New("a", "b", "c")
	edArg(f, 2)
	edMustRun(t, f, "kill-line")
	edWantText(t, f, "c")
	edWantYank(t, f, "a\nb\n")
}

func TestKillLineWithArgPastEndStopsAtBufferEnd(t *testing.T) {
	f := commandtest.New("a", "b")
	edArg(f, 99)
	edMustRun(t, f, "kill-line")
	edWantText(t, f, "")
	edWantYank(t, f, "a\nb")
}

func TestKillLineWithZeroArgKillsToStartOfLine(t *testing.T) {
	f := commandtest.New("hello")
	f.SetPoint(edAt(0, 3))
	edArg(f, 0)
	edMustRun(t, f, "kill-line")
	edWantText(t, f, "lo")
	edWantPoint(t, f, edAt(0, 0))
	edWantYank(t, f, "hel")
}

func TestKillLineAtEndOfBufferErrors(t *testing.T) {
	f := commandtest.New("")
	if err := edRun(t, f, "kill-line"); !errors.Is(err, command.ErrEndOfBuffer) {
		t.Errorf("kill-line at end of buffer: error = %v, want ErrEndOfBuffer", err)
	}
}

// --- kill accumulation -----------------------------------------------------

func TestThreeConsecutiveKillLinesAccumulateIntoOneEntry(t *testing.T) {
	// Three presses kill "a", then the newline, then "b" — accumulated as a
	// single block. This is the behaviour the ring exists to guarantee.
	f := commandtest.New("a", "b", "c")
	for i := 0; i < 3; i++ {
		edMustRun(t, f, "kill-line")
	}
	edWantText(t, f, "\nc")
	edWantYank(t, f, "a\nb")
	if n := f.Ring().Len(); n != 1 {
		t.Errorf("ring holds %d entries, want 1", n)
	}
}

func TestSixKillLinesYankBackAllThreeLinesAsOneBlock(t *testing.T) {
	// Two presses consume a line and its newline, so three lines take six.
	f := commandtest.New("a", "b", "c")
	for i := 0; i < 6; i++ {
		if err := edRun(t, f, "kill-line"); err != nil {
			break // the last newline does not exist; stop cleanly
		}
	}
	edWantText(t, f, "")
	edWantYank(t, f, "a\nb\nc")
}

func TestKillThenMoveThenKillMakesTwoEntries(t *testing.T) {
	f := commandtest.New("aaa", "bbb")
	edMustRun(t, f, "kill-line")
	f.Ring().BreakRun() // what dispatch does for any non-kill command
	f.SetPoint(edAt(1, 0))
	edMustRun(t, f, "kill-line")
	if n := f.Ring().Len(); n != 2 {
		t.Errorf("ring holds %d entries, want 2", n)
	}
	edWantYank(t, f, "bbb")
}

// --- kill-word / backward-kill-word ---------------------------------------

func TestKillWord(t *testing.T) {
	f := commandtest.New("foo bar")
	edMustRun(t, f, "kill-word")
	edWantText(t, f, " bar")
	edWantPoint(t, f, edAt(0, 0))
	edWantYank(t, f, "foo")
}

func TestKillWordWithArg(t *testing.T) {
	f := commandtest.New("foo bar baz")
	edArg(f, 2)
	edMustRun(t, f, "kill-word")
	edWantText(t, f, " baz")
	edWantYank(t, f, "foo bar")
}

func TestBackwardKillsPrependIntoReadingOrder(t *testing.T) {
	// The regression that matters: getting the direction backwards silently
	// reverses backward word kills, yielding "bar foo".
	f := commandtest.New("foo bar")
	f.SetPoint(edAt(0, 7))
	edMustRun(t, f, "backward-kill-word")
	edWantText(t, f, "foo ")
	edMustRun(t, f, "backward-kill-word")
	edWantText(t, f, "")
	edWantYank(t, f, "foo bar")
}

func TestKillWordCrossesLines(t *testing.T) {
	f := commandtest.New("foo", "bar")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "kill-word")
	edWantText(t, f, "foo")
	edWantYank(t, f, "\nbar")
}

func TestKillWordWithNegativeArgKillsBackward(t *testing.T) {
	f := commandtest.New("foo bar")
	f.SetPoint(edAt(0, 7))
	edArg(f, -1)
	edMustRun(t, f, "kill-word")
	edWantText(t, f, "foo ")
	edWantYank(t, f, "bar")
}

func TestBackwardKillWordAtBufferStartErrors(t *testing.T) {
	f := commandtest.New("foo")
	if err := edRun(t, f, "backward-kill-word"); !errors.Is(err, command.ErrBeginningOfBuffer) {
		t.Errorf("backward-kill-word at buffer start: error = %v, want ErrBeginningOfBuffer", err)
	}
}

// --- delete-char / delete-backward-char: graphemes, not runes -------------

func TestDeleteCharDeletesOneGrapheme(t *testing.T) {
	// "e" + U+0301 is two runes and one grapheme: it goes in one press.
	f := commandtest.New("éx")
	edMustRun(t, f, "delete-char")
	edWantText(t, f, "x")
	edWantPoint(t, f, edAt(0, 0))
}

func TestDeleteBackwardCharDeletesOneGrapheme(t *testing.T) {
	f := commandtest.New("éx")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "delete-backward-char")
	edWantText(t, f, "é")
	edMustRun(t, f, "delete-backward-char")
	edWantText(t, f, "")
	edWantPoint(t, f, edAt(0, 0))
}

func TestDeleteCharDoesNotTouchTheKillRing(t *testing.T) {
	// Emacs deletes small amounts of text without disturbing the ring.
	f := commandtest.New("abc")
	edMustRun(t, f, "delete-char")
	if n := f.Ring().Len(); n != 0 {
		t.Errorf("ring holds %d entries after delete-char, want 0", n)
	}
}

func TestDeleteCharWithArg(t *testing.T) {
	f := commandtest.New("abcdef")
	edArg(f, 3)
	edMustRun(t, f, "delete-char")
	edWantText(t, f, "def")
}

func TestDeleteCharWithNegativeArgDeletesBackward(t *testing.T) {
	f := commandtest.New("abc")
	f.SetPoint(edAt(0, 2))
	edArg(f, -1)
	edMustRun(t, f, "delete-char")
	edWantText(t, f, "ac")
	edWantPoint(t, f, edAt(0, 1))
}

func TestDeleteBackwardCharWithNegativeArgDeletesForward(t *testing.T) {
	f := commandtest.New("abc")
	f.SetPoint(edAt(0, 1))
	edArg(f, -1)
	edMustRun(t, f, "delete-backward-char")
	edWantText(t, f, "ac")
}

func TestDeleteCharJoinsLines(t *testing.T) {
	f := commandtest.New("ab", "cd")
	f.SetPoint(edAt(0, 2))
	edMustRun(t, f, "delete-char")
	edWantText(t, f, "abcd")
}

func TestDeleteCharAtEndOfBufferErrors(t *testing.T) {
	// Reported as the exported sentinel, so the dispatcher can tell a harmless
	// boundary from a genuine failure via errors.Is.
	f := commandtest.New("")
	if err := edRun(t, f, "delete-char"); !errors.Is(err, command.ErrEndOfBuffer) {
		t.Errorf("delete-char at end of buffer: error = %v, want ErrEndOfBuffer", err)
	}
}

func TestDeleteBackwardCharAtBufferStartErrors(t *testing.T) {
	f := commandtest.New("abc")
	if err := edRun(t, f, "delete-backward-char"); !errors.Is(err, command.ErrBeginningOfBuffer) {
		t.Errorf("delete-backward-char at buffer start: error = %v, want ErrBeginningOfBuffer", err)
	}
}

// --- newline and auto-indent ----------------------------------------------

func TestNewlineCopiesIndentation(t *testing.T) {
	f := commandtest.New("    foo")
	f.SetPoint(edAt(0, 7))
	edMustRun(t, f, "newline")
	edWantText(t, f, "    foo\n    ")
	edWantPoint(t, f, edAt(1, 4))
}

func TestNewlineCopiesIndentationVerbatim(t *testing.T) {
	// Tabs stay tabs; indentation is not normalised to spaces or vice versa.
	f := commandtest.New("\t\tfoo")
	f.SetPoint(edAt(0, 5))
	edMustRun(t, f, "newline")
	edWantText(t, f, "\t\tfoo\n\t\t")

	g := commandtest.New(" \tfoo")
	g.SetPoint(edAt(0, 5))
	edMustRun(t, g, "newline")
	edWantText(t, g, " \tfoo\n \t")
}

func TestNewlineOnAllWhitespaceLine(t *testing.T) {
	// The whole line is leading whitespace, so the whole line is the indent.
	f := commandtest.New("    ")
	f.SetPoint(edAt(0, 4))
	edMustRun(t, f, "newline")
	edWantText(t, f, "    \n    ")
	edWantPoint(t, f, edAt(1, 4))
}

func TestNewlineWithPointInsideLeadingWhitespace(t *testing.T) {
	// Indentation is measured from the start of the line, not from point, so
	// splitting inside the indentation still reproduces it below.
	f := commandtest.New("    foo")
	f.SetPoint(edAt(0, 2))
	edMustRun(t, f, "newline")
	edWantText(t, f, "  \n      foo")
	edWantPoint(t, f, edAt(1, 4))
}

func TestNewlineWithNoIndentation(t *testing.T) {
	f := commandtest.New("foo")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "newline")
	edWantText(t, f, "foo\n")
	edWantPoint(t, f, edAt(1, 0))
}

func TestNewlineWithArgInsertsSeveral(t *testing.T) {
	f := commandtest.New("  x")
	f.SetPoint(edAt(0, 3))
	edArg(f, 2)
	edMustRun(t, f, "newline")
	edWantText(t, f, "  x\n  \n  ")
	edWantPoint(t, f, edAt(2, 2))
}

// --- self-insert-command --------------------------------------------------

func TestSelfInsertDispatchesThroughTheRegistry(t *testing.T) {
	f := commandtest.New("")
	edType(t, f, 'x')
	edWantText(t, f, "x")
	edWantPoint(t, f, edAt(0, 1))
}

func TestSelfInsertWithoutARecordedRuneIsAnError(t *testing.T) {
	// Seq().LastRune is zero, so no key triggered this: inserting NUL would be
	// worse than refusing.
	f := commandtest.New("")
	if err := edRun(t, f, "self-insert-command"); err == nil {
		t.Error("self-insert-command with no recorded rune: want an error, got nil")
	}
	edWantText(t, f, "")
}

func TestSelfInsertHonoursArg(t *testing.T) {
	f := commandtest.New("")
	edArg(f, 3)
	edType(t, f, 'z')
	edWantText(t, f, "zzz")
	edWantPoint(t, f, edAt(0, 3))

	g := commandtest.New("")
	edArg(g, 40)
	edType(t, g, '-')
	edWantText(t, g, strings.Repeat("-", 40))
	edWantPoint(t, g, edAt(0, 40))
}

func TestSelfInsertWithArgIsOneUndoUnit(t *testing.T) {
	f := commandtest.New("")
	edArg(f, 40)
	edType(t, f, '-')
	if _, ok := f.Buf().Undo(); !ok {
		t.Fatal("Undo reported nothing to undo")
	}
	edWantText(t, f, "")
}

func TestSelfInsertCoalescesTypingIntoOneUndoUnit(t *testing.T) {
	f := commandtest.New("")
	for _, r := range "hello" {
		edType(t, f, r)
	}
	edWantText(t, f, "hello")
	if _, ok := f.Buf().Undo(); !ok {
		t.Fatal("Undo reported nothing to undo")
	}
	edWantText(t, f, "")
}

func TestSelfInsertNewlineBreaksTheUndoRun(t *testing.T) {
	f := commandtest.New("")
	for _, r := range "a\nb" {
		edType(t, f, r)
	}
	edWantText(t, f, "a\nb")
	if _, ok := f.Buf().Undo(); !ok {
		t.Fatal("Undo reported nothing to undo")
	}
	// Only the "b" comes back off: the newline ended the run.
	edWantText(t, f, "a\n")
}

func TestSelfInsertWideRune(t *testing.T) {
	f := commandtest.New("")
	edType(t, f, '日')
	edWantText(t, f, "日")
	edWantPoint(t, f, edAt(0, 1)) // one rune, whatever its display width
}

// --- open-line -------------------------------------------------------------

func TestOpenLineLeavesPointInPlace(t *testing.T) {
	f := commandtest.New("ab")
	f.SetPoint(edAt(0, 1))
	edMustRun(t, f, "open-line")
	edWantText(t, f, "a\nb")
	edWantPoint(t, f, edAt(0, 1))
}

func TestOpenLineWithArg(t *testing.T) {
	f := commandtest.New("ab")
	f.SetPoint(edAt(0, 1))
	edArg(f, 2)
	edMustRun(t, f, "open-line")
	edWantText(t, f, "a\n\nb")
	edWantPoint(t, f, edAt(0, 1))
}

// --- indent-for-tab-command ----------------------------------------------

func TestIndentForTabInsertsATab(t *testing.T) {
	f := commandtest.New("")
	edMustRun(t, f, "indent-for-tab-command")
	edWantText(t, f, "\t")
	edWantPoint(t, f, edAt(0, 1))
}

func TestIndentForTabWithArg(t *testing.T) {
	f := commandtest.New("")
	edArg(f, 3)
	edMustRun(t, f, "indent-for-tab-command")
	edWantText(t, f, "\t\t\t")
}

// --- transpose-chars ------------------------------------------------------

func TestTransposeCharsMidLine(t *testing.T) {
	f := commandtest.New("abc")
	f.SetPoint(edAt(0, 1))
	edMustRun(t, f, "transpose-chars")
	edWantText(t, f, "bac")
	edWantPoint(t, f, edAt(0, 2))
}

func TestTransposeCharsAtEndOfLineUsesThePrecedingTwo(t *testing.T) {
	// Emacs does not error here, and this is what makes C-t useful mid-typing.
	f := commandtest.New("abc")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "transpose-chars")
	edWantText(t, f, "acb")
	edWantPoint(t, f, edAt(0, 3))
}

func TestTransposeCharsWithWideGlyphs(t *testing.T) {
	f := commandtest.New("日本")
	f.SetPoint(edAt(0, 2))
	edMustRun(t, f, "transpose-chars")
	edWantText(t, f, "本日")
}

func TestTransposeCharsWithCombiningMark(t *testing.T) {
	// The grapheme moves as a unit rather than being split from its mark.
	f := commandtest.New("éx")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "transpose-chars")
	edWantText(t, f, "xé")
}

func TestTransposeCharsAtLineStartErrors(t *testing.T) {
	f := commandtest.New("abc")
	if err := edRun(t, f, "transpose-chars"); err == nil {
		t.Error("transpose-chars at line start: want an error, got nil")
	}
}

func TestTransposeCharsWithOneCharacterErrors(t *testing.T) {
	f := commandtest.New("a")
	f.SetPoint(edAt(0, 1))
	if err := edRun(t, f, "transpose-chars"); err == nil {
		t.Error("transpose-chars with one character: want an error, got nil")
	}
}

// --- transpose-words ------------------------------------------------------

func TestTransposeWords(t *testing.T) {
	f := commandtest.New("foo bar")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "transpose-words")
	edWantText(t, f, "bar foo")
	edWantPoint(t, f, edAt(0, 7))
}

func TestTransposeWordsAcrossANewline(t *testing.T) {
	f := commandtest.New("foo", "bar")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "transpose-words")
	edWantText(t, f, "bar\nfoo")
	edWantPoint(t, f, edAt(1, 3))
}

func TestTransposeWordsPreservesWhatLiesBetween(t *testing.T) {
	f := commandtest.New("foo   ,  bar")
	f.SetPoint(edAt(0, 3))
	edMustRun(t, f, "transpose-words")
	edWantText(t, f, "bar   ,  foo")
}

func TestTransposeWordsWithOnlyOneWordErrors(t *testing.T) {
	f := commandtest.New("foo")
	f.SetPoint(edAt(0, 3))
	if err := edRun(t, f, "transpose-words"); err == nil {
		t.Error("transpose-words with one word: want an error, got nil")
	}
}

// --- case commands --------------------------------------------------------

func TestUpcaseWord(t *testing.T) {
	f := commandtest.New("hello world")
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "HELLO world")
	edWantPoint(t, f, edAt(0, 5))
}

func TestDowncaseWord(t *testing.T) {
	f := commandtest.New("HELLO WORLD")
	edMustRun(t, f, "downcase-word")
	edWantText(t, f, "hello WORLD")
	edWantPoint(t, f, edAt(0, 5))
}

func TestCapitalizeWordCases(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"lower", "hello there", "Hello there"},
		{"already capitalized", "Hello there", "Hello there"},
		{"all caps", "HELLO there", "Hello there"},
		{"mixed", "hELLO there", "Hello there"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := commandtest.New(tc.in)
			edMustRun(t, f, "capitalize-word")
			edWantText(t, f, tc.want)
		})
	}
}

func TestCaseCommandsActOnTheRemainderFromMidWord(t *testing.T) {
	// Emacs acts on the part of the word from point onward.
	f := commandtest.New("hello")
	f.SetPoint(edAt(0, 2))
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "heLLO")
	edWantPoint(t, f, edAt(0, 5))

	g := commandtest.New("hello")
	g.SetPoint(edAt(0, 2))
	edMustRun(t, g, "capitalize-word")
	edWantText(t, g, "heLlo")
}

func TestCaseCommandsSkipLeadingSeparators(t *testing.T) {
	f := commandtest.New("   foo")
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "   FOO")
	edWantPoint(t, f, edAt(0, 6))
}

func TestCaseCommandsWithArg(t *testing.T) {
	f := commandtest.New("foo bar baz")
	edArg(f, 2)
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "FOO BAR baz")
	edWantPoint(t, f, edAt(0, 7))
}

func TestCaseCommandsWithNegativeArgActBackward(t *testing.T) {
	f := commandtest.New("foo bar")
	f.SetPoint(edAt(0, 7))
	edArg(f, -1)
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "foo BAR")
	edWantPoint(t, f, edAt(0, 4))
}

func TestCaseCommandsUseSimpleCaseMapping(t *testing.T) {
	// Go's strings.ToUpper applies *simple* case mapping, which is strictly
	// rune-for-rune: "ß" has no single-rune upper case, so it is left alone
	// rather than becoming "SS". Point is nevertheless advanced over the
	// replacement rather than to the old word end, so adopting full Unicode
	// casing later cannot silently misplace it.
	f := commandtest.New("ß")
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "ß")
	edWantPoint(t, f, edAt(0, 1))

	// A mapping that does change the rune, though not the rune count.
	g := commandtest.New("ǳ")
	edMustRun(t, g, "upcase-word")
	edWantText(t, g, "Ǳ")
	edWantPoint(t, g, edAt(0, 1))
}

func TestCaseCommandsCrossLines(t *testing.T) {
	f := commandtest.New("foo", "bar")
	edArg(f, 2)
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "FOO\nBAR")
	edWantPoint(t, f, edAt(1, 3))
}

func TestCaseCommandsWithNoWordLeftAreHarmless(t *testing.T) {
	f := commandtest.New("foo   ")
	f.SetPoint(edAt(0, 6))
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "foo   ")
}

func TestCaseCommandsWithZeroArgDoNothing(t *testing.T) {
	f := commandtest.New("foo")
	edArg(f, 0)
	edMustRun(t, f, "upcase-word")
	edWantText(t, f, "foo")
}

// --- point and goal column ------------------------------------------------

func TestEditingClearsTheGoalColumn(t *testing.T) {
	// Editing is not vertical motion, so a column C-n was aiming for is stale.
	f := commandtest.New("abc", "def")
	f.Win().GoalCol = 2
	edMustRun(t, f, "delete-char")
	if got := f.Win().GoalCol; got != -1 {
		t.Errorf("GoalCol = %d after an edit, want GoalColUnset (-1)", got)
	}
}

func TestEditsMarkTheBufferModified(t *testing.T) {
	f := commandtest.New("abc")
	if f.Buf().Modified() {
		t.Fatal("a fresh buffer reports modified")
	}
	edMustRun(t, f, "delete-char")
	if !f.Buf().Modified() {
		t.Error("buffer does not report modified after an edit")
	}
}

// --- line-crossing and argument clamping ----------------------------------

func TestDeleteBackwardCharAtColumnZeroJoinsLines(t *testing.T) {
	f := commandtest.New("ab", "cd")
	f.SetPoint(edAt(1, 0))
	edMustRun(t, f, "delete-backward-char")
	edWantText(t, f, "abcd")
	edWantPoint(t, f, edAt(0, 2))
}

func TestBackwardKillWordCrossesLines(t *testing.T) {
	// The region runs from the start of the previous word to point, so it spans
	// the newline and the lines join — which is what emacs does.
	f := commandtest.New("foo", "bar")
	f.SetPoint(edAt(1, 0))
	edMustRun(t, f, "backward-kill-word")
	edWantText(t, f, "bar")
	edWantYank(t, f, "foo\n")
}

func TestBackwardKillWordWithNegativeArgKillsForward(t *testing.T) {
	f := commandtest.New("foo bar")
	edArg(f, -1)
	edMustRun(t, f, "backward-kill-word")
	edWantText(t, f, " bar")
	edWantYank(t, f, "foo")
}

func TestKillLineWithNegativeArgKillsBackward(t *testing.T) {
	f := commandtest.New("aaa", "bbb", "ccc")
	f.SetPoint(edAt(2, 0))
	edArg(f, -2)
	edMustRun(t, f, "kill-line")
	edWantText(t, f, "ccc")
	edWantPoint(t, f, edAt(0, 0))
	edWantYank(t, f, "aaa\nbbb\n")
}

func TestKillLineWithZeroArgAtLineStartErrors(t *testing.T) {
	f := commandtest.New("hello")
	edArg(f, 0)
	if err := edRun(t, f, "kill-line"); !errors.Is(err, command.ErrBeginningOfBuffer) {
		t.Errorf("kill-line with zero arg at line start: error = %v, want ErrBeginningOfBuffer", err)
	}
}

func TestNonPositiveArgsBehaveAsOne(t *testing.T) {
	// C-u 0 on an inserting command still inserts once rather than nothing:
	// a repeat count below one is meaningless, not a request to do nothing.
	for _, tc := range []struct {
		name, cmd, want string
	}{
		{"open-line", "open-line", "\n"},
		{"indent-for-tab-command", "indent-for-tab-command", "\t"},
		{"newline", "newline", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := commandtest.New("")
			edArg(f, 0)
			edMustRun(t, f, tc.cmd)
			edWantText(t, f, tc.want)
		})
	}
}

func TestSelfInsertWithNonPositiveArgInsertsOnce(t *testing.T) {
	f := commandtest.New("")
	edArg(f, 0)
	edType(t, f, 'x')
	edWantText(t, f, "x")
}

func TestTransposeWordsAtBufferStartErrors(t *testing.T) {
	// No word precedes point, so there is nothing to swap with.
	f := commandtest.New("foo bar")
	if err := edRun(t, f, "transpose-words"); err == nil {
		t.Error("transpose-words at buffer start: want an error, got nil")
	}
}
