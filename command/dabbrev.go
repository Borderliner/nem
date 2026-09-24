package command

import (
	"github.com/Borderliner/nem/text"
)

// Dynamic abbreviations: M-/ completes the word before point from words that
// are already written somewhere. The nearest come first - before point, then
// after it, then in the other buffers - since the word being typed is most
// often one used a few lines up. Pressing M-/ again replaces the expansion
// with the next candidate; when they run out, the word goes back to what was
// typed.

// dabbrevMax bounds the candidates collected, which bounds the first M-/ in a
// very large buffer.
const dabbrevMax = 1000

// dabbrevState is an expansion in progress.
type dabbrevState struct {
	buf    *text.Buffer
	start  text.Pos // where the typed prefix begins
	end    text.Pos // where the current expansion ends, and point is
	prefix string
	cands  []string
	next   int
}

// RegisterDabbrev adds dabbrev-expand to r.
func RegisterDabbrev(r *Registry) error {
	return r.Register(Command{
		Name:        "dabbrev-expand",
		Doc:         "Complete the word before point from words already in the text; repeat for the next.",
		Fn:          dabbrevExpand,
		Interactive: true,
	})
}

func dabbrevExpand(e Env) error {
	b, p := e.Buf(), e.Win().Pt
	st := &e.Seq().dabbrev
	continuing := e.LastCommand() == "dabbrev-expand" && st.buf == b && st.end == p

	if !continuing {
		rs := b.Line(p.Line).View()
		i := int(p.Col)
		for i > 0 && isSymbolRune(rs[i-1]) {
			i--
		}
		if i == int(p.Col) {
			e.Echo("No dynamic expansion for \"\" found")
			return nil
		}
		start := text.Pos{Line: p.Line, Col: text.RuneIdx(i)}
		prefix := string(rs[i:p.Col])
		*st = dabbrevState{buf: b, start: start, end: p, prefix: prefix,
			cands: dabbrevCandidates(e, b, start, p, prefix)}
	}

	word, more := st.prefix, st.next < len(st.cands)
	if more {
		word = st.cands[st.next]
		st.next++
	}
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if err := b.Delete(st.start, st.end); err != nil {
		return err
	}
	if err := b.Insert(st.start, []rune(word)); err != nil {
		return err
	}
	st.end = text.Pos{Line: st.start.Line, Col: st.start.Col + text.RuneIdx(len([]rune(word)))}
	edSetPoint(e, st.end)

	if !more {
		if continuing {
			e.Echo("No further dynamic expansion for %q found", st.prefix)
		} else {
			e.Echo("No dynamic expansion for %q found", st.prefix)
		}
		st.buf = nil // the next M-/ starts afresh
	}
	return nil
}

// dabbrevCandidates collects the words that extend prefix, nearest first:
// before start in b, working back; after end in b; then the other buffers
// from their tops. Each word is offered once.
func dabbrevCandidates(e Env, b *text.Buffer, start, end text.Pos, prefix string) []string {
	seen := map[string]bool{prefix: true}
	var out []string
	take := func(w string) bool {
		if len(w) > len(prefix) && w[:len(prefix)] == prefix && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
		return len(out) < dabbrevMax
	}

	// Backwards from the prefix: the rest of its line, then the lines above.
	for ln := start.Line; ln >= 0; ln-- {
		rs := b.Line(ln).View()
		if ln == start.Line {
			rs = rs[:start.Col]
		}
		words := wordsIn(rs)
		for i := len(words) - 1; i >= 0; i-- {
			if !take(words[i]) {
				return out
			}
		}
	}
	for ln := end.Line; ln < b.NumLines(); ln++ {
		rs := b.Line(ln).View()
		if ln == end.Line {
			rs = rs[end.Col:]
		}
		for _, w := range wordsIn(rs) {
			if !take(w) {
				return out
			}
		}
	}
	for _, other := range e.Buffers() {
		if other == b {
			continue
		}
		for ln := range other.NumLines() {
			for _, w := range wordsIn(other.Line(ln).View()) {
				if !take(w) {
					return out
				}
			}
		}
	}
	return out
}

// wordsIn splits rs into its words, in order.
func wordsIn(rs []rune) []string {
	var out []string
	for i := 0; i < len(rs); {
		if !isSymbolRune(rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && isSymbolRune(rs[j]) {
			j++
		}
		out = append(out, string(rs[i:j]))
		i = j
	}
	return out
}
