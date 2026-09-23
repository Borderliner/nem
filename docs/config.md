# Configuring nem

nem reads `~/.config/nem/init.lua` at startup. The config file is a real Lua
script, not a declarative format — anything you can compute, you can configure.

It loads **after** built-in commands register and after default bindings are
installed, so your config always wins.

## Key notation

Emacs notation, with the modifiers you would expect:

| Written | Means |
|---|---|
| `C-x` | Control+x |
| `M-x` | Meta+x (Alt, or Escape followed by the key) |
| `C-M-f` | Control+Meta+f |
| `C-x C-f` | A two-key sequence: `C-x`, then `C-f` |
| `<f5>` `<up>` `<home>` `<pgdn>` | Named keys, in angle brackets |
| `SPC` `RET` `TAB` `DEL` `ESC` | Named ASCII keys |
| `S-<up>` | Shift+Up |

Shift is only meaningful on named keys. For ordinary characters the character
itself carries the case, so write `X`, never `S-x` — the latter is an error.

### `DEL` means Backspace, not forward-delete

This trips up everyone once. In emacs tradition, and therefore in nem:

- `DEL` (or `<backspace>`) deletes **backward**.
- `<delete>` deletes **forward**.

So `nem.bind("DEL", "delete-char")` binds Backspace to *forward* deletion, which
is almost certainly not what you meant. You wanted `delete-backward-char`.

### A note on `C-h`

`<f1>` is nem's help prefix: `<f1> b` lists bindings, `<f1> k` describes a key.

`C-h` is bound to the same things, but **it only works on terminals that
negotiate the extended keyboard protocol** (kitty, foot, ghostty, recent xterm).
On older terminals the terminal itself cannot distinguish `C-h` from Backspace
before nem ever sees it, so the binding silently does nothing. This is a
limitation of terminal encoding, not of nem. Use `<f1>` if you want help to work
everywhere.

## Settings

```lua
-- Display
nem.set("tab-width", 4)          -- display width of a tab; default 8
nem.set("scroll-margin", 3)      -- lines of context kept around point; default 2
nem.set("line-numbers", true)    -- show a line-number gutter; default true
nem.set("delete-selection", true)-- typing replaces the selection; default true
nem.set("syntax", true)          -- colour code; default true
nem.set("theme", "auto")         -- syntax palette: "auto", "dark" or "light"

-- Completion
nem.set("completion-style", "popup")  -- "popup" (centred panel) or "bottom"; default "popup"
nem.set("completion-rows", 10)        -- candidates visible at once; default 10

-- Discovery
nem.set("which-key-delay", 300)  -- ms a prefix waits before listing what follows;
                                 -- default 300, 0 disables

-- Safety
nem.set("autosave-idle", 30)     -- seconds of idleness before autosaving; 0 disables
nem.set("backup", true)          -- keep the previous contents on first save; default true

-- System
nem.set("clipboard", "osc52")    -- "osc52" or "off"; default "osc52"
nem.set("undo-style", "linear")  -- "linear" is the default and currently the only mode
```

### Line numbers

The gutter is drawn outside the text, so line numbers **cannot be selected,
marked or copied**. That is structural rather than a special case: `C-w` and
`M-w` operate on positions inside the buffer, and a line number is never in the
buffer. The line point is on is shown in bolder type.

A pane too narrow to leave room for text drops the gutter rather than squeezing
the text out.

### Typing replaces the selection

With a region active, typing replaces it, and Backspace or `C-d` deletes it
rather than removing a single character. `C-y` replaces it with what you yank.
The whole replacement is one undo step.

This is emacs's `delete-selection-mode`, which vanilla emacs leaves off and
every other editor leaves on. `delete-selection = false` restores emacs's
behaviour, where a selection sits inert until a command explicitly uses it.

`C-w` and `M-w` are unaffected — they consume the region themselves. So is
`TAB`: with a block selected it indents, and deleting instead would be
destructive.

### Syntax colours and the two palettes

nem never paints a background, so it sits inside whatever terminal colours you
already run. That means one palette cannot serve everyone: colours with enough
contrast on black wash out on white. So there are two, and `theme` picks.

`"auto"` guesses from the `COLORFGBG` environment variable, which several
terminals set and many do not. When it is absent nem guesses dark, because most
terminals are and because the dark palette on a light background is faint rather
than invisible. If your code looks washed out, set `theme = "light"`.

Highlighting works out of the box with nothing installed. It comes from three
places, in order:

1. **Hand-written lexers** for Go, Lua, JSON and Markdown. These carry state
   across lines, so a block comment or a raw string spanning lines is handled
   exactly rather than approximately.
2. **Bundled rules**, compiled into the binary, for C, Python, shell, Rust,
   JavaScript, TypeScript, YAML, TOML, HTML, CSS, SQL, Makefile, Dockerfile,
   XML and INI.
3. **nano's rules**, read from `/usr/share/nano` when nano happens to be
   installed, covering roughly 40 more languages. Purely a bonus — nem never
   needs them.

A file with no extension is matched on its `#!` line, so a script called
`deploy` starting with `#!/bin/sh` is highlighted as shell.

Anything unmatched renders plain, including `*scratch*`.

### Where backups and autosaves go

Never beside your file. Everything lives under `$XDG_STATE_HOME/nem` (usually
`~/.local/state/nem`) with the file's path mirrored beneath it:

```
~/.local/state/nem/backups/home/you/project/main.go~
~/.local/state/nem/autosave/home/you/project/main.go#
```

So a git working tree stays clean, and two files with the same name in different
projects cannot collide. If nem tells you an autosave is newer than the file on
disk, `M-x recover-file` restores it.

A save also refuses to overwrite a file that changed on disk underneath you,
asking first rather than clobbering it silently.

### Clipboard

`C-w` and `M-w` also put the text on your system clipboard using OSC 52, which
works over SSH. Inside tmux you need `set -g allow-passthrough on` for it to
leave the pane.

`C-y` reads the clipboard back, so text copied in another application is what it
pastes; `M-y` then steps back through nem's own kills as usual. Reading goes
through the desktop's clipboard tool rather than the terminal, because few
terminals answer an OSC 52 read: `wl-paste` on Wayland, `xclip` or `xsel` on X11,
`pbpaste` on macOS. With no tool installed, no display (a plain SSH session), or
something other than text on the clipboard, `C-y` yanks from the kill ring alone.

Set `clipboard = "off"` if you would rather nem never touched the clipboard in
either direction.

### Pasting with the terminal

Pasting with the terminal's own paste key (`Ctrl-Shift-V`, `Cmd-V`) inserts the
text exactly as it was copied: nem turns on bracketed paste, so pasted lines are
not re-indented and pasted characters are never taken as commands. One `C-/`
undoes the whole paste, and pasting over a selection replaces it. In a prompt a
paste stays on one line: a trailing newline is dropped and any other newline
becomes a space.

## Rebinding keys

```lua
nem.bind("C-x C-f", "find-file")
nem.bind("M-g M-g", "goto-line")
nem.bind("<f5>",    "save-buffer")
```

The second argument is a **command name**, not a function — the same name `M-x`
completes over. `<f1> b` shows everything currently bound, and `<f1> k` tells you
what a key does.

Binding a new sequence whose prefix is already bound to a command is an error,
and nem will tell you which two bindings conflict. Rebinding an existing exact
sequence simply replaces it.

## Defining your own commands

```lua
nem.command("reverse-line", "Reverse the characters on the current line.",
  function()
    local l = nem.buf.line()
    nem.buf.replace_line(l:reverse())
  end)

nem.bind("C-c r", "reverse-line")
```

A command you define this way is registered in exactly the same table as the
built-ins, so it appears in `M-x`, it is bindable, and `<f1> k` describes it.
There is no second-class citizenship for Lua commands.

## Reading and changing the buffer

Inside a command or a hook, `nem.buf` reaches the current buffer. Outside one
there is no buffer to act on, and these raise an error rather than guessing.

**Line numbers are 1-based**, everywhere and without exception — matching Lua's
own convention, the `line:col` the modeline shows, and `M-x goto-line`. Line 1 is
the first line. Asking for a line that does not exist is an error, not a silently
empty string.

Where point is:

| Call | Does |
|---|---|
| `nem.buf.line()` | the text of the line point is on |
| `nem.buf.replace_line(s)` | rewrite that line |
| `nem.buf.point()` | returns line, column |
| `nem.buf.set_point(line, col)` | move point |

Addressed by line number:

| Call | Does |
|---|---|
| `nem.buf.line_count()` | how many lines the buffer has |
| `nem.buf.get_line(n)` | the text of line `n` |
| `nem.buf.set_line(n, s)` | rewrite line `n` |
| `nem.buf.insert_line(n, s)` | insert a line, which becomes line `n` |
| `nem.buf.remove_line(n)` | delete line `n` |

`insert_line` accepts `line_count() + 1` to append, the same range
`table.insert` takes.

The whole buffer, and facts about it:

| Call | Does |
|---|---|
| `nem.buf.text()` | the entire buffer as one string |
| `nem.buf.set_text(s)` | replace the entire buffer |
| `nem.buf.path()` | the file path, or `""` |
| `nem.buf.modified()` | whether there are unsaved changes |

`set_text` compares what you give it against what is there and rewrites only the
lines that actually differ. So reading the buffer, transforming the string, and
writing it back does not disturb your mark, other windows' cursors, or your undo
history any more than the real change requires. Writing back identical text does
nothing at all.

Every change goes through the same edit machinery the built-in commands use, so
anything a script does is undoable with `C-/`.

## Hooks

```lua
-- Strip trailing whitespace from every line before saving.
nem.hook("before-save", function(buf)
  for i = 1, nem.buf.line_count() do
    nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
  end
end)
```

Hooks fire around command dispatch and are keyed on command name, so a hook
never needs to know how the editor is wired.

### Shelling out is not possible

A hook cannot run `gofmt` or any other external program. Scripts do not get Lua's
`io` or `os` libraries, so there is no way to spawn a process or read a file —
see [What scripts can reach](#what-scripts-can-reach). A formatter written in Lua
against `nem.buf` works; a formatter that calls out to a binary does not, yet.

### One Lua idiom to avoid

Do not call a method directly on a concatenation involving a `nem.` function:

```lua
-- Crashes: the interpreter faults on this shape.
for line in (nem.buf.text() .. "\n"):gmatch("([^\n]*)\n") do end

-- Fine: land the string in a local first.
local src = nem.buf.text() .. "\n"
for line in src:gmatch("([^\n]*)\n") do end
```

This is a defect in the Lua interpreter nem embeds, not in your config. It is
caught safely — you get an error in the echo area rather than a lost buffer — but
the error is unhelpful, so it is worth recognising. Iterating with
`for i = 1, nem.buf.line_count()` avoids the shape entirely.

## When your config has a bug

A broken config **never takes the editor down with it.** Config loading and every
callback run under `pcall`; a failure shows up in the echo area with the Lua
traceback, and nem carries on with the built-in default for whatever failed. You
can fix the file and restart without having lost anything.

## What scripts can reach

Scripts get the `nem` table plus the pure-computation half of Lua's standard
library: `string`, `table`, `math`, and the base functions.

They do **not** get `io`, `os`, `debug`, or `package`, and `dofile`, `loadfile`
and `require` are removed. So a config cannot open a file, spawn a process, read
an environment variable, or load more Lua from disk. It also has no route to the
screen or to the editor's internals except through the documented `nem` calls.

Two reasons for starting this tight. Relaxing a capability boundary later is
harmless, while tightening one breaks configs people have already written. And
keeping the surface small is what lets the internals move without breaking your
config, and would let a sandboxed runtime be swapped in later if scripts ever
come from anyone but you.

`nem.api_version` is the version of this surface, so a config written against a
future incompatible one can say so instead of failing in pieces.
