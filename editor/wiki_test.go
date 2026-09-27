package editor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Borderliner/nem/keymap"
)

// The wiki's reference pages - every key, and every command - are written
// from the code, so they cannot fall behind it. The pages live in docs/wiki
// beside the ones written by hand, and are published from there to the
// GitHub wiki. This test builds them afresh and fails when the copies in
// docs/wiki differ: a key rebound or a command added without them fails
// the build. NEM_UPDATE_WIKI=1 writes them instead.

// wikiDir is where the wiki's pages are kept, from this package.
const wikiDir = "../docs/wiki"

// wikiNotice heads every generated page.
const wikiNotice = "<!-- Generated from nem's code by TestWikiReferenceIsCurrent in editor/wiki_test.go.\n" +
	"     Do not edit: change the code, then run NEM_UPDATE_WIKI=1 go test ./editor -run TestWikiReferenceIsCurrent. -->\n\n"

func TestWikiReferenceIsCurrent(t *testing.T) {
	e, _ := newTestEditor(t)
	pages := map[string]string{
		"Keybindings.md": keybindingsPage(t, e),
		"Commands.md":    commandsPage(e),
	}
	for name, want := range pages {
		path := filepath.Join(wikiDir, name)
		if os.Getenv("NEM_UPDATE_WIKI") != "" {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("docs/wiki/%s does not match the code. Run\n\tNEM_UPDATE_WIKI=1 go test ./editor -run TestWikiReferenceIsCurrent\nand publish the wiki.", name)
		}
	}
}

// bindingGroup is a heading and the bindings under it, in the order the
// code lists them.
type bindingGroup struct {
	heading  string
	bindings []struct{ Spec, Command string }
}

// defaultBindingGroups splits defaultBindings into its sections. The
// sections are the comments in the table - "// Motion.", "// Files." - read
// from bindings.go itself, so the page is grouped as the code is.
func defaultBindingGroups(t *testing.T) []bindingGroup {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bindings.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var lit *ast.CompositeLit
	ast.Inspect(f, func(n ast.Node) bool {
		if vs, ok := n.(*ast.ValueSpec); ok && len(vs.Names) == 1 && vs.Names[0].Name == "defaultBindings" {
			lit, _ = vs.Values[0].(*ast.CompositeLit)
			return false
		}
		return true
	})
	if lit == nil || len(lit.Elts) != len(defaultBindings) {
		t.Fatal("cannot find defaultBindings' entries in bindings.go")
	}
	var groups []bindingGroup
	next := 0 // the next comment group to consider
	for i, elt := range lit.Elts {
		for next < len(f.Comments) && f.Comments[next].End() < elt.Pos() {
			if c := f.Comments[next]; c.Pos() > lit.Lbrace {
				groups = append(groups, bindingGroup{heading: sectionHeading(c.Text())})
			}
			next++
		}
		if len(groups) == 0 {
			t.Fatalf("defaultBindings' entry %d, %v, comes before any section heading", i, defaultBindings[i])
		}
		g := &groups[len(groups)-1]
		g.bindings = append(g.bindings, defaultBindings[i])
	}
	return groups
}

// sectionHeading is a section's name from the comment that opens it: its
// first sentence, and of that what comes before a colon. "Projects:
// project.el's keys, with projectile's commands. See project.go." is
// Projects.
func sectionHeading(comment string) string {
	s := strings.TrimSpace(strings.SplitN(comment, "\n", 2)[0])
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

// firstSentence is a command's doc, cut to its first sentence.
func firstSentence(doc string) string {
	doc = strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(doc, ". "); i >= 0 {
		return doc[:i+1]
	}
	return doc
}

// keyNames are the names a keymap reports keys by, and the ones people
// write them with, which the pages use.
var keyNames = strings.NewReplacer("<return>", "RET", "<tab>", "TAB", "<escape>", "ESC")

// sortedKeys orders keys as a reader looks for them: typed keys before named
// ones, and short before long - RET f before M-<down>.
func sortedKeys(ks []string) []string {
	ks = slices.Clone(ks)
	for i, k := range ks {
		k = keyNames.Replace(k)
		if strings.HasSuffix(k, "S-TAB") {
			k = strings.TrimSuffix(k, "S-TAB") + "<backtab>"
		}
		ks[i] = k
	}
	named := func(k string) bool { return strings.Contains(k, "<") }
	sort.Slice(ks, func(i, j int) bool {
		if named(ks[i]) != named(ks[j]) {
			return !named(ks[i])
		}
		if len(ks[i]) != len(ks[j]) {
			return len(ks[i]) < len(ks[j])
		}
		return ks[i] < ks[j]
	})
	return ks
}

// keyCode writes a key as inline code, safe in a table: a backtick in the
// key takes a longer fence, and a bar is escaped from the table.
func keyCode(spec string) string {
	s := "`" + spec + "`"
	if strings.Contains(spec, "`") {
		s = "`` " + spec + " ``"
	}
	return strings.ReplaceAll(s, "|", `\|`)
}

// bindingTable is a table of bindings, one row a command, its keys together,
// in the order the command first appears.
func bindingTable(e *Editor, bindings []struct{ Spec, Command string }) string {
	var order []string
	keys := map[string][]string{}
	for _, b := range bindings {
		if _, seen := keys[b.Command]; !seen {
			order = append(order, b.Command)
		}
		if !slices.Contains(keys[b.Command], b.Spec) {
			keys[b.Command] = append(keys[b.Command], b.Spec)
		}
	}
	var sb strings.Builder
	sb.WriteString("| Key | Command | Does |\n|---|---|---|\n")
	for _, cmd := range order {
		var ks []string
		for _, k := range keys[cmd] {
			ks = append(ks, keyCode(k))
		}
		doc := ""
		if c, ok := e.reg.Lookup(cmd); ok {
			doc = firstSentence(c.Doc)
		}
		fmt.Fprintf(&sb, "| %s | `%s` | %s |\n", strings.Join(ks, " "), cmd, doc)
	}
	return sb.String()
}

// promptControls says what each key of a prompt's own keymap does. They are
// not commands, so they have no docs of their own; a control added without a
// line here fails the test.
var promptControls = map[string]string{
	miniAccept:     "Take the highlighted candidate, or what was typed where there are none.",
	miniAbort:      "Cancel the prompt.",
	miniComplete:   "Complete what was typed to the highlighted candidate.",
	miniSearchFwd:  "In a search, go to the next match.",
	miniSearchBack: "In a search, go to the previous match.",
	miniNext:       "Highlight the next candidate; with none listed, the next entry of the history.",
	miniPrev:       "Highlight the previous candidate; with none listed, the previous entry of the history.",
	miniAcceptText: "Take exactly what was typed, not the highlighted candidate: a new file whose name matches another.",
	miniHistPrev:   "Bring back the previous thing entered at this kind of prompt.",
	miniHistNext:   "Go forward again through that history.",
}

// promptTable is the table of a prompt's own keys.
func promptTable(t *testing.T) string {
	t.Helper()
	m := miniKeymap()
	byControl := map[string][]string{}
	for spec, name := range m.Bindings() {
		byControl[name] = append(byControl[name], spec)
	}
	controls := make([]string, 0, len(byControl))
	for name := range byControl {
		if _, ok := promptControls[name]; !ok {
			t.Errorf("the prompt keymap binds %s, which promptControls does not describe", name)
		}
		controls = append(controls, name)
	}
	// In the order a reader meets them: taking, cancelling, moving.
	rank := []string{miniAccept, miniAcceptText, miniAbort, miniComplete, miniNext, miniPrev, miniHistPrev, miniHistNext, miniSearchFwd, miniSearchBack}
	sort.Slice(controls, func(i, j int) bool { return slices.Index(rank, controls[i]) < slices.Index(rank, controls[j]) })
	var sb strings.Builder
	sb.WriteString("| Key | Does |\n|---|---|\n")
	for _, name := range controls {
		ks := sortedKeys(byControl[name])
		for i, k := range ks {
			ks[i] = keyCode(k)
		}
		fmt.Fprintf(&sb, "| %s | %s |\n", strings.Join(ks, " "), promptControls[name])
	}
	return sb.String()
}

// globalKeys lists the keys bound globally to cmd, as code.
func globalKeys(e *Editor, cmd string) string {
	ks := sortedKeys(e.keys.Where(cmd))
	for i, k := range ks {
		ks[i] = keyCode(k)
	}
	return strings.Join(ks, " ")
}

// keybindingsPage is the Keybindings page.
func keybindingsPage(t *testing.T, e *Editor) string {
	var sb strings.Builder
	sb.WriteString(wikiNotice)
	sb.WriteString(`# Keybindings

Every key nem binds out of the box. Keys are written in emacs notation: ` + "`C-x`" + ` is Control+x, ` + "`M-x`" + ` is Meta+x (Alt, or Escape then x), and ` + "`C-x C-f`" + ` is two keys in turn. [Configuration](Configuration#key-notation) has the whole notation.

Any key can be bound to another command, and any command to a key of your own, with [` + "`nem.bind`" + `](Lua-API#nembind). [Commands](Commands) lists every command, including those bound to no key, which ` + "`M-x`" + ` runs by name. Inside nem, ` + "`<f1> b`" + ` lists what is bound and ` + "`<f1> k`" + ` says what a key does.

**On this page:** [Global keys](#global-keys) · [Dired](#dired) · [Editing file names](#editing-file-names) · [Search results](#search-results) · [Compilation output](#compilation-output) · [In a prompt](#in-a-prompt) · [Answering a question](#answering-a-question) · [Arguments and repeats](#arguments-and-repeats)

## Global keys

These work everywhere, unless a mode or a prompt below gives a key a meaning of its own there.

`)
	for _, g := range defaultBindingGroups(t) {
		fmt.Fprintf(&sb, "### %s\n\n%s\n", g.heading, bindingTable(e, g.bindings))
	}

	modes := []struct {
		heading, mode, intro string
		bindings             []struct{ Spec, Command string }
	}{
		{"Dired", "dired", "In a directory listing: `C-x d`, `C-x C-j`, or any directory opened as a file. The listing is read-only, so plain letters are commands here.", diredBindings},
		{"Editing file names", "wdired", "After `C-x C-q` in a listing, the file names are text to edit; these keys finish or abandon the edit.", wdiredBindings},
		{"Search results", "grep", "In the results of a project search (`C-x p g`) or of `M-s o` (occur).", grepBindings},
		{"Compilation output", "compilation", "In the output of `M-x compile`, `M-x recompile`, `C-x p c` and `M-&`.", compileBindings},
	}
	for _, m := range modes {
		fmt.Fprintf(&sb, "## %s\n\n%s Bind a key here with `nem.bind(key, command, %q)`.\n\n%s\n", m.heading, m.intro, m.mode, bindingTable(e, m.bindings))
	}

	sb.WriteString(`## In a prompt

A prompt - ` + "`M-x`" + `, ` + "`C-x C-f`" + `, ` + "`C-x b`" + `, a search - edits its one line as a buffer is edited: ` + "`C-a`" + `, ` + "`C-k`" + `, ` + "`M-b`" + `, ` + "`C-y`" + ` and the rest all work in it. On top of those it has keys of its own:

`)
	sb.WriteString(promptTable(t))
	fmt.Fprintf(&sb, `
While candidates are listed, the keys that page and jump through a buffer move through the list instead:

| Key | Does |
|---|---|
| %s | Next page of candidates. |
| %s | Previous page of candidates. |
| %s | First candidate. |
| %s | Last candidate. |

`, globalKeys(e, "scroll-up-command"), globalKeys(e, "scroll-down-command"), globalKeys(e, "beginning-of-buffer"), globalKeys(e, "end-of-buffer"))
	sb.WriteString(`An incremental search (` + "`C-s`" + `, ` + "`C-r`" + `) moves to the first match as you type. ` + "`C-s`" + ` and ` + "`C-r`" + ` go on to the next and previous match, wrapping round the buffer after saying they have reached its end; ` + "`RET`" + ` stops at the match, and ` + "`C-g`" + ` goes back to where the search began.

## Answering a question

Some commands stop to ask. A question takes one key, with no ` + "`RET`" + `; ` + "`C-g`" + ` cancels it.

| Asked by | Keys |
|---|---|
| ` + "`M-%`" + ` and ` + "`C-M-%`" + `, at each match | ` + "`y`" + ` replace it · ` + "`n`" + ` skip it · ` + "`!`" + ` replace it and all the rest · ` + "`q`" + ` stop |
| ` + "`C-x p r`" + `, at each match | as above, and ` + "`N`" + ` skip the rest of this file · ` + "`Y`" + ` replace all the rest, in every file; ` + "`!`" + ` is the rest of this file |
| ` + "`C-x s`" + `, for each unsaved file | ` + "`y`" + ` save it · ` + "`n`" + ` don't · ` + "`!`" + ` save it and all the rest · ` + "`q`" + ` stop |
| ` + "`C-x C-c`" + `, for each unsaved file | ` + "`y`" + ` save it · ` + "`n`" + ` don't · ` + "`q`" + ` stay in nem |
| Opening a file that isn't text | ` + "`s`" + ` open it with the system's app · ` + "`t`" + ` as text · ` + "`S`" + ` and ` + "`T`" + ` the same for every such file, for the rest of the session |
| Anything else asked yes or no | ` + "`y`" + ` · ` + "`n`" + ` |

## Arguments and repeats

| Key | Does |
|---|---|
| ` + "`C-u`" + ` | Give the next command an argument: 4, and 16 with ` + "`C-u C-u`" + `. Digits after it give a number: ` + "`C-u 12 C-f`" + ` moves 12 characters. |
| ` + "`M-0`" + ` … ` + "`M-9`" + `, ` + "`M--`" + ` | Start an argument without ` + "`C-u`" + `: ` + "`M-1 M-2`" + ` is 12, ` + "`M--`" + ` is -1. |
| ` + "`C-g`" + ` | Cancel a half-typed prefix or argument, a prompt, or a question. |
| ` + "`e`" + ` | Straight after ` + "`C-x e`" + `, play the keyboard macro again. |

Pausing after a prefix key such as ` + "`C-x`" + ` lists the keys that can follow it, after a second by default: the ` + "`which-key-delay`" + ` [setting](Settings#which-key-delay).
`)
	return sb.String()
}

// commandsPage is the Commands page: every command, its keys and its doc.
func commandsPage(e *Editor) string {
	cmds := e.reg.All()
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })

	// Where each command is bound: globally, and in each mode.
	modes := []struct {
		name string
		m    *keymap.Map
	}{{"", e.keys}, {"dired", e.diredKeys}, {"wdired", e.wdiredKeys}, {"grep", e.grepKeys}, {"compilation", e.compileKeys}}
	keysOf := func(cmd string) string {
		var parts []string
		for _, md := range modes {
			ks := sortedKeys(md.m.Where(cmd))
			if len(ks) == 0 {
				continue
			}
			for i, k := range ks {
				ks[i] = keyCode(k)
			}
			s := strings.Join(ks, " ")
			if md.name != "" {
				s = md.name + ": " + s
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "; ")
	}

	var sb strings.Builder
	sb.WriteString(wikiNotice)
	sb.WriteString(`# Commands

Every command nem has, by name. ` + "`M-x`" + ` runs any of them by name, with fuzzy completion: ` + "`M-x fwc`" + ` finds ` + "`forward-char`" + `. [` + "`nem.bind`" + `](Lua-API#nembind) binds a key to any of them, and [` + "`nem.run`" + `](Lua-API#nemrun) runs one from a command of your own. Commands you define with [` + "`nem.command`" + `](Lua-API#nemcommand) join this list, as equals.

A key in a mode - ` + "`dired: f`" + ` - works only in that mode's buffers; see [Keybindings](Keybindings).

| Command | Keys | Does |
|---|---|---|
`)
	for _, c := range cmds {
		doc := strings.Join(strings.Fields(c.Doc), " ")
		if !c.Interactive {
			doc += " *(Not offered by `M-x`.)*"
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s |\n", c.Name, keysOf(c.Name), strings.ReplaceAll(doc, "|", `\|`))
	}
	fmt.Fprintf(&sb, "\n%s commands in all.\n", strconv.Itoa(len(cmds)))
	return sb.String()
}

// wikiPage reads one of the wiki's pages.
func wikiPage(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(wikiDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// luaExamples are the Lua examples published: every ```lua block of the
// wiki's pages, and the example config, by where each is.
func luaExamples(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	pages, err := filepath.Glob(filepath.Join(wikiDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		src := wikiPage(t, filepath.Base(p))
		for i, part := range strings.Split(src, "```lua\n")[1:] {
			block, _, ok := strings.Cut(part, "```")
			if !ok {
				t.Fatalf("%s: an unclosed lua block", filepath.Base(p))
			}
			out[fmt.Sprintf("%s, block %d", filepath.Base(p), i+1)] = block
		}
	}
	example, err := os.ReadFile(filepath.Join("..", "examples", "init.lua"))
	if err != nil {
		t.Fatal(err)
	}
	out["examples/init.lua"] = string(example)
	return out
}

// Every Lua example published loads without an error, binds keys only to
// commands nem has, and every command it defines runs - on a Go file, which
// is then saved, so its hooks run too. An example that has gone stale fails
// here, before anyone copies it.
func TestWikiLuaExamplesWork(t *testing.T) {
	defines := regexp.MustCompile(`nem\.command\(\s*"([^"]+)"`)
	for where, src := range luaExamples(t) {
		e, _ := newTestEditor(t)
		dir := t.TempDir()
		cfg := filepath.Join(dir, "init.lua")
		if err := os.WriteFile(cfg, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := e.LoadConfig(cfg); err != nil {
			t.Errorf("%s does not load: %v", where, err)
			continue
		}
		if dangling := e.host.UnresolvedBindings(); len(dangling) > 0 {
			t.Errorf("%s binds keys to commands nem does not have: %v", where, dangling)
		}
		file := filepath.Join(dir, "sample.go")
		if err := os.WriteFile(file, []byte("package main  \n\nfunc main() {\n\tprintln(\"hi\")\t\n}\n\n\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		visit(t, e, file)
		for _, m := range defines.FindAllStringSubmatch(src, -1) {
			if err := e.dispatch(m[1]); err != nil {
				t.Errorf("%s: %s fails: %v", where, m[1], err)
			}
		}
		if err := e.dispatch("save-buffer"); err != nil {
			t.Errorf("%s: saving fails: %v", where, err)
		}
		if msg := e.Message(); strings.HasPrefix(msg, "before-save") || strings.HasPrefix(msg, "after-save") {
			t.Errorf("%s: a hook fails: %s", where, msg)
		}
	}
}

// The Lua API page names every hook nem fires and every mode nem.bind binds
// in. Adding either without saying so there fails here.
func TestWikiNamesEveryHookAndMode(t *testing.T) {
	page := wikiPage(t, "Lua-API.md")
	for hook := range hookCommands {
		if !strings.Contains(page, "`"+hook+"`") {
			t.Errorf("Lua-API.md does not describe the %s hook", hook)
		}
	}
	e, _ := newTestEditor(t)
	for mode := range e.modeKeymaps() {
		if !strings.Contains(page, "`\""+mode+"\"`") {
			t.Errorf("Lua-API.md does not list the %q mode", mode)
		}
	}
}
