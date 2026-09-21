package syntax

import "testing"

func TestJSONKeysAreDistinctFromValues(t *testing.T) {
	lx := jsonLexer{}
	assertSpanCovers(t, lx, `{"name": "value"}`, `"name"`, Function)
	assertSpanCovers(t, lx, `{"name": "value"}`, `"value"`, String)
	// Whitespace between the key and its colon is still a key.
	assertSpanCovers(t, lx, `{"name"  : 1}`, `"name"`, Function)
	// A string in an array is a value, not a key.
	assertSpanCovers(t, lx, `["a", "b"]`, `"a"`, String)
}

func TestJSONConstantsAndNumbers(t *testing.T) {
	lx := jsonLexer{}
	for _, tc := range []struct {
		src, sub string
		want     Class
	}{
		{`{"a": true}`, "true", Constant},
		{`{"a": false}`, "false", Constant},
		{`{"a": null}`, "null", Constant},
	} {
		assertSpanCovers(t, lx, tc.src, tc.sub, tc.want)
	}
	for _, lit := range []string{"0", "42", "-1", "3.14", "1e9", "-2.5E-3"} {
		assertSpanCovers(t, lx, `{"a": `+lit+`}`, lit, Number)
	}
}

func TestJSONPunctuation(t *testing.T) {
	line := []rune(`{"a":[1,2]}`)
	spans, out := jsonLexer{}.Lex(line, 0)
	checkSpans(t, "json punctuation", line, spans)
	if out != 0 {
		t.Errorf("JSON carried state %v; it should never need to", out)
	}
	if classAt(spans, 0) != Punctuation {
		t.Error("{ should be Punctuation")
	}
}

// An unterminated string must not carry state into the next line: JSON strings
// cannot contain a raw newline, so this is a typo, and ending it at the line
// break keeps one mistake from recolouring the rest of the document.
func TestJSONUnterminatedStringDoesNotCarry(t *testing.T) {
	_, out := jsonLexer{}.Lex([]rune(`{"a": "oops`), 0)
	if out != 0 {
		t.Errorf("state %v carried from an unterminated JSON string", out)
	}
}

// A bare word that is not a JSON constant is left Plain rather than guessed at.
func TestJSONUnknownBareWordIsPlain(t *testing.T) {
	assertClass(t, jsonLexer{}, `{"a": undefined}`, "undefined", Plain)
}
