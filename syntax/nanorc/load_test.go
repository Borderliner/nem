package nanorc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/syntax/nanorc"
)

// write puts a .nanorc in a directory. Fixtures are written here rather than
// copied from /usr/share/nano: those files are GPL and must not enter this
// repository. See the package comment.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMatchesByExtension(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "demo.nanorc", "syntax demo \"\\.demo$\"\ncolor green \"\\<(hi)\\>\"\n")

	set, probs := nanorc.Load(dir)
	if len(probs) != 0 {
		t.Fatalf("problems: %v", probs)
	}
	if got := set.For("/home/x/thing.demo", ""); got == nil {
		t.Fatal("no lexer for a .demo file")
	} else if got.Name() != "demo" {
		t.Errorf("Name = %q, want demo", got.Name())
	}
	if got := set.For("/home/x/thing.other", ""); got != nil {
		t.Errorf("got a lexer %q for an unmatched extension, want nil", got.Name())
	}
}

// A script with no extension is matched on its shebang, which is the only way
// those files ever get highlighted.
func TestLoadMatchesByHeader(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "demo.nanorc",
		"syntax demo \"\\.demo$\"\nheader \"^#!.*/demosh\"\ncolor green \"\\<(hi)\\>\"\n")

	set, _ := nanorc.Load(dir)
	if got := set.For("/usr/local/bin/runme", "#!/usr/bin/demosh"); got == nil {
		t.Error("no lexer matched the shebang")
	}
	if got := set.For("/usr/local/bin/runme", "#!/bin/sh"); got != nil {
		t.Errorf("matched %q on an unrelated shebang", got.Name())
	}
	// An unknown extension with no first line must still not match.
	if got := set.For("/usr/local/bin/runme", ""); got != nil {
		t.Errorf("matched %q with no extension and no header", got.Name())
	}
}

func TestMissingDirectoryIsNotAnError(t *testing.T) {
	set, probs := nanorc.Load(filepath.Join(t.TempDir(), "nope"))
	if len(probs) != 0 {
		t.Errorf("problems for a missing directory: %v", probs)
	}
	if set.Len() != 0 {
		t.Errorf("Len = %d, want 0", set.Len())
	}
	// A nil-safe Set is what lets a caller skip checking before every use.
	if got := set.For("x.demo", ""); got != nil {
		t.Errorf("For on an empty set = %v, want nil", got)
	}
}

func TestMalformedFileIsReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "bad.nanorc", "color green \"x\"\n") // no syntax directive
	write(t, dir, "good.nanorc", "syntax good \"\\.good$\"\ncolor green \"\\<(a)\\>\"\n")

	set, probs := nanorc.Load(dir)
	if len(probs) == 0 {
		t.Error("want a reported problem for the malformed file")
	}
	if set.For("x.good", "") == nil {
		t.Error("the malformed file cost the good one its language")
	}
}

// The user's own directory is searched last so a hand-written definition
// replaces the system one rather than competing with it.
func TestLaterDirectoryWins(t *testing.T) {
	sys, usr := t.TempDir(), t.TempDir()
	write(t, sys, "demo.nanorc", "syntax demo \"\\.demo$\"\ncolor green \"\\<(sys)\\>\"\n")
	write(t, usr, "demo.nanorc", "syntax demo \"\\.mine$\"\ncolor green \"\\<(usr)\\>\"\n")

	set, _ := nanorc.Load(sys, usr)
	if set.Len() != 1 {
		t.Errorf("Len = %d, want the two definitions merged into one language", set.Len())
	}
	if set.For("x.mine", "") == nil {
		t.Error("the user's definition did not take effect")
	}
	if got := set.For("x.demo", ""); got != nil {
		t.Errorf("the system definition still matches %q; it should have been replaced", got.Name())
	}
}

// The value of the whole feature is how much of nano's real corpus works, so
// the number is asserted rather than assumed. It is skipped where nano is not
// installed, which is normal and not a failure.
func TestRealCorpusLoads(t *testing.T) {
	if _, err := os.Stat("/usr/share/nano"); err != nil {
		t.Skip("nano is not installed here")
	}
	set, probs := nanorc.Load(nanorc.DefaultDirs()...)

	if set.Len() < 35 {
		t.Errorf("loaded %d languages, want at least 35", set.Len())
	}
	// One pattern in nano's own objc.nanorc is an unbalanced regex - one open
	// paren, two closes - so a small number of skips is expected and is not
	// nem's bug. A jump here means the parser has regressed.
	if len(probs) > 5 {
		t.Errorf("%d problems, want at most a handful:", len(probs))
		for i, p := range probs {
			if i < 10 {
				t.Logf("   %v", p)
			}
		}
	}
	t.Logf("loaded %d languages with %d skipped patterns", set.Len(), len(probs))

	// Spot-check that common languages actually resolve.
	for _, tc := range []struct{ path, want string }{
		{"main.c", "c"}, {"script.py", "python"}, {"lib.rs", "rust"},
		{"conf.yaml", "yaml"}, {"page.html", "html"}, {"run.sh", "sh"},
	} {
		got := set.For(tc.path, "")
		if got == nil {
			t.Errorf("%s: no lexer", tc.path)
			continue
		}
		if got.Name() != tc.want {
			t.Errorf("%s: lexer %q, want %q", tc.path, got.Name(), tc.want)
		}
	}
}

// Real rules against real lines, checking the contract holds on text the
// fixtures cannot cover.
func TestRealCorpusLexesCleanly(t *testing.T) {
	if _, err := os.Stat("/usr/share/nano"); err != nil {
		t.Skip("nano is not installed here")
	}
	set, _ := nanorc.Load(nanorc.DefaultDirs()...)
	lex := set.For("x.c", "")
	if lex == nil {
		t.Skip("no c definition")
	}
	var st syntax.State
	for _, s := range []string{
		`#include <stdio.h>`,
		`/* a block comment`,
		`   still inside */`,
		`int main(void) { printf("hi %d\n", 42); return 0; }`,
		`// trailing`,
		`const char *s = "with \"escapes\" inside";`,
		`日本語 = 1; // multibyte`,
	} {
		line := []rune(s)
		var spans []syntax.Span
		spans, st = lex.Lex(line, st)
		prev := -1
		for _, sp := range spans {
			if sp.Start < 0 || sp.End > len(line) || sp.End <= sp.Start || sp.Start < prev {
				t.Errorf("%q: bad span %v in a line of %d runes", s, sp, len(line))
			}
			prev = sp.End
		}
	}
}

func BenchmarkLexRealLine(b *testing.B) {
	if _, err := os.Stat("/usr/share/nano"); err != nil {
		b.Skip("nano is not installed here")
	}
	set, _ := nanorc.Load(nanorc.DefaultDirs()...)
	lex := set.For("x.c", "")
	if lex == nil {
		b.Skip("no c definition")
	}
	line := []rune(`    for (int i = 0; i < n; i++) { printf("%d\n", i); } // loop`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}

func BenchmarkLexPythonLine(b *testing.B) {
	if _, err := os.Stat("/usr/share/nano"); err != nil {
		b.Skip("nano is not installed here")
	}
	set, _ := nanorc.Load(nanorc.DefaultDirs()...)
	lex := set.For("x.py", "")
	if lex == nil {
		b.Skip("no python definition")
	}
	line := []rune(`    def handler(self, request, timeout=30):  # entry point`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lex.Lex(line, 0)
	}
}

func TestSetNamesAreSorted(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "b.nanorc", "syntax bee \"\\.b$\"\n")
	write(t, dir, "a.nanorc", "syntax ay \"\\.a$\"\n")
	got := strings.Join((func() []string {
		s, _ := nanorc.Load(dir)
		return s.Names()
	})(), ",")
	if got != "ay,bee" {
		t.Errorf("Names = %q, want sorted", got)
	}
}
