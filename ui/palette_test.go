package ui

import (
	"math"
	"os"
	"testing"
)

// relLum is the WCAG relative luminance of an sRGB hex colour.
func relLum(hex string) float64 {
	var rgb [3]float64
	for i := 0; i < 3; i++ {
		var v int
		for j := 0; j < 2; j++ {
			c := hex[1+i*2+j]
			d := 0
			switch {
			case c >= '0' && c <= '9':
				d = int(c - '0')
			case c >= 'a' && c <= 'f':
				d = int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = int(c-'A') + 10
			}
			v = v*16 + d
		}
		f := float64(v) / 255
		if f <= 0.03928 {
			rgb[i] = f / 12.92
		} else {
			rgb[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}

func contrast(a, b string) float64 {
	la, lb := relLum(a), relLum(b)
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// Each palette must be readable on the ground it is for. This is measured
// rather than eyeballed, so a future colour change that looks fine on one
// developer's terminal cannot quietly make the editor unreadable on someone
// else's. WCAG AA wants 4.5:1 for body text.
//
// The dark palette is checked against a near-black ground rather than pure
// black, because a terminal's "black" is usually a little lighter and that is
// the harder case.
func TestSyntaxPalettesAreReadable(t *testing.T) {
	const minRatio = 4.4 // AA, with a hair of slack for the near-black ground

	for _, p := range []struct {
		name   string
		ground string
		cols   map[string]string
	}{
		{"dark", "#1e1e1e", map[string]string{
			"keyword": darkKeyword, "string": darkString, "comment": darkComment,
			"number": darkNumber, "function": darkFunction, "type": darkType,
		}},
		{"light", "#ffffff", map[string]string{
			"keyword": lightKeyword, "string": lightString, "comment": lightComment,
			"number": lightNumber, "function": lightFunction, "type": lightType,
		}},
	} {
		for role, col := range p.cols {
			if got := contrast(col, p.ground); got < minRatio {
				t.Errorf("%s palette: %s %s on %s is %.2f:1, want at least %.1f:1",
					p.name, role, col, p.ground, got, minRatio)
			}
		}
	}
}

// The dark palette on a white ground is the mistake this guards against: every
// One Dark colour measures below 4:1 there, so shipping one palette for both
// grounds would have left light-terminal users with washed-out code.
func TestTheDarkPaletteIsNotUsableOnWhite(t *testing.T) {
	if got := contrast(darkType, "#ffffff"); got >= 4.4 {
		t.Errorf("dark type on white is %.2f:1 - if this now passes, the two-palette "+
			"split may no longer be needed, so re-check the rest before removing it", got)
	}
}

func TestTerminalIsLight(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"", false},     // unset: guess dark, the safer miss
		{"15;0", false}, // light text on black
		{"0;15", true},  // black text on bright white
		{"0;7", true},   // black on white
		{"15;default;0", false},
		{"0;default;15", true},
		{"nonsense", false},
	} {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("COLORFGBG", tc.env)
			if tc.env == "" {
				os.Unsetenv("COLORFGBG")
			}
			if got := TerminalIsLight(); got != tc.want {
				t.Errorf("COLORFGBG=%q: TerminalIsLight() = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

// Switching palettes must actually change the styles.
func TestUseSyntaxPaletteSwitches(t *testing.T) {
	th := DefaultTheme()
	th.UseSyntaxPalette(false)
	dark := th.SyntaxStyle
	th.UseSyntaxPalette(true)
	if th.SyntaxStyle == dark {
		t.Error("UseSyntaxPalette(true) left the dark styles in place")
	}
}
