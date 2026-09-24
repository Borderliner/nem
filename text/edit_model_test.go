package text

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// Random edits against a plain []rune model, then undo back to the start and
// redo forward again. Insert and Delete work in place on lines' arrays, and a
// mistake there - two lines sharing an array, a head overwriting a tail not
// yet copied - shows up as text that silently differs from the model.
func TestRandomEditsMatchAModel(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 9))
	pieces := []string{"a", "bc", "\n", "x\ny", "\n\n", "漢", "é", "\t", "long line of text ", "\nz"}
	for trial := range 300 {
		b := NewBuffer()
		var model []rune
		var snapshots []string
		for range 60 {
			snapshots = append(snapshots, b.String())
			// Positions are chosen in the model and mapped to the buffer.
			pos := func(i int) Pos {
				p := Pos{}
				for _, r := range model[:i] {
					if r == '\n' {
						p.Line++
						p.Col = 0
					} else {
						p.Col++
					}
				}
				return p
			}
			if len(model) == 0 || rng.IntN(3) > 0 {
				i := rng.IntN(len(model) + 1)
				ins := []rune(pieces[rng.IntN(len(pieces))])
				if err := b.Insert(pos(i), ins); err != nil {
					t.Fatal(err)
				}
				model = append(model[:i:i], append(ins, model[i:]...)...)
			} else {
				i := rng.IntN(len(model))
				j := i + 1 + rng.IntN(min(len(model)-i, 12))
				if err := b.Delete(pos(i), pos(j)); err != nil {
					t.Fatal(err)
				}
				model = append(model[:i:i], model[j:]...)
			}
			b.BreakUndo()
			if got, want := b.String(), string(model); got != want {
				t.Fatalf("trial %d: buffer\n%q\nmodel\n%q", trial, got, want)
			}
		}
		final := b.String()
		for k := len(snapshots) - 1; k >= 0; k-- {
			if _, ok := b.Undo(); !ok {
				t.Fatalf("trial %d: undo ran out at step %d", trial, k)
			}
			if got := b.String(); got != snapshots[k] {
				t.Fatalf("trial %d: undo to step %d gave\n%q\nwant\n%q", trial, k, got, snapshots[k])
			}
		}
		for range snapshots {
			b.Redo()
		}
		if got := b.String(); got != final {
			t.Fatalf("trial %d: redo gave\n%q\nwant\n%q", trial, got, final)
		}
		if strings.Count(final, "\n")+1 != b.NumLines() {
			t.Fatalf("trial %d: %d lines for %d newlines", trial, b.NumLines(), strings.Count(final, "\n"))
		}
	}
}
