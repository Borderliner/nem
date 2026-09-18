package backup

import (
	"path/filepath"
	"strings"
	"testing"
)

// contains is the guard that keeps backups out of the user's tree. Mirroring
// makes it structurally impossible to reach its refusal through the public API,
// so the predicate is tested here directly: if the mirroring is ever changed,
// this is what still has to hold.
func TestContains(t *testing.T) {
	s := New("/state/nem")
	for _, tc := range []struct {
		p    string
		want bool
	}{
		{"/state/nem/backups/home/x.go~", true},
		{"/state/nem/a", true},
		{"/state/nem", false},           // the root itself is not a file location
		{"/state", false},               // above the root
		{"/state/nem/../escape", false}, // climbs out
		{"/etc/passwd", false},          // unrelated
		{"/state/nemesis/x", false},     // shares a name prefix but is a sibling
		{"/state/nem/..foo", true},      // a file whose name merely starts with dots
		{"/state/nem/a/../b", true},     // climbs, but not past the root
	} {
		if got := s.contains(tc.p); got != tc.want {
			t.Errorf("contains(%q) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

// A sibling directory sharing the root's name prefix must read as outside.
// A string-prefix check would call /state/nemesis contained; filepath.Rel is
// what makes this correct.
func TestContainsRejectsSiblingNamePrefix(t *testing.T) {
	if New("/state/nem").contains("/state/nemesis/backups/x~") {
		t.Error("a sibling directory sharing the root's name prefix read as contained")
	}
}

// guard's refusal branch is deliberately unreachable while mirroring is
// correct: mirror and contains derive from the same root, so they cannot
// disagree. It is a tripwire, not live logic — its value is that breaking the
// cleaning in mirror turns an escape into a refused write rather than a file
// written into the user's project. That is demonstrated by mutation rather than
// by a test here, since any test would have to corrupt the Store to reach it.
//
// What is tested directly is contains, above, which is the logic the tripwire
// depends on.

// The filesystem root leaves nothing to mirror, so the suffix alone names the
// file. Nobody edits "/", but the result must still land inside the store.
func TestMirrorOfFilesystemRoot(t *testing.T) {
	s := New("/state/nem")
	got := s.mirror(backupsDir, "/", backupSuffix)
	if want := "/state/nem/backups/~"; got != want {
		t.Errorf("mirror(/) = %q, want %q", got, want)
	}
	if !s.contains(got) {
		t.Errorf("mirror(/) = %q, which is not inside the root", got)
	}
}

// A relative path is resolved against the working directory, so it still lands
// under the root rather than beside the file.
func TestMirrorAbsolutisesRelativeInput(t *testing.T) {
	s := New("/state/nem")
	got := s.mirror(backupsDir, "rel/main.go", backupSuffix)
	if !strings.HasPrefix(got, "/state/nem/backups/") {
		t.Errorf("mirror(rel/main.go) = %q, want it under /state/nem/backups/", got)
	}
	if strings.Contains(got, "..") {
		t.Errorf("mirror produced %q, which still contains ..", got)
	}
}

// Cleaning is what removes "..", and it must happen before the join. Joining the
// raw input would let ../../etc/passwd climb out of the root.
func TestMirrorResolvesDotDotBeforeJoining(t *testing.T) {
	s := New("/state/nem")
	got := s.mirror(backupsDir, "/home/reza/../../etc/passwd", backupSuffix)
	if want := "/state/nem/backups/etc/passwd~"; got != want {
		t.Errorf("mirror = %q, want %q", got, want)
	}
	if !s.contains(got) {
		t.Fatalf("mirror = %q, which escapes the root", got)
	}
}

func TestRoot(t *testing.T) {
	if got, want := New("/state/nem/").Root(), filepath.Clean("/state/nem"); got != want {
		t.Errorf("Root = %q, want %q", got, want)
	}
}
