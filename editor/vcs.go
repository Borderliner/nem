package editor

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Borderliner/nem/text"
)

// The modeline shows which branch a file sits on, which means reading .git.
//
// Nothing here shells out to git. Spawning a process to learn a fact that is
// one line of one file would cost milliseconds per frame, make the editor
// depend on a binary being installed, and put a fork in the draw path. The two
// files involved - .git/HEAD and, for a worktree, the .git file pointing at the
// real directory - are both small and both plain text.

// branchTTL is how long a branch reading is trusted.
//
// The branch is consulted for every window on every frame, so it cannot be read
// each time; but it changes underneath the editor whenever the user checks
// something out in another terminal, so it cannot be read once either. Two
// seconds is short enough that a checkout shows up as fast as anyone notices
// and long enough that a burst of typing costs one stat, not hundreds.
//
// Invalidating on save instead would be cheaper and wrong: a checkout happens
// without nem being touched at all.
const branchTTL = 2 * time.Second

// headLimit caps how much of a HEAD or .git file is read.
//
// Both are a single short line. A limit means a corrupt or hostile file in a
// repository someone cloned cannot make the editor read a gigabyte into memory
// on the way to drawing a modeline.
const headLimit = 4 << 10

// branchEntry is one cached reading and when it was taken.
type branchEntry struct {
	branch string
	at     time.Time
}

// BranchOf reports the git branch b's file sits on, or empty when it is not in
// a repository. It is the BranchFunc handed to the renderer.
func (e *Editor) BranchOf(b *text.Buffer) string {
	return e.branchAt(b, time.Now())
}

// branchAt is BranchOf with the clock injected, so cache expiry is testable
// without sleeping.
func (e *Editor) branchAt(b *text.Buffer, now time.Time) string {
	if b == nil {
		return ""
	}
	dir := e.bufferDir(b)
	if dir == "" {
		return "" // a path-less buffer has no file and so no repository
	}
	if ent, ok := e.vcs[b]; ok && now.Sub(ent.at) < branchTTL {
		return ent.branch
	}
	// The miss is cached too, including the empty answer. A file outside any
	// repository would otherwise walk to the filesystem root on every frame.
	br := gitBranchIn(dir)
	e.vcs[b] = branchEntry{branch: br, at: now}
	return br
}

// forgetBranch drops a buffer's cached branch.
//
// Called when a buffer is killed, so the map does not outlive its keys, and
// when a buffer adopts a new path, where the old reading describes a different
// file and possibly a different repository.
func (e *Editor) forgetBranch(b *text.Buffer) { delete(e.vcs, b) }

// gitBranch reports the branch checked out in the repository containing path.
func gitBranch(path string) string { return gitBranchIn(filepath.Dir(path)) }

// gitBranchIn reports the branch checked out in the repository containing dir.
func gitBranchIn(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	gitDir, ok := findGitDir(dir)
	if !ok {
		return ""
	}
	return headBranch(gitDir)
}

// findGitDir walks up from start looking for .git, returning the directory that
// actually holds HEAD.
//
// .git is usually a directory. In a linked worktree or a submodule it is a file
// containing "gitdir: <path>" pointing at the real one, which is why this
// cannot simply join ".git" and read from there.
func findGitDir(start string) (string, bool) {
	for dir := start; ; {
		candidate := filepath.Join(dir, ".git")
		switch fi, err := os.Lstat(candidate); {
		case err != nil:
			// Not here. Keep walking up; a permission error on one directory
			// should not stop the search any more than an absence does.
		case fi.IsDir():
			return candidate, true
		default:
			return gitDirFromFile(candidate)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false // reached the filesystem root
		}
		dir = parent
	}
}

// gitDirFromFile resolves a .git file to the directory it points at.
func gitDirFromFile(path string) (string, bool) {
	line, err := readLimited(path)
	if err != nil {
		return "", false
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return "", false
	}
	target := strings.TrimSpace(rest)
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		// A relative gitdir is relative to the file that names it, not to the
		// process's working directory - which is wherever nem happened to be
		// started from and has nothing to do with the repository.
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target), true
}

// headBranch reads gitDir/HEAD and names what it points at.
func headBranch(gitDir string) string {
	line, err := readLimited(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	line = strings.TrimSpace(line)

	if rest, ok := strings.CutPrefix(line, "ref:"); ok {
		ref := strings.TrimSpace(rest)
		// Strip the prefix rather than taking the base name: a branch called
		// feature/login is "feature/login", and path.Base would report it as
		// "login" - a different branch, possibly one that also exists.
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return name
		}
		return ref // some other ref; show whatever it is rather than nothing
	}

	// Detached HEAD holds a raw object id. Show it the way git does.
	if isObjectID(line) {
		return line[:7]
	}
	return ""
}

// isObjectID reports whether s looks like a git object id: hex, and long enough
// that the short form is meaningful.
func isObjectID(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// readLimited reads at most headLimit bytes of path.
func readLimited(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, headLimit))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FileType reports b's file type for the modeline: "go", "lua", "json", "md",
// or empty for text nem has no grammar for.
//
// It asks the highlight cache rather than looking at the extension itself, so
// the segment and the colouring cannot disagree - a modeline claiming "go" over
// uncoloured text would read as the highlighter having failed.
func (e *Editor) FileType(b *text.Buffer) string {
	if b == nil {
		return ""
	}
	if st := e.diredOf(b); st != nil {
		if st.wd != nil {
			return "wdired"
		}
		return "dired"
	}
	if e.grepOf(b) != nil {
		return "grep"
	}
	switch name := e.cacheFor(b).Lexer().Name(); name {
	case "text":
		return "" // no grammar, so nothing worth a segment
	case "markdown":
		return "md" // the modeline is tight; the extension is what people read
	default:
		return name
	}
}
