# Lua API

Everything a script can reach is in the `nem` table: five functions, a table
for the buffer, and a version number. Nothing else of the editor is exposed,
which is what lets nem change inside without breaking your config. Where the
script lives and how it is loaded is on [Configuration](Configuration).

| | |
|---|---|
| [`nem.set(name, value)`](#nemset) | Change a setting |
| [`nem.bind(key, command [, mode])`](#nembind) | Bind a key to a command |
| [`nem.command(name, doc, fn)`](#nemcommand) | Define a command |
| [`nem.run(name)`](#nemrun) | Run a command, from inside a command or hook |
| [`nem.hook(name, fn)`](#nemhook) | Run a function around a command |
| [`nem.buf`](#nembuf) | Read and change the current buffer |
| [`nem.api_version`](#nemapi_version) | The version of this API |

## nem.set

```
nem.set(name, value)
```

Changes a setting. [Settings](Settings) lists them all, with the values each
takes and its default. Settings are applied once the whole script has run, so
the last value set wins.

```lua
nem.set("tab-width", 4)
nem.set("completion-style", "popup")
nem.set("which-key-delay", 500)
```

An unknown name, a value of the wrong type, or a number out of range raises an
error saying what is allowed.

## nem.bind

```
nem.bind(key, command)
nem.bind(key, command, mode)
```

Binds `key`, in [key notation](Configuration#key-notation), to the command
named `command` - the same name `M-x` runs it by, and that
[Commands](Commands) lists. A binding replaces whatever the key was bound to.

```lua
nem.bind("<f5>", "save-buffer")
nem.bind("C-x C-b", "switch-to-buffer")
nem.bind("M-o", "other-window")
```

With a `mode`, the key is bound only in that mode's buffers, where it comes
before the global keys:

| Mode | Its buffers |
|---|---|
| `"dired"` | Directory listings |
| `"wdired"` | A listing whose file names are being edited, after `C-x C-q` |
| `"grep"` | The results of a project search (`C-x p g`) or of occur (`M-s o`) |
| `"compilation"` | The output of `M-x compile` and `M-&` |

```lua
nem.bind("k", "dired-do-delete", "dired")     -- k deletes, as D does
nem.bind("-", "dired-up-directory", "dired")
nem.bind("C-c C-s", "wdired-finish-edit", "wdired")
```

[Keybindings](Keybindings) shows what each mode binds already.

A key can be bound to a command before the command is defined, so a config can
bind first and define later. A key still bound to nothing once the whole config
has run is reported. Binding a sequence under a prefix that is itself bound to
a command - `C-x C-f C-a`, say - is an error naming the binding in the way.

`C-c` is yours: nem binds nothing under it globally, and only the
compilation and wdired modes use `C-c C-c` and `C-c C-k` there, as emacs's do.

## nem.command

```
nem.command(name, doc, fn)
```

Defines a command called `name`, which runs `fn` with no arguments. `doc`
describes it, and is required: it is what `M-x` and `<f1> k` show.

```lua
nem.command("duplicate-line", "Copy the current line below itself.",
  function()
    local line, _ = nem.buf.point()
    nem.buf.insert_line(line + 1, nem.buf.line())
  end)
nem.bind("C-c d", "duplicate-line")
```

A command defined this way is in the same table as the built-in ones: `M-x`
completes it, a key can be bound to it, `<f1> k` describes it, a keyboard macro
can play it. An error in it is shown in the echo area, and the command stops
there. Whatever it changed, before an error or without one, is one change:
one `C-/` takes it all back.

## nem.run

```
nem.run(name)
```

Runs the command called `name`, built-in or defined in Lua, as if its key had
been pressed. Only inside a command or a hook: at load time there is nothing
to run it on. A command that fails raises an error.

```lua
-- Comment the line, or uncomment it, and go on to the next.
nem.command("comment-line-and-down", "Toggle the line's comment, then move down.",
  function()
    nem.run("comment-dwim")
    nem.run("next-line")
  end)
nem.bind("C-c ;", "comment-line-and-down")
```

A command that asks something - `C-x b`, `C-x k` - asks it when run this way
too, and waits for the answer.

## nem.hook

```
nem.hook(name, fn)
```

Runs `fn` whenever the hook called `name` fires. There are two:

| Hook | Fires |
|---|---|
| `before-save` | Before `C-x C-s` (`save-buffer`) or `C-x C-w` (`write-file`) writes the buffer |
| `after-save` | After either has written it |

`fn` gets a table describing the buffer being saved:

| Field | |
|---|---|
| `path` | The file's path |
| `name` | The buffer's name |
| `modified` | Whether it has unsaved changes |

```lua
-- Keep trailing whitespace out of Go files.
nem.hook("before-save", function(buf)
  if buf.path:match("%.go$") then
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
    end
  end
end)
```

Inside a hook, `nem.buf` is the buffer being saved. Every function hooked to a
name runs, in the order they were added, even if one fails; a failure is shown
in the echo area, and does not stop the save. A name that is not `before-…` or
`after-…` is an error.

## nem.buf

The current buffer, from inside a command or a hook. Outside one - while
`init.lua` is loading - there is no buffer, and each of these raises an error
saying so.

**Lines are numbered from 1**, everywhere: as Lua counts, as the mode line
shows, and as `M-x goto-line` takes them. A line number that does not exist is
an error, not an empty string. Columns are counted in characters, also from 1.

### Where the cursor is

| Call | Returns or does |
|---|---|
| `nem.buf.line()` | The text of the line the cursor is on |
| `nem.buf.replace_line(s)` | Replace that line's text with `s` |
| `nem.buf.point()` | The cursor's line and column: `local line, col = nem.buf.point()` |
| `nem.buf.set_point(line, col)` | Move the cursor; out of range, it goes to the nearest place there is |

### By line number

| Call | Returns or does |
|---|---|
| `nem.buf.line_count()` | How many lines the buffer has |
| `nem.buf.get_line(n)` | The text of line `n` |
| `nem.buf.set_line(n, s)` | Replace line `n`'s text with `s` |
| `nem.buf.insert_line(n, s)` | Insert `s` as a new line `n`, moving the lines from `n` on down; `line_count() + 1` appends |
| `nem.buf.remove_line(n)` | Delete line `n` |

The text given to `replace_line`, `set_line` and `insert_line` is one line, and
may not hold a newline: `set_text` changes the number of lines.

### The whole buffer

| Call | Returns or does |
|---|---|
| `nem.buf.text()` | The whole buffer as one string, lines joined by `"\n"` |
| `nem.buf.set_text(s)` | Replace the whole buffer with `s` |
| `nem.buf.path()` | The buffer's file, or `""` if it has none |
| `nem.buf.modified()` | Whether it has unsaved changes |

### How changes are made

Every change goes through the same editing as the built-in commands, so
everything a script changes is undone by `C-/`. A command's changes, however
many, undo as one, and so do those of the hooks run for one save.

Changes are as small as they can be. `set_line` rewrites only the characters
that differ, and `set_text` compares what it is given with what is there and
rewrites only the lines that changed. So reading the buffer, transforming it
and writing it all back disturbs the mark, other windows' cursors and the undo
history no more than the real change does, and writing back the same text
changes nothing at all.

### One shape to avoid

The Lua interpreter nem embeds faults when a method is called straight on a
concatenation that involves a `nem` call:

```
-- Faults: a method on (nem.buf.text() .. "\n").
for line in (nem.buf.text() .. "\n"):gmatch("([^\n]*)\n") do end

-- Works: the string in a local first.
local src = nem.buf.text() .. "\n"
for line in src:gmatch("([^\n]*)\n") do end
```

It is caught - the error goes to the echo area and nothing is lost - but the
message does not say what went wrong. Going by line number,
`for i = 1, nem.buf.line_count()`, avoids the shape altogether.

## nem.api_version

A number, the version of this API: `1`. Should a later nem change the API in
a way that breaks configs, the number goes up, so a config can check it and
say what it needs rather than fail in pieces.

```lua
if nem.api_version ~= 1 then
  error("this config is written for nem's API version 1")
end
```

## What a script cannot do

Scripts have Lua's `string`, `table` and `math`, and its base functions, but not
`io`, `os`, `debug` or `package`, nor `require`, `dofile` or `loadfile`. So a
script cannot open a file, run a program - no calling `gofmt` from a hook - or
read the environment. A formatter written in Lua against `nem.buf` works; one
that runs a program does not. For running programs there are `M-!`, `M-|`,
`M-&` and `M-x compile`.

A command or hook that runs for more than five seconds is stopped.
