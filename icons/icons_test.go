package icons

import (
	"testing"

	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

func TestFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind Kind
		want rune
	}{
		{"src", Dir, ''},
		{".", Dir, ''},
		{"..", Parent, ''},
		{".git", Dir, ''},
		{"main.go", File, ''},
		{"MAIN.GO", File, ''},
		{"a/b/lib.rs", File, ''},
		{"notes.txt", File, ''},
		{"photo.JPEG", File, ''},
		{"song.mp3", File, ''},
		{"paper.pdf", File, ''},
		{"release.tar.gz", File, ''},
		{"Makefile", File, ''},
		{"Dockerfile.dev", File, ''},
		{"README.md", File, ''},
		{"LICENSE", File, ''},
		{"go.sum", File, ''},
		{".env.local", File, ''},
		{"run", Exec, ''},
		{"link", Link, ''},
		{"linked-dir", LinkDir, ''},
		{"mystery", File, ''},
	} {
		if got := For(tc.name, tc.kind).Glyph; got != tc.want {
			t.Errorf("For(%q, %v) = %U, want %U", tc.name, tc.kind, got, tc.want)
		}
	}
}

func TestForCandidateAndBuffer(t *testing.T) {
	for in, want := range map[string]rune{
		"./":          '',
		"src/":        '',
		"cmd/nem/":    '',
		"cmd/main.go": '',
		"README.md":   '',
	} {
		if got := ForCandidate(in).Glyph; got != want {
			t.Errorf("ForCandidate(%q) = %U, want %U", in, got, want)
		}
	}
	for in, want := range map[string]rune{
		"*scratch*":  '',
		"main.go<2>": '',
		"nem/":       '',
		"notes.txt":  '',
	} {
		if got := ForBuffer(in).Glyph; got != want {
			t.Errorf("ForBuffer(%q) = %U, want %U", in, got, want)
		}
	}
}

// Every glyph is a single-width Private Use Area character with a class a
// theme styles. Single width is what the layouts assume: the icon and one
// space in front of each name.
func TestEveryGlyphIsOneCellInThePrivateUseArea(t *testing.T) {
	for _, g := range All() {
		if g < 0xe000 || g > 0xf8ff {
			t.Errorf("%U is outside the BMP Private Use Area", g)
		}
		l := text.NewLine([]rune{g})
		if w := l.Width(); w != 1 {
			t.Errorf("%U measures %d cells, want 1", g, w)
		}
	}
	for _, m := range []map[string]Icon{byExt, byName, byDirName} {
		for k, ic := range m {
			if ic.Class > syntax.Punctuation {
				t.Errorf("%s has class %v, which no theme styles", k, ic.Class)
			}
		}
	}
}
