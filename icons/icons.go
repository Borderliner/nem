// Package icons picks a Nerd Font glyph for a file: a folder for a directory,
// the Go gopher's mark for a .go file, a picture for a PNG.
//
// The glyphs live in the Private Use Area and draw only in a terminal whose
// font has them - a Nerd Font, or a terminal like Ghostty, Kitty or WezTerm
// that ships the symbols itself. Everywhere else they are empty boxes, which is
// why the editor has a setting to turn them off.
//
// Every codepoint here was checked by name against a Nerd Fonts v3 font
// (glyph names are in each comment). Font Awesome's are the most stable across
// Nerd Fonts versions, so they carry the categories; the language marks come
// from the Seti and Devicons sets.
package icons

import (
	"path/filepath"
	"strings"

	"github.com/Borderliner/nem/syntax"
)

// Icon is one glyph and how to colour it.
//
// The colour is a syntax class rather than a colour of its own, so icons take
// the light or dark palette with everything else instead of adding a palette
// a theme would have to match.
type Icon struct {
	Glyph rune
	Class syntax.Class
}

// Kind is what the caller knows about a file beyond its name.
type Kind int

const (
	File Kind = iota
	Dir
	// Parent is the ".." entry, the way up.
	Parent
	Link
	LinkDir
	Exec
)

var (
	folder     = Icon{'\uf07b', syntax.Function} // fa-folder
	folderOpen = Icon{'\uf07c', syntax.Function} // fa-folder_open
	levelUp    = Icon{'\uf148', syntax.Function} // fa-level_up
	linkFile   = Icon{'\uf481', syntax.Keyword}  // oct-file_symlink_file
	linkDir    = Icon{'\uf482', syntax.Keyword}  // oct-file_symlink_directory
	program    = Icon{'\uf471', syntax.String}   // oct-file_binary
	plain      = Icon{'\uf016', syntax.Comment}  // fa-file_o
	textFile   = Icon{'\uf0f6', syntax.Comment}  // fa-file_text_o
	special    = Icon{'\uf249', syntax.Comment}  // fa-sticky_note

	image    = Icon{'\uf1c5', syntax.Keyword}  // fa-file_picture_o
	audio    = Icon{'\uf1c7', syntax.Keyword}  // fa-file_sound_o
	video    = Icon{'\uf1c8', syntax.Keyword}  // fa-file_video_o
	archive  = Icon{'\uf1c6', syntax.Number}   // fa-file_zipper
	pdf      = Icon{'\uf1c1', syntax.Number}   // fa-file_pdf_o
	word     = Icon{'\uf1c2', syntax.Function} // fa-file_word_o
	sheet    = Icon{'\uf1c3', syntax.String}   // fa-file_excel_o
	slides   = Icon{'\uf1c4', syntax.Number}   // fa-file_powerpoint_o
	font     = Icon{'\uf031', syntax.Comment}  // fa-font
	database = Icon{'\uf1c0', syntax.Type}     // fa-database
	lock     = Icon{'\uf023', syntax.Comment}  // fa-lock
	key      = Icon{'\uf084', syntax.Type}     // fa-key
	license  = Icon{'\uf24e', syntax.Type}     // fa-scale_balanced
	book     = Icon{'\uf02d', syntax.Type}     // fa-book
	table    = Icon{'\uf0ce', syntax.String}   // fa-table
	code     = Icon{'\uf121', syntax.Type}     // fa-code
	config   = Icon{'\ue615', syntax.Comment}  // seti-config
	shell    = Icon{'\uf489', syntax.String}   // oct-terminal
	git      = Icon{'\ue702', syntax.Number}   // dev-git
	docker   = Icon{'\ue7b0', syntax.Function} // dev-docker
	makefile = Icon{'\ue673', syntax.Number}   // seti-makefile
	npm      = Icon{'\ue71e', syntax.Number}   // dev-npm
)

// byExt maps a lower-cased extension to its icon.
var byExt = map[string]Icon{
	// languages
	"go":     {'\ue627', syntax.Function}, // seti-go
	"rs":     {'\ue7a8', syntax.Number},   // dev-rust
	"py":     {'\ue606', syntax.Type},     // seti-python
	"js":     {'\ue60c', syntax.Type},     // seti-javascript
	"mjs":    {'\ue60c', syntax.Type},
	"cjs":    {'\ue60c', syntax.Type},
	"ts":     {'\ue628', syntax.Function}, // seti-typescript
	"mts":    {'\ue628', syntax.Function},
	"jsx":    {'\ue625', syntax.Function}, // seti-react
	"tsx":    {'\ue625', syntax.Function},
	"lua":    {'\ue620', syntax.Function}, // seti-lua
	"c":      {'\ue61e', syntax.Function}, // custom-c
	"h":      {'\ue61e', syntax.Keyword},
	"cpp":    {'\ue61d', syntax.Function}, // custom-cpp
	"cc":     {'\ue61d', syntax.Function},
	"cxx":    {'\ue61d', syntax.Function},
	"hpp":    {'\ue61d', syntax.Keyword},
	"java":   {'\ue738', syntax.Number},   // dev-java
	"kt":     {'\ue634', syntax.Keyword},  // seti-kotlin
	"swift":  {'\ue699', syntax.Number},   // seti-swift
	"rb":     {'\ue739', syntax.Number},   // dev-ruby
	"php":    {'\ue73d', syntax.Keyword},  // dev-php
	"dart":   {'\ue64c', syntax.Function}, // seti-dart
	"scala":  {'\ue68e', syntax.Number},   // seti-scala
	"hs":     {'\ue61f', syntax.Keyword},  // seti-haskell
	"ex":     {'\ue62d', syntax.Keyword},  // seti-elixir
	"exs":    {'\ue62d', syntax.Keyword},
	"elm":    {'\ue62c', syntax.Function}, // seti-elm
	"zig":    {'\ue6a9', syntax.Number},   // seti-zig
	"nix":    {'\uf313', syntax.Function}, // linux-nixos
	"ml":     {'\ue67a', syntax.Number},   // seti-ocaml
	"clj":    {'\ue642', syntax.String},   // seti-clojure
	"pl":     {'\ue67e', syntax.Function}, // seti-perl
	"r":      {'\ue68a', syntax.Function}, // seti-r
	"jl":     {'\ue624', syntax.Keyword},  // seti-julia
	"vim":    {'\ue62b', syntax.String},   // custom-vim
	"tf":     {'\ue69a', syntax.Keyword},  // seti-terraform
	"vue":    {'\ue6a0', syntax.String},   // seti-vue
	"svelte": {'\ue697', syntax.Number},   // seti-svelte
	"sh":     shell,
	"bash":   shell,
	"zsh":    shell,
	"fish":   shell,

	// markup, styles and data
	"md":       {'\ue609', syntax.Function}, // seti-markdown
	"markdown": {'\ue609', syntax.Function},
	"html":     {'\ue60e', syntax.Number}, // seti-html
	"htm":      {'\ue60e', syntax.Number},
	"css":      {'\ue749', syntax.Function}, // dev-css3
	"scss":     {'\ue603', syntax.Keyword},  // seti-sass
	"sass":     {'\ue603', syntax.Keyword},
	"xml":      {'\ue619', syntax.Number},  // seti-xml
	"svg":      {'\ue698', syntax.Type},    // seti-svg
	"tex":      {'\ue69b', syntax.String},  // seti-tex
	"json":     {'\ueb0f', syntax.Type},    // cod-json
	"yaml":     {'\ue6a8', syntax.Keyword}, // seti-yml
	"yml":      {'\ue6a8', syntax.Keyword},
	"toml":     {'\ue6b2', syntax.Comment}, // custom-toml
	"ini":      config,
	"conf":     config,
	"cfg":      config,
	"env":      config,
	"csv":      table,
	"tsv":      table,
	"sql":      database,
	"txt":      textFile,
	"log":      textFile,
	"diff":     code,
	"patch":    code,
	"lock":     lock,
	"sum":      lock,

	// things that are not text
	"png": image, "jpg": image, "jpeg": image, "gif": image, "webp": image,
	"bmp": image, "ico": image, "tif": image, "tiff": image, "heic": image,
	"avif": image, "psd": image, "xcf": image,
	"mp3": audio, "flac": audio, "wav": audio, "ogg": audio, "oga": audio,
	"opus": audio, "m4a": audio, "aac": audio,
	"mp4": video, "mkv": video, "webm": video, "avi": video, "mov": video,
	"m4v": video, "wmv": video,
	"zip": archive, "gz": archive, "tgz": archive, "bz2": archive,
	"xz": archive, "zst": archive, "7z": archive, "rar": archive,
	"tar": archive, "iso": archive, "deb": archive, "rpm": archive,
	"jar": archive, "apk": archive, "dmg": archive,
	"pdf": pdf,
	"doc": word, "docx": word, "odt": word, "rtf": word,
	"xls": sheet, "xlsx": sheet, "ods": sheet,
	"ppt": slides, "pptx": slides, "odp": slides,
	"ttf": font, "otf": font, "woff": font, "woff2": font,
	"db": database, "sqlite": database, "sqlite3": database,
	"pem": key, "crt": key, "key": key, "pub": key, "gpg": key, "asc": key,
	"exe": program, "dll": program, "so": program, "dylib": program,
	"o": program, "a": program, "wasm": program, "class": program,
}

// byName maps a whole lower-cased file name to its icon, for files known by
// name rather than by extension.
var byName = map[string]Icon{
	"makefile":           makefile,
	"gnumakefile":        makefile,
	"cmakelists.txt":     {'\ue794', syntax.Function}, // dev-cmake
	"dockerfile":         docker,
	"containerfile":      docker,
	"docker-compose.yml": docker,
	"compose.yaml":       docker,
	".gitignore":         git,
	".gitattributes":     git,
	".gitmodules":        git,
	".gitconfig":         git,
	".editorconfig":      {'\ue652', syntax.Comment}, // seti-editorconfig
	"package.json":       npm,
	".npmrc":             npm,
	"yarn.lock":          {'\ue6a7', syntax.Function}, // seti-yarn
	"go.mod":             {'\ue627', syntax.Function},
	"go.sum":             {'\ue627', syntax.Comment},
	"build.gradle":       {'\ue660', syntax.Function}, // seti-gradle
}

// byDirName gives some directories an icon of their own.
var byDirName = map[string]Icon{
	".git":         git,
	".github":      {'\uf07b', syntax.Comment},
	"node_modules": npm,
}

// For returns the icon for a file called name, given what the caller knows
// about it. name may be a path; only its last element is looked at.
func For(name string, kind Kind) Icon {
	base := strings.ToLower(filepath.Base(strings.TrimRight(name, `/\`)))
	switch kind {
	case Parent:
		return levelUp
	case Dir:
		if ic, ok := byDirName[base]; ok {
			return ic
		}
		if base == "." {
			return folderOpen
		}
		return folder
	case LinkDir:
		return linkDir
	case Link:
		return linkFile
	}
	if ic, ok := byName[base]; ok {
		return ic
	}
	switch {
	case strings.HasPrefix(base, "readme"):
		return book
	case strings.HasPrefix(base, "license"), strings.HasPrefix(base, "licence"),
		strings.HasPrefix(base, "copying"):
		return license
	case strings.HasPrefix(base, "dockerfile"):
		return docker
	case strings.HasPrefix(base, ".env"):
		return config
	}
	if ext := strings.TrimPrefix(filepath.Ext(base), "."); ext != "" {
		if ic, ok := byExt[ext]; ok {
			return ic
		}
	}
	if kind == Exec {
		return program
	}
	return plain
}

// ForCandidate is For for a completion candidate: a path, a directory when it
// ends in a separator, and "./" for the directory being listed.
func ForCandidate(cand string) Icon {
	if strings.HasSuffix(cand, "/") || strings.HasSuffix(cand, `\`) {
		return For(cand, Dir)
	}
	return For(cand, File)
}

// ForBuffer is the icon for a buffer's name: its file's, a folder for a
// directory listing (named with a trailing separator), and a note for
// buffers like *scratch* that no file stands behind.
func ForBuffer(name string) Icon {
	if strings.HasPrefix(name, "*") && strings.HasSuffix(name, "*") {
		return special
	}
	// A second main.go is main.go<2>, which is still a Go file.
	if i := strings.LastIndexByte(name, '<'); i > 0 && strings.HasSuffix(name, ">") {
		name = name[:i]
	}
	return ForCandidate(name)
}

// All lists every distinct glyph the package can return, for tests that check
// them against a font.
func All() []rune {
	seen := map[rune]bool{}
	var out []rune
	add := func(ic Icon) {
		if !seen[ic.Glyph] {
			seen[ic.Glyph] = true
			out = append(out, ic.Glyph)
		}
	}
	for _, ic := range []Icon{folder, folderOpen, levelUp, linkFile, linkDir, program, plain, textFile, special, book, license, config, docker} {
		add(ic)
	}
	for _, m := range []map[string]Icon{byExt, byName, byDirName} {
		for _, ic := range m {
			add(ic)
		}
	}
	return out
}
