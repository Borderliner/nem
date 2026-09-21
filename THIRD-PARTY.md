# Third-party notices

nem is MIT licensed. Its released binaries are statically linked and therefore
contain code from the libraries below. All of them are permissively licensed and
compatible with redistributing nem under the MIT License; none is copyleft.

| Library | Licence |
|---|---|
| github.com/gdamore/tcell/v2 | Apache License 2.0 |
| github.com/gdamore/encoding | Apache License 2.0 |
| github.com/charmbracelet/lipgloss | MIT |
| github.com/charmbracelet/colorprofile | MIT |
| github.com/charmbracelet/x/ansi | MIT |
| github.com/charmbracelet/x/cellbuf | MIT |
| github.com/charmbracelet/x/term | MIT |
| github.com/muesli/termenv | MIT |
| github.com/lucasb-eyer/go-colorful | MIT |
| github.com/mattn/go-isatty | MIT |
| github.com/mattn/go-runewidth | MIT |
| github.com/rivo/uniseg | MIT |
| github.com/xo/terminfo | MIT |
| github.com/yuin/gopher-lua | MIT |
| golang.org/x/sys | BSD 3-Clause |
| golang.org/x/term | BSD 3-Clause |
| golang.org/x/text | BSD 3-Clause |

The Apache 2.0 entries are listed here because that licence asks for its notice
to travel with a derivative work, and a statically linked binary is one.

## nano's syntax definitions

nem can read syntax highlighting rules from `/usr/share/nano` when GNU nano is
installed. Those files are GPL licensed and are **not** distributed with nem:
they are read from the user's own system at runtime, the way nano reads them.
Nothing from them is copied into this repository or into a released binary.

The rules nem bundles in `syntax/rules/` are original work, written from each
language's grammar, and are covered by nem's own MIT licence.
