package text

import (
	"errors"
	"os"
	"strings"
)

// ErrNoPath is returned by Save when the buffer has no associated file.
var ErrNoPath = errors.New("text: buffer has no file path")

// LoadFile reads path into a new buffer. A file that does not exist yields an
// empty buffer bound to that path rather than an error: that is find-file on a
// new file, not a failure. Invalid UTF-8 is replaced with U+FFFD.
func LoadFile(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			b := NewBuffer()
			b.path = path
			b.finalNL = true // a new file gets a trailing newline when saved
			return b, nil
		}
		return nil, err
	}

	b := NewBuffer()
	b.path = path

	// string(data) replaces invalid UTF-8 sequences with U+FFFD on conversion.
	content := string(data)

	// Line ending style is whatever the first line ending uses.
	if i := strings.IndexByte(content, '\n'); i > 0 && content[i-1] == '\r' {
		b.crlf = true
	}
	if b.crlf {
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}

	switch {
	case content == "":
		b.lines = []Line{NewLine(nil)}
		b.finalNL = false
	default:
		parts := strings.Split(content, "\n")
		if parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
			b.finalNL = true
		}
		b.lines = make([]Line, len(parts))
		for i, p := range parts {
			b.lines[i] = NewLine([]rune(p))
		}
	}

	b.undo = newUndoLog()
	return b, nil
}

// bytes renders the buffer using its recorded line ending style.
func (b *Buffer) bytes() []byte {
	sep := "\n"
	if b.crlf {
		sep = "\r\n"
	}
	var sb strings.Builder
	for i := range b.lines {
		if i > 0 {
			sb.WriteString(sep)
		}
		sb.WriteString(b.lines[i].String())
	}
	if b.finalNL {
		sb.WriteString(sep)
	}
	return []byte(sb.String())
}

// Save writes the buffer back to its file, preserving the line ending style
// and trailing-newline convention it was loaded with.
func (b *Buffer) Save() error {
	if b.path == "" {
		return ErrNoPath
	}
	if err := os.WriteFile(b.path, b.bytes(), 0o644); err != nil {
		return err
	}
	b.SetModified(false)
	b.BreakUndo()
	return nil
}

// SaveAs writes the buffer to path and adopts it as the buffer's file.
func (b *Buffer) SaveAs(path string) error {
	if path == "" {
		return ErrNoPath
	}
	// Write first and adopt the path only on success. Assigning the path up
	// front and then delegating to Save leaves a failed write with the buffer
	// claiming a file it was never written to, so a later C-x C-s would
	// silently write somewhere the user never asked for.
	if err := os.WriteFile(path, b.bytes(), 0o644); err != nil {
		return err
	}
	b.path = path
	b.SetModified(false)
	b.BreakUndo()
	return nil
}
