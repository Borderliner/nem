# nem

A terminal text editor with nano's shape and emacs's keybindings.

One file, one screen, no modes — but `C-f` moves forward, `C-k` kills a line,
`C-x C-s` saves, and the kill ring behaves the way your fingers expect.

```
 1 package main                      │  1 # notes
 2                                   │  2
 3 import "fmt"                      │  3 - ship the thing
 4                                   │  4 - write it down
 5 func main() {                     │  5
 6     // say hello                  │
 7     fmt.Println("hi", 42)         │
 8 }                                 │
▍main.go  go  ⎇ main ──────── 7:26  │  notes.md  md ──────── 3:0
Wrote main.go
```

Code is coloured, line numbers sit outside the text so they can never be
selected, and the status line carries the file type and git branch. It keeps
your terminal's own background rather than painting over it.

## Install

Download a binary for your platform from the
[latest release](https://github.com/Borderliner/nem/releases/latest), or:

```sh
go install github.com/Borderliner/nem/cmd/nem@latest
```

From source:

```sh
git clone https://github.com/Borderliner/nem
cd nem && make build
./nem file.txt
```

Go 1.27 or later, and nothing else.

`make build` sets `CGO_ENABLED=0`, which matters: Go turns cgo on by default
when a C compiler is present, and a dependency pulls in `os/user`, which links
libc for NSS lookups. With it off the binary is **fully static** — no libc, no
glibc version skew, runs on musl and Alpine — and strips to about 5 MB. A plain
`go build` still works but gives you a dynamically linked binary.

Being cgo-free is also why every platform cross-compiles from any other, so
releases for Linux, macOS and Windows on both amd64 and arm64 all come off one
machine. Windows binaries are published but lightly tested; state lives under
`%LOCALAPPDATA%` there rather than the XDG path used elsewhere.

## What it does

**Editing** — the emacs motion and editing set: `C-f`/`C-b`/`C-n`/`C-p`,
`M-f`/`M-b`, `C-a`/`C-e`, `M-<`/`M->`, `C-d`, `C-k`, `M-d`, `C-t`, `M-u`/`M-l`/`M-c`.
Cursor motion stops at grapheme boundaries, so a decomposed `é` or a ZWJ family
emoji is one press, not seven.

**Mark and kill ring** — `C-SPC` sets the mark, `C-x h` selects the whole
buffer, `C-w` kills the region, `M-w`
copies it, `C-y` yanks and `M-y` rotates. Consecutive kills accumulate into one
entry, so `C-k C-k` then `C-y` gives you back what you took. Backward kills
prepend, so `M-DEL M-DEL` over "foo bar" yanks as `foo bar`, not `bar foo`.

**Buffers and windows** — `C-x C-f` find file, `C-x b` switch buffer,
`C-x 2`/`C-x 3` split, `C-x o` move between windows. Two windows onto one buffer
keep independent cursors and viewports.

**Dired** — `C-x d` lists a directory, `C-x C-j` lists the one the current file
is in with point on it, and `nem .` or `C-x C-f` on a directory does too. The
listing is aligned and coloured by kind, directories first and dotfiles hidden
until you press `.`; `(` hides the permissions, sizes and dates, and `s` sorts
by name, time or size.

| Key | Does |
|---|---|
| `RET` `^` | open the file or directory at point · go up a level |
| `n` `p` | next and previous file |
| `m` `u` `t` `U` | mark · unmark · invert the marks · unmark all |
| `d` `x` | flag for deletion · delete the flagged files |
| `D` `R` `C` | delete · rename or move · copy — the marked files, or the one at point |
| `+` `g` `w` `o` `q` | new directory · re-read · copy the name · open in the other window · leave |

Deleting asks you to type `yes`, nothing is replaced without asking, and a
rename carries any buffer visiting the file along with it. One listing follows
you around the tree rather than a new buffer piling up per directory.

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
highlighted so you can see why something matched. `RET` takes the highlighted
entry, and on a directory it walks into it and lists what is inside. The
directory itself comes first, as `./` before you have typed anything, so
`C-x C-f RET` opens it in dired. `M-RET` takes exactly what you typed, for a new
file or buffer whose name happens to match an existing one. Buffers are listed most recently visited first, so
`C-x b RET` flips back to the previous one.

**Line numbers** — shown by default, and drawn outside the text so they can
never be selected or copied. `C-x n` toggles them.

**Syntax highlighting** — Go, Lua, JSON and Markdown have hand-written lexers;
C, Python, shell, Rust, JavaScript, TypeScript, YAML, TOML, HTML, CSS, SQL,
Makefile, Dockerfile, XML and INI are bundled into the binary. If nano happens
to be installed, its definitions add about forty more. Nothing needs installing.

**Selection** — `C-SPC` marks, and the region is visible. Typing replaces it,
Backspace deletes it, and that undoes in one step.

**Moving text** — `M-<up>` and `M-<down>` move the current line, or every line
the region covers, keeping the selection so the key repeats.

**System clipboard** — `C-w` and `M-w` also put the text on your system
clipboard over OSC 52, which works through SSH. `C-y` pastes what you copied in
other applications, through `wl-paste`, `xclip`, `xsel` or `pbpaste`, and falls
back to nem's own kill ring when there is no text to be had.

**Pasting** — the terminal's paste key inserts text verbatim, without
re-indenting it, as a single undo step.

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

Mouse support · line wrapping (long lines truncate with `$` and scroll
horizontally instead) · undo tree · multi-line search patterns · language-aware
indentation

## Design

Layered so the hard parts are testable without a terminal:

| Package | Responsibility |
|---|---|
| `text` | Buffer, lines, positions, edit primitives, undo log |
| `keymap` | Key parsing and the prefix tree — standard library only |
| `view` | Windows, the split tree, layout arithmetic |
| `command` | Named commands and the `Env` they act through |
| `dired` | Directory listings and the file operations dired performs |
| `ui` | tcell rendering; Lip Gloss for chrome |
| `lua` | Config and scripting host |
| `editor` | The event loop that wires it together |

Two decisions worth knowing. **The minibuffer is a real buffer in a real
window**, as in emacs, so `C-a`/`C-e`/`C-k` and the kill ring work inside prompts
with no extra code, and `M-x`, `C-s`, find-file and query-replace are all callers
of one mechanism. And **commands never reach the screen** — they act through an
interface, which is why the whole command layer is tested headlessly.

The full design, including the decisions that were rejected and why, is in
[docs/design/2026-09-18-nem-design.md](docs/design/2026-09-18-nem-design.md).

## Licence

MIT — see [LICENSE](LICENSE).

Released binaries are statically linked and so contain the permissively licensed
libraries listed in [THIRD-PARTY.md](THIRD-PARTY.md). nem reads nano's syntax
definitions from the system when they are present but never distributes them;
the rules it bundles are its own.
