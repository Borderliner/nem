// Package imenu finds the definitions in a file - its functions, types,
// classes, headings - for M-g i to jump to by name, as emacs's imenu does.
//
// It reads lines, not code: each language has a few patterns for the lines
// that begin a definition, and a definition is a line that matches one. That
// is approximate at the edges and fast everywhere, which is the trade imenu
// has always made; it needs no parser, and it is right for the way code is
// actually laid out.
package imenu

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is one definition.
type Entry struct {
	// Name is what the definition is called, with what kind of thing it is
	// where that helps tell two apart: "func main", "type Buffer",
	// "(*Buffer) Insert".
	Name string
	// Line is where it is, from 0.
	Line int
}

// rule turns the lines one pattern matches into entries. The pattern's
// "name" group is the name; "kind", when present, goes in front of it.
type rule struct {
	re *regexp.Regexp
}

func rules(patterns ...string) []rule {
	out := make([]rule, len(patterns))
	for i, p := range patterns {
		out[i] = rule{regexp.MustCompile(p)}
	}
	return out
}

var (
	goRules = rules(
		`^func\s+\((?P<recv>[^)]*)\)\s*(?P<name>\w+)`,
		`^(?P<kind>func)\s+(?P<name>\w+)`,
		`^(?P<kind>type)\s+(?P<name>\w+)`,
	)
	pythonRules = rules(
		`^\s*(?P<kind>class)\s+(?P<name>\w+)`,
		`^\s*(?:async\s+)?(?P<kind>def)\s+(?P<name>\w+)`,
	)
	jsRules = rules(
		`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?P<kind>class|interface|enum)\s+(?P<name>[\w$]+)`,
		`^\s*(?:export\s+)?(?:default\s+)?(?P<kind>type)\s+(?P<name>[\w$]+)\s*[=<]`,
		`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?(?P<kind>function)\*?\s+(?P<name>[\w$]+)`,
		`^\s*(?:export\s+)?(?:const|let|var)\s+(?P<name>[\w$]+)\s*(?::[^=]+)?=\s*(?:async\s+)?(?:function\b|\([^)]*\)\s*(?::[^=]+)?=>|[\w$]+\s*=>)`,
		`^\s{2,4}(?:(?:public|private|protected|static|async|get|set|readonly)\s+)*(?P<name>[\w$]+)\s*\([^)]*\)\s*(?::[^{]+)?\{\s*$`,
	)
	rustRules = rules(
		`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(?:const\s+)?(?:extern\s+"[^"]*"\s+)?(?P<kind>fn)\s+(?P<name>\w+)`,
		`^\s*(?:pub(?:\([^)]*\))?\s+)?(?P<kind>struct|enum|trait|union|mod|type|macro_rules!)\s*(?P<name>\w+)`,
		`^\s*(?P<kind>impl)(?:<[^>]*>)?\s+(?P<name>[\w:<>, ]+?)\s*(?:where\b.*)?\{`,
	)
	cRules = rules(
		`^(?P<kind>struct|enum|union|class|namespace)\s+(?P<name>\w+)\s*[:{]?\s*$`,
		`^(?:[\w:*&<>][\w:*&<>,\s]*?[\s*&])?(?P<name>[\w:~]+)\s*\([^;]*$`,
		`^#define\s+(?P<name>\w+)`,
	)
	javaRules = rules(
		`^\s*(?:(?:public|private|protected|abstract|final|static|sealed|open|internal|data|inner)\s+)*(?P<kind>class|interface|enum|record|object)\s+(?P<name>\w+)`,
		`^\s+(?:(?:public|private|protected|abstract|final|static|synchronized|override|suspend|internal|open)\s+)*(?:fun\s+)?[\w<>\[\],.? ]*?\b(?P<name>\w+)\s*\([^;]*\)\s*(?::[^{]*)?(?:throws [\w., ]+)?\s*\{?\s*$`,
	)
	rubyRules = rules(
		`^\s*(?P<kind>class|module)\s+(?P<name>[\w:]+)`,
		`^\s*(?P<kind>def)\s+(?P<name>(?:self\.)?[\w?!=]+)`,
	)
	luaRules = rules(
		`^\s*(?:local\s+)?(?P<kind>function)\s+(?P<name>[\w.:]+)`,
		`^\s*(?:local\s+)?(?P<name>[\w.]+)\s*=\s*function\b`,
	)
	shellRules = rules(
		`^\s*(?:function\s+)?(?P<name>[\w.-]+)\s*\(\)\s*\{?`,
		`^\s*function\s+(?P<name>[\w.-]+)`,
	)
	makeRules = rules(
		`^(?P<name>[\w./%-][\w./% -]*?)\s*::?(?:[^=]|$)`,
	)
	elispRules = rules(
		`^\s*\((?P<kind>defun|defmacro|defvar|defcustom|defconst)\s+(?P<name>[^\s()]+)`,
	)
	markdownHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
)

// rulesByExt picks a file's rules by its extension.
var rulesByExt = map[string][]rule{
	"go": goRules,
	"py": pythonRules, "pyi": pythonRules,
	"js": jsRules, "mjs": jsRules, "cjs": jsRules, "jsx": jsRules,
	"ts": jsRules, "mts": jsRules, "cts": jsRules, "tsx": jsRules,
	"rs": rustRules,
	"c":  cRules, "h": cRules, "cc": cRules, "cpp": cRules, "cxx": cRules, "hpp": cRules, "hh": cRules,
	"java": javaRules, "kt": javaRules, "kts": javaRules, "cs": javaRules, "scala": javaRules,
	"rb":  rubyRules,
	"lua": luaRules,
	"sh":  shellRules, "bash": shellRules, "zsh": shellRules,
	"mk": makeRules,
	"el": elispRules,
}

// For lists the definitions in a file at path whose text is lines, in order.
// A kind of file it has no rules for has none.
func For(path string, lines []string) []Entry {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	switch {
	case ext == "md" || ext == "markdown":
		return headings(lines)
	case base == "makefile" || base == "gnumakefile":
		return scan(lines, makeRules)
	}
	rs, ok := rulesByExt[ext]
	if !ok {
		return nil
	}
	return scan(lines, rs)
}

// scan applies rs to every line, the first that matches a line naming it.
func scan(lines []string, rs []rule) []Entry {
	var out []Entry
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// A comment is not a definition, however much it reads like one; nor
		// is a statement, which a loose pattern for a method can take for one:
		// return f(x) is a call.
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") ||
			(strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#define")) || strings.HasPrefix(t, "--") {
			continue
		}
		if f := strings.Fields(t); statement[f[0]] {
			continue
		}
		for _, r := range rs {
			if name, ok := match(r.re, line); ok {
				out = append(out, Entry{Name: name, Line: i})
				break
			}
		}
	}
	return out
}

// statement are the words a line starts with when it is a statement, not a
// definition.
var statement = map[string]bool{
	"return": true, "throw": true, "new": true, "else": true, "await": true, "yield": true,
	"echo": true, "print": true, "println": true, "assert": true, "defer": true, "go": true,
}

// keyword are words a loose pattern - one with no fn, def or class to go by
// - can take for a name: a C function pattern sees "if (x) {" and a Java
// method pattern "for (...) {".
var keyword = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "return": true, "catch": true,
	"else": true, "do": true, "sizeof": true, "new": true, "delete": true, "case": true,
	"try": true, "synchronized": true, "foreach": true, "using": true, "lock": true, "when": true,
}

// match is the entry name a rule gives for line: its name, after its kind or
// receiver where it has one.
func match(re *regexp.Regexp, line string) (string, bool) {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	get := func(g string) string {
		if i := re.SubexpIndex(g); i >= 0 {
			return strings.TrimSpace(m[i])
		}
		return ""
	}
	name, kind := get("name"), get("kind")
	if name == "" || (kind == "" && keyword[name]) {
		return name, false
	}
	if recv := get("recv"); recv != "" {
		// A method is named by its receiver's type: (*Buffer) Insert.
		fields := strings.Fields(recv)
		return "(" + fields[len(fields)-1] + ") " + name, true
	}
	if kind != "" {
		return kind + " " + name, true
	}
	return name, true
}

// headings lists a Markdown file's headings, indented by level, outside
// fenced code.
func headings(lines []string) []Entry {
	var out []Entry
	fence := ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			switch {
			case fence == "":
				fence = t[:3]
			case strings.HasPrefix(t, fence):
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if m := markdownHeading.FindStringSubmatch(line); m != nil {
			out = append(out, Entry{Name: strings.Repeat("  ", len(m[1])-1) + m[2], Line: i})
		}
	}
	return out
}
