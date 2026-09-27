package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// benchTree is a project of n Go-like files of 300 lines each.
func benchTree(b *testing.B, n int) (string, []string) {
	b.Helper()
	root := b.TempDir()
	var body strings.Builder
	for i := range 300 {
		fmt.Fprintf(&body, "\tvalue%d := compute(%d) // a line of ordinary code\n", i, i)
	}
	var files []string
	for i := range n {
		rel := fmt.Sprintf("pkg%d/file%d.go", i%40, i)
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body.String()), 0o644); err != nil {
			b.Fatal(err)
		}
		files = append(files, rel)
	}
	return root, files
}

// C-x p g over 2000 files, 600,000 lines, for a word that is rare.
func BenchmarkSearch(b *testing.B) {
	root, files := benchTree(b, 2000)
	re := regexp.MustCompile("value17 ")
	none := func(string) ([]string, bool) { return nil, false }
	b.ReportAllocs()
	for b.Loop() {
		Search(root, files, re, none)
	}
}

// The same, for a pattern that ignores case, as C-x p g makes one typed in
// lower case.
func BenchmarkSearchFolded(b *testing.B) {
	root, files := benchTree(b, 2000)
	re := regexp.MustCompile("(?i)value17 ")
	none := func(string) ([]string, bool) { return nil, false }
	b.ReportAllocs()
	for b.Loop() {
		Search(root, files, re, none)
	}
}

// Listing a project without git: the walk C-x p f makes.
func BenchmarkFilesWalk(b *testing.B) {
	root, _ := benchTree(b, 2000)
	if err := os.WriteFile(filepath.Join(root, Marker), nil, 0o644); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Files(root); err != nil {
			b.Fatal(err)
		}
	}
}
