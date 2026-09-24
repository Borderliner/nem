package icons

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// glyphNames is what each glyph is called in Nerd Fonts v3. The table in
// icons.go was built by looking these up, and this pins it: a codepoint typed
// wrong draws some other picture, which no other test would notice.
var glyphNames = map[rune]string{
	'': "fa-folder", '': "fa-folder_open", '': "fa-level_up",
	'': "oct-file_symlink_file", '': "oct-file_symlink_directory",
	'': "oct-file_binary", '': "fa-file_o", '': "fa-file_text_o",
	'': "fa-sticky_note", '': "fa-file_picture_o", '': "fa-file_sound_o",
	'': "fa-file_video_o", '': "fa-file_zipper", '': "fa-file_pdf_o",
	'': "fa-file_word_o", '': "fa-file_excel_o", '': "fa-file_powerpoint_o",
	'': "fa-font", '': "fa-database", '': "fa-lock", '': "fa-key",
	'': "fa-scale_balanced", '': "fa-book", '': "fa-table",
	'': "fa-code", '': "seti-config", '': "oct-terminal",
	'': "dev-git", '': "dev-docker", '': "seti-makefile", '': "dev-npm",
	'': "seti-go", '': "dev-rust", '': "seti-python",
	'': "seti-javascript", '': "seti-typescript", '': "seti-react",
	'': "seti-lua", '': "custom-c", '': "custom-cpp", '': "dev-java",
	'': "seti-kotlin", '': "seti-swift", '': "dev-ruby", '': "dev-php",
	'': "seti-dart", '': "seti-scala", '': "seti-haskell",
	'': "seti-elixir", '': "seti-elm", '': "seti-zig", '': "linux-nixos",
	'': "seti-ocaml", '': "seti-clojure", '': "seti-perl", '': "seti-r",
	'': "seti-julia", '': "custom-vim", '': "seti-terraform",
	'': "seti-vue", '': "seti-svelte", '': "seti-markdown",
	'': "seti-html", '': "dev-css3", '': "seti-sass", '': "seti-xml",
	'': "seti-svg", '': "seti-tex", '': "cod-json", '': "seti-yml",
	'': "custom-toml", '': "dev-cmake", '': "seti-editorconfig",
	'': "seti-yarn", '': "seti-gradle",
}

// Every glyph has a recorded name, so the check below covers the whole table.
func TestEveryGlyphHasAName(t *testing.T) {
	for _, g := range All() {
		if _, ok := glyphNames[g]; !ok {
			t.Errorf("%U has no entry in glyphNames; look it up in a Nerd Font and add it", g)
		}
	}
}

// Against a real Nerd Font, when one is installed: each glyph must be the one
// its name says. Set NEM_NERD_FONT to a .ttf to choose the font.
func TestGlyphsMatchANerdFont(t *testing.T) {
	path := os.Getenv("NEM_NERD_FONT")
	if path == "" {
		for _, pattern := range []string{
			"/usr/share/fonts/*/*/*NerdFont*-Regular.ttf",
			"/usr/share/fonts/*/*NerdFont*-Regular.ttf",
			filepath.Join(os.Getenv("HOME"), ".local/share/fonts/*NerdFont*-Regular.ttf"),
			filepath.Join(os.Getenv("HOME"), "Library/Fonts/*NerdFont*-Regular.ttf"),
		} {
			if m, _ := filepath.Glob(pattern); len(m) > 0 {
				path = m[0]
				break
			}
		}
	}
	if path == "" {
		t.Skip("no Nerd Font installed; set NEM_NERD_FONT to check the glyphs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cmap, names, err := parseFont(data)
	if err != nil {
		t.Skipf("%s: %v", path, err)
	}
	for g, want := range glyphNames {
		id, ok := cmap[g]
		if !ok {
			t.Errorf("%U (%s) is not in %s", g, want, filepath.Base(path))
			continue
		}
		if got := names[id]; got != want {
			t.Errorf("%U is %q in %s, want %q", g, got, filepath.Base(path), want)
		}
	}
}

// parseFont reads a TrueType font's format 12 character map and its version 2
// glyph names: just enough to say which picture a codepoint draws.
func parseFont(data []byte) (map[rune]int, map[int]string, error) {
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(data[o:])) }
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(data[o:])) }
	tables := map[string][2]int{}
	for i := range u16(4) {
		rec := 12 + 16*i
		tables[string(data[rec:rec+4])] = [2]int{u32(rec + 8), u32(rec + 12)}
	}
	cm, ok1 := tables["cmap"]
	post, ok2 := tables["post"]
	if !ok1 || !ok2 {
		return nil, nil, errors.New("no cmap or post table")
	}

	cmap := map[rune]int{}
	for i := range u16(cm[0] + 2) {
		sub := cm[0] + u32(cm[0]+4+8*i+4)
		if u16(sub) != 12 {
			continue
		}
		for g := range u32(sub + 12) {
			grp := sub + 16 + 12*g
			start, end, first := u32(grp), u32(grp+4), u32(grp+8)
			for c := start; c <= end; c++ {
				cmap[rune(c)] = first + c - start
			}
		}
		break
	}

	if u32(post[0]) != 0x00020000 {
		return nil, nil, errors.New("glyph names are not stored")
	}
	n := u16(post[0] + 32)
	var extra []string
	for p := post[0] + 34 + 2*n; p < post[0]+post[1]; {
		l := int(data[p])
		extra = append(extra, string(data[p+1:p+1+l]))
		p += 1 + l
	}
	names := map[int]string{}
	for id := range n {
		if ix := u16(post[0] + 34 + 2*id); ix >= 258 && ix-258 < len(extra) {
			names[id] = extra[ix-258]
		}
	}
	return cmap, names, nil
}
