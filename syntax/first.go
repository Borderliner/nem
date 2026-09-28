package syntax

import (
	"regexp"
	regsyntax "regexp/syntax"
	"unicode"
)

// first is the runes a match can begin with, so a place in a line whose rune
// is not one of them is passed over without running the pattern. A language
// has a handful of regions and patterns and a line has dozens of places, and
// asking each pattern at each place was most of the time a line took.
type first struct {
	set runeSet
	any bool // it may begin with anything, or with nothing at all
}

func (f *first) has(r rune) bool { return f.any || f.set.has(r) }

func (f *first) add(o first) {
	if o.any {
		f.any = true
		return
	}
	for r, ok := range o.set.ascii {
		if ok {
			f.set.ascii[r] = true
		}
	}
	for r := range o.set.other {
		if f.set.other == nil {
			f.set.other = map[rune]bool{}
		}
		f.set.other[r] = true
	}
}

func (f *first) addRune(r rune, fold bool) {
	rs := []rune{r}
	if fold {
		for c := unicode.SimpleFold(r); c != r; c = unicode.SimpleFold(c) {
			rs = append(rs, c)
		}
	}
	for _, c := range rs {
		if c < 128 {
			f.set.ascii[c] = true
			continue
		}
		if f.set.other == nil {
			f.set.other = map[rune]bool{}
		}
		f.set.other[c] = true
	}
}

// maxClass is how many runes a class may hold before first gives up on it and
// takes any rune: [^"] holds nearly all of them.
const maxClass = 512

// firstOf works out what re can begin with.
func firstOf(re *regexp.Regexp) first {
	tree, err := regsyntax.Parse(re.String(), regsyntax.Perl)
	if err != nil {
		return first{any: true}
	}
	f, empty := firstTree(tree.Simplify())
	if empty {
		// It can match nothing at all, and a match of nothing is passed
		// over wherever it is; but what follows may still begin it.
		f.any = true
	}
	return f
}

// firstTree returns what t can begin with, and whether it can match nothing.
func firstTree(t *regsyntax.Regexp) (first, bool) {
	fold := t.Flags&regsyntax.FoldCase != 0
	switch t.Op {
	case regsyntax.OpNoMatch:
		return first{}, false
	case regsyntax.OpEmptyMatch, regsyntax.OpBeginLine, regsyntax.OpEndLine,
		regsyntax.OpBeginText, regsyntax.OpEndText, regsyntax.OpWordBoundary,
		regsyntax.OpNoWordBoundary:
		return first{}, true
	case regsyntax.OpLiteral:
		var f first
		if len(t.Rune) == 0 {
			return f, true
		}
		f.addRune(t.Rune[0], fold)
		return f, false
	case regsyntax.OpCharClass:
		var f first
		n := 0
		for i := 0; i+1 < len(t.Rune); i += 2 {
			n += int(t.Rune[i+1]-t.Rune[i]) + 1
			if n > maxClass {
				return first{any: true}, false
			}
		}
		for i := 0; i+1 < len(t.Rune); i += 2 {
			for r := t.Rune[i]; r <= t.Rune[i+1]; r++ {
				f.addRune(r, fold)
			}
		}
		return f, false
	case regsyntax.OpAnyChar, regsyntax.OpAnyCharNotNL:
		return first{any: true}, false
	case regsyntax.OpCapture:
		return firstTree(t.Sub[0])
	case regsyntax.OpStar, regsyntax.OpQuest:
		f, _ := firstTree(t.Sub[0])
		return f, true
	case regsyntax.OpPlus:
		return firstTree(t.Sub[0])
	case regsyntax.OpRepeat:
		f, empty := firstTree(t.Sub[0])
		return f, empty || t.Min == 0
	case regsyntax.OpConcat:
		var f first
		for _, sub := range t.Sub {
			sf, empty := firstTree(sub)
			f.add(sf)
			if !empty {
				return f, false
			}
		}
		return f, true
	case regsyntax.OpAlternate:
		var f first
		empty := false
		for _, sub := range t.Sub {
			sf, e := firstTree(sub)
			f.add(sf)
			empty = empty || e
		}
		return f, empty
	}
	return first{any: true}, true
}
