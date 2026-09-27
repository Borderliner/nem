package editorconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// tree writes files under a fresh directory and returns it. Every tree has a
// root .editorconfig at its top unless the test writes its own, so nothing
// above the temporary directory - on a developer's machine, anything at all -
// can leak into what a test sees.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if _, ok := files[FileName]; !ok {
		files[FileName] = "root = true\n"
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func lookup(t *testing.T, dir, name string) Props {
	t.Helper()
	return NewCache().Lookup(filepath.Join(dir, filepath.FromSlash(name)))
}

func TestLookup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		file  string
		want  Props
	}{
		{
			name: "the properties of a matching section",
			files: map[string]string{FileName: `root = true
[*]
indent_style = space
indent_size = 2
trim_trailing_whitespace = true
insert_final_newline = false
`},
			file: "a.js",
			want: Props{IndentStyle: "space", IndentSize: 2, TabWidth: 2, TrimTrailingWhitespace: True, InsertFinalNewline: False},
		},
		{
			name:  "nothing for a file no section matches",
			files: map[string]string{FileName: "root = true\n[*.py]\nindent_style = space\n"},
			file:  "a.go",
			want:  Props{},
		},
		{
			name: "a later section wins over an earlier one",
			files: map[string]string{FileName: `root = true
[*]
indent_style = space
indent_size = 4
[*.go]
indent_style = tab
`},
			file: "main.go",
			want: Props{IndentStyle: "tab", IndentSize: 4, TabWidth: 4},
		},
		{
			name: "a nearer file wins over a farther one",
			files: map[string]string{
				FileName:          "root = true\n[*]\nindent_style = space\nindent_size = 4\ninsert_final_newline = true\n",
				"web/" + FileName: "[*]\nindent_size = 2\n",
			},
			file: "web/app.js",
			want: Props{IndentStyle: "space", IndentSize: 2, TabWidth: 2, InsertFinalNewline: True},
		},
		{
			name: "root = true stops the search",
			files: map[string]string{
				FileName:          "root = true\n[*]\nindent_style = tab\n",
				"sub/" + FileName: "root = true\n[*]\nindent_size = 3\n",
			},
			file: "sub/x.c",
			want: Props{IndentSize: 3, TabWidth: 3},
		},
		{
			name: "root only counts before the first section",
			files: map[string]string{
				FileName:          "root = true\n[*]\nindent_style = tab\n",
				"sub/" + FileName: "[*]\nroot = true\nindent_size = 3\n",
			},
			file: "sub/x.c",
			want: Props{IndentStyle: "tab", IndentSize: 3, TabWidth: 3},
		},
		{
			name: "unset takes a property back out",
			files: map[string]string{
				FileName:          "root = true\n[*]\nindent_style = tab\ntrim_trailing_whitespace = true\n",
				"sub/" + FileName: "[*.md]\ntrim_trailing_whitespace = unset\n",
			},
			file: "sub/README.md",
			want: Props{IndentStyle: "tab", TrimTrailingWhitespace: Unset},
		},
		{
			name:  "keys and values are case-insensitive",
			files: map[string]string{FileName: "ROOT = TRUE\n[*]\nIndent_Style = Space\nINDENT_SIZE = 8\nInsert_Final_Newline = TRUE\n"},
			file:  "x",
			want:  Props{IndentStyle: "space", IndentSize: 8, TabWidth: 8, InsertFinalNewline: True},
		},
		{
			name:  "but a section's glob is case-sensitive",
			files: map[string]string{FileName: "root = true\n[*.MD]\nindent_size = 2\n"},
			file:  "x.md",
			want:  Props{},
		},
		{
			name:  "unknown properties and values are ignored",
			files: map[string]string{FileName: "root = true\n[*]\ncolor = blue\nindent_style = tabs\nindent_size = huge\ntrim_trailing_whitespace = maybe\n"},
			file:  "x",
			want:  Props{},
		},
		{
			name:  "comments, blank lines and stray lines are passed over",
			files: map[string]string{FileName: "# top\n; also\n\nroot = true\n\n[*]\n  # indented comment\nnot a pair\nindent_style = space\n"},
			file:  "x",
			want:  Props{IndentStyle: "space"},
		},
		{
			name:  "CRLF line endings and a byte-order mark",
			files: map[string]string{FileName: bom + "root = true\r\n[*]\r\nindent_size = 2\r\n"},
			file:  "x",
			want:  Props{IndentSize: 2, TabWidth: 2},
		},
		{
			name:  "indent_size = tab is the tab width",
			files: map[string]string{FileName: "root = true\n[*]\nindent_style = tab\nindent_size = tab\ntab_width = 4\n"},
			file:  "x",
			want:  Props{IndentStyle: "tab", IndentSize: 4, TabWidth: 4},
		},
		{
			name:  "a tab-indented file's indent size is its tab width",
			files: map[string]string{FileName: "root = true\n[*]\nindent_style = tab\ntab_width = 8\n"},
			file:  "x",
			want:  Props{IndentStyle: "tab", IndentSize: 8, TabWidth: 8},
		},
		{
			name:  "tab_width is not overridden by indent_size",
			files: map[string]string{FileName: "root = true\n[*]\nindent_size = 2\ntab_width = 8\n"},
			file:  "x",
			want:  Props{IndentSize: 2, TabWidth: 8},
		},
		{
			name:  "a glob with a slash is anchored at its .editorconfig",
			files: map[string]string{FileName: "root = true\n[lib/*.js]\nindent_size = 2\n"},
			file:  "src/lib/a.js",
			want:  Props{},
		},
		{
			name: "a glob in a lower .editorconfig is relative to it",
			files: map[string]string{
				"sub/" + FileName: "[lib/*.js]\nindent_size = 2\n",
			},
			file: "sub/lib/a.js",
			want: Props{IndentSize: 2, TabWidth: 2},
		},
		{
			name:  "a glob without a slash reaches every directory below",
			files: map[string]string{FileName: "root = true\n[*.js]\nindent_size = 2\n"},
			file:  "a/b/c/d.js",
			want:  Props{IndentSize: 2, TabWidth: 2},
		},
		{
			name:  "a file that does not exist yet is still configured",
			files: map[string]string{FileName: "root = true\n[*.py]\nindent_size = 4\n"},
			file:  "new/unsaved.py",
			want:  Props{IndentSize: 4, TabWidth: 4},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tree(t, tc.files)
			if got := lookup(t, dir, tc.file); got != tc.want {
				t.Errorf("Lookup(%s) = %+v, want %+v", tc.file, got, tc.want)
			}
		})
	}
}

// A directory called .editorconfig is not one.
func TestADirectoryNamedEditorconfigIsIgnored(t *testing.T) {
	dir := tree(t, map[string]string{"sub/" + FileName + "/x": ""})
	if got := lookup(t, dir, "sub/a.go"); got != (Props{}) {
		t.Errorf("Lookup = %+v, want nothing", got)
	}
}

// The cache answers from what it has parsed while the files are unchanged,
// and reads one again as soon as it changes.
func TestCacheRereadsOnlyWhatChanged(t *testing.T) {
	dir := tree(t, map[string]string{
		FileName:          "root = true\n[*]\nindent_style = tab\n",
		"sub/" + FileName: "[*]\nindent_size = 2\n",
	})
	c := NewCache()
	file := filepath.Join(dir, "sub", "a.go")

	want := Props{IndentStyle: "tab", IndentSize: 2, TabWidth: 2}
	if got := c.Lookup(file); got != want {
		t.Fatalf("first Lookup = %+v, want %+v", got, want)
	}
	if c.reads != 2 {
		t.Fatalf("first Lookup read %d files, want 2", c.reads)
	}
	for range 3 {
		if got := c.Lookup(file); got != want {
			t.Fatalf("repeated Lookup = %+v, want %+v", got, want)
		}
	}
	if c.reads != 2 {
		t.Errorf("repeated Lookups read %d files in all, want still 2", c.reads)
	}

	// Changed on disk - with a modification time moved on, since a fast
	// filesystem can write twice inside one tick.
	sub := filepath.Join(dir, "sub", FileName)
	if err := os.WriteFile(sub, []byte("[*]\nindent_size = 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(sub, later, later); err != nil {
		t.Fatal(err)
	}
	want = Props{IndentStyle: "tab", IndentSize: 4, TabWidth: 4}
	if got := c.Lookup(file); got != want {
		t.Errorf("Lookup after an edit = %+v, want %+v", got, want)
	}
	if c.reads != 3 {
		t.Errorf("Lookup after an edit read %d files in all, want 3", c.reads)
	}

	// And removed.
	if err := os.Remove(sub); err != nil {
		t.Fatal(err)
	}
	want = Props{IndentStyle: "tab"}
	if got := c.Lookup(file); got != want {
		t.Errorf("Lookup after removal = %+v, want %+v", got, want)
	}
}

// A relative path is taken from the working directory, like any other.
func TestLookupOfARelativePath(t *testing.T) {
	dir := tree(t, map[string]string{FileName: "root = true\n[*.txt]\nindent_size = 5\n"})
	t.Chdir(dir)
	if got := NewCache().Lookup("notes.txt"); got.IndentSize != 5 {
		t.Errorf("Lookup(relative) = %+v, want indent size 5", got)
	}
}
