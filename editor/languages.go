package editor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/syntax"
	"github.com/Borderliner/nem/text"
)

// registerLanguageCommands adds edit-language.
func registerLanguageCommands(e *Editor, reg *command.Registry) error {
	return reg.Register(command.Command{
		Name:        "edit-language",
		Doc:         "Edit how a language is coloured, in its .syntax file beside init.lua; saving it recolours every buffer.",
		Interactive: true,
		Fn:          func(command.Env) error { return e.editLanguage() },
	})
}

// editLanguage opens the user's definition of a language, the one the buffer
// on screen is written in unless another is asked for. A language with no
// definition of the user's yet starts as a copy of nem's own, to change as
// they like - saved, it replaces nem's - or, for a language nem does not
// know, as the outline of one.
func (e *Editor) editLanguage() error {
	if e.syntaxDir == "" {
		return errors.New("edit-language: nem found no config directory to keep languages in")
	}
	// The buffer's own language first, so RET takes it.
	var names []string
	current := e.LanguageOf(e.active.Buf)
	if current != nil {
		names = append(names, current.Name())
	}
	for _, l := range e.languages().Languages() {
		if l != current {
			names = append(names, l.Name())
		}
	}
	name, err := e.ReadString(command.ReadOpts{
		Prompt:   "Edit language: ",
		History:  "language",
		Complete: command.CompleteFrom(names),
	})
	if err != nil || name == "" {
		return err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !syntax.ValidName(name) {
		return fmt.Errorf("%q cannot name a language: lower-case letters, digits and + # . _ -", name)
	}
	if err := os.MkdirAll(e.syntaxDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(e.syntaxDir, name+".syntax")
	_, statErr := os.Stat(path)
	if err := e.visitFile(path); err != nil {
		return err
	}
	b := e.active.Buf
	if statErr == nil || b.NumLines() > 1 || b.Line(0).Len() > 0 {
		return nil // the user's own, or one being written already
	}
	src, builtin := syntax.BuiltinSource(name)
	if !builtin {
		src = languageOutline(name)
	}
	if err := b.Insert(text.Pos{}, []rune(src)); err != nil {
		return err
	}
	e.active.Pt = text.Pos{}
	if builtin {
		e.Echo("nem's own %s: change it and save, and yours replaces it", name)
	} else {
		e.Echo("a new language: fill it in and save, and %s files are coloured", name)
	}
	return nil
}

// languageOutline is where a new language starts: every directive a language
// usually needs, to be filled in. It loads as it is, so saving it before it
// is finished is no error.
func languageOutline(name string) string {
	return fmt.Sprintf(`# %[1]s, as nem colours it. Saving this file recolours every buffer, and
# a mistake is reported with its line. The format is on the wiki's
# Languages page, and nem's own languages are examples of it.

language %[1]s
files *.%[1]s

comment #
string "
string '

keywords
types
constants
`, name)
}
