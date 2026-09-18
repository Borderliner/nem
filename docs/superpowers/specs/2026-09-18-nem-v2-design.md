# nem v2 — beyond emacs defaults

Approved 2026-09-18. Direction: where emacs's defaults are bad for a terminal
editor you live in, deviate deliberately.

## Scope

| Feature | Why it's here |
|---|---|
| Completion panel, fuzzy, popup or bottom | `TAB`-completes-a-prefix is not a completion UI |
| Prefix-key discovery (which-key) | The biggest discoverability gap in emacs |
| Move region up/down | Emacs has no native line move |
| Data safety: backup, autosave, external-change | Emacs does these well; nem did none |
| System clipboard (OSC 52) | The kill ring was an island |
| Undo grouping in `text` | Two features above need it |

Not in v2: mouse, multiple cursors, expand-region, semantic selection.

## Settled decisions

**One prompt state, two renderers.** The minibuffer stays a real `text.Buffer`
in a real `view.Window` — that is what makes `C-a`/`C-k`/yank work inside
prompts. Where it *draws* is independent: a floating panel (default) or the
bottom rows (`completion-style = "bottom"`). Same state, same candidate list.

**Panels never tile.** Geometry is pure maths in `view`; drawing is a pass in
`ui` that runs after the tiled frame and before `ShowCursor`. A panel cannot
violate the exact-tiling invariant because it is not part of the tiling.

**Fuzzy, with visible reasons.** Subsequence matching with scoring. The panel
highlights the matched characters, because unexplained fuzzy results look
arbitrary. Smart case, matching isearch's rule: case-insensitive while the query
is all lowercase.

**`RET` is governed by a per-prompt `RequireMatch` flag.** `M-x` requires a
match; `find-file` and `switch-to-buffer` do not, so `RET` takes exactly what you
typed. `M-RET` forces literal input where allowed. Emacs's `completing-read`
model, not Vertico's, because otherwise `C-x C-f newfile.go` cannot create a file.

**Backups never touch the project directory.** `$XDG_STATE_HOME/nem` (default
`~/.local/state/nem`) with the absolute path mirrored beneath it. `file~` beside
the file clutters a git tree and is the wrong default.

**tcell owns the clipboard escape.** `Screen.SetClipboard([]byte)` base64-encodes
and emits via terminfo under tcell's own lock; `GetClipboard()` queries and the
reply arrives as `*tcell.EventClipboard`. No raw writes, no hand-rolled base64.
Verified in tcell 2.13 source (`tscreen.go:1301`).

## Contracts — wave 1

These five are independent of each other and of everything above them. Built
first, in parallel.

### `text`: undo grouping

```go
func (b *Buffer) BeginUndoGroup()
func (b *Buffer) EndUndoGroup()
```

An undo group makes several `Insert`/`Delete` calls one undo unit. Groups nest by
depth count; only the outermost boundary closes a unit. Required by
`move-lines-*` (a move is a delete plus an insert — two units without this, so
five presses cost ten `C-/`) and by Lua `set_text`.

Must keep working: insert coalescing, the `savedAt`/`Modified` relationship, mark
adjustment, and redo-branch discarding.

### `view`: panel placement

```go
type Anchor int // AnchorPoint, AnchorBottom, AnchorCenter
type PanelReq struct {
    W, H     int
    Anchor   Anchor
    Frame    Rect
    PtX, PtY int
}
func PlacePanel(req PanelReq) (Rect, bool)
```

Clamp to `Frame`; prefer below point and flip above when it will not fit; never
cover the echo row; return `false` when the frame cannot hold a useful panel, so
the caller degrades to bottom rendering rather than drawing a 3-column box.

### `keymap`: prefix continuations

```go
type Continuation struct {
    Key      Key
    Command  string // empty when IsPrefix
    IsPrefix bool
    Count    int    // bindings beneath, when IsPrefix
}
func (m *Map) Continuations(seq []Key) []Continuation
```

Deterministically sorted. Derivable from `Bindings()` today, but this is its only
caller and a direct walk is clearer. Stdlib only, as the rest of `keymap`.

### `fuzzy`: new package, stdlib only

```go
type Match struct {
    Score   int
    Indices []int // rune indices in the candidate that matched
}
func Score(query, candidate string) (Match, bool)

type Ranked struct {
    Candidate string
    Match     Match
}
func Rank(query string, candidates []string) []Ranked // sorted, best first
```

Subsequence match: query runes appear in order, not necessarily contiguous, so
`fwc` finds `forward-char`. Scoring: bonus at a word boundary (string start, or
after `-`, `_`, `/`, `.`, or a lower-to-upper case change), bonus for consecutive
runs, penalty for gap length, shorter candidate wins ties. Smart case:
case-insensitive while the query is all lowercase.

An empty query matches everything with score 0 and no indices, in input order.
Ordering must be total and stable — pinned against a fixed candidate set, because
"predictable" is a feature and a scoring tweak must show up as a test diff.

### `backup`: new package

```go
func DefaultRoot() (string, error) // $XDG_STATE_HOME/nem, else ~/.local/state/nem
type Store struct{ /* root */ }
func New(root string) *Store

func (s *Store) BackupPath(file string) string   // <root>/backups/<abs path>~
func (s *Store) AutosavePath(file string) string // <root>/autosave/<abs path>#

func (s *Store) WriteBackup(file string, content []byte) error
func (s *Store) WriteAutosave(file string, content []byte) error
func (s *Store) RemoveAutosave(file string) error
func (s *Store) ReadAutosave(file string) ([]byte, error)
func (s *Store) AutosaveNewer(file string) (bool, time.Time, error)
```

Paths are mirrored from the cleaned absolute path, so `/home/reza/p/main.go`
backs up to `<root>/backups/home/reza/p/main.go~` — browsable, and two files with
the same base name in different projects cannot collide. Directories created as
needed. Nothing is ever written inside the edited file's own directory.

## Wave 2 — depends on wave 1

**`ui` panel drawing.** `Frame` gains `Panels []Panel`; a panel is a rect plus
styled lines plus an optional selected index, and a line carries matched-index
ranges so fuzzy hits can be emphasised. Drawn after dividers, before
`ShowCursor`, because a prompt rendered inside a panel needs the hardware cursor
there. New `Theme` fields for the panel border, background-less body, selected
row, and matched characters.

**Completion session** in the minibuffer: candidates recomputed on every content
change (the existing `OnChange` path already fires on backspace), ranked by
`fuzzy`, with a selection index. Keys: `C-n`/`↓`, `C-p`/`↑`, `TAB` completes to
the selection, `RET` per `RequireMatch`, `M-RET` literal, `C-g` aborts. Ten rows
by default, selection kept in view, a `3/57` count.

**Which-key.** A pending prefix starts a timer (`which-key-delay`, default 300ms,
`0` disables). On expiry, a panel built from `Continuations`. Any real key
dismisses it. **This changes the event loop's shape:** `PollEvent` becomes a
`select` over `scr.ChannelEvents` and a timer. Still one consumer goroutine, so
the single-goroutine rule for Lua and the keymap holds.

**`move-lines-up` / `move-lines-down`** on `M-<up>` / `M-<down>`. Whole lines
spanned by the region, or the current line when there is none. Mark and point
both shift so the selection survives and the key repeats. One undo group per
move. Refuses at a buffer edge with a message.

**Data safety wiring.** Backup on a buffer's first save of the session. Autosave
every 30s of idle while modified, removed on a successful save. On `find-file`,
an autosave newer than the file is offered in the echo area and `M-x recover-file`
restores it. `mtime` and size recorded on load and save; if they moved when you
save, `File changed on disk. Overwrite? (y/n)` rather than a silent clobber.

**Clipboard.** `M-w` and `C-w` also call `Screen.SetClipboard`. A yank may request
`GetClipboard()` and use the `EventClipboard` reply when it arrives, falling back
to nem's ring — read support is far less widely implemented than write and this
will not pretend otherwise. `clipboard = "osc52" | "off"`, default on. tmux needs
`allow-passthrough`; document it.

## Config additions

```lua
nem.set("completion-style", "popup")   -- or "bottom"
nem.set("completion-rows", 10)
nem.set("which-key-delay", 300)        -- ms; 0 disables
nem.set("autosave-idle", 30)           -- seconds; 0 disables
nem.set("backup", true)
nem.set("clipboard", "osc52")          -- or "off"
```
