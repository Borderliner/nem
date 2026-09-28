package syntax

import "testing"

// Each language here is one .syntax file in languages/. Its test checks the
// files it claims, the comment M-; writes with, a word of each class it
// lists, every construct that runs over lines, and each rule written as a
// match or a region, since those are where a definition goes wrong. A line
// left unfinished, as one being typed is, must carry nothing to the next
// unless the construct it opens is meant to.

func TestM4(t *testing.T) {
	for _, p := range []string{"configure.ac", "aclocal.m4", "configure.in", "CONFIGURE.AC"} {
		if got := For(p).Name(); got != "m4" {
			t.Errorf("For(%q) = %q, want m4", p, got)
		}
	}
	l := lang(t, "m4")
	if start, end, _ := l.Comment(); start != "dnl" || end != "" {
		t.Errorf("M-; comments m4 with %q %q, want dnl", start, end)
	}

	assertSpanCovers(t, l, "dnl a note", "dnl a note", Comment)
	assertSpanCovers(t, l, "AC_PROG_CC # a note", "# a note", Comment)
	assertSpanCovers(t, l, "x=${foo#bar} dnl trailing", "dnl trailing", Comment)
	// ${…} and $# are read before # can open a comment.
	assertSpanCovers(t, l, "x=${foo#bar} dnl trailing", "${foo#bar}", Constant)
	assertSpanCovers(t, l, "ifelse($#, 0, [none])", "$#", Constant)
	assertClass(t, l, "ifelse($#, 0, [none])", "ifelse", Keyword)
	assertClass(t, l, "ifelse($#, 0, [none])", "0", Number)

	// autoconf's macros are functions, called with arguments or without.
	assertClass(t, l, "AC_PROG_CC # a note", "AC_PROG_CC", Function)
	assertClass(t, l, "AC_INIT([nem], [1.0])", "AC_INIT", Function)
	assertClass(t, l, "m4_include([m4/ax.m4])", "m4_include", Function)
	assertClass(t, l, "x = eval(1 + 2)", "eval", Function)

	// A macro a definition names.
	assertClass(t, l, "define(`greet', `hi $1')", "define", Keyword)
	assertClass(t, l, "define(`greet', `hi $1')", "greet", Function)
	assertClass(t, l, "define(`greet', `hi $1')", "$1", Constant)
	assertClass(t, l, "AC_DEFUN([AX_FOO], [", "AX_FOO", Function)
	assertClass(t, l, "m4_define([my_macro], [$1])", "my_macro", Function)

	// Quoted text on one line is a string, and a # in it no comment; quoted
	// text that calls a macro is code.
	assertSpanCovers(t, l, "AC_INIT([nem], [1.0])", "[nem]", String)
	assertSpanCovers(t, l, "AC_LANG_PROGRAM([[#include <stdio.h>]])", "[#include <stdio.h>]", String)
	assertClass(t, l, "AS_IF([x], [AC_MSG_RESULT(yes)])", "AC_MSG_RESULT", Function)

	// configure.ac is shell between its macros.
	assertClass(t, l, `if test "$x" = yes; then`, "if", Keyword)
	assertClass(t, l, `if test "$x" = yes; then`, "then", Keyword)
	assertSpanCovers(t, l, `if test "$x" = yes; then`, `"$x"`, String)

	// Nothing in m4 runs over lines.
	for _, s := range []string{"[open", `"open`, "${", "define(`x", "AC_DEFUN([", "dnl", "#"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "m4 "+s, line, spans)
		if st != 0 {
			t.Errorf("m4 %q left %v open", s, st)
		}
	}
}

func TestGroff(t *testing.T) {
	for _, p := range []string{"ls.1", "printf.3", "init.8", "tmux.9", "page.man", "paper.ms", "doc.me", "doc.mom", "book.roff", "an.tmac"} {
		if got := For(p).Name(); got != "groff" {
			t.Errorf("For(%q) = %q, want groff", p, got)
		}
	}
	l := lang(t, "groff")
	if start, end, _ := l.Comment(); start != `.\"` || end != "" {
		t.Errorf(`M-; comments groff with %q %q, want .\"`, start, end)
	}

	// Requests and macro calls start their line.
	assertClass(t, l, `.TH LS 1 "2024-01-01"`, ".TH", Keyword)
	assertSpanCovers(t, l, `.TH LS 1 "2024-01-01"`, `"2024-01-01"`, String)
	assertSpanCovers(t, l, `.  B "a ""quoted"" arg"`, `"a ""quoted"" arg"`, String)
	assertSpanCovers(t, l, ".SH NAME", ".SH", Keyword)
	assertClass(t, l, ".SH NAME", "NAME", Plain)
	assertClass(t, l, "'br", "'br", Keyword)
	assertClass(t, l, ".de XX", ".de", Keyword)
	assertClass(t, l, ".de XX", "XX", Function)
	assertClass(t, l, `.\}`, ".", Keyword)
	assertClass(t, l, `.\}`, `\}`, Constant)

	// Comments: a whole line, the rest of one, and groff's \#.
	assertSpanCovers(t, l, `.\" Copyright`, `.\" Copyright`, Comment)
	assertSpanCovers(t, l, `'\" t`, `'\" t`, Comment)
	assertSpanCovers(t, l, `Text \" trailing`, `\" trailing`, Comment)
	assertSpanCovers(t, l, `Text \# groff's own`, `\# groff's own`, Comment)

	// Escapes are constants, and the prose around them is text.
	assertSpanCovers(t, l, `[\fIFILE\fR]`, `\fI`, Constant)
	assertSpanCovers(t, l, `[\fIFILE\fR]`, `\fR`, Constant)
	assertClass(t, l, `[\fIFILE\fR]`, "FILE", Plain)
	assertSpanCovers(t, l, `a \(em b`, `\(em`, Constant)
	assertSpanCovers(t, l, `a \[em] b`, `\[em]`, Constant)
	assertSpanCovers(t, l, `\*[Nm] b`, `\*[Nm]`, Constant)
	assertSpanCovers(t, l, `x \n(.l y`, `\n(.l`, Constant)
	assertSpanCovers(t, l, `\s-1LS\s+1`, `\s-1`, Constant)
	assertSpanCovers(t, l, `ls \- list`, `\-`, Constant)
	assertClass(t, l, `ls \- list 2 files`, "2", Plain)

	for _, s := range []string{`\`, `\f`, `\(e`, `\[em`, `\s'1`, `"open`, `.`, `'`} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "groff "+s, line, spans)
		if st != 0 {
			t.Errorf("groff %q left %v open", s, st)
		}
	}
}

func TestSpec(t *testing.T) {
	if got := For("nem.spec").Name(); got != "spec" {
		t.Errorf("For(nem.spec) = %q, want spec", got)
	}
	l := lang(t, "spec")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments spec with %q %q, want #", start, end)
	}

	// Tags, with Requires' scriptlet in parentheses; not a word in prose.
	assertClass(t, l, "Name:           nem", "Name", Function)
	assertClass(t, l, "Requires(post): systemd", "Requires", Function)
	assertClass(t, l, "BuildRequires:  golang >= 1.22", "BuildRequires", Function)
	assertSpanCovers(t, l, "BuildRequires:  golang >= 1.22", "1.22", Number)
	assertClass(t, l, "Source0: x.tar.gz", "Source0", Function)
	assertClass(t, l, "Name the thing", "Name", Plain)

	// Sections, conditionals and directives are keywords; any other %name is
	// a macro, however it starts.
	for _, s := range []string{"%prep", "%build", "%install", "%check", "%files", "%changelog", "%description", "%post", "%endif", "%autosetup", "%license"} {
		assertSpanCovers(t, l, s, s, Keyword)
	}
	assertClass(t, l, "%global debug_package %{nil}", "%global", Keyword)
	assertSpanCovers(t, l, "%global debug_package %{nil}", "%{nil}", Constant)
	assertSpanCovers(t, l, "%make_build", "%make_build", Constant)
	assertSpanCovers(t, l, "%filesystem", "%filesystem", Constant)
	assertSpanCovers(t, l, "Release: 1%{?dist}", "%{?dist}", Constant)
	assertSpanCovers(t, l, "Release: 1%{?dist}", "1", Number)
	// A macro inside a macro does not end it early.
	assertSpanCovers(t, l, "%{!?_licensedir:%global license %%doc} x", "%{!?_licensedir:%global license %%doc}", Constant)
	assertSpanCovers(t, l, "v=%(date +%%Y)", "%(date +%%Y)", Constant)

	// A # starts a comment only at the start of a line.
	assertSpanCovers(t, l, "# a note", "# a note", Comment)
	assertSpanCovers(t, l, "  # indented", "  # indented", Comment)
	assertClass(t, l, "Source0: %{url}/v.tar.gz#/nem.tar.gz", "#", Plain)
	assertClass(t, l, "Source0: %{url}/v.tar.gz#/nem.tar.gz", "nem", Plain)
	assertClass(t, l, "- Fix bug #42", "#", Plain)

	// Scriptlets are shell.
	assertSpanCovers(t, l, `echo "built" > $RPM_BUILD_ROOT/x`, `"built"`, String)
	assertSpanCovers(t, l, `echo "built" > $RPM_BUILD_ROOT/x`, "$RPM_BUILD_ROOT", Constant)

	for _, s := range []string{"%{", "%{?x", "%(", "%[", `"open`, "Requires(", "%"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "spec "+s, line, spans)
		if st != 0 {
			t.Errorf("spec %q left %v open", s, st)
		}
	}
}

func TestNftables(t *testing.T) {
	if got := For("firewall.nft").Name(); got != "nftables" {
		t.Errorf("For(firewall.nft) = %q, want nftables", got)
	}
	if got := ForWithHeader("firewall", "#!/usr/sbin/nft -f").Name(); got != "nftables" {
		t.Errorf("#!/usr/sbin/nft -f is %q, want nftables", got)
	}
	l := lang(t, "nftables")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments nftables with %q %q, want #", start, end)
	}

	assertClass(t, l, "table inet filter {", "table", Keyword)
	assertClass(t, l, "table inet filter {", "inet", Type)
	assertClass(t, l, "chain my_chain {", "my_chain", Function)
	assertClass(t, l, "meta mark set 0x1 jump to_lan", "to_lan", Function)
	assertSpanCovers(t, l, "meta mark set 0x1 jump to_lan", "0x1", Number)
	assertClass(t, l, "type filter hook input priority 0; policy drop;", "hook", Keyword)
	assertClass(t, l, "type filter hook input priority 0; policy drop;", "input", Constant)
	assertClass(t, l, "type filter hook input priority 0; policy drop;", "drop", Keyword)
	assertClass(t, l, "ct state established,related accept # keep", "established", Constant)
	assertClass(t, l, "ct state established,related accept # keep", "accept", Keyword)
	assertSpanCovers(t, l, "ct state established,related accept # keep", "# keep", Comment)
	assertSpanCovers(t, l, `iifname "lo" accept`, `"lo"`, String)
	assertSpanCovers(t, l, "ip saddr $lan tcp dport @ports accept", "$lan", Constant)
	assertSpanCovers(t, l, "ip saddr $lan tcp dport @ports accept", "@ports", Constant)

	// Addresses and ports are numbers, IPv6 and MAC ones too, though they may
	// start with a letter; a word of hex letters is still a word.
	assertSpanCovers(t, l, "ip saddr 192.168.1.0/24 drop", "192.168.1.0/24", Number)
	assertSpanCovers(t, l, "dnat to 10.0.0.2:8080", "10.0.0.2", Number)
	assertSpanCovers(t, l, "dnat to 10.0.0.2:8080", "8080", Number)
	assertSpanCovers(t, l, "ip6 saddr fe80::/10 accept", "fe80::/10", Number)
	assertSpanCovers(t, l, "ip6 daddr 2001:db8::1 drop", "2001:db8::1", Number)
	assertSpanCovers(t, l, "ether saddr aa:bb:cc:dd:ee:ff drop", "aa:bb:cc:dd:ee:ff", Number)
	assertSpanCovers(t, l, "tcp dport 1024-65535 accept", "1024", Number)
	assertSpanCovers(t, l, "tcp dport 1024-65535 accept", "65535", Number)
	assertSpanCovers(t, l, "ct timeout 30s", "30s", Number)
	assertClass(t, l, "add dead beef", "dead", Plain)

	for _, s := range []string{`"open`, "$", "@", "fe80:", "::", "10.0.0."} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "nftables "+s, line, spans)
		if st != 0 {
			t.Errorf("nftables %q left %v open", s, st)
		}
	}
}

func TestTexinfo(t *testing.T) {
	for _, p := range []string{"nem.texi", "nem.texinfo", "nem.txi"} {
		if got := For(p).Name(); got != "texinfo" {
			t.Errorf("For(%q) = %q, want texinfo", p, got)
		}
	}
	l := lang(t, "texinfo")
	if start, end, _ := l.Comment(); start != "@c" || end != "" {
		t.Errorf("M-; comments texinfo with %q %q, want @c", start, end)
	}

	assertSpanCovers(t, l, "@c a note", "@c a note", Comment)
	assertSpanCovers(t, l, "@comment a note", "@comment a note", Comment)
	assertSpanCovers(t, l, "@c", "@c", Comment)
	// A command that starts with @c is not a comment.
	assertSpanCovers(t, l, "@cindex editor", "@cindex", Keyword)
	assertClass(t, l, "@cindex editor", "editor", Plain)
	assertSpanCovers(t, l, "@center A line", "@center", Keyword)
	assertSpanCovers(t, l, "This is @code{nem} here", "@code", Keyword)
	assertSpanCovers(t, l, "This is @code{nem} here", "nem", String)
	assertSpanCovers(t, l, "an @emph{editor}", "@emph", Keyword)
	assertClass(t, l, "an @emph{editor}", "{", Punctuation)
	assertClass(t, l, "an @emph{editor}", "editor", Plain)
	assertClass(t, l, "@i{italic}", "@i", Keyword)
	assertSpanCovers(t, l, "Mail a@@b.org", "@@", Constant)
	assertSpanCovers(t, l, "@end example", "@end example", Keyword)
	assertClass(t, l, `\input texinfo`, `\input`, Keyword)

	// A node's name, and a heading's whole line.
	assertClass(t, l, "@node Top, Next, Prev, Up", "@node", Keyword)
	assertSpanCovers(t, l, "@node Top, Next, Prev, Up", "Top", Function)
	assertSpanCovers(t, l, "@chapter Getting Started", "@chapter Getting Started", Keyword)
	assertClass(t, l, "See 3 pages", "3", Plain)

	// @ignore hides every line to its @end ignore, @-commands and all.
	_, spans := lexDoc(l, "@ignore\n@chapter Hidden\nnone of @code{this}\n@end ignore\nAfter @var{x}\n")
	for i := 0; i <= 3; i++ {
		if classAt(spans[i], 0) != Comment {
			t.Errorf("texinfo line %d should be inside @ignore", i)
		}
	}
	if classAt(spans[4], 0) == Comment || classAt(spans[4], 6) != Keyword {
		t.Errorf("@end ignore did not end the block: %v", spans[4])
	}

	for _, s := range []string{"@", "@code{", "@code{x", "@node", "@end", "@c{"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "texinfo "+s, line, spans)
		if st != 0 {
			t.Errorf("texinfo %q left %v open", s, st)
		}
	}
}

func TestPovray(t *testing.T) {
	if got := For("scene.pov").Name(); got != "povray" {
		t.Errorf("For(scene.pov) = %q, want povray", got)
	}
	l := lang(t, "povray")
	if start, end, _ := l.Comment(); start != "//" || end != "" {
		t.Errorf("M-; comments povray with %q %q, want //", start, end)
	}

	assertSpanCovers(t, l, "#declare Radius = 1.5;", "#declare", Keyword)
	assertClass(t, l, "#declare Radius = 1.5;", "Radius", Function)
	assertSpanCovers(t, l, "#declare Radius = 1.5;", "1.5", Number)
	assertSpanCovers(t, l, `#include "colors.inc"`, `"colors.inc"`, String)
	assertClass(t, l, "#macro Ball(Pos, R)", "Ball", Function)
	assertClass(t, l, "sphere { 0, 1 pigment { color rgb <1, 0.5, -2> } }", "sphere", Keyword)
	assertClass(t, l, "sphere { 0, 1 pigment { color rgb <1, 0.5, -2> } }", "pigment", Keyword)
	assertClass(t, l, "#if (clock > 0.5)", "clock", Constant)
	assertClass(t, l, "#local V = vnormalize(<1,1,1>);", "vnormalize", Function)
	assertSpanCovers(t, l, "x = 2; // a note", "// a note", Comment)

	// A vector of numbers is one value; one with names in it is read as
	// what it holds.
	assertSpanCovers(t, l, "sphere { 0, 1 pigment { color rgb <1, 0.5, -2> } }", "<1, 0.5, -2>", Number)
	assertSpanCovers(t, l, "#local V = vnormalize(<1,1,1>);", "<1,1,1>", Number)
	assertClass(t, l, "translate <x, 1, 0>", "<", Operator)
	assertClass(t, l, "translate <x, 1, 0>", "x", Constant)
	assertClass(t, l, "#if (a < 2)", "<", Operator)

	// A block comment nests, and runs over lines.
	_, spans := lexDoc(l, "/* a /* b */ still\nmore */ sphere { }\n")
	if classAt(spans[0], 14) != Comment || classAt(spans[1], 0) != Comment {
		t.Error("povray: the inner */ ended the nested comment")
	}
	if classAt(spans[1], 8) != Keyword {
		t.Errorf("povray: the outer */ did not end the comment: %v", spans[1])
	}

	for _, s := range []string{`"open`, "<1, 2", "#", "/"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "povray "+s, line, spans)
		if st != 0 {
			t.Errorf("povray %q left %v open", s, st)
		}
	}
}

func TestGLSL(t *testing.T) {
	for _, p := range []string{"shader.glsl", "a.vert", "a.frag", "a.geom", "a.tesc", "a.tese", "a.comp"} {
		if got := For(p).Name(); got != "glsl" {
			t.Errorf("For(%q) = %q, want glsl", p, got)
		}
	}
	l := lang(t, "glsl")
	if start, end, _ := l.Comment(); start != "//" || end != "" {
		t.Errorf("M-; comments glsl with %q %q, want //", start, end)
	}

	// C's preprocessor rule takes GLSL's directives.
	assertClass(t, l, "#version 450 core", "#version", Keyword)
	assertSpanCovers(t, l, "#version 450 core", "450", Number)
	assertClass(t, l, "#version 450 core", "core", Constant)
	assertClass(t, l, "#extension GL_ARB_foo : enable", "#extension", Keyword)
	assertClass(t, l, "#extension GL_ARB_foo : enable", "enable", Constant)

	assertClass(t, l, "layout(location = 0) in vec3 pos;", "layout", Keyword)
	assertClass(t, l, "layout(location = 0) in vec3 pos;", "in vec3", Keyword)
	assertClass(t, l, "layout(location = 0) in vec3 pos;", "vec3", Type)
	assertClass(t, l, "uniform sampler2D tex;", "uniform", Keyword)
	assertClass(t, l, "uniform sampler2D tex;", "sampler2D", Type)
	assertClass(t, l, "precision highp float;", "highp", Keyword)
	assertClass(t, l, "precision highp float;", "float", Type)
	assertClass(t, l, "if (c.a < 0.1) discard;", "discard", Keyword)
	assertClass(t, l, "struct Light { vec3 pos; };", "Light", Type)
	assertClass(t, l, "uint n = 3u;", "uint", Type)
	assertSpanCovers(t, l, "uint n = 3u;", "3u", Number)

	// The pipeline's variables and GLSL's built-in functions.
	assertSpanCovers(t, l, "gl_Position = vec4(p, 1.0);", "gl_Position", Constant)
	assertSpanCovers(t, l, "gl_Position = vec4(p, 1.0);", "1.0", Number)
	assertClass(t, l, "vec4 c = texture(tex, uv);", "texture", Function)
	assertClass(t, l, "float m = mix(a, b, 0.5);", "mix", Function)
	assertSpanCovers(t, l, "x = 1.0; // blend", "// blend", Comment)

	_, spans := lexDoc(l, "/* lighting\n   params */\nuniform mat4 m;\n")
	if classAt(spans[1], 0) != Comment || classAt(spans[2], 0) != Keyword {
		t.Errorf("glsl block comment: %v / %v", spans[1], spans[2])
	}
}

func TestVue(t *testing.T) {
	if got := For("App.vue").Name(); got != "vue" {
		t.Errorf("For(App.vue) = %q, want vue", got)
	}
	l := lang(t, "vue")
	if start, end, _ := l.Comment(); start != "<!--" || end != "-->" {
		t.Errorf("M-; comments vue with %q %q, want <!-- -->", start, end)
	}

	// HTML's own rules still hold.
	assertClass(t, l, `<div id="app">`, "<div", Keyword)
	assertClass(t, l, `<div id="app">`, "id", Type)
	assertSpanCovers(t, l, `<div id="app">`, `"app"`, String)
	assertSpanCovers(t, l, "Tom &amp; Jerry", "&amp;", Constant)
	assertSpanCovers(t, l, "<!-- note -->", "<!-- note -->", Comment)

	// Directives, by name or by :, @ and #.
	assertSpanCovers(t, l, `<div :class="cls" @click.prevent="go">`, ":class", Keyword)
	assertSpanCovers(t, l, `<div :class="cls" @click.prevent="go">`, `"cls"`, String)
	assertSpanCovers(t, l, `<div :class="cls" @click.prevent="go">`, "@click.prevent", Keyword)
	assertSpanCovers(t, l, `<p v-if="n > 0">`, "v-if", Keyword)
	assertSpanCovers(t, l, `<p v-if="n > 0">`, `"n > 0"`, String)
	assertSpanCovers(t, l, `<MyComp v-on:change="f" />`, "v-on:change", Keyword)
	assertClass(t, l, "<p v-else>None</p>", "v-else", Keyword)
	assertClass(t, l, "<template #header>", "#header", Keyword)
	assertSpanCovers(t, l, `<template #item="{ item }">`, "#item", Keyword)
	// A style block's #id is not a slot.
	assertClass(t, l, "#app { color: red; }", "#app", Plain)

	assertSpanCovers(t, l, "Count: {{ count }} left", "{{ count }}", Constant)
	assertClass(t, l, "Count: {{ count }} left", "left", Plain)

	_, spans := lexDoc(l, "<!-- a\nb -->\n<p v-if=\"x\">\n")
	if classAt(spans[1], 0) != Comment || classAt(spans[2], 0) != Keyword || classAt(spans[2], 3) != Keyword {
		t.Errorf("vue comment over lines: %v / %v", spans[1], spans[2])
	}

	for _, s := range []string{"{{", "{{ open", `v-if="open`, "#", ":", "@"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "vue "+s, line, spans)
		if st != 0 {
			t.Errorf("vue %q left %v open", s, st)
		}
	}
}

func TestSvelte(t *testing.T) {
	if got := For("App.svelte").Name(); got != "svelte" {
		t.Errorf("For(App.svelte) = %q, want svelte", got)
	}
	l := lang(t, "svelte")
	if start, end, _ := l.Comment(); start != "<!--" || end != "-->" {
		t.Errorf("M-; comments svelte with %q %q, want <!-- -->", start, end)
	}

	// Blocks and tags: the keyword and its brace, and what it is given.
	assertSpanCovers(t, l, "{#if count > 5}", "{#if", Keyword)
	assertClass(t, l, "{#if count > 5}", "count", Constant)
	assertSpanCovers(t, l, "{#if count > 5}", "}", Keyword)
	assertSpanCovers(t, l, "{:else if n}", "{:else if", Keyword)
	assertSpanCovers(t, l, "{:else}", "{:else}", Keyword)
	assertSpanCovers(t, l, "{/if}", "{/if}", Keyword)
	assertSpanCovers(t, l, "{#each items as item, i (item.id)}", "{#each", Keyword)
	assertSpanCovers(t, l, "{@html raw}", "{@html", Keyword)

	// Directives, and attributes given an expression.
	assertSpanCovers(t, l, "<button on:click={inc}>", "on:click", Keyword)
	assertSpanCovers(t, l, "<button on:click={inc}>", "{inc}", Constant)
	assertSpanCovers(t, l, "<input bind:value={name} />", "bind:value", Keyword)
	assertClass(t, l, "<li class:active>", "class:active", Keyword)
	assertSpanCovers(t, l, `<li transition:fade="x">`, `"x"`, String)
	assertClass(t, l, "<button disabled={busy}>", "disabled", Type)
	assertSpanCovers(t, l, "<button disabled={busy}>", "{busy}", Constant)
	assertSpanCovers(t, l, `<p class="big">`, `"big"`, String)

	// An expression in the text; a script's or style's braces are not one.
	assertSpanCovers(t, l, "Clicked {count} times", "{count}", Constant)
	assertClass(t, l, "  button { color: red; }", "color", Plain)
	assertSpanCovers(t, l, "<!-- note -->", "<!-- note -->", Comment)

	_, spans := lexDoc(l, "<!-- a\nb -->\n{#if x}\n")
	if classAt(spans[1], 0) != Comment || classAt(spans[2], 0) != Keyword {
		t.Errorf("svelte comment over lines: %v / %v", spans[1], spans[2])
	}

	for _, s := range []string{"{", "{#if", "{#if x", "on:click={", "{/", "{@"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "svelte "+s, line, spans)
		if st != 0 {
			t.Errorf("svelte %q left %v open", s, st)
		}
	}
}

func TestStarlark(t *testing.T) {
	for _, p := range []string{"defs.bzl", "rules.star", "BUILD.bazel", "WORKSPACE.bazel", "MODULE.bazel", "extra.bazel", "/repo/pkg/BUILD.bazel"} {
		if got := For(p).Name(); got != "starlark" {
			t.Errorf("For(%q) = %q, want starlark", p, got)
		}
	}
	// A bare BUILD or WORKSPACE is not claimed: a script called build is
	// shell, and says so.
	for _, p := range []string{"BUILD", "WORKSPACE", "build"} {
		if got := For(p).Name(); got == "starlark" {
			t.Errorf("For(%q) = starlark; it must not be claimed", p)
		}
	}
	if got := ForWithHeader("build", "#!/bin/sh").Name(); got != "sh" {
		t.Errorf("a file named build with #!/bin/sh is %q, want sh", got)
	}
	l := lang(t, "starlark")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments starlark with %q %q, want #", start, end)
	}

	assertClass(t, l, `load("@rules_cc//cc:defs.bzl", "cc_library")`, "load", Function)
	assertSpanCovers(t, l, `load("@rules_cc//cc:defs.bzl", "cc_library")`, `"@rules_cc//cc:defs.bzl"`, String)
	assertClass(t, l, `srcs = glob(["*.cc"]),`, "glob", Function)
	assertClass(t, l, "deps = select({", "select", Function)
	assertClass(t, l, "cc_library(", "cc_library", Function)
	assertClass(t, l, "exports_files", "exports_files", Function)
	assertClass(t, l, "def _impl(ctx):", "def", Keyword)
	assertClass(t, l, "def _impl(ctx):", "_impl", Function)
	assertClass(t, l, "if x == None:", "None", Constant)
	assertSpanCovers(t, l, "depth = 3  # levels", "3", Number)
	assertSpanCovers(t, l, "depth = 3  # levels", "# levels", Comment)

	_, spans := lexDoc(l, "def f():\n    \"\"\"Doc\n    more\n    \"\"\"\n    return 1\n")
	for i := 1; i <= 3; i++ {
		if classAt(spans[i], 4) != String {
			t.Errorf("starlark line %d should be inside the docstring", i)
		}
	}
	if classAt(spans[4], 4) != Keyword {
		t.Errorf("starlark: the docstring did not end: %v", spans[4])
	}
}

func TestCoffeeScript(t *testing.T) {
	for _, p := range []string{"app.coffee", "package.cson"} {
		if got := For(p).Name(); got != "coffeescript" {
			t.Errorf("For(%q) = %q, want coffeescript", p, got)
		}
	}
	if got := ForWithHeader("app", "#!/usr/bin/env coffee").Name(); got != "coffeescript" {
		t.Errorf("#!/usr/bin/env coffee is %q, want coffeescript", got)
	}
	l := lang(t, "coffeescript")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments coffeescript with %q %q, want #", start, end)
	}

	assertSpanCovers(t, l, "x = 1 # a note", "# a note", Comment)
	assertClass(t, l, "class Animal extends Base", "class", Keyword)
	assertClass(t, l, "class Animal extends Base", "Animal", Type)
	assertClass(t, l, "class Animal extends Base", "Base", Type)
	assertSpanCovers(t, l, "alert @name unless m is 0", "@name", Constant)
	assertClass(t, l, "alert @name unless m is 0", "unless", Keyword)
	assertClass(t, l, "alert @name unless m is 0", "is", Keyword)
	assertClass(t, l, "ok = yes and not off", "yes", Constant)
	assertClass(t, l, "ok = yes and not off", "off", Constant)
	assertClass(t, l, "ok = yes and not off", "and", Keyword)
	assertSpanCovers(t, l, "x = 0x1F if y isnt null", "0x1F", Number)
	assertClass(t, l, "x = 0x1F if y isnt null", "isnt", Keyword)
	assertClass(t, l, "x = 0x1F if y isnt null", "null", Constant)
	assertSpanCovers(t, l, "s = 'hi'", "'hi'", String)
	assertSpanCovers(t, l, `s = "a #{b} c"`, `"a #{b} c"`, String)

	// A function is an arrow, and the name given one is the function's.
	assertClass(t, l, "square = (x) -> x * x", "square", Function)
	assertSpanCovers(t, l, "square = (x) -> x * x", "->", Keyword)
	assertClass(t, l, "  move: (m = 5) =>", "move", Function)
	assertSpanCovers(t, l, "  move: (m = 5) =>", "=>", Keyword)
	assertClass(t, l, "greet = -> hi()", "greet", Function)

	// ### opens a block comment; a rule of more #s is a line comment.
	_, spans := lexDoc(l, "###\ninside\n###\nx = yes\n")
	if classAt(spans[1], 0) != Comment || classAt(spans[3], 4) != Constant {
		t.Errorf("coffeescript ### block: %v / %v", spans[1], spans[3])
	}
	_, spans = lexDoc(l, "########## rule ##########\nx = 1\n")
	if classAt(spans[0], 0) != Comment || classAt(spans[1], 4) != Number {
		t.Errorf("coffeescript: a rule of #s opened a block comment: %v", spans[1])
	}

	// Triple-quoted strings and block regular expressions run over lines.
	for _, doc := range []string{"s = \"\"\"\n  <p>#{name}</p>\n\"\"\"\ny = 2\n", "s = '''\n  text\n'''\ny = 2\n", "re = ///\n  ^ \\d+ # digits\n///\ny = 2\n"} {
		_, spans := lexDoc(l, doc)
		if classAt(spans[1], 2) != String || classAt(spans[2], 0) != String || classAt(spans[3], 4) != Number {
			t.Errorf("coffeescript %q: %v", doc, spans)
		}
	}

	for _, s := range []string{`"open`, "'open", "@", "->", "#"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "coffeescript "+s, line, spans)
		if st != 0 {
			t.Errorf("coffeescript %q left %v open", s, st)
		}
	}
}

func TestSolidity(t *testing.T) {
	if got := For("Token.sol").Name(); got != "solidity" {
		t.Errorf("For(Token.sol) = %q, want solidity", got)
	}
	l := lang(t, "solidity")
	if start, end, _ := l.Comment(); start != "//" || end != "" {
		t.Errorf("M-; comments solidity with %q %q, want //", start, end)
	}

	assertSpanCovers(t, l, "// SPDX-License-Identifier: MIT", "// SPDX-License-Identifier: MIT", Comment)
	assertSpanCovers(t, l, "/// @notice Holds balances.", "/// @notice Holds balances.", Comment)
	assertSpanCovers(t, l, "x = 1; /** @dev y */ z", "/** @dev y */", Comment)
	assertClass(t, l, "pragma solidity ^0.8.20;", "pragma", Keyword)
	assertClass(t, l, "contract Token is ERC20 {", "contract", Keyword)
	assertClass(t, l, "contract Token is ERC20 {", "Token", Type)
	assertClass(t, l, "contract Token is ERC20 {", "is", Keyword)
	assertClass(t, l, "mapping(address => uint256) public balances;", "mapping", Type)
	assertClass(t, l, "mapping(address => uint256) public balances;", "address", Type)
	assertClass(t, l, "mapping(address => uint256) public balances;", "uint256", Type)
	assertClass(t, l, "mapping(address => uint256) public balances;", "public", Keyword)
	assertClass(t, l, `bytes32 constant ROLE = keccak256("ROLE");`, "bytes32", Type)
	assertClass(t, l, `bytes32 constant ROLE = keccak256("ROLE");`, "keccak256", Function)
	assertSpanCovers(t, l, `bytes32 constant ROLE = keccak256("ROLE");`, `"ROLE"`, String)
	assertClass(t, l, "event Transfer(address indexed from, uint256 value);", "Transfer", Function)
	assertClass(t, l, "event Transfer(address indexed from, uint256 value);", "from", Plain)
	assertClass(t, l, "function send(address to) external payable returns (bool) {", "send", Function)
	assertClass(t, l, "emit Transfer(a, b, n);", "emit", Keyword)
	assertClass(t, l, "if (x) revert Insufficient(n);", "revert", Keyword)
	assertClass(t, l, "return true;", "true", Constant)

	// Units and the globals every contract sees.
	assertClass(t, l, `require(msg.value >= 1 ether, "x");`, "msg", Constant)
	assertClass(t, l, `require(msg.value >= 1 ether, "x");`, "ether", Constant)
	assertSpanCovers(t, l, `require(msg.value >= 1 ether, "x");`, "1", Number)
	assertClass(t, l, "uint t = block.timestamp + 1 days;", "days", Constant)
	assertSpanCovers(t, l, "uint amount = 1_000e18;", "1_000e18", Number)

	// A string may be hex or unicode.
	assertSpanCovers(t, l, `bytes memory h = hex"00ff";`, `hex"00ff"`, String)
	assertSpanCovers(t, l, `string memory s = unicode"héllo";`, `unicode"héllo"`, String)

	// Yul, inside assembly.
	assertClass(t, l, "assembly { let x := mload(0x40) }", "let", Keyword)
	assertSpanCovers(t, l, "assembly { let x := mload(0x40) }", "0x40", Number)

	_, spans := lexDoc(l, "/* storage\n   layout */\nuint x;\n")
	if classAt(spans[1], 0) != Comment || classAt(spans[2], 0) != Type {
		t.Errorf("solidity block comment: %v / %v", spans[1], spans[2])
	}

	for _, s := range []string{`"open`, `hex"00`, "'open", "unicode"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "solidity "+s, line, spans)
		if st != 0 {
			t.Errorf("solidity %q left %v open", s, st)
		}
	}
}

func TestCrystal(t *testing.T) {
	if got := For("app.cr").Name(); got != "crystal" {
		t.Errorf("For(app.cr) = %q, want crystal", got)
	}
	if got := ForWithHeader("app", "#!/usr/bin/env crystal").Name(); got != "crystal" {
		t.Errorf("#!/usr/bin/env crystal is %q, want crystal", got)
	}
	l := lang(t, "crystal")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments crystal with %q %q, want #", start, end)
	}

	assertSpanCovers(t, l, "x = 1 # a note", "# a note", Comment)
	assertClass(t, l, `require "json"`, "require", Keyword)
	assertSpanCovers(t, l, `require "json"`, `"json"`, String)
	assertClass(t, l, "class Buffer(T) < Base", "Buffer", Type)
	assertClass(t, l, "macro define_getter(name)", "define_getter", Function)
	assertSpanCovers(t, l, "c = 'x'", "'x'", String)
	assertSpanCovers(t, l, "puts 1.5f32", "1.5f32", Number)
	assertClass(t, l, "puts 1.5f32", "puts", Function)

	// A method's name may end in ? or !.
	assertSpanCovers(t, l, "def empty? : Bool", "empty?", Function)
	assertClass(t, l, "def empty? : Bool", "Bool", Type)
	assertSpanCovers(t, l, "@lines.empty? && !@name.nil?", "nil?", Keyword)

	// Sigils: instance and class variables, symbols, annotations.
	assertSpanCovers(t, l, "@lines.empty? && !@name.nil?", "@lines", Constant)
	assertSpanCovers(t, l, "@@count = 0_i64", "@@count", Constant)
	assertSpanCovers(t, l, "@@count = 0_i64", "0_i64", Number)
	assertSpanCovers(t, l, "sym = :done", ":done", Constant)
	assertSpanCovers(t, l, `@[JSON::Field(key: "n")]`, "@[JSON::Field", Function)
	// A path's :: and a type restriction's : are not symbols.
	assertSpanCovers(t, l, "module Nem::Util", "::", Operator)
	assertClass(t, l, "module Nem::Util", "Util", Plain)
	assertClass(t, l, "def initialize(@name : String)", "String", Type)

	assertSpanCovers(t, l, "words = %w(apple banana)", "%w(apple banana)", String)
	assertSpanCovers(t, l, "x = %i[a b]", "%i[a b]", String)

	// Macros: {% %} around code, and {{ }} closed only by its own }}.
	assertSpanCovers(t, l, "{% for k in [:a, :b] %}", "{%", Keyword)
	assertClass(t, l, "{% for k in [:a, :b] %}", "for", Keyword)
	assertSpanCovers(t, l, "{% for k in [:a, :b] %}", ":a", Constant)
	assertSpanCovers(t, l, "{% for k in [:a, :b] %}", "%}", Keyword)
	assertSpanCovers(t, l, "def {{name.id}}", "{{", Keyword)
	assertSpanCovers(t, l, "def {{name.id}}", "}}", Keyword)
	assertClass(t, l, "h = {a: {b: 1}}", "}}", Punctuation)

	// A heredoc runs to a line holding only its word.
	_, spans := lexDoc(l, "text = <<-EOS\n  Hello #{name}\n  EOS is not the end\n  EOS\nx = 1\n")
	for i := 1; i <= 3; i++ {
		if classAt(spans[i], 2) != String {
			t.Errorf("crystal line %d should be inside the heredoc", i)
		}
	}
	if classAt(spans[4], 4) != Number {
		t.Errorf("crystal: the heredoc did not end at its word: %v", spans[4])
	}

	for _, s := range []string{`"open`, "'", "%w(open", "{{ open", "{%", ":", "@", "@["} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "crystal "+s, line, spans)
		if st != 0 {
			t.Errorf("crystal %q left %v open", s, st)
		}
	}
}

func TestNinja(t *testing.T) {
	for _, p := range []string{"build.ninja", "rules.ninja"} {
		if got := For(p).Name(); got != "ninja" {
			t.Errorf("For(%q) = %q, want ninja", p, got)
		}
	}
	l := lang(t, "ninja")
	if start, end, _ := l.Comment(); start != "#" || end != "" {
		t.Errorf("M-; comments ninja with %q %q, want #", start, end)
	}

	assertSpanCovers(t, l, "# a note", "# a note", Comment)
	assertSpanCovers(t, l, "  # indented", "  # indented", Comment)
	// In a value, # is text.
	assertClass(t, l, "  command = gcc -c $in -o $out # not a note", "#", Plain)
	assertClass(t, l, "  command = gcc -c $in -o $out # not a note", "note", Plain)

	assertClass(t, l, "rule cc", "rule", Keyword)
	assertClass(t, l, "rule cc", "cc", Function)
	assertClass(t, l, "build main.o: cc main.c", "build", Keyword)
	assertSpanCovers(t, l, "build $builddir/main.o: cc src/main.c | gen.h", "$builddir", Constant)
	assertSpanCovers(t, l, "build $builddir/main.o: cc src/main.c | gen.h", "cc", Function)
	assertClass(t, l, "build all: phony x", "phony", Function)
	assertClass(t, l, "default all", "default", Keyword)
	assertClass(t, l, "include rules.ninja", "include", Keyword)
	// A keyword only starts its line: build/ elsewhere is a directory.
	assertClass(t, l, "subninja sub/build.ninja", "subninja", Keyword)
	assertClass(t, l, "subninja sub/build.ninja", "build", Plain)

	// Bindings, variables and escapes.
	assertSpanCovers(t, l, "  description = CC ${out}", "description", Function)
	assertSpanCovers(t, l, "  description = CC ${out}", "${out}", Constant)
	assertSpanCovers(t, l, "cflags = -O2", "cflags", Function)
	assertSpanCovers(t, l, "build C$:/x.o: cc x.c", "$:", Constant)
	assertSpanCovers(t, l, "cflags = -O2 $", "$", Constant)
	assertClass(t, l, "  pool = console", "console", Constant)
	assertClass(t, l, "  depth = 4", "4", Plain)

	for _, s := range []string{"$", "${", "${out", "rule", "build x:", "#"} {
		line := []rune(s)
		spans, st := l.Lex(line, 0)
		checkSpans(t, "ninja "+s, line, spans)
		if st != 0 {
			t.Errorf("ninja %q left %v open", s, st)
		}
	}
}
