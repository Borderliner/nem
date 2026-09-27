package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Kept files go under the root, in their kind's directory, never over one
// another, whatever the name they are given.
func TestKeepNeverOverwritesOrEscapes(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	var paths []string
	for _, name := range []string{"*scratch*.txt", "*scratch*.txt", "../../etc/passwd", "", "C:\\x"} {
		p, err := s.Keep(Rescued, name, []byte(name))
		if err != nil {
			t.Fatalf("Keep(%q): %v", name, err)
		}
		if !strings.HasPrefix(p, filepath.Join(root, Rescued)+string(filepath.Separator)) {
			t.Errorf("Keep(%q) wrote %s, outside %s", name, p, filepath.Join(root, Rescued))
		}
		got, err := os.ReadFile(p)
		if err != nil || string(got) != name {
			t.Errorf("Keep(%q): %s holds %q, %v", name, p, got, err)
		}
		paths = append(paths, p)
	}
	if paths[0] == paths[1] {
		t.Errorf("the same name kept twice went to one file, %s", paths[0])
	}
	if filepath.Base(paths[0]) != "scratch_.txt" || filepath.Base(paths[1]) != "scratch_-2.txt" {
		t.Errorf("kept as %s and %s", filepath.Base(paths[0]), filepath.Base(paths[1]))
	}
	if _, err := s.Keep("elsewhere", "x", nil); err == nil {
		t.Error("kept a file of no known kind")
	}
}
