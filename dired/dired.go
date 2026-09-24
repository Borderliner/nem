// Package dired formats a directory listing for nem's dired mode: reading
// entries, ordering them, laying out the buffer text and colouring it, plus the
// file operations dired performs.
//
// It is pure in the sense that matters to the editor: it holds no editor state.
// Reading and formatting are separate steps so the editor can re-format the
// same entries when the user toggles hidden files, details or the sort order,
// without touching the disk again, and so the layout can be tested against
// fixed Entry values and a fixed clock rather than a real directory.
package dired

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Entry is one directory entry.
type Entry struct {
	// Name is the base name as it is on disk, unsanitised: it is what file
	// operations are given, so it must round-trip exactly.
	Name string
	// Mode comes from Lstat, so a symlink carries fs.ModeSymlink rather than
	// its target's type.
	Mode fs.FileMode
	// Size is the Lstat size; for a symlink that is the length of its text.
	Size    int64
	ModTime time.Time
	// Target is a symlink's link text (os.Readlink), and "" for anything else.
	Target string
	// Broken reports a symlink whose target does not exist.
	Broken bool
	// IsDir is true for a directory, and for a symlink that resolves to one:
	// both are things the user can descend into.
	IsDir bool

	// resolved is the mode of what a symlink points at (the entry's own mode
	// for anything else). Mode must stay the Lstat mode so the listing can say
	// "this is a link", but whether the link runs a program depends on the
	// target's execute bits.
	resolved fs.FileMode
}

// ParentName names the entry for the directory above, which Read lists so a
// listing can be left the same way it was entered: by RET on a directory.
const ParentName = ".."

// IsParent reports whether e is the entry for the directory above.
func (e Entry) IsParent() bool { return e.Name == ParentName }

// Hidden reports whether the name starts with a dot. The parent entry is not
// hidden: it is the way out, not a dotfile.
func (e Entry) Hidden() bool { return strings.HasPrefix(e.Name, ".") && !e.IsParent() }

// Executable reports whether the entry is a regular file, or a symlink to one,
// with any execute bit set. Directories are excluded: their x bit means
// "searchable", and colouring every directory as a program would be noise.
func (e Entry) Executable() bool {
	if e.IsDir {
		return false
	}
	m := e.Mode
	if m&fs.ModeSymlink != 0 {
		if e.Broken {
			return false
		}
		m = e.resolved
	}
	return m.IsRegular() && m&0o111 != 0
}

// unknown reports an entry whose Lstat failed, so nothing but its name is
// known.
//
// It is judged from the exported fields alone - zero mode and zero time -
// rather than from a hidden flag, so an Entry the editor builds itself means
// the same thing as one Read built. A real file with mode 000 still has a
// modification time, so it is not mistaken for one.
func (e Entry) unknown() bool { return e.Mode == 0 && e.ModTime.IsZero() }

// special reports a named pipe, socket or device: things that are neither
// data nor a place, and that opening can block on or disturb.
func (e Entry) special() bool {
	return e.Mode&(fs.ModeNamedPipe|fs.ModeSocket|fs.ModeDevice|fs.ModeCharDevice) != 0
}

// SortKey orders a listing. Directories always come before everything else,
// whatever the key, because the first thing a user scans a listing for is
// where they can go next.
type SortKey int

const (
	// ByName is case-insensitive and ignores one leading dot, so ".bashrc"
	// sorts among the b's rather than in a clump at the top. Ties are broken
	// by the raw name, which makes the order total.
	ByName SortKey = iota
	// ByTime puts the newest first; ties fall back to name order.
	ByTime
	// BySize puts the largest first; directories, grouped first anyway, are
	// ordered by name since their size says nothing about their contents.
	// Ties fall back to name order.
	BySize
)

// String names the key as the editor shows it in the mode line.
func (k SortKey) String() string {
	switch k {
	case ByName:
		return "name"
	case ByTime:
		return "time"
	case BySize:
		return "size"
	}
	return "SortKey(?)"
}

// Next cycles ByName -> ByTime -> BySize -> ByName, for a single key that
// steps through the orders.
func (k SortKey) Next() SortKey {
	switch k {
	case ByName:
		return ByTime
	case ByTime:
		return BySize
	}
	return ByName
}

// Options controls what a listing shows. The zero value is the default:
// details shown, hidden entries omitted, sorted by name, no icons.
type Options struct {
	ShowHidden  bool
	HideDetails bool
	Sort        SortKey
	// Icons puts a Nerd Font glyph before each name. Off in the zero value
	// because it needs a font this package cannot know the terminal has.
	Icons bool
}

// Read lists every entry of dir (not recursive), hidden ones included; Format
// does the filtering and ordering, so toggling either needs no second read.
// Unless dir is a filesystem root, the list also holds a ParentName entry for
// the directory above.
//
// An entry whose Lstat fails - it vanished between readdir and lstat, or the
// directory is readable but not searchable - is still listed with its name and
// zero Mode, Size and ModTime. Dropping it would hide from the user a file
// they can see with ls. The error is only for failing to read the directory
// itself; entries read before such a failure are returned alongside it.
func Read(dir string) ([]Entry, error) {
	names, err := readNames(dir)
	entries := make([]Entry, 0, len(names)+1)
	if p, ok := parentEntry(dir); ok {
		entries = append(entries, p)
	}
	for _, name := range names {
		entries = append(entries, readEntry(filepath.Join(dir, name), name))
	}
	return entries, err
}

// readNames lists a directory's names without statting anything. Read stats
// each name itself with Lstat, and os.ReadDir would sort names only for Format
// to sort them again.
func readNames(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

// parentEntry describes the directory above dir, or reports false at a root.
//
// The parent is the lexical one - dir with its last component removed - which
// is where dired goes on RET or ^, so a directory reached through a symlink is
// left back the way it was entered. It is Stat'ed rather than Lstat'ed: that
// lexical parent may itself be a link, and ".." is a place to go, not a link
// to show a target for.
func parentEntry(dir string) (Entry, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Entry{}, false
	}
	up := filepath.Dir(abs)
	if up == abs {
		return Entry{}, false
	}
	e := Entry{Name: ParentName, IsDir: true}
	if info, err := os.Stat(up); err == nil {
		e.Mode, e.ModTime = info.Mode(), info.ModTime()
		e.resolved = e.Mode
	}
	return e, true
}

func readEntry(path, name string) Entry {
	e := Entry{Name: name}
	info, err := os.Lstat(path)
	if err != nil {
		return e
	}
	e.Mode, e.Size, e.ModTime = info.Mode(), info.Size(), info.ModTime()
	e.resolved = e.Mode
	e.IsDir = info.IsDir()
	if e.Mode&fs.ModeSymlink == 0 {
		return e
	}
	e.Target, _ = os.Readlink(path)
	st, err := os.Stat(path)
	if err != nil {
		e.Broken = true
		e.resolved = 0
		return e
	}
	e.resolved = st.Mode()
	e.IsDir = st.IsDir()
	return e
}

// sortEntries orders entries in place: directories first, then by key.
func sortEntries(entries []Entry, key SortKey) {
	// The folded name is computed once per entry rather than inside the
	// comparison, which would allocate twice per comparison - n log n
	// allocations on a large directory, on every re-sort.
	type keyed struct {
		folded string
		e      Entry
	}
	ks := make([]keyed, len(entries))
	for i, e := range entries {
		ks[i] = keyed{strings.ToLower(strings.TrimPrefix(e.Name, ".")), e}
	}
	byName := func(a, b keyed) int {
		if c := strings.Compare(a.folded, b.folded); c != 0 {
			return c
		}
		return strings.Compare(a.e.Name, b.e.Name)
	}
	slices.SortFunc(ks, func(a, b keyed) int {
		// The way out is always at the top, whatever the order below it.
		if a.e.IsParent() != b.e.IsParent() {
			if a.e.IsParent() {
				return -1
			}
			return 1
		}
		if a.e.IsDir != b.e.IsDir {
			if a.e.IsDir {
				return -1
			}
			return 1
		}
		switch key {
		case ByTime:
			if c := b.e.ModTime.Compare(a.e.ModTime); c != 0 {
				return c
			}
		case BySize:
			if !a.e.IsDir {
				if c := cmp.Compare(b.e.Size, a.e.Size); c != 0 {
					return c
				}
			}
		}
		return byName(a, b)
	})
	for i := range ks {
		entries[i] = ks[i].e
	}
}
