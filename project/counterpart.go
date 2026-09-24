package project

import (
	"path"
	"slices"
	"strings"
)

// testAffixes are how languages name a file's tests, from its stem: foo.go has
// foo_test.go, foo.ts has foo.test.ts or foo.spec.ts, foo.py has test_foo.py,
// Foo.java has FooTest.java, foo.rb has foo_spec.rb.
var (
	testSuffixes = []string{"_test", ".test", ".spec", "_spec", "-test", "-spec", "Test", "Tests", "Spec"}
	testPrefixes = []string{"test_", "test-"}
)

// otherExt maps an extension to the one its tests may use instead: Elixir's
// tests are scripts, foo.ex tested by foo_test.exs.
var otherExt = map[string]string{".ex": ".exs", ".exs": ".ex"}

// Counterparts lists the files among files that are the other half of rel: its
// tests if it is code, the code it tests if it is a test. The nearest come
// first - in rel's own directory, then those sharing most of its path - so
// the one wanted is almost always the first.
func Counterparts(rel string, files []string) []string {
	want := map[string]bool{}
	for _, n := range counterpartNames(path.Base(rel)) {
		want[n] = true
	}
	var out []string
	for _, f := range files {
		if f != rel && want[path.Base(f)] {
			out = append(out, f)
		}
	}
	dir := path.Dir(rel)
	slices.SortStableFunc(out, func(a, b string) int {
		if sa, sb := sharedDirs(dir, path.Dir(a)), sharedDirs(dir, path.Dir(b)); sa != sb {
			return sb - sa
		}
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	return out
}

// IsTest reports whether name, a file's base name, is named as a test is.
func IsTest(name string) bool {
	_, ok := testedStem(name)
	return ok
}

// counterpartNames are the base names name's other half could have.
func counterpartNames(name string) []string {
	ext := path.Ext(name)
	exts := []string{ext}
	if o, ok := otherExt[ext]; ok {
		exts = append(exts, o)
	}
	if stem, ok := testedStem(name); ok {
		var out []string
		for _, e := range exts {
			out = append(out, stem+e)
		}
		return out
	}
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		return nil
	}
	var out []string
	for _, e := range exts {
		for _, s := range testSuffixes {
			out = append(out, stem+s+e)
		}
		for _, p := range testPrefixes {
			out = append(out, p+stem+e)
		}
	}
	return out
}

// testedStem is the stem of the file a test named name tests, and false if
// name is not a test's.
func testedStem(name string) (string, bool) {
	stem := strings.TrimSuffix(name, path.Ext(name))
	for _, s := range testSuffixes {
		if t, ok := strings.CutSuffix(stem, s); ok && t != "" {
			return t, true
		}
	}
	for _, p := range testPrefixes {
		if t, ok := strings.CutPrefix(stem, p); ok && t != "" {
			return t, true
		}
	}
	return "", false
}

// sharedDirs counts the leading directories a and b have in common, with
// being the same directory counting most of all.
func sharedDirs(a, b string) int {
	if a == b {
		return 1 << 20
	}
	pa, pb := strings.Split(a, "/"), strings.Split(b, "/")
	n := 0
	for n < len(pa) && n < len(pb) && pa[n] == pb[n] {
		n++
	}
	return n
}
