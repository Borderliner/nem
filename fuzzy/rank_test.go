package fuzzy

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// nemCommands is nem's real interactive command set, which is what M-x ranks.
// Using the real names rather than invented ones keeps the ordering assertions
// honest about the data this package actually sees.
var nemCommands = []string{
	"backward-char", "backward-kill-word", "backward-word", "beginning-of-buffer",
	"capitalize-word", "delete-backward-char", "delete-char", "delete-other-windows",
	"delete-window", "describe-bindings", "describe-key", "downcase-word",
	"end-of-buffer", "exchange-point-and-mark", "execute-extended-command",
	"find-file", "forward-char", "forward-word", "goto-line",
	"indent-for-tab-command", "isearch-backward", "isearch-forward", "keyboard-quit",
	"kill-buffer", "kill-line", "kill-region", "kill-ring-save", "kill-word",
	"list-buffers", "move-beginning-of-line", "move-end-of-line", "newline",
	"next-line", "open-line", "other-window", "previous-line", "query-replace",
	"recenter-top-bottom", "redo", "save-buffer", "save-buffers-kill-terminal",
	"save-some-buffers", "scroll-down-command", "scroll-up-command",
	"self-insert-command", "set-mark-command", "split-window-below",
	"split-window-right", "switch-to-buffer", "transpose-chars", "transpose-words",
	"undo", "upcase-word", "write-file", "yank", "yank-pop",
}

func names(r []Ranked) []string {
	out := make([]string, len(r))
	for i, x := range r {
		out[i] = x.Candidate
	}
	return out
}

// The headline case from the design: an abbreviation of the word starts finds
// the command, and finds it first.
func TestRankFindsForwardCharFromAbbreviation(t *testing.T) {
	got := names(Rank("fwc", nemCommands))
	if len(got) == 0 {
		t.Fatal(`Rank("fwc") returned nothing`)
	}
	if got[0] != "forward-char" {
		t.Errorf(`Rank("fwc")[0] = %q, want "forward-char" (full order: %v)`, got[0], got)
	}
}

// A whole word typed in full must put that word's commands on top, not a
// candidate where the letters happen to appear scattered.
func TestRankPutsWholeWordMatchesFirst(t *testing.T) {
	for _, tc := range []struct{ query, wantPrefix string }{
		{"kill", "kill-"},
		{"sav", "save-"},
		{"wind", ""}, // window commands, checked below
	} {
		if tc.wantPrefix == "" {
			continue
		}
		got := names(Rank(tc.query, nemCommands))
		if len(got) == 0 {
			t.Fatalf("Rank(%q) returned nothing", tc.query)
		}
		if !strings.HasPrefix(got[0], tc.wantPrefix) {
			t.Errorf("Rank(%q)[0] = %q, want something starting %q (order: %v)",
				tc.query, got[0], tc.wantPrefix, got[:min(5, len(got))])
		}
	}
}

// An empty query is the state of a prompt before the user types: everything is
// a candidate, in the order given, so the menu does not jump on the first key.
func TestRankWithEmptyQueryKeepsInputOrder(t *testing.T) {
	got := names(Rank("", nemCommands))
	if !reflect.DeepEqual(got, nemCommands) {
		t.Errorf("Rank(\"\") reordered or dropped candidates: got %d, want %d in input order",
			len(got), len(nemCommands))
	}
}

// Non-matching candidates are absent entirely, so a caller can show the result
// without filtering it again.
func TestRankDropsNonMatches(t *testing.T) {
	got := Rank("zzzz", nemCommands)
	if len(got) != 0 {
		t.Errorf("Rank(\"zzzz\") = %v, want no matches", names(got))
	}
}

// Ordering is total and stable: the same inputs must always give the same
// order, or a menu reshuffles for reasons the user cannot see.
func TestRankIsStableAcrossRepeatedCalls(t *testing.T) {
	first := names(Rank("bu", nemCommands))
	for i := 0; i < 20; i++ {
		if got := names(Rank("bu", nemCommands)); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differed:\n got %v\nwant %v", i, got, first)
		}
	}
}

// Shorter candidates win ties, so an exact short name is not buried under a
// longer one that matched equally well.
func TestRankBreaksTiesByLength(t *testing.T) {
	got := names(Rank("undo", []string{"undo-something-longer", "undo"}))
	if got[0] != "undo" {
		t.Errorf("got %v, want the shorter %q first", got, "undo")
	}
}

// Exact ordering, pinned. This is a regression guard rather than a claim that
// these orders are the only defensible ones: a change to any scoring constant
// must surface here as a visible diff instead of silently reshuffling somebody's
// M-x menu. If a change is deliberate and the new order is better, update this
// table and say so.
func TestRankOrderIsPinned(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"fwc", []string{"forward-char"}},
		{"kill", []string{
			"kill-line", "kill-word", "kill-buffer", "kill-region",
			"kill-ring-save", "backward-kill-word", "save-buffers-kill-terminal",
		}},
		{"sav", []string{
			"save-buffer", "save-some-buffers", "save-buffers-kill-terminal",
			"kill-ring-save",
		}},
		{"wind", []string{
			"other-window", "split-window-below", "split-window-right",
			"delete-window", "delete-other-windows",
		}},
		{"bufl", []string{"save-buffers-kill-terminal"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			if got := names(Rank(tc.query, nemCommands)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Rank(%q)\n got %v\nwant %v", tc.query, got, tc.want)
			}
		})
	}
}

// Stability, isolated. Every candidate here has the same score and the same
// length, so the score and length comparisons are both ties and only a stable
// sort can keep input order. Without this, sort.Slice passes every other test:
// the length tie-break hides instability wherever lengths differ.
func TestRankIsStableWhenScoreAndLengthBothTie(t *testing.T) {
	// Deliberately more than a dozen: Go's sort.Slice falls back to insertion
	// sort for small inputs, which is stable by accident, so a short slice
	// cannot tell a stable sort from an unstable one.
	// Two interleaved score groups, so partitioning genuinely has to move
	// elements past each other: an unstable sort can then scramble order within
	// a group. All-equal input is not enough, because Go's pdqsort detects that
	// pattern and returns without swapping anything.
	var in []string
	for i := 0; i < 20; i++ {
		in = append(in, fmt.Sprintf("a-%02d", i)) // 'a' at a boundary, higher score
		in = append(in, fmt.Sprintf("xa%02d", i)) // 'a' mid-word, lower score
	}
	got := names(Rank("a", in))
	var want []string
	for i := 0; i < 20; i++ {
		want = append(want, fmt.Sprintf("a-%02d", i))
	}
	for i := 0; i < 20; i++ {
		want = append(want, fmt.Sprintf("xa%02d", i))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rank reordered within equal-scoring groups:\n got %v\nwant %v", got, want)
	}
	in = nil
}

// Runs sorted apart and merged are the whole sorted at once, whatever their
// lengths - a worker whose share matched nothing brings an empty one - and
// however many ties the scores and lengths leave to the list's order. Each
// keeps its indices, and a match that had none has nil.
func TestMergeIsTheWholeSorted(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for range 500 {
		var cands []string
		var want []Ranked
		runs := make([]run, 1+r.IntN(14))
		for w := range runs {
			for range r.IntN(3) * r.IntN(40) {
				cand := strings.Repeat("x", r.IntN(4))
				h := hit{score: r.IntN(5) - 2, length: len(cand), index: len(cands), at: len(runs[w].ind), n: r.IntN(3)}
				var idx []int
				for k := range h.n {
					idx = append(idx, h.index*10+k)
				}
				runs[w].hits = append(runs[w].hits, h)
				runs[w].ind = append(runs[w].ind, idx...)
				cands = append(cands, cand)
				want = append(want, Ranked{Candidate: cand, Match: Match{Score: h.score, Indices: idx}, Index: h.index})
			}
			slices.SortFunc(runs[w].hits, better)
		}
		slices.SortStableFunc(want, func(x, y Ranked) int {
			return better(hit{score: x.Match.Score, length: len(x.Candidate), index: x.Index},
				hit{score: y.Match.Score, length: len(y.Candidate), index: y.Index})
		})
		if got := merge(runs, cands); len(got) != len(want) || len(want) > 0 && !reflect.DeepEqual(got, want) {
			t.Fatalf("merged %v; want %v", got, want)
		}
	}
}

// Each ranked candidate says where it was in the list, empty query or not.
func TestRankedKnowTheirPlace(t *testing.T) {
	for _, q := range []string{"", "kill", "w"} {
		for _, r := range Rank(q, nemCommands) {
			if nemCommands[r.Index] != r.Candidate {
				t.Fatalf("Rank(%q) puts %q at %d, where the list has %q", q, r.Candidate, r.Index, nemCommands[r.Index])
			}
		}
	}
	many := manyPaths(3 * parallelMin)
	for _, r := range Rank("edwin", many) {
		if many[r.Index] != r.Candidate {
			t.Fatalf("Rank over many puts %q at %d, where the list has %q", r.Candidate, r.Index, many[r.Index])
		}
	}
}
