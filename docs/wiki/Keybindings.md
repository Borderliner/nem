<!-- Generated from nem's code by TestWikiReferenceIsCurrent in editor/wiki_test.go.
     Do not edit: change the code, then run NEM_UPDATE_WIKI=1 go test ./editor -run TestWikiReferenceIsCurrent. -->

# Keybindings

Every key nem binds out of the box. Keys are written in emacs notation: `C-x` is Control+x, `M-x` is Meta+x (Alt, or Escape then x), and `C-x C-f` is two keys in turn. [Configuration](Configuration#key-notation) has the whole notation.

Any key can be bound to another command, and any command to a key of your own, with [`nem.bind`](Lua-API#nembind). [Commands](Commands) lists every command, including those bound to no key, which `M-x` runs by name. Inside nem, `<f1> b` lists what is bound and `<f1> k` says what a key does.

**On this page:** [Global keys](#global-keys) · [Dired](#dired) · [Editing file names](#editing-file-names) · [Search results](#search-results) · [Compilation output](#compilation-output) · [In a prompt](#in-a-prompt) · [Answering a question](#answering-a-question) · [Arguments and repeats](#arguments-and-repeats)

## Global keys

These work everywhere, unless a mode or a prompt below gives a key a meaning of its own there.

### Motion

| Key | Command | Does |
|---|---|---|
| `C-f` | `forward-char` | Move point one grapheme forward. |
| `<right>` | `right-char` | Move one character to the right: forward, or backward in a line that reads right to left. |
| `C-b` | `backward-char` | Move point one grapheme backward. |
| `<left>` | `left-char` | Move one character to the left: backward, or forward in a line that reads right to left. |
| `C-n` `<down>` | `next-line` | Move point down one line, keeping the goal column. |
| `C-p` `<up>` | `previous-line` | Move point up one line, keeping the goal column. |
| `M-f` | `forward-word` | Move point to the end of the next word. |
| `M-b` | `backward-word` | Move point to the start of the previous word. |
| `C-a` `<home>` | `move-beginning-of-line` | Move point to the start of the line. |
| `C-e` `<end>` | `move-end-of-line` | Move point to the end of the line. |
| `M-<` | `beginning-of-buffer` | Move point to the start of the buffer. |
| `M->` | `end-of-buffer` | Move point to the end of the buffer. |
| `C-v` `<pgdn>` | `scroll-up-command` | Move forward one screenful. |
| `M-v` `<pgup>` | `scroll-down-command` | Move backward one screenful. |
| `M-g M-g` | `goto-line` | Move point to the start of a numbered line. |
| `M-g i` | `imenu` | Go to a definition in this buffer - a function, a type, a heading - by name. |
| `C-l` | `recenter-top-bottom` | Scroll point to the centre, then the top, then the bottom. |

### Editing

| Key | Command | Does |
|---|---|---|
| `C-d` `<delete>` | `delete-char` | Delete the character after point. |
| `<backspace>` | `delete-backward-char` | Delete the character before point. |
| `M-d` | `kill-word` | Kill forward to the end of the next word. |
| `M-<backspace>` | `backward-kill-word` | Kill backward to the start of the previous word. |
| `C-k` | `kill-line` | Kill to the end of the line, or the newline if already there. |
| `C-o` | `open-line` | Insert a newline after point, leaving point in place. |
| `C-t` | `transpose-chars` | Transpose the characters around point. |
| `M-t` | `transpose-words` | Transpose the words around point. |
| `RET` | `newline` | Insert a newline, copying the current line's indentation. |
| `TAB` | `indent-for-tab-command` | Indent at point, or the region's lines, with the file's tabs or spaces. |
| `M-u` | `upcase-word` | Convert the following word to upper case. |
| `M-l` | `downcase-word` | Convert the following word to lower case. |
| `M-c` | `capitalize-word` | Capitalize the following word. |
| `M-;` | `comment-dwim` | Comment or uncomment the lines of the region, or the current line. |
| `M-/` | `dabbrev-expand` | Complete the word before point from words already in the text; repeat for the next. |
| `M-SPC` | `just-one-space` | Leave exactly one space around point. |
| `M-\` | `delete-horizontal-space` | Delete all spaces and tabs around point. |
| `M-^` | `delete-indentation` | Join this line to the previous one, with one space between. |
| `M-m` | `back-to-indentation` | Move to the first non-blank character on the line. |
| `M-z` | `zap-to-char` | Kill up to and including the next occurrence of a character, ARG times. |
| `M-{` | `backward-paragraph` | Move to the start of the paragraph, ARG paragraphs back. |
| `M-}` | `forward-paragraph` | Move to the end of the paragraph, ARG paragraphs forward. |
| `M-q` | `fill-paragraph` | Re-wrap the paragraph at point to the fill column. |
| `C-M-f` | `forward-sexp` | Move over the next balanced expression, ARG times. |
| `C-M-b` | `backward-sexp` | Move back over the previous balanced expression, ARG times. |
| `C-M-k` | `kill-sexp` | Kill the balanced expression after point. |
| `M-=` | `count-words` | Count the lines, words and characters in the region, or the buffer. |
| `C-x =` | `what-cursor-position` | Describe the character at point and where point is. |

### Indentation and regions

| Key | Command | Does |
|---|---|---|
| `<backtab>` | `indent-rigidly-left-to-tab-stop` | Shift the region's lines, or the current line, one level left. |
| `C-x TAB` | `indent-rigidly` | Shift the region's lines right by ARG columns, or one level; left with a negative ARG. |
| `C-x C-u` | `upcase-region` | Convert the region to upper case. |
| `C-x C-l` | `downcase-region` | Convert the region to lower case. |
| `C-M-%` | `query-replace-regexp` | Replace matches of a regexp, asking about each one. |

### Keyboard macros

| Key | Command | Does |
|---|---|---|
| `<f3>` | `kmacro-start-macro-or-insert-counter` | Start recording a keyboard macro, or while recording, insert the macro counter. |
| `<f4>` | `kmacro-end-or-call-macro` | Stop recording a keyboard macro, or play the last one, ARG times (0: until it fails). |
| `C-x (` | `kmacro-start-macro` | Start recording a keyboard macro. |
| `C-x )` | `kmacro-end-macro` | Stop recording a keyboard macro. |
| `C-x e` | `kmacro-end-and-call-macro` | Play the last keyboard macro, ARG times; then e plays it again. |

### Line movement

| Key | Command | Does |
|---|---|---|
| `M-<up>` | `move-lines-up` | Move the region's lines, or the current line, one line up. |
| `M-<down>` | `move-lines-down` | Move the region's lines, or the current line, one line down. |

### Mark, region, kill ring

| Key | Command | Does |
|---|---|---|
| `C-SPC` | `set-mark-command` | Set the mark where point is. |
| `C-x C-x` | `exchange-point-and-mark` | Put point where the mark is, and the mark where point was. |
| `C-x h` | `mark-whole-buffer` | Put point at the start of the buffer and the mark at the end. |
| `C-w` | `kill-region` | Kill the text between point and mark, saving it on the kill ring. |
| `M-w` | `kill-ring-save` | Save the text between point and mark on the kill ring without removing it. |
| `C-y` | `yank` | Insert the current kill-ring entry at point. |
| `M-y` | `yank-pop` | Replace the text just yanked with the next-older kill-ring entry. |

### Undo

| Key | Command | Does |
|---|---|---|
| `C-_` `C-/` `C-x u` | `undo` | Undo the most recent change. |
| `M-_` | `redo` | Redo the change most recently undone. |

### Search and replace

| Key | Command | Does |
|---|---|---|
| `C-s` | `isearch-forward` | Search incrementally forward as you type. |
| `C-r` | `isearch-backward` | Search incrementally backward as you type. |
| `M-%` | `query-replace` | Replace occurrences of a string, asking about each one. |
| `M-s o` | `occur` | List the lines of this buffer that match a regexp, each leading to its line. |

### Files

| Key | Command | Does |
|---|---|---|
| `C-x C-f` | `find-file` | Visit a file in the current window, creating it if absent. |
| `C-x C-r` | `recentf-open` | Open one of the files opened recently. |
| `C-x C-s` | `save-buffer` | Save the current buffer, prompting for a name if it has none. |
| `C-x C-w` | `write-file` | Save the current buffer under a new name. |
| `C-x s` | `save-some-buffers` | Offer to save each modified buffer. |
| `C-x x g` | `revert-buffer` | Read the buffer's file again, throwing away any edits after asking; a listing is read again, a compilation run again. |

### Buffers

| Key | Command | Does |
|---|---|---|
| `C-x b` | `switch-to-buffer` | Display another buffer, creating it if the name is new. |
| `C-x k` | `kill-buffer` | Kill the current buffer, confirming if it is modified. |
| `C-x C-b` | `list-buffers` | Display a listing of every live buffer. |
| `C-x d` | `dired` | List a directory, to browse it and act on its files. |
| `C-x C-j` | `dired-jump` | List the directory of the current file, with point on the file. |

### Projects

| Key | Command | Does |
|---|---|---|
| `C-x p f` | `project-find-file` | Open a file in the current project, found by name. |
| `C-x p p` | `project-switch-project` | Open a file in another project nem knows. |
| `C-x p b` | `project-switch-to-buffer` | Switch to a buffer of the current project. |
| `C-x p d` | `project-find-dir` | List a directory of the current project, found by name. |
| `C-x p D` | `project-dired` | List the current project's top directory. |
| `C-x p g` | `project-find-regexp` | Search the current project's files for a regexp, listing the matching lines. |
| `C-x p r` | `project-query-replace` | Replace text across the current project's files, asking at each match. |
| `C-x p e` | `project-recentf` | Open one of the current project's recently opened files. |
| `C-x p t` | `project-toggle-test` | Go from a file to its test, or from a test to the file it tests. |
| `C-x p k` | `project-kill-buffers` | Kill every buffer of the current project, after asking. |
| `C-x p S` | `project-save-buffers` | Save every modified file of the current project. |
| `C-x p c` | `project-compile` | Run a compilation from the top of the current project. |

### Shell commands and compiling

| Key | Command | Does |
|---|---|---|
| `M-!` | `shell-command` | Run a shell command and show its output; with C-u, insert it at point. |
| `M-\|` | `shell-command-on-region` | Run a shell command with the region as its input and show its output; with C-u, replace the region with it. |
| `M-&` | `async-shell-command` | Run a shell command, showing its output as it comes, in *Async Shell Command*. |
| `M-g n` `M-g M-n` `` C-x ` `` | `next-error` | Go to the next match of the last search, or error of the last compilation, ARG on. |
| `M-g p` `M-g M-p` | `previous-error` | Go to the previous match of the last search, or error of the last compilation, ARG back. |

### Windows

| Key | Command | Does |
|---|---|---|
| `C-x 2` | `split-window-below` | Split the current window, stacking the new one beneath it. |
| `C-x 3` | `split-window-right` | Split the current window, placing the new one beside it. |
| `C-x 1` | `delete-other-windows` | Make the current window fill the frame. |
| `C-x 0` | `delete-window` | Remove the current window. |
| `C-x o` | `other-window` | Select another window, ARG windows forward. |

### Display

| Key | Command | Does |
|---|---|---|
| `C-x n` | `toggle-line-numbers` | Show or hide the line-number gutter. |

### Session

| Key | Command | Does |
|---|---|---|
| `C-g` | `keyboard-quit` | Abandon the current operation. |
| `C-x C-c` | `save-buffers-kill-terminal` | Offer to save modified buffers, then exit. |
| `M-x` | `execute-extended-command` | Read a command name in the minibuffer and run it. |

### Help

| Key | Command | Does |
|---|---|---|
| `<f1> b` `C-h b` | `describe-bindings` | Show every key binding in a help buffer. |
| `<f1> k` `C-h k` | `describe-key` | Read a key sequence and report the command it runs. |

## Dired

In a directory listing: `C-x d`, `C-x C-j`, or any directory opened as a file. The listing is read-only, so plain letters are commands here. Bind a key here with `nem.bind(key, command, "dired")`.

| Key | Command | Does |
|---|---|---|
| `RET` `f` `e` `M-<down>` | `dired-find-file` | Visit the file at point, or list the directory at point. |
| `o` | `dired-find-file-other-window` | Visit the file at point in another window. |
| `^` `M-<up>` | `dired-up-directory` | List the parent directory, with point on this one. |
| `M-p` | `dired-history-back` | List the directory listed before this one, as a browser goes back. |
| `M-n` | `dired-history-forward` | List the directory gone back from, as a browser goes forward. |
| `n` `SPC` `C-n` `<down>` | `dired-next-line` | Move to the next file, ARG files down. |
| `p` `C-p` `<up>` | `dired-previous-line` | Move to the previous file, ARG files up. |
| `M-<` | `dired-first-file` | Move to the first file in the listing, below the parent entry. |
| `M->` | `dired-last-file` | Move to the last file in the listing. |
| `m` | `dired-mark` | Mark the file at point for the next operation, and move down. |
| `u` | `dired-unmark` | Remove the mark on the file at point, and move down. |
| `DEL` | `dired-unmark-backward` | Move up a file and remove its mark. |
| `U` | `dired-unmark-all` | Remove every mark and deletion flag. |
| `t` | `dired-toggle-marks` | Mark every unmarked file and unmark every marked one. |
| `d` | `dired-flag-file-deletion` | Flag the file at point for deletion by x, and move down. |
| `x` | `dired-do-flagged-delete` | Delete the files flagged with d, after asking. |
| `D` | `dired-do-delete` | Delete the marked files, or the file at point, after asking. |
| `R` | `dired-do-rename` | Rename the file at point, or move the marked files into a directory. |
| `C` | `dired-do-copy` | Copy the file at point, or the marked files into a directory. |
| `+` | `dired-create-directory` | Create a directory, and any missing parents. |
| `g` | `dired-revert` | Read the directory again. |
| `(` | `dired-toggle-details` | Show or hide permissions, sizes and dates. |
| `.` | `dired-toggle-hidden` | Show or hide dotfiles. |
| `s` | `dired-sort-toggle` | Sort by name, then by time, then by size. |
| `w` | `dired-copy-filename` | Copy the names of the marked files, or of the file at point. |
| `E` | `dired-do-open` | Open the marked files, or the file at point, with the system app. |
| `C-x C-q` | `wdired-change-to-wdired-mode` | Edit the file names in this listing as text, to rename the files. |
| `q` | `quit-window` | Show another buffer here, putting this one at the back of the list. |

## Editing file names

After `C-x C-q` in a listing, the file names are text to edit; these keys finish or abandon the edit. Bind a key here with `nem.bind(key, command, "wdired")`.

| Key | Command | Does |
|---|---|---|
| `C-c C-c` `C-x C-s` | `wdired-finish-edit` | Rename the files to the names as edited, and return to the listing. |
| `C-c C-k` | `wdired-abort-changes` | Throw the edited names away, and return to the listing. |
| `C-x C-q` | `wdired-exit` | Return to the listing, asking whether to apply any edited names. |
| `C-a` `<home>` | `wdired-beginning-of-line` | Move to the start of the file name, or of the line off an entry. |
| `C-e` `<end>` | `wdired-end-of-line` | Move to the end of the file name, or of the line off an entry. |

## Search results

In the results of a project search (`C-x p g`) or of `M-s o` (occur). Bind a key here with `nem.bind(key, command, "grep")`.

| Key | Command | Does |
|---|---|---|
| `RET` | `grep-goto-match` | Go to the match at point, in the other window. |
| `o` | `grep-display-match` | Show the match at point in the other window, staying here. |
| `n` | `grep-next-match` | Move to the next match, ARG matches on, showing it in the other window. |
| `p` | `grep-previous-match` | Move to the previous match, ARG matches back, showing it in the other window. |
| `C-n` `<down>` | `grep-next-line` | Move to the next match, ARG matches on. |
| `C-p` `<up>` | `grep-previous-line` | Move to the previous match, ARG matches back. |
| `}` | `grep-next-file` | Move to the next file's matches, ARG files on. |
| `{` | `grep-previous-file` | Move to the previous file's matches, ARG files back. |
| `g` | `grep-revert` | Search again, for the same thing in the same project or buffer. |
| `q` | `quit-window` | Show another buffer here, putting this one at the back of the list. |

## Compilation output

In the output of `M-x compile`, `M-x recompile`, `C-x p c` and `M-&`. Bind a key here with `nem.bind(key, command, "compilation")`.

| Key | Command | Does |
|---|---|---|
| `RET` | `compile-goto-error` | Go to the place the line at point names, in the other window. |
| `o` `C-o` | `compilation-display-error` | Show the place the line at point names in the other window, staying here. |
| `n` | `compilation-next-error` | Move to the next line naming a place, ARG on, showing it in the other window. |
| `p` | `compilation-previous-error` | Move to the previous line naming a place, ARG back, showing it in the other window. |
| `g` | `recompile` | Run the last compilation again, or the one this buffer shows. |
| `C-c C-k` | `kill-compilation` | Stop the running compilation. |
| `q` | `quit-window` | Show another buffer here, putting this one at the back of the list. |

## In a prompt

A prompt - `M-x`, `C-x C-f`, `C-x b`, a search - edits its one line as a buffer is edited: `C-a`, `C-k`, `M-b`, `C-y` and the rest all work in it. On top of those it has keys of its own:

| Key | Does |
|---|---|
| `RET` | Take the highlighted candidate, or what was typed where there are none. |
| `M-RET` | Take exactly what was typed, not the highlighted candidate: a new file whose name matches another. |
| `C-g` | Cancel the prompt. |
| `TAB` | Complete what was typed to the highlighted candidate. |
| `C-n` `<down>` | Highlight the next candidate; with none listed, the next entry of the history. |
| `C-p` `<up>` | Highlight the previous candidate; with none listed, the previous entry of the history. |
| `M-p` | Bring back the previous thing entered at this kind of prompt. |
| `M-n` | Go forward again through that history. |
| `C-s` | In a search, go to the next match. |
| `C-r` | In a search, go to the previous match. |

While candidates are listed, the keys that page and jump through a buffer move through the list instead:

| Key | Does |
|---|---|
| `C-v` `<pgdn>` | Next page of candidates. |
| `M-v` `<pgup>` | Previous page of candidates. |
| `M-<` | First candidate. |
| `M->` | Last candidate. |

An incremental search (`C-s`, `C-r`) moves to the first match as you type. `C-s` and `C-r` go on to the next and previous match, wrapping round the buffer after saying they have reached its end; `RET` stops at the match, and `C-g` goes back to where the search began.

## Answering a question

Some commands stop to ask. A question takes one key, with no `RET`; `C-g` cancels it.

| Asked by | Keys |
|---|---|
| `M-%` and `C-M-%`, at each match | `y` replace it · `n` skip it · `!` replace it and all the rest · `q` stop |
| `C-x p r`, at each match | as above, and `N` skip the rest of this file · `Y` replace all the rest, in every file; `!` is the rest of this file |
| `C-x s`, for each unsaved file | `y` save it · `n` don't · `!` save it and all the rest · `q` stop |
| `C-x C-c`, for each unsaved file | `y` save it · `n` don't · `q` stay in nem |
| Opening a file that isn't text | `s` open it with the system's app · `t` as text · `S` and `T` the same for every such file, for the rest of the session |
| Anything else asked yes or no | `y` · `n` |

## Arguments and repeats

| Key | Does |
|---|---|
| `C-u` | Give the next command an argument: 4, and 16 with `C-u C-u`. Digits after it give a number: `C-u 12 C-f` moves 12 characters. |
| `M-0` … `M-9`, `M--` | Start an argument without `C-u`: `M-1 M-2` is 12, `M--` is -1. |
| `C-g` | Cancel a half-typed prefix or argument, a prompt, or a question. |
| `e` | Straight after `C-x e`, play the keyboard macro again. |

Pausing after a prefix key such as `C-x` lists the keys that can follow it, after a second by default: the `which-key-delay` [setting](Settings#which-key-delay).
