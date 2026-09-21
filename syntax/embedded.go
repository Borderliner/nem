package syntax

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// rulesFS carries nem's own language definitions into the binary.
//
// They are embedded rather than read from disk so that highlighting works on a
// machine with nothing else installed. nano's files are read at runtime when
// they happen to be present, but they cannot be bundled: they are GPL, and
// carrying them would decide this project's licensing. These are written from
// each language's grammar instead.
//
//go:embed rules/*.nemrc
var rulesFS embed.FS

var (
	embeddedOnce sync.Once
	embeddedSets []*RuleSet
	embeddedErr  error
)

// loadEmbedded parses every bundled rule file, once.
//
// A failure here is a bug in a file that ships inside the binary, so it is kept
// and surfaced by EmbeddedError rather than swallowed - a rule file that stopped
// parsing would otherwise look exactly like a language quietly losing its
// colours.
func loadEmbedded() {
	entries, err := rulesFS.ReadDir("rules")
	if err != nil {
		embeddedErr = err
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".nemrc") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // deterministic match order

	for _, name := range names {
		data, err := rulesFS.ReadFile(filepath.Join("rules", name))
		if err != nil {
			embeddedErr = fmt.Errorf("reading %s: %w", name, err)
			return
		}
		rs, err := ParseRuleFile(name, strings.NewReader(string(data)))
		if err != nil {
			embeddedErr = err
			return
		}
		embeddedSets = append(embeddedSets, rs)
	}
}

func embedded() []*RuleSet {
	embeddedOnce.Do(loadEmbedded)
	return embeddedSets
}

// EmbeddedError reports a problem with the bundled rule files, or nil.
//
// Exported so a test can assert the binary's own definitions are sound; nothing
// at runtime can act on it.
func EmbeddedError() error {
	embeddedOnce.Do(loadEmbedded)
	return embeddedErr
}

// EmbeddedLanguages lists the bundled languages, sorted.
func EmbeddedLanguages() []string {
	out := make([]string, 0, len(embedded()))
	for _, rs := range embedded() {
		out = append(out, rs.name)
	}
	sort.Strings(out)
	return out
}

// EmbeddedRuleSet returns a bundled language by name, for tests.
func EmbeddedRuleSet(name string) *RuleSet {
	for _, rs := range embedded() {
		if rs.name == name {
			return rs
		}
	}
	return nil
}

// embeddedFor returns the bundled lexer for a file, or nil.
//
// nil rather than PlainLexer so a caller can tell "nothing of ours covers this"
// from "this is plain text", and fall through to whatever else it has.
func embeddedFor(path, firstLine string) Lexer {
	base := filepath.Base(path)
	for _, rs := range embedded() {
		if rs.matchesFile(base) {
			return rs
		}
	}
	for _, rs := range embedded() {
		if rs.matchesHeader(firstLine) {
			return rs
		}
	}
	return nil
}

// ForWithHeader picks a lexer knowing the file's first line.
//
// The first line is what identifies an extensionless script from its shebang -
// a file simply called `deploy` starting with #!/bin/sh. Pass "" when it is not
// known and extension matching still applies.
//
// Precedence is hand-written lexer, then bundled rules, then plain. nano's
// definitions sit outside this package and are consulted by the caller only
// when this returns the plain lexer, so a language nem describes itself is
// never shadowed by nano's version of it.
func ForWithHeader(path, firstLine string) Lexer {
	if lex := nativeFor(path); lex != nil {
		return lex
	}
	if lex := embeddedFor(path, firstLine); lex != nil {
		return lex
	}
	return PlainLexer{}
}
