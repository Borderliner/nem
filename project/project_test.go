package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// tree makes files under a fresh directory, each holding its own path. A name
// ending in / is made as an empty directory.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range names {
		p := filepath.Join(root, filepath.FromSlash(n))
		if strings.HasSuffix(n, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := n + "\n"
		if filepath.Base(n) == Marker {
			body = "" // a marker says nothing unless a test gives it rules
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRootIsTheNearestMarkedDirectory(t *testing.T) {
	root := tree(t, ".git/", "go.mod", "a/b/c.go", "sub/.git/", "sub/x/y.go", "mono/.projectile", "mono/app/z.go")
	for dir, want := range map[string]string{
		"a/b":      "",
		"sub/x":    "sub",
		"mono/app": "mono",
		".":        "",
	} {
		got, ok := Root(filepath.Join(root, dir))
		if w := filepath.Join(root, want); !ok || got != w {
			t.Errorf("Root(%s) = %q, %v; want %q", dir, got, ok, w)
		}
	}
}

// A build file makes a project only where there is no repository.
func TestRootFallsBackToABuildFile(t *testing.T) {
	root := tree(t, "tool/go.mod", "tool/cmd/main.go", "loose/notes.txt")
	if got, ok := Root(filepath.Join(root, "tool", "cmd")); !ok || got != filepath.Join(root, "tool") {
		t.Errorf("Root(tool/cmd) = %q, %v; want tool", got, ok)
	}
	if got, ok := Root(filepath.Join(root, "loose")); ok {
		t.Errorf("Root(loose) = %q; want no project", got)
	}
}

// A home directory kept in git is not one project of everything under it.
func TestRootPassesOverHome(t *testing.T) {
	home := tree(t, ".git/", "notes/todo.txt", "work/.projectile")
	t.Setenv("HOME", home)
	if got, ok := Root(filepath.Join(home, "notes")); ok {
		t.Errorf("Root(~/notes) = %q; want no project", got)
	}
	if got, ok := Root(filepath.Join(home, "work")); !ok || got != filepath.Join(home, "work") {
		t.Errorf("Root(~/work) = %q, %v; want the marked directory", got, ok)
	}
}

// Without git the tree is walked, passing over repositories' stores and
// dependency trees.
func TestFilesWalksTheTree(t *testing.T) {
	root := tree(t, ".projectile", "b.go", "a/x.go", "node_modules/dep/index.js", ".hg/store", "a/.cache/junk")
	got, err := Files(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".projectile", "a/x.go", "b.go"}; !slices.Equal(got, want) {
		t.Errorf("Files = %q, want %q", got, want)
	}
}

// A marker's patterns narrow the list: - and bare lines leave things out,
// anchored to the root with a leading /; + keeps only what it names.
func TestFilesFollowsTheMarkersRules(t *testing.T) {
	root := tree(t, "src/a.go", "src/gen/b.go", "src/c.log", "docs/d.md", "log/e.txt", "tmp/f")
	write := func(rules string) {
		if err := os.WriteFile(filepath.Join(root, Marker), []byte(rules), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ rules, want string }{
		{"-/log\n*.log\n# a comment\ntmp/\n", ".projectile docs/d.md src/a.go src/gen/b.go"},
		{"+/src\n-/src/gen\n", "src/a.go src/c.log"},
		{"-gen\n-/docs/*.md\n", ".projectile log/e.txt src/a.go src/c.log tmp/f"},
	} {
		write(tc.rules)
		got, err := Files(root)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, " ") != tc.want {
			t.Errorf("with %q: Files = %q, want %q", tc.rules, got, tc.want)
		}
	}
}

// In a git repository git says what the files are: .gitignore is honoured,
// new files are included, and a deleted file is not.
func TestFilesAsksGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root := tree(t, "keep.go", "gone.go", "ignored.log", ".gitignore", "sub/new.go")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "keep.go", "gone.go", ".gitignore")
	git("commit", "-q", "-m", "x")
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}

	got, err := Files(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".gitignore", "keep.go", "sub/new.go"}; !slices.Equal(got, want) {
		t.Errorf("Files = %q, want %q", got, want)
	}
}

func TestDirs(t *testing.T) {
	got := Dirs([]string{"a/b/c/x.go", "a/y.go", "z.go", "d/e.go"})
	if want := []string{"a/", "a/b/", "a/b/c/", "d/"}; !slices.Equal(got, want) {
		t.Errorf("Dirs = %q, want %q", got, want)
	}
}

func TestSearch(t *testing.T) {
	root := tree(t, "a.go", "b.go", "bin.dat")
	os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\tfoo := foo()\nbar\n"), 0o644)
	os.WriteFile(filepath.Join(root, "bin.dat"), []byte("foo\x00foo"), 0o644)
	// b.go is open with an unsaved edit, which is what gets searched.
	open := func(rel string) ([]string, bool) {
		if rel == "b.go" {
			return []string{"one", "two foo"}, true
		}
		return nil, false
	}

	got, more := Search(root, []string{"a.go", "b.go", "bin.dat"}, regexp.MustCompile("foo"), open)
	if more || len(got) != 2 {
		t.Fatalf("Search = %+v, more %v; want a line in a.go and one in b.go", got, more)
	}
	a, b := got[0], got[1]
	if a.File != "a.go" || a.Line != 1 || a.Col != 1 || a.Text != " foo := foo()" ||
		!slices.Equal(a.Spans, [][2]int{{1, 4}, {8, 11}}) {
		t.Errorf("a.go match = %+v", a)
	}
	if b.File != "b.go" || b.Line != 1 || b.Col != 4 {
		t.Errorf("b.go match = %+v", b)
	}
}

// A long line is shown around its match, and the spans follow.
func TestSearchShowsPartOfALongLine(t *testing.T) {
	root := tree(t, "min.js")
	line := strings.Repeat("x", 1000) + "needle" + strings.Repeat("y", 1000)
	os.WriteFile(filepath.Join(root, "min.js"), []byte(line), 0o644)
	got, _ := Search(root, []string{"min.js"}, regexp.MustCompile("needle"), func(string) ([]string, bool) { return nil, false })
	if len(got) != 1 {
		t.Fatalf("got %d matches", len(got))
	}
	m := got[0]
	if m.Col != 1000 || len([]rune(m.Text)) > shownWidth+2 || !strings.HasPrefix(m.Text, "…") || !strings.HasSuffix(m.Text, "…") {
		t.Errorf("match col %d, text of %d runes %q…", m.Col, len([]rune(m.Text)), m.Text[:20])
	}
	s := m.Spans[0]
	if string([]rune(m.Text)[s[0]:s[1]]) != "needle" {
		t.Errorf("span %v covers %q", s, string([]rune(m.Text)[s[0]:s[1]]))
	}
}

func TestSearchStopsAtMaxMatches(t *testing.T) {
	root := tree(t, "many.txt")
	os.WriteFile(filepath.Join(root, "many.txt"), []byte(strings.Repeat("hit\n", MaxMatches+5)), 0o644)
	got, more := Search(root, []string{"many.txt"}, regexp.MustCompile("hit"), func(string) ([]string, bool) { return nil, false })
	if len(got) != MaxMatches || !more {
		t.Errorf("got %d matches, more %v; want %d and more", len(got), more, MaxMatches)
	}
}

func TestCounterparts(t *testing.T) {
	files := []string{
		"pkg/foo.go", "pkg/foo_test.go", "other/foo_test.go",
		"src/app.ts", "src/app.test.ts", "src/app.spec.ts",
		"lib/calc.py", "tests/test_calc.py",
		"src/main/java/Foo.java", "src/test/java/FooTest.java",
		"lib/x.ex", "test/x_test.exs",
	}
	for rel, want := range map[string][]string{
		"pkg/foo.go":                 {"pkg/foo_test.go", "other/foo_test.go"},
		"pkg/foo_test.go":            {"pkg/foo.go"},
		"src/app.ts":                 {"src/app.spec.ts", "src/app.test.ts"},
		"src/app.test.ts":            {"src/app.ts"},
		"lib/calc.py":                {"tests/test_calc.py"},
		"tests/test_calc.py":         {"lib/calc.py"},
		"src/main/java/Foo.java":     {"src/test/java/FooTest.java"},
		"src/test/java/FooTest.java": {"src/main/java/Foo.java"},
		"lib/x.ex":                   {"test/x_test.exs"},
		"test/x_test.exs":            {"lib/x.ex"},
	} {
		if got := Counterparts(rel, files); !slices.Equal(got, want) {
			t.Errorf("Counterparts(%s) = %q, want %q", rel, got, want)
		}
	}
	if !IsTest("foo_test.go") || IsTest("contest.go") || IsTest("latest.txt") {
		t.Error("IsTest misjudges a name")
	}
}

func TestContains(t *testing.T) {
	for _, tc := range []struct {
		p    string
		want bool
	}{{"/p", true}, {"/p/a/b", true}, {"/pa", false}, {"/", false}, {"/q/p", false}} {
		if got := Contains("/p", tc.p); got != tc.want {
			t.Errorf("Contains(/p, %s) = %v", tc.p, got)
		}
	}
}
