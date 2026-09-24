package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/keymap"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Whole-keystroke benchmarks: what one key costs from HandleKey through a
// drawn frame, on inputs large enough to show where the time goes.

// goSource is a realistic Go file of about n lines.
func goSource(n int) string {
	const unit = `// Package demo is filler for a benchmark.
package demo

import (
	"fmt"
	"strings"
)

// Widget holds a name and a count.
type Widget struct {
	Name  string
	Count int // how many
}

func (w *Widget) String() string {
	return fmt.Sprintf("%s=%d", strings.ToUpper(w.Name), w.Count)
}

func sum(xs []int) (total int) {
	for _, x := range xs {
		total += x /* running */
	}
	return total
}
`
	var b strings.Builder
	for b.Len() < n*30 {
		b.WriteString(unit)
	}
	return b.String()
}

// specKeysB parses an emacs key spec for a benchmark.
func specKeysB(b *testing.B, spec string) []keymap.Key {
	b.Helper()
	seq, err := keymap.ParseSpec(spec)
	if err != nil {
		b.Fatal(err)
	}
	for i := range seq {
		seq[i] = keymap.Normalize(seq[i])
	}
	return seq
}

// benchScreen is an editor on a w by h simulation screen.
func benchScreen(b *testing.B, w, h int) *Editor {
	b.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		b.Fatal(err)
	}
	scr.SetSize(w, h)
	b.Cleanup(scr.Fini)
	e, err := New(scr)
	if err != nil {
		b.Fatal(err)
	}
	e.SetBackupRoot(b.TempDir())
	return e
}

// benchEditor is an editor visiting a Go file of about n lines, point in the
// middle, with one frame already drawn.
func benchEditor(b *testing.B, w, h, n int) *Editor {
	b.Helper()
	e := benchScreen(b, w, h)
	path := filepath.Join(b.TempDir(), "big.go")
	if err := os.WriteFile(path, []byte(goSource(n)), 0o644); err != nil {
		b.Fatal(err)
	}
	buf, err := e.OpenFile(path)
	if err != nil {
		b.Fatal(err)
	}
	e.active.Visit(buf)
	e.active.Pt = text.Pos{Line: buf.NumLines() / 2}
	e.Redraw()
	return e
}

func pressB(b *testing.B, e *Editor, spec string) {
	b.Helper()
	for _, k := range specKeysB(b, spec) {
		e.HandleKey(k)
	}
}

func BenchmarkRedraw80x24(b *testing.B) {
	e := benchEditor(b, 80, 24, 20000)
	b.ReportAllocs()
	for b.Loop() {
		e.Redraw()
	}
}

func BenchmarkRedraw200x60(b *testing.B) {
	e := benchEditor(b, 200, 60, 20000)
	b.ReportAllocs()
	for b.Loop() {
		e.Redraw()
	}
}

func BenchmarkRedrawFourWindows(b *testing.B) {
	e := benchEditor(b, 200, 60, 20000)
	pressB(b, e, "C-x 3 C-x 2 C-x o C-x o C-x 2")
	b.ReportAllocs()
	for b.Loop() {
		e.Redraw()
	}
}

// A typed character, then the frame it produces - the editor's most frequent
// operation.
func BenchmarkTypeAndRedraw(b *testing.B) {
	e := benchEditor(b, 120, 40, 20000)
	x, del := specKeysB(b, "x")[0], specKeysB(b, "DEL")[0]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Typing then deleting keeps the line the same length, so every
		// iteration measures the same keystroke.
		if i%2 == 0 {
			e.HandleKey(x)
		} else {
			e.HandleKey(del)
		}
		i++
		e.Redraw()
	}
}

// Typing at the end of one 200KB line - minified JSON, say - with the view
// scrolled far to the right.
func BenchmarkTypeOnALongLine(b *testing.B) {
	e := benchScreen(b, 120, 40)
	long := strings.Repeat(`{"key":"value","n":12345},`, 8000)
	if err := e.Buf().Insert(text.Pos{}, []rune(long)); err != nil {
		b.Fatal(err)
	}
	e.active.Pt = e.Buf().End()
	e.Redraw()
	x, del := specKeysB(b, "x")[0], specKeysB(b, "DEL")[0]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%2 == 0 {
			e.HandleKey(x)
		} else {
			e.HandleKey(del)
		}
		i++
		e.Redraw()
	}
}

func BenchmarkNextLineAndRedraw(b *testing.B) {
	e := benchEditor(b, 120, 40, 20000)
	n, p := specKeysB(b, "C-n")[0], specKeysB(b, "C-p")[0]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Alternating keeps point in place, so each frame does the same work.
		if i%2 == 0 {
			e.HandleKey(n)
		} else {
			e.HandleKey(p)
		}
		i++
		e.Redraw()
	}
}

func BenchmarkPageDownAndRedraw(b *testing.B) {
	e := benchEditor(b, 120, 40, 20000)
	v, top := specKeysB(b, "C-v")[0], specKeysB(b, "M-<")[0]
	b.ReportAllocs()
	for b.Loop() {
		e.HandleKey(v)
		if e.active.Pt.Line > 19000 {
			e.HandleKey(top)
		}
		e.Redraw()
	}
}

// Pasting 200KB of code in one go.
func BenchmarkPaste200K(b *testing.B) {
	src := []rune(goSource(7000))
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		e := benchEditor(b, 120, 40, 100)
		e.paste.buf = src
		b.StartTimer()
		if err := e.insertPasted(); err != nil {
			b.Fatal(err)
		}
		e.Redraw()
	}
}

// Opening a 20MB file.
func BenchmarkLoadFile20MB(b *testing.B) {
	path := filepath.Join(b.TempDir(), "big.go")
	if err := os.WriteFile(path, []byte(goSource(700000)), 0o644); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := text.LoadFile(path); err != nil {
			b.Fatal(err)
		}
	}
}

// A keystroke at a directory prompt listing 3000 directories.
func BenchmarkFindFileKeystroke(b *testing.B) {
	dir := b.TempDir()
	for i := range 3000 {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("dir-%04d", i)), 0o755); err != nil {
			b.Fatal(err)
		}
	}
	c := newCompletion(command.DirectoryCompleter(), "")
	in := dir + string(filepath.Separator) + "f"
	b.ReportAllocs()
	for b.Loop() {
		c.refresh(in)
	}
}

// m and u in a listing of 3000 files.
func BenchmarkDiredMark3000(b *testing.B) {
	dir := b.TempDir()
	for i := range 3000 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file-%04d.txt", i)), nil, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	e := benchScreen(b, 120, 40)
	buf, err := e.Dired(dir)
	if err != nil {
		b.Fatal(err)
	}
	e.active.Visit(buf)
	m, u, top := specKeysB(b, "m")[0], specKeysB(b, "u")[0], specKeysB(b, "M-<")[0]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%2 == 0 {
			e.HandleKey(m)
		} else {
			e.HandleKey(u)
		}
		if i%100 == 99 {
			e.HandleKey(top)
		}
		i++
		e.Redraw()
	}
}

// Typing a run of characters and undoing it.
func BenchmarkTypeThenUndo(b *testing.B) {
	e := benchEditor(b, 120, 40, 2000)
	x, undo := specKeysB(b, "x")[0], specKeysB(b, "C-/")[0]
	b.ReportAllocs()
	for b.Loop() {
		for range 50 {
			e.HandleKey(x)
		}
		e.HandleKey(undo)
		e.Redraw()
	}
}

// RET and DEL in the middle of a 700,000-line file: a line split and a join.
func BenchmarkNewlineInHugeFile(b *testing.B) {
	e := benchEditor(b, 120, 40, 700000)
	ret, del := specKeysB(b, "RET")[0], specKeysB(b, "DEL")[0]
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%2 == 0 {
			e.HandleKey(ret)
		} else {
			e.HandleKey(del)
		}
		i++
		e.Redraw()
	}
}

// M-f across a 20KB line of words.
func BenchmarkForwardWordOnALongLine(b *testing.B) {
	e := benchScreen(b, 120, 40)
	if err := e.Buf().Insert(text.Pos{}, []rune(strings.Repeat("lorem ipsum dolor ", 1200))); err != nil {
		b.Fatal(err)
	}
	f := specKeysB(b, "M-f")[0]
	b.ReportAllocs()
	for b.Loop() {
		e.active.Pt = text.Pos{}
		for range 100 {
			e.HandleKey(f)
		}
	}
}

// A search that finds nothing in a 700,000-line file: every line scanned.
func BenchmarkSearchMissInHugeFile(b *testing.B) {
	e := benchEditor(b, 120, 40, 700000)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := command.SearchForward(e.Buf(), "no such text", text.Pos{}, true); ok {
			b.Fatal("found")
		}
	}
}

// Starting up: a new editor, the config loaded (no file) and the first frame.
func BenchmarkStartup(b *testing.B) {
	cfg := filepath.Join(b.TempDir(), "init.lua")
	b.ReportAllocs()
	for b.Loop() {
		e := benchScreen(b, 120, 40)
		_ = e.LoadConfig(cfg)
		e.Redraw()
		e.CloseConfig()
	}
}
