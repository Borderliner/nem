package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"

	"github.com/Borderliner/nem/syntax"
	"os"
)

// fgOnScreen is the foreground colour of one rendered cell. Syntax styles set
// only a foreground, so this is what the whole feature comes down to.
func fgOnScreen(t *testing.T, scr tcell.SimulationScreen, x, y int) tcell.Color {
	t.Helper()
	cells, w, h := scr.GetContents()
	if x < 0 || y < 0 || x >= w || y >= h {
		t.Fatalf("cell (%d,%d) out of bounds %dx%d", x, y, w, h)
	}
	fg, _, _ := cells[y*w+x].Style.Decompose()
	return fg
}

// anyColourOnRow reports whether a row holds any cell with a non-default
// foreground, which is the coarse "is this highlighted at all" question.
func anyColourOnRow(t *testing.T, scr tcell.SimulationScreen, y int) bool {
	t.Helper()
	cells, w, _ := scr.GetContents()
	for x := 0; x < w; x++ {
		if fg, _, _ := cells[y*w+x].Style.Decompose(); fg != tcell.ColorDefault {
			return true
		}
	}
	return false
}

// A Go file must actually arrive on screen coloured. Every piece of this works
// in isolation - the lexer, the cache, the renderer - so only an end-to-end
// assertion can catch them being wired together wrongly.
func TestGoBufferIsHighlightedOnScreen(t *testing.T) {
	e, scr := newTestEditor(t)
	b := e.NewBuffer("main.go")
	b.SetPath(filepath.Join(t.TempDir(), "main.go"))
	if err := b.Insert(text.Pos{}, []rune("package main")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	e.Active().Visit(b)
	e.Active().Pt = text.Pos{}
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Fatal("a Go buffer rendered with no colour at all; the wiring is dead")
	}
	// "package" is a keyword and must not be the terminal default.
	if got := fgOnScreen(t, scr, gutterOffset(t, e), 0); got == tcell.ColorDefault {
		t.Error("the keyword is uncoloured")
	}
}

// gutterOffset is the first text column, so a test can address the start of a
// line without hard-coding the gutter's width.
func gutterOffset(t *testing.T, e *Editor) int {
	t.Helper()
	if !e.th.LineNumbers {
		return 0
	}
	// Buffers in these tests are short, so the gutter is one digit plus its
	// separating column.
	return 2
}

// A buffer with no file has no language, so it must render plain. *scratch* and
// *Buffer List* are the everyday cases.
func TestScratchBufferIsNotHighlighted(t *testing.T) {
	e, scr := newTestEditor(t, "package main")
	e.Redraw()
	if anyColourOnRow(t, scr, 0) {
		t.Error("*scratch* is coloured; a buffer with no file has no language to lex")
	}
}

// Saving a path-less buffer as main.go must switch it to the Go lexer. Without
// this the file stays plain until the editor restarts, which reads as
// highlighting being broken rather than as a stale lexer.
func TestWriteFileRetunesTheLexer(t *testing.T) {
	e, scr := newTestEditor(t, "package main")
	b := e.Buf()

	e.Redraw()
	if anyColourOnRow(t, scr, 0) {
		t.Fatal("setup: the buffer is coloured before it has a path")
	}

	if err := e.SaveBuffer(b, filepath.Join(t.TempDir(), "main.go")); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Error("after saving as main.go the buffer is still unhighlighted")
	}
}

// A killed buffer's cache holds one lexer state per line it had. Nothing else
// would ever remove it, so a long session would leak one per buffer opened.
func TestKillingABufferDropsItsHighlightCache(t *testing.T) {
	e, _ := newTestEditor(t, "alpha")
	b := e.NewBuffer("other.go")
	b.SetPath(filepath.Join(t.TempDir(), "other.go"))

	// Touch it so a cache exists.
	e.spansOf(b, 0)
	if _, ok := e.hl[b]; !ok {
		t.Fatal("setup: no cache was created")
	}

	if err := e.KillBuffer(b); err != nil {
		t.Fatalf("KillBuffer: %v", err)
	}
	if _, ok := e.hl[b]; ok {
		t.Error("the cache outlived the buffer it describes")
	}
}

// Editing must not leave stale colours behind. This is the whole point of the
// cache's invalidation, seen from the outside.
func TestEditingUpdatesTheColours(t *testing.T) {
	e, scr := newTestEditor(t)
	b := e.NewBuffer("edit.go")
	b.SetPath(filepath.Join(t.TempDir(), "edit.go"))
	if err := b.Insert(text.Pos{}, []rune("x")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	e.Active().Visit(b)
	e.Active().Pt = b.End()
	e.Redraw()

	if anyColourOnRow(t, scr, 0) {
		t.Fatal("setup: a bare identifier should not be coloured")
	}

	// Turn the line into a comment; every character should now be one colour.
	e.Active().Pt = text.Pos{}
	press(t, e, "/", "/")
	e.Redraw()

	if !anyColourOnRow(t, scr, 0) {
		t.Error("typing // did not colour the line as a comment")
	}
}

// A file is coloured end to end, from its definition to its spans.
func TestAFileIsColouredByItsLanguage(t *testing.T) {
	e, _ := newTestEditor(t, `def handler(self):`, `    return "hi"  # done`)
	e.Buf().SetPath(filepath.Join(t.TempDir(), "script.py"))
	e.retuneHighlight(e.Buf())

	if name := e.cacheFor(e.Buf()).Lexer().Name(); name != "python" {
		t.Fatalf("lexer = %q, want python", name)
	}
	var sawString, sawComment bool
	for ln := 0; ln < e.Buf().NumLines(); ln++ {
		for _, s := range e.spansOf(e.Buf(), ln) {
			sawString = sawString || s.Class == syntax.String
			sawComment = sawComment || s.Class == syntax.Comment
		}
	}
	if !sawString || !sawComment {
		t.Errorf("string %v, comment %v: a Python file lost its colours", sawString, sawComment)
	}
}

// userLanguages starts an editor whose config directory holds the given
// definitions, as ~/.config/nem/syntax would.
func userLanguages(t *testing.T, defs map[string]string) (*Editor, string) {
	e, dir, _ := userLanguagesScreen(t, defs)
	return e, dir
}

// userLanguagesScreen is userLanguages, with the screen to type into.
func userLanguagesScreen(t *testing.T, defs map[string]string) (*Editor, string, tcell.SimulationScreen) {
	t.Helper()
	cfg := t.TempDir()
	dir := filepath.Join(cfg, "syntax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range defs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, scr := newTestEditor(t, "x")
	_ = e.LoadConfig(filepath.Join(cfg, "init.lua"))
	return e, dir, scr
}

// A language of the user's own colours its files, and one named as nem's
// replaces it.
func TestUserLanguagesColourTheirFiles(t *testing.T) {
	e, _ := userLanguages(t, map[string]string{
		"ada.syntax": "language ada\nfiles *.adb\nignore-case\ncomment --\nkeywords procedure is begin end\n",
		"go.syntax":  "language go\nlike go\nkeywords must\n",
	})
	b := e.Buf()
	b.SetPath(filepath.Join(t.TempDir(), "main.adb"))
	e.retuneHighlight(b)
	if name := e.cacheFor(b).Lexer().Name(); name != "ada" {
		t.Fatalf("main.adb is lexed as %q", name)
	}
	if lang := e.LanguageOf(b); lang == nil || lang.Name() != "ada" {
		t.Errorf("LanguageOf(main.adb) = %v", lang)
	}
	b.SetPath(filepath.Join(t.TempDir(), "main.go"))
	e.retuneHighlight(b)
	spans, _ := e.cacheFor(b).Lexer().Lex([]rune("must(x)"), 0)
	if len(spans) == 0 || spans[0].Class != syntax.Keyword {
		t.Errorf("the user's go did not replace nem's: %v", spans)
	}
}

// Saving a definition reloads the languages and recolours what is open, so
// a language is written and tried without leaving nem.
func TestSavingADefinitionRecolours(t *testing.T) {
	e, dir := userLanguages(t, map[string]string{
		"nim.syntax": "language nim\nfiles *.nim\nkeywords proc\n",
	})
	code := e.Buf()
	code.SetPath(filepath.Join(t.TempDir(), "a.nim"))
	code.Insert(text.Pos{}, []rune("echo 1\n"))
	e.retuneHighlight(code)
	if spans := e.cacheFor(code).Spans(code, 0); len(spans) > 1 || len(spans) == 1 && spans[0].Class == syntax.Keyword {
		t.Fatalf("echo is coloured before the definition says so: %v", spans)
	}

	def := e.NewBuffer("nim.syntax")
	def.Insert(text.Pos{}, []rune("language nim\nfiles *.nim\nkeywords proc echo\n"))
	if err := e.SaveBuffer(def, filepath.Join(dir, "nim.syntax")); err != nil {
		t.Fatal(err)
	}
	spans := e.cacheFor(code).Spans(code, 0)
	if len(spans) == 0 || spans[0].Class != syntax.Keyword {
		t.Errorf("after saving the definition echo is %v, want a keyword", spans)
	}

	// A mistake saved is reported, and the languages that were loaded stay.
	def.Insert(text.Pos{Line: 2}, []rune("match keyowrd x\n"))
	if err := e.SaveBuffer(def, ""); err != nil {
		t.Fatal(err)
	}
	if msg := e.Message(); !strings.Contains(msg, "nim.syntax:3") {
		t.Errorf("echo %q does not report the broken line", msg)
	}
}

// A broken definition is reported when nem starts, and costs only itself.
func TestABrokenDefinitionIsReported(t *testing.T) {
	e, _ := userLanguages(t, map[string]string{
		"bad.syntax":  "language bad\nfiles *.bad\nmatch keyowrd x\n",
		"good.syntax": "language good\nfiles *.good\nkeywords yes\n",
	})
	if msg := e.Message(); !strings.Contains(msg, "bad.syntax:3") {
		t.Errorf("echo %q does not report the broken definition", msg)
	}
	if e.languages().Language("good") == nil || e.languages().Language("go") == nil {
		t.Error("one broken definition cost the others")
	}
}

// An extensionless script must highlight from its shebang using the bundled
// rules, with no nano installed. Before this, only nano's definitions were
// given the first line, so the case that motivated bundling did not work.
func TestExtensionlessScriptHighlightsFromItsShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy") // deliberately no extension
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	b, err := text.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	e, _ := newTestEditor(t)
	lex := e.lexerFor(b)
	if _, plain := lex.(syntax.PlainLexer); plain {
		t.Fatal("an extensionless #!/bin/sh script got the plain lexer")
	}
	if got := lex.Name(); got != "sh" {
		t.Errorf("lexer = %q, want the bundled sh rules", got)
	}
}

// And a file with neither an extension nor a shebang stays plain.
func TestNamelessUnmarkedFileStaysPlain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes")
	if err := os.WriteFile(path, []byte("just some prose\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	b, err := text.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	e, _ := newTestEditor(t)
	if _, plain := e.lexerFor(b).(syntax.PlainLexer); !plain {
		t.Error("a file with no extension and no shebang should stay plain")
	}
}

// M-; writes the comment of the language the user defined, as it colours it.
func TestCommentFollowsTheUsersLanguage(t *testing.T) {
	e, _ := userLanguages(t, map[string]string{
		"ada.syntax": "language ada\nfiles *.adb\ncomment --\n",
	})
	b := e.Buf()
	b.SetPath(filepath.Join(t.TempDir(), "main.adb"))
	b.Insert(text.Pos{}, []rune("Put_Line (X);"))
	press(t, e, "M-;")
	if got := b.Line(0).String(); !strings.HasPrefix(got, "-- Put_Line (X);") {
		t.Errorf("M-; made %q, want Ada's comment", got)
	}
}

// edit-language opens the definition of the language on screen: a copy of
// nem's own to change, which saved replaces nem's; or the outline of a
// language nem does not know, which saved colours its files.
func TestEditLanguage(t *testing.T) {
	e, dir, scr := userLanguagesScreen(t, nil)
	e.Buf().SetPath(filepath.Join(t.TempDir(), "main.go"))

	feed(t, scr, key(t, "RET"))
	if err := e.Run("edit-language"); err != nil {
		t.Fatal(err)
	}
	b := e.Buf()
	if b.Path() != filepath.Join(dir, "go.syntax") {
		t.Fatalf("editing %s, want the user's go.syntax", b.Path())
	}
	if src, _ := syntax.BuiltinSource("go"); b.String() != src {
		t.Error("the user's go.syntax did not start as nem's own")
	}

	feed(t, scr, txt("ada"), key(t, "M-RET"))
	if err := e.Run("edit-language"); err != nil {
		t.Fatal(err)
	}
	b = e.Buf()
	if !strings.Contains(b.String(), "language ada\n") {
		t.Fatalf("a new language starts as %q", b.String())
	}
	if err := e.SaveBuffer(b, ""); err != nil {
		t.Fatal(err)
	}
	if e.languages().Language("ada") == nil {
		t.Error("the outline saved as it was did not load")
	}
}
