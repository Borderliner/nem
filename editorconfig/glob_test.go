package editorconfig

import "testing"

// Each glob is matched against paths relative to the .editorconfig's
// directory, with a leading slash, as Lookup hands them over.
func TestGlobs(t *testing.T) {
	for _, tc := range []struct {
		glob string
		yes  []string
		no   []string
	}{
		// No slash: the name, in any directory below.
		{"*", []string{"/a", "/a.go", "/src/a.go"}, nil},
		{"*.go", []string{"/a.go", "/src/deep/a.go"}, []string{"/a.gox", "/a.c", "/go"}},
		{"Makefile", []string{"/Makefile", "/src/Makefile"}, []string{"/makefile", "/Makefile.am"}},
		{"*.min.js", []string{"/a.min.js"}, []string{"/a.js"}},

		// A slash anchors it where the .editorconfig is.
		{"src/*.go", []string{"/src/a.go"}, []string{"/a.go", "/x/src/a.go", "/src/sub/a.go"}},
		{"/src/*.go", []string{"/src/a.go"}, []string{"/x/src/a.go"}},
		{"lib/**.js", []string{"/lib/a.js", "/lib/x/y/a.js"}, []string{"/a.js"}},
		{"a/**/b.c", []string{"/a/b.c", "/a/x/b.c", "/a/x/y/b.c"}, []string{"/a.b.c", "/x/a/b.c"}},
		{"**/vendor/*", []string{"/vendor/a", "/x/vendor/a"}, []string{"/vendor/x/a"}},

		// * stops at a slash; ** does not.
		{"a*z.c", []string{"/az.c", "/amnz.c", "/q/amnz.c"}, []string{"/am/nz.c"}},
		{"/a**z.c", []string{"/az.c", "/am/nz.c", "/a/z.c"}, []string{"/q/az.c"}},

		// ?
		{"?.c", []string{"/a.c", "/x/b.c"}, []string{"/ab.c", "/.c"}},
		{"/a?b", []string{"/axb"}, []string{"/a/b"}},

		// Brackets.
		{"*.[ch]", []string{"/a.c", "/a.h"}, []string{"/a.o", "/a.ch"}},
		{"[a-c].txt", []string{"/a.txt", "/b.txt", "/c.txt"}, []string{"/d.txt"}},
		{"[!a-c].txt", []string{"/d.txt"}, []string{"/a.txt"}},
		{"[^a].txt", []string{"/b.txt"}, []string{"/a.txt"}},
		{"[]x].txt", []string{"/].txt", "/x.txt"}, []string{"/y.txt"}},
		{`[\]].txt`, []string{"/].txt"}, []string{"/x.txt"}},
		{"[a-].txt", []string{"/a.txt", "/-.txt"}, []string{"/b.txt"}},
		{"[.txt", []string{"/[.txt"}, []string{"/a.txt"}},
		{"/[a/b].txt", []string{"/[a/b].txt"}, []string{"/a.txt"}},

		// Braces.
		{"*.{js,ts}", []string{"/a.js", "/a.ts"}, []string{"/a.go", "/a.{js,ts}"}},
		{"{a,{b,c}}.go", []string{"/a.go", "/b.go", "/c.go"}, []string{"/d.go"}},
		{"{*.js,lib/*.go}", []string{"/x.js", "/lib/x.go"}, nil},
		{"{single}.c", []string{"/{single}.c"}, []string{"/single.c"}},
		{"{}.c", []string{"/{}.c"}, []string{"/.c"}},
		{"a{b.c", []string{"/a{b.c"}, []string{"/ab.c"}},
		{"{a,b}}.c", []string{"/a}.c"}, []string{"/a.c"}},
		{`{a\,b,c}.c`, []string{"/a,b.c", "/c.c"}, []string{"/a.c"}},

		// Numeric ranges.
		{"file{1..3}.txt", []string{"/file1.txt", "/file3.txt", "/file+2.txt"}, []string{"/file0.txt", "/file4.txt", "/filea.txt"}},
		{"n{-2..2}", []string{"/n-2", "/n0", "/n2"}, []string{"/n-3", "/n3"}},
		{"n{10..3}", []string{"/n3", "/n10"}, []string{"/n11"}},
		{"{x,{1..3}}", []string{"/x", "/2"}, []string{"/4"}},

		// Escapes.
		{`\*.c`, []string{"/*.c"}, []string{"/a.c"}},
		{`a\?`, []string{"/a?"}, []string{"/ab"}},
		{`\{a,b\}`, []string{"/{a,b}"}, []string{"/a"}},

		// Regular-expression characters are just characters.
		{"a+b(c).txt", []string{"/a+b(c).txt"}, []string{"/aab(c).txt"}},
		{"a.c", []string{"/a.c"}, []string{"/abc"}},
	} {
		g := compileGlob(tc.glob)
		if g == nil {
			t.Errorf("glob %q did not compile", tc.glob)
			continue
		}
		for _, p := range tc.yes {
			if !g.match(p) {
				t.Errorf("glob %q does not match %q, want it to (regexp %s)", tc.glob, p, g.re)
			}
		}
		for _, p := range tc.no {
			if g.match(p) {
				t.Errorf("glob %q matches %q, want it not to (regexp %s)", tc.glob, p, g.re)
			}
		}
	}
}

// A glob that cannot be compiled matches nothing, rather than failing the
// file around it.
func TestABadGlobMatchesNothing(t *testing.T) {
	g := compileGlob("[z-a].c")
	if g.match("/b.c") || g.match("/z-a.c") {
		t.Error("a glob with a backwards range matched")
	}
}
