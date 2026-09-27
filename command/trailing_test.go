package command_test

import (
	"errors"
	"testing"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/command/commandtest"
	"github.com/Borderliner/nem/text"
)

func TestDeleteTrailingWhitespaceInTheBuffer(t *testing.T) {
	f := commandtest.New("a  ", "b\t", "c", " \t ", "d  e", "")
	f.SetPoint(text.Pos{Line: 0, Col: 3}) // in the whitespace that goes
	if err := tryRun(t, f, "delete-trailing-whitespace"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a\nb\nc\n\nd  e\n")
	pointIs(t, f, text.Pos{Line: 0, Col: 1})
	if got := lastEcho(t, f.Echoes); got != "Deleted trailing whitespace on 3 lines" {
		t.Errorf("echo %q", got)
	}

	// All of it, as one undo.
	f.Buf().BreakUndo()
	f.Buf().Undo()
	textIs(t, f, "a  \nb\t\nc\n \t \nd  e\n")
}

// With a region, only its lines - and not the line it ends at the start of.
func TestDeleteTrailingWhitespaceInTheRegion(t *testing.T) {
	f := commandtest.New("a ", "b ", "c ", "d ")
	activate(f, text.Pos{Line: 1, Col: 1}, text.Pos{Line: 3})
	if err := tryRun(t, f, "delete-trailing-whitespace"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a \nb\nc\nd ")
	if got := lastEcho(t, f.Echoes); got != "Deleted trailing whitespace on 2 lines" {
		t.Errorf("echo %q", got)
	}
}

func TestDeleteTrailingWhitespaceWithNoneToDelete(t *testing.T) {
	f := commandtest.New("a", "b c")
	if err := tryRun(t, f, "delete-trailing-whitespace"); err != nil {
		t.Fatal(err)
	}
	if got := lastEcho(t, f.Echoes); got != "No trailing whitespace" {
		t.Errorf("echo %q", got)
	}
	if f.Buf().Modified() {
		t.Error("nothing deleted, yet the buffer is modified")
	}
}

func TestDeleteTrailingWhitespaceIsRefusedByAReadOnlyBuffer(t *testing.T) {
	f := commandtest.New("a  ")
	f.Buf().SetReadOnly(true)
	if err := tryRun(t, f, "delete-trailing-whitespace"); !errors.Is(err, text.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	textIs(t, f, "a  ")
}

// A buffer read-only in parts keeps those parts; the rest is trimmed.
func TestDeleteTrailingWhitespacePassesOverWhatCannotBeEdited(t *testing.T) {
	f := commandtest.New("a ", "b ", "c ")
	f.Buf().SetEditGuard(func(from, _ text.Pos, _ []rune) error {
		if from.Line == 1 {
			return errors.New("fixed")
		}
		return nil
	})
	if err := tryRun(t, f, "delete-trailing-whitespace"); err != nil {
		t.Fatal(err)
	}
	textIs(t, f, "a\nb \nc")
}

func TestTrimTrailingWhitespaceCountsWhatItChanged(t *testing.T) {
	b := commandtest.New("x ", "y", "z\t\t").Buf()
	n, err := command.TrimTrailingWhitespace(b, 0, 2)
	if err != nil || n != 2 {
		t.Errorf("TrimTrailingWhitespace = %d, %v; want 2, nil", n, err)
	}
	if got := b.String(); got != "x\ny\nz" {
		t.Errorf("text %q", got)
	}
}
