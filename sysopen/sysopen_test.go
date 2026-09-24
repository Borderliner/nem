package sysopen

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// What counts as text: what a person would edit, including a file in an old
// encoding, and not what only a program reads.
func TestIsText(t *testing.T) {
	latin1 := []byte("caf\xe9 cr\xe8me br\xfbl\xe9e, a menu written long ago\n")
	// A cut through a multibyte character at the sample's end must not count.
	cut := append(bytes.Repeat([]byte("a"), sniffLen-1), "é"...)
	noisy := make([]byte, 400)
	for i := range noisy {
		noisy[i] = byte(0x80 + i%64) // continuation bytes with no lead: invalid
	}

	for _, tc := range []struct {
		name string
		file string
		data []byte
		want bool
	}{
		{"plain text", "notes.txt", []byte("hello\nworld\n"), true},
		{"utf-8", "poem.md", []byte("naïve café — ☕\n"), true},
		{"empty", "new.go", nil, true},
		{"latin-1", "menu.txt", latin1, true},
		{"cut character", "long.txt", cut, true},
		{"svg is text", "logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), true},
		{"a NUL byte", "data", []byte("abc\x00def"), false},
		{"mostly invalid", "blob", noisy, false},
		{"pdf by extension", "paper.pdf", []byte("%PDF-1.7\nplain looking header\n"), false},
		{"extension case", "PHOTO.JPG", []byte("looks like text"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsText(write(t, tc.file, tc.data))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("IsText(%s) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

func TestIsTextReportsAMissingFile(t *testing.T) {
	if _, err := IsText(filepath.Join(t.TempDir(), "gone.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want not-exist", err)
	}
}

func TestExt(t *testing.T) {
	for in, want := range map[string]string{
		"a/b/Photo.PNG": "png", "archive.tar.gz": "gz", "Makefile": "", ".bashrc": "bashrc",
	} {
		if got := Ext(in); got != want {
			t.Errorf("Ext(%q) = %q, want %q", in, got, want)
		}
	}
}

// Each platform's opener, and the freedesktop rule that without a display
// there is none: over plain SSH, xdg-open would fail or open a window nobody
// can see.
func TestOpener(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			if slices.Contains(names, n) {
				return "/usr/bin/" + n, nil
			}
			return "", errors.New("not found")
		}
	}
	vars := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}

	for _, tc := range []struct {
		name string
		env  env
		want []string // nil means unavailable
	}{
		{"macOS", env{"darwin", has(), vars()}, []string{"open", "f.pdf"}},
		{"Windows", env{"windows", has(), vars()}, []string{"rundll32", "url.dll,FileProtocolHandler", "f.pdf"}},
		{"X11", env{"linux", has("xdg-open"), vars("DISPLAY", ":0")}, []string{"xdg-open", "f.pdf"}},
		{"Wayland", env{"freebsd", has("xdg-open"), vars("WAYLAND_DISPLAY", "wayland-0")}, []string{"xdg-open", "f.pdf"}},
		{"no display", env{"linux", has("xdg-open"), vars()}, nil},
		{"no xdg-open", env{"linux", has(), vars("DISPLAY", ":0")}, nil},
		{"WSL", env{"linux", has("wslview"), vars()}, []string{"wslview", "f.pdf"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.env.opener("f.pdf")
			if tc.want == nil {
				if !errors.Is(err, ErrUnavailable) {
					t.Errorf("opener = %q, %v; want ErrUnavailable", got, err)
				}
				return
			}
			if err != nil || !slices.Equal(got, tc.want) {
				t.Errorf("opener = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
