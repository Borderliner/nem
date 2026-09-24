package icons

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Whether the terminal can draw the icons cannot be asked: no escape sequence
// reports which font a terminal renders with, and a glyph the font lacks still
// advances the cursor one cell, so even measuring tells nothing. Detect guesses
// from what can be known, in order of how much each says:
//
//  1. Ghostty, Kitty and WezTerm carry the Nerd Font symbols themselves, so
//     in them the icons draw whatever font is configured. They say who they
//     are in the environment, and TERM names Ghostty and Kitty even over SSH.
//  2. Over SSH, and in the Linux console, nothing local says anything: the
//     fonts that matter are on the other machine, or there are none.
//  3. Locally, if an installed font has the glyphs, the terminal will almost
//     always find it - as its own font, or by the fallback every modern
//     terminal does for a character its font lacks. fontconfig answers this
//     directly on Linux and the BSDs; on macOS and Windows a Nerd Font in the
//     fonts folders is the sign.
//
// It errs towards off. An icon that cannot be drawn is an empty box beside
// every file name, which is worse than no icon.

// Detect reports whether the terminal nem is running in can probably draw the
// icons.
func Detect() bool {
	ok, _ := realProbe().detect()
	return ok
}

// probe is what Detect consults, injectable so every rule can be tested on any
// machine.
type probe struct {
	goos   string
	getenv func(string) string
	// covered reports whether an installed font has every rune in rs.
	covered func(rs []rune) bool
	glob    func(pattern string) []string
}

func realProbe() probe {
	return probe{
		goos:    runtime.GOOS,
		getenv:  os.Getenv,
		covered: fontconfigCovers,
		glob: func(p string) []string {
			m, _ := filepath.Glob(p)
			return m
		},
	}
}

// detect is Detect with its reasoning, for tests.
func (p probe) detect() (bool, string) {
	term := p.getenv("TERM")
	switch {
	case p.getenv("GHOSTTY_RESOURCES_DIR") != "", term == "xterm-ghostty",
		p.getenv("TERM_PROGRAM") == "ghostty":
		return true, "Ghostty carries the symbols"
	case p.getenv("KITTY_WINDOW_ID") != "", term == "xterm-kitty":
		return true, "Kitty carries the symbols"
	case p.getenv("WEZTERM_PANE") != "", p.getenv("TERM_PROGRAM") == "WezTerm":
		return true, "WezTerm carries the symbols"
	case term == "linux", term == "dumb":
		return false, "a console with no font to speak of"
	case p.getenv("SSH_CONNECTION") != "", p.getenv("SSH_CLIENT") != "", p.getenv("SSH_TTY") != "":
		return false, "over SSH the terminal's fonts are on the other machine"
	}

	switch p.goos {
	case "darwin":
		home := p.getenv("HOME")
		for _, dir := range []string{filepath.Join(home, "Library", "Fonts"), "/Library/Fonts"} {
			if len(p.glob(filepath.Join(dir, "*Nerd*"))) > 0 {
				return true, "a Nerd Font is installed"
			}
		}
		return false, "no Nerd Font installed"
	case "windows":
		for _, dir := range []string{
			filepath.Join(p.getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
			filepath.Join(p.getenv("WINDIR"), "Fonts"),
		} {
			if len(p.glob(filepath.Join(dir, "*Nerd*"))) > 0 {
				return true, "a Nerd Font is installed"
			}
		}
		return false, "no Nerd Font installed"
	}
	// Two glyphs from two of the sets the icons draw on: a font with both is a
	// Nerd Font or its symbols-only companion.
	if p.covered([]rune{'', ''}) {
		return true, "an installed font has the glyphs"
	}
	return false, "no installed font has the glyphs"
}

// fontconfigCovers asks fontconfig for any font containing every rune in rs.
// No fontconfig - a server, a container - is a no.
func fontconfigCovers(rs []rune) bool {
	hex := make([]string, len(rs))
	for i, r := range rs {
		hex[i] = strconv.FormatInt(int64(r), 16)
	}
	// Bounded, since it runs at startup: fontconfig answers from its cache in
	// milliseconds, and a machine where it does not should not delay the editor.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-list", "--format", "%{family[0]}\n",
		":charset="+strings.Join(hex, " ")).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}
