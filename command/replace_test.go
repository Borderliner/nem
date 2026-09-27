package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

// qrr runs query-replace-regexp on lines, replacing pat with to and answering
// each match from answers, and returns the fake afterwards.
func qrr(t *testing.T, pat, to, answers string, lines ...string) *commandtest.Fake {
	t.Helper()
	f := commandtest.New(lines...)
	f.Replies = []string{pat, to}
	f.Chars = []rune(answers)
	if err := tryRun(t, f, "query-replace-regexp"); err != nil {
		t.Fatalf("query-replace-regexp %q with %q: %v", pat, to, err)
	}
	return f
}

// Every match, with ! for all of them, and emacs's replacement syntax
// expanded as Go's would be - with a typed $ staying a dollar sign.
func TestQueryReplaceRegexpReplacements(t *testing.T) {
	for _, tc := range []struct {
		name, line, pat, to, want string
	}{
		{"plain", "foo1 bar22", `[0-9]+`, "#", "foo# bar#"},
		{"the whole match", "a bc", `[a-z]+`, `<\&>`, "<a> <bc>"},
		{"the whole match as \\0", "a bc", `[a-z]+`, `(\0)`, "(a) (bc)"},
		{"groups", "x=1, y=2", `(\w)=(\d)`, `\2=\1`, "1=x, 2=y"},
		{"a group that did not take part", "ab", `a(z)?`, `[\1]`, "[]b"},
		{"a backslash", "a b", `a`, `\\`, `\ b`},
		{"a dollar sign", "cost 5", `(\d)`, `$1 $$ \1$`, "cost $1 $$ 5$"},
		{"a dollar sign before a name", "x", `(?P<n>x)`, `${n}`, "${n}"},
		{"nothing", "a-b-c", `-`, "", "abc"},
		{"not across lines", "a\nb", `a\nb`, "X", "a\nb"},
		{"to the end of each line", "ab\ncd", `$`, ";", "ab;\ncd;"},
		{"the start of each line", "ab\ncd", `^`, "> ", "> ab\n> cd"},
		// An empty match is not found again where the last one was
		// replaced, nor straight after a match.
		{"empty matches", "abc", `x*`, "-", "-a-b-c-"},
		{"empty matches after a match", "baaac", `a*`, "-", "-b-c-"},
		// Searching on from the middle of a line keeps the line's context:
		// ^ is its start alone, and \b sees the character before.
		{"^ only at the start", "aaa", `^a`, "X", "Xaa"},
		{"\\b sees what came before", "bb bb", `\bb`, "X", "Xb Xb"},
		// Replacing a match with text the pattern matches does not loop.
		{"growing", "aaa", `a`, "aa", "aaaaaa"},
	} {
		f := qrr(t, tc.pat, tc.to, "!", strings.Split(tc.line, "\n")...)
		if got := f.Text(); got != tc.want {
			t.Errorf("%s: %q with %q in %q = %q, want %q", tc.name, tc.pat, tc.to, tc.line, got, tc.want)
		}
	}
}

// As with query-replace, a pattern all in small letters ignores case, and
// one with a capital does not. A capital in an escape does not count.
func TestQueryReplaceRegexpSmartCase(t *testing.T) {
	for _, tc := range []struct {
		line, pat, want string
	}{
		{"Foo foo FOO", `foo`, "X X X"},
		{"Foo foo FOO", `Foo`, "X foo FOO"},
		{"aB ab", `a\S`, "X X"},
		{"aB ab", `a\pL`, "X X"},
		// Folding, a class of capitals takes small letters too, as emacs's
		// [[:upper:]] does while case is ignored.
		{"aB ab", `a\p{Lu}`, "X X"},
		{"aB ab", `(?-i)a\p{Lu}`, "X ab"},
		{"aB ab", `(?P<First>a)b`, "X X"},
	} {
		f := qrr(t, tc.pat, "X", "!", tc.line)
		if got := f.Text(); got != tc.want {
			t.Errorf("%q in %q = %q, want %q", tc.pat, tc.line, got, tc.want)
		}
	}
	if !command.FoldCaseRegexp(`\W+`) || command.FoldCaseRegexp(`\WX`) {
		t.Error("FoldCaseRegexp counts the capital of an escape, or misses one after it")
	}
}

func TestQueryReplaceRegexpAsksAboutEachMatch(t *testing.T) {
	f := qrr(t, `a(\d)`, `b\1`, "yny", "a1 a2 a3")
	textIs(t, f, "b1 a2 b3")
	if got := f.CharPrompts[0]; got != `Query replacing regexp a(\d) with b\1 (y/n/!/q): ` {
		t.Errorf("prompt %q", got)
	}
	if got := f.Prompts; len(got) != 2 || got[0] != "Query replace regexp: " || got[1] != `Query replace regexp a(\d) with: ` {
		t.Errorf("prompts %q", got)
	}
	if got := lastEcho(t, f.Echoes); got != "Replaced 2 occurrences" {
		t.Errorf("echo %q", got)
	}

	f = qrr(t, `\d`, "#", "yq", "1 2 3")
	textIs(t, f, "# 2 3")
	if got := lastEcho(t, f.Echoes); got != "Replaced 1 occurrence" {
		t.Errorf("echo %q", got)
	}
}

// It starts from point, as query-replace does.
func TestQueryReplaceRegexpStartsAtPoint(t *testing.T) {
	f := commandtest.New("a1", "a2", "a3")
	f.SetPoint(text.Pos{Line: 1, Col: 1})
	f.Replies = []string{`a\d`, "X"}
	f.Chars = []rune("!")
	if err := tryRun(t, f, "query-replace-regexp"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a1\na2\nX")
}

// A mistake in the pattern is reported before the replacement is asked for;
// one in the replacement before anything changes.
func TestQueryReplaceRegexpRefusesMistakes(t *testing.T) {
	f := commandtest.New("abc")
	f.Replies = []string{`(`}
	if err := tryRun(t, f, "query-replace-regexp"); err == nil || !strings.Contains(err.Error(), "invalid regexp") {
		t.Errorf("a bad pattern: err = %v", err)
	}
	if len(f.Prompts) != 1 {
		t.Errorf("a bad pattern went on to prompt %q", f.Prompts)
	}

	for _, to := range []string{`\n`, `x\`, `\2`} {
		f = commandtest.New("abc")
		f.Replies = []string{`(b)`, to}
		if err := tryRun(t, f, "query-replace-regexp"); err == nil {
			t.Errorf("replacement %q: no error", to)
		}
		textIs(t, f, "abc")
	}
}

func TestQueryReplaceRegexpInAReadOnlyBuffer(t *testing.T) {
	f := commandtest.New("abc")
	f.Buf().SetReadOnly(true)
	if err := tryRun(t, f, "query-replace-regexp"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	if len(f.Prompts) != 0 {
		t.Error("prompted in a read-only buffer")
	}
}

// Matches the buffer refuses are passed over, not offered, and counted.
func TestQueryReplaceRegexpSkipsWhatCannotBeEdited(t *testing.T) {
	f := commandtest.New("a1", "a2", "a3")
	f.Buf().SetEditGuard(func(from, _ text.Pos, _ []rune) error {
		if from.Line == 1 {
			return errors.New("fixed")
		}
		return nil
	})
	f.Replies = []string{`\d`, "#"}
	f.Chars = []rune("yy")
	if err := tryRun(t, f, "query-replace-regexp"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a#\na2\na#")
	if got := lastEcho(t, f.Echoes); got != "Replaced 2 occurrences (skipped 1 that cannot be edited)" {
		t.Errorf("echo %q", got)
	}
}
