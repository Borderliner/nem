package command

import (
	"slices"
	"strings"

	"github.com/Borderliner/nem/text"
)

// Rewriting the region in place: its case.

// RegisterTransform adds the commands that rewrite the region's text to r.
func RegisterTransform(r *Registry) error {
	cmds := []Command{
		{Name: "upcase-region", Doc: "Convert the region to upper case.", Fn: upcaseRegion},
		{Name: "downcase-region", Doc: "Convert the region to lower case.", Fn: downcaseRegion},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// Messages emacs gives for a command that needs a region when there is none:
// no mark at all, or one that is not active.
const (
	noRegion       = "The mark is not set now, so there is no region"
	regionInactive = "The mark is not active now"
)

// requireRegion reports whether there is an active region, and says why not
// when there is not.
//
// Active, not merely a mark: these commands rewrite text, and the mark a yank
// or a buffer jump leaves behind can be anywhere. Emacs disables C-x C-u by
// default for the same reason - pressed by accident, it changes text the user
// never looked at. Like requireMark, it refuses through the echo area: asking
// for a region with none is a slip, not a failure.
func requireRegion(e Env) bool {
	b := e.Buf()
	switch {
	case !b.HasMark():
		e.Echo(noRegion)
		return false
	case !b.MarkActive():
		e.Echo(regionInactive)
		return false
	}
	return true
}

func upcaseRegion(e Env) error   { return caseRegion(e, strings.ToUpper) }
func downcaseRegion(e Env) error { return caseRegion(e, strings.ToLower) }

// caseRegion rewrites the region's text as fn makes it, a line at a time and
// only where that changes anything, as one undo step. Point and mark stay at
// the region's two ends, around the same text.
func caseRegion(e Env, fn func(string) string) error {
	if !requireRegion(e) {
		return nil
	}
	b := e.Buf()
	lo, hi := region(e)
	var spans []lineSpan
	for ln := lo.Line; ln <= hi.Line; ln++ {
		from, to := text.RuneIdx(0), b.Line(ln).Len()
		if ln == lo.Line {
			from = lo.Col
		}
		if ln == hi.Line {
			to = hi.Col
		}
		old := b.Line(ln).View()[from:to]
		if now := []rune(fn(string(old))); !slices.Equal(now, old) {
			spans = append(spans, lineSpan{line: ln, from: from, to: to, text: now})
		}
	}
	return rewriteSpans(e, spans)
}
