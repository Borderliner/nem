package text

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Saving a 20MB file, 700,000 lines.
func BenchmarkSave20MB(b *testing.B) {
	path := filepath.Join(b.TempDir(), "big.txt")
	line := "\tfmt.Println(strings.ToUpper(w.Name), w.Count) // a comment\n"
	if err := os.WriteFile(path, []byte(strings.Repeat(line, 20<<20/len(line))), 0o644); err != nil {
		b.Fatal(err)
	}
	buf, err := LoadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := buf.Save(); err != nil {
			b.Fatal(err)
		}
	}
}
