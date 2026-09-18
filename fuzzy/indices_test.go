package fuzzy

import (
	"reflect"
	"testing"
)

// Indices address RUNES, not bytes. The completion panel emphasises exactly
// these positions, so a byte offset highlights the wrong character - or lands
// mid-rune and highlights nothing - the moment a candidate is not pure ASCII.
func TestIndicesAreRuneOffsetsNotBytes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		query     string
		candidate string
		want      []int
	}{
		{"ascii", "fc", "forward-char", []int{0, 8}},
		// 日本語-x: each CJK rune is 3 bytes, so a byte offset for 'x' would be 9.
		{"after cjk", "x", "日本語x", []int{3}},
		{"within cjk", "語", "日本語x", []int{2}},
		// An emoji outside the BMP is 4 bytes; a byte offset would be 4.
		{"after emoji", "a", "🙂a", []int{1}},
		// Decomposed e + U+0301: the accent is its own rune, so 'x' is rune 2.
		{"after combining mark", "x", "éx", []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := Score(tc.query, tc.candidate)
			if !ok {
				t.Fatalf("Score(%q, %q) reported no match", tc.query, tc.candidate)
			}
			if !reflect.DeepEqual(m.Indices, tc.want) {
				t.Errorf("Indices = %v, want %v", m.Indices, tc.want)
			}
		})
	}
}

// Indices must be strictly ascending. A highlighter walking them in order
// would otherwise emphasise cells twice or skip backwards.
func TestIndicesAreStrictlyAscending(t *testing.T) {
	m, ok := Score("sbkt", "save-buffers-kill-terminal")
	if !ok {
		t.Fatal("no match")
	}
	for i := 1; i < len(m.Indices); i++ {
		if m.Indices[i] <= m.Indices[i-1] {
			t.Fatalf("Indices %v not strictly ascending at %d", m.Indices, i)
		}
	}
}

// One index per query rune, always.
func TestIndicesCountEqualsQueryLength(t *testing.T) {
	m, ok := Score("swb", "switch-to-buffer")
	if !ok {
		t.Fatal("no match")
	}
	if len(m.Indices) != 3 {
		t.Errorf("len(Indices) = %d for a 3-rune query, want 3 (%v)", len(m.Indices), m.Indices)
	}
}
