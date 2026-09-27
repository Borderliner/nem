package text

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// A line measured again only from an edit must come out exactly as one
// measured from scratch, whatever the edit: in the middle of a cluster, a
// combining mark or a joiner after one, tabs whose width depends on what
// comes before them, text in other scripts, emoji sequences.
func TestIncrementalLayoutMatchesAFreshOne(t *testing.T) {
	pieces := [][]rune{
		[]rune("abc"), []rune("x"), []rune("\t"), []rune("é"), []rune("́"),
		[]rune("日本"), []rune("👍🏽"), []rune("‍"), []rune("👩‍💻"),
		[]rune("سلام"), []rune("🇮🇷"), []rune("🇮"), []rune("🇷"), []rune(" "), []rune("َ"),
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for round := range 3000 {
		b := NewBuffer()
		for range 1 + rng.IntN(4) {
			p := pieces[rng.IntN(len(pieces))]
			if err := b.Insert(b.End(), p); err != nil {
				t.Fatal(err)
			}
		}
		l := b.Line(0)
		l.Width() // measure, so the edits below leave a cache to reuse
		for range 1 + rng.IntN(3) {
			n := int(l.Len())
			switch rng.IntN(2) {
			case 0:
				at := Pos{Col: RuneIdx(rng.IntN(n + 1))}
				if err := b.Insert(at, pieces[rng.IntN(len(pieces))]); err != nil {
					t.Fatal(err)
				}
			default:
				if n == 0 {
					continue
				}
				from := rng.IntN(n)
				to := from + 1 + rng.IntN(n-from)
				if err := b.Delete(Pos{Col: RuneIdx(from)}, Pos{Col: RuneIdx(to)}); err != nil {
					t.Fatal(err)
				}
			}
			if rng.IntN(2) == 0 {
				l.Width() // sometimes measured between edits, sometimes not
			}
		}

		fresh := NewLine(l.View())
		fresh.build()
		l.build()
		if !slices.Equal(l.segs, fresh.segs) || l.width != fresh.width || l.ASCII() != fresh.ASCII() {
			t.Fatalf("round %d, %q: measured again from the edit\n%+v (width %d, ascii %v)\nfrom scratch\n%+v (width %d, ascii %v)",
				round, string(l.View()), l.segs, l.width, l.ASCII(), fresh.segs, fresh.width, fresh.ASCII())
		}
	}
}
