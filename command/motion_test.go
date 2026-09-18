package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/command/commandtest"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// family is a ZWJ emoji sequence: seven runes, one grapheme, one cursor stop.
const family = "👨‍👩‍👧‍👦"

// accented is "e" followed by a combining acute: two runes, one grapheme.
const accented = "é"

func newMotionFake(t *testing.T, lines ...string) *commandtest.Fake {
	t.Helper()
	f := commandtest.New(lines...)
	if err := command.RegisterMotion(f.Reg); err != nil {
		t.Fatalf("RegisterMotion: %v", err)
	}
	return f
}

func run(t *testing.T, f *commandtest.Fake, name string) {
	t.Helper()
	if err := f.Run(name); err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
}

func wantPoint(t *testing.T, f *commandtest.Fake, line int, col text.RuneIdx) {
	t.Helper()
	got := f.Point()
	if got.Line != line || got.Col != col {
		t.Fatalf("point = {%d,%d}, want {%d,%d}", got.Line, got.Col, line, col)
	}
}

// --- registration -----------------------------------------------------------

func TestRegisterMotionRegistersEveryCommand(t *testing.T) {
	f := newMotionFake(t, "hello")
	want := []string{
		"backward-char", "backward-word", "beginning-of-buffer", "end-of-buffer",
		"forward-char", "forward-word", "goto-line", "move-beginning-of-line",
		"move-end-of-line", "next-line", "previous-line", "recenter-top-bottom",
		"scroll-down-command", "scroll-up-command",
	}
	for _, name := range want {
		if _, ok := f.Reg.Lookup(name); !ok {
			t.Errorf("command %q not registered", name)
		}
	}
	for _, name := range want {
		c, _ := f.Reg.Lookup(name)
		if !c.Interactive {
			t.Errorf("%q should be interactive (M-x must find it)", name)
		}
		if c.Doc == "" {
			t.Errorf("%q has no doc string", name)
		}
	}
}

func TestRegisterMotionIsIdempotentlyRejected(t *testing.T) {
	f := newMotionFake(t, "hello")
	if err := command.RegisterMotion(f.Reg); !errors.Is(err, command.ErrDuplicateCommand) {
		t.Fatalf("second RegisterMotion error = %v, want ErrDuplicateCommand", err)
	}
}

// --- grapheme-wise character motion ----------------------------------------

func TestForwardCharMovesByGraphemeNotRune(t *testing.T) {
	// "x" + e+combining-acute + "y": 4 runes, 3 graphemes.
	f := newMotionFake(t, "x"+accented+"y")
	wantPoint(t, f, 0, 0)
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 1) // onto the accented cluster
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 3) // skips both runes of the cluster
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 4) // end of line
}

func TestForwardCharSkipsWholeZWJSequence(t *testing.T) {
	// "a" + 7-rune ZWJ family + "b": 9 runes, 3 graphemes.
	line := "a" + family + "b"
	if got := len([]rune(line)); got != 9 {
		t.Fatalf("test premise wrong: %d runes, want 9", got)
	}
	f := newMotionFake(t, line)
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 1)
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 8) // one stop across all seven runes
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 9)
}

func TestBackwardCharMovesByGrapheme(t *testing.T) {
	line := "a" + family + "b"
	f := newMotionFake(t, line)
	f.SetPoint(text.Pos{Line: 0, Col: 9})
	run(t, f, "backward-char")
	wantPoint(t, f, 0, 8)
	run(t, f, "backward-char")
	wantPoint(t, f, 0, 1)
	run(t, f, "backward-char")
	wantPoint(t, f, 0, 0)
}

func TestCharMotionCrossesLineBoundaries(t *testing.T) {
	f := newMotionFake(t, "ab", "cd")
	f.SetPoint(text.Pos{Line: 0, Col: 2}) // end of first line
	run(t, f, "forward-char")
	wantPoint(t, f, 1, 0) // onto the start of the next line
	run(t, f, "backward-char")
	wantPoint(t, f, 0, 2) // back to the end of the previous line
}

func TestCharMotionStopsAtBufferBoundariesWithoutError(t *testing.T) {
	f := newMotionFake(t, "ab")
	run(t, f, "backward-char") // already at origin
	wantPoint(t, f, 0, 0)

	f.SetPoint(text.Pos{Line: 0, Col: 2})
	run(t, f, "forward-char") // already at end of buffer
	wantPoint(t, f, 0, 2)
}

// --- goal column ------------------------------------------------------------

// TestGoalColumnSurvivesShortLine is the most valuable test in this file.
// Moving down through a short line and back out must return to the original
// column, not to the short line's end.
func TestGoalColumnSurvivesShortLine(t *testing.T) {
	f := newMotionFake(t, "0123456789abc", "ab", "0123456789xyz")
	f.SetPoint(text.Pos{Line: 0, Col: 10})

	run(t, f, "next-line")
	wantPoint(t, f, 1, 2) // clamped to the short line's end

	run(t, f, "next-line")
	wantPoint(t, f, 2, 10) // goal column restored on the long line

	run(t, f, "previous-line")
	wantPoint(t, f, 1, 2) // clamped again on the way back

	run(t, f, "previous-line")
	wantPoint(t, f, 0, 10) // and back to where we started
}

func TestHorizontalMotionResetsGoalColumn(t *testing.T) {
	f := newMotionFake(t, "0123456789abc", "ab", "0123456789xyz")
	f.SetPoint(text.Pos{Line: 0, Col: 10})

	run(t, f, "next-line")
	wantPoint(t, f, 1, 2)

	run(t, f, "backward-char") // horizontal motion clears the goal
	wantPoint(t, f, 1, 1)

	run(t, f, "next-line")
	wantPoint(t, f, 2, 1) // new goal is 1, not the old 10
}

func TestVerticalMotionPreservesGoalColumnField(t *testing.T) {
	f := newMotionFake(t, "0123456789abc", "ab", "0123456789xyz")
	f.SetPoint(text.Pos{Line: 0, Col: 10})
	run(t, f, "next-line")
	if got := f.Win().GoalCol; got != 10 {
		t.Fatalf("GoalCol = %d after vertical motion, want 10", got)
	}
}

func TestEveryNonVerticalMotionClearsGoalColumn(t *testing.T) {
	// Each of these must leave the goal column unset so that the next
	// vertical move establishes a fresh one from the current column.
	for _, name := range []string{
		"forward-char", "backward-char", "forward-word", "backward-word",
		"move-beginning-of-line", "move-end-of-line",
		"beginning-of-buffer", "end-of-buffer",
		"scroll-up-command", "scroll-down-command",
	} {
		t.Run(name, func(t *testing.T) {
			f := newMotionFake(t, "hello world", "second line here", "third")
			f.SetPoint(text.Pos{Line: 1, Col: 4})
			f.Win().GoalCol = 9 // pretend a vertical run established one
			run(t, f, name)
			if got := f.Win().GoalCol; got != view.GoalColUnset {
				t.Fatalf("%s left GoalCol = %d, want GoalColUnset", name, got)
			}
		})
	}
}

func TestVerticalMotionClampsAtBufferEnds(t *testing.T) {
	f := newMotionFake(t, "one", "two")
	run(t, f, "previous-line") // already on the first line
	wantPoint(t, f, 0, 0)

	f.SetPoint(text.Pos{Line: 1, Col: 0})
	run(t, f, "next-line") // already on the last line
	wantPoint(t, f, 1, 0)
}

// --- universal argument -----------------------------------------------------

func TestArgRepeatsMotion(t *testing.T) {
	f := newMotionFake(t, "0123456789")
	f.ArgN, f.ArgExplicit = 5, true
	run(t, f, "forward-char")
	wantPoint(t, f, 0, 5)
}

func TestNegativeArgReversesDirection(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		start    text.Pos
		arg      int
		wantLine int
		wantCol  text.RuneIdx
	}{
		{"forward-char back", "forward-char", text.Pos{Line: 0, Col: 8}, -3, 0, 5},
		{"backward-char forward", "backward-char", text.Pos{Line: 0, Col: 2}, -3, 0, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newMotionFake(t, "0123456789")
			f.SetPoint(tc.start)
			f.ArgN, f.ArgExplicit = tc.arg, true
			run(t, f, tc.command)
			wantPoint(t, f, tc.wantLine, tc.wantCol)
		})
	}
}

func TestNegativeArgOnVerticalMotion(t *testing.T) {
	f := newMotionFake(t, "aaa", "bbb", "ccc", "ddd")
	f.SetPoint(text.Pos{Line: 3, Col: 0})
	f.ArgN, f.ArgExplicit = -2, true
	run(t, f, "next-line") // negative C-n goes up
	wantPoint(t, f, 1, 0)
}

func TestArgOnVerticalMotion(t *testing.T) {
	f := newMotionFake(t, "aaa", "bbb", "ccc", "ddd")
	f.ArgN, f.ArgExplicit = 3, true
	run(t, f, "next-line")
	wantPoint(t, f, 3, 0)
}

// --- word motion ------------------------------------------------------------

func TestForwardWord(t *testing.T) {
	f := newMotionFake(t, "hello world  foo")
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 5) // end of "hello"
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 11) // end of "world"
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 16) // end of "foo"
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 16) // stays put at end of buffer
}

func TestBackwardWord(t *testing.T) {
	f := newMotionFake(t, "hello world  foo")
	f.SetPoint(text.Pos{Line: 0, Col: 16})
	run(t, f, "backward-word")
	wantPoint(t, f, 0, 13) // start of "foo"
	run(t, f, "backward-word")
	wantPoint(t, f, 0, 6) // start of "world"
	run(t, f, "backward-word")
	wantPoint(t, f, 0, 0) // start of "hello"
	run(t, f, "backward-word")
	wantPoint(t, f, 0, 0) // stays put at origin
}

func TestWordMotionCrossesLines(t *testing.T) {
	f := newMotionFake(t, "foo", "bar")
	f.SetPoint(text.Pos{Line: 0, Col: 3}) // end of "foo"
	run(t, f, "forward-word")
	wantPoint(t, f, 1, 3) // over the newline and through "bar"

	run(t, f, "backward-word")
	wantPoint(t, f, 1, 0) // start of "bar"
	run(t, f, "backward-word")
	wantPoint(t, f, 0, 0) // back over the newline to "foo"
}

func TestWordMotionTreatsCombiningMarksAsWordConstituents(t *testing.T) {
	// "café" written with a combining acute must be one word, so a single
	// forward-word lands past all of it.
	f := newMotionFake(t, "caf"+accented+" next")
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 5) // c,a,f,e,combining = 5 runes
}

func TestWordMotionHonoursArg(t *testing.T) {
	f := newMotionFake(t, "one two three four")
	f.ArgN, f.ArgExplicit = 3, true
	run(t, f, "forward-word")
	wantPoint(t, f, 0, 13) // end of "three"
}

// --- line ends --------------------------------------------------------------

func TestMoveBeginningAndEndOfLine(t *testing.T) {
	f := newMotionFake(t, "hello world")
	f.SetPoint(text.Pos{Line: 0, Col: 5})
	run(t, f, "move-end-of-line")
	wantPoint(t, f, 0, 11)
	run(t, f, "move-beginning-of-line")
	wantPoint(t, f, 0, 0)
}

func TestMoveBeginningOfLineWithArgMovesLines(t *testing.T) {
	// Emacs: C-u 3 C-a moves forward two lines, then to that line's start.
	f := newMotionFake(t, "aaa", "bbb", "ccc")
	f.SetPoint(text.Pos{Line: 0, Col: 2})
	f.ArgN, f.ArgExplicit = 3, true
	run(t, f, "move-beginning-of-line")
	wantPoint(t, f, 2, 0)
}

func TestBeginningAndEndOfBuffer(t *testing.T) {
	f := newMotionFake(t, "first", "middle", "last line")
	f.SetPoint(text.Pos{Line: 1, Col: 3})
	run(t, f, "end-of-buffer")
	wantPoint(t, f, 2, 9)
	run(t, f, "beginning-of-buffer")
	wantPoint(t, f, 0, 0)
}

// --- scrolling --------------------------------------------------------------

func TestScrollUpMovesPointByScreenfulLessTwo(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 5)
	}
	f := newMotionFake(t, lines...)
	f.Height = 24
	run(t, f, "scroll-up-command")
	// A screenful less two lines of overlap.
	wantPoint(t, f, 22, 0)
}

func TestScrollDownMovesPointBack(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 5)
	}
	f := newMotionFake(t, lines...)
	f.Height = 24
	f.SetPoint(text.Pos{Line: 50, Col: 0})
	f.Win().Top = 50
	run(t, f, "scroll-down-command")
	wantPoint(t, f, 28, 0)
}

func TestScrollWithExplicitArgScrollsThatManyLines(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "y"
	}
	f := newMotionFake(t, lines...)
	f.Height = 24
	f.ArgN, f.ArgExplicit = 5, true
	run(t, f, "scroll-up-command")
	wantPoint(t, f, 5, 0)
}

func TestScrollClampsAtBufferEnds(t *testing.T) {
	f := newMotionFake(t, "one", "two", "three")
	f.Height = 24
	run(t, f, "scroll-up-command")
	wantPoint(t, f, 2, 0) // last line, not past it
	run(t, f, "scroll-down-command")
	wantPoint(t, f, 0, 0)
	if got := f.Win().Top; got < 0 {
		t.Fatalf("Top = %d, must never go negative", got)
	}
}

// --- goto-line --------------------------------------------------------------

func TestGotoLinePromptsWhenNoArg(t *testing.T) {
	f := newMotionFake(t, "one", "two", "three", "four")
	f.Replies = []string{"3"}
	run(t, f, "goto-line")
	wantPoint(t, f, 2, 0) // user line 3 is index 2
	if len(f.Prompts) != 1 {
		t.Fatalf("prompts = %v, want exactly one", f.Prompts)
	}
}

func TestGotoLineUsesArgWithoutPrompting(t *testing.T) {
	f := newMotionFake(t, "one", "two", "three", "four")
	f.ArgN, f.ArgExplicit = 2, true
	run(t, f, "goto-line")
	wantPoint(t, f, 1, 0)
	if len(f.Prompts) != 0 {
		t.Fatalf("prompts = %v, want none when an arg was given", f.Prompts)
	}
}

func TestGotoLineRejectsNonNumericWithEchoNotError(t *testing.T) {
	f := newMotionFake(t, "one", "two")
	f.Replies = []string{"abc"}
	if err := f.Run("goto-line"); err != nil {
		t.Fatalf("goto-line returned error %v, want a message via Echo", err)
	}
	wantPoint(t, f, 0, 0) // point unmoved
	if len(f.Echoes) == 0 {
		t.Fatal("expected a message in the echo area")
	}
}

func TestGotoLineClampsOutOfRange(t *testing.T) {
	cases := []struct {
		reply    string
		wantLine int
	}{
		{"999", 2}, // past the end clamps to the last line
		{"0", 0},   // before the start clamps to the first
		{"-5", 0},
	}
	for _, tc := range cases {
		t.Run(tc.reply, func(t *testing.T) {
			f := newMotionFake(t, "one", "two", "three")
			f.Replies = []string{tc.reply}
			run(t, f, "goto-line")
			wantPoint(t, f, tc.wantLine, 0)
		})
	}
}

func TestGotoLinePropagatesQuit(t *testing.T) {
	f := newMotionFake(t, "one", "two")
	f.Replies = []string{commandtest.Quit}
	err := f.Run("goto-line")
	if !errors.Is(err, command.ErrQuit) {
		t.Fatalf("error = %v, want ErrQuit", err)
	}
	wantPoint(t, f, 0, 0)
}

func TestGotoLineToleratesSurroundingSpace(t *testing.T) {
	f := newMotionFake(t, "one", "two", "three")
	f.Replies = []string{"  2  "}
	run(t, f, "goto-line")
	wantPoint(t, f, 1, 0)
}

// --- recenter ---------------------------------------------------------------

func TestRecenterCyclesCentreTopBottom(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "z"
	}
	f := newMotionFake(t, lines...)
	f.Height = 21 // odd, so the centre is unambiguous
	f.SetPoint(text.Pos{Line: 50, Col: 0})

	run(t, f, "recenter-top-bottom")
	if got, want := f.Win().Top, 40; got != want {
		t.Fatalf("first press: Top = %d, want %d (centre)", got, want)
	}

	f.SetLastCommand("recenter-top-bottom")
	run(t, f, "recenter-top-bottom")
	if got, want := f.Win().Top, 50; got != want {
		t.Fatalf("second press: Top = %d, want %d (top)", got, want)
	}

	f.SetLastCommand("recenter-top-bottom")
	run(t, f, "recenter-top-bottom")
	if got, want := f.Win().Top, 30; got != want {
		t.Fatalf("third press: Top = %d, want %d (bottom)", got, want)
	}

	f.SetLastCommand("recenter-top-bottom")
	run(t, f, "recenter-top-bottom")
	if got, want := f.Win().Top, 40; got != want {
		t.Fatalf("fourth press: Top = %d, want %d (back to centre)", got, want)
	}
}

func TestRecenterCycleResetsAfterAnotherCommand(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "z"
	}
	f := newMotionFake(t, lines...)
	f.Height = 21
	f.SetPoint(text.Pos{Line: 50, Col: 0})

	run(t, f, "recenter-top-bottom")
	f.SetLastCommand("recenter-top-bottom")
	run(t, f, "recenter-top-bottom") // now at "top"
	if got := f.Win().Top; got != 50 {
		t.Fatalf("Top = %d, want 50", got)
	}

	f.SetLastCommand("forward-char") // a different command intervenes
	run(t, f, "recenter-top-bottom")
	if got, want := f.Win().Top, 40; got != want {
		t.Fatalf("after reset: Top = %d, want %d (centre again)", got, want)
	}
}

func TestRecenterNeverSetsNegativeTop(t *testing.T) {
	f := newMotionFake(t, "one", "two", "three")
	f.Height = 21
	f.SetPoint(text.Pos{Line: 0, Col: 0})
	run(t, f, "recenter-top-bottom")
	if got := f.Win().Top; got < 0 {
		t.Fatalf("Top = %d, must never be negative", got)
	}
}
