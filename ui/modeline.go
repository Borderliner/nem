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
func modelineString(th Theme, w *view.Window, width int, active bool, nameOf NameFunc) string {
	if width <= 0 {
		return ""
	}

	// The mark occupies a cell whether or not the buffer is modified, so the
	// name never shifts sideways the moment you start typing.
	mark := " "
	if w.Buf.Modified() {
		mark = th.ModelineMark.Render(string(ModifiedMark))
	}

	name := bufferName(w.Buf, nameOf)

	// Line numbers are 1-based and columns 0-based, which is what emacs
	// reports. The column is a display column, not a rune index, so a cursor
	// past a CJK glyph reads 2 rather than 1.
	pt := w.Buf.ClampPos(w.Pt)
	pos := fmt.Sprintf("%d:%d", pt.Line+1, w.Buf.Line(pt.Line).DisplayCol(pt.Col))

	nameStyle := th.ModelineNameOff
	if active {
		nameStyle = th.ModelineName
	}

	nameW, posW := lipgloss.Width(name), lipgloss.Width(pos)

	// Preferred shape: mark, name, space, rule, space, position, trailing
	// space. Four cells of fixed spacing plus at least one cell of rule.
	if gap := width - 4 - nameW - posW; gap >= 1 {
		return mark + nameStyle.Render(name) + " " +
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
