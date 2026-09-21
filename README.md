# nem

A terminal text editor with nano's shape and emacs's keybindings.

One file, one screen, no modes — but `C-f` moves forward, `C-k` kills a line,
`C-x C-s` saves, and the kill ring behaves the way your fingers expect.

```
original line                          │  package main
                                       │
                                       │  func main() {
                                       │  }
--  notes.md                    1:0    │ **  main.go                    3:1
C-x C-s save   C-x C-c exit   M-x command
```

## Build

```sh
go build ./cmd/nem
./nem file.txt
```

Go 1.27 or later. No cgo, no runtime — one static binary.

## What it does

**Editing** — the emacs motion and editing set: `C-f`/`C-b`/`C-n`/`C-p`,
`M-f`/`M-b`, `C-a`/`C-e`, `M-<`/`M->`, `C-d`, `C-k`, `M-d`, `C-t`, `M-u`/`M-l`/`M-c`.
Cursor motion stops at grapheme boundaries, so a decomposed `é` or a ZWJ family
emoji is one press, not seven.

**Mark and kill ring** — `C-SPC` sets the mark, `C-w` kills the region, `M-w`
copies it, `C-y` yanks and `M-y` rotates. Consecutive kills accumulate into one
entry, so `C-k C-k` then `C-y` gives you back what you took. Backward kills
prepend, so `M-DEL M-DEL` over "foo bar" yanks as `foo bar`, not `bar foo`.

**Buffers and windows** — `C-x C-f` find file, `C-x b` switch buffer,
`C-x 2`/`C-x 3` split, `C-x o` move between windows. Two windows onto one buffer
keep independent cursors and viewports.

**Search** — `C-s` is genuinely incremental: it moves as you type, backspace
walks point back, and `C-g` returns you to where you started. `M-%` is
query-replace with `y`/`n`/`!`/`q`.

**Prefix keys, `M-x`, and `C-u`** — `C-x` is a real prefix keymap, `M-x` completes
over every command by name, and `C-u` takes numeric and negative arguments.

**Undo** — linear undo and redo (`C-/` and `M-_`). Typing coalesces into one
undo unit; a new edit after undoing discards the redo branch.

**Help** — `<f1> b` lists every binding, `<f1> k` describes a key. Pause on a
prefix like `C-x` and a panel shows everything that can follow it.

**Completion** — `M-x`, `C-x C-f` and `C-x b` open a centred panel that filters
as you type, fuzzily: `fwc` finds `forward-char`. Matched characters are
highlighted so you can see why something matched.

**Line numbers** — shown by default, and drawn outside the text so they can
never be selected or copied.

**Moving text** — `M-<up>` and `M-<down>` move the current line, or every line
the region covers, keeping the selection so the key repeats.

**Your work is kept** — a backup of the previous contents on first save, an
autosave every 30 seconds while modified, and a refusal to overwrite a file that
changed on disk underneath you. Nothing is written beside your file; it all goes
under `~/.local/state/nem`.

> `C-h` is bound to help too, but only works on terminals that negotiate the
> extended keyboard protocol. On older terminals the terminal itself cannot tell
> `C-h` from Backspace before nem sees it. Use `<f1>`.

## Configuration

`~/.config/nem/init.lua` is a real Lua script, not a config format:

```lua
nem.set("tab-width", 4)
nem.bind("<f5>", "save-buffer")

nem.command("strip-trailing-space", "Remove trailing whitespace everywhere.",
  function()
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
    end
  end)
nem.bind("C-c w", "strip-trailing-space")

nem.hook("before-save", function(buf)
  if buf.path:match("%.go$") then nem.run("strip-trailing-space") end
end)
```

Commands you define are registered alongside the built-ins, so `M-x` finds them
and `<f1> k` describes them. Line numbers are 1-based, and anything a script
changes is undoable with `C-/`. A broken config never takes the editor down — the
error lands in the echo area and nem carries on with defaults.

Scripts get `string`, `table` and `math`, but not `io` or `os`, so a hook cannot
shell out to an external formatter. A formatter written in Lua against `nem.buf`
works.

See [docs/config.md](docs/config.md).

## Not yet

Syntax highlighting · mouse support · line wrapping (long lines truncate with
`$` and scroll horizontally) · undo tree · multi-line search patterns

## Design

Layered so the hard parts are testable without a terminal:

| Package | Responsibility |
|---|---|
| `text` | Buffer, lines, positions, edit primitives, undo log |
| `keymap` | Key parsing and the prefix tree — standard library only |
| `view` | Windows, the split tree, layout arithmetic |
| `command` | Named commands and the `Env` they act through |
| `ui` | tcell rendering; Lip Gloss for chrome |
| `lua` | Config and scripting host |
| `editor` | The event loop that wires it together |

Two decisions worth knowing. **The minibuffer is a real buffer in a real
window**, as in emacs, so `C-a`/`C-e`/`C-k` and the kill ring work inside prompts
with no extra code, and `M-x`, `C-s`, find-file and query-replace are all callers
of one mechanism. And **commands never reach the screen** — they act through an
interface, which is why the whole command layer is tested headlessly.

The full design, including the decisions that were rejected and why, is in
[docs/superpowers/specs/2026-09-18-nem-design.md](docs/superpowers/specs/2026-09-18-nem-design.md).
