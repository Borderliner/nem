-- Example nem config. Copy to ~/.config/nem/init.lua and restart nem.
--
-- Everything here uses only the real API surface: nem.set, nem.bind,
-- nem.command, nem.run, nem.hook, nem.api_version, and the nem.buf table
-- (line, replace_line, point, set_point, path, modified, text).

nem.set("tab-width", 4)
nem.set("scroll-margin", 3)

-- A plain rebinding: F5 saves.
nem.bind("<f5>", "save-buffer")

-- A command of your own. It appears in M-x and <f1> k describes it, exactly
-- like a built-in -- Lua commands are registered in the same table.
nem.command("reverse-line", "Reverse the characters on the current line.",
  function()
    nem.buf.replace_line(nem.buf.line():reverse())
  end)
nem.bind("C-c r", "reverse-line")

-- Strip trailing whitespace from the current line. Observable: put spaces at
-- the end of a line, press C-c w, and watch the column jump back.
nem.command("strip-trailing-space", "Remove trailing whitespace on this line.",
  function()
    nem.buf.replace_line((nem.buf.line():gsub("%s+$", "")))
  end)
nem.bind("C-c w", "strip-trailing-space")

-- A hook, to prove dispatch seams fire. Strips the current line before every
-- save, so trailing spaces never reach disk.
nem.hook("before-save", function(buf)
  if buf.path and buf.path:match("%.go$") then
    nem.run("strip-trailing-space")
  end
end)

-- Uncomment either line to watch the error boundary hold: nem should report
-- the failure in the echo area at startup and keep running with defaults.
-- nem.bind("this is not a key", "forward-char")
-- error("deliberate config failure")
