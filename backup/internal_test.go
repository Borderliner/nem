package backup

import (
	"path/filepath"
	"runtime"
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
	root, want := "/", "/state/nem/backups/~"
	if runtime.GOOS == "windows" {
		// Each volume has a root of its own, inside the volume's directory.
		root, want = `C:\`, `\state\nem\backups\C\~`
	}
	got := s.mirror(backupsDir, root, backupSuffix)
	if got != want {
		t.Errorf("mirror(%s) = %q, want %q", root, got, want)
	}
	if !s.contains(got) {
		t.Errorf("mirror(%s) = %q, which is not inside the root", root, got)
	}
}

// A relative path is resolved against the working directory, so it still lands
// under the root rather than beside the file.
func TestMirrorAbsolutisesRelativeInput(t *testing.T) {
	s := New("/state/nem")
	got := s.mirror(backupsDir, "rel/main.go", backupSuffix)
	if want := filepath.FromSlash("/state/nem/backups/"); !strings.HasPrefix(got, want) {
		t.Errorf("mirror(rel/main.go) = %q, want it under %s", got, want)
	}
	if strings.Contains(got, "..") {
		t.Errorf("mirror produced %q, which still contains ..", got)
	}
}

// Cleaning is what removes "..", and it must happen before the join. Joining the
// raw input would let ../../etc/passwd climb out of the root.
func TestMirrorResolvesDotDotBeforeJoining(t *testing.T) {
	s := New("/state/nem")
	in, want := "/home/reza/../../etc/passwd", "/state/nem/backups/etc/passwd~"
	if runtime.GOOS == "windows" {
		in, want = `C:\home\reza\..\..\etc\passwd`, `\state\nem\backups\C\etc\passwd~`
	}
	got := s.mirror(backupsDir, in, backupSuffix)
	if got != want {
		t.Errorf("mirror = %q, want %q", got, want)
	}
	if !s.contains(got) {
		t.Fatalf("mirror = %q, which escapes the root", got)
	}
}

// A volume cannot sit in the middle of a path - backups\C:\Users is not a name
// Windows will create - so each kind of volume is mirrored as a plain
// directory.
func TestMirrorTurnsTheVolumeIntoADirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows paths have a volume")
	}
	s := New(`C:\state\nem`)
	for in, want := range map[string]string{
		`C:\Users\reza\main.go`:      `C:\state\nem\backups\C\Users\reza\main.go~`,
		`d:\x.go`:                    `C:\state\nem\backups\d\x.go~`,
		`\\host\share\p\main.go`:     `C:\state\nem\backups\host\share\p\main.go~`,
		`\\?\C:\Users\reza\main.go`:  `C:\state\nem\backups\C\Users\reza\main.go~`,
		`\\.\C:\Users\reza\main.go`:  `C:\state\nem\backups\C\Users\reza\main.go~`,
		`\\?\UNC\host\share\main.go`: `C:\state\nem\backups\UNC\host\share\main.go~`,
	} {
		got := s.mirror(backupsDir, in, backupSuffix)
		if got != want {
			t.Errorf("mirror(%s) = %q, want %q", in, got, want)
		}
		if strings.Contains(got[len(`C:`):], ":") {
			t.Errorf("mirror(%s) = %q, which has a colon past its own volume", in, got)
		}
	}
}

// Dropping the characters a file name cannot hold could leave a volume whose
// components read as "..". Cleaning the result as a rooted path is what stops
// that climbing out of the store.
func TestVolumeDirCannotClimb(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows paths have a volume")
	}
	for _, vol := range []string{`\\.?.\.?.`, `\\?\.?.`, `\\host\.?.`} {
		if d := volumeDir(vol); d == ".." || strings.HasPrefix(d, `..\`) {
			t.Errorf("volumeDir(%s) = %q, which climbs", vol, d)
		}
	}
}

func TestRoot(t *testing.T) {
	if got, want := New("/state/nem/").Root(), filepath.Clean("/state/nem"); got != want {
		t.Errorf("Root = %q, want %q", got, want)
	}
}
