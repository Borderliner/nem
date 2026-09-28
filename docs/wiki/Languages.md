# Languages

Every language nem colours is described the same way: in a `.syntax` file.
nem's own are built in, and yours sit beside `init.lua`. Adding a language
means writing a short file of its keywords and how it writes comments and
strings. That's also where `M-;` learns how to write a comment.

**On this page:** [Adding a language](#adding-a-language) ·
[The format](#the-format) · [How a line is read](#how-a-line-is-read) ·
[Regions](#regions) · [Patterns](#patterns) · [Starting from another language](#starting-from-another-language) ·
[Built in](#built-in)

## Adding a language

The quickest start is `M-x edit-language`. It asks which language, defaulting
to the one on screen, and opens that language's file in your `syntax`
directory. For one of nem's languages the file starts as a copy of nem's
definition, so you can change anything. For a language nem doesn't know, it
starts as an outline to fill in.

The files live in the `syntax` directory beside `init.lua`, one per language,
named after it:

| System | Directory |
|---|---|
| Linux, BSD | `~/.config/nem/syntax/`, or `$XDG_CONFIG_HOME/nem/syntax/` |
| macOS | `~/Library/Application Support/nem/syntax/` |
| Windows | `%AppData%\nem\syntax\` |

Here is Ada, as `ada.syntax`:

```
# Ada.
language ada
files *.adb *.ads
ignore-case

comment --
string " doubled
match string '.'

keywords abort abs accept access all and array at begin body case constant
keywords declare delay do else elsif end entry exception exit for function
keywords generic goto if in is limited loop mod new not null of or others out
keywords package pragma private procedure raise range record rem renames
keywords return reverse select separate subtype task terminate then type use
keywords when while with xor
types Boolean Character Float Integer Natural Positive String
constants True False
declares function function procedure
declares type type subtype package
```

Open an Ada file in one window and the definition in the other, with `C-x 2`
and `C-x o`. Each time you save the definition with `C-x C-s`, nem reads it
again and recolours the Ada file, so you see what each line does as you write
it. If a line is wrong, the echo area says which one and why:
`syntax: ada.syntax:12: unknown directive "keyword"`. Until you put it right,
the colours stay as they were.

A file of yours with the same name as one of nem's languages replaces it. To
add to one of nem's languages rather than replace it, see
[Starting from another language](#starting-from-another-language).

## The format

One directive a line. A line starting with `#` is a comment. Since `#` is a
common delimiter, a `#` anywhere else is part of the line.

| Directive | Means |
|---|---|
| `language NAME` | The language's name, which the mode line shows. It comes first, in lower case. |
| `like NAME` | Start from another language: see [below](#starting-from-another-language). It comes straight after `language`. |
| `files GLOB...` | The file names the language claims: `*.adb`, `Makefile`, `Dockerfile.*`. Case doesn't matter. |
| `shebang WORD...` | Programs in a script's `#!` first line. `shebang python` claims `#!/usr/bin/env python3` too, since a trailing version is ignored. |
| `header REGEX` | A first line to claim, for files that start with something other than `#!`: `<\?xml`. |
| `ignore-case` | Keywords, types, constants and functions match in any case, as in Ada, SQL and Pascal. |
| `word-chars CHARS` | Characters a name may hold besides letters, digits and `_`. Lisp's names hold `-*+!?<>=/`, so `empty?` is one name. |
| `keywords WORD...` | Words coloured as keywords. `types`, `constants` and `functions` work the same way. Each may be repeated, and the lists add up. |
| `declares CLASS WORD...` | The name after one of the words is coloured as CLASS: `declares function def`, `declares type class struct`. |
| `calls paren\|lisp\|none` | Which names are calls, coloured as functions. `paren` (the default) means a name followed by `(`, as in `print(x)`. `lisp` means a name right after `(`, as in `(print x)`. `none` means neither. |
| `numbers REGEX\|none` | What a number looks like, if not the usual. The default covers `42`, `1_000`, `0x1F`, `0b101`, `0o17`, `3.14`, `1e9`, `.5` and suffixes such as `10UL` and `1.5f`. |
| `operators CHARS\|none` | Characters coloured as operators. The default is `+-*/%&\|^<>=!~?:`. |
| `punctuation CHARS\|none` | Characters coloured as punctuation. The default is `()[]{},;.`. |
| `comment START [END] [nested]` | A comment to the end of the line, or with END a block comment, which may span lines. `nested` means a block closes only after every one opened inside it has. |
| `string START [END] [raw\|doubled\|escape C] [multiline]` | A string. `\` escapes the next character unless it is `raw` (nothing escapes), `doubled` (`""` inside `"…"`) or `escape C` (C escapes). A string ends with its line unless it is `multiline`. |
| `region CLASS` | Anything from a start to an end: see [Regions](#regions). |
| `match CLASS REGEX` | Colour what REGEX matches: see [Patterns](#patterns). |

The classes are what a palette colours:

| Class | For | Dark palette |
|---|---|---|
| `keyword` | control flow, declarations, modifiers | purple |
| `type` | built-in types, and type names where they're unmistakable | yellow |
| `constant` | `true`, `nil`, symbols, atoms, variables marked by a sigil | orange |
| `number` | numbers | orange |
| `function` | built-in functions, calls, names being declared, macros | blue |
| `string` | strings, characters, here-documents, regular expressions | green |
| `comment` | comments | grey |
| `operator`, `punctuation` | left uncoloured by nem's palettes, so code doesn't read as noise | none |
| `plain` | nothing; useful to keep a pattern's text out of other rules | none |

`M-;` writes the first comment a definition lists, so list the one you'd type
first. Lisp lists `comment ;;` before `comment ;`.

## How a line is read

nem reads a line left to right, one piece at a time. At each point the first
of these that applies wins:

1. a `match`, in the order they're written;
2. the start of a `comment`, `string` or `region` (the longest, if several
   start at the same point);
3. a number;
4. a name, which is a keyword, type, constant or function if it's listed; the
   name a `declares` word announced; a call; or else plain;
5. an operator or punctuation character.

Everything else is plain. Because the line is read in order, a `//` inside a
string is part of the string, and a quote inside a comment is part of the
comment. The order you write the rules in doesn't matter for that.

A block comment carries on to the next line until it ends. So does a string or
region marked `multiline`; anything else ends with its line, so a string you're
still typing never recolours the rest of the file.

## Regions

A region is a `comment` or `string` whose delimiters need a pattern. Its fields
go on the indented lines below it:

```
# Lua's long strings: [[ ... ]], or [==[ ... ]==] with any number of =.
region string
    start \[(=*)\[
    end \]\1\]
    multiline
```

| Field | Means |
|---|---|
| `start REGEX` | Where the region begins. Required. |
| `end REGEX` | Where it ends. Required. `\1` to `\9` stand for what `start`'s groups matched, so a region opened with `[==[` ends only at `]==]`. |
| `escape REGEX` | Text inside that can't end the region, such as `\\.` for a backslash and the character after it. |
| `nested` | A `start` inside the region opens another level, and the region ends only when every level has ended. |
| `multiline` | The region may continue onto the next lines. |

A here-document is the same idea, ended by a line holding only the word it
started with:

```
region string
    start <<-?\s*['"]?([A-Za-z_]\w*)['"]?
    end ^\s*\1$
    multiline
```

## Patterns

Patterns are [Go regular expressions](https://pkg.go.dev/regexp/syntax). A
`^` matches at the start of the line, and nowhere else.

`match CLASS REGEX` colours what the pattern matches:

```
match function @[A-Za-z_][\w.]*        # Python's decorators
match keyword ^\s*#\s*[a-z]+           # C's #include and #define
```

`match REGEX` without a class colours each **named group** by its name, and
leaves the text after the last named group to be read again. Go's regular
expressions can't look ahead, and this does the same job:

```
# A string followed by a colon is a key, and the colon is still punctuation.
match (?P<function>"(?:[^"\\]|\\.)*")\s*:

# A Markdown link: the text in one colour, the address in another.
match (?P<function>\[[^\]]*\])(?P<string>\([^)]*\))
```

Pattern checks are fast: nem only tries a pattern at points where its first
character could match.

## Starting from another language

`like NAME` starts a definition from another language's, and what follows adds
to it. Your own lists and rules come first, so they win. This is how nem's C++
builds on C:

```
language cpp
like c
files *.cpp *.hpp *.cc
keywords class namespace template typename public private virtual
```

A language that's `like` another doesn't take the other's `files`, so
`language gotmpl`, `like go` and `files *.gotmpl` add a language for Go
templates and leave `.go` files as they were.

To add to one of nem's own languages, name your language the same and start
from it. This `go.syntax` colours `must` as a keyword in Go, and keeps
everything else nem's Go does:

```
language go
like go
keywords must
```

## What a definition can't do

A definition describes how a language looks, not its grammar, so it's
approximate at the edges. A few things it can't say yet:

- **Code inside a string.** In an interpolation such as `"${ "x" }"`, the
  inner quote ends the string, and the code inside isn't coloured as code.
- **Another language inside a file.** A `<script>` in HTML, a fenced block in
  Markdown and HTML around PHP are coloured as the file's own language.
- **Looking back.** A pattern can't look at what came before its match, so
  `/` as division and `/` opening a regular expression are told apart only
  where the characters around them decide it.
- **Brackets that nest inside a region.** Ruby's `%w[a [b] c]` ends at the
  first `]`. A `nested` region counts only its own start.

## Built in

nem's own definitions are in the
[repository](https://github.com/Borderliner/nem/tree/main/syntax/languages).
They make good starting points: copy one into your `syntax` directory and
change it. These are the languages built in, and the files each claims:

<!-- languages: generated from the definitions by TestWikiReferenceIsCurrent -->

| Language | Files |
|---|---|
| ada | `*.adb` `*.ads` `*.ada` `*.gpr` |
| asm | `*.asm` `*.s` `*.nasm` |
| awk | `*.awk` `#!awk` `#!gawk` `#!mawk` `#!nawk` |
| batch | `*.bat` `*.cmd` |
| c | `*.c` `*.h` |
| clojure | `*.clj` `*.cljs` `*.cljc` `*.edn` `*.bb` `#!bb` |
| cmake | `cmakelists.txt` `*.cmake` |
| coffeescript | `*.coffee` `*.cson` `#!coffee` |
| cpp | `*.cc` `*.cpp` `*.cxx` `*.c++` `*.hh` `*.hpp` `*.hxx` `*.h++` `*.ipp` `*.tpp` `*.inl` |
| crystal | `*.cr` `#!crystal` |
| csharp | `*.cs` `*.csx` |
| css | `*.css` |
| d | `*.d` `*.di` `#!rdmd` |
| dart | `*.dart` |
| diff | `*.diff` `*.patch` `*.rej` |
| dockerfile | `dockerfile` `dockerfile.*` `*.dockerfile` `containerfile` `containerfile.*` |
| dot | `*.dot` `*.gv` |
| dotenv | `.env` `.env.*` `*.env` |
| elisp | `*.el` `.emacs` |
| elixir | `*.ex` `*.exs` `#!elixir` |
| elm | `*.elm` |
| erlang | `*.erl` `*.hrl` `*.escript` `#!escript` |
| fish | `*.fish` `#!fish` |
| fortran | `*.f` `*.for` `*.ftn` `*.f77` `*.f90` `*.f95` `*.f03` `*.f08` `*.fpp` |
| fsharp | `*.fs` `*.fsi` `*.fsx` |
| gitcommit | `commit_editmsg` `merge_msg` `tag_editmsg` `squash_msg` |
| gitignore | `.gitignore` `.dockerignore` `.npmignore` `.hgignore` `.prettierignore` `.eslintignore` `.gcloudignore` `.containerignore` |
| gleam | `*.gleam` |
| glsl | `*.glsl` `*.vert` `*.frag` `*.geom` `*.tesc` `*.tese` `*.comp` |
| go | `*.go` |
| gomod | `go.mod` `go.work` |
| graphql | `*.graphql` `*.gql` `*.graphqls` |
| groff | `*.[1-9]` `*.man` `*.ms` `*.me` `*.mom` `*.roff` `*.tmac` |
| groovy | `*.groovy` `*.gradle` `*.gvy` `jenkinsfile` `#!groovy` |
| haskell | `*.hs` `#!runhaskell` `#!runghc` `#!stack` |
| hcl | `*.tf` `*.tfvars` `*.hcl` `*.nomad` |
| html | `*.html` `*.htm` `*.xhtml` `*.shtml` |
| ini | `*.ini` `*.cfg` `*.conf` `*.desktop` `*.service` `*.socket` `*.timer` `*.target` `*.mount` `*.path` `*.slice` `*.network` `*.netdev` `*.link` `.gitconfig` `.gitmodules` `.editorconfig` `.npmrc` `.curlrc` `.wgetrc` |
| java | `*.java` |
| javascript | `*.js` `*.mjs` `*.cjs` `*.jsx` `#!node` `#!deno` `#!bun` |
| json | `*.json` `*.jsonc` `*.jsonl` `*.json5` `*.geojson` `*.webmanifest` `.babelrc` `.eslintrc` |
| julia | `*.jl` `#!julia` |
| just | `justfile` `.justfile` `*.just` |
| kotlin | `*.kt` `*.kts` |
| latex | `*.tex` `*.sty` `*.cls` `*.ltx` `*.dtx` |
| lisp | `*.lisp` `*.lsp` `*.cl` `*.asd` `#!sbcl` `#!clisp` |
| lua | `*.lua` `*.rockspec` `.luacheckrc` `#!lua` `#!luajit` |
| m4 | `*.m4` `*.ac` `configure.in` |
| makefile | `makefile` `gnumakefile` `*.mk` `*.make` |
| markdown | `*.md` `*.markdown` `*.mkd` `*.mdown` |
| meson | `meson.build` `meson_options.txt` `meson.options` |
| nftables | `*.nft` `#!nft` |
| nim | `*.nim` `*.nims` `*.nimble` `#!nim` |
| ninja | `*.ninja` |
| nix | `*.nix` |
| objc | `*.m` `*.mm` |
| ocaml | `*.ml` `*.mli` `*.mll` `*.mly` `#!ocaml` |
| odin | `*.odin` |
| pascal | `*.pas` `*.pp` `*.dpr` `*.lpr` `*.dpk` |
| perl | `*.pl` `*.pm` `*.t` `*.psgi` `#!perl` |
| php | `*.php` `*.phtml` `*.phpt` `#!php` |
| po | `*.po` `*.pot` |
| povray | `*.pov` |
| powershell | `*.ps1` `*.psm1` `*.psd1` `#!pwsh` |
| properties | `*.properties` |
| protobuf | `*.proto` |
| python | `*.py` `*.pyi` `*.pyw` `sconstruct` `sconscript` `#!python` `#!pypy` |
| r | `*.r` `.rprofile` `#!Rscript` `#!rscript` |
| ruby | `*.rb` `*.rake` `*.gemspec` `*.ru` `rakefile` `gemfile` `guardfile` `podfile` `vagrantfile` `brewfile` `#!ruby` |
| rust | `*.rs` |
| scala | `*.scala` `*.sc` `#!scala` |
| scheme | `*.scm` `*.ss` `*.sld` `*.sls` `*.rkt` `*.rktl` `#!guile` `#!racket` `#!csi` `#!gsi` `#!chez` |
| scss | `*.scss` `*.sass` `*.less` |
| sh | `*.sh` `*.bash` `*.zsh` `*.ksh` `.bashrc` `.bash_profile` `.bash_logout` `.zshrc` `.zprofile` `.zshenv` `.zlogin` `.profile` `.kshrc` `pkgbuild` `apkbuild` `*.ebuild` `#!sh` `#!bash` `#!zsh` `#!ksh` `#!dash` `#!ash` `#!mksh` |
| solidity | `*.sol` |
| spec | `*.spec` |
| sql | `*.sql` `*.ddl` `*.psql` |
| starlark | `*.bzl` `*.star` `*.bazel` `build.bazel` `workspace.bazel` `module.bazel` |
| svelte | `*.svelte` |
| swift | `*.swift` |
| syntax | `*.syntax` |
| tcl | `*.tcl` `*.tk` `#!tclsh` `#!wish` `#!expect` |
| texinfo | `*.texi` `*.texinfo` `*.txi` |
| toml | `*.toml` `cargo.lock` `poetry.lock` `uv.lock` |
| typescript | `*.ts` `*.tsx` `*.mts` `*.cts` |
| vb | `*.vb` `*.vbs` |
| verilog | `*.v` `*.vh` `*.sv` `*.svh` |
| vhdl | `*.vhd` `*.vhdl` |
| vim | `*.vim` `.vimrc` `.gvimrc` `_vimrc` `.exrc` |
| vue | `*.vue` |
| xml | `*.xml` `*.xsd` `*.xsl` `*.xslt` `*.svg` `*.rss` `*.atom` `*.plist` `*.pom` `*.csproj` `*.fsproj` `*.vbproj` `*.props` `*.targets` `*.xaml` `*.kml` `*.gpx` `*.wsdl` `*.xul` |
| yaml | `*.yaml` `*.yml` `.clang-format` `.clang-tidy` |
| zig | `*.zig` `*.zon` |

<!-- /languages -->
