# nem — design

A terminal text editor with nano's shape and emacs's keybindings.

Status: approved 2026-09-18. Section 1 approved in conversation; Sections 2–4
written against the same settled decisions.

## Settled decisions

| Decision | Choice | Why |
|---|---|---|
| Language | Go 1.27 | Windows sharing buffers is a pointer graph; GC removes the one structural problem the chosen scope guarantees. |
| Renderer | tcell engine + Lip Gloss chrome | Real hardware cursor, exact key events, free damage-diffing, terminfo portability. Lip Gloss for styling only. |
| Scope | Pocket emacs + multi-buffer + window splits | User's call. Prefix keymaps, `M-x`, `C-u`, minibuffer, isearch, kill ring, mark, undo. |
| In v1 | Lua config + rebinding; auto-indent + bracket matching | |
| Deferred | Syntax highlighting; mouse support | Render path stays per-cell-styleable so highlighting drops in without a rewrite. |
| Undo | Linear undo/redo | Predictable. `C-/` and `C-_` undo, `M-_` redoes; new edit after undo discards the redo branch. |
| Architecture | Layered packages; minibuffer is a real buffer | Prompts reuse real editing; `M-x`/`C-s`/find-file/query-replace become callers of one mechanism. |
| Scripting | gopher-lua, config-as-script | Pure Go, no cgo, single static binary preserved. No config-format migration later. |

Rejected: Rust (borrow-checker friction on the windows-share-buffers graph, and
`ropey`'s advantage is moot at nano scale); Bubble Tea (fake cursor, key
normalization, Elm ceremony against a mutable buffer); Luau (cgo cost
undermines the single static binary; its sandbox and gradual typing are
calibrated for untrusted third-party scripts at Roblox scale).

## Section 1 — Package layout and the text model

```
nem/
  cmd/nem/main.go      flag parsing, terminal setup/teardown, panic recovery
  text/                buffer, lines, positions, edit ops, undo log
  keymap/              key parsing, prefix tree, binding resolution
  view/                Window (buffer + point + viewport), split tree, layout maths
  command/             named command registry + Env
  ui/                  tcell screen, render, Lip Gloss blitter
  lua/                 gopher-lua host: config loading, API surface
  editor/              wires it all together, owns the event loop
```

Dependency direction is strictly one-way: `text` and `keymap` import nothing of
ours, `view` imports `text`, `command` imports `text` + `keymap` + `view`, `ui`
imports `text` + `view`, `lua` imports `command`, `editor` imports everything,
and nothing imports `editor`. That is what keeps the terminal out of the
testable parts.

`view` exists so that `command` can reach the active window without importing
the renderer. A `Window` is buffer + point + viewport and the split tree is
pure geometry — neither knows what a terminal is, so both stay unit-testable
and `ui` is left holding only tcell and drawing.

### Three coordinate spaces

Conflating these is where editors get their cursor bugs. They are distinct
named types so the compiler catches mixups.

1. **Rune index** (`RuneIdx`) — position within `[]rune`. Storage and editing.
2. **Grapheme cluster boundary** — where the cursor is allowed to stop.
   `é` as `e`+U+0301 is two runes, one stop. 👨‍👩‍👧‍👦 is seven runes, one stop.
3. **Display column** (`ColIdx`) — screen cells. Tab advances to the next
   multiple of 8; CJK and most emoji are 2 cells; combining marks are 0.

`C-f` moves one grapheme. `C-n` preserves a display column. The buffer stores
runes.

### Types

`Line` holds `[]rune` plus a cached display width invalidated on edit.
`Buffer` is a line array (`[]Line`) — not a rope. Nano and micro both do this;
it is fine to multi-megabyte files, and `text` is the only package that changes
if gigabyte files ever matter.

**Point lives in the Window, not the Buffer** — two windows on one buffer need
independent cursors. The buffer keeps a saved point for when no window displays
it, and the mark (the region is per-buffer in emacs).

### Mutation

All mutation goes through exactly two primitives: `Insert(Pos, []rune)` and
`Delete(from, to Pos)`. Every command composes from those, so undo, mark
adjustment, and render invalidation hook in at one place rather than forty.

Undo is a log of those primitives with inverses. Consecutive single-rune
inserts coalesce into one unit, broken by movement, any non-insert command, or
a save.

## Section 2 — Input pipeline

### Event loop

```
tcell event
  → editor.decodeKey() → keymap.Key        (the ONLY place tcell meets keymap)
  → append to pending sequence
  → resolve against the keymap stack
  → Pending:   echo the prefix, wait for the next key
  → Undefined: echo "C-x C-z is undefined", clear pending
  → Found:     registry lookup by name → execute with *Env
```

The keymap stack is consulted innermost-first: **minibuffer map** (only while a
prompt is active) → **buffer-local map** → **global map**.

`decodeKey` is where every terminal quirk is absorbed — `C-SPC` arriving as
NUL, `C-/` arriving as `C-_`, Meta arriving either as Alt or as a separate ESC
event. `keymap` itself stays stdlib-only and therefore unit-testable; the
quirk folds live in `keymap.Normalize` and are applied here.

### Universal argument

`C-u` → 4, `C-u C-u` → 16, `C-u 1 2` → 12, `M-1 M-2` → 12, `C-u -` → negative.
The pending argument lives on the Env and is consumed by the command:
`e.Arg() (n int, explicit bool)`.

### Command registry

```go
type Func func(*Env) error

type Command struct {
    Name        string // "kill-line"
    Doc         string
    Fn          Func
    Interactive bool   // appears in M-x
}

func (r *Registry) Register(Command) error
func (r *Registry) Lookup(name string) (Command, bool)
func (r *Registry) Names() []string // M-x completion
```

One table with three consumers: `M-x` searches it, the Lua config binds against
it, and `C-u` feeds it an argument. Lua-defined commands register into the same
table, so `M-x` finds them with no special casing.

### Env — what a command may touch

```go
func (e *Env) Win() *view.Window     // active window (point lives here)
func (e *Env) Buf() *text.Buffer     // active buffer
func (e *Env) Arg() (int, bool)      // universal argument
func (e *Env) Kill(s string)         // push onto the kill ring
func (e *Env) KillAppend(s string)   // append to the top entry (consecutive C-k)
func (e *Env) Yank() string
func (e *Env) YankPop() (string, error)
func (e *Env) Echo(format string, a ...any)
func (e *Env) ReadString(ReadOpts) (string, error) // nested minibuffer edit
func (e *Env) Buffers() []*text.Buffer
func (e *Env) OpenFile(path string) (*text.Buffer, error)
func (e *Env) SplitWindow(vertical bool)
func (e *Env) OtherWindow(n int)
func (e *Env) DeleteWindow()
func (e *Env) Quit(force bool) error
```

Commands never reach the `Screen` or the layout tree. That boundary is what
keeps commands testable against a headless Env.

### The minibuffer is a recursive edit

`ReadString` enters a **nested event loop** with the minibuffer window active,
returning on `RET` and returning `ErrQuit` on `C-g`. This is how emacs does it,
and it is why commands read as straight-line code:

```go
func findFile(e *command.Env) error {
    path, err := e.ReadString(command.ReadOpts{
        Prompt:   "Find file: ",
        Complete: command.CompleteFile,
    })
    if err != nil { return err }
    buf, err := e.OpenFile(path)
    if err != nil { return err }
    e.Win().Visit(buf)
    return nil
}
```

The alternative — commands returning "I need input, call me back" continuations
— makes every prompting command a state machine. Rejected.

The loop carries a recursion depth guard (max 8) so a misbehaving Lua script
cannot stack prompts forever.

`ReadOpts.OnChange func(string)` is the hook that makes incremental search fall
out of the same mechanism: isearch is `ReadString` with an `OnChange` that
searches and moves point, plus `C-s`/`C-r` bound inside the minibuffer map to
advance the match. `C-g` restores the point saved at entry.

Because the minibuffer is a real buffer in a real window, `C-a`, `C-e`, `C-k`,
`M-b`, and the kill ring all work inside prompts with no extra code.

## Section 3 — Rendering

### The rule that de-risks the blitter

- **Text area → direct tcell, cell by cell.** Hot path, needs exact control,
  and is where per-cell styling will later hang syntax highlighting.
- **Chrome → Lip Gloss through the blitter.** Modeline, minibuffer, dividers,
  hint bar. Cold path, redrawn once per frame at most.

The blitter therefore never touches the hot path. If it proves fragile, only
the chrome is affected.

### Window tree

A binary tree of splits; leaves hold windows. Lives in `view`, not `ui` — it is
geometry, and computing rects needs no terminal.

```go
type Node interface{ isNode() }
type Leaf  struct { Win *Window }
type Split struct {
    Vertical bool    // true = side by side (C-x 3); false = stacked (C-x 2)
    A, B     Node
    Ratio    float64
}
```

Layout walks the tree assigning each leaf a rect. The modeline is the leaf's
last row. **The minibuffer window is not in the tree** — it is a dedicated
single row pinned to the bottom of the screen, as in emacs.

### Frame

1. Walk the tree, assign rects.
2. Per leaf: adjust `top` to keep point visible (scroll margin 2), draw visible
   lines directly to tcell, draw the modeline via the blitter.
3. Draw dividers.
4. Draw the minibuffer or echo message via the blitter.
5. `scr.ShowCursor(x, y)` at point in the active window — the real hardware
   cursor, which is the whole reason tcell won over Bubble Tea.

### Long lines: truncate, do not wrap (v1)

Lines wider than the window are truncated with a `$` continuation marker and
the window scrolls horizontally to follow point. Wrapping makes one buffer line
map to N screen rows, which changes `C-n`/`C-p` semantics, scrolling maths, and
every rect calculation. Deferred, and flagged as user-visible: emacs and nano
both wrap by default, so this will feel different until it lands.

## Section 4 — Lua layer, config, and testing

### API surface

`~/.config/nem/init.lua`, loaded at startup after built-in commands register
and after built-in bindings are installed, so user config overrides cleanly.

```lua
nem.set("tab-width", 4)
nem.set("scroll-margin", 3)

nem.bind("C-x C-f", "find-file")
nem.bind("C-c r", "reverse-line")

nem.command("reverse-line", "Reverse the current line.", function()
  local l = nem.buf.line()
  nem.buf.replace_line(l:reverse())
end)

nem.hook("before-save", function(buf)
  if buf.path:match("%.go$") then nem.run("gofmt-buffer") end
end)
```

`nem.command` registers into the same Go registry, so Lua commands appear in
`M-x` and are bindable exactly like built-ins.

**A script error must never kill the editor.** Config load and every Lua
callback run under `pcall`; failures surface in the echo area with the Lua
traceback and the editor continues with the built-in default.

Scripts receive a narrow `nem` table, never a raw `Env` — that keeps the door
open to swapping in a sandboxed VM later without breaking scripts.

### Testing strategy

| Layer | How | Terminal needed |
|---|---|---|
| `text` | Table-driven unit tests | No |
| `keymap` | Table-driven + `ParseSpec`/`String` round-trip property | No |
| `ui/blit` | `tcell.NewSimulationScreen()`, assert cells and styles | No |
| `command` | Headless fake Env | No |
| `editor` | Integration: scripted key sequences → assert buffer + screen | Simulation only |

The integration harness is the one that catches emacs-behaviour regressions:
feed `C-k C-k C-y`, assert both killed lines came back as one block. Feed
`C-n` through a short line, assert the goal column survived.

## Build order

1. `text`, `keymap`, `ui/blit` — no dependencies on our other code. **Parallel.**
2. `view` — Window, split tree, layout rects. Depends only on `text`.
3. `command` registry + Env + the movement/edit command set.
4. `ui` render pass over the layout rects.
5. `editor` event loop, keymap stack, `decodeKey`, recursive minibuffer edit.
6. `lua` host and config loading.
7. Auto-indent, bracket matching, integration harness.

## Deferred, by explicit decision

Syntax highlighting · mouse support · line wrapping · undo tree · rope buffer ·
plugin ecosystem and sandboxing · language-aware indentation

## Implementation findings

Recorded as the leaf packages landed. These are constraints, not suggestions —
each one was discovered by building or verifying, and each is invisible enough
that it would otherwise be rediscovered the hard way.

### Verified, load-bearing

**All three layers agree on grapheme width by construction.** `x/ansi`,
`cellbuf` and tcell itself all segment with `rivo/uniseg` — tcell 2.13 calls
`uniseg.FirstGraphemeClusterInString` at `cell.go:76`. The entire class of
emoji/CJK off-by-one corruption the blitter spike existed to find cannot occur.

**tcell resolves ESC-vs-Meta itself.** `input.go` sets a 50ms expiry with a 60ms
`AfterFunc` and rewrites an ESC-prefixed key as `ModAlt` (line 108). `decodeKey`
therefore maps `ModAlt → Meta` and no timing logic belongs in our code. This was
the one place the clean `keymap` boundary looked like it would cost us; it does
not. It is also a concrete reason tcell beat the raw-ANSI fallback, which would
have required writing this timer by hand.

**`uniseg.StringWidth("\t")` is 0.** Tab-stop advancement is implemented in
`text`, not inherited from the segmenter.

### Required calls and invariants

**`blit.SyncLipglossProfile(scr)` must be called once after `Screen.Init`, and
again after any screen reinitialisation (suspend/resume).** Lip Gloss decides
how much colour to emit by probing `os.Stdout`, which is meaningless once tcell
owns the terminal — without this the chrome renders unstyled in a way that looks
exactly like a blitter bug. The profile is derived from `scr.Colors()`, since
tcell has already done real terminfo negotiation.

**The text area calls `scr.SetContent` directly and never goes through `blit`.**
Serialising buffer text to ANSI only to parse it back is waste. `blit` earns its
place only where Lip Gloss does real layout work: borders, joins, padding.

**Single-goroutine rule: all Lua execution and all `keymap.Map` mutation happen
on the input goroutine.** `Map` is not safe for concurrent use, and a Lua config
reload racing the input loop would corrupt it silently. Resolved by rule rather
than by lock; `editor` must not move Lua onto another goroutine without also
introducing an atomic Map swap.

**`text.Buffer.Line(i)` returns a pointer into the line slice.** Any edit that
splices lines invalidates it. `ui` must not hold one across an edit.

**`Shift` is only meaningful for special keys.** For rune keys the rune carries
case, so `keymap.Normalize` clears `Shift` when `Special == SpecialNone`. This
makes the round-trip property total rather than conventional.

### Spec corrections made during implementation

**`Modified()` is derived, not stored.** A `bool` cannot clear itself when you
undo back to the saved state. `UndoLog.savedAt` holds the saved position and
`Modified()` is `pos != savedAt`; saving then undoing past the save sets
`savedAt = -1`, correctly reporting permanently-modified because the saved state
has become unreachable.

**A newline breaks undo coalescing**, so typing a paragraph does not collapse
into a single undo unit.

**`keymap.Map.Where(command) []string`** was added alongside `Bindings()`.
`Bindings()` is keyed by spec, which suits `describe-bindings` but forces every
other caller to invert it; `M-x` and help need command → sequences, and a
command legitimately has several (`undo` is both `C-_` and `C-x u`).

**`C-X` folds to `C-x` for all letters.** Ctrl+Shift+letter is indistinguishable
from Ctrl+letter on every VT-lineage terminal.

### Known lossy behaviour

Loading replaces invalid UTF-8 with U+FFFD, so **saving does not preserve the
original bytes**, and a file with mixed line endings normalises to whichever
style appeared first. Both follow from the design, but they can corrupt a binary
file opened by accident. `editor` should gain a binary-file guard before this is
used on anything that matters.

ANSI features that cannot round-trip into tcell cells: **conceal (SGR 8)** —
approximated as fg=bg; **OSC 8 hyperlinks** — dropped, tcell has no per-cell
link concept, worth knowing if clickable paths in a compile buffer ever appeal.

### For the Lua config documentation

**`DEL` means backspace, not forward-delete.** Forward-delete is `<delete>`.
This is emacs-faithful and a guaranteed first-time mistake in a user config, so
it belongs in the user-facing docs rather than only in a code comment.

## Corrections from wave 2

### `Env` is an interface, not a struct

Section 2 wrote `func (e *Env) Win() ...`, implying a struct holding editor
state. That cannot work: the state lives in `editor`, so a struct `Env` in
`command` would force `command` to import `editor`, which imports `command`.

**`Env` is an interface declared in `command` and implemented by `editor`.**
`Func` becomes `func(Env) error`. This also makes Section 4's "headless fake
Env" testing strategy actually possible — commands become pure functions over
an interface, and `command/commandtest` supplies the double.

### tcell's key space: my brief was wrong

I told the decoder agent that `KeyCtrlI == KeyTab`, `KeyCtrlM == KeyEnter` and
similar. **No such collisions exist.** `KeyCtrlSpace = iota + 64` numbers Ctrl'd
keys 64–95 — the ASCII codes of `@`, `A`–`Z`, `[`–`_` — a space entirely
separate from the C0 codes. `KeyCtrlH` is 72; `KeyBackspace` is 8. Verified at
runtime. The only genuine identity is `KeyBackspace2 == KeyDEL == 127`.

Consequence: for constants 64–95 the constant *is* the rune, and the emacs folds
still happen, in `keymap.Normalize` rather than by numeric accident.

**For raw C0 bytes, use `byte | 0x40`, never tcell's `ch + 0x60`.** The latter is
correct only for the 26 letters: at `0x1F` it yields `0x7F` (DEL) where
`byte | 0x40` yields `_`. Since **`C-_` is undo**, tcell's formula silently
breaks it.

### `C-h` is not reliably available, so help lives on `<f1>`

tcell destroys the `C-h` / Backspace distinction in two independent places on the
legacy input path: `input.go:445` is `case '\b', '\x7F':`, and `key.go:296`
folds `KeyBackspace2` into `KeyBackspace`.

tcell *does* request the advanced keyboard protocols when the terminfo entry is
`XTermLike` (`tscreen.go:332` sends modifyOtherKeys + kitty CSI-u +
win32-input-mode), so `C-h` is recoverable on a modern terminal — but not on a
legacy one, and nem cannot depend on it.

**Resolution: `<f1>` is the primary help prefix, with `C-h` bound alongside it.**
F1 is `help-command` in emacs too, so this is faithful rather than a compromise,
and users whose terminals negotiate CSI-u get the muscle memory anyway.

### Layout: dividers occupy real columns

The brief asked for children summing to the parent rect *and* a divider between
them; those contradict. Resolved: the divider lives inside the parent, so the
invariant is `A.W + 1 + B.W == parent.W`, and exact tiling means windows **plus
dividers** cover every cell exactly once. Horizontal splits carry no divider —
the upper window's modeline already separates the panes, as in emacs. Tiling is
therefore deliberately asymmetric between the two directions.

`Tree.Split` validates minimum sizes against the frame size recorded by the last
`Layout`, and permits anything before the first `Layout` (the startup case), so
its behaviour depends on call order. The check gates only the dimension being
divided: stacking panes cannot worsen a pre-existing narrowness.

## Boundary rulings (wave 3)

Decided while building the `command` contract. Each is a boundary someone will
otherwise try to cross, so they are recorded as rulings rather than notes.

**`Env` grew from 16 to 30 methods, and that was correct.** Walking the v1
command inventory against the original interface found eight commands that could
not be written at all. The notable one: `M-y` must delete the text the previous
yank inserted, so it needs that extent — sequencing state is not optional.
Additions covered screenful height (`C-v`/`M-v`/`C-l`), char and key prompts
(`M-%`, `C-x s`, `describe-key`), buffer naming and lifecycle (`C-x b`/`k`/`C-x
C-b`, `*scratch*`), command invocation and completion (`M-x`, and Lua's
`nem.run`), and binding introspection (`describe-bindings`/`describe-key`).

**Cross-command state is a typed `Seq` struct, never an untyped `any`.** `Env`
exposes `Seq() *Seq` returning a mutable pointer. A failed type assertion in an
editor means a panic, and a panic means data loss; a struct field is checked at
compile time and a future sequencing need costs a field rather than an interface
method.

**Lua hooks fire in `editor`, around dispatch, keyed on command name.** There is
no `RunHook` on `Env`. Commands stay ignorant that a scripting layer exists,
which is what keeps `command` free of `lua`.

**`show-paren` is computed by `ui` from point at render time, not by a command.**
`Env` cannot reach the screen by design, so bracket matching has nowhere to
report to — correctly. Nothing highlight-related belongs on `Env`.

**Buffer names live in `editor`, not `text`.** `text.Buffer` has only `Path()`.
`editor` owns the name↔buffer map, which is why `*scratch*` and `*Buffer List*`
are editor concepts rather than text ones.

**`commandtest.Fake` holds real `text.Buffer`, `view.Window` and `KillRing`
instances, not stubs**, so kill-run accumulation and yank behaviour are genuine
in tests. It invokes `OnChange` once per successive prefix (`f`, `fo`, `foo`) so
incremental-search commands are driven the way real typing drives them, and
exhausting its canned replies returns `ErrNoReply` rather than `ErrQuit` — a test
bug must never masquerade as the user pressing `C-g`.

## Tracked follow-ups

Real defects or design regressions found while building, deliberately deferred so
they do not race the in-flight command work. Each is small and each has a known
fix — they are tracked here rather than left in a transcript.

**1. Horizontal scroll is in the wrong package.** It currently lives in
`ui.Renderer.hscroll`, a `map[*view.Window]text.ColIdx`, because the render
agent was told not to modify `view`. That was my constraint and it was wrong:
vertical scroll is `view.Window.Top`, and horizontal scroll is the same kind of
state, so it belongs as `view.Window.LeftCol`. Two consequences of the current
home: `Render` cannot be a package-level function, and the map leaks an entry per
closed window (mitigated today by pruning windows absent from the layout each
frame, which is a workaround for misplaced state).

**2. `text.Line.Runes()` allocates in the hot path.** It copies the whole line,
and the render path needs every visible line's runes on every keystroke — one
allocation per visible line per frame. Fix: a lending accessor, or better a
grapheme iterator yielding `(runes, startCol, width)` so `ui` stops
reconstructing one from `NextGrapheme` + `DisplayCol`.

**3. `DisplayCol(Len()) == Width()` is relied upon but only implied.** The render
path depends on it for the last cluster of every line. I verified it holds for
ASCII, tabs, CJK, emoji, combining marks and ZWJ clusters — it should be promoted
to a documented guarantee on `Line` with a test pinning it, since it is currently
an accident of the implementation rather than a contract.

### One ordering hazard, recorded rather than fixed

`Render` paints dividers **after** windows, so a pane writing into the divider
column gets repainted over. The wide-glyph straddle-left guard in the draw path
is therefore defensive rather than load-bearing *as currently ordered*. If anyone
reorders the frame so dividers precede windows, that guard becomes the only thing
stopping a wide glyph from eating the divider. Do not reorder without testing
that case.

### A tcell behaviour worth knowing

tcell silently converts control runes to spaces. So writing a literal `\t`
instead of expanding it produces byte-identical output under `StyleDefault` — and
stops being identical the moment the text area carries a background colour. The
render tests assert the tab region carries `Theme.Text` precisely because that is
the form the bug will take once syntax highlighting lands.

## Search decisions (wave 4)

**A search pattern containing a newline never matches.** `SearchForward` and
`SearchBackward` reject it outright rather than silently misbehaving. This sits
next to the truncate-don't-wrap decision as a v1 simplification: the buffer is a
line array, so a multi-line pattern would need matching to span elements, and
nothing in the v1 command set needs it. `query-replace` *replacements* may
contain newlines and work correctly — only patterns are restricted.

**Incremental search is an exported `Isearch` session**, not a closure inside the
command. It carries `Update`/`Advance`/`Abandon`/`Pattern`, because the spec
requires `editor`'s minibuffer keymap to advance the match on a repeated `C-s`
— which is impossible if the session state is unreachable from `editor`. This
seam was implied by the architecture and would otherwise have been discovered
missing while writing the event loop.

Every search runs from the origin captured at entry. That is what makes
shortening the pattern with backspace walk point back toward the origin rather
than leaving it stranded at a match that no longer applies.

**`self-insert-command` needs the triggering key**, which no other command does.
It is served by `Seq.LastRune`, set by the event loop before dispatch. The
alternative — an exported `SelfInsert(e, r)` called directly by the loop — leaves
`M-x self-insert-command` and any Lua binding to it broken, so the `Seq` field is
the correct home.
