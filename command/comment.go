package command

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Borderliner/nem/text"
)

// Commenting: M-; turns the lines of the region, or the current line, into
// comments, or back again when they all are already.
//
// That is not quite emacs's comment-dwim, which without a region adds a
// comment at the end of the line. Toggling whole lines is what nearly every
// other editor does with its comment key, and it is the thing people reach
// for: silencing a block of code. An empty line still gets a comment started
// on it, which is emacs's behaviour there too.

// commentSyntax is how a kind of file writes a comment: a start marker, and an
// end marker for languages that have only block comments.
type commentSyntax struct{ start, end string }

var (
	slashes   = commentSyntax{start: "//"}
	hash      = commentSyntax{start: "#"}
	dashes    = commentSyntax{start: "--"}
	semicolon = commentSyntax{start: ";"}
	percent   = commentSyntax{start: "%"}
	markup    = commentSyntax{start: "<!--", end: "-->"}
	slashStar = commentSyntax{start: "/*", end: "*/"}
)

// commentByExt maps a lower-cased extension to its comment syntax.
var commentByExt = map[string]commentSyntax{
	"go": slashes, "c": slashes, "h": slashes, "cc": slashes, "cpp": slashes,
	"cxx": slashes, "hpp": slashes, "java": slashes, "js": slashes, "mjs": slashes,
	"cjs": slashes, "ts": slashes, "mts": slashes, "jsx": slashes, "tsx": slashes,
	"rs": slashes, "swift": slashes, "kt": slashes, "scala": slashes, "dart": slashes,
	"zig": slashes, "cs": slashes, "php": slashes, "scss": slashes, "jsonc": slashes,
	"proto": slashes, "groovy": slashes, "gradle": slashes, "v": slashes,

	"py": hash, "sh": hash, "bash": hash, "zsh": hash, "fish": hash, "rb": hash,
	"pl": hash, "r": hash, "yaml": hash, "yml": hash, "toml": hash, "conf": hash,
	"cfg": hash, "nix": hash, "tf": hash, "ex": hash, "exs": hash, "jl": hash,
	"cmake": hash, "mk": hash, "env": hash, "ps1": hash, "nim": hash, "cr": hash,

	"lua": dashes, "sql": dashes, "hs": dashes, "elm": dashes, "ada": dashes,

	"ini": semicolon, "el": {start: ";;"}, "lisp": {start: ";;"}, "scm": {start: ";;"},
	"clj": {start: ";;"}, "asm": semicolon, "s": semicolon,

	"tex": percent, "erl": percent, "m": percent,

	"html": markup, "htm": markup, "xml": markup, "svg": markup, "md": markup,
	"markdown": markup, "vue": markup,

	"css": slashStar, "ml": {start: "(*", end: "*)"},
	"vim": {start: `"`},
}

// commentByName maps whole lower-cased file names, for files known by name.
var commentByName = map[string]commentSyntax{
	"makefile": hash, "gnumakefile": hash, "dockerfile": hash, "containerfile": hash,
	"cmakelists.txt": hash, ".gitignore": hash, ".bashrc": hash, ".zshrc": hash,
	".profile": hash, ".gitconfig": hash, ".editorconfig": hash, "go.mod": slashes,
}

// commentFor is the comment syntax for a file at path. A buffer with no file,
// or a kind nem does not know, gets # - the most widely understood marker, and
// the one scripts and configuration use.
func commentFor(path string) commentSyntax {
	if cs, ok := knownComment(path); ok {
		return cs
	}
	return hash
}

// knownComment is the comment syntax for path when nem knows the kind of
// file, and false when it would only be guessing.
func knownComment(path string) (commentSyntax, bool) {
	base := strings.ToLower(filepath.Base(path))
	if cs, ok := commentByName[base]; ok {
		return cs, true
	}
	if strings.HasPrefix(base, "dockerfile") {
		return hash, true
	}
	cs, ok := commentByExt[strings.TrimPrefix(filepath.Ext(base), ".")]
	return cs, ok
}

// RegisterComment adds the comment commands to r.
func RegisterComment(r *Registry) error {
	return r.Register(Command{
		Name:        "comment-dwim",
		Doc:         "Comment or uncomment the lines of the region, or the current line.",
		Fn:          commentDwim,
		Interactive: true,
	})
}

func commentDwim(e Env) error {
	b, w := e.Buf(), e.Win()
	cs := commentFor(b.Path())

	first, last := w.Pt.Line, w.Pt.Line
	if b.MarkActive() {
		lo, hi := text.OrderPos(w.Pt, b.Mark())
		first, last = lo.Line, hi.Line
		// A region ending at the start of a line does not reach into it:
		// selecting whole lines by dragging to the next line's start is the
		// usual way to select them.
		if hi.Col == 0 && hi.Line > lo.Line {
			last--
		}
	}

	if !b.MarkActive() && strings.TrimSpace(b.Line(first).String()) == "" {
		return commentStart(e, cs)
	}

	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if linesCommented(b, first, last, cs) {
		return uncommentLines(e, first, last, cs)
	}
	return commentLines(e, first, last, cs)
}

// commentStart begins a comment on an empty line, at its indentation, with
// point where the comment's text goes.
func commentStart(e Env, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	at := text.Pos{Line: w.Pt.Line, Col: b.Line(w.Pt.Line).Len()}
	ins := cs.start + " "
	if cs.end != "" {
		ins += " " + cs.end
	}
	if err := b.Insert(at, []rune(ins)); err != nil {
		return err
	}
	edSetPoint(e, text.Pos{Line: at.Line, Col: at.Col + text.RuneIdx(utf8.RuneCountInString(cs.start)+1)})
	return nil
}

// indentOf is how many runes of leading space and tab a line has.
func indentOf(rs []rune) int {
	n := 0
	for n < len(rs) && (rs[n] == ' ' || rs[n] == '\t') {
		n++
	}
	return n
}

// linesCommented reports whether every non-blank line in [first, last] starts,
// after its indentation, with the comment marker. Blank lines do not count
// either way, so a commented block with a gap in it still uncomments.
func linesCommented(b *text.Buffer, first, last int, cs commentSyntax) bool {
	any := false
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		ind := indentOf(rs)
		if ind == len(rs) {
			continue
		}
		if !strings.HasPrefix(string(rs[ind:]), cs.start) {
			return false
		}
		any = true
	}
	return any
}

// commentLines comments every non-blank line in [first, last], with all the
// markers in one column - the least indentation among them - so the block
// still reads as one.
func commentLines(e Env, first, last int, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	col := -1
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		if ind := indentOf(rs); ind < len(rs) && (col < 0 || ind < col) {
			col = ind
		}
	}
	if col < 0 {
		return nil
	}
	open := []rune(cs.start + " ")
	for i := first; i <= last; i++ {
		l := b.Line(i)
		if indentOf(l.View()) == int(l.Len()) {
			continue // blank lines stay blank
		}
		if cs.end != "" {
			if err := b.Insert(text.Pos{Line: i, Col: l.Len()}, []rune(" "+cs.end)); err != nil {
				return err
			}
		}
		if err := b.Insert(text.Pos{Line: i, Col: text.RuneIdx(col)}, open); err != nil {
			return err
		}
		if i == w.Pt.Line && int(w.Pt.Col) >= col {
			w.Pt.Col += text.RuneIdx(len(open))
		}
	}
	return nil
}

// uncommentLines removes the marker, and one space after it, from every
// commented line in [first, last], and the end marker where there is one.
func uncommentLines(e Env, first, last int, cs commentSyntax) error {
	b, w := e.Buf(), e.Win()
	for i := first; i <= last; i++ {
		rs := b.Line(i).View()
		ind := indentOf(rs)
		body := string(rs[ind:])
		if !strings.HasPrefix(body, cs.start) {
			continue
		}
		if cs.end != "" {
			trimmed := strings.TrimRight(body, " \t")
			if strings.HasSuffix(trimmed, cs.end) {
				cut := strings.TrimSuffix(trimmed, cs.end)
				cut = strings.TrimSuffix(cut, " ")
				from := text.Pos{Line: i, Col: text.RuneIdx(ind + utf8.RuneCountInString(cut))}
				if err := b.Delete(from, text.Pos{Line: i, Col: b.Line(i).Len()}); err != nil {
					return err
				}
			}
		}
		n := utf8.RuneCountInString(cs.start)
		if rest := b.Line(i).View()[ind+n:]; len(rest) > 0 && rest[0] == ' ' {
			n++
		}
		if err := b.Delete(text.Pos{Line: i, Col: text.RuneIdx(ind)}, text.Pos{Line: i, Col: text.RuneIdx(ind + n)}); err != nil {
			return err
		}
		if i == w.Pt.Line && int(w.Pt.Col) > ind {
			w.Pt.Col = text.RuneIdx(max(ind, int(w.Pt.Col)-n))
		}
	}
	w.Pt = b.ClampPos(w.Pt)
	return nil
}
