package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajianpour/nem/ui"
)

// The segments are worth nothing unless they reach the screen. Each half of
// this - the editor knowing the answer and the renderer asking for it - was
// individually correct the last time the modeline was wrong, which is why this
// asserts on rendered cells rather than on either half.
func TestModelineSegmentsReachTheScreen(t *testing.T) {
	e, scr := newTestEditor(t)
	file := repoWith(t, "ref: refs/heads/feature/login\n")

	b, err := e.OpenFile(file)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	e.Active().Visit(b)

	row := modelineRow(t, e, scr)
	for _, want := range []string{"main.go", "go", "feature/login", string(ui.BranchMark)} {
		if !strings.Contains(row, want) {
			t.Errorf("modeline = %q, want it to contain %q", row, want)
		}
	}
}

// A buffer nem has no grammar for contributes no file-type segment, and a file
// outside a repository contributes no branch.
func TestModelineSegmentsAbsentWhenThereIsNothingToSay(t *testing.T) {
	e, scr := newTestEditor(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	b, err := e.OpenFile(file)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	e.Active().Visit(b)

	row := modelineRow(t, e, scr)
	if !strings.Contains(row, "notes.txt") {
		t.Errorf("modeline = %q, want the buffer name", row)
	}
	if strings.ContainsRune(row, ui.BranchMark) {
		t.Errorf("modeline = %q, want no branch outside a repository", row)
	}
}

// Saving a scratch buffer as a .go file moves it onto a branch and gives it a
// grammar. Both segments must follow, or the modeline describes the file the
// buffer used to be.
func TestModelineSegmentsFollowAWriteFile(t *testing.T) {
	e, scr := newTestEditor(t, "package main")
	file := repoWith(t, "ref: refs/heads/trunk\n")
	target := filepath.Join(filepath.Dir(file), "written.go")

	if before := modelineRow(t, e, scr); strings.Contains(before, "go") &&
		strings.ContainsRune(before, ui.BranchMark) {
		t.Fatalf("setup: scratch modeline already shows segments: %q", before)
	}

	if err := e.SaveBuffer(e.Buf(), target); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	row := modelineRow(t, e, scr)
	for _, want := range []string{"written.go", "go", "trunk"} {
		if !strings.Contains(row, want) {
			t.Errorf("after write-file modeline = %q, want it to contain %q", row, want)
		}
	}
}

// write-file can move a buffer between repositories, and the branch it was on a
// moment ago describes the file it used to be.
//
// The scratch-buffer case above does not cover this: a path-less buffer never
// caches a branch in the first place, so there is nothing stale to drop and the
// test passes whether or not the cache is invalidated. This starts from a
// buffer that has a path and therefore a cached reading - which is the only
// shape in which forgetting can fail.
func TestWriteFileIntoAnotherRepositoryUpdatesTheBranch(t *testing.T) {
	e, scr := newTestEditor(t)

	outside := filepath.Join(t.TempDir(), "notes.go")
	if err := os.WriteFile(outside, []byte("package notes\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := e.OpenFile(outside)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	e.Active().Visit(b)

	// Cache the reading for the file where it is now: outside any repository.
	if got := e.BranchOf(b); got != "" {
		t.Fatalf("setup: branch = %q, want empty outside a repository", got)
	}

	into := repoWith(t, "ref: refs/heads/landing\n")
	target := filepath.Join(filepath.Dir(into), "moved.go")
	if err := e.SaveBuffer(b, target); err != nil {
		t.Fatalf("SaveBuffer: %v", err)
	}

	// Immediately, not once the TTL happens to lapse: the user saved into a
	// repository and the modeline should say so on the next frame.
	if got := e.BranchOf(b); got != "landing" {
		t.Errorf("branch = %q after write-file, want %q", got, "landing")
	}
	if row := modelineRow(t, e, scr); !strings.Contains(row, "landing") {
		t.Errorf("modeline = %q, want the new branch", row)
	}
}
