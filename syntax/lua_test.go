package syntax

import "testing"

func TestLuaKeywordsConstantsAndCalls(t *testing.T) {
	lx := luaLexer{}
	for _, tc := range []struct {
		src, sub string
		want     Class
	}{
		{"local x = 1", "local", Keyword},
		{"function f(a)", "function", Keyword},
		{"for i = 1, 10 do", "for", Keyword},
		{"if x then end", "then", Keyword},
		{"x = nil", "nil", Constant},
		{"x = true", "true", Constant},
		{"formatting = 1", "formatting", Plain}, // not "for"
		{"endless = 2", "endless", Plain},       // not "end"
		{"function myfn(a)", "myfn", Function},
		{"print(x)", "print", Function},
		{"function obj:method(a)", "method", Function},
		{"x = y", "y", Plain},
	} {
		assertClass(t, lx, tc.src, tc.sub, tc.want)
	}
}

func TestLuaLineComment(t *testing.T) {
	lx := luaLexer{}
	assertSpanCovers(t, lx, "x = 1 -- a note", "-- a note", Comment)
	// A -- inside a string is not a comment.
	assertSpanCovers(t, lx, `s = "a -- b"`, `"a -- b"`, String)
}

func TestLuaQuotedStrings(t *testing.T) {
	lx := luaLexer{}
	assertSpanCovers(t, lx, `s = "hello"`, `"hello"`, String)
	assertSpanCovers(t, lx, `s = 'hello'`, `'hello'`, String)
	assertSpanCovers(t, lx, `s = "a\"b"`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `s = 'it\'s'`, `'it\'s'`, String)
}

func TestLuaLongCommentSpansLines(t *testing.T) {
	_, spans := lexDoc(luaLexer{}, "--[[ open\nstill\nclosed ]] x = 1\n")
	for i := 0; i < 3; i++ {
		if classAt(spans[i], 0) != Comment {
			t.Errorf("line %d should be inside the long comment", i)
		}
	}
	if classAt(spans[2], 11) == Comment {
		t.Error("code after ]] is not part of the comment")
	}
}

func TestLuaLongStringSpansLines(t *testing.T) {
	_, spans := lexDoc(luaLexer{}, "s = [[ open\nstill\nclosed ]]\n")
	if classAt(spans[0], 4) != String {
		t.Error("[[ should open a long string")
	}
	if classAt(spans[1], 0) != String {
		t.Error("line 1 is inside the long string")
	}
	if classAt(spans[2], 0) != String {
		t.Error("line 2 up to ]] is still the string")
	}
}

// The level must match. This is the case a lexer that merely looks for "]]"
// gets wrong, and it silently recolours the rest of the file.
func TestLuaLongBracketLevelsMustMatch(t *testing.T) {
	_, spans := lexDoc(luaLexer{}, "s = [==[ open\ncontains ]] which does not close it\nreal close ]==]\nx = 1\n")
	if classAt(spans[1], 10) != String {
		t.Error("]] must not close a [==[ string")
	}
	if classAt(spans[2], 0) != String {
		t.Error("line 2 up to ]==] is still the string")
	}
	if classAt(spans[3], 0) == String {
		t.Error("after ]==] the string is closed")
	}

	// And the reverse: a longer closer does not close a shorter opener early,
	// because ]==] contains no ]] that matches level 0 exactly.
	_, spans2 := lexDoc(luaLexer{}, "s = [[ open\nnot ]=] either\nclose ]]\n")
	if classAt(spans2[1], 4) != String {
		t.Error("]=] must not close a [[ string")
	}
	if classAt(spans2[2], 0) != String {
		t.Error("line 2 up to ]] is still the string")
	}
}

func TestLuaLongCommentWithLevel(t *testing.T) {
	_, spans := lexDoc(luaLexer{}, "--[==[ open\nstill\n]==]\nx = 1\n")
	if classAt(spans[1], 0) != Comment {
		t.Error("line 1 is inside the level-2 long comment")
	}
	if classAt(spans[3], 0) == Comment {
		t.Error("after ]==] the comment is closed")
	}
}

func TestLuaNumbers(t *testing.T) {
	lx := luaLexer{}
	for _, lit := range []string{"42", "3.14", "0xff", "1e9", "1E-3", ".5"} {
		assertSpanCovers(t, lx, "x = "+lit+" + y", lit, Number)
	}
}

// A lone [ is a table index, not a long bracket.
func TestLuaSingleBracketIsPunctuation(t *testing.T) {
	assertClass(t, luaLexer{}, "x = t[1]", "[", Punctuation)
}
