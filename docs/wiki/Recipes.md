# Recipes

Pieces of `init.lua` to copy. Each is complete on its own; put the ones you
want in [your config](Configuration#where-initlua-lives) and restart nem.
The calls they use are on [Lua API](Lua-API), the settings on
[Settings](Settings).

## Looks

A light terminal, four-column tabs, no line numbers, long lines wrapped,
candidates in a panel in the middle of the screen:

```lua
nem.set("theme", "light")
nem.set("tab-width", 4)
nem.set("line-numbers", false)
nem.set("line-wrap", true)
nem.set("completion-style", "popup")
nem.set("completion-rows", 15)
```

Icons over SSH, when the terminal you connect from has a Nerd Font - nem
cannot see its fonts from the far end:

```lua
nem.set("icons", true)
```

## Keys from elsewhere

projectile's `C-c p` prefix, for the project commands nem keeps under `C-x p`:

```lua
nem.bind("C-c p f", "project-find-file")
nem.bind("C-c p p", "project-switch-project")
nem.bind("C-c p s", "project-find-regexp")
nem.bind("C-c p r", "project-query-replace")
nem.bind("C-c p c", "project-compile")
nem.bind("C-c p d", "project-find-dir")
nem.bind("C-c p b", "project-switch-to-buffer")
```

Keys most editors share, beside emacs's:

```lua
nem.bind("M-o", "other-window")
nem.bind("C-x C-b", "switch-to-buffer")   -- instead of the buffer list
nem.bind("<f5>", "recompile")
nem.bind("<f6>", "next-error")
```

Dired as some file managers have it - `-` goes up, `k` deletes, `r` renames:

```lua
nem.bind("-", "dired-up-directory", "dired")
nem.bind("k", "dired-do-delete", "dired")
nem.bind("r", "dired-do-rename", "dired")
```

emacs's own behaviour, where nem's defaults differ from it - a selection that
typing does not replace, and brackets that do not pair themselves:

```lua
nem.set("delete-selection", false)
nem.set("auto-pair", false)
```

An Arabic keyboard, whose ط د ذ ز ظ sit on other keys than a Persian one's:

```lua
nem.set("keyboard-layout", "arabic")
```

## Commands of your own

Copy the line below itself:

```lua
nem.command("duplicate-line", "Copy the current line below itself.",
  function()
    local line, col = nem.buf.point()
    nem.buf.insert_line(line + 1, nem.buf.line())
    nem.buf.set_point(line + 1, col)
  end)
nem.bind("C-c d", "duplicate-line")
```

Reverse the order of the buffer's lines (`M-x sort-lines` is built in):

```lua
nem.command("reverse-lines", "Put the buffer's lines in the opposite order.",
  function()
    local lines, n = {}, nem.buf.line_count()
    for i = 1, n do
      lines[n + 1 - i] = nem.buf.get_line(i)
    end
    nem.buf.set_text(table.concat(lines, "\n"))
  end)
```

Number the lines - one `C-/` takes the numbers away again:

```lua
nem.command("number-lines", "Put its number before every line.",
  function()
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, i .. "  " .. nem.buf.get_line(i))
    end
  end)
```

Title Case the current line:

```lua
nem.command("title-case-line", "Capitalize every word of the current line.",
  function()
    local s = nem.buf.line():gsub("(%a)([%w']*)", function(first, rest)
      return first:upper() .. rest:lower()
    end)
    nem.buf.replace_line(s)
  end)
```

Comment a line and go on to the next, for commenting out a block a line at a
time:

```lua
nem.command("comment-line-and-down", "Toggle the line's comment, then move down.",
  function()
    nem.run("comment-dwim")
    nem.run("next-line")
  end)
nem.bind("C-c ;", "comment-line-and-down")
```

## On save

Strip trailing whitespace from every line of a Go, Lua or Python file before it
is saved. The trimming is one change to undo:

```lua
nem.hook("before-save", function(buf)
  if buf.path:match("%.go$") or buf.path:match("%.lua$") or buf.path:match("%.py$") then
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
    end
  end
end)
```

A project's `.editorconfig` does the same without any Lua, for everyone who
works on it: `trim_trailing_whitespace = true`.

Leave no blank lines at the end of a file:

```lua
nem.hook("before-save", function(buf)
  local n = nem.buf.line_count()
  while n > 1 and nem.buf.get_line(n) == "" do
    nem.buf.remove_line(n)
    n = n - 1
  end
end)
```

A script cannot run a program, so formatting with `gofmt` or `black` on save is
not possible from a hook; `M-!` or `M-x compile` can run them.

## Only on some versions

A config that says so when the API it was written for has gone:

```lua
if nem.api_version ~= 1 then
  error("this init.lua is written for nem's API version 1")
end
```
