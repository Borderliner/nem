package editor

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Borderliner/nem/command"
)

func tenLines() []string {
	var ls []string
	for i := range 10 {
		ls = append(ls, fmt.Sprintf("line %d", i))
	}
	return ls
}

// M-p brings back what was entered at this kind of prompt, older with each
// press; M-n comes forward again, to what was being typed.
func TestPromptHistory(t *testing.T) {
	e, scr := newTestEditor(t, tenLines()...)
	for _, n := range []string{"3", "7"} {
		feed(t, scr, txt(n), key(t, "RET"))
		press(t, e, "M-g", "M-g")
	}
	wantPt(t, e, 6, 0)

	// M-p twice is "3"; M-n once is "7"; RET goes there.
	feed(t, scr, txt("9"), key(t, "M-p", "M-p", "M-n", "RET"))
	press(t, e, "M-g", "M-g")
	wantPt(t, e, 6, 0)

	// Past the newest entry is what was typed.
	feed(t, scr, txt("2"), key(t, "M-p", "M-n", "RET"))
	press(t, e, "M-g", "M-g")
	wantPt(t, e, 1, 0)

	if got := e.mem.HistoryOf("line"); !slices.Equal(got, []string{"2", "7", "3"}) {
		t.Errorf("line history %q", got)
	}
}

// In a prompt with no candidate list, the arrows walk the history too: <up>
// at C-s is the last search.
func TestSearchHistoryWithArrows(t *testing.T) {
	e, scr := newTestEditor(t, tenLines()...)
	feed(t, scr, txt("line 4"), key(t, "RET"))
	press(t, e, "C-s")
	press(t, e, "M-<")
	feed(t, scr, key(t, "<up>", "RET"))
	press(t, e, "C-s")
	wantPt(t, e, 4, 6)
}

// M-x opens on the commands used last.
func TestCommandsUsedLastComeFirst(t *testing.T) {
	e, _ := newTestEditor(t, "x")
	e.mem.AddHistory("command", "count-words")
	e.mem.AddHistory("command", "dired")
	c := newCompletion(command.CompleteFrom(e.CommandNames()), "")
	c.history = e.mem.HistoryOf("command")
	c.refresh("")
	if got := []string{c.ranked[0].Candidate, c.ranked[1].Candidate}; !slices.Equal(got, []string{"dired", "count-words"}) {
		t.Errorf("M-x starts with %q, want the two used last, most recent first", got)
	}
	// Typing ranks as ever.
	c.refresh("fwc")
	if got := c.ranked[0].Candidate; got != "forward-char" {
		t.Errorf("typing fwc ranks %q first", got)
	}
}

// History outlives the session: another editor on the same state directory
// has it, and a prompt left empty adds nothing.
func TestHistoryPersists(t *testing.T) {
	dir := t.TempDir()
	e, scr := newTestEditor(t, tenLines()...)
	if err := e.UseStateDir(dir); err != nil {
		t.Fatal(err)
	}
	feed(t, scr, txt("5"), key(t, "RET"))
	press(t, e, "M-g", "M-g")
	feed(t, scr, key(t, "RET"))
	press(t, e, "M-g", "M-g")

	e2, _ := newTestEditor(t, "x")
	if err := e2.UseStateDir(dir); err != nil {
		t.Fatal(err)
	}
	if got := e2.mem.HistoryOf("line"); !slices.Equal(got, []string{"5"}) {
		t.Errorf("a new session sees %q, want [5]", got)
	}
}
