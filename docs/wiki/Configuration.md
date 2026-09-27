# Configuration

nem is configured with a Lua script, `init.lua`. It is a real program, not a
list of options: anything you can compute, you can configure. What a script
can call is on [Lua API](Lua-API); the options it can set are on
[Settings](Settings).

## Where init.lua lives

| System | Path |
|---|---|
| Linux, BSD | `~/.config/nem/init.lua`, or `$XDG_CONFIG_HOME/nem/init.lua` |
| macOS | `~/Library/Application Support/nem/init.lua` |
| Windows | `%AppData%\nem\init.lua` |

Without one, nem runs on its defaults. The repository has an
[example](https://github.com/Borderliner/nem/blob/main/examples/init.lua) to
start from.

## How it is loaded

`init.lua` runs once, at startup, after every built-in command has been
registered and every default key bound. So whatever it binds replaces the
default, and a command it defines can take any name not already taken.

It runs top to bottom, like any Lua script. Bindings (`nem.bind`), commands
(`nem.command`) and hooks (`nem.hook`) are made as it goes, and settings
(`nem.set`) are applied once it has finished, so the last value set wins.
Editing the buffer (`nem.buf`) and running commands (`nem.run`) are for inside
a command or a hook: at startup there is nothing yet to act on.

A change to `init.lua` takes effect the next time nem starts.

## When your config has a bug

A broken config never takes the editor down with it. Loading it, and every
command and hook it defines, runs protected, and an error is shown in the echo
area rather than ending anything. An error while loading stops the script where
it happened: what came before it is in effect, and nem starts with its defaults
for the rest. Fix the file and restart; nothing is lost.

A few mistakes are caught on purpose rather than left to misbehave:

- `nem.set` with a setting that does not exist names the ones that do. A value
  of the wrong type, or out of range, says what is allowed.
- `nem.bind` with a key that cannot be parsed, or one whose prefix is already a
  command of its own, says which two bindings conflict.
- A key bound to a command that nothing ever defined is reported once the whole
  config has run: `init.lua: 1 binding(s) name no command`.
- A hook name that is not `before-…` or `after-…` is refused.
- A command or hook that runs for more than five seconds is stopped, so a loop
  that never ends cannot freeze the editor.

## What scripts can reach

Scripts get the `nem` table and the pure half of Lua's standard library:
`string`, `table`, `math`, and the base functions.

They do **not** get `io`, `os`, `debug` or `package`, and `dofile`, `loadfile`
and `require` are gone. So a config cannot open a file, run a program, read an
environment variable or load more Lua from disk, and it reaches the editor only
through the calls on [Lua API](Lua-API).

Starting this tight is deliberate. Loosening a boundary later breaks nothing,
where tightening one breaks configs people have written; and a small surface is
what lets nem's insides change without breaking yours. `nem.api_version` is the
version of that surface, so a config can tell if it ever changes incompatibly.

## Key notation

Keys are written as emacs writes them, in `nem.bind` and throughout this wiki:

| Written | Means |
|---|---|
| `C-x` | Control+x |
| `M-x` | Meta+x: Alt+x, or Escape then x |
| `C-M-f` | Control+Meta+f |
| `C-x C-f` | Two keys in turn: `C-x`, then `C-f` |
| `RET` `TAB` `SPC` `ESC` | Return, Tab, Space, Escape |
| `DEL` or `<backspace>` | Backspace |
| `<delete>` | Delete, which deletes forward |
| `<up>` `<down>` `<left>` `<right>` | The arrows |
| `<home>` `<end>` `<pgup>` `<pgdn>` | Home, End, Page Up, Page Down |
| `<f1>` … `<f12>` | The function keys |
| `S-<up>` | Shift+Up |
| `<backtab>` | Shift+Tab |

Shift is written only on named keys. On a character the character carries its
case: write `X`, never `S-x`, which is an error.

### DEL is Backspace

In emacs's notation, and so in nem's, `DEL` is **Backspace**, which deletes
backward; forward-delete is `<delete>`. So `nem.bind("DEL", "delete-char")`
makes Backspace delete forward, which is rarely what anyone means: that key
wants `delete-backward-char`.

### C-h and <f1>

`<f1>` is the help prefix: `<f1> b` lists the bindings, `<f1> k` describes a
key. `C-h` is bound to the same, but works only in terminals that speak the
extended keyboard protocol - kitty, foot, Ghostty, recent xterm. Elsewhere the
terminal sends `C-h` as Backspace before nem sees it. Use `<f1>` to be sure.

### Other keyboard layouts

With a Persian, Arabic, Hebrew, Russian, Ukrainian or Greek layout switched on,
Control on the key where x is sends `C-ط`, or `C-ч`, rather than `C-x`. nem reads
such a key as the Latin key it sits on, so every binding works whatever the
layout; the [`keyboard-layout`](Settings#keyboard-layout) setting changes that.
Bind keys by their Latin names.

## Where nem keeps its state

Never beside your files. Backups, autosaves, what was rescued when nem had to
stop, reports of its own bugs, and the history of what you typed at each prompt
all live under `$XDG_STATE_HOME/nem` - usually `~/.local/state/nem`, and
`%LocalAppData%\nem` on Windows - with each file's path mirrored beneath it:

```
~/.local/state/nem/backups/home/you/project/main.go~
~/.local/state/nem/autosave/home/you/project/main.go#
```

- **Backups**: a file's previous contents, the first time it is saved in a
  session. The [`backup`](Settings#backup) setting turns them off.
- **Autosaves**: an unsaved buffer's text, after a pause or a few hundred
  keystrokes; see [`autosave-idle`](Settings#autosave-idle). When a file has an
  autosave newer than itself, opening it says so, and `M-x recover-file` brings
  the text back.
- **Rescued**: when the terminal closes, an SSH connection drops or the system
  shuts down, every unsaved buffer is written away before nem exits - a file's
  to its autosave, one with no file, such as `*scratch*`, to `rescued/`.
- **Faults**: a bug in nem is caught as it happens. The unsaved work is
  autosaved, the session carries on, and a report for sending in goes to
  `faults/`.

A save replaces the file whole or not at all, and asks before overwriting a
file that something else changed on disk since nem read it.
