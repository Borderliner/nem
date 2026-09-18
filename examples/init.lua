-- Example nem config. Copy to ~/.config/nem/init.lua and restart nem.
--
-- Everything here uses only the documented API: nem.set, nem.bind, nem.command,
-- nem.run, nem.hook, nem.api_version, and the nem.buf table. See docs/config.md.

nem.set("tab-width", 4)
nem.set("scroll-margin", 3)

-- A plain rebinding: F5 saves.
nem.bind("<f5>", "save-buffer")

-- A command of your own. It appears in M-x and <f1> k describes it, exactly
-- like a built-in -- Lua commands go into the same registry.
nem.command("reverse-line", "Reverse the characters on the current line.",
  function()
    nem.buf.replace_line(nem.buf.line():reverse())
  end)
nem.bind("C-c r", "reverse-line")

-- Line numbers are 1-based throughout, matching the modeline and goto-line.
nem.command("strip-trailing-space", "Remove trailing whitespace from every line.",
  function()
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, (nem.buf.get_line(i):gsub("%s+$", "")))
    end
  end)
nem.bind("C-c w", "strip-trailing-space")

-- Number the lines, to show changing the line count rather than just rewriting
-- lines in place. C-/ undoes the whole thing.
nem.command("number-lines", "Prefix every line with its number.",
  function()
    for i = 1, nem.buf.line_count() do
      nem.buf.set_line(i, i .. "  " .. nem.buf.get_line(i))
    end
  end)

-- Hooks fire around command dispatch. This one keeps trailing whitespace out of
-- Go files without you having to think about it.
nem.hook("before-save", function(buf)
  if buf.path and buf.path:match("%.go$") then
    nem.run("strip-trailing-space")
  end
end)

-- Note: scripts get string, table and math but NOT io or os, so a hook cannot
-- shell out to gofmt or any other binary. A formatter written in Lua against
-- nem.buf works; one that calls a program does not.

-- Uncomment either line to watch the error boundary hold: nem reports the
-- failure in the echo area at startup and carries on with defaults.
-- nem.bind("this is not a key", "forward-char")
-- error("deliberate config failure")
