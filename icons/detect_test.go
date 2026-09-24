package icons

import (
	"os/exec"
	"strings"
	"testing"
)

// Each rule of the guess, with everything else absent: no font covers the
// glyphs and no fonts are installed unless a case says so.
func TestDetect(t *testing.T) {
	type env map[string]string
	for _, tc := range []struct {
		name    string
		goos    string
		env     env
		covered bool
		fonts   []string
		want    bool
	}{
		{"Ghostty", "linux", env{"GHOSTTY_RESOURCES_DIR": "/usr/share/ghostty"}, false, nil, true},
		{"Ghostty over SSH", "linux", env{"TERM": "xterm-ghostty", "SSH_CONNECTION": "x"}, false, nil, true},
		{"Kitty", "linux", env{"TERM": "xterm-kitty"}, false, nil, true},
		{"WezTerm", "darwin", env{"TERM_PROGRAM": "WezTerm"}, false, nil, true},
		{"font installed locally", "linux", env{"TERM": "foot"}, true, nil, true},
		{"no font locally", "linux", env{"TERM": "xterm-256color"}, false, nil, false},
		{"SSH to a plain terminal", "linux", env{"TERM": "xterm-256color", "SSH_TTY": "/dev/pts/1"}, true, nil, false},
		// An agent socket is not a remote session.
		{"ssh-agent is not SSH", "linux", env{"SSH_AUTH_SOCK": "/tmp/agent"}, true, nil, true},
		{"Linux console", "linux", env{"TERM": "linux"}, true, nil, false},
		{"macOS with a Nerd Font", "darwin", env{"HOME": "/Users/u"}, false, []string{"/Users/u/Library/Fonts/HackNerdFont-Regular.ttf"}, true},
		{"macOS without", "darwin", env{"HOME": "/Users/u", "TERM_PROGRAM": "Apple_Terminal"}, false, nil, false},
		{"Windows with a Nerd Font", "windows", env{"WINDIR": `C:\Windows`}, false, []string{`C:\Windows\Fonts\CaskaydiaCoveNerdFont-Regular.ttf`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := probe{
				goos:    tc.goos,
				getenv:  func(k string) string { return tc.env[k] },
				covered: func([]rune) bool { return tc.covered },
				// Directories compared with either separator: detect joins with
				// the host's, and these cases describe other hosts.
				glob: func(pattern string) []string {
					slash := func(s string) string { return strings.ReplaceAll(s, `\`, "/") }
					dir := slash(pattern[:strings.LastIndexAny(pattern, `/\`)])
					var out []string
					for _, f := range tc.fonts {
						if strings.HasPrefix(slash(f), dir+"/") {
							out = append(out, f)
						}
					}
					return out
				},
			}
			if got, why := p.detect(); got != tc.want {
				t.Errorf("detect = %v (%s), want %v", got, why, tc.want)
			}
		})
	}
}

// The fontconfig query itself, where fontconfig exists: a letter every font
// has is covered, and a codepoint no font has is not.
func TestFontconfigCovers(t *testing.T) {
	if _, err := exec.LookPath("fc-list"); err != nil {
		t.Skip("no fontconfig here")
	}
	if !fontconfigCovers([]rune{'A'}) {
		t.Skip("fontconfig lists no fonts here")
	}
	if fontconfigCovers([]rune{'\U0010fffd'}) {
		t.Error("fontconfig claims a font covers a private codepoint no font uses")
	}
}
