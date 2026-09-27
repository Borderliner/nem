package project

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// MaxFiles is where listing a project stops. A tree that large is a home
// directory or a disk given a Marker, not a project, and listing it all
// would hold the editor up for as long as the disk takes.
const MaxFiles = 200_000

// ErrTooManyFiles says a listing stopped at MaxFiles. The files it does return
// are still worth offering.
var ErrTooManyFiles = errors.New("the project has too many files; the list stops at 200000")

// skipDirs are never walked into when a project is listed without git: a
// repository's own store, editors' and tools' caches, and the dependency trees
// that dwarf the code beside them. git leaves them out of its list already, by
// .gitignore.
var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".jj": true, ".bzr": true, "_darcs": true, ".pijul": true,
	".sl": true, ".svn": true, ".idea": true, ".vscode": true, ".cache": true,
	".stack-work": true, ".tox": true, ".venv": true, "__pycache__": true,
	"node_modules": true, ".ccls-cache": true, ".clangd": true, ".direnv": true,
	".eunit": true, "_build": true, ".zig-cache": true, "zig-cache": true,
}

// vcsDirs are the version control systems' own stores, left out even of a
// search told to ignore nothing.
var vcsDirs = map[string]bool{".git": true, ".hg": true, ".jj": true, ".bzr": true, "_darcs": true, ".pijul": true, ".sl": true, ".svn": true}

// Files lists the files of the project at root, relative to it and sorted,
// with "/" between the parts of a path whatever the OS.
//
// In a git repository git says what the files are - tracked, and new ones not
// ignored - so .gitignore is honoured and listing is as quick as git is.
// Elsewhere, or without git, the tree is walked, passing over the directories
// in skipDirs.
//
// Either way a Marker at root can narrow the list, as projectile's does. Each
// line is a pattern: one starting with + keeps only the directory it names,
// one starting with - or with neither leaves out what it matches, and # starts
// a comment. A pattern starting with / is matched from root; one without is
// matched against every part of a path, so "*.log" leaves out every log and
// "tmp" every directory called tmp.
func Files(root string) ([]string, error) {
	return FilesWith(root, false)
}

// FilesWith is Files, and with noIgnore - a search's -u - the files
// .gitignore leaves out as well, and the directories a walk passes over.
// Only a repository's own store is still left out.
func FilesWith(root string, noIgnore bool) ([]string, error) {
	var files []string
	var err error
	if inRepository(root) {
		files, err = gitFiles(root, noIgnore)
	}
	if files == nil {
		files, err = walkFiles(root, noIgnore)
	}
	if r := readRules(root); r != nil {
		files = slices.DeleteFunc(files, r.excludes)
	}
	return files, err
}

// gitFiles asks git for the project's files, and nil if git cannot answer -
// it is not installed, or root is a broken repository.
//
// Three asks, because one cannot do it all: submodules' files come only with
// the tracked list, new files only with the untracked one, and a file deleted
// but not yet committed is still in the tracked list until it is asked for by
// itself and taken out.
func gitFiles(root string, noIgnore bool) ([]string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	run := func(args ...string) ([]string, error) {
		cmd := exec.Command("git", append([]string{"-C", root, "ls-files", "-z"}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var names []string
		for _, n := range bytes.Split(out, []byte{0}) {
			if len(n) > 0 {
				names = append(names, string(n))
			}
		}
		return names, nil
	}
	// The three at once: each is a process, and in a large repository each
	// takes a good part of a second.
	var tracked, untracked, deleted []string
	var err error
	var wg sync.WaitGroup
	wg.Go(func() { tracked, err = run("--cached", "--recurse-submodules") })
	wg.Go(func() {
		if noIgnore {
			untracked, _ = run("--others")
		} else {
			untracked, _ = run("--others", "--exclude-standard")
		}
	})
	wg.Go(func() { deleted, _ = run("--deleted") })
	wg.Wait()
	if err != nil {
		return nil, nil
	}

	gone := make(map[string]bool, len(deleted))
	for _, d := range deleted {
		gone[d] = true
	}
	files := make([]string, 0, len(tracked)+len(untracked))
	for _, f := range append(tracked, untracked...) {
		if !gone[f] {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	files = slices.Compact(files) // a file mid-merge is listed once per side
	if len(files) > MaxFiles {
		return files[:MaxFiles], ErrTooManyFiles
	}
	return files, nil
}

// inRepository reports whether dir is in a git repository: it or a directory
// above it has a .git. git lists the files of a directory inside one from
// there, as paths from there, as it lists a repository's from its top.
func inRepository(dir string) bool {
	for {
		if exists(filepath.Join(dir, ".git")) {
			return true
		}
		up := filepath.Dir(dir)
		if up == dir {
			return false
		}
		dir = up
	}
}

// walkFiles lists the files under root by walking the tree, sorted.
//
// Directories are read in parallel, a goroutine each, as many reading at
// once as there are processors to spare: a large tree is mostly waiting on
// the disk, one directory after another, and a home directory's 200,000
// files took a second read in turn. A tree past MaxFiles is cut short where
// the walk had got to, which, walked in parallel, is not always the same
// place.
func walkFiles(root string, noIgnore bool) ([]string, error) {
	var (
		mu    sync.Mutex
		files []string
		count atomic.Int64
		wg    sync.WaitGroup
		// rootErr is the error reading root itself: an unreadable corner
		// below it is left out, not the whole list.
		rootErr error
	)
	reading := make(chan struct{}, 2*runtime.GOMAXPROCS(0))
	var walk func(dir, rel string)
	walk = func(dir, rel string) {
		defer wg.Done()
		if count.Load() >= MaxFiles {
			return
		}
		reading <- struct{}{}
		ents, err := os.ReadDir(dir)
		<-reading
		if err != nil {
			if rel == "" {
				rootErr = err
			}
			return
		}
		var here []string
		for _, d := range ents {
			name := d.Name()
			p := name
			if rel != "" {
				p = rel + "/" + name
			}
			if d.IsDir() {
				if skipDirs[name] && (!noIgnore || vcsDirs[name]) {
					continue
				}
				wg.Add(1)
				go walk(filepath.Join(dir, name), p)
				continue
			}
			here = append(here, p)
		}
		count.Add(int64(len(here)))
		mu.Lock()
		files = append(files, here...)
		mu.Unlock()
	}
	wg.Add(1)
	walk(root, "")
	wg.Wait()
	if rootErr != nil {
		return nil, rootErr
	}
	slices.Sort(files)
	if len(files) >= MaxFiles {
		return files[:MaxFiles], ErrTooManyFiles
	}
	return files, nil
}

// rules are a Marker file's patterns.
type rules struct {
	keep, drop []string
}

// readRules reads the Marker at root, or nil if there is none or it says
// nothing.
func readRules(root string) *rules {
	data, err := os.ReadFile(filepath.Join(root, Marker))
	if err != nil {
		return nil
	}
	r := &rules{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "+"):
			if p := cleanPattern(line[1:]); p != "" {
				r.keep = append(r.keep, strings.TrimPrefix(p, "/"))
			}
		case strings.HasPrefix(line, "-"):
			if p := cleanPattern(line[1:]); p != "" {
				r.drop = append(r.drop, p)
			}
		default:
			if p := cleanPattern(line); p != "" {
				r.drop = append(r.drop, p)
			}
		}
	}
	if len(r.keep) == 0 && len(r.drop) == 0 {
		return nil
	}
	return r
}

// cleanPattern trims a pattern and the / that marks it a directory, which
// matches the same way without it.
func cleanPattern(p string) string {
	p = strings.TrimSpace(p)
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// excludes reports whether the rules leave rel out of the project.
func (r *rules) excludes(rel string) bool {
	if len(r.keep) > 0 && !slices.ContainsFunc(r.keep, func(k string) bool { return underPattern(k, rel) }) {
		return true
	}
	for _, p := range r.drop {
		if anchored, ok := strings.CutPrefix(p, "/"); ok {
			if underPattern(anchored, rel) {
				return true
			}
			continue
		}
		if strings.Contains(p, "/") {
			if underPattern(p, rel) {
				return true
			}
			continue
		}
		for _, part := range strings.Split(rel, "/") {
			if ok, _ := path.Match(p, part); ok {
				return true
			}
		}
	}
	return false
}

// underPattern reports whether rel is what pattern names, a glob matched from
// the project's root, or lies inside it.
func underPattern(pattern, rel string) bool {
	parts := strings.Split(rel, "/")
	for i := 1; i <= len(parts); i++ {
		if ok, _ := path.Match(pattern, strings.Join(parts[:i], "/")); ok {
			return true
		}
	}
	return false
}

// Dirs lists the directories that hold files, each with a "/" after it: every
// directory on the way to one, not only those with files of their own, so a
// directory of directories can be found too.
func Dirs(files []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		for d := path.Dir(f); d != "." && !seen[d]; d = path.Dir(d) {
			seen[d] = true
			dirs = append(dirs, d+"/")
		}
	}
	slices.Sort(dirs)
	return dirs
}
