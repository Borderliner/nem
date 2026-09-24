package text

import (
	"errors"
	"slices"
	"testing"
)

func bufFrom(s string) *Buffer {
	b := NewBuffer()
	if s != "" {
		if err := b.Insert(Pos{0, 0}, []rune(s)); err != nil {
			panic(err)
		}
		b.BreakUndo()
	}
	return b
}

func TestNewBufferHasOneEmptyLine(t *testing.T) {
	b := NewBuffer()
	if got := b.NumLines(); got != 1 {
		t.Fatalf("NumLines() = %d, want 1", got)
	}
	if got := b.Line(0).Len(); got != 0 {
		t.Errorf("line 0 length = %d, want 0", got)
	}
	if b.Modified() {
		t.Error("fresh buffer reports modified")
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		name string
		init string
		at   Pos
		ins  string
		want string
	}{
		{"into empty", "", Pos{0, 0}, "hi", "hi"},
		{"at line start", "world", Pos{0, 0}, "hello ", "hello world"},
		{"at line end", "hello", Pos{0, 5}, " there", "hello there"},
		{"mid line", "helo", Pos{0, 3}, "l", "hello"},
		{"newline splits line", "helloworld", Pos{0, 5}, "\n", "hello\nworld"},
		{"multiline insert", "ac", Pos{0, 1}, "b\nX\n", "ab\nX\nc"},
		{"on second line", "a\nc", Pos{1, 0}, "b", "a\nbc"},
		{"trailing newline", "a", Pos{0, 1}, "\n", "a\n"},
		{"unicode", "ab", Pos{0, 1}, "世", "a世b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bufFrom(tt.init)
			if err := b.Insert(tt.at, []rune(tt.ins)); err != nil {
				t.Fatalf("Insert() error = %v", err)
			}
			if got := b.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if !b.Modified() {
				t.Error("Modified() = false after insert")
			}
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		init     string
		from, to Pos
		want     string
	}{
		{"within line", "hello", Pos{0, 1}, Pos{0, 3}, "hlo"},
		{"to line end", "hello", Pos{0, 2}, Pos{0, 5}, "he"},
		{"whole line content", "hello", Pos{0, 0}, Pos{0, 5}, ""},
		{"joins two lines", "ab\ncd", Pos{0, 2}, Pos{1, 0}, "abcd"},
		{"across lines", "ab\ncd", Pos{0, 1}, Pos{1, 1}, "ad"},
		{"spans three lines", "ab\nXX\ncd", Pos{0, 1}, Pos{2, 1}, "ad"},
		{"empty range is a no-op", "hello", Pos{0, 2}, Pos{0, 2}, "hello"},
		{"reversed args normalize", "hello", Pos{0, 3}, Pos{0, 1}, "hlo"},
		{"entire buffer", "ab\ncd", Pos{0, 0}, Pos{1, 2}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bufFrom(tt.init)
			if err := b.Delete(tt.from, tt.to); err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if got := b.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBufferAlwaysHasAtLeastOneLine(t *testing.T) {
	b := bufFrom("a\nb\nc")
	if err := b.Delete(Pos{0, 0}, b.End()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := b.NumLines(); got != 1 {
		t.Errorf("NumLines() = %d, want 1", got)
	}
	if got := b.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
}

func TestOutOfRangeIsAnError(t *testing.T) {
	b := bufFrom("hello")
	if err := b.Insert(Pos{5, 0}, []rune("x")); err == nil {
		t.Error("Insert past last line: want error")
	}
	if err := b.Insert(Pos{0, 99}, []rune("x")); err == nil {
		t.Error("Insert past line end: want error")
	}
	if err := b.Delete(Pos{0, 0}, Pos{9, 0}); err == nil {
		t.Error("Delete past last line: want error")
	}
	if got := b.String(); got != "hello" {
		t.Errorf("buffer mutated by failed ops: %q", got)
	}
}

func TestText(t *testing.T) {
	b := bufFrom("hello\nworld\nagain")
	tests := []struct {
		name     string
		from, to Pos
		want     string
	}{
		{"within line", Pos{0, 1}, Pos{0, 4}, "ell"},
		{"whole first line", Pos{0, 0}, Pos{0, 5}, "hello"},
		{"across one boundary", Pos{0, 3}, Pos{1, 2}, "lo\nwo"},
		{"across two boundaries", Pos{0, 4}, Pos{2, 1}, "o\nworld\na"},
		{"empty", Pos{1, 2}, Pos{1, 2}, ""},
		{"reversed normalizes", Pos{0, 4}, Pos{0, 1}, "ell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(b.Text(tt.from, tt.to)); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarkAdjustsAcrossEdits(t *testing.T) {
	tests := []struct {
		name string
		init string
		mark Pos
		op   func(*Buffer) error
		want Pos
	}{
		{
			"insert before mark on same line shifts column",
			"hello", Pos{0, 4},
			func(b *Buffer) error { return b.Insert(Pos{0, 1}, []rune("XX")) },
			Pos{0, 6},
		},
		{
			"insert after mark leaves it alone",
			"hello", Pos{0, 1},
			func(b *Buffer) error { return b.Insert(Pos{0, 4}, []rune("XX")) },
			Pos{0, 1},
		},
		{
			"insert at the mark leaves it alone",
			"hello", Pos{0, 2},
			func(b *Buffer) error { return b.Insert(Pos{0, 2}, []rune("XX")) },
			Pos{0, 2},
		},
		{
			"multiline insert before mark shifts line and column",
			"hello", Pos{0, 4},
			func(b *Buffer) error { return b.Insert(Pos{0, 1}, []rune("A\nB")) },
			Pos{1, 4},
		},
		{
			"insert on an earlier line shifts only the line",
			"ab\ncd", Pos{1, 1},
			func(b *Buffer) error { return b.Insert(Pos{0, 1}, []rune("X\nY")) },
			Pos{2, 1},
		},
		{
			"delete before mark on same line shifts column back",
			"hello", Pos{0, 4},
			func(b *Buffer) error { return b.Delete(Pos{0, 1}, Pos{0, 3}) },
			Pos{0, 2},
		},
		{
			"delete spanning the mark collapses it to the start",
			"hello", Pos{0, 3},
			func(b *Buffer) error { return b.Delete(Pos{0, 1}, Pos{0, 4}) },
			Pos{0, 1},
		},
		{
			"delete after mark leaves it alone",
			"hello", Pos{0, 1},
			func(b *Buffer) error { return b.Delete(Pos{0, 3}, Pos{0, 5}) },
			Pos{0, 1},
		},
		{
			"line join pulls the mark up",
			"ab\ncd", Pos{1, 1},
			func(b *Buffer) error { return b.Delete(Pos{0, 2}, Pos{1, 0}) },
			Pos{0, 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bufFrom(tt.init)
			b.SetMark(tt.mark)
			if err := tt.op(b); err != nil {
				t.Fatalf("op error = %v", err)
			}
			if got := b.Mark(); !got.Equal(tt.want) {
				t.Errorf("Mark() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSavePointAdjustsToo(t *testing.T) {
	b := bufFrom("hello")
	b.SetSavePoint(Pos{0, 4})
	if err := b.Insert(Pos{0, 0}, []rune("XX")); err != nil {
		t.Fatal(err)
	}
	if got := b.SavePoint(); !got.Equal(Pos{0, 6}) {
		t.Errorf("SavePoint() = %v, want {0 6}", got)
	}
}

func TestEndAndClamp(t *testing.T) {
	b := bufFrom("ab\ncde")
	if got := b.End(); !got.Equal(Pos{1, 3}) {
		t.Errorf("End() = %v, want {1 3}", got)
	}
	tests := []struct{ in, want Pos }{
		{Pos{0, 1}, Pos{0, 1}},
		{Pos{-3, 0}, Pos{0, 0}},
		{Pos{9, 0}, Pos{1, 0}},
		{Pos{0, 99}, Pos{0, 2}},
		{Pos{0, -4}, Pos{0, 0}},
	}
	for _, tt := range tests {
		if got := b.ClampPos(tt.in); !got.Equal(tt.want) {
			t.Errorf("ClampPos(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSetPath(t *testing.T) {
	b := NewBuffer()
	if b.Path() != "" {
		t.Errorf("Path() = %q, want empty", b.Path())
	}
	b.SetPath("/tmp/x.txt")
	if b.Path() != "/tmp/x.txt" {
		t.Errorf("Path() = %q, want /tmp/x.txt", b.Path())
	}
}

// A read-only buffer refuses both primitives, and undo, and is left exactly as
// it was: a listing that half-applied a keystroke would no longer describe the
// directory it claims to.
func TestReadOnlyBufferRefusesEdits(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("one\ntwo")); err != nil {
		t.Fatal(err)
	}
	b.BreakUndo()
	b.SetReadOnly(true)

	if err := b.Insert(Pos{0, 1}, []rune("x")); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Insert returned %v, want ErrReadOnly", err)
	}
	if err := b.Delete(Pos{0, 0}, Pos{1, 0}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Delete returned %v, want ErrReadOnly", err)
	}
	if _, ok := b.Undo(); ok {
		t.Error("Undo reported success in a read-only buffer")
	}
	if got := b.String(); got != "one\ntwo" {
		t.Errorf("text = %q, want it untouched", got)
	}

	// Lifting it restores editing, and the history from before is intact.
	b.SetReadOnly(false)
	if _, ok := b.Undo(); !ok || b.String() != "" {
		t.Errorf("after SetReadOnly(false), undo gave %q, want the insert undone", b.String())
	}
}

// An edit guard sees each edit before it is made, and one it refuses leaves the
// text as it was. Regenerate is not an edit, and passes it.
func TestEditGuardVetsEdits(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("fixed: name")); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("refused")
	// Only the part after "fixed: " may change, and not by a newline.
	b.SetEditGuard(func(from, to Pos, ins []rune) error {
		if from.Col < 7 || from.Line != to.Line || slices.Contains(ins, '\n') {
			return refused
		}
		return nil
	})

	if err := b.Insert(Pos{0, 11}, []rune("s")); err != nil {
		t.Errorf("Insert in the open part: %v", err)
	}
	if err := b.Delete(Pos{0, 7}, Pos{0, 8}); err != nil {
		t.Errorf("Delete in the open part: %v", err)
	}
	for _, try := range []func() error{
		func() error { return b.Insert(Pos{0, 2}, []rune("x")) },
		func() error { return b.Insert(Pos{0, 9}, []rune("\n")) },
		func() error { return b.Delete(Pos{0, 5}, Pos{0, 9}) },
	} {
		if err := try(); !errors.Is(err, refused) {
			t.Errorf("guarded edit returned %v, want the guard's error", err)
		}
	}
	if got := b.String(); got != "fixed: ames" {
		t.Errorf("text = %q, want only the allowed edits made", got)
	}

	if b.Vet(Pos{0, 8}, Pos{0, 9}, nil) != nil || b.Vet(Pos{0, 2}, Pos{0, 2}, []rune("x")) == nil {
		t.Error("Vet does not answer as the guard would")
	}
	if got := b.String(); got != "fixed: ames" {
		t.Errorf("Vet changed the text to %q", got)
	}

	b.Regenerate([]rune("other"))
	if got := b.String(); got != "other" {
		t.Errorf("Regenerate under a guard gave %q", got)
	}
	b.SetEditGuard(nil)
	if err := b.Insert(Pos{}, []rune("x")); err != nil {
		t.Errorf("Insert with the guard removed: %v", err)
	}
}

// Regenerate rewrites a read-only buffer without lifting the flag, and leaves
// nothing to undo and nothing unsaved.
func TestRegenerateReplacesGeneratedText(t *testing.T) {
	b := NewBuffer()
	if err := b.Insert(Pos{}, []rune("old listing")); err != nil {
		t.Fatal(err)
	}
	b.SetReadOnly(true)

	b.Regenerate([]rune("new\nlisting"))

	if got := b.String(); got != "new\nlisting" {
		t.Errorf("text = %q, want the new listing", got)
	}
	if !b.ReadOnly() {
		t.Error("Regenerate lifted the read-only flag")
	}
	if b.Modified() {
		t.Error("a regenerated buffer reports unsaved changes")
	}
	b.SetReadOnly(false)
	if _, ok := b.Undo(); ok {
		t.Errorf("undo after Regenerate restored %q; the history should be gone", b.String())
	}
}

// RegenerateLine rewrites one line of a read-only buffer and leaves no trace
// in undo or in the modified flag.
func TestRegenerateLine(t *testing.T) {
	b := NewBuffer()
	b.Regenerate([]rune("one\ntwo\nthree"))
	b.SetReadOnly(true)

	b.RegenerateLine(1, []rune("TWO"))

	if got := b.String(); got != "one\nTWO\nthree" {
		t.Errorf("text = %q", got)
	}
	if b.Modified() || !b.ReadOnly() {
		t.Error("RegenerateLine left the buffer modified or writable")
	}
	b.SetReadOnly(false)
	if _, ok := b.Undo(); ok {
		t.Error("RegenerateLine left something to undo")
	}
}
