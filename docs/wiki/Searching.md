# Searching and finding

nem has ripgrep and fzf built in. There is nothing to install beside it, and they work the same on Linux, macOS and Windows.

| Key | Command | Does |
|---|---|---|
| `M-s r` | `rg` | Search every file of the project as you type, with ripgrep's options |
| `M-s f` | `fzf` | Open a file anywhere under the project by a few letters of its path |
| `M-s l` | `fzf-lines` | Go to a line of the buffer by a few letters of it |
| `C-x p g` | `project-find-regexp` | Search the project, every match listed in a results buffer |
| `C-x p f` | `project-find-file` | Find a file of the project by a few letters of its path |
| `M-s o` | `occur` | List every line of the buffer that matches a regexp |
| `C-s` `C-r` | `isearch-forward` `isearch-backward` | Search the buffer as you type |

`rg` and `fzf` work on the project of the buffer you are in: its repository, or the nearest directory with a build file such as `go.mod`. Outside a project they work on the buffer's directory. With `C-u` first (`C-u M-s r`, `C-u M-s f`) they ask which directory to use.

## rg: search as you type

`M-s r` lists the matching lines of every file as you type the pattern, and the matches arrive while the search goes on. Each line shows its file and line number, with the match picked out.

- `RET` goes to the highlighted match.
- `M-RET` lists every match in a results buffer instead, as `C-x p g` does. There `RET` visits a match, `n` and `p` step through them, and `g` searches again; the rest of its keys are under [Search results](Keybindings#search-results).
- `C-g` gives up.

The pattern is a Go regexp, typed as ripgrep's command line has it: options first, then the pattern. The pattern is the rest of the line, spaces included.

```
needle                   needle, in any case: case is smart
-t go -w count           count as a whole word, in Go files
-i -g '!vendor/**' TODO  TODO in any case, outside vendor
-F a.b(c)                a.b(c) as text, not a regexp
-u -t js fetch(          also search the files .gitignore leaves out
```

| Option | Means |
|---|---|
| `-i`, `--ignore-case` | Ignore case. |
| `-s`, `--case-sensitive` | Respect case. |
| `-S`, `--smart-case` | Ignore case unless the pattern has a capital letter. This is the default. |
| `-w`, `--word-regexp` | Match whole words only. |
| `-F`, `--fixed-strings` | Treat the pattern as text, not a regexp. |
| `-t TYPE`, `--type=TYPE` | Search only files of this type. May be repeated. |
| `-T TYPE`, `--type-not=TYPE` | Leave out files of this type. May be repeated. |
| `-g GLOB`, `--glob=GLOB` | Search only files matching the glob, or leave them out with a leading `!`. May be repeated. |
| `-u`, `--no-ignore` | Also search files that `.gitignore` leaves out. |
| `-e PATTERN`, `--regexp=PATTERN`, `--` | Take what follows as the pattern, for one that starts with a dash. |

Short options run together: `-wi` is `-w -i`. Quotes group, as in a shell: `-g '!*.min.js'`. If the pattern is not a valid regexp yet, for example a half-typed `fetch(`, nem looks for it as text rather than stopping to complain.

The types are ripgrep's names for them: `c`, `cpp`, `cs`, `css`, `csv`, `docker`, `elisp`, `go`, `html`, `java`, `js`, `json`, `kotlin`, `lua`, `make`, `markdown` (or `md`), `nix`, `php`, `proto`, `py`, `rb` (or `ruby`), `rust`, `sh`, `sql`, `swift`, `toml`, `ts` (or `typescript`), `txt`, `xml`, `yaml` and `zig`.

`C-x p g` takes the same options and lists its results in a buffer straight away.

A file open in nem is searched as its buffer has it, unsaved edits included. A search lists up to 2,000 lines as you type, and `M-RET` lists up to 10,000.

## fzf: find a file

`M-s f` lists every file under the project and narrows the list as you type, fzf style. In a git repository `.gitignore` is honoured. Elsewhere, directories such as `.git`, `node_modules` and `.venv` are passed over. `RET` opens the highlighted file.

Very large trees work too. The list stops at 200,000 files, and each keystroke ranks only what the previous keystroke left.

## fzf-lines: find a line

`M-s l` lists the lines of the buffer, numbered and in their own order, and narrows them as you type. The cursor follows the highlighted line, so you see it in place before you choose.

- `RET` leaves the cursor on the line, at the text that matched. The mark stays where you started.
- `C-g` puts the cursor back where it was.

## Narrowing a list: fzf's syntax

Every list under a prompt narrows the same way: `M-x`, `C-x C-f`, `C-x b`, `M-s f`, `M-s l` and the rest. Typed letters match fuzzily: they must appear in the candidate in order, with gaps allowed, so `fwc` finds `forward-char`. Matches that start words, and letters that run together, rank higher.

A query of several words, separated by spaces, is read as fzf reads it:

| Typed | Matches a candidate |
|---|---|
| `abc` | with `a`, `b` and `c` in that order, gaps allowed |
| `'abc` | with `abc` in it, just as typed |
| `^abc` | that starts with `abc` |
| `abc$` | that ends with `abc` |
| `^abc$` | that is `abc` |
| `!abc` | without `abc` in it; `!^abc` and `!abc$` work too |
| `abc \| def` | that matches `abc` or `def` |

Every word must match, in any order: in nem's own tree, `edi 'go !test` lists the Go sources under `editor`, tests left out. Case works as in search: a word typed in lower case ignores case, and a capital letter makes that word match case exactly. `\ ` puts a space inside a word.
