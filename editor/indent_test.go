package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Borderliner/nem/text"
)

// configured visits a file called name holding content, in a directory of its
// own whose .editorconfig is root = true followed by config - root, so what
// the machine running the test has above its temporary directory stays out
// of it.
func configured(t *testing.T, e *Editor, name, config, content string) *text.Buffer {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n"+config), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	b := visit(t, e, path)
	e.Active().Pt = text.Pos{}
	return b
}

// TAB indents at point the way the file does.
func TestTabIndentsAsTheFileDoes(t *testing.T) {
	e, _ := newTestEditor(t)
	configured(t, e, "a.yaml", "", "key:\n")
	press(t, e, "C-e", "RET", "TAB")
	wantText(t, e, "key:\n  ")

	// And a file whose .editorconfig says otherwise.
	e, _ = newTestEditor(t)
	configured(t, e, "a.yaml", "[*.yaml]\nindent_size = 4\n", "key:\n")
	press(t, e, "C-e", "RET", "TAB")
	wantText(t, e, "key:\n    ")
}
