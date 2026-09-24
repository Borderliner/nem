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

// ModifiedMark sits at the head of a modeline when the buffer has unsaved
// changes, in place of emacs's ** flag. One mark carries the whole state: a
// clean buffer shows a blank cell, so the buffer name never shifts position
// when you start typing.
const ModifiedMark = '▍'

// ModelineRule fills a modeline between the buffer name and the position
// readout. The rule is what separates stacked panes, since a horizontal split
// draws no divider row of its own - the modeline is the boundary.
const ModelineRule = '─'

// DefaultScrollMargin is how many rows of context the render path keeps above
// and below point where the buffer allows.
const DefaultScrollMargin = 2

// nem's palette is three colours, and the most important thing about it is the
// absence of a fourth: no background is ever painted behind text. The editor
// sits in whatever terminal colours you already chose instead of replacing
// them, which is what makes it feel native rather than like an application
// squatting in your terminal.
//
// The reference is letterpress, not a CRT - a hairline rule with the buffer
// name set into it. Focus is shown by weight rather than by a coloured bar,
// because a bar is the thing that dates a terminal program.
//
// Values are truecolor; tcell degrades them for 256- and 16-colour terminals.
const (
	// colourQuiet carries secondary information: inactive buffer names, the
	// position readout, transient messages. Deliberately a mid-grey rather
	// than a dark one - nem cannot know whether it is sitting on a light or a
	// dark terminal, so both ends have to stay legible.
	colourQuiet = "#8a857c"
	// colourRule draws each modeline's hairline and the divider between
	// side-by-side panes. Dimmer than quiet, still readable either way.
	colourRule = "#6b655b"
	// colourMark is the only saturated colour in the editor and it marks
	// exactly one thing: unsaved changes. The palette's single accent is spent
	// on the one piece of state you can lose work by ignoring.
	colourMark = "#b4543a"
)

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
	// bracket whose partner is missing or of the wrong kind. Match is marked by
	// weight and an underline rather than by a hue, so it needs no colour and
	// cannot clash with a terminal palette; a mismatch borrows the accent,
	// since an unclosed bracket is the other thing worth interrupting you for.
	ParenMatch    tcell.Style
	ParenMismatch tcell.Style
	// LineNumbers turns the gutter on. It is on by default: a line number is
	// the one piece of chrome you want in view constantly rather than on
	// request, and every editor people arrive from shows them.
	//
	// The numbers live only here, in the renderer. They are never in the buffer,
	// so a region cannot reach them and C-w cannot copy them - the gutter is
	// drawn outside the span handed to drawLine, which is what makes that
	// structural rather than a rule someone has to remember.
	//
	// LineNumber is quiet for the same reason inactive buffer names are: it is
	// secondary information you read only when you are looking for it.
	// LineNumberCurrent marks the line point is on, using the terminal's own
	// foreground plus weight rather than a colour, so it reads correctly on a
	// light or a dark background without nem knowing which it is on.
	LineNumbers       bool
	LineNumber        tcell.Style
	LineNumberCurrent tcell.Style

	// Region styles the cells between point and the mark.
	//
	// A selection is the one thing in the editor that legitimately needs a
	// background: a span cannot be indicated with a foreground alone. That sits
	// against the palette's rule that nothing paints a background, which exists
	// so nem works on a light or a dark terminal without knowing which it is on.
	//
	// Reverse resolves it. Swapping foreground and background guarantees correct
	// contrast on any palette because it adapts to the palette rather than
	// guessing at it, and an inverted span is the universal terminal convention
	// for a selection.
	//
	// This is deliberately not a contradiction of dropping reverse video from
	// bracket matching. Reverse as decoration - marking something that is merely
	// interesting - is the dated tell. Reverse as selection is what it means
	// everywhere, and is the only palette-independent way to mark a span.
	Region tcell.Style

	// Panels - the completion list and prefix-key discovery - are drawn cell by
	// cell rather than through Lip Gloss, because they need to emphasise
	// individual runes inside a row and to clip a wide glyph at the border.
	//
	// PanelBorder is the same hairline as the modeline rule and the pane divider,
	// so all of nem's chrome reads as one system rather than three.
	//
	// PanelSelected marks the row the user is on, and is reverse for exactly the
	// reason Region is: it is a selection, and swapping foreground and background
	// adapts to the terminal's palette instead of guessing at it.
	//
	// PanelMatch emphasises the runes a fuzzy query matched. Weight alone,
	// deliberately: it composes with PanelSelected's inversion instead of
	// fighting it, it needs no colour so it cannot clash with a palette, and it
	// leaves the single accent spent where it belongs - on unsaved changes.
	PanelBorder   tcell.Style
	PanelSelected tcell.Style
	PanelMatch    tcell.Style
	// ListCursor is the bar across the row point is on in a listing buffer. It
	// is the panel's selected row in another place - the same act of picking
	// one item from a list - so it is reverse for the same reason.
	ListCursor tcell.Style
	// MiniNote styles the candidate count beside a prompt at the bottom of the
	// screen: quiet, since it is secondary to what is being typed.
	MiniNote tcell.Style
	// PanelNote styles a candidate's note - a command's keys at M-x - quiet
	// for the same reason: it is read when looked for, not first.
	PanelNote tcell.Style

	// Chrome, rendered through Lip Gloss and blitted in. The modeline is built
	// from four separately styled segments rather than one flat bar, which is
	// what lets focus read as weight instead of as a block of colour.
	//
	// ModelineName carries no colour on purpose: the focused buffer name uses
	// the terminal's own foreground, so it is the most legible thing on screen
	// whatever palette you run.
	ModelineName    lipgloss.Style
	ModelineNameOff lipgloss.Style
	ModelineRule    lipgloss.Style
	ModelinePos     lipgloss.Style
	ModelineMark    lipgloss.Style

	Divider lipgloss.Style
	// Echo styles a transient message, which is quiet to read as ephemeral.
	Echo lipgloss.Style
	// Mini styles an active minibuffer prompt. It is deliberately not quieted:
	// text being typed is ordinary content, not a passing notice.
	Mini lipgloss.Style

	// Syntax enables colouring the text area from a SpansFunc. Off restores
	// exactly the uncoloured rendering.
	Syntax bool
	// SyntaxStyle maps a syntax.Class to its style. Foregrounds only: see
	// defaultSyntaxStyles and the note above about never painting a background.
	SyntaxStyle [numSyntaxClasses]tcell.Style

	// ScrollMargin is the rows of context kept around point.
	ScrollMargin int
}

// DefaultTheme returns nem's default look.
func DefaultTheme() Theme {
	var (
		quiet = lipgloss.Color(colourQuiet)
		rule  = lipgloss.Color(colourRule)
		mark  = lipgloss.Color(colourMark)
	)

	return Theme{
		Text:  tcell.StyleDefault,
		Trunc: tcell.StyleDefault.Foreground(tcell.GetColor(colourQuiet)),

		ParenMatch:    tcell.StyleDefault.Bold(true).Underline(true),
		ParenMismatch: tcell.StyleDefault.Bold(true).Foreground(tcell.GetColor(colourMark)),
		Region:        tcell.StyleDefault.Reverse(true),

		LineNumbers:       true,
		LineNumber:        tcell.StyleDefault.Foreground(tcell.GetColor(colourQuiet)),
		LineNumberCurrent: tcell.StyleDefault.Bold(true),

		PanelBorder:   tcell.StyleDefault.Foreground(tcell.GetColor(colourRule)),
		PanelSelected: tcell.StyleDefault.Reverse(true),
		PanelMatch:    tcell.StyleDefault.Bold(true),
		ListCursor:    tcell.StyleDefault.Reverse(true),
		MiniNote:      tcell.StyleDefault.Foreground(tcell.GetColor(colourQuiet)),
		PanelNote:     tcell.StyleDefault.Foreground(tcell.GetColor(colourQuiet)),

		ModelineName:    lipgloss.NewStyle().Bold(true),
		ModelineNameOff: lipgloss.NewStyle().Foreground(quiet),
		ModelineRule:    lipgloss.NewStyle().Foreground(rule),
		ModelinePos:     lipgloss.NewStyle().Foreground(quiet),
		ModelineMark:    lipgloss.NewStyle().Foreground(mark).Bold(true),

		Divider: lipgloss.NewStyle().Foreground(rule),
		Echo:    lipgloss.NewStyle().Foreground(quiet),
		Mini:    lipgloss.NewStyle(),

		Syntax:      true,
		SyntaxStyle: defaultSyntaxStyles(),

		ScrollMargin: DefaultScrollMargin,
	}
}
