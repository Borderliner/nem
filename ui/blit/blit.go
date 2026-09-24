// Package blit renders Lip Gloss-styled ANSI strings into a tcell screen.
//
// Lip Gloss produces strings carrying in-band SGR escape sequences; tcell owns
// a grid of individually styled cells. The two do not compose directly. This
// package bridges them: it decodes a styled string into grapheme clusters plus
// a running style, and writes each cluster into the destination screen as a
// tcell cell.
//
// Decoding is delegated to github.com/charmbracelet/x/ansi, which arrives as a
// Lip Gloss dependency, and SGR interpretation to
// github.com/charmbracelet/x/cellbuf. Both use rivo/uniseg for grapheme
// segmentation, as does tcell itself, so all three agree on how many cells a
// cluster occupies. That agreement is what makes this bridge safe for emoji and
// CJK text.
package blit

import (
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/gdamore/tcell/v2"
	"github.com/muesli/termenv"
)

// SyncLipglossProfile points Lip Gloss at the terminal tcell actually opened.
//
// Lip Gloss decides how much colour to emit by probing os.Stdout. That probe is
// wrong for us in both directions: tcell owns the terminal, so stdout tells us
// nothing useful, and if stdout is ever redirected Lip Gloss silently renders
// every style with no colour at all. tcell has already done real terminfo
// negotiation, so its answer is the authoritative one.
//
// Call this once after Screen.Init and again on a Screen that has been
// reinitialised. Without it, chrome can come out unstyled for reasons that look
// like a blitter bug and are not.
func SyncLipglossProfile(scr tcell.Screen) {
	if scr == nil {
		return
	}
	var p termenv.Profile
	switch n := scr.Colors(); {
	case n >= 1<<24:
		p = termenv.TrueColor
	case n >= 256:
		p = termenv.ANSI256
	case n >= 16:
		p = termenv.ANSI
	default:
		p = termenv.Ascii
	}
	lipgloss.SetColorProfile(p)
}

// Draw renders a Lip Gloss-styled string into scr with its top-left corner at
// (x, y), clipped to a w by h cell rectangle. Content extending past the
// rectangle is dropped, not wrapped: callers are expected to have laid the
// string out already.
//
// Newlines advance a row and return to the left edge of the rectangle; carriage
// returns return to the left edge alone. Escape sequences other than SGR are
// consumed and ignored, so unsupported terminal control cannot corrupt the
// grid.
func Draw(scr tcell.Screen, x, y, w, h int, s string) {
	if scr == nil || w <= 0 || h <= 0 || s == "" {
		return
	}

	parser := getParser()
	defer parsers.Put(parser)
	var (
		pen      cellbuf.Style
		state    byte
		row, col int
		// Position of the last cluster written, so a stray zero-width mark can
		// attach to it rather than claiming a cell of its own.
		lastX, lastY = -1, -1
		lastCluster  []rune
	)

	for len(s) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(s, state, parser)
		if n == 0 {
			break // Defensive: a zero-length read would spin forever.
		}

		switch {
		case width > 0:
			// A printable grapheme cluster.
			if row >= h {
				return // Past the bottom edge; nothing further can land.
			}
			runes := []rune(seq)
			// Draw only if the cluster fits entirely inside the rectangle. A
			// wide glyph straddling the right edge is dropped rather than
			// half-drawn, which would leave a broken cell behind.
			if col >= 0 && col+width <= w && len(runes) > 0 {
				cx, cy := x+col, y+row
				scr.SetContent(cx, cy, runes[0], runes[1:], toTcellStyle(pen))
				lastX, lastY, lastCluster = cx, cy, runes
			} else {
				lastX, lastY = -1, -1
			}
			col += width

		case ansi.HasCsiPrefix(seq) && parser.Command() == 'm':
			// SGR: the only sequence class that changes how cells look.
			cellbuf.ReadStyle(parser.Params(), &pen)

		case ansi.Equal(seq, "\n"):
			row++
			col = 0
			lastX, lastY = -1, -1

		case ansi.Equal(seq, "\r"):
			col = 0
			lastX, lastY = -1, -1

		default:
			// Width-zero and not a sequence we act on. If it is printable text
			// (a lone combining mark that opened the string, say), fold it into
			// the previous cluster so it decorates that cell instead of being
			// lost. Everything else - cursor moves, OSC, unknown escapes - is
			// deliberately dropped.
			if isPrintableZeroWidth(seq) && lastX >= 0 {
				merged := append(append([]rune{}, lastCluster...), []rune(seq)...)
				scr.SetContent(lastX, lastY, merged[0], merged[1:], toTcellStyle(pen))
				lastCluster = merged
			}
		}

		state = newState
		s = s[n:]
	}
}

// parsers are reused rather than made per call. Each carries a 64KB buffer for
// control-string payloads, and a frame draws a modeline per window, the
// dividers and the echo row: a new parser each time was the largest
// allocation in a frame, and all of it garbage by the next.
var parsers = sync.Pool{New: func() any { return ansi.NewParser() }}

// getParser returns a parser in its ground state.
func getParser() *ansi.Parser {
	p := parsers.Get().(*ansi.Parser)
	p.Reset()
	return p
}

// Size reports the cell dimensions a styled string occupies: the width of its
// widest line and its line count. It decodes via the same path as Draw, so the
// two cannot disagree about how much room a string needs.
func Size(s string) (w, h int) {
	if s == "" {
		return 0, 0
	}

	parser := getParser()
	defer parsers.Put(parser)
	var (
		state byte
		col   int
	)
	h = 1

	for len(s) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(s, state, parser)
		if n == 0 {
			break
		}

		switch {
		case width > 0:
			col += width
			if col > w {
				w = col
			}
		case ansi.Equal(seq, "\n"):
			h++
			col = 0
		case ansi.Equal(seq, "\r"):
			col = 0
		}

		state = newState
		s = s[n:]
	}
	return w, h
}

// isPrintableZeroWidth reports whether a zero-width sequence is text rather
// than a control or escape sequence.
func isPrintableZeroWidth(seq string) bool {
	if seq == "" {
		return false
	}
	r := []rune(seq)[0]
	return r >= 0x20 && r != 0x7f
}

// toTcellStyle converts a cellbuf pen into the equivalent tcell style.
func toTcellStyle(s cellbuf.Style) tcell.Style {
	st := tcell.StyleDefault.
		Foreground(toTcellColor(s.Fg)).
		Background(toTcellColor(s.Bg))

	if s.Attrs&cellbuf.BoldAttr != 0 {
		st = st.Bold(true)
	}
	if s.Attrs&cellbuf.FaintAttr != 0 {
		st = st.Dim(true)
	}
	if s.Attrs&cellbuf.ItalicAttr != 0 {
		st = st.Italic(true)
	}
	if s.Attrs&(cellbuf.SlowBlinkAttr|cellbuf.RapidBlinkAttr) != 0 {
		st = st.Blink(true)
	}
	if s.Attrs&cellbuf.ReverseAttr != 0 {
		st = st.Reverse(true)
	}
	if s.Attrs&cellbuf.StrikethroughAttr != 0 {
		st = st.StrikeThrough(true)
	}

	if ul := toUnderlineStyle(s.UlStyle); ul != tcell.UnderlineStyleNone {
		st = st.Underline(ul)
		if s.Ul != nil {
			st = st.Underline(toTcellColor(s.Ul))
		}
	}

	// Conceal has no tcell attribute. Render it as foreground-on-background so
	// the text is genuinely unreadable rather than silently visible.
	if s.Attrs&cellbuf.ConcealAttr != 0 {
		st = st.Foreground(toTcellColor(s.Bg))
	}
	return st
}

// toUnderlineStyle maps cellbuf underline styles onto tcell's. cellbuf's type
// is an alias of ansi.UnderlineStyle, whose values match the SGR 4:n
// sub-parameters.
func toUnderlineStyle(u cellbuf.UnderlineStyle) tcell.UnderlineStyle {
	switch u {
	case cellbuf.SingleUnderline:
		return tcell.UnderlineStyleSolid
	case cellbuf.DoubleUnderline:
		return tcell.UnderlineStyleDouble
	case cellbuf.CurlyUnderline:
		return tcell.UnderlineStyleCurly
	case cellbuf.DottedUnderline:
		return tcell.UnderlineStyleDotted
	case cellbuf.DashedUnderline:
		return tcell.UnderlineStyleDashed
	default:
		return tcell.UnderlineStyleNone
	}
}

// toTcellColor converts an ANSI color to a tcell color.
//
// Palette colors stay palette colors rather than being flattened to RGB, so the
// user's terminal theme continues to apply: "red" means the red they chose.
// Only true colors, which carry their own RGB, are passed through as RGB.
func toTcellColor(c ansi.Color) tcell.Color {
	if c == nil {
		return tcell.ColorDefault
	}
	switch v := c.(type) {
	case ansi.BasicColor:
		return tcell.PaletteColor(int(v))
	case ansi.ExtendedColor:
		return tcell.PaletteColor(int(v))
	case ansi.TrueColor:
		return tcell.NewHexColor(int32(v & 0xFFFFFF))
	}
	// Any other color.Color implementation: RGBA is 16-bit alpha-premultiplied.
	r, g, b, _ := c.RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))
}
