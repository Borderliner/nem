# One way to describe a language

Approved 2026-09-28. Every language nem colours is described the same way, in
a `.syntax` file, and anyone can add one.

## Why

Highlighting had grown three mechanisms and a fourth table:

| Mechanism | Covered | Could a user extend it? |
|---|---|---|
| Hand-written Go lexers | Go, Lua, JSON, Markdown | No |
| `.nemrc` regex rules, embedded | 15 languages | No: embedded only |
| nano's `.nanorc`, read at runtime | ~40, only where nano is installed | Only in nano's format, with classes guessed from the patterns |
| `comment-dwim`'s extension table | the comment marker for `M-;` | No |

nano's files are GPL, so they could only ever be read, never shipped. A
machine without nano lost most languages. Their colours carry no meaning, so
every rule's class was a guess. And a language someone wanted, such as Ada, Nim
or Lisp, had no good home.

## The format

A `.syntax` file is one language, one directive a line. A `#` starts a
comment only at the start of a line, since `#` is a common delimiter.

```
# Ada.
language ada
files *.adb *.ads *.ada
ignore-case

comment --
string " doubled
match string '.'

keywords abort abs abstract accept access aliased all and array at begin body
keywords case constant declare delay delta digits do else elsif end entry
types Boolean Character Duration Float Integer Natural Positive String
constants True False
declares function function procedure entry
declares type type subtype package
```

### Directives

| Directive | Means |
|---|---|
| `language NAME` | First, required. Lower case: the name the mode line shows. |
| `like NAME` | Start from language NAME: everything but its `files`, `shebang` and `header`, unless NAME is this language's own name, when those are kept too. What follows adds to it. |
| `files GLOB...` | File names this language claims, matched against the base name without regard to case: `*.go`, `Makefile`, `Dockerfile.*`. |
| `shebang WORD...` | Interpreters in a `#!` first line: `#!/usr/bin/env python3` is `python`, since a trailing version is ignored. |
| `header REGEX` | A first line to claim, for anything a `#!` does not name: `<\?xml`. |
| `ignore-case` | Keywords, types, constants and functions match in any case. |
| `word-chars CHARS` | Runes a name may hold besides letters, digits and `_`: Lisp's `-*+!?<>=/:%&.`. |
| `keywords WORD...` | Also `types`, `constants`, `functions`. Repeatable; they add up. |
| `declares CLASS WORD...` | The name after any of WORDs is a CLASS: `declares function def`, `declares type class struct`. |
| `calls paren\|lisp\|none` | A name followed by `(` is a function (`paren`, the default), or a name just after `(` is (`lisp`), or neither. |
| `numbers REGEX\|none` | What a number looks like, in place of the default: `0x`, `0b`, `0o`, digits with `_`, a fraction, an exponent and a suffix. |
| `operators CHARS\|none` | Runes coloured as operators. Default `+-*/%&\|^<>=!~?:`. |
| `punctuation CHARS\|none` | Runes coloured as punctuation. Default `()[]{},;.`. |
| `comment START [END] [nested]` | A line comment, or with END a block comment, which may span lines. `nested` counts inner opens. |
| `string START [END] [raw\|doubled\|escape C] [multiline]` | A string. The escape is `\` unless it is `raw` (none), `doubled` (`""` inside `"…"`) or another rune. |
| `region CLASS` | A general region, its fields indented on the lines below: `start REGEX`, `end REGEX`, `escape REGEX`, `nested`, `multiline`. `end` may say `\1`…`\9` for what `start`'s groups captured, quoted: Lua's `start \[(=*)\[` with `end \]\1\]`. |
| `match CLASS REGEX` | Colour what REGEX matches. |
| `match REGEX` | Colour each named group by its name, `(?P<function>\w+)\s*\(`. Text after the last named group is read again, which is how a pattern looks ahead. |

The classes are the ten a theme colours: `keyword`, `type`, `constant`,
`function`, `string`, `comment`, `number`, `operator`, `punctuation` and
`plain`.

Patterns are Go's regexps (RE2): no look-around and no back-references, which
is what `region`'s `\1` and `match`'s named groups stand in for. A `^` matches
only at the start of the line.

### How a line is read

Left to right, one token at a time. At each place the first of these that
applies wins:

1. a `match`, in the order they are written;
2. the start of a `comment`, `string` or `region`, the longest if several
   start here, the first written if they tie;
3. a number;
4. a name, classified: keyword, type, constant, function, then a pending
   `declares`, then a call;
5. an operator or punctuation rune.

Anything else is plain. Reading tokens, rather than letting later rules paint
over earlier ones as nano does, is what makes a `//` inside a string a string
and a `"` inside a comment a comment, with no ordering of rules to get right.

A region that is still open at the end of a line continues onto the next if it
is `multiline`, and a block comment always is. Otherwise it ends with the line,
so a string someone is still typing never recolours the file.

## Where definitions come from

nem's own are embedded from `syntax/languages/`, written from each language's
grammar and covered by nem's MIT licence. The user's are
`<config dir>/nem/syntax/*.syntax`, beside `init.lua`. A user definition
replaces a built-in one of the same name, and user definitions are matched
before built-in ones. A broken file is reported, as `init.lua` errors are, and
does not cost the other languages.

Saving a `.syntax` file in that directory from nem reloads the definitions and
recolours every buffer, so a definition is written and tried in one sitting.

A file is matched by name first, an exact name such as `Makefile` before any
glob, and then by its first line.

`M-;` and `M-q` take their comment markers from the same definition: the
first comment it lists, so Lisp lists `;;` before `;`.

## What goes

- The hand-written lexers for Go, Lua, JSON and Markdown. Their tests stay as
  the tests of the definitions that replace them, which is how the format was
  held to doing what they did: Go's type after `type`, Lua's long brackets of
  any level, Markdown's fences.
- The `.nemrc` format and its painter.
- Reading nano's files, and the `syntax/nanorc` package. The languages nano
  covered are written as definitions instead, with Ada, Nim, Lisp and more
  besides.
- `comment-dwim`'s own table.

## State

`State` stays a comparable `uint32`: the open region (8 bits), a nesting depth
(8 bits), and an id for what the region's start captured (16 bits). The ids
are interned per language, so a line that opens `[==[` leaves the same state
wherever it is, and the cache's convergence check still means what it did.
