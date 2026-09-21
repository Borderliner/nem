package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// NameFunc reports what a buffer is called.
//
// Buffer names live in the editor, which owns the name map; text.Buffer carries
// only a path, by design. Without this the renderer can only guess a name from
// the path, which labels every path-less buffer *scratch* - so a listing buffer
// renders its contents correctly while the modeline insists you are still in
// *scratch*. Each half looks right alone, which is why no unit test caught it.
type NameFunc func(*text.Buffer) string

// TypeFunc reports a buffer's file type for the modeline: "go", "lua", "json",
// "md", or empty for text nem has no grammar for.
//
// It comes from the editor rather than being derived here so that it cannot
// disagree with the colouring: both answer from the lexer the editor already
// chose. A modeline claiming "go" over uncoloured text would be worse than no
// segment at all, because it would look like the highlighter had failed.
type TypeFunc func(*text.Buffer) string

// BranchFunc reports the git branch a buffer's file sits on, or empty when it
// is not in a repository.
//
// Reading .git is I/O, and this is consulted for every window on every frame,
// so the editor caches the answer. See editor/vcs.go.
type BranchFunc func(*text.Buffer) string

// BranchMark precedes the branch name so it cannot be mistaken for part of the
// file name.
//
// U+2387 is a standard codepoint rather than a Private Use Area glyph, so it
// needs no patched font - but coverage in terminal fonts is thinner than for the
// box-drawing runes used elsewhere, and a font without it shows a replacement
// box. It is a constant so that swapping it costs one line.
const BranchMark = '⎇'

// modelineInfo is what the editor knows about a buffer and the renderer cannot
// work out for itself.
//
// Three optional functions rather than three values: the modeline is built per
// window per frame, and only the windows actually drawn should pay for a name
// lookup or a cache probe.
type modelineInfo struct {
	Name   NameFunc
	Type   TypeFunc
	Branch BranchFunc
}

// segments asks for b's file type and branch, tolerating nil sources.
func (i modelineInfo) segments(b *text.Buffer) (fileType, branch string) {
	if i.Type != nil {
		fileType = i.Type(b)
	}
	if i.Branch != nil {
		branch = i.Branch(b)
	}
	return fileType, branch
}

// segmentText renders the segments that follow a buffer's name, each preceded by
// two spaces so they read as separate fields rather than as one run of words.
// An absent segment contributes nothing, not an empty gap.
func segmentText(fileType, branch string) string {
	var b strings.Builder
	if fileType != "" {
		b.WriteString("  ")
		b.WriteString(fileType)
	}
	if branch != "" {
		b.WriteString("  ")
		b.WriteRune(BranchMark)
		b.WriteByte(' ')
		b.WriteString(branch)
	}
	return b.String()
}

// bufferName asks nameOf what b is called, falling back to b's file name and
// then to *scratch*.
//
// The fallback keeps ui usable as a library: a caller that does not track buffer
// names gets the old path-derived behaviour rather than a blank modeline.
func bufferName(b *text.Buffer, nameOf NameFunc) string {
	if nameOf != nil {
		if n := nameOf(b); n != "" {
			return n
		}
	}
	if p := b.Path(); p != "" {
		return filepath.Base(p)
	}
	return ScratchName
}

// modelineString builds one window's status line, exactly width cells wide.
//
// The line is a hairline rule with the buffer name set into it: a modified mark
// and the name on the left, the position flush right, and the rule filling
// everything between. That rule is doing real work, not decoration - a
// horizontal split draws no divider row, so this line is the only boundary
// between stacked panes.
//
// Focus reads as weight: the active window's name uses the terminal's own
// foreground, an inactive one is quieted. Nothing is filled with a background
// colour, which is what keeps the editor sitting inside your terminal's palette
// rather than painting over it.
//
// Padding is computed in display cells rather than runes, so a buffer named in
// CJK still lines up.
func modelineString(th Theme, w *view.Window, width int, active bool, info modelineInfo) string {
	if width <= 0 {
		return ""
	}

	// The mark occupies a cell whether or not the buffer is modified, so the
	// name never shifts sideways the moment you start typing.
	mark := " "
	if w.Buf.Modified() {
		mark = th.ModelineMark.Render(string(ModifiedMark))
	}

	name := bufferName(w.Buf, info.Name)

	// Line numbers are 1-based and columns 0-based, which is what emacs
	// reports. The column is a display column, not a rune index, so a cursor
	// past a CJK glyph reads 2 rather than 1.
	pt := w.Buf.ClampPos(w.Pt)
	pos := fmt.Sprintf("%d:%d", pt.Line+1, w.Buf.Line(pt.Line).DisplayCol(pt.Col))

	nameStyle := th.ModelineNameOff
	if active {
		nameStyle = th.ModelineName
	}

	fileType, branch := info.segments(w.Buf)
	nameW, posW := lipgloss.Width(name), lipgloss.Width(pos)

	// Preferred shape: mark, name, segments, space, rule, space, position,
	// trailing space. Four cells of fixed spacing plus at least one cell of
	// rule.
	//
	// Segments are given up from the least essential end as the pane narrows:
	// the branch first, then the file type. Where you are in the file and what
	// the file is called are worth more than either, and the rule after them is
	// what separates stacked panes, so it is given up only when both segments
	// already have been.
	//
	// Segments borrow ModelinePos rather than introducing a style of their own:
	// it is the palette's quiet secondary-information style, which is exactly
	// what a file type and a branch are.
	for _, segs := range []string{
		segmentText(fileType, branch),
		segmentText(fileType, ""),
		"",
	} {
		segW := lipgloss.Width(segs)
		gap := width - 4 - nameW - segW - posW
		if gap < 1 {
			continue
		}
		return mark + nameStyle.Render(name) + th.ModelinePos.Render(segs) + " " +
			th.ModelineRule.Render(strings.Repeat(string(ModelineRule), gap)) + " " +
			th.ModelinePos.Render(pos) + " "
	}

	// No room for a rule, but both facts still fit. Separate them with plain
	// space rather than dropping the position - knowing where you are matters
	// more than the rule does.
	if pad := width - 2 - nameW - posW; pad >= 1 {
		return mark + nameStyle.Render(name) + strings.Repeat(" ", pad) +
			th.ModelinePos.Render(pos) + " "
	}

	// Narrower still: the name is the one thing worth keeping.
	avail := width - 1
	if avail <= 0 {
		return mark
	}
	trimmed := lipgloss.NewStyle().MaxWidth(avail).Render(name)
	return mark + nameStyle.Render(trimmed) +
		strings.Repeat(" ", avail-lipgloss.Width(trimmed))
}
