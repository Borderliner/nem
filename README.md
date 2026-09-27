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

From source, with [xmake](https://xmake.io) and Go 1.27 or later:

```sh
git clone https://github.com/Borderliner/nem
cd nem && xmake
xmake run nem file.txt
```

```sh
xmake                                  # static, stripped build/<plat>/<arch>/release/nem
xmake f -m debug && xmake              # with symbols, for a debugger
xmake f --goos=windows --goarch=arm64 && xmake   # cross-compile
xmake install --user                   # into ~/.local/bin, no root needed
xmake install [-o DIR]                 # into DIR/bin, default /usr/local/bin
xmake test                             # gofmt, go vet, tests with the race detector
xmake bench -p ./editor -f Redraw      # benchmarks; xmake bench --help
```

The build sets `CGO_ENABLED=0`, which matters: Go turns cgo on by default when
a C compiler is present, and a dependency pulls in `os/user`, which links libc
for NSS lookups. With it off the binary is **fully static** — no libc, no glibc
version skew, runs on musl and Alpine — and strips to about 5 MB. Without
xmake, the same build is:

```sh
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o nem ./cmd/nem
```

Being cgo-free is also why every platform cross-compiles from any other, so
releases for Linux, macOS and Windows on both amd64 and arm64 all come off one
machine. Windows binaries are published but lightly tested; state lives under
`%LOCALAPPDATA%` there rather than the XDG path used elsewhere.

## What it does

**Editing** — the emacs motion and editing set: `C-f`/`C-b`/`C-n`/`C-p`,
`M-f`/`M-b`, `C-a`/`C-e`, `M-<`/`M->`, `C-d`, `C-k`, `M-d`, `C-t`, `M-u`/`M-l`/`M-c`.
Cursor motion stops at grapheme boundaries, so a decomposed `é` or a ZWJ family
emoji is one press, not seven.

**The rest of the editing set** — `M-;` comments or uncomments the line or the
region in the file's own syntax, `M-/` completes a word from words already in
the text (again for the next), `M-q` re-wraps a paragraph - a comment block
stays a comment block - and `M-{`/`M-}` move by paragraph. `C-M-f`/`C-M-b` jump
over a bracketed group, string or word and `C-M-k` kills one. `M-z` kills up to
a character, `M-SPC` and `M-\` squeeze or delete the spaces around point, `M-^`
joins a line to the one above and `M-m` goes to its first non-blank character.
`M-=` counts words and `C-x =` describes the character under point.

**Indentation** — `TAB` indents the way the file already does: a tab in Go or
a Makefile, spaces elsewhere, as many as the file uses. nem asks the file's
`.editorconfig` first, then looks at how the lines already there are indented,
and failing both goes by the language: two spaces for JavaScript, TypeScript,
JSON, YAML, Lua, Ruby, HTML and CSS, four for Python, Rust, C, Java, Kotlin,
C#, PHP, shell and SQL, and a tab for anything it does not recognise. With a
region, `TAB` shifts every line it touches a level right and keeps the region,
so it can be pressed again; `S-TAB` shifts them back, and `C-x TAB` shifts
them by the prefix argument in columns (`C-u -2 C-x TAB` is two to the left).
Only indentation moves, so shifting left never eats text.

**Regions and lines** — `C-x C-u` and `C-x C-l` upcase and downcase the
region. `M-x sort-lines` sorts the lines the region covers (in reverse with
`C-u`), `M-x reverse-region` turns them upside down, and `M-x
delete-trailing-whitespace` strips the spaces and tabs from the ends of the
buffer's lines, or the region's. Each is one undo.

| Key | Does |
|---|---|
| `TAB` | indent at point the file's way; with a region, shift its lines a level right |
| `S-TAB` · `C-x TAB` | shift the line, or the region's lines, a level left · by `C-u` columns |
| `C-x C-u` · `C-x C-l` | upcase · downcase the region |
| `C-M-%` | query-replace a regexp, `\&` and `\1`…`\9` in the replacement |
| `M-s o` · `M-g i` | list the lines matching a regexp · go to a definition by name |
| `M-!` · `M-\|` · `M-&` | run a shell command · on the region · in the background |

**It remembers** — `M-p` and `M-n` bring back what you typed at a prompt, in
this session or an earlier one, and `M-x` lists the commands you used last
first. `C-x C-r` opens a recent file, and a file opens again where you left
it. `C-u C-SPC` jumps back to where the mark was - where a search started,
where `M->` jumped from - and again for the place before that. It is all kept
in `~/.local/state/nem/memory.json`.

**Keyboard macros** — `F3` starts recording, `F4` stops, and `F4` plays it
back; `C-u 3 F4` plays it three times and `C-u 0 F4` until it fails, which is
how a macro runs down the rest of a file. `F3` while recording inserts a
counter that counts on as it plays. `C-x (`, `C-x )` and `C-x e` work too.
Searches and prompts inside a macro replay exactly, and playback stops at the
first thing that fails, as in emacs.

**Brackets and quotes pair themselves** — `(` gives `()` with point between,
typing `)` steps over it, and Backspace between the two removes both. Over a
selection, `(` or `"` wraps it. It holds back in front of a word, and an
apostrophe stays single; `nem.set("auto-pair", false)` turns it off.

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
| `RET` `^` | open the file or directory at point (`..` goes up) · go up a level |
| `M-<down>` `M-<up>` | the same: open what is at point · go up a level |
| `M-p` `M-n` | back and forward through the directories listed, as a browser does |
| `n` `p` | next and previous file |
| `m` `u` `t` `U` | mark · unmark · invert the marks · unmark all |
| `d` `x` | flag for deletion · delete the flagged files |
| `D` `R` `C` | delete · rename or move · copy — the marked files, or the one at point |
| `+` `g` `w` `o` `q` | new directory · re-read · copy the name · open in the other window · leave |
| `E` | open with the system's app — the marked files, or the one at point |
| `C-x C-q` | edit the file names as text, to rename the files (below) |

Each name has an icon for its kind - a folder, a language's mark, a picture,
an archive - and so do the candidates at `C-x C-f` and `C-x b`. They need a
[Nerd Font](https://www.nerdfonts.com), or a terminal that ships its symbols
(Ghostty, Kitty and WezTerm do), so they turn on by themselves only where that
looks true: in those terminals, or when an installed font has the glyphs. Over
SSH nem cannot see your fonts; `nem.set("icons", true)` or `false` settles it.

Deleting asks you to type `yes`, nothing is replaced without asking, and a
rename carries any buffer visiting the file along with it. One listing follows
you around the tree rather than a new buffer piling up per directory.

`C-x C-q` makes the names editable, as emacs's wdired does, so renaming is
text editing: `M-%` turns `IMG_` into `holiday-` across every name, a keyboard
macro numbers them, `C-a` and `C-e` go to either end of a name. Only names
change; the rest of each line stays put. `C-c C-c` renames the files to match
and `C-c C-k` forgets the edits. The renames happen together, so two files can
swap names, and all or none of them do: a name already taken, or two files
given one name, is refused before anything moves. A name with a `/` moves the
file into that directory, and an emptied name flags the file for `x`.

**Projects** — the repository a file is in is its project (or the nearest
directory holding a `.projectile` file, or failing both, a build file such as
`go.mod` or `package.json`), and `C-x p` works on the whole of it, as
projectile does. The prefix is emacs's own `project.el` one, since `C-c` is
yours.

| Key | Does |
|---|---|
| `C-x p f` | find any file in the project by a few letters of its path; the ones opened last come first |
| `C-x p p` | switch to another project nem has seen, and find a file in it |
| `C-x p g` | search every file for a regexp, listing the matching lines (case is ignored unless you type a capital) |
| `C-x p r` | query-replace across the project, file by file — `Y` does all the rest, `N` skips a file |
| `C-x p b` `C-x p e` | switch to one of its buffers · open one of its recent files |
| `C-x p d` `C-x p D` | list one of its directories · list its top |
| `C-x p t` | go from a file to its test and back: `foo.go` ↔ `foo_test.go`, `app.ts` ↔ `app.test.ts`, … |
| `C-x p S` `C-x p k` | save its modified files · kill its buffers |
| `C-x p c` | compile the project, from its top directory (below) |

In the search results `RET` opens a match in the other window, `n` and `p`
show each in turn there, `{` and `}` jump between files, and `g` searches
again; `M-g n` and `M-g p` step through the matches from any buffer. Open
files are searched as they are, unsaved edits and all.

A project's files come from `git ls-files` in a repository, so `.gitignore`
is honoured; elsewhere the tree is walked, passing over `node_modules` and its
kind. A `.projectile` file narrows the list with projectile's syntax: `-/build`
or `*.min.js` leaves things out, `+/src` keeps only `src`.

**Compiling** — `M-x compile` runs a build, the tests, a linter, anything, and
shows the output beside you as it comes; `C-x p c` does it from the project's
top. The first time it offers what the project's build files suggest - `go
build ./...` beside a `go.mod`, `cargo build` beside a `Cargo.toml`, `make -k`
otherwise - and after that the command you ran there last. Every line naming a
place in a file leads there: errors and warnings from gcc, clang, go, rust,
tsc and javac, Python tracebacks, JavaScript stack frames, go test's failures.
`M-g n` and `M-g p` step through them from any buffer.

In the output, `RET` goes to the error on its line, `n` and `p` show each in
the other window, `g` runs the command again and `C-c C-k` stops it. A test
failure that names only `foo_test.go` finds the one file of the project it can
be, and asks which when several can.

**Shell commands** — `M-!` runs a command and shows its output: in the echo
area if it is a line, in a buffer if it is more. `C-u M-!` puts the output at
point instead. `M-|` runs a command with the region as its input, and `C-u
M-|` replaces the region with the output, which is how anything becomes a
filter: `C-u M-| sort`, `C-u M-| jq .`, `C-u M-| column -t`. A command that
fails changes nothing. `M-&`, or a command ending in `&`, runs in the
background into its own buffer. Commands run with `sh`, from the buffer's
directory; `C-g` stops one you are waiting for.

**Search** — `C-s` is genuinely incremental: it moves as you type, backspace
walks point back, and `C-g` returns you to where you started. Past the last
match `C-s` says it is failing, and pressing it again wraps round to the top;
`C-r` does the same going back, and turns a forward search round. `M-%` is
query-replace with `y`/`n`/`!`/`q`, and `C-M-%` does the same for a regexp,
with `\&` for the match and `\1`…`\9` for its groups in the replacement (few
terminals can send `C-M-%`; `M-x query-replace-regexp` always works). `M-x
replace-string` and `M-x replace-regexp` replace everything after point, or in
the region, without asking. `M-s o` is emacs's occur: every line of
the buffer matching a regexp, listed like a project search's results, each
leading back to its line. `M-g i` is imenu: a function, type, class or
heading of the buffer by name, with its line beside it - for Go, Python,
JavaScript and TypeScript, Rust, C and C++, Java and Kotlin, Ruby, Lua,
shell, Makefiles, Emacs Lisp and Markdown.

**Prefix keys, `M-x`, and `C-u`** — `C-x` is a real prefix keymap, `M-x` completes
over every command by name, with the keys that run each one shown beside it,
and `C-u` takes numeric and negative arguments.

**Undo** — linear undo and redo (`C-/` and `M-_`). Typing coalesces into one
undo unit; a new edit after undoing discards the redo branch.

**Help** — `<f1> b` lists every binding, `<f1> k` describes a key. Pause on a
prefix like `C-x` for a second and everything that can follow it is listed
where completions go - at the bottom, or in the centred panel with
`completion-style` set to `"popup"`.

**Completion** — `M-x`, `C-x C-f` and `C-x b` list their candidates under the
prompt at the bottom of the screen, as Vertico does, and filter them as you
type, fuzzily: `fwc` finds `forward-char`. Matched characters are highlighted
so you can see why something matched, and the windows shrink to make room
rather than being covered. `nem.set("completion-style", "popup")` shows the
same list in a centred panel instead. `C-n` and `C-p` step through the list,
`C-v` and `M-v` (or `PgDn` and `PgUp`) page through it, and `M-<` and `M->`
jump to its first and last entries.

`RET` takes the highlighted entry, and on a directory it walks into it and
lists what is inside. The directory itself comes first, as `./` before you have
typed anything, so `C-x C-f RET` opens it in dired. `M-RET` takes exactly what
you typed, for a new file or buffer whose name happens to match an existing
one. Buffers are listed most recently visited first, so `C-x b RET` flips back
to the previous one.

In a file prompt `~/` is your home directory, and typing `~/` or `/` after the
directory the prompt opened on starts the path over from there, as emacs's
minibuffer does: `C-x C-f ~/notes/` needs nothing erased first.

**Files that aren't text** — a PDF, a photo, a song or a zip is not dumped into
a buffer as garbage. nem asks: `s` opens it with your system's app for that
type, `t` opens it as text anyway, and `S` or `T` answers the same for every
file of that type for the rest of the session. To answer without being asked,
set `nem.set("open-binary", "system")` or `"text"`. `M-x open-externally` hands
any file to the system app on purpose, and so does `E` in dired. With no desktop
to open things on (over plain SSH, say) files open as text, as before. Opened as
text, such a file is shown byte for byte as it is stored - uncoloured, never laid
out right to left - and its mode line says `binary`.

**Line numbers** — shown by default, and drawn outside the text so they can
never be selected or copied. `C-x n` toggles them. The line the cursor is on
lies on a faint band, its number picked out on it; `nem.set("hl-line", false)`
takes the band away. In dired and in search results the selected row is a
slightly stronger band, under the row's own colours.

**Long lines** — cut off at the window's edge with a `$`, the view scrolling
sideways to follow the cursor. `nem.set("line-wrap", true)`, or `C-x x t` while
nem runs, folds them into rows instead, broken after spaces so words stay whole;
`C-n` and `C-p` then go a row at a time, as emacs's do.

**Right-to-left text** — Persian, Arabic and Hebrew are written and shown
right to left, their letters joined, as emacs shows them: most terminals do
neither, so nem lays the text out itself, by the Unicode bidirectional
algorithm. In prose a line takes its direction from its first letter, and one
in Persian sits against the right edge; in code every line stays left to
right, with any Persian in it - a comment, a string - reading right to left
where it is. The arrow keys go the way the line reads.

Key bindings work whatever keyboard layout is switched on. With a Persian
keyboard, Ctrl on the x key sends `C-ط`; nem reads it as the `C-x` it sits on,
as emacs's reverse-im does - and the same for Arabic, Hebrew, Russian,
Ukrainian and Greek keyboards. Only where a key is a command: with Ctrl or Alt,
after a prefix (`C-x ب` is `C-x f`), in dired's listing, answering y or n.
Typed on its own, a letter is still the letter. Konsole, GNOME Terminal
and mlterm lay the text out themselves, so there nem leaves it to them;
`nem.set("bidi", true)` or `false` decides it.

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
autosave after 30 seconds' pause or 300 keystrokes, and a refusal to overwrite a
file that changed on disk underneath you. A save replaces the file whole or not
at all, so a full disk or a crash mid-save cannot leave it cut short. Closing
the terminal, a dropped SSH connection or a shutdown writes every unsaved buffer
away before nem exits, and a bug in nem is caught and reported rather than
taking your work with it. Nothing is written beside your file; it all goes under
`~/.local/state/nem`.

**`.editorconfig`** — a project's `.editorconfig` is honoured: its
indentation settings decide what `TAB` inserts, and `trim_trailing_whitespace`
and `insert_final_newline` are applied when a file is saved - to the buffer,
so what you see is what was written, and `C-/` brings the whitespace back.

**Files changed elsewhere** — a buffer with no edits of its own follows its
file: after a `git checkout`, a formatter, a build that generates code, it is
read again by itself, as emacs's `global-auto-revert-mode` has it. A buffer
you have edited is never touched behind your back. `C-x x g` reads any file
again on request, asking first if that throws edits away; `nem.set("auto-revert",
false)` leaves it to that.

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

Every setting, every Lua call and every key is in the
[wiki](https://github.com/Borderliner/nem/wiki): [Settings](https://github.com/Borderliner/nem/wiki/Settings),
[Lua API](https://github.com/Borderliner/nem/wiki/Lua-API),
[Keybindings](https://github.com/Borderliner/nem/wiki/Keybindings),
[Commands](https://github.com/Borderliner/nem/wiki/Commands) and
[Recipes](https://github.com/Borderliner/nem/wiki/Recipes).

## Not yet

Mouse support · undo tree · multi-line search patterns · indentation that
knows a language's syntax (TAB follows the file's style, not its braces)

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
