package imenu

import (
	"fmt"
	"testing"
)

// M-g i on a Go file of 20,000 lines.
func BenchmarkFor(b *testing.B) {
	var lines []string
	for i := range 2000 {
		lines = append(lines,
			"// Widget holds a name.",
			fmt.Sprintf("type Widget%d struct {", i),
			"\tName string",
			"}",
			"",
			fmt.Sprintf("func (w *Widget%d) String() string {", i),
			"\treturn strings.ToUpper(w.Name)",
			"}",
			"",
			fmt.Sprintf("func helper%d(x int) int { return x * 2 }", i))
	}
	b.ReportAllocs()
	for b.Loop() {
		For("big.go", lines)
	}
}
