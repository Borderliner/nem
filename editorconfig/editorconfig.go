// Package editorconfig reads .editorconfig files: for a file, what the
// .editorconfig files in its directory and the directories above it say
// about it.
//
// It follows the EditorConfig specification (https://spec.editorconfig.org).
// The files are gathered from the file's own directory upward until one says
// root = true, or the filesystem runs out. A nearer file is applied after a
// farther one, so it wins; within a file, a later section wins over an
// earlier one; and a value of "unset" takes a property back out. Only the
// properties nem acts on are interpreted. The rest are read and ignored, which
// is what the specification asks of an editor that does not support them.
//
// It depends on nothing but the standard library and holds no editor state,
// so it is tested against directories on disk, as the project package is.
package editorconfig

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FileName is the name of the file this package reads.
const FileName = ".editorconfig"

// Bool is a property that can be true, false, or not said at all.
//
// Not said is not false, and the difference matters: insert_final_newline =
// false takes a file's last newline away, while a file no .editorconfig
// mentions is saved as it is.
type Bool int8

// The three values of a Bool.
const (
	Unset Bool = iota
	True
	False
)

// Props is what the .editorconfig files say about one file. A zero field is
// one no file mentions.
type Props struct {
	// IndentStyle is "tab" or "space", or "" when nothing says.
	IndentStyle string

	// IndentSize is how many columns one level of indentation takes, and
	// TabWidth how many a tab stands for; 0 when nothing says. Each falls
	// back on the other as the specification lays out: tab_width defaults to
	// indent_size, and indent_size = tab - or no indent_size at all in a file
	// indented with tabs - means the tab width.
	IndentSize, TabWidth int

	// TrimTrailingWhitespace asks for the spaces and tabs at the ends of
	// lines to be removed when the file is saved.
	TrimTrailingWhitespace Bool

	// InsertFinalNewline asks for the saved file to end with a newline, or,
	// when False, not to.
	InsertFinalNewline Bool
}

// file is one .editorconfig, parsed.
type file struct {
	root     bool
	sections []section
}

// section is one [glob] and the properties under it, in the order written.
type section struct {
	glob  *glob
	props []pair
}

// pair is one key = value line, both lower-cased.
type pair struct{ key, value string }

// bom is the byte-order mark some Windows editors put at the start of a file.
const bom = "\xef\xbb\xbf"

// parse reads an .editorconfig's text. It never fails: a line that is none of
// the kinds the specification defines is passed over, as is a section whose
// glob cannot be compiled, so one mistake costs one line rather than the file.
func parse(data []byte) *file {
	f := &file{}
	cur := -1 // the section being filled; -1 in the preamble
	text := strings.TrimPrefix(string(data), bom)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line) // takes a CRLF's CR with it
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
			continue
		case line[0] == '[' && line[len(line)-1] == ']':
			f.sections = append(f.sections, section{glob: compileGlob(line[1 : len(line)-1])})
			cur = len(f.sections) - 1
		default:
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.ToLower(strings.TrimSpace(v))
			if cur < 0 {
				// Before the first section only root means anything.
				if k == "root" {
					f.root = v == "true"
				}
				continue
			}
			f.sections[cur].props = append(f.sections[cur].props, pair{k, v})
		}
	}
	return f
}

// props turns the properties that apply to a file into Props, filling in the
// defaults the specification gives each in terms of the others.
func props(m map[string]string) Props {
	var p Props
	if s := m["indent_style"]; s == "tab" || s == "space" {
		p.IndentStyle = s
	}
	tab := positive(m["tab_width"])
	size := positive(m["indent_size"])
	if tab == 0 {
		tab = size
	}
	if size == 0 && (m["indent_size"] == "tab" || (m["indent_size"] == "" && p.IndentStyle == "tab")) {
		size = tab
	}
	p.IndentSize, p.TabWidth = size, tab
	p.TrimTrailingWhitespace = boolean(m["trim_trailing_whitespace"])
	p.InsertFinalNewline = boolean(m["insert_final_newline"])
	return p
}

// positive is s as a whole number above zero, or 0 when it is not one.
func positive(s string) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return 0
}

func boolean(s string) Bool {
	switch s {
	case "true":
		return True
	case "false":
		return False
	}
	return Unset
}

// Cache remembers the .editorconfig files it has read, so a lookup costs a
// stat of each directory's .editorconfig rather than a read and a parse of
// it. Pressing TAB asks which way to indent; it should not reread the
// project's configuration to find out. A file whose modification time or size
// has changed since is read again, so an edit to one applies at once.
//
// A Cache is safe for concurrent use.
type Cache struct {
	mu    sync.Mutex
	files map[string]cached
	reads int // how many files have been read, for the tests
}

// cached is a parsed .editorconfig and what it looked like on disk when read.
type cached struct {
	mod  time.Time
	size int64
	f    *file
}

// NewCache returns an empty cache.
func NewCache() *Cache { return &Cache{files: map[string]cached{}} }

// shared is the cache Lookup uses.
var shared = NewCache()

// Lookup is what the .editorconfig files say about the file at path, read
// through a cache shared by the whole program.
func Lookup(path string) Props { return shared.Lookup(path) }

// Lookup is what the .editorconfig files say about the file at path. The file
// itself need not exist - a buffer not yet saved is configured by where it
// will go.
//
// An .editorconfig that cannot be read is treated as absent rather than as
// an error: it is advice about whitespace, and there is nothing better to do
// without it than what would be done if it were not there.
func (c *Cache) Lookup(path string) Props {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Props{}
	}
	type found struct {
		dir string
		f   *file
	}
	var chain []found // nearest first
	for dir := filepath.Dir(abs); ; {
		if f := c.load(filepath.Join(dir, FileName)); f != nil {
			chain = append(chain, found{dir, f})
			if f.root {
				break
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	m := map[string]string{}
	for i := len(chain) - 1; i >= 0; i-- {
		rel, err := filepath.Rel(chain[i].dir, abs)
		if err != nil {
			continue
		}
		rel = "/" + filepath.ToSlash(rel)
		for _, s := range chain[i].f.sections {
			if !s.glob.match(rel) {
				continue
			}
			for _, kv := range s.props {
				if kv.value == "unset" {
					delete(m, kv.key)
				} else {
					m[kv.key] = kv.value
				}
			}
		}
	}
	return props(m)
}

// load returns the parsed .editorconfig at path, from the cache while the file
// is unchanged, or nil when there is none.
func (c *Cache) load(path string) *file {
	fi, err := os.Stat(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil || !fi.Mode().IsRegular() {
		delete(c.files, path)
		return nil
	}
	if hit, ok := c.files[path]; ok && hit.mod.Equal(fi.ModTime()) && hit.size == fi.Size() {
		return hit.f
	}
	data, err := os.ReadFile(path)
	if err != nil {
		delete(c.files, path)
		return nil
	}
	c.reads++
	f := parse(data)
	c.files[path] = cached{mod: fi.ModTime(), size: fi.Size(), f: f}
	return f
}
