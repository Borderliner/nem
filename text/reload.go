package text

import (
	"bytes"
	"os"
)

// Revert replaces the buffer's text with its file's as it is on disk now,
// and leaves the buffer unmodified: what the file says is what the buffer
// says again, line endings and final newline included.
//
// It is one undoable edit, as emacs's revert is, so text thrown away by a
// revert that was not meant is a C-/ away. A file that already matches
// changes nothing and leaves nothing to undo.
func (b *Buffer) Revert() error {
	if b.path == "" {
		return ErrNoPath
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		return err
	}
	lines, crlf, finalNL := decodeLines(data)
	sep := "\n"
	if crlf {
		sep = "\r\n"
	}
	if bytes.Equal(b.encode(sep, b.finalNL), data) {
		b.crlf, b.finalNL = crlf, finalNL
		b.SetModified(false)
		return nil
	}

	n := len(lines) - 1
	for _, l := range lines {
		n += len(l.runes)
	}
	rs := make([]rune, 0, n)
	for i, l := range lines {
		if i > 0 {
			rs = append(rs, '\n')
		}
		rs = append(rs, l.runes...)
	}

	b.BeginUndoGroup()
	err = b.Delete(Pos{}, b.End())
	if err == nil {
		err = b.Insert(Pos{}, rs)
	}
	b.EndUndoGroup()
	if err != nil {
		return err
	}
	b.crlf, b.finalNL = crlf, finalNL
	b.SetModified(false)
	return nil
}
