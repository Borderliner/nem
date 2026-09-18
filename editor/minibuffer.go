package editor

import (
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/hajianpour/nem/command"
	"github.com/hajianpour/nem/keymap"
	"github.com/hajianpour/nem/text"
	"github.com/hajianpour/nem/view"
)

// The minibuffer is a real text.Buffer shown in a real view.Window, exactly as
// it is in emacs, and it is deliberately NOT in the split tree — it is a single
// row pinned to the bottom of the screen, which is why the tree is laid out in
// h-1 rows.
//
// The payoff is that a prompt needs no editing implementation of its own. While
// a prompt is open, Editor.Win reports the prompt's window, so move-end-of-line,
// kill-line, backward-word and yank all act on the prompt because they act on
// whatever Win reports. The prompt's keymap binds only accept, abort, complete
// and search-advance; everything else falls through to the global map.

// Prompt-control names. They are not registry commands: accepting or abandoning
// a prompt is a property of the prompt, not an editor-wide operation, and
// registering them would put four commands in M-x that do nothing outside one.
const (
	miniAccept     = "minibuffer-accept"
	miniAbort      = "minibuffer-abort"
	miniComplete   = "minibuffer-complete"
	miniSearchFwd  = "minibuffer-isearch-forward"
	miniSearchBack = "minibuffer-isearch-backward"
)

// miniState is one active prompt.
type miniState struct {
	prompt string
	buf    *text.Buffer
	win    *view.Window
	opts   command.ReadOpts
	keys   *keymap.Map

	// search is non-nil while this prompt is driving an incremental search.
	search *command.Isearch

	// last is the contents as of the most recent OnChange, so a change can be
	// detected however it was made — typed, backspaced, killed or yanked.
	last string

	done  bool
	abort bool
}

// miniKeymap binds only what belongs to a prompt. Everything it omits falls
// through to the global map, which is the whole point.
func miniKeymap() *keymap.Map {
	m := keymap.New()
	for _, b := range []struct{ spec, cmd string }{
		{"RET", miniAccept},
		{"C-g", miniAbort},
		{"TAB", miniComplete},
		{"C-s", miniSearchFwd},
		{"C-r", miniSearchBack},
	} {
		if err := bindSpec(m, b.spec, b.cmd); err != nil {
			panic("editor: bad minibuffer binding: " + err.Error()) // a build-time constant is wrong
		}
	}
	return m
}

// line renders the prompt and its contents for the echo row.
func (ms *miniState) line() string { return ms.prompt + ms.buf.String() }

// cursorCol is the display column of point within the echo row, counted from
// the screen's left edge, so the prompt's own width is included.
func (ms *miniState) cursorCol() text.ColIdx {
	pt := ms.buf.ClampPos(ms.win.Pt)
	return text.ColIdx(len([]rune(ms.prompt))) + ms.buf.Line(pt.Line).DisplayCol(pt.Col)
}

// contents returns what the user has typed.
func (ms *miniState) contents() string { return ms.buf.String() }

// ReadString prompts in the minibuffer and returns what was typed, or ErrQuit
// if the user pressed C-g.
//
// It enters a nested event loop rather than returning a continuation, which is
// what lets a prompting command read as straight-line code: find-file is
// ReadString then OpenFile then Visit, not a three-state machine.
//
// # Why the editor owns the incremental-search session
//
// A repeated C-s inside the prompt must advance to the next match, and the spec
// puts that in the prompt's keymap. But ReadOpts cannot carry the session:
// OnChange is bound to Isearch.Update, and Advance is not recoverable from that
// closure. So the editor infers from the command name that a search is starting
// (see noteIsearch) and creates its own session here, before the prompt opens,
// so that the session's origin is the text window's point.
//
// The consequence, which is worth knowing: for a search prompt the caller's
// OnChange is not invoked, because this session drives the search instead. The
// two sessions share an origin and only one is ever asked to move point, so the
// behaviour is identical — but ReadOpts growing a field for the session, or for
// an OnAdvance callback, would remove the inference and the duplication both.
func (e *Editor) ReadString(opts command.ReadOpts) (string, error) {
	if e.scr == nil {
		return "", command.ErrQuit // nothing to prompt on
	}
	if e.miniDepth >= maxMiniDepth {
		return "", ErrTooDeep
	}

	buf := text.NewBuffer()
	if opts.Initial != "" {
		if err := buf.Insert(text.Pos{}, []rune(opts.Initial)); err != nil {
			return "", err
		}
	}
	buf.BreakUndo()
	win := view.NewWindow(buf)
	win.Pt = buf.End()

	ms := &miniState{
		prompt: opts.Prompt,
		buf:    buf,
		win:    win,
		opts:   opts,
		keys:   miniKeymap(),
		last:   buf.String(),
	}
	// Created while Win still reports the text window, so the session's origin
	// is point in the buffer being searched.
	if e.wantSearch != nil {
		ms.search = command.NewIsearch(e, *e.wantSearch)
		e.wantSearch = nil
	}

	// The echo area is deliberately NOT saved and restored. A prompt's own text
	// lives in ms.line(), not in e.echo, so there is nothing of the prompt's to
	// clean up — and a message the prompt produced, such as a failing
	// incremental search, must survive the prompt closing.
	savedMini, savedPending := e.mini, e.pending
	e.mini, e.pending = ms, nil
	e.miniDepth++
	defer func() {
		e.mini, e.pending = savedMini, savedPending
		e.miniDepth--
	}()

	e.readLoop(ms)

	if ms.abort || e.quit {
		return "", command.ErrQuit
	}
	return ms.contents(), nil
}

// readLoop is the nested event loop a prompt runs in.
func (e *Editor) readLoop(ms *miniState) {
	e.Redraw()
	for !ms.done && !e.quit {
		ev := e.scr.PollEvent()
		if ev == nil {
			ms.abort = true
			return
		}
		e.HandleEvent(ev)
		e.Redraw()
	}
}

// control handles a prompt-control key.
func (ms *miniState) control(e *Editor, name string) {
	switch name {
	case miniAccept:
		ms.done = true
	case miniAbort:
		ms.done, ms.abort = true, true
		if ms.search != nil {
			e.withTextWindow(func() { ms.search.Abandon() })
		}
	case miniComplete:
		ms.complete(e)
	case miniSearchFwd, miniSearchBack:
		if ms.search != nil {
			e.withTextWindow(func() { ms.search.Advance() })
		}
	}
}

// complete extends the contents to the candidates' common prefix, and lists
// them when that is ambiguous.
func (ms *miniState) complete(e *Editor) {
	if ms.opts.Complete == nil {
		return
	}
	cur := ms.contents()
	cands := ms.opts.Complete(cur)
	switch len(cands) {
	case 0:
		e.Echo("No match")
		return
	case 1:
		ms.replace(e, cands[0])
		return
	}
	if pre := commonPrefix(cands); len(pre) > len(cur) {
		ms.replace(e, pre)
		return
	}
	sorted := append([]string(nil), cands...)
	sort.Strings(sorted)
	e.Echo("%s", strings.Join(sorted, " "))
}

// replace sets the prompt's contents, leaving point at the end.
func (ms *miniState) replace(e *Editor, s string) {
	if err := ms.buf.Delete(text.Pos{}, ms.buf.End()); err != nil {
		return
	}
	if s != "" {
		if err := ms.buf.Insert(text.Pos{}, []rune(s)); err != nil {
			return
		}
	}
	ms.win.Pt = ms.buf.End()
	e.afterMiniEdit()
}

// commonPrefix returns the longest prefix shared by every candidate.
func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	pre := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, pre) {
			pre = pre[:len(pre)-1]
			if pre == "" {
				return ""
			}
		}
	}
	return pre
}

// afterMiniEdit fires OnChange when the prompt's contents have changed.
//
// It compares contents rather than watching for insertions, so every route to a
// change is covered: typing, <backspace>, C-k, and yank alike. Shortening is
// what makes an incremental search walk point back toward its origin, and it is
// exactly what a grow-only notification cannot express.
func (e *Editor) afterMiniEdit() {
	ms := e.mini
	if ms == nil {
		return
	}
	cur := ms.contents()
	if cur == ms.last {
		return
	}
	ms.last = cur
	switch {
	case ms.search != nil:
		e.withTextWindow(func() { ms.search.Update(cur) })
	case ms.opts.OnChange != nil:
		e.withTextWindow(func() { ms.opts.OnChange(cur) })
	}
}

// withTextWindow runs fn with Win reporting the text window rather than the
// prompt's.
//
// Search-as-you-type moves point in the buffer being searched, not in the
// prompt, so Isearch.Update and Advance must see the underlying window — while
// C-a and C-k, dispatched ordinarily, must see the prompt. Both needs are real
// and this is the seam between them.
func (e *Editor) withTextWindow(fn func()) {
	saved := e.miniTransparent
	e.miniTransparent = true
	defer func() { e.miniTransparent = saved }()
	fn()
}

// ReadChar prompts for a single keystroke, accepting only the runes in valid.
//
// query-replace's y/n/!/q loop is built from this: a ReadString would force the
// user to press RET after every answer.
func (e *Editor) ReadChar(prompt string, valid []rune) (rune, error) {
	if e.scr == nil {
		return 0, command.ErrQuit
	}
	if e.miniDepth >= maxMiniDepth {
		return 0, ErrTooDeep
	}
	e.miniDepth++
	savedEcho := e.echo
	defer func() { e.miniDepth--; e.echo = savedEcho }()

	for {
		e.Echo("%s", prompt)
		e.Redraw()
		ev := e.scr.PollEvent()
		if ev == nil {
			return 0, command.ErrQuit
		}
		ke, ok := ev.(*tcell.EventKey)
		if !ok {
			e.HandleEvent(ev)
			continue
		}
		k := DecodeKey(ke, e.keys.TreatCtrlHAsBackspace)
		if k.Ctrl && !k.Meta && k.Rune == 'g' {
			return 0, command.ErrQuit
		}
		if k.Special != keymap.SpecialNone || k.Ctrl || k.Meta || k.Rune == 0 {
			continue
		}
		if len(valid) == 0 || containsRune(valid, k.Rune) {
			return k.Rune, nil
		}
		// An unaccepted answer re-prompts rather than aborting, so a stray
		// keystroke cannot silently cancel a replace the user is halfway
		// through.
	}
}

// ReadKey prompts for one raw keystroke, for describe-key.
func (e *Editor) ReadKey(prompt string) (keymap.Key, error) {
	if e.scr == nil {
		return keymap.Key{}, command.ErrQuit
	}
	if e.miniDepth >= maxMiniDepth {
		return keymap.Key{}, ErrTooDeep
	}
	e.miniDepth++
	savedEcho := e.echo
	defer func() { e.miniDepth--; e.echo = savedEcho }()

	for {
		e.Echo("%s", prompt)
		e.Redraw()
		ev := e.scr.PollEvent()
		if ev == nil {
			return keymap.Key{}, command.ErrQuit
		}
		ke, ok := ev.(*tcell.EventKey)
		if !ok {
			e.HandleEvent(ev)
			continue
		}
		if k := DecodeKey(ke, e.keys.TreatCtrlHAsBackspace); k != (keymap.Key{}) {
			return k, nil
		}
	}
}

func containsRune(rs []rune, r rune) bool {
	for _, c := range rs {
		if c == r {
			return true
		}
	}
	return false
}
