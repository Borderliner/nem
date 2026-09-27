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
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/Borderliner/nem/view"
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
	a, lo, up := anchor(needle, fold)
	for ln := from.Line; ln < b.NumLines(); ln++ {
		hay := b.Line(ln).View()
		first := 0
		if ln == from.Line {
			first = int(from.Col)
		}
		for i := first + a; i+len(needle)-a <= len(hay); i++ {
			// The anchor's two cases compared as they are, and the fold
			// only for a rune that is one of them: a search that finds
			// nothing looks at every rune of every line.
			if r := hay[i]; lo < 0 {
				if foldIf(r, fold) != needle[0] {
					continue
				}
			} else if r != lo && r != up {
				continue
			}
			if matchAt(hay, needle, i-a, fold) {
				return text.Pos{Line: ln, Col: text.RuneIdx(i - a)},
					text.Pos{Line: ln, Col: text.RuneIdx(i - a + len(needle))}, true
			}
		}
	}
	return from, from, false
}

// commonness ranks ASCII by how often it turns up in code and prose, most
// often first; a rune not in it is rarer than all of them.
const commonness = " etaoinsrlcdhupmfgbywvkxjqz_.,;:()[]{}=\"'0123456789"

// anchor picks the rune of needle a search scans for - its rarest, so the
// scan stops as seldom as can be - and the two runes that are it: its lower
// and upper case when case is ignored, itself twice when not. A needle with
// no ASCII in it is anchored on its first rune, which the scan folds each
// rune to compare with; lo is then -1.
func anchor(needle []rune, fold bool) (at int, lo, up rune) {
	best := -1
	for i, r := range needle {
		if r >= utf8.RuneSelf {
			continue
		}
		rank := strings.IndexByte(commonness, byte(foldRune(r)))
		if rank < 0 {
			rank = len(commonness)
		}
		if best < 0 || rank > best {
			at, best = i, rank
		}
	}
	if best < 0 {
		return 0, -1, -1
	}
	lo, up = needle[at], needle[at]
	if fold && 'a' <= lo && lo <= 'z' {
		up = lo - 'a' + 'A'
	}
	return at, lo, up
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
// open, and stepping to the next match is a property of the session, not of
// the pattern. Update is the ReadOpts.OnChange hook; Step is what C-s and C-r
// call; Abandon is the C-g path.
//
// It behaves as emacs's does. A step goes on from the current match, in the
// direction of the key - so C-r in a forward search turns round. At the last
// match a step fails, and says so; the next one wraps round the end of the
// buffer and carries on from the other end. Typing more of the pattern
// extends the current match where it can; any other edit searches again from
// where the session opened, so shortening the pattern walks point back toward
// that origin rather than leaving it stranded at a match the shorter pattern
// no longer justifies.
type Isearch struct {
	e        Env
	backward bool

	// win and buf are the window and buffer the session searches. A command
	// run at the prompt can switch the window to another buffer, or make
	// another window current; positions in buf mean nothing there, so the
	// session moves point only while win still shows buf.
	win *view.Window
	buf *text.Buffer

	// origin is where point was when the session opened, and where Abandon
	// returns it. lastGood is the most recent successful match, where point
	// waits out a failing pattern.
	origin   text.Pos
	lastGood text.Pos

	pat string

	// at is where the current match starts, which steps go on from; matched
	// reports there is one.
	at      text.Pos
	matched bool

	// failing reports that the pattern, as it stands, is not found - or that
	// a step found nothing further. edge is the second of those: the next
	// step wraps. wrapped reports that the search has gone round the end of
	// the buffer, so a search from the origin may too.
	failing bool
	edge    bool
	wrapped bool
}

// NewIsearch opens a session searching forward, or backward when backward is
// set, from the active window's current point.
func NewIsearch(e Env, backward bool) *Isearch {
	w := e.Win()
	return &Isearch{e: e, backward: backward, win: w, buf: w.Buf, origin: w.Pt, lastGood: w.Pt}
}

// goTo moves point to p, if the session's window still shows its buffer.
func (s *Isearch) goTo(p text.Pos) {
	if s.win.Buf == s.buf {
		s.win.Pt = s.buf.ClampPos(p)
	}
}

// Update re-runs the search for a changed pattern and moves point to the
// match. It is ReadOpts.OnChange.
func (s *Isearch) Update(pat string) {
	extends := s.matched && s.pat != "" && strings.HasPrefix(pat, s.pat)
	s.pat, s.edge = pat, false
	if pat == "" {
		s.goTo(s.origin)
		s.lastGood = s.origin
		s.matched, s.failing = false, false
		return
	}
	// More typed: the match grows where it is, if it can. Anything else: the
	// search starts again from the origin.
	from := s.origin
	if extends {
		from = s.at
		if s.backward {
			from = text.Pos{Line: s.at.Line, Col: s.at.Col + 1}
		}
	}
	start, end, ok := s.search(from, s.backward)
	if !ok && s.wrapped {
		start, end, ok = s.search(s.bufferEdge(s.backward), s.backward)
	}
	if !ok {
		s.fail()
		return
	}
	s.land(start, end)
}

// Step goes to the next match forward, or backward, from the current one:
// C-s and C-r inside the prompt. Where there is none it fails, and the step
// after that wraps round the end of the buffer.
func (s *Isearch) Step(backward bool) {
	if s.pat == "" {
		return
	}
	if backward != s.backward {
		// Turning round starts from the current match, and forgets that the
		// other direction had run out.
		s.backward, s.edge = backward, false
	}
	from := s.lastGood
	if s.matched {
		from = s.at
		if !backward {
			from.Col++ // past this match's start, so the next may overlap it
		}
	}
	start, end, ok := s.search(from, backward)
	if !ok && s.edge {
		start, end, ok = s.search(s.bufferEdge(backward), backward)
		if ok {
			s.wrapped = true
		}
	}
	if !ok {
		s.edge = true
		s.fail()
		return
	}
	s.edge = false
	s.land(start, end)
}

// Advance steps on in the direction the search was opened in, as a repeated
// C-s does in a forward search.
func (s *Isearch) Advance() { s.Step(s.backward) }

// Abandon restores point to where the session opened. This is C-g, and it is
// the behaviour users rely on most.
func (s *Isearch) Abandon() {
	s.goTo(s.origin)
}

// Pattern returns the pattern currently being searched for.
func (s *Isearch) Pattern() string { return s.pat }

// Prompt is the search's prompt as it stands, in emacs's words: "I-search:",
// with "Failing" or "Wrapped" in front when that is how it is going and
// "backward" after when it is.
func (s *Isearch) Prompt() string {
	p := "I-search"
	if s.backward {
		p += " backward"
	}
	switch {
	case s.failing && s.wrapped:
		p = "Failing wrapped " + p
	case s.failing:
		p = "Failing " + p
	case s.wrapped:
		p = "Wrapped " + p
	}
	return p + ": "
}

// search finds the first match from from in the direction given.
func (s *Isearch) search(from text.Pos, backward bool) (start, end text.Pos, ok bool) {
	fold := FoldCase(s.pat)
	if backward {
		return SearchBackward(s.buf, s.pat, from, fold)
	}
	return SearchForward(s.buf, s.pat, from, fold)
}

// bufferEdge is where a wrapped search starts: the top of the buffer going
// forward, the bottom going back.
func (s *Isearch) bufferEdge(backward bool) text.Pos {
	if backward {
		return s.buf.End()
	}
	return text.Pos{}
}

// land makes the match between start and end the current one. A forward
// search leaves point after it and a backward one before it, so that going
// on in either direction moves away from it, as emacs does.
func (s *Isearch) land(start, end text.Pos) {
	s.at, s.matched, s.failing = start, true, false
	p := end
	if s.backward {
		p = start
	}
	s.goTo(p)
	s.lastGood = p
}

// fail marks the search as having no match, leaving point at the last one it
// had. The prompt says so while it is open, and the echo area after.
func (s *Isearch) fail() {
	s.failing = true
	s.goTo(s.lastGood)
	s.e.Echo("Failing I-search: %s", s.pat)
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
		if _, err := e.ReadString(ReadOpts{Prompt: prompt, Session: s, History: "search"}); err != nil {
			if errors.Is(err, ErrQuit) {
				s.Abandon()
				e.Echo("Quit")
			}
			return err
		}
		if s.failing {
			return ErrSearchFailed
		}
		// Where the search started is kept, as emacs keeps it, so C-u C-SPC
		// returns there.
		if s.win.Buf == s.buf && s.win.Pt != s.origin {
			s.buf.SetMark(s.origin)
			e.Echo("Mark saved where search started")
		}
		return nil
	}
}

// --- query-replace -------------------------------------------------------

func queryReplace(e Env) error {
	if e.Buf().ReadOnly() {
		return text.ErrReadOnly
	}
	from, err := e.ReadString(ReadOpts{Prompt: "Query replace: ", History: "replace"})
	if err != nil {
		return err
	}
	if from == "" {
		e.Echo("Nothing to replace")
		return nil
	}
	to, err := e.ReadString(ReadOpts{Prompt: fmt.Sprintf("Query replace %s with: ", from), History: "replace"})
	if err != nil {
		return err
	}

	return replaceMatches(e, replaceRun{
		find: literalMatches(from, to),
		ask:  fmt.Sprintf("Query replacing %s with %s (y/n/!/q): ", from, to),
		from: e.Win().Pt,
	})
}

// literalMatches finds from, with emacs's smart case, to be replaced with
// to as it stands.
func literalMatches(from, to string) func(*text.Buffer, text.Pos) (match, bool) {
	fold := FoldCase(from)
	return func(b *text.Buffer, at text.Pos) (match, bool) {
		start, end, ok := SearchForward(b, from, at, fold)
		return match{start: start, end: end, to: to}, ok
	}
}

// CanReplace reports whether b would let the text between start and end be
// replaced with to.
func CanReplace(b *text.Buffer, start, end text.Pos, to string) bool {
	return b.Vet(start, end, nil) == nil && (to == "" || b.Vet(start, start, []rune(to)) == nil)
}

// ReplaceMatch replaces the text between start and end with to, as
// query-replace does, and returns where searching goes on from: just past the
// replacement. Without that, replacing "a" with "aa" would find what it had
// written and never finish. A replacement the buffer refuses leaves the text
// as it was rather than half replaced.
func ReplaceMatch(b *text.Buffer, start, end text.Pos, to string) (text.Pos, error) {
	was := b.Text(start, end)
	if err := b.Delete(start, end); err != nil {
		return start, err
	}
	if to != "" {
		if err := b.Insert(start, []rune(to)); err != nil {
			_ = b.Insert(start, was)
			return start, err
		}
	}
	return posAfter(start, to), nil
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

// keysOf annotates a command with the keys that run it, for M-x: as many as
// two, shortest first, from wherever M-x was opened - so in a directory
// listing dired's own keys are the ones shown. The bindings are read once for
// the prompt, not once per candidate per frame.
func keysOf(e Env) func(string) string {
	byCmd := map[string][]string{}
	for spec, cmd := range e.Bindings() {
		byCmd[cmd] = append(byCmd[cmd], spec)
	}
	for _, specs := range byCmd {
		slices.SortFunc(specs, func(a, b string) int {
			if len(a) != len(b) {
				return len(a) - len(b)
			}
			return strings.Compare(a, b)
		})
	}
	return func(cmd string) string {
		specs := byCmd[cmd]
		if len(specs) > 2 {
			specs = specs[:2]
		}
		return strings.Join(specs, ", ")
	}
}

func executeExtendedCommand(e Env) error {
	name, err := e.ReadString(ReadOpts{
		Prompt:       "M-x ",
		Complete:     CompleteFrom(e.CommandNames()),
		Annotate:     keysOf(e),
		RequireMatch: true,
		History:      "command",
		HistoryFirst: true,
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
