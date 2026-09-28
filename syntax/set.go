package syntax

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// languagesFS carries nem's own language definitions into the binary, so
// that colour works on a machine with nothing else installed. They are
// written from each language's grammar - a keyword list is a fact about a
// language - and are covered by nem's own licence.
//
//go:embed languages/*.syntax
var languagesFS embed.FS

// Set is the languages nem knows: its own, and the user's beside them or in
// their place.
type Set struct {
	// langs is in the order files are matched: the user's first, then nem's,
	// each by name.
	langs  []*Language
	byName map[string]*Language
}

var (
	builtinOnce sync.Once
	builtinSet  *Set
	builtinDefs map[string]*Def
	builtinErr  error
)

// loadBuiltin reads the embedded definitions, once. A failure is a bug in a
// file that ships in the binary; it is kept for BuiltinError, which a test
// asserts is nil, and the languages that did load are still offered.
func loadBuiltin() {
	builtinDefs = map[string]*Def{}
	var errs []error
	entries, err := languagesFS.ReadDir("languages")
	if err != nil {
		builtinErr = err
		builtinSet = &Set{byName: map[string]*Language{}}
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".syntax") {
			continue
		}
		// An embed.FS names files with forward slashes on every system.
		data, err := languagesFS.ReadFile(path.Join("languages", e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		d, err := ParseDef(e.Name(), strings.NewReader(string(data)))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if want := strings.TrimSuffix(e.Name(), ".syntax"); d.Name != want {
			errs = append(errs, fmt.Errorf("%s names its language %s; the file is named for it", e.Name(), d.Name))
			continue
		}
		builtinDefs[d.Name] = d
	}
	set, more := build(builtinDefs, nil)
	builtinSet = set
	builtinErr = errors.Join(append(errs, more...)...)
}

// BuiltinSource returns the text of nem's own definition of a language, as
// it is written: what a user starts from to change it.
func BuiltinSource(name string) (string, bool) {
	if !ValidName(name) {
		return "", false
	}
	data, err := languagesFS.ReadFile(path.Join("languages", name+".syntax"))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// ValidName reports whether name may name a language: lower-case letters,
// digits and + # . _ -.
func ValidName(name string) bool { return languageName.MatchString(name) }

// Builtin returns nem's own languages.
func Builtin() *Set {
	builtinOnce.Do(loadBuiltin)
	return builtinSet
}

// BuiltinError reports a problem with nem's own definitions, or nil.
func BuiltinError() error {
	builtinOnce.Do(loadBuiltin)
	return builtinErr
}

// Load returns nem's languages with the user's from dir: each *.syntax file
// there is a language, matched before nem's, and one named as one of nem's
// replaces it. A missing dir is no error; a bad file is reported and the
// rest still load.
func Load(dir string) (*Set, []error) {
	builtinOnce.Do(loadBuiltin)
	if dir == "" {
		return builtinSet, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return builtinSet, nil
		}
		return builtinSet, []error{err}
	}
	var errs []error
	user := map[string]*Def{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".syntax") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		f, err := os.Open(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// Named by the file alone: the directory is the one the user put it
		// in, and the whole path crowds the echo area out of the message.
		d, err := ParseDef(e.Name(), f)
		f.Close()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if prev, dup := user[d.Name]; dup {
			errs = append(errs, fmt.Errorf("%s: language %s is already defined in %s", e.Name(), d.Name, prev.Source))
			continue
		}
		user[d.Name] = d
	}
	set, more := build(builtinDefs, user)
	return set, append(errs, more...)
}

// build compiles the built-in definitions and the user's over them.
func build(builtin, user map[string]*Def) (*Set, []error) {
	var errs []error
	resolved := map[string]*Def{}
	resolving := map[string]bool{}

	// resolve merges a definition with what it is like. A definition like
	// its own name starts from nem's language of that name, which it
	// replaces; like another name, from the user's language of that name
	// if there is one, else nem's.
	var resolve func(d *Def, isUser bool) (*Def, error)
	resolve = func(d *Def, isUser bool) (*Def, error) {
		key := d.Name
		if !isUser {
			key = "\x00" + d.Name
		}
		if r, ok := resolved[key]; ok {
			return r, nil
		}
		if d.Like == "" {
			resolved[key] = d
			return d, nil
		}
		if resolving[key] {
			return nil, fmt.Errorf("%s: like %s goes round in a circle", d.Source, d.Like)
		}
		resolving[key] = true
		defer delete(resolving, key)
		var base *Def
		baseUser := false
		if u, ok := user[d.Like]; ok && isUser && d.Like != d.Name {
			base, baseUser = u, true
		} else if b, ok := builtin[d.Like]; ok {
			base = b
		} else {
			return nil, fmt.Errorf("%s: like %s, but there is no language %s", d.Source, d.Like, d.Like)
		}
		rb, err := resolve(base, baseUser)
		if err != nil {
			return nil, err
		}
		m := merge(rb, d)
		resolved[key] = m
		return m, nil
	}

	compile := func(defs map[string]*Def, isUser bool) []*Language {
		names := make([]string, 0, len(defs))
		for n := range defs {
			names = append(names, n)
		}
		sort.Strings(names)
		var out []*Language
		for _, n := range names {
			d, err := resolve(defs[n], isUser)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			l, err := Compile(d)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			out = append(out, l)
		}
		return out
	}

	s := &Set{byName: map[string]*Language{}}
	for _, l := range compile(user, true) {
		s.langs = append(s.langs, l)
		s.byName[l.name] = l
	}
	for _, l := range compile(builtin, false) {
		if _, replaced := s.byName[l.name]; replaced {
			continue
		}
		s.langs = append(s.langs, l)
		s.byName[l.name] = l
	}
	return s, errs
}

// Language returns a language by name, or nil.
func (s *Set) Language(name string) *Language { return s.byName[name] }

// Languages lists the set's languages, by name.
func (s *Set) Languages() []*Language {
	out := append([]*Language(nil), s.langs...)
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// Detect returns the language of a file, or nil: by its name, a whole name
// such as Makefile before any pattern, and then by its first line - the #!
// of a script with no extension, or a header such as <?xml.
func (s *Set) Detect(p, firstLine string) *Language {
	if s == nil {
		return nil
	}
	base := strings.ToLower(filepath.Base(p))
	if p != "" {
		for _, l := range s.langs {
			for _, g := range l.files {
				if !strings.ContainsAny(g, "*?[") && g == base {
					return l
				}
			}
		}
		for _, l := range s.langs {
			for _, g := range l.files {
				if ok, _ := path.Match(g, base); ok {
					return l
				}
			}
		}
	}
	if firstLine == "" {
		return nil
	}
	if interp := interpreter(firstLine); interp != "" {
		for _, l := range s.langs {
			if l.shebangs[interp] {
				return l
			}
		}
	}
	for _, l := range s.langs {
		for _, re := range l.headers {
			if re.MatchString(firstLine) {
				return l
			}
		}
	}
	return nil
}

// For returns the lexer for a file: its language, or the plain lexer, so a
// caller never has to check before lexing.
func (s *Set) For(p, firstLine string) Lexer {
	if l := s.Detect(p, firstLine); l != nil {
		return l
	}
	return PlainLexer{}
}

// interpreter reads the program a #! line runs, without its directory or a
// version at its end: python for #!/usr/bin/env python3.12, sh for #!/bin/sh.
// It returns "" for a line that is not a #!.
func interpreter(first string) string {
	rest, ok := strings.CutPrefix(first, "#!")
	if !ok {
		return ""
	}
	f := strings.Fields(rest)
	if len(f) == 0 {
		return ""
	}
	prog := path.Base(filepath.ToSlash(f[0]))
	if prog == "env" {
		prog = ""
		for _, a := range f[1:] {
			// env's own options, and the variables it is asked to set.
			if strings.HasPrefix(a, "-") || strings.Contains(a, "=") {
				continue
			}
			prog = path.Base(a)
			break
		}
	}
	return strings.TrimRight(prog, "0123456789.")
}

// For returns nem's lexer for a path, by file name. It never returns nil:
// a name nothing recognises gets the plain lexer.
func For(p string) Lexer { return Builtin().For(p, "") }

// ForWithHeader picks nem's lexer knowing the file's first line, which is
// what identifies a script with no extension by its #!.
func ForWithHeader(p, firstLine string) Lexer { return Builtin().For(p, firstLine) }
