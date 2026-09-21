package nanorc

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Borderliner/nem/syntax"
)

// DefaultDirs are searched in order, with later directories winning.
//
// The user's own directory comes last so a hand-written definition replaces the
// system one for the same language rather than competing with it.
func DefaultDirs() []string {
	dirs := []string{"/usr/share/nano", "/usr/share/nano/extra"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "nem", "syntax"))
	}
	return dirs
}

// Set is the loaded collection of language definitions.
//
// The zero Set matches nothing, which is what a machine without nano installed
// gets - and is why every caller can use a Set without checking for nil.
type Set struct {
	byName map[string]*Syntax
	order  []string // deterministic iteration, so matching never depends on map order
}

// Load reads every .nanorc in the given directories.
//
// A missing directory is not an error: most machines will have some of these
// and not others. Problems with individual files and individual patterns are
// returned rather than raised, so a caller can report them without one bad file
// costing the user every other language.
func Load(dirs ...string) (*Set, []error) {
	set := &Set{byName: map[string]*Syntax{}}
	var problems []error

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // absent or unreadable: normal, not worth reporting
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".nanorc") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names) // deterministic across filesystems
		for _, name := range names {
			path := filepath.Join(dir, name)
			f, err := os.Open(path)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			s, skips, err := Parse(path, f)
			f.Close()
			problems = append(problems, skips...)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			if _, seen := set.byName[s.Name]; !seen {
				set.order = append(set.order, s.Name)
			}
			set.byName[s.Name] = s // a later directory replaces an earlier one
		}
	}
	return set, problems
}

// Len reports how many languages loaded.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byName)
}

// Names lists the loaded languages, sorted.
func (s *Set) Names() []string {
	if s == nil {
		return nil
	}
	out := append([]string(nil), s.order...)
	sort.Strings(out)
	return out
}

// For returns a lexer for a file, or nil if no definition matches.
//
// firstLine is matched against header patterns, which is how a script with no
// extension gets highlighted from its shebang. Pass "" when it is not known;
// extension matching still works.
//
// It returns nil rather than a plain lexer so a caller can tell "no nanorc
// covers this" from "this is plain text", and keep its own lexers in front.
func (s *Set) For(path, firstLine string) syntax.Lexer {
	if s == nil || len(s.byName) == 0 {
		return nil
	}
	base := filepath.Base(path)
	for _, name := range s.order {
		syn := s.byName[name]
		for _, re := range syn.files {
			// nano matches the file regex against the name, and its patterns
			// are written as suffix anchors like "\.py$".
			if re.MatchString(base) || re.MatchString(path) {
				return Lexer{s: syn}
			}
		}
	}
	if firstLine != "" {
		for _, name := range s.order {
			syn := s.byName[name]
			for _, re := range syn.headers {
				if re.MatchString(firstLine) {
					return Lexer{s: syn}
				}
			}
		}
	}
	return nil
}
