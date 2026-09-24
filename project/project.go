// Package project finds the project a file belongs to and says what is in it:
// its files, the lines in them that match a search, and which file is another's
// test. It is the part of projectile that holds no editor state, so it can be
// tested against directories rather than through buffers, as the dired package
// is.
package project

import (
	"os"
	"path/filepath"
)

// Marker is the file that makes a directory a project when nothing else
// does, or a different project from the repository around it. It is
// projectile's, so a tree already set up for projectile works as it is, and it
// may list what to leave out of the project; see Files.
const Marker = ".projectile"

// vcsMarkers make the directory holding them a project: a repository is what
// people mean by one.
var vcsMarkers = []string{
	".git", ".hg", ".jj", ".bzr", "_darcs", ".pijul", ".sl", ".fslckout", "_FOSSIL_",
}

// buildMarkers make a directory a project when it is in no repository: a
// directory that builds is one.
var buildMarkers = []string{
	"go.mod", "Cargo.toml", "package.json", "deno.json", "pyproject.toml", "setup.py",
	"Gemfile", "composer.json", "mix.exs", "rebar.config", "pom.xml", "build.gradle",
	"build.gradle.kts", "build.sbt", "CMakeLists.txt", "meson.build", "xmake.lua",
	"Makefile", "build.zig", "dune-project", "stack.yaml", "Project.toml",
	"pubspec.yaml", "shard.yml", "flake.nix",
}

// Root is the project dir belongs to: the nearest directory, dir or one above
// it, holding a Marker or a repository; failing that, the nearest holding a
// build file. False means dir is in no project.
//
// Nearest rather than outermost, so a repository inside another - a
// submodule, a vendored checkout - is a project of its own, as is a directory
// given a Marker inside a monorepo.
//
// The home directory and the filesystem root are passed over unless they hold
// a Marker. A home directory kept in git for its dotfiles would otherwise make
// every file anywhere under it one project of a hundred thousand files.
func Root(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		home = filepath.Clean(home)
	}
	passedOver := func(d string) bool { return d == home || filepath.Dir(d) == d }

	for d := dir; ; d = filepath.Dir(d) {
		if exists(filepath.Join(d, Marker)) {
			return d, true
		}
		if !passedOver(d) && anyExists(d, vcsMarkers) {
			return d, true
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	for d := dir; ; d = filepath.Dir(d) {
		if !passedOver(d) && anyExists(d, buildMarkers) {
			return d, true
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return "", false
}

// Name is how a project is called in prompts and messages: its directory's
// name.
func Name(root string) string { return filepath.Base(root) }

// Contains reports whether path is root or lies beneath it.
func Contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !hasDotDotPrefix(rel))
}

func hasDotDotPrefix(rel string) bool {
	return len(rel) >= 3 && rel[:2] == ".." && os.IsPathSeparator(rel[2])
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func anyExists(dir string, names []string) bool {
	for _, n := range names {
		if exists(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}
