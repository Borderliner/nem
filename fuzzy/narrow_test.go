package fuzzy

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// The keystrokes a prompt sees, and whether each narrows the query before it.
// A false here is not a claim that the list widens, only that Narrows cannot
// promise it does not; the property test below is what checks the promises.
func TestNarrows(t *testing.T) {
	for _, tc := range []struct {
		prev, next string
		want       bool
	}{
		{"ab", "ab", true},
		{"", "ab", true},
		{"'", "'ab", true}, // a marker alone matches everything
		{"ab |", "ab", true},

		// Adding to the last word.
		{"ab", "abc", true},
		{"'ab", "'abc", true},
		{"^ab", "^abc", true},
		{"ab", "abC", true}, // now exact about case, and so narrower still
		{"ab", "ab$", true},
		{"^ab", "^ab$", true},
		{"ab$", "ab$c", false}, // the anchor is no longer at the end
		{"!ab", "!abc", false}, // excludes less
		{`ab\`, `ab\ `, false}, // the escape takes the backslash away

		// Adding a word.
		{"ab", "ab c", true},
		{"ab", "ab !c", true},
		{"a | b", "a | b c", true},
		{"a |", "a | b", false}, // joins the group, and widens it
		{"a b", "a b | c", false},

		// Adding to a group's last word: a or bc implies a or b.
		{"a | b", "a | bc", true},
		{"a | b", "ab | b", true},
		{"a | b", "a | b | c", false},

		// Not additions at all, but provably narrower.
		{"ab", "'ab", true},
		{"!abc", "!ab", true},
		{"ab cd", "cd ab", true},
		{"abc", "ab", false},
		{"'ab", "ab", false},
		{"Ab", "ab", false},
	} {
		if got := Narrows(tc.prev, tc.next); got != tc.want {
			t.Errorf("Narrows(%q, %q) = %v, want %v", tc.prev, tc.next, got, tc.want)
		}
	}
}

// The promise itself: whenever Narrows says next narrows prev, ranking next
// finds no candidate that ranking prev did not. Queries and candidates are
// random, from runes that are syntax, that fold, and that do neither. Most
// of the time next is prev with something typed after it, since that is what
// a prompt will ask about; otherwise it is prev reworded, or anything at all.
func TestNarrowsNeverDropsAMatch(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	queryRunes := []rune(`abcAB  '^$!|\`)
	candRunes := []rune(`abcABé-_ '^$!|\`)
	pick := func(from []rune, lo, hi int) string {
		out := make([]rune, lo+r.IntN(hi-lo+1))
		for i := range out {
			out[i] = from[r.IntN(len(from))]
		}
		return string(out)
	}
	seen := map[string]bool{}
	var cands []string
	for len(cands) < 300 {
		if c := pick(candRunes, 0, 8); !seen[c] {
			seen[c] = true
			cands = append(cands, c)
		}
	}
	// Random candidates seldom look like the query, and the ones that break a
	// false promise usually do: the text of a word, with a rune more or less
	// either side, in another case. So each pair brings some of its own.
	near := func(queries ...string) []string {
		var out []string
		for _, q := range queries {
			for _, w := range strings.Fields(q) {
				w = strings.TrimLeft(w, `!'^`)
				for _, v := range []string{w, strings.TrimRight(w, "$"), strings.ToUpper(w), strings.ToLower(w)} {
					out = append(out, v, pick(candRunes, 1, 2)+v, v+pick(candRunes, 1, 2), pick(candRunes, 1, 1)+v+pick(candRunes, 1, 1))
				}
			}
		}
		return out
	}
	// Typing only ever adds to a query, which leaves most of the rules
	// untried: those for a term implied by one of another kind. So some pairs
	// are prev reworded instead - each word's markers changed, a rune added
	// to it or taken away, the words shuffled and now and then joined by a |.
	markers := []string{"", "'", "^", "!", "!^", "!'"}
	reword := func(q string) string {
		var words []string
		for _, w := range strings.Fields(q) {
			w = strings.TrimRight(strings.TrimLeft(w, `!'^`), "$")
			switch r.IntN(3) {
			case 0:
				w += pick(queryRunes[:5], 1, 1)
			case 1:
				if w != "" {
					w = w[:len(w)-1]
				}
			}
			w = markers[r.IntN(len(markers))] + w
			if r.IntN(3) == 0 {
				w += "$"
			}
			words = append(words, w)
			if r.IntN(4) == 0 {
				words = append(words, "|")
			}
		}
		r.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
		return strings.Join(words, " ")
	}
	narrowed := 0
	for range 30000 {
		prev := pick(queryRunes, 0, 6)
		var next string
		switch r.IntN(8) {
		case 0:
			next = pick(queryRunes, 0, 8)
		case 1, 2, 3:
			next = reword(prev)
		default:
			next = prev + pick(queryRunes, 1, 3)
		}
		if !Narrows(prev, next) {
			continue
		}
		narrowed++
		cands := append(near(prev, next), cands...)
		before := map[string]bool{}
		for _, m := range Rank(prev, cands) {
			before[m.Candidate] = true
		}
		for _, m := range Rank(next, cands) {
			if !before[m.Candidate] {
				t.Fatalf("Narrows(%q, %q), but %q matches the second and not the first", prev, next, m.Candidate)
			}
		}
	}
	if narrowed < 5000 {
		t.Fatalf("only %d of 30000 pairs narrowed, too few for the test to mean much", narrowed)
	}
}

// The keystrokes that leave a query as it was, as Rank reads it, and some
// that look as if they might and do not.
func TestSame(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"ab", "ab", true},
		{"ab", "ab ", true},
		{"ab", "  ab  ", true},
		{"ab", "ab '", true},
		{"ab", "ab ^", true},
		{"ab", "ab !", true},
		{"ab", "ab !^", true},
		{"ab", "ab |", true},
		{"", "'", true},
		{"a b", "a  b", true},

		{"ab", "abc", false},
		{"ab", "Ab", false},
		{"ab", "'ab", false},
		{"ab", "ab$", false},
		{"a b", "b a", false}, // ranked alike, but not the one query
		{`ab\`, `ab\ `, false},
		{"a | b", "a b", false},
	} {
		if got := Same(tc.a, tc.b); got != tc.want {
			t.Errorf("Same(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// What Same promises: two queries it calls the same rank a list alike,
// scores, indices, order and all.
func TestSameRanksAlike(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	queryRunes := []rune(`abAB  '^$!|\`)
	pick := func(lo, hi int) string {
		out := make([]rune, lo+r.IntN(hi-lo+1))
		for i := range out {
			out[i] = queryRunes[r.IntN(len(queryRunes))]
		}
		return string(out)
	}
	cands := manyPaths(500)
	for i := range 200 {
		cands = append(cands, strings.Repeat("ab", i%5)+pick(0, 6))
	}
	same := 0
	for range 20000 {
		a := pick(0, 6)
		b := a + pick(1, 2)
		if !Same(a, b) {
			continue
		}
		same++
		if got, want := Rank(b, cands), Rank(a, cands); !reflect.DeepEqual(got, want) {
			t.Fatalf("Same(%q, %q), but they rank the list differently", a, b)
		}
	}
	if same < 1000 {
		t.Fatalf("only %d of 20000 pairs were the same, too few for the test to mean much", same)
	}
}
