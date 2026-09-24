// Package sysopen decides which files are not for a text editor, and hands
// those to the operating system's own application: a PDF to the document
// viewer, a song to the music player, a photo to the image viewer.
//
// It knows nothing of buffers or prompts. Whether to ask first, and what to
// remember, is the editor's business; this package answers "is it text?" and
// does "open it the way the desktop would".
package sysopen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

// sniffLen is how much of a file IsText reads. Enough to see past a text-like
// header - a PDF's first line, an SVG's XML declaration - and small enough to
// cost nothing on a large file.
const sniffLen = 8 << 10

// binaryExts are formats that are never usefully edited as text, whatever
// their first few kilobytes look like. A PDF can open with pages of plain
// objects before its first compressed stream, and an uncompressed WAV header
// is mostly ASCII; the extension settles those without reading further.
//
// Text formats with binary-sounding names - svg, html, ps, rtf - are
// deliberately absent: they are text, and people do edit them.
var binaryExts = map[string]bool{
	// documents
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "odt": true, "ods": true, "odp": true,
	"epub": true, "djvu": true, "mobi": true,
	// images
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true,
	"bmp": true, "ico": true, "tif": true, "tiff": true, "heic": true,
	"avif": true, "psd": true, "xcf": true, "raw": true,
	// audio and video
	"mp3": true, "flac": true, "wav": true, "ogg": true, "oga": true,
	"opus": true, "m4a": true, "aac": true, "wma": true, "mp4": true,
	"m4v": true, "mkv": true, "webm": true, "avi": true, "mov": true,
	"wmv": true, "flv": true, "mpg": true, "mpeg": true,
	// archives and disk images
	"zip": true, "gz": true, "tgz": true, "bz2": true, "xz": true,
	"zst": true, "7z": true, "rar": true, "tar": true, "iso": true,
	"dmg": true, "deb": true, "rpm": true, "apk": true, "jar": true,
	// fonts, programs, databases
	"ttf": true, "otf": true, "woff": true, "woff2": true,
	"exe": true, "dll": true, "so": true, "dylib": true, "o": true,
	"a": true, "class": true, "wasm": true, "pyc": true,
	"sqlite": true, "sqlite3": true, "db": true,
}

// Ext returns path's extension, lower-cased and without the dot: the key the
// editor remembers a choice under. A file with none gives "".
func Ext(path string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
}

// IsText reports whether the regular file at path is text worth opening in an
// editor.
//
// A known binary extension says no without reading. Otherwise the first
// sniffLen bytes decide: any NUL byte means binary, since text almost never
// holds one; and so does a sample that is more than a tenth invalid UTF-8.
// The tolerance is what lets a Latin-1 file with a few accented letters
// through as text, while compressed or encoded data - where invalid sequences
// run to about half - is caught. An empty file is text: it is what a new file
// looks like.
func IsText(path string) (bool, error) {
	if binaryExts[Ext(path)] {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, sniffLen)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	return looksLikeText(buf[:n]), nil
}

// looksLikeText is IsText's judgement on a sample.
func looksLikeText(b []byte) bool {
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	// The sample may end partway through a character. That is where it was
	// cut, not a fault in the file, so the fragment is dropped rather than
	// counted as invalid.
	for i := 1; i < utf8.UTFMax && i <= len(b); i++ {
		if utf8.RuneStart(b[len(b)-i]) {
			if !utf8.FullRune(b[len(b)-i:]) {
				b = b[:len(b)-i]
			}
			break
		}
	}
	chars, invalid := 0, 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			invalid++
		}
		chars++
		b = b[size:]
	}
	return invalid*10 <= chars
}

// ErrUnavailable is wrapped by the errors Available and Open return when the
// system has no way to open a file for the user.
var ErrUnavailable = errors.New("no system app to open files with")

// env is what choosing an opener consults, injectable so every platform's rule
// is testable on any one of them.
type env struct {
	goos     string
	lookPath func(string) (string, error)
	getenv   func(string) string
}

func realEnv() env { return env{runtime.GOOS, exec.LookPath, os.Getenv} }

// opener returns the command that opens path the way the desktop would.
//
// macOS has open and Windows has its shell's file association, reached through
// rundll32 because start is a cmd.exe builtin with quoting rules a path cannot
// be trusted to survive. Everything else is a freedesktop system, where
// xdg-open is the standard - and it needs a display: over plain SSH it would
// either fail or start a program on a screen nobody is looking at, so without
// one there is no opener. WSL has no xdg-open by default but often has
// wslview, which opens with the Windows side's app.
func (v env) opener(path string) ([]string, error) {
	switch v.goos {
	case "darwin":
		return []string{"open", path}, nil
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", path}, nil
	}
	if v.getenv("DISPLAY") == "" && v.getenv("WAYLAND_DISPLAY") == "" {
		if _, err := v.lookPath("wslview"); err == nil {
			return []string{"wslview", path}, nil
		}
		return nil, fmt.Errorf("%w: no graphical display", ErrUnavailable)
	}
	for _, name := range []string{"xdg-open", "wslview"} {
		if _, err := v.lookPath(name); err == nil {
			return []string{name, path}, nil
		}
	}
	return nil, fmt.Errorf("%w: xdg-open is not installed", ErrUnavailable)
}

// Available reports why files cannot be handed to the system, or nil when they
// can. It is cheap enough to ask before offering the choice at all.
func Available() error {
	_, err := realEnv().opener("x")
	return err
}

// reportWindow is how long after launch a failing opener is still reported.
// xdg-open fails at once when nothing handles a type; later, it may simply be
// the viewer it started being closed, which is not news.
const reportWindow = 5 * time.Second

// Open starts the system's app on path and returns without waiting for it.
//
// The app is detached: its output goes nowhere near nem's terminal, and it
// lives on after nem exits. If the opener fails soon after starting, failed is
// called - from another goroutine, so it must not touch editor state directly.
//
// The opener's messages are discarded rather than captured. Captured through
// a pipe, the viewer it starts would inherit that pipe: the goroutine would
// then wait on the viewer, not the opener, and once nem closed its end the
// viewer's next log line would kill it with SIGPIPE. The exit status is enough
// to say that opening failed.
func Open(path string, failed func(error)) error {
	argv, err := realEnv().opener(path)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	detach(cmd)
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		if err == nil || failed == nil || time.Since(start) > reportWindow {
			return
		}
		failed(fmt.Errorf("%s could not open %s (%w)", argv[0], filepath.Base(path), err))
	}()
	return nil
}
