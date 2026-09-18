# nem — v1 command inventory

The implementation checklist for `command/defaults.go`. Every row is a
registered `Command` with a name, a doc string, and a default binding.
`M-x` searches this table; Lua rebinds against these names.

`ARG` marks commands that honour the universal argument (`C-u`).

## Motion

| Binding | Command | Notes |
|---|---|---|
| `C-f` / `<right>` | `forward-char` | ARG. One **grapheme**, not one rune. |
| `C-b` / `<left>` | `backward-char` | ARG. |
| `C-n` / `<down>` | `next-line` | ARG. Preserves goal column. |
| `C-p` / `<up>` | `previous-line` | ARG. Preserves goal column. |
| `M-f` | `forward-word` | ARG. |
| `M-b` | `backward-word` | ARG. |
| `C-a` / `<home>` | `move-beginning-of-line` | |
| `C-e` / `<end>` | `move-end-of-line` | |
| `M-<` | `beginning-of-buffer` | |
| `M->` | `end-of-buffer` | |
| `C-v` / `<pgdn>` | `scroll-up-command` | One screen less two lines. |
| `M-v` / `<pgup>` | `scroll-down-command` | |
| `M-g M-g` | `goto-line` | Prompts unless ARG given. |
| `C-l` | `recenter-top-bottom` | Cycles centre → top → bottom. |

Goal column is the rule people notice when it is wrong: `C-n` through a short
line and back out must return to the original column, so the goal column is
set by horizontal motion and preserved by vertical motion.

## Editing

| Binding | Command | Notes |
|---|---|---|
| `C-d` / `<delete>` | `delete-char` | ARG. |
| `<backspace>` | `delete-backward-char` | ARG. |
| `M-d` | `kill-word` | ARG. Appends to kill ring when consecutive. |
| `M-<backspace>` | `backward-kill-word` | ARG. |
| `C-k` | `kill-line` | ARG. Consecutive `C-k` **appends** to the same kill-ring entry. |
| `C-o` | `open-line` | |
| `C-t` | `transpose-chars` | |
| `M-t` | `transpose-words` | |
| `RET` | `newline` | Auto-indent: copies previous line's leading whitespace. |
| `TAB` | `indent-for-tab-command` | |
| `M-u` | `upcase-word` | ARG. |
| `M-l` | `downcase-word` | ARG. |
| `M-c` | `capitalize-word` | ARG. |
| `M-;` | `comment-line` | Deferred with syntax highlighting — needs language knowledge. |

## Mark, region, kill ring

| Binding | Command | Notes |
|---|---|---|
| `C-SPC` | `set-mark-command` | Arrives as NUL; see `keymap.Normalize`. |
| `C-x C-x` | `exchange-point-and-mark` | |
| `C-w` | `kill-region` | |
| `M-w` | `kill-ring-save` | |
| `C-y` | `yank` | |
| `M-y` | `yank-pop` | **Only valid immediately after a yank or yank-pop.** Otherwise errors. |

The kill ring is a fixed-size ring (60 entries, as emacs). Consecutive kill
commands append to the top entry rather than pushing; any non-kill command
breaks the run. `C-k C-k C-k` then `C-y` restores three lines as one block —
this is the single most-cited kill-ring behaviour and the integration harness
tests it directly.

Run-awareness lives **in the ring**, not in the commands: `KillForward` and
`KillBackward` decide for themselves whether to extend the newest entry or push
a new one. This deliberately differs from emacs, where `kill-region` inspects
`last-command` to choose between `kill-new` and `kill-append` — putting it in
the ring means an individual command cannot get it wrong.

Backward kills (`M-<backspace>`) extend the entry on the **left**, so killing
backward word by word yields reading order: `M-DEL M-DEL` over "foo bar" yanks
back as `"foo bar"`, not `"bar foo"`. Getting this backwards fails silently,
which is why it has a named regression test.

**The yank pointer survives a broken run.** After `M-y` leaves the pointer
mid-ring, a *later* `C-y` yanks from there rather than from the front — real
emacs computes `(mod (- n (length kill-ring-yank-pointer)) (length kill-ring))`
and only a fresh kill resets the pointer. This is faithful, non-obvious, and
looks like a bug in a year's time, so it is pinned by test.

`Kill("")` is a complete no-op: no entry, no run started or broken, no pending
yank-pop invalidated. Emacs's `kill-new ""` does push an empty entry; we
diverge deliberately, since an empty entry is pure noise in the rotation.

**`BreakRun` belongs in the central dispatch path**, called for every command
that is neither a kill nor a yank. Scattered across individual commands it will
be forgotten in exactly one of them, and the symptom — two kills fusing into
one entry — will look like a ring bug.

## Undo

| Binding | Command | Notes |
|---|---|---|
| `C-/` / `C-_` | `undo` | ARG. Same physical key on most terminals. |
| `M-_` | `redo` | ARG. Not an emacs binding — we chose linear undo. |
| `C-x u` | `undo` | Alias. |

## Search and replace

| Binding | Command | Notes |
|---|---|---|
| `C-s` | `isearch-forward` | Incremental. `ReadString` with an `OnChange` hook. |
| `C-r` | `isearch-backward` | |
| `M-%` | `query-replace` | Prompts twice, then `y`/`n`/`!`/`q` per match. |

Inside isearch: `C-s` advances to the next match, `C-r` reverses, `<backspace>`
shortens the pattern and moves back, `RET` exits leaving point at the match,
`C-g` **restores the point saved at entry**. That restore is the behaviour
people rely on and the easiest to forget.

## Files

| Binding | Command | Notes |
|---|---|---|
| `C-x C-f` | `find-file` | Prompts with filename completion. |
| `C-x C-s` | `save-buffer` | Runs the `before-save` Lua hook. |
| `C-x C-w` | `write-file` | Save as. |
| `C-x s` | `save-some-buffers` | Prompts per modified buffer. |

## Buffers

| Binding | Command | Notes |
|---|---|---|
| `C-x b` | `switch-to-buffer` | Completes over buffer names. |
| `C-x k` | `kill-buffer` | Confirms if modified. |
| `C-x C-b` | `list-buffers` | Opens a buffer listing in a window. |

## Windows

| Binding | Command | Notes |
|---|---|---|
| `C-x 2` | `split-window-below` | |
| `C-x 3` | `split-window-right` | |
| `C-x 1` | `delete-other-windows` | |
| `C-x 0` | `delete-window` | |
| `C-x o` | `other-window` | ARG. |

Splitting a window gives the new window the **same buffer and the same point**
as the one it split, which is how two views onto one buffer arise — the case
that drove the whole architecture.

## Session and help

| Binding | Command | Notes |
|---|---|---|
| `C-g` | `keyboard-quit` | Cancels a pending prefix, a prompt, or the mark. Never a no-op. |
| `C-x C-c` | `save-buffers-kill-terminal` | Prompts on unsaved buffers. |
| `M-x` | `execute-extended-command` | Completes over `Registry.Names()`. |
| `C-u` | `universal-argument` | Handled in the event loop, not as an ordinary command. |
| `C-h b` | `describe-bindings` | Renders `Map.Bindings()`. |
| `C-h k` | `describe-key` | Reads one key sequence, reports its command. |

## Self-insert

Any printable key with no binding runs `self-insert-command`, which honours ARG
(`C-u 40 -` inserts forty dashes) and coalesces into a single undo unit until
the run is broken.
