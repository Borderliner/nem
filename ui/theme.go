// Package ui draws nem's frame onto a tcell screen.
//
// The package is split along one rule, and it is the rule that keeps the Lip
// Gloss bridge out of the hot path:
//
//   - The text area is written with scr.SetContent, one cell at a time. It is
//     redrawn on every keystroke and needs exact control of every cell, and it
//     is where syntax highlighting will later hang per-cell styles. Serialising
//     buffer text into ANSI only to parse it back out would be waste.
//   - The chrome - modelines, dividers, the echo area - is built as Lip Gloss
//     strings and blitted in. Lip Gloss does real layout work there (padding,
//     alignment, truncation by display width), and it is drawn at most once per
//     frame per window.
//
// So if the blitter were ever to prove fragile, only the chrome is exposed.
package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/gdamore/tcell/v2"
)

// ScratchName is the modeline name for a buffer with no file.
const ScratchName = "*scratch*"

// TruncMarker is shown in the last column of a line continuing past the right
// edge of its window. nem truncates long lines rather than wrapping them, so
// this is the only hint that there is more text out of view.
const TruncMarker = '$'

// DividerRune is drawn down the column between side-by-side windows.
const DividerRune = '│'

// DefaultScrollMargin is how many rows of context the render path keeps above
// and below point where the buffer allows.
const DefaultScrollMargin = 2

// Theme collects every colour and style in one place.
//
// It deliberately holds two kinds of style. The text area is drawn through
// tcell and so needs tcell.Style; the chrome is built with Lip Gloss and so
// needs lipgloss.Style. Mixing them in one struct is not an oversight - it is
// the package's central rule made visible.
type Theme struct {
	// Text area, applied via tcell directly.
	Text  tcell.Style
	Trunc tcell.Style
	// ParenMatch styles both halves of a matched bracket pair, ParenMismatch a
	// bracket whose partner is missing or of the wrong kind. Both are tcell
	// styles because they apply to the text area, which never goes through Lip
	// Gloss.
	ParenMatch    tcell.Style
	ParenMismatch tcell.Style

	// Chrome, rendered through Lip Gloss and blitted in.
	ModelineActive   lipgloss.Style
	ModelineInactive lipgloss.Style
	Divider          lipgloss.Style
	// Echo styles a transient message, which is dimmed to read as ephemeral.
	Echo lipgloss.Style
	// Mini styles an active minibuffer prompt. It is deliberately not dimmed:
	// text being typed is ordinary content, not a passing notice.
	Mini lipgloss.Style

	// ScrollMargin is the rows of context kept around point.
	ScrollMargin int
}

// DefaultTheme returns nem's default look.
//
// The palette is drawn from the 256-colour cube rather than from named ANSI
// colours, so it reads the same against a light or a dark terminal background
// instead of inheriting whatever the user's palette maps "blue" to. The text
// area itself is left at the terminal's default foreground and background,
// which is what makes an editor feel native.
func DefaultTheme() Theme {
	var (
		dim       = lipgloss.Color("244")
		divider   = lipgloss.Color("240")
		modeBgOn  = lipgloss.Color("60")
		modeFgOn  = lipgloss.Color("231")
		modeBgOff = lipgloss.Color("236")
		modeFgOff = lipgloss.Color("245")
	)

	return Theme{
		Text:  tcell.StyleDefault,
		Trunc: tcell.StyleDefault.Foreground(tcell.ColorGray),
		// A matched pair is marked by weight and reverse video rather than by a
		// hue, so it stays legible whatever the terminal palette is; a mismatch
		// is red, the one colour that reads as wrong everywhere.
		ParenMatch:    tcell.StyleDefault.Bold(true).Reverse(true),
		ParenMismatch: tcell.StyleDefault.Bold(true).Foreground(tcell.ColorWhite).Background(tcell.ColorRed),

		ModelineActive:   lipgloss.NewStyle().Foreground(modeFgOn).Background(modeBgOn).Bold(true),
		ModelineInactive: lipgloss.NewStyle().Foreground(modeFgOff).Background(modeBgOff),
		Divider:          lipgloss.NewStyle().Foreground(divider),
		Echo:             lipgloss.NewStyle().Foreground(dim),
		Mini:             lipgloss.NewStyle(),

		ScrollMargin: DefaultScrollMargin,
	}
}
