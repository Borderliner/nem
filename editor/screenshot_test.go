package editor

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// The README's screenshots, drawn by nem itself. Each scene is set up on the
// simulated screen the tests draw on, driven by the keys a user would press,
// and its cells written out as a page; docs/images/shoot.sh has a headless
// browser turn the pages into pictures. So a change to how nem looks is a
// run of the script away from the README showing it:
//
//	docs/images/shoot.sh
//
// Without NEM_SCREENSHOTS, the directory the pages go in, this does nothing.

// shotScale is how many pixels of the picture stand for one of the page:
// twice, so text is sharp on a high-density screen. The README shows the
// pictures at half their width.
const shotScale = 2

// shotCell is the size of a cell on the page, in pixels at scale.
const shotCellW, shotCellH = 9 * shotScale, 20 * shotScale

// shotColours are the terminal's own colours in the pictures: what a cell
// nem leaves at the terminal's default is drawn in.
const shotFG, shotBG = "#d9d4cb", "#1d1c1a"

// shot is one screen's worth of cells and where the cursor is.
type shot struct {
	cells      []tcell.SimCell
	w, h       int
	cx, cy     int
	showCursor bool
}

// capture takes what the screen shows now.
func capture(scr tcell.SimulationScreen) shot {
	cells, w, h := scr.GetContents()
	cx, cy, vis := scr.GetCursor()
	return shot{append([]tcell.SimCell(nil), cells...), w, h, cx, cy, vis}
}

// cssColour is c as CSS, or def where the terminal's own colour stands.
func cssColour(c tcell.Color, def string) string {
	if c == tcell.ColorDefault || c == tcell.ColorReset {
		return def
	}
	if v := c.Hex(); v >= 0 {
		return fmt.Sprintf("#%06x", v)
	}
	return def
}

// page is s as a page: a terminal window, its title bar saying title, on a
// quiet background, every cell at its place.
func (s shot) page(title string) (string, int, int) {
	const k = shotScale
	pad, inset, bar := 36*k, 14*k, 32*k
	w := 2*pad + 2*inset + s.w*shotCellW
	h := 2*pad + bar + 2*inset + s.h*shotCellH
	var sb strings.Builder
	fmt.Fprintf(&sb, `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:%[1]dpx;height:%[2]dpx;overflow:hidden;background:linear-gradient(135deg,#4b4f6b 0%%,#262838 100%%)}
.win{position:absolute;left:%[3]dpx;top:%[3]dpx;border-radius:%[4]dpx;overflow:hidden;box-shadow:0 %[5]dpx %[6]dpx rgba(0,0,0,.55);background:%[7]s}
.bar{height:%[8]dpx;background:#2c2b28;display:flex;align-items:center;padding:0 %[9]dpx;gap:%[10]dpx}
.dot{width:%[11]dpx;height:%[11]dpx;border-radius:50%%}
.title{position:absolute;left:0;right:0;text-align:center;color:#a39d93;font:%[12]dpx/%[8]dpx system-ui,sans-serif}
.term{position:relative;margin:%[9]dpx;width:%[13]dpx;height:%[14]dpx;font-family:"UbuntuSansMono Nerd Font Mono","DejaVu Sans Mono",monospace;font-size:%[15]dpx;line-height:%[16]dpx;color:%[17]s}
.c{position:absolute;height:%[16]dpx;text-align:center;white-space:pre}
</style></head><body><div class="win"><div class="bar"><span class="dot" style="background:#ff5f57"></span><span class="dot" style="background:#febc2e"></span><span class="dot" style="background:#28c840"></span><span class="title">%[18]s</span></div><div class="term">`,
		w, h, pad, 10*k, 18*k, 48*k, shotBG, bar, inset, 8*k, 12*k, 13*k,
		s.w*shotCellW, s.h*shotCellH, 15*k, shotCellH, shotFG, html.EscapeString(title))
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			c := s.cells[y*s.w+x]
			fg, bg, attr := c.Style.Decompose()
			fgc, bgc := cssColour(fg, shotFG), cssColour(bg, "")
			if attr&tcell.AttrReverse != 0 {
				fgc, bgc = cssColour(bg, shotBG), cssColour(fg, shotFG)
			}
			cursor := s.showCursor && x == s.cx && y == s.cy
			if cursor {
				fgc, bgc = shotBG, "#e8c07d"
			}
			glyph := string(c.Runes)
			width := 1
			if len(c.Runes) > 0 {
				ln := text.NewLine(c.Runes)
				width = max(int(ln.Width()), 1)
			}
			if glyph == "" || glyph == " " {
				if bgc == "" {
					continue
				}
				glyph = " "
			}
			style := fmt.Sprintf("left:%dpx;top:%dpx;width:%dpx;color:%s", x*shotCellW, y*shotCellH, width*shotCellW, fgc)
			if bgc != "" {
				style += ";background:" + bgc
			}
			if attr&tcell.AttrBold != 0 {
				style += ";font-weight:bold"
			}
			if attr&tcell.AttrUnderline != 0 {
				style += ";text-decoration:underline"
			}
			if attr&tcell.AttrItalic != 0 {
				style += ";font-style:italic"
			}
			fmt.Fprintf(&sb, `<span class="c" style="%s">%s</span>`, style, html.EscapeString(glyph))
		}
	}
	sb.WriteString(`</div></div></body></html>`)
	return sb.String(), w, h
}

// shootFiles writes files under root, by their paths.
func shootFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// snapper binds C-], where nothing else is, to taking the screen as it
// was last drawn: pressed inside a prompt, it takes the prompt open.
func snapper(t *testing.T, e *Editor, scr tcell.SimulationScreen) *shot {
	t.Helper()
	var taken shot
	if err := e.reg.Register(command.Command{Name: "zz-snap", Fn: func(command.Env) error {
		taken = capture(scr)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := bindSpec(e.keys, "C-]", "zz-snap"); err != nil {
		t.Fatal(err)
	}
	return &taken
}

func TestScreenshots(t *testing.T) {
	out := os.Getenv("NEM_SCREENSHOTS")
	if out == "" {
		t.Skip("NEM_SCREENSHOTS names no directory to draw the screenshots in")
	}
	// A home of its own, so dired says ~/code/nem, and a project in it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	proj := filepath.Join(home, "code", "wordfreq")
	shootFiles(t, proj, shotProject)

	scenes := []struct {
		name, title string
		w, h        int
		set         func(t *testing.T, e *Editor, scr tcell.SimulationScreen) shot
	}{
		{"hero", "nem ~/code/wordfreq", 118, 28, func(t *testing.T, e *Editor, scr tcell.SimulationScreen) shot {
			code := visit(t, e, filepath.Join(proj, "main.go"))
			d, err := e.Dired(proj)
			if err != nil {
				t.Fatal(err)
			}
			press(t, e, "C-x", "3")
			left, right := e.tree.Windows()[0], e.tree.Windows()[1]
			left.Visit(code)
			left.Pt = text.Pos{Line: 11, Col: 9}
			right.Visit(d)
			right.Pt = text.Pos{Line: 5, Col: 30}
			e.active = left
			snap := snapper(t, e, scr)
			feed(t, scr, txt("file"), key(t, "C-]", "C-g"))
			press(t, e, "M-x")
			return *snap
		}},
		{"rtl", "nem ~/code/wordfreq/notes.md", 84, 18, func(t *testing.T, e *Editor, scr tcell.SimulationScreen) shot {
			e.SetLineWrap(true)
			visit(t, e, filepath.Join(proj, "notes.md"))
			e.active.Pt = text.Pos{Line: 2, Col: 30}
			e.Redraw()
			return capture(scr)
		}},
		{"whichkey", "nem ~/code/wordfreq/main.go", 96, 24, func(t *testing.T, e *Editor, scr tcell.SimulationScreen) shot {
			visit(t, e, filepath.Join(proj, "main.go"))
			e.active.Pt = text.Pos{Line: 22, Col: 2}
			press(t, e, "C-x")
			e.fireWhichKey()
			e.Redraw()
			return capture(scr)
		}},
	}
	// Four columns to a tab, as most people who write Go set it.
	was := text.TabWidth
	text.TabWidth = 4
	defer func() { text.TabWidth = was }()
	for _, sc := range scenes {
		e, scr := newTestEditor(t)
		e.SetIcons(true)
		scr.SetSize(sc.w, sc.h)
		s := sc.set(t, e, scr)
		page, w, h := s.page(sc.title)
		if err := os.WriteFile(filepath.Join(out, sc.name+".html"), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, sc.name+".size"), []byte(fmt.Sprintf("%d,%d", w, h)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// shotProject is the little project the screenshots show.
var shotProject = map[string]string{
	"go.mod": "module example.com/wordfreq\n\ngo 1.27\n",
	"main.go": `package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// count tallies how often each word appears.
func count(sc *bufio.Scanner) map[string]int {
	seen := make(map[string]int)
	for sc.Scan() {
		for _, w := range strings.Fields(sc.Text()) {
			seen[strings.ToLower(w)]++
		}
	}
	return seen
}

func main() {
	words := count(bufio.NewScanner(os.Stdin))
	keys := make([]string, 0, len(words))
	for w := range words {
		keys = append(keys, w)
	}
	sort.Slice(keys, func(i, j int) bool {
		return words[keys[i]] > words[keys[j]]
	})
	for i, w := range keys {
		if i == 10 {
			break
		}
		fmt.Printf("%-12s %d\n", w, words[w])
	}
}
`,
	"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {}\n",
	"README.md":    "# wordfreq\n\nCounts the words it reads.\n",
	"LICENSE":      "MIT\n",
	"Makefile":     "build:\n\tgo build ./...\n",
	"notes.md": `# Notes

The counter treats "Word" and "word" as one, and splits on spaces alone, so "word," and "word" are two. That is worth fixing before anyone relies on the numbers.

یادداشت: شمارنده باید نشانه‌های نگارشی را هم کنار بگذارد، وگرنه «کلمه،» و «کلمه» دو واژهٔ جدا شمرده می‌شوند. این را پیش از انتشار درست کنیم.

- [ ] strip punctuation
- [ ] read files named on the command line, not only stdin
`,
	"testdata/sample.txt": "the cat sat on the mat\n",
	"docs/design.md":      "# Design\n",
}
