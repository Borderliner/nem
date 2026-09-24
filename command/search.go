package command

// Search, replace, and the help commands.
//
// The help prefix is <f1>, not C-h. tcell's legacy input path cannot
// distinguish C-h from Backspace — it folds both 0x08 and 0x7F into
// KeyBackspace — so C-h is bound alongside <f1> but only reaches nem on
// terminals that negotiate the extended keyboard protocol. That reasoning is
// settled in the design spec; do not relitigate it here.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
)

// RegisterSearch adds the search, replace and help commands to r.
//
// It takes the registry rather than reading commands through Env because
// describe-key must report a command's documentation, and Env deliberately
// exposes no way to retrieve it: Env is what a command may touch at run time,
// while the registry is the table itself.
func RegisterSearch(r *Registry) error {
	cmds := []Command{{
		Name:        "isearch-forward",
		Doc:         "Search incrementally forward as you type.",
		Fn:          isearchCmd(false),
		Interactive: true,
	}, {
		Name:        "isearch-backward",
		Doc:         "Search incrementally backward as you type.",
		Fn:          isearchCmd(true),
		Interactive: true,
	}, {
		Name:        "query-replace",
		Doc:         "Replace occurrences of a string, asking about each one.",
		Fn:          queryReplace,
		Interactive: true,
	}, {
		Name:        "execute-extended-command",
		Doc:         "Read a command name in the minibuffer and run it.",
		Fn:          executeExtendedCommand,
		Interactive: true,
	}, {
		Name:        "describe-bindings",
		Doc:         "Show every key binding in a help buffer.",
		Fn:          describeBindings,
		Interactive: true,
	}, {
		Name:        "describe-key",
		Doc:         "Read a key sequence and report the command it runs.",
		Fn:          describeKey(r),
		Interactive: true,
	}}
	for _, c := range cmds {
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// --- search primitives ---------------------------------------------------

// FoldCase reports whether pat should match case-insensitively.
//
// This is emacs's smart case: a pattern is folded while it is entirely
// lowercase, and becomes case-sensitive as soon as it contains an uppercase
// letter. So "hello" finds "Hello", but "Hello" does not find "hello".
func FoldCase(pat string) bool {
	for _, r := range pat {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// SearchForward finds the first occurrence of pat starting at or after from.
// It reports the match's bounds, and ok is false when there is none.
//
// A pattern containing a newline never matches: v1 searches within single
// lines only.
func SearchForward(b *text.Buffer, pat string, from text.Pos, fold bool) (start, end text.Pos, ok bool) {
	needle := foldedNeedle(pat, fold)
	if len(needle) == 0 || strings.ContainsRune(pat, '\n') {
		return from, from, false
	}
	from = b.ClampPos(from)
	for ln := from.Line; ln < b.NumLines(); ln++ {
		hay := b.Line(ln).View()
		first := 0
		if ln == from.Line {
			first = int(from.Col)
		}
		for i := first; i+len(needle) <= len(hay); i++ {
			if foldIf(hay[i], fold) == needle[0] && matchAt(hay, needle, i, fold) {
				return text.Pos{Line: ln, Col: text.RuneIdx(i)},
					text.Pos{Line: ln, Col: text.RuneIdx(i + len(needle))}, true
			}
		}
	}
	return from, from, false
}

// SearchBackward finds the last occurrence of pat beginning strictly before
// from, scanning backward. It reports the match's bounds.
func SearchBackward(b *text.Buffer, pat string, from text.Pos, fold bool) (start, end text.Pos, ok bool) {
	needle := foldedNeedle(pat, fold)
	if len(needle) == 0 || strings.ContainsRune(pat, '\n') {
		return from, from, false
	}
	from = b.ClampPos(from)
	for ln := from.Line; ln >= 0; ln-- {
		hay := b.Line(ln).View()
		// last is one past the greatest start index we may consider.
		last := len(hay) - len(needle) + 1
		if ln == from.Line && int(from.Col) < last {
			last = int(from.Col)
		}
		for i := last - 1; i >= 0; i-- {
			if foldIf(hay[i], fold) == needle[0] && matchAt(hay, needle, i, fold) {
				return text.Pos{Line: ln, Col: text.RuneIdx(i)},
					text.Pos{Line: ln, Col: text.RuneIdx(i + len(needle))}, true
			}
		}
	}
	return from, from, false
}

// matchAt reports whether needle, already folded when fold is set, occurs in
// hay at index i.
func matchAt(hay, needle []rune, i int, fold bool) bool {
	for j, want := range needle {
		if foldIf(hay[i+j], fold) != want {
			return false
		}
	}
	return true
}

// foldedNeedle is the pattern as runes, lower-cased once when the search
// ignores case - rather than again at every position of every line.
func foldedNeedle(pat string, fold bool) []rune {
	needle := []rune(pat)
	if fold {
		for i, r := range needle {
			needle[i] = foldRune(r)
		}
	}
	return needle
}

// foldIf is foldRune when fold is set, and r unchanged otherwise.
func foldIf(r rune, fold bool) rune {
	if fold {
		return foldRune(r)
	}
	return r
}

// foldRune is unicode.ToLower with ASCII handled inline: a search that finds
// nothing folds every rune of every line it scans.
func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}
	return unicode.ToLower(r)
}

// --- incremental search --------------------------------------------------

// Isearch is one incremental search session.
//
// It exists as an exported type because the session outlives any single call:
// the minibuffer keymap in the editor binds C-s and C-r while the prompt is
// open, and advancing to the next match is a property of the session, not of
// the pattern. Update is the ReadOpts.OnChange hook; Advance is what a
// repeated C-s calls; Abandon is the C-g path.
//
// Every search runs from the position point held when the session opened, so
// shortening the pattern walks point back toward that origin rather than
// leaving it stranded at a match the shorter pattern no longer justifies.
type Isearch struct {
	e        Env
	backward bool

	// origin is where point was when the session opened, and where Abandon
	// returns it. lastGood is the most recent successful match, where point
	// waits out a failing pattern.
	origin   text.Pos
	lastGood text.Pos

	pat string

	// skip is how many matches to step past, incremented by Advance. A
	// pattern edit resets it, because the match numbering has changed.
	skip int
}

// NewIsearch opens a session searching forward, or backward when backward is
// set, from the active window's current point.
func NewIsearch(e Env, backward bool) *Isearch {
	p := e.Win().Pt
	return &Isearch{e: e, backward: backward, origin: p, lastGood: p}
}

// Update re-runs the search for a changed pattern and moves point to the
// match. It is ReadOpts.OnChange.
func (s *Isearch) Update(pat string) {
	s.pat = pat
	s.skip = 0
	s.run()
}

// Advance steps to the next match in the search direction, as a repeated C-s
// does inside the prompt. It stays put when there is no further match.
func (s *Isearch) Advance() {
	if s.pat == "" {
		return
	}
	s.skip++
	if p, ok := s.find(); ok {
		s.e.Win().Pt = p
		s.lastGood = p
		return
	}
	// Probing with find rather than run means overshooting the last match
	// reports itself plainly instead of echoing a spurious search failure.
	s.skip--
	s.e.Echo("No further match")
}

// Abandon restores point to where the session opened. This is C-g, and it is
// the behaviour users rely on most.
func (s *Isearch) Abandon() {
	s.e.Win().Pt = s.origin
}

// Pattern returns the pattern currently being searched for.
func (s *Isearch) Pattern() string { return s.pat }

// find locates where point should go for the current pattern and skip count,
// without touching the editor. Keeping it free of side effects is what lets
// Advance probe for a further match before committing to one.
//
// Every search restarts from the origin, which is why shortening the pattern
// walks point back rather than leaving it stranded.
func (s *Isearch) find() (text.Pos, bool) {
	fold := FoldCase(s.pat)
	b := s.e.Buf()
	from := s.origin

	var start, end text.Pos
	for i := 0; i <= s.skip; i++ {
		var ok bool
		if s.backward {
			start, end, ok = SearchBackward(b, s.pat, from, fold)
			if !ok {
				return text.Pos{}, false
			}
			from = start
		} else {
			start, end, ok = SearchForward(b, s.pat, from, fold)
			if !ok {
				return text.Pos{}, false
			}
			from = text.Pos{Line: start.Line, Col: start.Col + 1}
		}
	}

	// Forward search leaves point after the match, backward search before it,
	// so that continuing in either direction moves away from it. This is what
	// emacs does.
	if s.backward {
		return start, true
	}
	return end, true
}

// run performs the search and positions point, reporting whether it matched.
func (s *Isearch) run() bool {
	if s.pat == "" {
		s.e.Win().Pt = s.origin
		s.lastGood = s.origin
		return true
	}
	p, ok := s.find()
	if !ok {
		s.e.Win().Pt = s.lastGood
		s.e.Echo("Failing I-search: %s", s.pat)
		return false
	}
	s.e.Win().Pt = p
	s.lastGood = p
	return true
}

func isearchCmd(backward bool) Func {
	return func(e Env) error {
		s := NewIsearch(e, backward)
		prompt := "I-search: "
		if backward {
			prompt = "I-search backward: "
		}
		// The session is handed over rather than closed over: the minibuffer
		// drives Update on every edit and Advance when C-s is pressed again
		// inside the prompt. Advancing cannot happen here, because this call
		// blocks until the prompt closes.
		if _, err := e.ReadString(ReadOpts{Prompt: prompt, Session: s}); err != nil {
			if errors.Is(err, ErrQuit) {
				s.Abandon()
				e.Echo("Quit")
			}
			return err
		}
		return nil
	}
}

// --- query-replace -------------------------------------------------------

var queryReplaceAnswers = []rune{'y', 'n', '!', 'q'}

func queryReplace(e Env) error {
	from, err := e.ReadString(ReadOpts{Prompt: "Query replace: "})
	if err != nil {
		return err
	}
	if from == "" {
		e.Echo("Nothing to replace")
		return nil
	}
	to, err := e.ReadString(ReadOpts{Prompt: fmt.Sprintf("Query replace %s with: ", from)})
	if err != nil {
		return err
	}

	fold := FoldCase(from)
	b := e.Buf()
	at := e.Win().Pt
	all, n := false, 0

	for {
		start, end, ok := SearchForward(b, from, at, fold)
		if !ok {
			break
		}
		e.Win().Pt = start

		replace := all
		if !all {
			c, err := e.ReadChar(
				fmt.Sprintf("Query replacing %s with %s (y/n/!/q): ", from, to),
				queryReplaceAnswers)
			if err != nil {
				return err
			}
			switch c {
			case 'y':
				replace = true
			case 'n':
				replace = false
			case '!':
				replace, all = true, true
			case 'q':
				e.Echo("Replaced %d occurrences", n)
				return nil
			}
		}

		if replace {
			if err := b.Delete(start, end); err != nil {
				return err
			}
			if to != "" {
				if err := b.Insert(start, []rune(to)); err != nil {
					return err
				}
			}
			n++
			// Resume after the text just inserted. Without this, replacing
			// "a" with "aa" would find what it had written and never finish.
			at = posAfter(start, to)
		} else {
			at = end
		}
		e.Win().Pt = at
	}

	e.Echo("Replaced %d occurrences", n)
	return nil
}

// posAfter returns the position just past s, had s been inserted at p.
func posAfter(p text.Pos, s string) text.Pos {
	if s == "" {
		return p
	}
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		tail := []rune(s[i+1:])
		return text.Pos{Line: p.Line + strings.Count(s, "\n"), Col: text.RuneIdx(len(tail))}
	}
	return text.Pos{Line: p.Line, Col: p.Col + text.RuneIdx(len([]rune(s)))}
}

// --- M-x -----------------------------------------------------------------

// CompleteFrom returns a CompleteFunc offering every one of names.
//
// It does not filter. The minibuffer ranks candidates with fuzzy matching, so
// narrowing here would defeat it: typing "fwc" for forward-char has no prefix
// match, and a pre-filtered list would come back empty.
func CompleteFrom(names []string) CompleteFunc {
	return func(string) []string {
		return append([]string(nil), names...)
	}
}

func executeExtendedCommand(e Env) error {
	name, err := e.ReadString(ReadOpts{
		Prompt:       "M-x ",
		Complete:     CompleteFrom(e.CommandNames()),
		RequireMatch: true,
	})
	if err != nil {
		return err
	}
	if name = strings.TrimSpace(name); name == "" {
		return nil
	}
	// The universal argument reaches the invoked command for free: Run passes
	// this same Env through, and Arg reads from it.
	if err := e.Run(name); err != nil {
		if errors.Is(err, ErrUnknownCommand) {
			e.Echo("No match")
			return nil
		}
		return err
	}
	return nil
}

// --- help ----------------------------------------------------------------

const bindingsBufferName = "*Bindings*"

func describeBindings(e Env) error {
	bindings := e.Bindings()
	specs := make([]string, 0, len(bindings))
	for spec := range bindings {
		specs = append(specs, spec)
	}
	sort.Strings(specs)

	width := len("Key")
	for _, s := range specs {
		if len(s) > width {
			width = len(s)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%-*s  %s\n", width, "Key", "Command")
	fmt.Fprintf(&sb, "%-*s  %s\n", width, strings.Repeat("-", width), strings.Repeat("-", len("Command")))
	for _, spec := range specs {
		fmt.Fprintf(&sb, "%-*s  %s\n", width, spec, bindings[spec])
	}

	b := e.NewBuffer(bindingsBufferName)
	if err := b.Insert(text.Pos{}, []rune(sb.String())); err != nil {
		return err
	}
	b.BreakUndo()
	b.SetModified(false)
	e.Win().Visit(b)
	return nil
}

func describeKey(r *Registry) Func {
	return func(e Env) error {
		bindings := e.Bindings()
		var seq []keymap.Key
		for {
			k, err := e.ReadKey("Describe key: ")
			if err != nil {
				return err
			}
			seq = append(seq, k)
			spec := keymap.SpecString(seq)

			if name, bound := bindings[spec]; bound {
				if c, known := r.Lookup(name); known && c.Doc != "" {
					e.Echo("%s runs %s: %s", spec, name, c.Doc)
				} else {
					e.Echo("%s runs %s", spec, name)
				}
				return nil
			}
			// Keep reading while what we have is only the start of a longer
			// binding, so C-x C-f reports find-file rather than "C-x is
			// undefined".
			if !isBindingPrefix(bindings, spec) {
				e.Echo("%s is undefined", spec)
				return nil
			}
		}
	}
}

func isBindingPrefix(bindings map[string]string, spec string) bool {
	p := spec + " "
	for s := range bindings {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
