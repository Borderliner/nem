package command_test

import (
	"testing"

	"github.com/Borderliner/nem/text"
)

// M-/ offers the nearest word first, then the next on each repeat, then the
// other buffers, and gives the typed word back when they run out.
func TestDabbrevExpand(t *testing.T) {
	f := fileEnv("a.go", "foobar foobaz", "fo", "foolish")
	f.AddBuffer("other", "fooother")
	f.SetPoint(text.Pos{Line: 1, Col: 2})

	for i, want := range []string{"foobaz", "foobar", "foolish", "fooother", "fo"} {
		runAll(t, f, "dabbrev-expand")
		if got := f.Buf().Line(1).String(); got != want {
			t.Fatalf("M-/ #%d gave %q, want %q", i+1, got, want)
		}
		if got := f.Point(); got != (text.Pos{Line: 1, Col: text.RuneIdx(len(want))}) {
			t.Errorf("M-/ #%d left point at %v", i+1, got)
		}
	}
	if got := lastEcho(t, f.Echoes); got != `No further dynamic expansion for "fo" found` {
		t.Errorf("after running out: %q", got)
	}
	if got := f.Text(); got != "foobar foobaz\nfo\nfoolish" {
		t.Errorf("buffer changed: %q", got)
	}
}

// Another command in between means the next M-/ starts a new expansion.
func TestDabbrevStartsAfreshAfterAnotherCommand(t *testing.T) {
	f := fileEnv("a.go", "alpha alps", "al")
	f.SetPoint(text.Pos{Line: 1, Col: 2})
	runAll(t, f, "dabbrev-expand") // alps
	f.SetLastCommand("forward-char")
	runAll(t, f, "dabbrev-expand") // expands "alps" further: nothing longer
	if got := f.Buf().Line(1).String(); got != "alps" {
		t.Errorf("got %q, want the finished word left alone", got)
	}
	if got := lastEcho(t, f.Echoes); got != `No dynamic expansion for "alps" found` {
		t.Errorf("echo %q", got)
	}
}

func TestDabbrevWithNothingBeforePoint(t *testing.T) {
	f := fileEnv("a.go", "word ", "")
	f.SetPoint(text.Pos{Line: 1})
	runAll(t, f, "dabbrev-expand")
	if got := f.Text(); got != "word \n" {
		t.Errorf("text %q", got)
	}
}
