package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hajianpour/nem/view"
)

// modelineString builds one window's status bar, exactly width cells wide.
//
// The layout is emacs-shaped: a modified flag and the buffer name on the left,
// the position flush right. Padding is computed in display cells rather than
// runes, so a buffer named in CJK still lines up.
func modelineString(th Theme, w *view.Window, width int, active bool) string {
	if width <= 0 {
		return ""
	}

	flag := "--"
	if w.Buf.Modified() {
		flag = "**"
	}

	name := ScratchName
	if p := w.Buf.Path(); p != "" {
		name = filepath.Base(p)
	}

	// Line numbers are 1-based and columns 0-based, which is what emacs
	// reports. The column is a display column, not a rune index, so a cursor
	// past a CJK glyph reads 2 rather than 1.
	pt := w.Buf.ClampPos(w.Pt)
	col := w.Buf.Line(pt.Line).DisplayCol(pt.Col)

	left := fmt.Sprintf(" %s  %s", flag, name)
	right := fmt.Sprintf("%d:%d ", pt.Line+1, col)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	content := left + strings.Repeat(" ", gap) + right

	style := th.ModelineInactive
	if active {
		style = th.ModelineActive
	}
	// Width pads a short modeline out to the pane; MaxWidth cuts a long one
	// down. Together they guarantee exactly width cells, so a modeline can
	// never bleed into the divider column beside it.
	return style.Width(width).MaxWidth(width).Render(content)
}
