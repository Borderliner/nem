package command_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
)

// newSearchFake returns a Fake with the search and help commands registered,
// so tests can drive them through Run the way the event loop will.
func newSearchFake(t *testing.T, lines ...string) *commandtest.Fake {
	t.Helper()
	f := commandtest.New(lines...)
	if err := command.RegisterSearch(f.Reg); err != nil {
		t.Fatalf("RegisterSearch: %v", err)
	}
	return f
}

func at(line int, col text.RuneIdx) text.Pos { return text.Pos{Line: line, Col: col} }

// --- registration --------------------------------------------------------

func TestRegisterSearchRegistersEveryCommand(t *testing.T) {
	f := newSearchFake(t)
	want := []string{
		"describe-bindings",
		"describe-key",
		"execute-extended-command",
		"isearch-backward",
		"isearch-forward",
		"query-replace",
	}
	for _, name := range want {
		if _, ok := f.Reg.Lookup(name); !ok {
			t.Errorf("command %q not registered", name)
		}
	}
	for _, name := range want {
		c, _ := f.Reg.Lookup(name)
		if c.Doc == "" {
			t.Errorf("command %q has no Doc; M-x and describe-key show it", name)
		}
		if !c.Interactive {
			t.Errorf("command %q is not Interactive, so M-x cannot find it", name)
		}
	}
}

func TestRegisterSearchTwiceIsRejected(t *testing.T) {
	f := newSearchFake(t)
	if err := command.RegisterSearch(f.Reg); err == nil {
		t.Fatal("registering twice succeeded; duplicate names must be rejected")
	}
}

// --- isearch: the state machine, driven directly -------------------------
//
// The Fake only ever grows a reply prefix, so shortening the pattern (the
// backspace case) is exercised by driving the state machine itself.

func TestIsearchGrowingPatternAdvancesPoint(t *testing.T) {
	f := commandtest.New("abcfoo")
	s := command.NewIsearch(f, false)

	steps := []struct {
		pat  string
		want text.Pos
	}{
		{"f", at(0, 4)},
		{"fo", at(0, 5)},
		{"foo", at(0, 6)},
	}
	for _, step := range steps {
		s.Update(step.pat)
		if got := f.Point(); !got.Equal(step.want) {
			t.Errorf("after %q: point %v, want %v", step.pat, got, step.want)
		}
	}
}

func TestIsearchShorteningPatternMovesPointBack(t *testing.T) {
	f := commandtest.New("abcfoo")
	s := command.NewIsearch(f, false)

	s.Update("foo")
	if got := f.Point(); !got.Equal(at(0, 6)) {
		t.Fatalf("setup: point %v, want %v", got, at(0, 6))
	}

	// Backspacing must walk point back toward the origin, not leave it
	// stranded at a match the shorter pattern no longer justifies.
	s.Update("fo")
	if got := f.Point(); !got.Equal(at(0, 5)) {
		t.Errorf(`after shortening to "fo": point %v, want %v`, got, at(0, 5))
	}
	s.Update("f")
	if got := f.Point(); !got.Equal(at(0, 4)) {
		t.Errorf(`after shortening to "f": point %v, want %v`, got, at(0, 4))
	}
}

func TestIsearchEmptyPatternReturnsToOrigin(t *testing.T) {
	f := commandtest.New("abcfoo")
	f.SetPoint(at(0, 1))
	s := command.NewIsearch(f, false)

	s.Update("foo")
	if got := f.Point(); !got.Equal(at(0, 6)) {
		t.Fatalf("setup: point %v, want %v", got, at(0, 6))
	}
	s.Update("")
	if got := f.Point(); !got.Equal(at(0, 1)) {
		t.Errorf("empty pattern: point %v, want the origin %v", got, at(0, 1))
	}
}

func TestIsearchFailingLeavesPointAtLastGoodMatch(t *testing.T) {
	f := commandtest.New("abcfoo")
	s := command.NewIsearch(f, false)

	s.Update("fo")
	good := f.Point()
	s.Update("fox")

	if got := f.Point(); !got.Equal(good) {
		t.Errorf("failing search moved point to %v; it must stay at the last good match %v", got, good)
	}
	if !echoContains(f, "Failing I-search: fox") {
		t.Errorf("echoes = %q, want one containing %q", f.Echoes, "Failing I-search: fox")
	}
}

func TestIsearchRecoversAfterFailing(t *testing.T) {
	f := commandtest.New("abcfoo")
	s := command.NewIsearch(f, false)

	s.Update("fox")
	s.Update("fo")

	if got := f.Point(); !got.Equal(at(0, 5)) {
		t.Errorf("after recovering: point %v, want %v", got, at(0, 5))
	}
}

// --- isearch: Advance, the seam the editor's minibuffer keymap drives ----

func TestIsearchAdvanceStepsToTheNextMatch(t *testing.T) {
	f := commandtest.New("foo bar foo")
	s := command.NewIsearch(f, false)

	s.Update("foo")
	if got := f.Point(); !got.Equal(at(0, 3)) {
		t.Fatalf("first match: point %v, want %v", got, at(0, 3))
	}

	s.Advance()
	if got := f.Point(); !got.Equal(at(0, 11)) {
		t.Errorf("after Advance: point %v, want the second match end %v", got, at(0, 11))
	}
}

func TestIsearchAdvanceStaysPutPastTheLastMatch(t *testing.T) {
	f := commandtest.New("foo bar foo")
	s := command.NewIsearch(f, false)

	s.Update("foo")
	s.Advance() // now at the second and final match
	before := f.Point()
	f.Echoes = nil

	s.Advance()

	if got := f.Point(); !got.Equal(before) {
		t.Errorf("point %v, want it unmoved at %v", got, before)
	}
	if echoContains(f, "Failing I-search") {
		t.Error("overshooting the last match must not echo a search failure")
	}
	if !echoContains(f, "No further match") {
		t.Errorf("echoes = %q, want %q", f.Echoes, "No further match")
	}
}

func TestIsearchAdvanceThenShortenPatternResetsToFirstMatch(t *testing.T) {
	f := commandtest.New("foo bar foo")
	s := command.NewIsearch(f, false)

	s.Update("foo")
	s.Advance() // second match
	s.Update("fo")

	// Editing the pattern renumbers the matches, so the skip count resets.
	if got := f.Point(); !got.Equal(at(0, 2)) {
		t.Errorf("point %v, want the first match end %v", got, at(0, 2))
	}
}

func TestIsearchAdvanceWithEmptyPatternDoesNothing(t *testing.T) {
	f := commandtest.New("foo")
	s := command.NewIsearch(f, false)

	s.Advance()

	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want the origin %v", got, at(0, 0))
	}
}

func TestIsearchAdvanceBackward(t *testing.T) {
	f := commandtest.New("foo bar foo")
	f.SetPoint(at(0, 11))
	s := command.NewIsearch(f, true)

	s.Update("foo")
	if got := f.Point(); !got.Equal(at(0, 8)) {
		t.Fatalf("first backward match: point %v, want %v", got, at(0, 8))
	}

	s.Advance()
	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("after Advance: point %v, want the earlier match %v", got, at(0, 0))
	}
}

func TestIsearchPatternReportsTheCurrentPattern(t *testing.T) {
	f := commandtest.New("abcfoo")
	s := command.NewIsearch(f, false)

	if got := s.Pattern(); got != "" {
		t.Errorf("Pattern() = %q, want empty before any update", got)
	}
	s.Update("fo")
	if got := s.Pattern(); got != "fo" {
		t.Errorf("Pattern() = %q, want %q", got, "fo")
	}
}

// --- isearch: smart case -------------------------------------------------

func TestIsearchIsCaseInsensitiveForLowercasePattern(t *testing.T) {
	f := commandtest.New("xxx", "Hello there")
	s := command.NewIsearch(f, false)

	s.Update("hello")

	want := at(1, 5) // end of the match on line 1
	if got := f.Point(); !got.Equal(want) {
		t.Errorf("point %v, want %v: an all-lowercase pattern folds case", got, want)
	}
}

func TestIsearchIsCaseSensitiveOncePatternHasUppercase(t *testing.T) {
	f := commandtest.New("hello world", "Hello again")
	s := command.NewIsearch(f, false)

	s.Update("Hello")

	want := at(1, 5) // must skip the lowercase line 0 entirely
	if got := f.Point(); !got.Equal(want) {
		t.Errorf("point %v, want %v: an uppercase in the pattern forces case sensitivity", got, want)
	}
}

func TestIsearchFoldsBothDirections(t *testing.T) {
	// A lowercase pattern must match uppercase text and vice versa is NOT
	// symmetric: "Hello" is case-sensitive, "hello" is not.
	f := commandtest.New("HELLO")
	s := command.NewIsearch(f, false)
	s.Update("hello")
	if got := f.Point(); !got.Equal(at(0, 5)) {
		t.Errorf("point %v, want %v", got, at(0, 5))
	}
}

// --- isearch: direction --------------------------------------------------

func TestIsearchBackwardFindsEarlierMatchAndLandsAtItsStart(t *testing.T) {
	f := commandtest.New("foo middle foo")
	f.SetPoint(at(0, 11)) // just before the second "foo"
	s := command.NewIsearch(f, true)

	s.Update("foo")

	// Backward search lands at the START of the match, as emacs does.
	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want %v", got, at(0, 0))
	}
}

func TestIsearchBackwardAcrossLines(t *testing.T) {
	f := commandtest.New("target here", "middle", "end")
	f.SetPoint(at(2, 3))
	s := command.NewIsearch(f, true)

	s.Update("target")

	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want %v", got, at(0, 0))
	}
}

func TestIsearchBackwardFailsWhenNothingBehindPoint(t *testing.T) {
	f := commandtest.New("only here")
	f.SetPoint(at(0, 0))
	s := command.NewIsearch(f, true)

	s.Update("only")

	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want the origin %v", got, at(0, 0))
	}
	if !echoContains(f, "Failing I-search: only") {
		t.Errorf("echoes = %q, want a failing message", f.Echoes)
	}
}

func TestIsearchForwardSearchesAcrossLines(t *testing.T) {
	f := commandtest.New("first", "second", "third")
	s := command.NewIsearch(f, false)

	s.Update("third")

	if got := f.Point(); !got.Equal(at(2, 5)) {
		t.Errorf("point %v, want %v", got, at(2, 5))
	}
}

// A pattern containing a newline never matches in v1; single-line patterns
// only. Pinned so the limitation is explicit rather than accidental.
func TestIsearchNewlineInPatternNeverMatches(t *testing.T) {
	f := commandtest.New("first", "second")
	s := command.NewIsearch(f, false)

	s.Update("first\nsecond")

	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want the origin %v", got, at(0, 0))
	}
	if !echoContains(f, "Failing I-search") {
		t.Errorf("echoes = %q, want a failing message", f.Echoes)
	}
}

// --- isearch: the whole command through Run ------------------------------

func TestIsearchForwardCommandLeavesPointAtMatchOnReturn(t *testing.T) {
	f := newSearchFake(t, "abcfoo")
	f.Replies = []string{"foo"}

	if err := f.Run("isearch-forward"); err != nil {
		t.Fatalf("isearch-forward: %v", err)
	}
	if got := f.Point(); !got.Equal(at(0, 6)) {
		t.Errorf("point %v, want %v", got, at(0, 6))
	}
	if len(f.Prompts) != 1 || !strings.Contains(f.Prompts[0], "I-search") {
		t.Errorf("prompts = %q, want one I-search prompt", f.Prompts)
	}
}

func TestIsearchBackwardCommandUsesBackwardPrompt(t *testing.T) {
	f := newSearchFake(t, "foo middle foo")
	f.SetPoint(at(0, 11))
	f.Replies = []string{"foo"}

	if err := f.Run("isearch-backward"); err != nil {
		t.Fatalf("isearch-backward: %v", err)
	}
	if got := f.Point(); !got.Equal(at(0, 0)) {
		t.Errorf("point %v, want %v", got, at(0, 0))
	}
	if len(f.Prompts) != 1 || !strings.Contains(f.Prompts[0], "backward") {
		t.Errorf("prompts = %q, want a backward I-search prompt", f.Prompts)
	}
}

// The behaviour users rely on most, and the easiest to forget.
func TestIsearchQuitRestoresPointSavedAtEntry(t *testing.T) {
	f := newSearchFake(t, "abcfoo")
	f.SetPoint(at(0, 2))
	f.Replies = []string{commandtest.Quit}

	err := f.Run("isearch-forward")

	if !errors.Is(err, command.ErrQuit) {
		t.Errorf("err = %v, want command.ErrQuit propagated", err)
	}
	if got := f.Point(); !got.Equal(at(0, 2)) {
		t.Errorf("point %v, want the entry point %v restored", got, at(0, 2))
	}
	if !echoContains(f, "Quit") {
		t.Errorf("echoes = %q, want %q", f.Echoes, "Quit")
	}
}

// C-g must restore the origin even after the search has moved point away.
func TestIsearchQuitRestoresOriginAfterPointMoved(t *testing.T) {
	f := commandtest.New("abcfoo")
	f.SetPoint(at(0, 1))
	s := command.NewIsearch(f, false)

	s.Update("foo") // point is now at 0,6
	s.Abandon()

	if got := f.Point(); !got.Equal(at(0, 1)) {
		t.Errorf("point %v, want the origin %v", got, at(0, 1))
	}
}

// --- query-replace -------------------------------------------------------

func TestQueryReplaceYesAndNo(t *testing.T) {
	f := newSearchFake(t, "a a a")
	f.Replies = []string{"a", "X"}
	f.Chars = []rune{'y', 'n', 'y'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "X a X"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// A match the buffer will not let be changed is passed over, not offered, and
// the replacing goes on past it.
func TestQueryReplaceSkipsWhatCannotBeEdited(t *testing.T) {
	f := newSearchFake(t, "a", "a fixed", "a")
	f.Buf().SetEditGuard(func(from, to text.Pos, _ []rune) error {
		if from.Line == 1 {
			return errors.New("fixed")
		}
		return nil
	})
	f.Replies = []string{"a", "X"}
	f.Chars = []rune{'y', 'y'} // one answer per match that can change

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "X\na fixed\nX"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if !echoContains(f, "skipped 1") {
		t.Errorf("echoes = %q, want the skipped match counted", f.Echoes)
	}
}

// A read-only buffer is refused before any prompt, not after both.
func TestQueryReplaceRefusesAReadOnlyBuffer(t *testing.T) {
	f := newSearchFake(t, "a")
	f.Buf().SetReadOnly(true)
	if err := f.Run("query-replace"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	if len(f.Prompts) != 0 {
		t.Errorf("prompted %q first", f.Prompts)
	}
}

func TestQueryReplacePromptsForBothStrings(t *testing.T) {
	f := newSearchFake(t, "a")
	f.Replies = []string{"a", "b"}
	f.Chars = []rune{'y'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if len(f.Prompts) != 2 {
		t.Fatalf("prompts = %q, want two", f.Prompts)
	}
	if !strings.Contains(f.Prompts[1], "a") {
		t.Errorf("second prompt %q should name the search string", f.Prompts[1])
	}
}

func TestQueryReplaceBangReplacesAllRemaining(t *testing.T) {
	f := newSearchFake(t, "a a a a")
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{'n', '!'} // skip the first, then take the rest

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "a Z Z Z"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if n := len(f.CharPrompts); n != 2 {
		t.Errorf("asked %d times, want 2: ! must stop prompting", n)
	}
}

func TestQueryReplaceQuitStopsWithoutFurtherChanges(t *testing.T) {
	f := newSearchFake(t, "a a a")
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{'y', 'q'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "Z a a"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// Replacing "a" with "aa" must advance past the inserted text. Without that
// the next search finds the text it just wrote and loops forever.
func TestQueryReplaceWithOverlappingReplacementTerminates(t *testing.T) {
	f := newSearchFake(t, "a a")
	f.Replies = []string{"a", "aa"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "aa aa"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestQueryReplaceReplacementContainingPatternTerminatesWithY(t *testing.T) {
	f := newSearchFake(t, "aaa")
	f.Replies = []string{"a", "ba"}
	f.Chars = []rune{'y', 'y', 'y'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "bababa"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// A replacement containing a newline splits the line, so resuming the scan
// means moving to a different line, not just a later column.
func TestQueryReplaceWithMultilineReplacement(t *testing.T) {
	f := newSearchFake(t, "a a")
	f.Replies = []string{"a", "X\nY"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "X\nY X\nY"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestQueryReplaceDeletingMatchWithEmptyReplacement(t *testing.T) {
	f := newSearchFake(t, "axbxc")
	f.Replies = []string{"x", ""}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "abc"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestQueryReplaceEchoesCount(t *testing.T) {
	f := newSearchFake(t, "a a a")
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if !echoContains(f, "3") {
		t.Errorf("echoes = %q, want a count of 3", f.Echoes)
	}
}

func TestQueryReplaceAcrossLines(t *testing.T) {
	f := newSearchFake(t, "a one", "b a two", "c a")
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "Z one\nb Z two\nc Z"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestQueryReplaceStartsFromPointNotBufferStart(t *testing.T) {
	f := newSearchFake(t, "a a a")
	f.SetPoint(at(0, 2))
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "a Z Z"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestQueryReplaceEmptyPatternIsRefused(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{"", ""}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "abc"; got != want {
		t.Errorf("text = %q, want it unchanged", got)
	}
	if len(f.CharPrompts) != 0 {
		t.Errorf("an empty search string must not start the y/n loop")
	}
}

func TestQueryReplaceNoMatchEchoesAndChangesNothing(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{"zzz", "Z"}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "abc"; got != want {
		t.Errorf("text = %q, want it unchanged", got)
	}
	if len(f.CharPrompts) != 0 {
		t.Errorf("no match must not prompt")
	}
}

func TestQueryReplaceQuitAtFirstPromptPropagates(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{commandtest.Quit}

	err := f.Run("query-replace")

	if !errors.Is(err, command.ErrQuit) {
		t.Errorf("err = %v, want command.ErrQuit", err)
	}
	if got, want := f.Text(), "abc"; got != want {
		t.Errorf("text = %q, want it unchanged", got)
	}
}

func TestQueryReplaceQuitAtCharPromptStops(t *testing.T) {
	f := newSearchFake(t, "a a")
	f.Replies = []string{"a", "Z"}
	f.Chars = []rune{commandtest.QuitChar}

	err := f.Run("query-replace")

	if !errors.Is(err, command.ErrQuit) {
		t.Errorf("err = %v, want command.ErrQuit", err)
	}
	if got, want := f.Text(), "a a"; got != want {
		t.Errorf("text = %q, want it unchanged", got)
	}
}

func TestQueryReplaceIsCaseSensitiveWithUppercasePattern(t *testing.T) {
	f := newSearchFake(t, "a A a")
	f.Replies = []string{"A", "Z"}
	f.Chars = []rune{'!'}

	if err := f.Run("query-replace"); err != nil {
		t.Fatalf("query-replace: %v", err)
	}
	if got, want := f.Text(), "a Z a"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// --- execute-extended-command (M-x) --------------------------------------

func TestExecuteExtendedCommandRunsTheNamedCommand(t *testing.T) {
	f := newSearchFake(t, "abc")
	ran := false
	registerCmd(t, f, command.Command{
		Name: "test-target", Doc: "t", Interactive: true,
		Fn: func(command.Env) error { ran = true; return nil },
	})
	f.Replies = []string{"test-target"}

	if err := f.Run("execute-extended-command"); err != nil {
		t.Fatalf("M-x: %v", err)
	}
	if !ran {
		t.Error("the named command did not run")
	}
}

func TestExecuteExtendedCommandUnknownEchoesNoMatch(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{"no-such-command"}

	if err := f.Run("execute-extended-command"); err != nil {
		t.Fatalf("M-x should not error on an unknown name: %v", err)
	}
	if !echoContains(f, "No match") {
		t.Errorf("echoes = %q, want %q", f.Echoes, "No match")
	}
}

// M-x shows each command's keys beside it: the two shortest, shortest first,
// and nothing for a command bound to no key.
func TestExecuteExtendedCommandAnnotatesKeys(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.BindingMap = map[string]string{
		"C-x C-f": "find-file",
		"C-p":     "previous-line",
		"<up>":    "previous-line",
		"M-p":     "previous-line",
		"C-x C-p": "previous-line",
	}
	f.Replies = []string{commandtest.Quit}
	_ = f.Run("execute-extended-command")

	if len(f.Reads) == 0 || f.Reads[0].Annotate == nil {
		t.Fatal("M-x does not annotate its candidates")
	}
	note := f.Reads[0].Annotate
	for cmd, want := range map[string]string{
		"find-file":     "C-x C-f",
		"previous-line": "C-p, M-p",
		"kill-line":     "",
	} {
		if got := note(cmd); got != want {
			t.Errorf("note for %s = %q, want %q", cmd, got, want)
		}
	}
}

func TestCompleteFromOffersEveryName(t *testing.T) {
	names := []string{"isearch-backward", "isearch-forward", "kill-line"}
	complete := command.CompleteFrom(names)

	// CompleteFrom deliberately does not filter. The minibuffer ranks candidates
	// by fuzzy match, so narrowing here would defeat it: "fwc" reaches
	// forward-char with no prefix match at all, and a pre-filtered list would
	// come back empty.
	for _, input := range []string{"", "is", "isearch-f", "kill", "zzz", "fwc"} {
		got := complete(input)
		if !slices.Equal(got, names) {
			t.Errorf("complete(%q) = %q, want every name %q", input, got, names)
		}
	}
}

// The returned slice must be the caller's own, so a consumer that sorts or
// truncates it cannot corrupt the command registry's names.
func TestCompleteFromDoesNotShareItsBackingArray(t *testing.T) {
	names := []string{"beta", "alpha"}
	got := command.CompleteFrom(names)("")
	got[0] = "MUTATED"
	if names[0] != "beta" {
		t.Errorf("mutating the result changed the source to %q", names)
	}
}

func TestExecuteExtendedCommandOffersCompletion(t *testing.T) {
	f := newSearchFake(t, "abc")
	// The first reply answers M-x; the second answers isearch's own prompt.
	f.Replies = []string{"isearch-forward", "abc"}

	if err := f.Run("execute-extended-command"); err != nil {
		t.Fatalf("M-x: %v", err)
	}
	// isearch-forward ran, which it could only do by name resolution through
	// the registry that also backs completion.
	if len(f.RunNames) != 2 || f.RunNames[1] != "isearch-forward" {
		t.Errorf("RunNames = %q, want M-x then isearch-forward", f.RunNames)
	}
}

// The universal argument must survive the indirection through M-x.
func TestExecuteExtendedCommandPassesUniversalArgument(t *testing.T) {
	f := newSearchFake(t, "abc")
	var sawN int
	var sawExplicit bool
	registerCmd(t, f, command.Command{
		Name: "record-arg", Doc: "t", Interactive: true,
		Fn: func(e command.Env) error { sawN, sawExplicit = e.Arg(); return nil },
	})
	f.ArgN, f.ArgExplicit = 4, true
	f.Replies = []string{"record-arg"}

	if err := f.Run("execute-extended-command"); err != nil {
		t.Fatalf("M-x: %v", err)
	}
	if sawN != 4 || !sawExplicit {
		t.Errorf("invoked command saw Arg() = (%d, %v), want (4, true)", sawN, sawExplicit)
	}
}

func TestExecuteExtendedCommandPropagatesTheCommandsError(t *testing.T) {
	f := newSearchFake(t, "abc")
	boom := errors.New("boom")
	registerCmd(t, f, command.Command{
		Name: "explode", Doc: "t", Interactive: true,
		Fn: func(command.Env) error { return boom },
	})
	f.Replies = []string{"explode"}

	if err := f.Run("execute-extended-command"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the command's own error", err)
	}
}

func TestExecuteExtendedCommandQuitPropagates(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{commandtest.Quit}

	if err := f.Run("execute-extended-command"); !errors.Is(err, command.ErrQuit) {
		t.Errorf("err = %v, want command.ErrQuit", err)
	}
}

func TestExecuteExtendedCommandEmptyNameDoesNothing(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Replies = []string{""}

	if err := f.Run("execute-extended-command"); err != nil {
		t.Fatalf("M-x: %v", err)
	}
	if len(f.RunNames) != 1 {
		t.Errorf("RunNames = %q, want only the M-x invocation itself", f.RunNames)
	}
}

// --- describe-bindings ---------------------------------------------------

func TestDescribeBindingsRendersSortedAlignedTable(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.BindingMap = map[string]string{
		"C-x C-s": "save-buffer",
		"C-f":     "forward-char",
		"C-x C-f": "find-file",
	}

	if err := f.Run("describe-bindings"); err != nil {
		t.Fatalf("describe-bindings: %v", err)
	}

	got := f.Text()
	for _, want := range []string{"C-f", "forward-char", "C-x C-f", "find-file", "C-x C-s", "save-buffer"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendering is missing %q:\n%s", want, got)
		}
	}

	// Sorted by key sequence.
	iCf, iCxCf, iCxCs := strings.Index(got, "C-f "), strings.Index(got, "C-x C-f"), strings.Index(got, "C-x C-s")
	if !(iCf < iCxCf && iCxCf < iCxCs) {
		t.Errorf("bindings are not sorted by key sequence:\n%s", got)
	}

	// Aligned: every command name starts at the same column.
	var cols []int
	for _, line := range strings.Split(got, "\n") {
		for _, cmd := range []string{"forward-char", "find-file", "save-buffer"} {
			if i := strings.Index(line, cmd); i > 0 {
				cols = append(cols, i)
			}
		}
	}
	if len(cols) != 3 {
		t.Fatalf("found %d command columns, want 3:\n%s", len(cols), got)
	}
	for _, c := range cols[1:] {
		if c != cols[0] {
			t.Errorf("command column not aligned: %v\n%s", cols, got)
		}
	}
}

func TestDescribeBindingsVisitsTheHelpBuffer(t *testing.T) {
	f := newSearchFake(t, "original text")
	f.BindingMap = map[string]string{"C-f": "forward-char"}

	if err := f.Run("describe-bindings"); err != nil {
		t.Fatalf("describe-bindings: %v", err)
	}
	if strings.Contains(f.Text(), "original text") {
		t.Error("the active window still shows the original buffer")
	}
	if _, ok := f.BufferByName("*Bindings*"); !ok {
		t.Error("no *Bindings* buffer was created")
	}
}

func TestDescribeBindingsWithNoBindings(t *testing.T) {
	f := newSearchFake(t, "abc")

	if err := f.Run("describe-bindings"); err != nil {
		t.Fatalf("describe-bindings: %v", err)
	}
	if _, ok := f.BufferByName("*Bindings*"); !ok {
		t.Error("an empty binding table should still produce the buffer")
	}
}

func TestDescribeBindingsBufferIsUnmodified(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.BindingMap = map[string]string{"C-f": "forward-char"}

	if err := f.Run("describe-bindings"); err != nil {
		t.Fatalf("describe-bindings: %v", err)
	}
	if f.Buf().Modified() {
		t.Error("a freshly rendered help buffer must not look modified")
	}
}

// --- describe-key --------------------------------------------------------

func ctrlKey(r rune) keymap.Key { return keymap.Normalize(keymap.Key{Rune: r, Ctrl: true}) }

func TestDescribeKeyReportsCommandAndDoc(t *testing.T) {
	f := newSearchFake(t, "abc")
	registerCmd(t, f, command.Command{
		Name: "forward-char", Doc: "Move point forward one grapheme.", Interactive: true,
		Fn: func(command.Env) error { return nil },
	})
	f.BindingMap = map[string]string{"C-f": "forward-char"}
	f.Keys = []keymap.Key{ctrlKey('f')}

	if err := f.Run("describe-key"); err != nil {
		t.Fatalf("describe-key: %v", err)
	}
	if !echoContains(f, "forward-char") {
		t.Errorf("echoes = %q, want the command name", f.Echoes)
	}
	if !echoContains(f, "Move point forward one grapheme.") {
		t.Errorf("echoes = %q, want the command's doc", f.Echoes)
	}
}

func TestDescribeKeyUndefinedKey(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.Keys = []keymap.Key{ctrlKey('q')}

	if err := f.Run("describe-key"); err != nil {
		t.Fatalf("describe-key: %v", err)
	}
	if !echoContains(f, "undefined") {
		t.Errorf("echoes = %q, want %q", f.Echoes, "undefined")
	}
	if !echoContains(f, "C-q") {
		t.Errorf("echoes = %q, want the key named", f.Echoes)
	}
}

// describe-key must keep reading while the sequence so far is only a prefix.
func TestDescribeKeyReadsAMultiKeySequence(t *testing.T) {
	f := newSearchFake(t, "abc")
	registerCmd(t, f, command.Command{
		Name: "find-file", Doc: "Visit a file.", Interactive: true,
		Fn: func(command.Env) error { return nil },
	})
	f.BindingMap = map[string]string{"C-x C-f": "find-file"}
	f.Keys = []keymap.Key{ctrlKey('x'), ctrlKey('f')}

	if err := f.Run("describe-key"); err != nil {
		t.Fatalf("describe-key: %v", err)
	}
	if len(f.KeyPrompts) != 2 {
		t.Errorf("read %d keys, want 2 for a two-key sequence", len(f.KeyPrompts))
	}
	if !echoContains(f, "find-file") {
		t.Errorf("echoes = %q, want find-file", f.Echoes)
	}
	if !echoContains(f, "C-x C-f") {
		t.Errorf("echoes = %q, want the full sequence named", f.Echoes)
	}
}

func TestDescribeKeyUnknownContinuationOfAPrefix(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.BindingMap = map[string]string{"C-x C-f": "find-file"}
	f.Keys = []keymap.Key{ctrlKey('x'), ctrlKey('z')}

	if err := f.Run("describe-key"); err != nil {
		t.Fatalf("describe-key: %v", err)
	}
	if !echoContains(f, "undefined") {
		t.Errorf("echoes = %q, want undefined", f.Echoes)
	}
}

func TestDescribeKeyQuitPropagates(t *testing.T) {
	f := newSearchFake(t, "abc")
	// No canned keys: ReadKey reports a test bug, which must not be mistaken
	// for a user quit.
	err := f.Run("describe-key")
	if err == nil {
		t.Fatal("want an error when no key is available")
	}
	if errors.Is(err, command.ErrQuit) {
		t.Error("an exhausted key queue must not masquerade as C-g")
	}
}

func TestDescribeKeyReportsCommandWithNoDoc(t *testing.T) {
	f := newSearchFake(t, "abc")
	f.BindingMap = map[string]string{"C-f": "mystery"}
	f.Keys = []keymap.Key{ctrlKey('f')}

	if err := f.Run("describe-key"); err != nil {
		t.Fatalf("describe-key: %v", err)
	}
	if !echoContains(f, "mystery") {
		t.Errorf("echoes = %q, want the command name even with no registration", f.Echoes)
	}
}

// --- helpers -------------------------------------------------------------

func registerCmd(t *testing.T, f *commandtest.Fake, c command.Command) {
	t.Helper()
	if err := f.Reg.Register(c); err != nil {
		t.Fatalf("Register(%q): %v", c.Name, err)
	}
}

func echoContains(f *commandtest.Fake, want string) bool {
	for _, e := range f.Echoes {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

// Case folding covers letters beyond ASCII, forwards and backwards: the
// folding was moved off the general path, and the first-rune check in front
// of each full comparison must fold as the comparison does.
func TestSearchFoldsBeyondASCII(t *testing.T) {
	b := text.NewBuffer()
	if err := b.Insert(text.Pos{}, []rune("Straße\nÉCOLE und Ecole\nNAÏVE")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pat  string
		want text.Pos
	}{
		{"école", text.Pos{Line: 1, Col: 0}},
		{"naïve", text.Pos{Line: 2, Col: 0}},
		{"ecole", text.Pos{Line: 1, Col: 10}},
		{"STRASSE", text.Pos{}}, // no full case folding: ß is not SS
	} {
		start, _, ok := command.SearchForward(b, tc.pat, text.Pos{}, true)
		if tc.pat == "STRASSE" {
			if ok {
				t.Errorf("%q matched at %v; ß does not fold to SS", tc.pat, start)
			}
			continue
		}
		if !ok || start != tc.want {
			t.Errorf("SearchForward(%q) = %v, %v; want %v", tc.pat, start, ok, tc.want)
		}
		if back, _, ok := command.SearchBackward(b, tc.pat, b.End(), true); !ok || back != tc.want {
			t.Errorf("SearchBackward(%q) = %v, %v; want %v", tc.pat, back, ok, tc.want)
		}
	}
}
