# Settings

Every option `nem.set` takes, in [`init.lua`](Configuration):

```lua
nem.set("tab-width", 4)
nem.set("theme", "light")
```

A name nem does not know, a value of the wrong type or one out of range is an
error that says what is allowed; see
[Configuration](Configuration#when-your-config-has-a-bug).

| Setting | Values | Default |
|---|---|---|
| [`auto-pair`](#auto-pair) | `true`, `false` | `true` |
| [`auto-revert`](#auto-revert) | `true`, `false` | `true` |
| [`autosave-idle`](#autosave-idle) | seconds, 0 to 3600 | `30` |
| [`backup`](#backup) | `true`, `false` | `true` |
| [`bidi`](#bidi) | `true`, `false`, `"auto"` | `"auto"` |
| [`clipboard`](#clipboard) | `"osc52"`, `"off"` | `"osc52"` |
| [`completion-rows`](#completion-rows) | 1 to 200 | `10` |
| [`completion-style`](#completion-style) | `"bottom"`, `"popup"` | `"bottom"` |
| [`delete-selection`](#delete-selection) | `true`, `false` | `true` |
| [`fill-column`](#fill-column) | 10 to 1000 | `70` |
| [`hl-line`](#hl-line) | `true`, `false` | `true` |
| [`icons`](#icons) | `true`, `false`, `"auto"` | `"auto"` |
| [`keyboard-layout`](#keyboard-layout) | `"auto"`, `"arabic"`, `false` | `"auto"` |
| [`line-numbers`](#line-numbers) | `true`, `false` | `true` |
| [`line-wrap`](#line-wrap) | `true`, `false` | `false` |
| [`open-binary`](#open-binary) | `"ask"`, `"system"`, `"text"` | `"ask"` |
| [`scroll-margin`](#scroll-margin) | 0 to 1000 | `2` |
| [`shell`](#shell) | a program | `""`: `/bin/sh`, or `cmd.exe` on Windows |
| [`syntax`](#syntax) | `true`, `false` | `true` |
| [`tab-width`](#tab-width) | 1 to 64 | `8` |
| [`theme`](#theme) | `"auto"`, `"dark"`, `"light"` | `"auto"` |
| [`undo-style`](#undo-style) | `"linear"` | `"linear"` |
| [`which-key-delay`](#which-key-delay) | milliseconds, 0 to 10000 | `1000` |

## Display

### line-numbers

A gutter of line numbers beside the text, the current line's picked out. It is
drawn outside the text, so a number can never be selected, marked or copied. A
window too narrow to spare the room drops it. `C-x n` turns it off and on while
nem runs.

### line-wrap

Fold a line wider than its window into rows, rather than cutting it off at the
edge with a `$` and scrolling sideways to follow the cursor. A row breaks after
a space or a tab, so words stay whole; only a word longer than a whole row is
broken inside it. The line's number goes beside its first row, and the band
under the current line runs under all of its rows.

Wrapped, `C-n` and `C-p` go a row on screen at a time, keeping the cursor's
column in the row, as emacs's do; `C-v`, `M-v` and `C-l` count rows too. `C-a`,
`C-e` and every command that works on lines still work on the whole line.
`C-x x t` turns wrapping on and off while nem runs.

### hl-line

A faint band under the line the cursor is on, beside its number in the gutter.
In dired and search results the selected row has a stronger bar either way.

### tab-width

How many columns a tab takes on screen, 1 to 64. It changes only how tabs are
drawn: what `TAB` inserts comes from the file's `.editorconfig`, or from the
indentation already in the file.

### scroll-margin

How many lines of context to keep above and below the cursor when scrolling,
0 to 1000.

### syntax

Colour code. Every language nem colours is described by a short definition,
and dozens are built in, from Ada to Zig; a file without an extension is
matched by its `#!` line. You can change how a language is coloured, or add
one nem doesn't know: see [Languages](Languages).

### theme

The palette code is coloured in. nem never paints a background, so it sits
inside your terminal's own colours - and colours that stand out on black wash
out on white, so there are two palettes. `"auto"` guesses from the `COLORFGBG`
variable some terminals set, and takes dark when there is none; if code looks
washed out, set `"light"`.

### bidi

Lay out right-to-left text - Persian, Arabic, Hebrew - in the order it is read,
with Arabic letters joined, and set a line that starts right to left against the
right edge. Only the drawing changes: the cursor, the region and every command
still work on the text as it is stored, as in emacs. `"auto"` does it unless the
terminal lays such text out itself - Konsole, mlterm, and GNOME Terminal and
others built on VTE - which would reverse it a second time.

### icons

An icon beside each name in dired and in the file and buffer prompts. They need
a [Nerd Font](https://www.nerdfonts.com), or a terminal that ships its symbols -
Ghostty, Kitty and WezTerm do; without one they draw as empty boxes. `"auto"`
shows them in those terminals, and where an installed font has the glyphs. Over
SSH nem cannot see the fonts of the terminal you connect from, so set `true`
there if it has one.

## Editing

### delete-selection

Typing replaces the selection, and `DEL` or `C-d` deletes it, as in nearly every
editor; `C-y` replaces it with what is yanked. The replacement is one undo step.
Emacs leaves this off; `false` restores its behaviour, where a selection sits
inert until a command uses it. `C-w`, `M-w` and `TAB` act on the region either
way.

### auto-pair

Typing `(`, `[`, `{`, `"`, `'` or `` ` `` inserts its partner too, with the
cursor between them; typing the closer when it is next steps over it, and `DEL`
between an empty pair deletes both. With a selection, typing an opener wraps the
selection instead of replacing it. Not in prompts: a search for `(` searches for
`(`.

### fill-column

The width `M-q` fills paragraphs to, 10 to 1000.

### undo-style

How undo works. `"linear"` is the only style so far: `C-/` undoes, `M-_` redoes.

### keyboard-layout

Read keys typed on another keyboard layout as the keys they sit on. On a
Persian, Arabic, Hebrew, Russian, Ukrainian or Greek layout, Control on the key
where x is sends `C-ط` or `C-ч`, and without this no binding would work until
you switched back to Latin. Only keys that are commands are read this way;
letters typed into text stay as they are.

`"auto"` knows all of those layouts, and follows the Persian keyboard for the
five letters it and the Arabic one put on different keys (ط د ذ ز ظ);
`"arabic"` follows the Arabic keyboard for those. `false` reads every key as the
terminal sends it.

## Prompts and discovery

### completion-style

Where `M-x`, `C-x C-f`, `C-x b` and the other prompts list their candidates.
`"bottom"` puts the prompt at the foot of the screen with the candidates under
it, as emacs's Vertico does, the windows shrinking to make room. `"popup"` shows
the same list in a panel in the middle of the screen.

### completion-rows

How many candidates are listed at once, 1 to 200. A short screen shows fewer.

### which-key-delay

How long, in milliseconds, a prefix key such as `C-x` waits before listing the
keys that can follow it, 0 to 10000. `0` never lists them.

## Files

### open-binary

What opening a file that is not text does - a PDF, a picture, a song, an
archive. `"ask"` asks each time: `s` opens it with the system's app for that
kind of file, `t` opens it as text, and `S` or `T` answers the same for every
such file for the rest of the session. `"system"` and `"text"` answer without
asking. Where there is no desktop to open things on, such as over plain SSH,
files open as text.

### auto-revert

Read a file again when it changes on disk - a `git checkout`, a formatter,
another editor - as long as its buffer has no edits of its own. A buffer with
edits is left alone, and saving it asks before overwriting the changed file.

### backup

Keep a copy of a file's previous contents the first time it is saved in a
session, under nem's [state directory](Configuration#where-nem-keeps-its-state),
never beside the file.

### autosave-idle

After how many seconds without a keystroke unsaved buffers are autosaved, 0 to
3600. Three hundred keystrokes without such a pause bring on an autosave too.
`0` turns autosaving off. Autosaves go to nem's
[state directory](Configuration#where-nem-keeps-its-state); `M-x recover-file`
brings one back.

## System

### clipboard

`"osc52"` puts what `C-w` and `M-w` kill on the system clipboard too, by the
OSC 52 escape sequence, which works over SSH (in tmux, set
`set -g allow-passthrough on`). `C-y` then yanks what is on the system
clipboard, read with `wl-paste`, `xclip`, `xsel` or `pbpaste`, whichever is
there; `M-y` goes on back through nem's own kills. `"off"` keeps nem away from
the clipboard both ways.

### shell

The program `M-!`, `M-|`, `M-&` and compile commands run in, given the command
after `-c`. Empty, the default, means `/bin/sh` - what Makefiles use, and the
same everywhere - or `cmd.exe` on Windows.

```lua
nem.set("shell", "/bin/bash")
```
