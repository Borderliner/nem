package text

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"unicode/utf8"
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
	b.lines, b.crlf, b.finalNL = decodeLines(data)
	b.undo = newUndoLog()
	return b, nil
}

// decodeLines splits a file's bytes into lines, reporting whether it uses
// CRLF - decided by its first line ending - and whether it ends in a newline.
//
// It decodes in one pass into one rune array that every line is a slice of,
// and makes every Line in one more allocation. Converting to a string,
// splitting it, and converting and then copying each part cost four copies of
// the file and two allocations a line: opening 20MB took 200ms. Each line's
// slice is capped at its own length, so an edit that grows it moves it to an
// array of its own rather than writing over the next line.
//
// Invalid UTF-8 becomes U+FFFD a byte at a time, as a []rune conversion does.
func decodeLines(data []byte) (lines []*Line, crlf, finalNL bool) {
	if i := bytes.IndexByte(data, '\n'); i > 0 && data[i-1] == '\r' {
		crlf = true
	}
	if len(data) == 0 {
		return []*Line{{}}, crlf, false
	}
	n := bytes.Count(data, []byte{'\n'}) + 1
	if data[len(data)-1] == '\n' {
		finalNL = true
		n--
	}

	runes := make([]rune, 0, utf8.RuneCount(data))
	slab := make([]Line, n)
	lines = make([]*Line, n)
	li, start := 0, 0
	for i := 0; i < len(data); {
		switch c := data[i]; {
		case c == '\n':
			slab[li].runes = runes[start:len(runes):len(runes)]
			lines[li] = &slab[li]
			li++
			start = len(runes)
			i++
		case c == '\r' && crlf && i+1 < len(data) && data[i+1] == '\n':
			i++ // the CR of a CRLF; the LF ends the line
		case c < utf8.RuneSelf:
			runes = append(runes, rune(c))
			i++
		default:
			r, size := utf8.DecodeRune(data[i:])
			runes = append(runes, r)
			i += size
		}
	}
	if li < n {
		slab[li].runes = runes[start:len(runes):len(runes)]
		lines[li] = &slab[li]
	}
	return lines, crlf, finalNL
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
