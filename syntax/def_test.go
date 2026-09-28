package syntax

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// def compiles a definition written in a test, failing on any error.
func def(t *testing.T, src string) *Language {
	t.Helper()
	d, err := ParseDef("test.syntax", strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseDef: %v", err)
	}
	l, err := Compile(d)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return l
}

// Every mistake a definition can make is reported with its line, and says
// what is wrong: a user writing one sees this, and a vague error sends them
// guessing.
func TestDefinitionErrors(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"no language", "keywords a b\n", "before language"},
		{"empty", "# nothing\n", "no language directive"},
		{"two languages", "language a\nlanguage b\n", "second language"},
		{"bad name", "language Ada\n", "lower-case"},
		{"unknown directive", "language a\nkeyword foo\n", `unknown directive "keyword"`},
		{"unknown class", "language a\nmatch keyowrd x\n", "which class"},
		{"unknown declared class", "language a\ndeclares keyowrd def\n", `unknown class "keyowrd"`},
		{"bad pattern", "language a\nmatch keyword \\b(unclosed\n", "missing closing )"},
		{"bad glob", "language a\nfiles [a\n", "files"},
		{"group not a class", "language a\nmatch (?P<name>x)\n", `group "name" is not a class`},
		{"class and groups", "language a\nmatch keyword (?P<type>x)\n", "not both"},
		{"like too late", "language a\nkeywords x\nlike b\n", "straight after language"},
		{"region without end", "language a\nregion string\n    start x\n", "a start and an end"},
		{"region bad ref", "language a\nregion string\n    start (x)\n    end \\2\n", "start has 1 groups"},
		{"field outside region", "language a\n    start x\n", "belongs to a region"},
		{"unknown field", "language a\nregion string\n    begin x\n", `unknown region field "begin"`},
		{"line comment nested", "language a\ncomment -- nested\n", "only a comment with an end"},
		{"string option", "language a\nstring \" ' loud\n", `"loud"`},
		{"calls", "language a\ncalls maybe\n", "paren, lisp or none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDef("x.syntax", strings.NewReader(tc.src))
			if err == nil {
				t.Fatalf("accepted:\n%s", tc.src)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "x.syntax") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

// Words are words: a keyword inside a longer name is not one, and with
// ignore-case they match in any case.
func TestWords(t *testing.T) {
	l := def(t, "language a\nkeywords begin end\ntypes Integer\nconstants True\nfunctions put\n")
	assertClass(t, l, "begin x end", "begin", Keyword)
	assertClass(t, l, "beginning", "beginning", Plain)
	assertClass(t, l, "x : Integer", "Integer", Type)
	assertClass(t, l, "x := True", "True", Constant)
	assertClass(t, l, "put x", "put", Function)
	assertClass(t, l, "BEGIN", "BEGIN", Plain)

	ci := def(t, "language a\nignore-case\nkeywords begin\ntypes Integer\n")
	assertClass(t, ci, "BEGIN", "BEGIN", Keyword)
	assertClass(t, ci, "x : INTEGER", "INTEGER", Type)
}

// word-chars makes runes part of a name, so a Lisp name with a - in it is
// one name, and a keyword inside it is not a keyword.
func TestWordChars(t *testing.T) {
	l := def(t, "language a\ncalls none\nword-chars -?!\nkeywords defun if\n")
	assertClass(t, l, "(defun empty? (x))", "empty?", Plain)
	assertClass(t, l, "(if-let x)", "if-let", Plain)
	assertClass(t, l, "(if x)", "if", Keyword)
}

func TestDeclares(t *testing.T) {
	l := def(t, "language a\nkeywords procedure package body is\ndeclares function procedure\ndeclares type package\n")
	assertClass(t, l, "procedure Main is", "Main", Function)
	// A keyword between the declaring word and the name keeps what the
	// name will be.
	assertClass(t, l, "package body Pkg is", "Pkg", Type)
	// Anything but a blank or a keyword in between ends it.
	assertClass(t, l, "procedure (x) y", "y", Plain)
}

func TestCalls(t *testing.T) {
	paren := def(t, "language a\n")
	assertClass(t, paren, "f(x)", "f", Function)
	assertClass(t, paren, "f (x)", "f", Function)
	assertClass(t, paren, "f x", "f", Plain)

	lisp := def(t, "language a\ncalls lisp\nword-chars -\n")
	assertClass(t, lisp, "(map-car f xs)", "map-car", Function)
	assertClass(t, lisp, "(map-car f xs)", "xs", Plain)
	assertClass(t, lisp, "f(x)", "f", Plain)

	none := def(t, "language a\ncalls none\n")
	assertClass(t, none, "f(x)", "f", Plain)
}

func TestNumbers(t *testing.T) {
	l := def(t, "language a\n")
	for _, lit := range []string{"42", "1_000", "0x1f", "0b101", "0o17", "3.14", "1e9", "1E-9", ".5", "10UL", "1.5f", "2i", "0x1p-3"} {
		assertSpanCovers(t, l, "x = "+lit+" + y", lit, Number)
	}
	assertClass(t, l, "x1 = 1", "x1", Plain)
	// A dot after a dot is a range, not the start of a fraction.
	assertSpanCovers(t, l, "1..9", "1", Number)
	assertSpanCovers(t, l, "1..9", "9", Number)
	assertClass(t, l, "1..9", "..", Punctuation)

	none := def(t, "language a\nnumbers none\n")
	assertClass(t, none, "x = 42", "42", Plain)

	// A pattern of its own, which may start with a sign; and a number that
	// runs into a name is the name, as Lisp's 1+ is.
	lisp := def(t, "language a\nword-chars +-\nnumbers [-+]?[0-9]+\n")
	assertSpanCovers(t, lisp, "(- -1 2)", "-1", Number)
	assertClass(t, lisp, "(1+ x)", "1+", Plain)
}

func TestOperatorsAndPunctuation(t *testing.T) {
	l := def(t, "language a\noperators :=\npunctuation ;\n")
	assertClass(t, l, "x := y;", ":=", Operator)
	assertClass(t, l, "x := y;", ";", Punctuation)
	assertClass(t, l, "x + y", "+", Plain)

	d := def(t, "language a\n")
	assertClass(t, d, "a + b", "+", Operator)
	assertClass(t, d, "f(a, b)", ",", Punctuation)
}

func TestComments(t *testing.T) {
	l := def(t, "language a\ncomment --\ncomment (* *) nested\nstring \"\n")
	assertSpanCovers(t, l, "x -- note", "-- note", Comment)
	assertSpanCovers(t, l, `s := "a -- b"`, `"a -- b"`, String)

	// A nested comment closes only when every comment opened in it has.
	_, spans := lexDoc(l, "(* a (* b *) still\nand still *) x\n")
	if classAt(spans[0], 16) != Comment || classAt(spans[1], 4) != Comment {
		t.Error("the inner close ended the nested comment")
	}
	if classAt(spans[1], 14) == Comment {
		t.Error("the outer close did not end it")
	}
}

func TestStrings(t *testing.T) {
	l := def(t, "language a\nstring \" doubled\nstring ' escape ^\nstring ` raw multiline\n")
	assertSpanCovers(t, l, `s := "say ""hi"" now" & x`, `"say ""hi"" now"`, String)
	assertSpanCovers(t, l, "s := 'it^'s' + x", "'it^'s'", String)
	assertSpanCovers(t, l, "s := `a\\` + x", "`a\\`", String)

	// A string that is not multiline ends with the line, however it ends.
	_, st := l.Lex([]rune(`s := "open`), 0)
	if st != 0 {
		t.Errorf("an unclosed single-line string carried state %v", st)
	}
	_, st = l.Lex([]rune("s := `open"), 0)
	if st == 0 {
		t.Error("an unclosed multiline string carried nothing")
	}
}

// A region's end can be made of what its start captured, which is how a
// delimiter chosen by the writer - Lua's [==[, a here-document's word - is
// closed only by its own.
func TestRegionEndFromTheStart(t *testing.T) {
	l := def(t, "language a\nregion string\n    start <<([A-Z]+)\n    end ^\\1$\n    multiline\n")
	_, spans := lexDoc(l, "cat <<EOF\nEND\nEOF is not alone\nEOF\nx\n")
	for i := 1; i <= 3; i++ {
		if classAt(spans[i], 0) != String {
			t.Errorf("line %d should be inside the here-document", i)
		}
	}
	if classAt(spans[4], 0) == String {
		t.Error("the here-document did not end at its word")
	}

	// The same capture on another line leaves the same state, which is what
	// lets the highlight cache stop.
	_, a := l.Lex([]rune("x <<ABC"), 0)
	_, b := l.Lex([]rune("y  <<ABC"), 0)
	_, c := l.Lex([]rune("y  <<XYZ"), 0)
	if a != b || a == c {
		t.Errorf("states: <<ABC %v and %v, <<XYZ %v", a, b, c)
	}
}

// A region that is not multiline, with a regexp end, ends with the line.
func TestRegionWithEscapePattern(t *testing.T) {
	l := def(t, "language a\nregion string\n    start %\\{\n    end \\}\n    escape \\\\.\n")
	assertSpanCovers(t, l, `x %{a\}b} y`, `%{a\}b}`, String)
}

// Among regions starting at one place, the longest start wins, whatever the
// order they were written in.
func TestLongestRegionStartWins(t *testing.T) {
	l := def(t, "language a\nstring \"\nstring \"\"\" multiline\ncomment --\ncomment --[[ ]]\n")
	assertSpanCovers(t, l, `x = """a " b""" y`, `"""a " b"""`, String)
	_, st := l.Lex([]rune("--[[ open"), 0)
	if st == 0 {
		t.Error("--[[ opened a line comment, not the block")
	}
}

// Match rules come first, in the order written, and a named group colours
// only itself: what follows the last one is read again, which is how a
// pattern looks ahead.
func TestMatch(t *testing.T) {
	l := def(t, "language a\nkeywords if\nmatch (?P<function>\\w+)\\s*=\\s*function\nmatch constant @\\w+\nmatch type ing\\b\nstring \"\n")
	assertClass(t, l, "handler = function(x)", "handler", Function)
	assertClass(t, l, "handler = function(x)", "=", Operator)
	assertClass(t, l, "@decorator", "@decorator", Constant)
	// Tokens are read in order, so a match inside a string is the string.
	assertSpanCovers(t, l, `s = "@x"`, `"@x"`, String)
	// A name is read whole, so a match does not start inside one.
	assertClass(t, l, "boxing", "boxing", Plain)
	assertClass(t, l, "user@host", "@host", Constant)
}

// ^ is the start of the line, not of what is left of it.
func TestCaretIsTheStartOfTheLine(t *testing.T) {
	l := def(t, "language a\nmatch keyword ^#\\w+\nmatch constant x|^y\n")
	assertClass(t, l, "#include", "#include", Keyword)
	assertClass(t, l, "a #include", "#include", Plain)
	assertClass(t, l, "y and x", "y", Constant)
	assertClass(t, l, "y and x", "x", Constant)
	line := []rune("a y")
	spans, _ := l.Lex(line, 0)
	if classAt(spans, 2) == Constant {
		t.Error("^y matched after the start of the line")
	}
}

// like starts from another language: its words and rules, not its files,
// unless it is the language itself. What follows adds, and wins.
func TestLike(t *testing.T) {
	base, _ := ParseDef("b.syntax", strings.NewReader("language b\nfiles *.b\nkeywords if\ntypes int\ncomment #\n"))
	own, _ := ParseDef("c.syntax", strings.NewReader("language c\nlike b\nfiles *.c\nkeywords int\n"))
	l, err := Compile(merge(base, own))
	if err != nil {
		t.Fatal(err)
	}
	assertClass(t, l, "if x", "if", Keyword)
	assertClass(t, l, "int x", "int", Keyword)
	assertSpanCovers(t, l, "x # note", "# note", Comment)
	if got := l.Def().Files; len(got) != 1 || got[0] != "*.c" {
		t.Errorf("files %v; a new language must not claim what it is like", got)
	}

	same, _ := ParseDef("b.syntax", strings.NewReader("language b\nlike b\nfiles *.bb\n"))
	if got := merge(base, same).Files; len(got) != 2 {
		t.Errorf("files %v; a language like itself keeps its files", got)
	}
}

// A user's definitions: a new language, one replacing nem's, one starting
// from nem's, and broken ones reported without costing the rest.
func TestLoadUserDefinitions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ada.syntax", "language ada\nfiles *.adb *.ads\nignore-case\ncomment --\nkeywords procedure is begin end\n")
	write("go.syntax", "language go\nlike go\nkeywords must\n")
	write("gotmpl.syntax", "language gotmpl\nlike go\nfiles *.gotmpl\n")
	write("broken.syntax", "language broken\nmatch keyowrd x\n")
	write("loop1.syntax", "language loop1\nlike loop2\n")
	write("loop2.syntax", "language loop2\nlike loop1\n")
	write("notes.txt", "not a definition")

	set, errs := Load(dir)
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"broken.syntax:2", "circle"} {
		if !strings.Contains(all, want) {
			t.Errorf("errors %q do not mention %q", all, want)
		}
	}

	ada := set.For("main.adb", "")
	if ada.Name() != "ada" {
		t.Fatalf("main.adb is %s", ada.Name())
	}
	assertClass(t, ada, "PROCEDURE Main IS", "PROCEDURE", Keyword)

	g := set.For("main.go", "")
	assertClass(t, g, "must(x)", "must", Keyword)
	assertClass(t, g, "func f()", "func", Keyword)
	if set.Language("go") == Builtin().Language("go") {
		t.Error("the user's go did not replace nem's")
	}
	if got := set.For("a.gotmpl", "").Name(); got != "gotmpl" {
		t.Errorf("a.gotmpl is %s", got)
	}
	if got := set.For("main.go", "").Name(); got != "go" {
		t.Errorf("main.go is %s: gotmpl, being like go, took its files", got)
	}
	if set.Language("python") == nil {
		t.Error("nem's own languages went missing beside the user's")
	}

	if _, errs := Load(filepath.Join(dir, "absent")); len(errs) != 0 {
		t.Errorf("a missing directory is no error: %v", errs)
	}
}

func TestLanguageComment(t *testing.T) {
	for _, tc := range []struct{ name, start, end string }{
		{"go", "//", ""},
		{"python", "#", ""},
		{"lua", "--", ""},
		{"xml", "<!--", "-->"},
		{"css", "/*", "*/"},
		{"scss", "//", ""},
		{"markdown", "<!--", "-->"},
	} {
		start, end, ok := lang(t, tc.name).Comment()
		if !ok || start != tc.start || end != tc.end {
			t.Errorf("%s comments with %q %q %v, want %q %q", tc.name, start, end, ok, tc.start, tc.end)
		}
	}
	if _, _, ok := lang(t, "json").Comment(); !ok {
		t.Error("json has JSONC's comments")
	}
	if _, _, ok := lang(t, "yaml").Comment(); ok {
		t.Error("yaml's comments are a match rule, since # starts one only after a blank; none is listed")
	}
}
