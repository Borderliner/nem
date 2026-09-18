package text

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMissingIsANewBufferNotAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "brand-new.txt")
	b, err := LoadFile(p)
	if err != nil {
		t.Fatalf("LoadFile() error = %v, want nil for a nonexistent file", err)
	}
	if b.NumLines() != 1 || b.Line(0).Len() != 0 {
		t.Errorf("new-file buffer = %q, want a single empty line", b.String())
	}
	if b.Path() != p {
		t.Errorf("Path() = %q, want %q", b.Path(), p)
	}
	if b.Modified() {
		t.Error("a freshly opened file reports modified")
	}
}

func TestLoadFileShapes(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantLines []string
	}{
		{"empty file", "", []string{""}},
		{"one line no newline", "hello", []string{"hello"}},
		{"one line trailing newline", "hello\n", []string{"hello"}},
		{"two lines", "a\nb\n", []string{"a", "b"}},
		{"two lines no final newline", "a\nb", []string{"a", "b"}},
		{"blank line in middle", "a\n\nb\n", []string{"a", "", "b"}},
		{"trailing blank line", "a\n\n", []string{"a", ""}},
		{"crlf", "a\r\nb\r\n", []string{"a", "b"}},
		{"crlf no final newline", "a\r\nb", []string{"a", "b"}},
		{"unicode", "héllo\n世界\n", []string{"héllo", "世界"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "f.txt")
			if err := os.WriteFile(p, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			b, err := LoadFile(p)
			if err != nil {
				t.Fatalf("LoadFile() error = %v", err)
			}
			if got := b.NumLines(); got != len(tt.wantLines) {
				t.Fatalf("NumLines() = %d, want %d (lines %q)", got, len(tt.wantLines), b.String())
			}
			for i, want := range tt.wantLines {
				if got := b.Line(i).String(); got != want {
					t.Errorf("line %d = %q, want %q", i, got, want)
				}
			}
			if b.Modified() {
				t.Error("freshly loaded buffer reports modified")
			}
		})
	}
}

func TestSaveRoundTripsBytesExactly(t *testing.T) {
	for _, content := range []string{
		"", "hello", "hello\n", "a\nb\n", "a\nb", "a\n\nb\n",
		"a\r\nb\r\n", "a\r\nb", "héllo\n世界\n",
	} {
		t.Run(content, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "f.txt")
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			b, err := LoadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Save(); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Errorf("round trip = %q, want %q", string(got), content)
			}
		})
	}
}

func TestSaveWritesEdits(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(Pos{0, 5}, []rune(" world")); err != nil {
		t.Fatal(err)
	}
	if !b.Modified() {
		t.Error("Modified() = false after edit")
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world\n" {
		t.Errorf("file = %q, want %q", string(got), "hello world\n")
	}
	if b.Modified() {
		t.Error("Modified() = true immediately after Save()")
	}
}

func TestSaveCRLFStyleSurvivesEditing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("a\r\nb\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Insert(Pos{1, 1}, []rune("\nc")); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "a\r\nb\r\nc\r\n" {
		t.Errorf("file = %q, want %q", string(got), "a\r\nb\r\nc\r\n")
	}
}

func TestLoadInvalidUTF8DoesNotPanic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte{'a', 0xff, 0xfe, 'b', '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := LoadFile(p)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	want := "a��b"
	if got := b.Line(0).String(); got != want {
		t.Errorf("line 0 = %q, want %q", got, want)
	}
}

func TestSaveWithoutPathIsAnError(t *testing.T) {
	b := NewBuffer()
	if err := b.Save(); err == nil {
		t.Error("Save() on a pathless buffer: want error")
	}
}

func TestSaveAsSetsPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.txt")
	b := NewBuffer()
	if err := b.Insert(Pos{0, 0}, []rune("hi")); err != nil {
		t.Fatal(err)
	}
	if err := b.SaveAs(p); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}
	if b.Path() != p {
		t.Errorf("Path() = %q, want %q", b.Path(), p)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hi" {
		t.Errorf("file = %q, want %q", string(got), "hi")
	}
}
