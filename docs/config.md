# Configuring nem

The reference for configuring nem is its wiki:
**<https://github.com/Borderliner/nem/wiki>**.

| Page | What it covers |
|---|---|
| [Configuration](https://github.com/Borderliner/nem/wiki/Configuration) | Where `init.lua` lives, how it is loaded, key notation, where nem keeps its state |
| [Settings](https://github.com/Borderliner/nem/wiki/Settings) | Every option `nem.set` takes |
| [Lua API](https://github.com/Borderliner/nem/wiki/Lua-API) | `nem.set`, `nem.bind`, `nem.command`, `nem.run`, `nem.hook`, `nem.buf` |
| [Keybindings](https://github.com/Borderliner/nem/wiki/Keybindings) | Every key, globally, in each mode, in prompts and in questions |
| [Commands](https://github.com/Borderliner/nem/wiki/Commands) | Every command by name, with its keys |
| [Recipes](https://github.com/Borderliner/nem/wiki/Recipes) | Configs to copy |

The pages are kept here, in [`docs/wiki`](wiki), and published from there, so
they change with the code. Keybindings and Commands are generated from the code
by `NEM_UPDATE_WIKI=1 go test ./editor -run TestWikiReferenceIsCurrent`; tests
fail when a page falls behind the code, and run every Lua example the pages
publish.
