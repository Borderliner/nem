package command

import (
	"path/filepath"
	"strings"

	"github.com/Borderliner/nem/editorconfig"
	"github.com/Borderliner/nem/text"
)

// Indentation: TAB indents the way the file already does, with tabs or with
// spaces.
//
// This is not language-aware indentation, which would need to know what a
// block is in each language. It is the half that needs no parser and that a
// wrong answer to hurts most: a tab typed into a Python or YAML file is an
// error, and spaces in a Makefile's recipe are another.

// Indent is how a buffer indents: with tabs, or with spaces, and how many
// columns one level takes. For tabs that is how wide a tab is taken to be.
type Indent struct {
	Tabs  bool
	Width int
}

// IndentFor is how b indents. The default asks, in order, the .editorconfig
// files that apply to its file, the indentation its text already has, and
// the convention of its language, taking each thing - tabs or spaces, and
// the width - from the first that says. It is a variable, as FillColumn is a
// setting, so the editor or a test can decide differently.
var IndentFor = defaultIndentFor

func defaultIndentFor(b *text.Buffer) Indent {
	var ec editorconfig.Props
	if p := b.Path(); p != "" {
		ec = editorconfig.Lookup(p)
	}
	guess, guessed := guessIndent(b)
	lang := languageIndent(b.Path())

	var tabs bool
	switch {
	case ec.IndentStyle != "":
		tabs = ec.IndentStyle == "tab"
	case guessed:
		tabs = guess.Tabs
	default:
		tabs = lang.Tabs
	}

	// The width comes from the first source that speaks of the same style:
	// a Python file's four is no guide to a tab's width.
	width := ec.IndentSize
	if tabs {
		width = ec.TabWidth
	}
	if width == 0 && guessed && guess.Tabs == tabs {
		width = guess.Width
	}
	if width == 0 && lang.Tabs == tabs {
		width = lang.Width
	}
	if width == 0 {
		width = 4
		if tabs {
			width = tabWidth()
		}
	}
	return Indent{Tabs: tabs, Width: width}
}

// tabWidth is the display's tab width, as a number of columns.
func tabWidth() int { return max(1, int(text.TabWidth)) }

var (
	byTabs     = Indent{Tabs: true}
	twoSpaces  = Indent{Width: 2}
	fourSpaces = Indent{Width: 4}
)

// indentByExt is how each language is conventionally indented, by
// lower-cased extension, for a file with nothing better to go on: no
// .editorconfig says, and it has no indented lines yet - a new file, most
// often. It sits with commentByExt in spirit: a table, not a guess from the
// highlighter's rules, because a language's indentation is its community's
// convention rather than anything its syntax says. A tab's width is left to
// the tab-width setting.
var indentByExt = map[string]Indent{
	// gofmt insists on tabs, and make needs one to start each line of a
	// recipe.
	"go": byTabs, "mk": byTabs, "mak": byTabs,

	"js": twoSpaces, "mjs": twoSpaces, "cjs": twoSpaces, "jsx": twoSpaces, "ts": twoSpaces,
	"mts": twoSpaces, "cts": twoSpaces, "tsx": twoSpaces, "json": twoSpaces, "jsonc": twoSpaces,
	"json5": twoSpaces, "yaml": twoSpaces, "yml": twoSpaces, "lua": twoSpaces, "rb": twoSpaces,
	"html": twoSpaces, "htm": twoSpaces, "css": twoSpaces, "scss": twoSpaces, "sass": twoSpaces,
	"less": twoSpaces, "nix": twoSpaces, "dart": twoSpaces, "xml": twoSpaces, "svg": twoSpaces,
	"md": twoSpaces, "markdown": twoSpaces, "vue": twoSpaces, "svelte": twoSpaces,
	"graphql": twoSpaces, "tf": twoSpaces, "hcl": twoSpaces, "proto": twoSpaces,
	"ex": twoSpaces, "exs": twoSpaces, "el": twoSpaces, "lisp": twoSpaces, "scm": twoSpaces,
	"clj": twoSpaces, "ml": twoSpaces, "nim": twoSpaces, "cr": twoSpaces, "scala": twoSpaces,
	"r": twoSpaces,

	"py": fourSpaces, "pyi": fourSpaces, "rs": fourSpaces, "java": fourSpaces, "kt": fourSpaces,
	"kts": fourSpaces, "c": fourSpaces, "h": fourSpaces, "cc": fourSpaces, "cpp": fourSpaces,
	"cxx": fourSpaces, "hpp": fourSpaces, "hh": fourSpaces, "cs": fourSpaces, "swift": fourSpaces,
	"php": fourSpaces, "sh": fourSpaces, "bash": fourSpaces, "zsh": fourSpaces, "fish": fourSpaces,
	"sql": fourSpaces, "pl": fourSpaces, "groovy": fourSpaces, "gradle": fourSpaces,
	"ps1": fourSpaces, "jl": fourSpaces, "zig": fourSpaces, "erl": fourSpaces, "hs": fourSpaces,
	"m": fourSpaces, "v": fourSpaces,
}

// indentByName is indentByExt for files known by their whole lower-cased
// name.
var indentByName = map[string]Indent{
	"makefile": byTabs, "gnumakefile": byTabs, "go.mod": byTabs, "go.work": byTabs,
	"gemfile": twoSpaces, "rakefile": twoSpaces,
	".bashrc": fourSpaces, ".zshrc": fourSpaces, ".profile": fourSpaces,
}

// languageIndent is the convention for the file at path. A file nem does not
// recognise, or a buffer with no file, is indented with tabs: that is what
// TAB always inserted, and a guess at spaces would be a guess at how many.
func languageIndent(path string) Indent {
	base := strings.ToLower(filepath.Base(path))
	if ind, ok := indentByName[base]; ok {
		return ind
	}
	if ind, ok := indentByExt[strings.TrimPrefix(filepath.Ext(base), ".")]; ok {
		return ind
	}
	return byTabs
}

// guessLines is how many lines guessIndent reads. The top of a file tells
// its style as well as the whole of it does, and stopping there keeps TAB
// instant in a file of a million lines.
const guessLines = 1000

// guessIndent is the indentation b's text already uses, if it has any to
// judge by: tabs when more of its indented lines start with a tab than with
// spaces, and spaces otherwise, as many as the step it most often indents
// by from one line to the next.
//
// A single leading space does not count: it is the " * " of a block comment,
// or alignment, never a level of indentation.
func guessIndent(b *text.Buffer) (Indent, bool) {
	tabbed, spaced, least := 0, 0, 0
	var steps [9]int // how often each step, 2 to 8, is taken
	prev := 0        // the last indented line's spaces; -1 after a tab
	for i := range min(b.NumLines(), guessLines) {
		rs := b.Line(i).View()
		n := indentOf(rs)
		if n == len(rs) {
			continue // a blank line says nothing, and does not break a step
		}
		if rs[0] == '\t' {
			tabbed++
			prev = -1
			continue
		}
		sp := 0
		for sp < n && rs[sp] == ' ' {
			sp++
		}
		if sp >= 2 {
			spaced++
			if least == 0 || sp < least {
				least = sp
			}
		}
		if d := sp - prev; prev >= 0 && d >= 2 && d <= 8 {
			steps[d]++
		}
		prev = sp
	}
	switch {
	case tabbed > spaced:
		return byTabs, true
	case spaced > tabbed:
		// The most common step, the smaller on a tie: a file indented by
		// two has its continuation lines four in, but one indented by four
		// has little that is two in.
		step := 0
		for d := 2; d <= 8; d++ {
			if steps[d] > steps[step] {
				step = d
			}
		}
		if step == 0 {
			// Every indented line at the same depth: its depth is the step.
			step = min(least, 8)
		}
		return Indent{Width: step}, true
	}
	return Indent{}, false
}

// indentForTab is TAB: a level inserted at point, as a tab, or as spaces up
// to the next multiple of the width, as a tab stop would take it. C-u 3 TAB
// is three levels.
func indentForTab(e Env) error {
	n, _ := e.Arg()
	if n < 1 {
		n = 1
	}
	b, p := e.Buf(), e.Win().Pt
	ind := IndentFor(b)

	var ins []rune
	if ind.Tabs {
		ins = []rune(strings.Repeat("\t", n))
	} else {
		col := int(b.Line(p.Line).DisplayCol(p.Col))
		ins = []rune(strings.Repeat(" ", ind.Width-col%ind.Width+(n-1)*ind.Width))
	}
	if err := b.Insert(p, ins); err != nil {
		return err
	}
	edSetPoint(e, edAdvance(p, ins))
	return nil
}
