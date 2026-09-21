package editor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajianpour/nem/text"
)

// repoWith builds a directory holding a .git directory whose HEAD is head, and
// returns a file path inside it.
func repoWith(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o600); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return file
}

func TestGitBranchReadsHEAD(t *testing.T) {
	for _, tc := range []struct {
		name string
		head string
		want string
	}{
		{"branch", "ref: refs/heads/main\n", "main"},
		{"no trailing newline", "ref: refs/heads/main", "main"},
		// path.Base would report this as "login", which is a different branch
		// and one that may well also exist.
		{"slashed branch", "ref: refs/heads/feature/login\n", "feature/login"},
		{"detached head", "9f2c1ab3d4e5f60718293a4b5c6d7e8f90123456\n", "9f2c1ab"},
		{"unknown ref", "ref: refs/tags/v1.0\n", "refs/tags/v1.0"},
		{"garbage", "not a ref at all\n", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gitBranch(repoWith(t, tc.head)); got != tc.want {
				t.Errorf("gitBranch = %q, want %q", got, tc.want)
			}
		})
	}
}

// A file outside any repository reports nothing rather than walking to the root
// and inventing an answer from some ancestor.
func TestGitBranchOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hi\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := gitBranch(file); got != "" {
		t.Errorf("gitBranch = %q, want empty outside a repository", got)
	}
}

// The repository root is usually several directories up from the file.
func TestGitBranchFoundFromASubdirectory(t *testing.T) {
	file := repoWith(t, "ref: refs/heads/trunk\n")
	root := filepath.Dir(file)
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	nested := filepath.Join(deep, "x.go")
	if err := os.WriteFile(nested, []byte("package x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := gitBranch(nested); got != "trunk" {
		t.Errorf("gitBranch = %q, want %q", got, "trunk")
	}
}

// In a linked worktree .git is a file pointing at the real directory. Joining
// ".git" and reading HEAD from it would find nothing here.
func TestGitBranchThroughAWorktreeFile(t *testing.T) {
	for _, rel := range []bool{false, true} {
		name := "absolute gitdir"
		if rel {
			name = "relative gitdir"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			realGit := filepath.Join(root, "store", "worktrees", "wt")
			if err := os.MkdirAll(realGit, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(realGit, "HEAD"),
				[]byte("ref: refs/heads/side\n"), 0o600); err != nil {
				t.Fatalf("write HEAD: %v", err)
			}

			work := filepath.Join(root, "work")
			if err := os.MkdirAll(work, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			target := realGit
			if rel {
				// A relative gitdir is relative to the .git file, not to the
				// process's working directory.
				target = filepath.Join("..", "store", "worktrees", "wt")
			}
			if err := os.WriteFile(filepath.Join(work, ".git"),
				[]byte("gitdir: "+target+"\n"), 0o600); err != nil {
				t.Fatalf("write .git: %v", err)
			}
			file := filepath.Join(work, "main.go")
			if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}

			if got := gitBranch(file); got != "side" {
				t.Errorf("gitBranch = %q, want %q", got, "side")
			}
		})
	}
}

// A .git file that says nothing useful must report no branch, not crash and not
// keep walking up into an unrelated parent repository.
func TestGitBranchWithABrokenGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("nonsense\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := gitBranch(file); got != "" {
		t.Errorf("gitBranch = %q, want empty", got)
	}
}

// HEAD is one short line. A pathological file in a cloned repository must not
// be read into memory on the way to drawing a modeline.
func TestHeadReadIsBounded(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	huge := make([]byte, headLimit*4)
	for i := range huge {
		huge[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), huge, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readLimited(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		t.Fatalf("readLimited: %v", err)
	}
	if len(got) > headLimit {
		t.Errorf("read %d bytes, want at most %d", len(got), headLimit)
	}
}

// The branch is consulted for every window on every frame, so a reading is
// reused; but a checkout in another terminal must show up, so it expires.
func TestBranchIsCachedAndExpires(t *testing.T) {
	e, _ := newTestEditor(t)
	file := repoWith(t, "ref: refs/heads/one\n")
	b, err := e.OpenFile(file)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	start := time.Now()
	if got := e.branchAt(b, start); got != "one" {
		t.Fatalf("branch = %q, want %q", got, "one")
	}

	// Check something else out underneath the editor.
	head := filepath.Join(filepath.Dir(file), ".git", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/two\n"), 0o600); err != nil {
		t.Fatalf("rewrite HEAD: %v", err)
	}

	if got := e.branchAt(b, start.Add(branchTTL/2)); got != "one" {
		t.Errorf("branch = %q within the TTL, want the cached %q", got, "one")
	}
	if got := e.branchAt(b, start.Add(branchTTL*2)); got != "two" {
		t.Errorf("branch = %q after the TTL, want the re-read %q", got, "two")
	}
}

// A path-less buffer has no file and so no repository, and must not cost a walk
// up the tree from wherever nem happens to have been started.
func TestBranchOfPathlessBuffer(t *testing.T) {
	e, _ := newTestEditor(t)
	if got := e.BranchOf(e.Buf()); got != "" {
		t.Errorf("branch = %q for a path-less buffer, want empty", got)
	}
	if _, cached := e.vcs[e.Buf()]; cached {
		t.Error("a path-less buffer should not occupy a cache entry")
	}
}

func TestBranchOfNilBuffer(t *testing.T) {
	e, _ := newTestEditor(t)
	if got := e.BranchOf(nil); got != "" {
		t.Errorf("branch = %q for a nil buffer, want empty", got)
	}
}

// Killing a buffer must drop its entry, or the map outlives its keys for the
// life of the session.
func TestKillingABufferForgetsItsBranch(t *testing.T) {
	e, _ := newTestEditor(t)
	file := repoWith(t, "ref: refs/heads/main\n")
	b, err := e.OpenFile(file)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if got := e.BranchOf(b); got != "main" {
		t.Fatalf("branch = %q, want %q", got, "main")
	}
	if err := e.KillBuffer(b); err != nil {
		t.Fatalf("KillBuffer: %v", err)
	}
	if _, cached := e.vcs[b]; cached {
		t.Error("killed buffer still holds a branch cache entry")
	}
}

// The file type must come from the same lexer that colours the text, or the
// modeline can claim "go" over text nothing highlighted.
func TestFileTypeMatchesTheLexer(t *testing.T) {
	e, _ := newTestEditor(t)
	for _, tc := range []struct{ path, want string }{
		{"/tmp/main.go", "go"},
		{"/tmp/init.lua", "lua"},
		{"/tmp/data.json", "json"},
		{"/tmp/README.md", "md"},
		{"/tmp/notes.txt", ""},
		{"", ""},
	} {
		b := text.NewBuffer()
		b.SetPath(tc.path)
		if got := e.FileType(b); got != tc.want {
			t.Errorf("FileType(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestFileTypeOfNilBuffer(t *testing.T) {
	e, _ := newTestEditor(t)
	if got := e.FileType(nil); got != "" {
		t.Errorf("FileType(nil) = %q, want empty", got)
	}
}
