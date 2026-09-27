package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Kinds of file Keep writes, each in a directory of its own under the root.
const (
	// Rescued holds the text of buffers with no file, written when nem was
	// ending without being able to ask about them: *scratch* typed into, when
	// the terminal closed.
	Rescued = "rescued"
	// Faults holds reports of nem's own bugs: what went wrong, and where.
	Faults = "faults"
)

// Keep writes content to a new file called name in the store's directory for
// kind, and returns its path. A name already taken gets a number after it:
// nothing kept is ever written over. Characters that could not be part of a
// file name, or could climb out of the directory, are replaced.
func (s *Store) Keep(kind, name string, content []byte) (string, error) {
	if kind != Rescued && kind != Faults {
		return "", fmt.Errorf("backup: no kind of kept file called %q", kind)
	}
	base := keepName(name)
	dir := filepath.Join(s.root, kind)
	for n := 1; n < 1000; n++ {
		p := filepath.Join(dir, base)
		if n > 1 {
			ext := filepath.Ext(base)
			p = filepath.Join(dir, strings.TrimSuffix(base, ext)+"-"+strconv.Itoa(n)+ext)
		}
		if !s.contains(p) {
			return "", fmt.Errorf("%w: %s", ErrEscapesRoot, p)
		}
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return p, writeAtomic(p, content)
	}
	return "", fmt.Errorf("backup: no free name for %s in %s", base, dir)
}

// keepName makes name safe as a file name: letters, digits and a few marks
// kept, anything else - a separator, a colon, the stars of *scratch* - made
// an underscore, and nothing left that starts with a dot.
func keepName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r > 0x7f:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "buffer"
	}
	return out
}
