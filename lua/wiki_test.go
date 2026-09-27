package lua

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	glua "github.com/yuin/gopher-lua"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
)

// The wiki's Settings and Lua API pages are written by hand, and these keep
// them whole: a setting or a function added to the code without a word on
// the page fails here. The keys and commands are checked in the editor
// package, where the pages about them are generated.

// wikiPage reads one of the wiki's pages.
func wikiPage(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "docs", "wiki", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Every setting nem.set knows has a row in the Settings page's table and a
// section of its own there.
func TestWikiDescribesEverySetting(t *testing.T) {
	page := wikiPage(t, "Settings.md")
	for _, name := range knownSettings {
		if !strings.Contains(page, "| [`"+name+"`](#"+name+") |") {
			t.Errorf("Settings.md's table has no row for %s", name)
		}
		if !strings.Contains(page, "\n### "+name+"\n") {
			t.Errorf("Settings.md has no section for %s", name)
		}
	}
}

// Every function a script can call - in the nem table, and in nem.buf - is on
// the Lua API page.
func TestWikiDescribesEveryFunction(t *testing.T) {
	h, err := New(Options{Registry: command.NewRegistry(), Keymap: keymap.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	page := wikiPage(t, "Lua-API.md")
	nem := h.l.GetGlobal("nem").(*glua.LTable)
	nem.ForEach(func(k, v glua.LValue) {
		name := "nem." + k.String()
		if !strings.Contains(page, "`"+name) {
			t.Errorf("Lua-API.md does not describe %s", name)
		}
		if buf, ok := v.(*glua.LTable); ok {
			buf.ForEach(func(k, _ glua.LValue) {
				if name := "nem.buf." + k.String(); !strings.Contains(page, "`"+name+"(") {
					t.Errorf("Lua-API.md does not describe %s", name)
				}
			})
		}
	})
}
