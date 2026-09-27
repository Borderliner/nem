package fuzzy

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// Every term form, matched and refused. The candidates are chosen so that a
// form read as the wrong one - an exact term read as fuzzy, a prefix read as
// exact - gives the wrong answer on at least one row.
func TestTermForms(t *testing.T) {
	for _, tc := range []struct {
		query, candidate string
		want             bool
	}{
		{"fwc", "forward-char", true},

		{"'forw", "forward-char", true},
		{"'fwc", "forward-char", false},
		{"'ward-ch", "forward-char", true},

		{"^forw", "forward-char", true},
		{"^char", "forward-char", false},
		{"^fwc", "forward-char", false},

		{"char$", "forward-char", true},
		{"forw$", "forward-char", false},
		{"chr$", "forward-char", false},

		{"^undo$", "undo", true},
		{"^undo$", "undo-tree", false},
		{"^und$", "undo", false},

		{"!kill", "yank", true},
		{"!kill", "kill-line", false},
		{"!kl", "kill-line", true}, // negation is exact, not fuzzy
		{"!'kill", "kill-line", false},
		{"!^kill", "backward-kill-word", true},
		{"!^kill", "kill-line", false},
		{"!word$", "word-count", true},
		{"!word$", "kill-word", false},
		{"!^undo$", "undo", false},
		{"!^undo$", "undo-tree", true},

		{"kill word", "backward-kill-word", true},
		{"kill word", "kill-line", false},
		{"word kill", "backward-kill-word", true}, // terms match in any order

		{"yank | undo", "yank", true},
		{"yank | undo", "undo", true},
		{"yank | undo", "redo", false},
		{"kill | yank word", "kill-word", true},
		{"kill | yank word", "yank-word", true},
		{"kill | yank word", "yank", false},
		{"^kill | ^yank", "kill-line", true},
		{"^kill | ^yank", "backward-kill-word", false},
		{"!kill | ^yank", "yank-pop", true},

		{`a\ b`, "a b", true},
		{`a\ b`, "x a yb", true},
		{`a\ b`, "a-b", false},
		{`'a\ b`, "xa yb", false},
		{`'a\ b`, "xa b", true},
		{"a   b", "b-a", true}, // a run of spaces is one separator
		{" a ", "a", true},
	} {
		t.Run(tc.query+" "+tc.candidate, func(t *testing.T) {
			if _, got := Score(tc.query, tc.candidate); got != tc.want {
				t.Errorf("Score(%q, %q) matched = %v, want %v", tc.query, tc.candidate, got, tc.want)
			}
		})
	}
}

// Smart case belongs to each term, so "Makefile !test" can be exact about
// the file without making the negation exact too.
func TestSmartCaseIsPerTerm(t *testing.T) {
	for _, tc := range []struct {
		query, candidate string
		want             bool
	}{
		{"Kill word", "Kill-WORD", true},
		{"Kill word", "kill-word", false},
		{"kill Word", "KILL-Word", true},
		{"kill Word", "kill-word", false},
		{"'Kill", "kill", false},
		{"'kill", "KILL", true},
		{"^Kill", "kill", false},
		{"!Test", "a_test.go", true},
		{"!test", "a_Test.go", false},
		{"'ÉTÉ", "été", false},
		{"'été", "ÉTÉ", true},
		{"été$", "ÉTÉ", true},
	} {
		t.Run(tc.query+" "+tc.candidate, func(t *testing.T) {
			if _, got := Score(tc.query, tc.candidate); got != tc.want {
				t.Errorf("Score(%q, %q) matched = %v, want %v", tc.query, tc.candidate, got, tc.want)
			}
		})
	}
}

// A marker typed before its word, or a | before the next term, must not
// empty the list for a keystroke: the query reads as though it were not
// there yet.
func TestMarkersAloneAreIgnored(t *testing.T) {
	all := names(Rank("", nemCommands))
	for _, q := range []string{"'", "^", "$", "!", "!^", "!'", "^$", "|", "' ^", " | "} {
		if got := names(Rank(q, nemCommands)); !reflect.DeepEqual(got, all) {
			t.Errorf("Rank(%q) = %d candidates in some order, want all %d in input order", q, len(got), len(all))
		}
		if m, ok := Score(q, "anything"); !ok || m.Score != 0 || m.Indices != nil {
			t.Errorf("Score(%q) = %v, %v; want a match with nothing to show", q, m, ok)
		}
	}
	kill := Rank("kill", nemCommands)
	for _, q := range []string{"kill '", "kill ^", "kill $", "kill !", "kill !^", "kill |", "| kill", "kill | '", "' kill"} {
		if got := Rank(q, nemCommands); !reflect.DeepEqual(got, kill) {
			t.Errorf("Rank(%q) = %v, want Rank(\"kill\") = %v", q, names(got), names(kill))
		}
	}
}

// One plain word - no spaces, no markers - is what every query was before
// the syntax, and must score exactly as it did: same score, same indices.
// scoreRunes is that scorer, unchanged.
func TestPlainQueryScoresAsItAlwaysDid(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	alphabet := []rune("abcdefgkilnorw-_/.ABCDÉé日")
	cands := append(slices.Clone(nemCommands), "Makefile", "été/ÉTÉ.go", "日本語-x", "a-b_c/d.e")
	matched := 0
	for range 5000 {
		cand := cands[r.IntN(len(cands))]
		var q []rune
		if r.IntN(2) == 0 {
			// Most random queries match nothing, which tests only the
			// rejection; half are drawn from the candidate instead, some
			// with a rune shouted to make them case-sensitive.
			for _, c := range cand {
				if c != ' ' && r.IntN(3) == 0 {
					q = append(q, c)
				}
			}
			if len(q) > 0 && r.IntN(4) == 0 {
				i := r.IntN(len(q))
				q[i] = unicode.ToUpper(q[i])
			}
		}
		for len(q) == 0 || r.IntN(4) == 0 {
			q = append(q, alphabet[r.IntN(len(alphabet))])
		}
		query := string(q)
		got, gotOK := Score(query, cand)
		want, wantOK := scoreRunes(q, []rune(cand), smartCaseFold(q))
		if gotOK != wantOK || !reflect.DeepEqual(got, want) {
			t.Fatalf("Score(%q, %q) = %v, %v; want %v, %v", query, cand, got, gotOK, want, wantOK)
		}
		if gotOK {
			matched++
		}
	}
	if matched < 1000 {
		t.Fatalf("only %d of 5000 queries matched, too few to show the scores agree", matched)
	}
}

// An exact term is scored as a fuzzy one would score the same runes matched
// in a row, so where the fuzzy match is that run the two agree exactly.
func TestExactTermsShareTheFuzzyScale(t *testing.T) {
	for _, tc := range []struct{ query, candidate string }{
		{"abc", "abc"},
		{"abc", "x-abc-y"},
		{"abc", "xABc"},
		{"char", "forward-char"},
	} {
		fuzzy := mustScore(t, tc.query, tc.candidate)
		for _, q := range []string{"'" + tc.query, tc.query + "$", "'" + tc.query + "$"} {
			if !strings.HasSuffix(tc.candidate, tc.query) && strings.HasSuffix(q, "$") {
				continue
			}
			if got := mustScore(t, q, tc.candidate); got != fuzzy {
				t.Errorf("Score(%q, %q) = %d, want %d as for %q", q, tc.candidate, got, fuzzy, tc.query)
			}
		}
	}
	if got, want := mustScore(t, "^abc$", "abc"), mustScore(t, "abc", "abc"); got != want {
		t.Errorf(`Score("^abc$", "abc") = %d, want %d`, got, want)
	}
}

// Of several occurrences an exact term takes the best, as the fuzzy scorer
// takes the best alignment: here the go that starts a word, not the first.
func TestExactTermTakesItsBestOccurrence(t *testing.T) {
	m, ok := Score("'go", "cargo/go.mod")
	if !ok {
		t.Fatal("no match")
	}
	if want := []int{6, 7}; !reflect.DeepEqual(m.Indices, want) {
		t.Errorf("Indices = %v, want %v, the go after the slash", m.Indices, want)
	}
	got := names(Rank("'char", []string{"xchar", "x-char"}))
	if want := []string{"x-char", "xchar"}; !reflect.DeepEqual(got, want) {
		t.Errorf(`Rank("'char") = %v, want %v: the run on a word start first`, got, want)
	}
}

// Terms' scores add up and their indices merge, sorted and each once; a
// negated term adds neither.
func TestTermsSumAndMerge(t *testing.T) {
	m, ok := Score("word kill", "backward-kill-word")
	if !ok {
		t.Fatal("no match")
	}
	kill, _ := Score("kill", "backward-kill-word")
	word, _ := Score("word", "backward-kill-word")
	if m.Score != kill.Score+word.Score {
		t.Errorf("Score = %d, want %d + %d", m.Score, kill.Score, word.Score)
	}
	if want := []int{9, 10, 11, 12, 14, 15, 16, 17}; !reflect.DeepEqual(m.Indices, want) {
		t.Errorf("Indices = %v, want %v", m.Indices, want)
	}

	if m, _ := Score("go 'go", "go"); !reflect.DeepEqual(m.Indices, []int{0, 1}) {
		t.Errorf(`Score("go 'go", "go").Indices = %v, want each offset once`, m.Indices)
	}

	with, _ := Score("kill !yank", "kill-line")
	without, _ := Score("kill", "kill-line")
	if !reflect.DeepEqual(with, without) {
		t.Errorf("a negated term changed the match: %v, want %v", with, without)
	}
	if m, ok := Score("!yank", "kill-line"); !ok || m.Score != 0 || m.Indices != nil {
		t.Errorf(`Score("!yank", "kill-line") = %v, %v; want a match scoring 0 with no indices`, m, ok)
	}
}

// A group scores as its best member that matches, and a member that does
// not match costs it nothing.
func TestOrGroupScoresItsBestMember(t *testing.T) {
	for _, tc := range []struct{ query, best, candidate string }{
		{"zzz | abc", "abc", "abc"},
		{"abc | zzz", "abc", "abc"},
		{"ac | abc", "abc", "abc"},
		{"abc | ac", "abc", "abc"},
		{"!zzz | abc", "abc", "abc"},
	} {
		got, _ := Score(tc.query, tc.candidate)
		want, _ := Score(tc.best, tc.candidate)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Score(%q, %q) = %v, want %v as for %q", tc.query, tc.candidate, got, want, tc.best)
		}
	}
}

// A query too long to have been typed is text to find: its spaces and
// markers are runes like any other, not syntax.
func TestYankedQueryIsText(t *testing.T) {
	long := strings.Repeat("x", maxSyntaxLen) + " !y"
	if _, ok := Score(long, long); !ok {
		t.Error("a long query with a ! in it did not match itself")
	}
	if _, ok := Score(long, strings.Repeat("x", maxSyntaxLen)); ok {
		t.Error("a long query matched without its space and !y")
	}
}

// Rank and Score agree on every candidate, on one core and many, for a
// query using every form at once.
func TestRankAgreesWithScore(t *testing.T) {
	var cands []string
	for i := range 3 * parallelMin {
		cands = append(cands, fmt.Sprintf("dir%d/Sub%d/file_%d%s", i%37, i%11, i, []string{".go", "_test.go", ".md"}[i%3]))
	}
	for _, q := range []string{"d3fi", "d3 'go !test", "^dir1 | Sub2 .md$", "!^dir2 '_1 | ^dir3 fl"} {
		many := Rank(q, cands)
		var want []Ranked
		for _, c := range cands {
			if m, ok := Score(q, c); ok {
				want = append(want, Ranked{Candidate: c, Match: m})
			}
		}
		if len(many) != len(want) || len(many) == 0 {
			t.Fatalf("Rank(%q) found %d, Score %d", q, len(many), len(want))
		}
		byName := make(map[string]Match, len(want))
		for _, w := range want {
			byName[w.Candidate] = w.Match
		}
		for _, r := range many {
			if !reflect.DeepEqual(r.Match, byName[r.Candidate]) {
				t.Fatalf("Rank(%q) matched %q as %v, Score as %v", q, r.Candidate, r.Match, byName[r.Candidate])
			}
		}
		func() {
			defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
			if one := Rank(q, cands); !reflect.DeepEqual(many, one) {
				t.Fatalf("Rank(%q) differs on one core", q)
			}
		}()
	}
}
