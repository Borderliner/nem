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
nem.set("tab-width", 4)        -- display width of a tab; default 8
nem.set("scroll-margin", 3)    -- lines of context kept around point; default 2
nem.set("undo-style", "linear")-- "linear" is the default and currently the only mode
```

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

## Hooks

```lua
nem.hook("before-save", function(buf)
  if buf.path:match("%.go$") then
    nem.run("gofmt-buffer")
  end
end)
```

Hooks fire around command dispatch and are keyed on command name, so a hook
never needs to know how the editor is wired.

## When your config has a bug

A broken config **never takes the editor down with it.** Config loading and every
callback run under `pcall`; a failure shows up in the echo area with the Lua
traceback, and nem carries on with the built-in default for whatever failed. You
can fix the file and restart without having lost anything.

## What scripts can reach

Scripts get the `nem` table and nothing else — no filesystem handles into the
editor's internals, no direct access to buffers other than through the documented
API. That boundary exists so the scripting surface can stay stable even as the
internals move, and so a future sandboxed runtime could be swapped in without
breaking the scripts you have written.
