package syntax

import "testing"

// Build files, configuration and data formats: one test a language, each
// covering the files it claims, how M-; comments in it, what it colours on a
// line, and every construct that runs over lines.

// cfgDetect asserts that each path is detected as want, by its name alone.
func cfgDetect(t *testing.T, want string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if got := For(p).Name(); got != want {
			t.Errorf("For(%q) = %q, want %q", p, got, want)
		}
	}
}

// cfgComment asserts the comment M-; writes: start, and end for a block
// comment.
func cfgComment(t *testing.T, l *Language, start, end string) {
	t.Helper()
	s, e, ok := l.Comment()
	if !ok || s != start || e != end {
		t.Errorf("%s comments with %q %q %v, want %q %q", l.Name(), s, e, ok, start, end)
	}
}

// cfgCase is one expectation: sub, within src, is of class want.
type cfgCase struct {
	src, sub string
	want     Class
}

// cfgClasses checks where each case's sub starts, as assertClass does.
func cfgClasses(t *testing.T, lx Lexer, cases []cfgCase) {
	t.Helper()
	for _, c := range cases {
		assertClass(t, lx, c.src, c.sub, c.want)
	}
}

// cfgSpans checks that a span of each case's class covers exactly its sub.
func cfgSpans(t *testing.T, lx Lexer, cases []cfgCase) {
	t.Helper()
	for _, c := range cases {
		assertSpanCovers(t, lx, c.src, c.sub, c.want)
	}
}

// cfgAt is an expectation about one line of a document: every rune of sub,
// on line number line, is of class want.
type cfgAt struct {
	line int
	sub  string
	want Class
}

// cfgDoc lexes doc from its first line, carrying state, and checks each
// expectation: how a construct that runs over lines is tested.
func cfgDoc(t *testing.T, lx Lexer, doc string, checks ...cfgAt) {
	t.Helper()
	lines, spans := lexDoc(lx, doc)
	for i, l := range lines {
		checkSpans(t, string(l), l, spans[i])
	}
	for _, c := range checks {
		if c.line >= len(lines) {
			t.Fatalf("no line %d in %q", c.line, doc)
		}
		src := string(lines[c.line])
		at := len([]rune(src[:indexOnce(t, src, c.sub)]))
		for k := range []rune(c.sub) {
			if got := classAt(spans[c.line], at+k); got != c.want {
				t.Errorf("line %d %q: %q is %v at rune %d, want %v (spans: %s)",
					c.line, src, c.sub, got, k, c.want, describe(lines[c.line], spans[c.line]))
				break
			}
		}
	}
}

// cfgOpen asserts whether line, lexed from nothing open, leaves something
// open for the next.
func cfgOpen(t *testing.T, lx Lexer, line string, open bool) {
	t.Helper()
	r := []rune(line)
	spans, st := lx.Lex(r, 0)
	checkSpans(t, line, r, spans)
	if (st != 0) != open {
		t.Errorf("%s: %q leaves state %v; open is %v", lx.Name(), line, st, open)
	}
}

func TestCMake(t *testing.T) {
	cfgDetect(t, "cmake", "CMakeLists.txt", "cmakelists.txt", "/src/lib/CMakeLists.txt",
		"FindFoo.cmake", "toolchain.CMake")
	lx := lang(t, "cmake")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"cmake_minimum_required(VERSION 3.16)", "cmake_minimum_required", Function},
		// A command is one command in any case.
		{"if(WIN32)", "if", Keyword},
		{"IF(WIN32)", "IF", Keyword},
		{"EndIf()", "EndIf", Keyword},
		{"FOREACH(x IN LISTS y)", "FOREACH", Keyword},
		{"function(my_func arg)", "function", Keyword},
		{"function(my_func arg)", "my_func", Function},
		{"MACRO( my_macro )", "my_macro", Function},
		{"add_executable(demo main.c)", "demo", Plain},
		// if()'s operators are written in upper case only.
		{"if(NOT DEFINED FOO)", "NOT", Keyword},
		{"if(NOT DEFINED FOO)", "DEFINED", Keyword},
		{`if(A STREQUAL "b" AND c)`, "STREQUAL", Keyword},
		{`if(A STREQUAL "b" AND c)`, "AND", Keyword},
		{"if(v VERSION_LESS_EQUAL 3)", "VERSION_LESS_EQUAL", Keyword},
		{"set(x not)", "not", Plain},
		{"set(NOTED 1)", "NOTED", Plain},
		{"target_link_libraries(demo PUBLIC m)", "PUBLIC", Keyword},
		{`option(USE_X "use x" ON)`, "ON", Constant},
		{"set(A off)", "off", Constant},
		{"set(B NOTFOUND)", "NOTFOUND", Constant},
		// An escaped quote opens no string, and the # after it is a comment.
		{`add_definitions(-DFOO=\"bar\") # note`, "bar", Plain},
		{`add_definitions(-DFOO=\"bar\") # note`, "# note", Comment},
		{"set(A a#b)", "#b)", Comment},
	})
	cfgSpans(t, lx, []cfgCase{
		{"cmake_minimum_required(VERSION 3.16)", "3.16", Number},
		{"set(SRC ${EXTRA_SRC} $ENV{HOME})", "${EXTRA_SRC}", Constant},
		{"set(SRC ${EXTRA_SRC} $ENV{HOME})", "$ENV{HOME}", Constant},
		{"set(P $CACHE{PATH})", "$CACHE{PATH}", Constant},
		{"set(N ${a_${b}})", "${a_${b}}", Constant},
		{"target_include_directories(t PUBLIC $<BUILD_INTERFACE:inc>)", "$<BUILD_INTERFACE", Constant},
		{`message(STATUS "hello ${NAME}")`, `"hello ${NAME}"`, String},
		{"set(A 1) # a note", "# a note", Comment},
		{"set(A [[raw ${x}]])", "[[raw ${x}]]", String},
		{"#[[inline]] set(A 1)", "#[[inline]]", Comment},
	})

	// A bracket comment ends only at a bracket of its own level.
	cfgDoc(t, lx, "#[==[ bracket\ncomment with ]] inside\n]==] set(A 1)\n",
		cfgAt{0, "#[==[ bracket", Comment},
		cfgAt{1, "comment with ]] inside", Comment},
		cfgAt{2, "]==]", Comment},
		cfgAt{2, "set", Function},
	)
	// So does a bracket argument.
	cfgDoc(t, lx, "set(S [=[ a\nlong ]] string ]=])\nset(T 1)\n",
		cfgAt{0, "[=[ a", String},
		cfgAt{1, "long ]] string ]=]", String},
		cfgAt{1, ")", Punctuation},
		cfgAt{2, "set", Function},
	)
	// A quoted argument may hold a line break.
	cfgDoc(t, lx, "message(\"multi\nline\")\nset(T 1)\n",
		cfgAt{0, `"multi`, String},
		cfgAt{1, `line"`, String},
		cfgAt{2, "set", Function},
	)
}

func TestNix(t *testing.T) {
	cfgDetect(t, "nix", "default.nix", "flake.nix", "/etc/nixos/configuration.nix", "shell.NIX")
	lx := lang(t, "nix")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"let x = 1; in x", "let", Keyword},
		{"let x = 1; in x", "in", Keyword},
		{"with pkgs; [ hello ]", "with", Keyword},
		{"rec { a = 1; }", "rec", Keyword},
		{"inherit (pkgs) stdenv;", "inherit", Keyword},
		{"y = if a then b else c;", "then", Keyword},
		{"a.b or c", "or", Keyword},
		{"ok = true;", "true", Constant},
		{"v = null;", "null", Constant},
		{"p = import ./p.nix;", "import", Function},
		{"s = builtins.toString 1;", "builtins", Constant},
		{"s = builtins.toString 1;", "toString", Function},
		// No parenthesis finds a call: a list's element is not one.
		{"xs = [ foo (bar 1) ];", "foo", Plain},
		// An attribute, before its =.
		{"  pkg-config = 1;", "pkg-config", Function},
		{"x' = 1;", "x'", Function},
		{"b = a == c;", "a", Plain},
		// // is the update operator, not a path or a comment.
		{"c = a // b;", "//", Operator},
		{"c = a // b;", "b;", Plain},
	})
	cfgSpans(t, lx, []cfgCase{
		{"attrs.foo.bar = 1;", "attrs.foo.bar", Function},
		{"src = ./src/main.c;", "./src/main.c", String},
		{"up = ../lib;", "../lib", String},
		{"m = x/y;", "x/y", String},
		{"h = ~/.config/nix;", "~/.config/nix", String},
		{"p = import <nixpkgs> {};", "<nixpkgs>", String},
		{`s = "a ${b} c";`, `"a ${b} c"`, String},
		{`s = "say \"hi\"";`, `"say \"hi\""`, String},
		{"${name} = 1;", "${name}", Constant},
		{"xs = [ 1 2.5 ];", "2.5", Number},
		{"x = 1; # a note", "# a note", Comment},
		{"x = /* inline */ 1;", "/* inline */", Comment},
		{"e = '''';", "''''", String},
	})

	// An indented string runs over lines, and ''' ''$ and ''\ do not end it.
	cfgDoc(t, lx, "s = ''\n  echo ''${HOME} ''' done ''\\n\n  '';\nx = 1;\n",
		cfgAt{0, "''", String},
		cfgAt{1, "  echo ''${HOME} ''' done ''\\n", String},
		cfgAt{2, "  ''", String},
		cfgAt{2, ";", Punctuation},
		cfgAt{3, "x", Function},
	)
	// So do a block comment and a double-quoted string.
	cfgDoc(t, lx, "/* block\ncomment */ z = 1;\ns = \"two\nlines\";\n",
		cfgAt{0, "/* block", Comment},
		cfgAt{1, "comment */", Comment},
		cfgAt{1, "z", Function},
		cfgAt{2, `"two`, String},
		cfgAt{3, `lines"`, String},
		cfgAt{3, ";", Punctuation},
	)
}

func TestProtobuf(t *testing.T) {
	cfgDetect(t, "protobuf", "api.proto", "/x/v1/Users.PROTO")
	lx := lang(t, "protobuf")
	cfgComment(t, lx, "//", "")

	cfgClasses(t, lx, []cfgCase{
		{`syntax = "proto3";`, "syntax", Keyword},
		{"package demo.v1;", "package", Keyword},
		{"message User {", "message", Keyword},
		{"message User {", "User", Type},
		{"enum Status {", "Status", Type},
		{"service Users {", "Users", Type},
		{"  string name = 1;", "string", Type},
		{"  string name = 1;", "name", Plain},
		{"  repeated int64 ids = 2;", "repeated", Keyword},
		{"  repeated int64 ids = 2;", "int64", Type},
		{"  map<string, Project> projects = 3;", "map", Keyword},
		{"  oneof kind {", "oneof", Keyword},
		{"  optional sfixed64 x = 4;", "sfixed64", Type},
		{"  rpc GetUser(GetUserRequest) returns (stream User);", "rpc", Keyword},
		{"  rpc GetUser(GetUserRequest) returns (stream User);", "GetUser(", Function},
		{"  rpc GetUser(GetUserRequest) returns (stream User);", "GetUserRequest", Plain},
		{"  rpc GetUser(GetUserRequest) returns (stream User);", "returns", Keyword},
		{"  rpc GetUser(GetUserRequest) returns (stream User);", "stream", Keyword},
		{`option java_package = "com.example";`, "java_package", Function},
		{"  bool ok = 5 [deprecated = true];", "true", Constant},
		{"  reserved 6, 8;", "reserved", Keyword},
	})
	cfgSpans(t, lx, []cfgCase{
		{`syntax = "proto3";`, `"proto3"`, String},
		{`import "google/protobuf/timestamp.proto";`, `"google/protobuf/timestamp.proto"`, String},
		{`  string s = 1 [default = 'a\'b'];`, `'a\'b'`, String},
		{"  string name = 12;", "12", Number},
		{"  float f = 1 [default = 1.5e3];", "1.5e3", Number},
		{"  string name = 1; // the name", "// the name", Comment},
		{"  /* inline */ string s = 1;", "/* inline */", Comment},
	})

	cfgDoc(t, lx, "/* a block\n   comment */ message X {}\n",
		cfgAt{0, "/* a block", Comment},
		cfgAt{1, "   comment */", Comment},
		cfgAt{1, "message", Keyword},
		cfgAt{1, "X", Type},
	)
	cfgOpen(t, lx, `string s = "unclosed`, false)
}

func TestGraphQL(t *testing.T) {
	cfgDetect(t, "graphql", "schema.graphql", "query.gql", "types.graphqls")
	lx := lang(t, "graphql")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"type User implements Node & Entity {", "type", Keyword},
		{"type User implements Node & Entity {", "User", Type},
		{"type User implements Node & Entity {", "implements", Keyword},
		{"type User implements Node & Entity {", "Node", Type},
		{"  id: ID!", "ID", Type},
		{"  age: Int", "Int", Type},
		{"  owner: User", "User", Plain},
		{"input NewUser {", "NewUser", Type},
		{"enum Role { ADMIN }", "Role", Type},
		{"enum Role { ADMIN }", "ADMIN", Plain},
		{"scalar Date", "Date", Type},
		{"union Result = A | B", "Result", Type},
		{"query GetUser($id: ID!) {", "query", Keyword},
		{"query GetUser($id: ID!) {", "GetUser", Function},
		{"mutation Rename {", "Rename", Function},
		{"fragment UserFields on User {", "UserFields", Function},
		{"fragment UserFields on User {", "on", Keyword},
		{"  ... on Admin { level }", "Admin", Type},
		{"  user(id: 4) { name }", "user", Function},
		{"  user(id: 4) { name }", "name", Plain},
		{"  f(ok: true, v: null)", "true", Constant},
		{"  f(ok: true, v: null)", "null", Constant},
		{"extend schema @link(url: \"x\")", "extend", Keyword},
		{"directive @auth on FIELD_DEFINITION", "directive", Keyword},
	})
	cfgSpans(t, lx, []cfgCase{
		{"query GetUser($id: ID!) {", "$id", Constant},
		{"  user(id: $user_id) {", "$user_id", Constant},
		{"  name @include(if: $show)", "@include", Function},
		{"  name: String @deprecated", "@deprecated", Function},
		{`  name(format: String = "x y")`, `"x y"`, String},
		{`  f(s: "a \"b\" c")`, `"a \"b\" c"`, String},
		{`  """one line"""`, `"""one line"""`, String},
		{"  f(n: 10, x: 1.5)", "10", Number},
		{"  f(n: 10, x: 1.5)", "1.5", Number},
		{"  id: ID! # the id", "# the id", Comment},
	})

	// A block string runs over lines, and \""" does not end it.
	cfgDoc(t, lx, "\"\"\"\nA \"quoted\" word\nwith \\\"\"\" inside\n\"\"\"\ntype X\n",
		cfgAt{0, `"""`, String},
		cfgAt{1, `A "quoted" word`, String},
		cfgAt{2, `with \""" inside`, String},
		cfgAt{3, `"""`, String},
		cfgAt{4, "type", Keyword},
		cfgAt{4, "X", Type},
	)
	cfgOpen(t, lx, `  f(s: "unclosed`, false)
}

func TestHCL(t *testing.T) {
	cfgDetect(t, "hcl", "main.tf", "terraform.tfvars", "prod.auto.tfvars", "packer.pkr.hcl",
		".terraform.lock.hcl", "docs.nomad")
	lx := lang(t, "hcl")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		// A block's type, whatever the block: Terraform's, Nomad's.
		{`resource "aws_instance" "web" {`, "resource", Keyword},
		{`variable "region" {`, "variable", Keyword},
		{`job "docs" {`, "job", Keyword},
		{"  lifecycle {", "lifecycle", Keyword},
		{"  lifecycle { create_before_destroy = true }", "create_before_destroy", Function},
		{`  dynamic "ingress" {`, "dynamic", Keyword},
		{"locals {", "locals", Keyword},
		// An attribute, before its =; == and => are not one.
		{"  ami = data.aws_ami.ubuntu.id", "ami =", Function},
		{"  ami = data.aws_ami.ubuntu.id", "data", Plain},
		{`  tags = { Name = "web" }`, "tags", Function},
		{`  tags = { Name = "web" }`, "Name", Function},
		{"  ok = a == b", "a", Plain},
		{"  m = {for k, v in var.m : k => v}", "for", Keyword},
		{"  m = {for k, v in var.m : k => v}", "in", Keyword},
		{"  m = {for k, v in var.m : k => v}", "k =>", Plain},
		{"  m = {for k, v in var.m : k => v}", "=>", Operator},
		{`  l = [for s in var.l : upper(s) if s != ""]`, "if", Keyword},
		{`  l = [for s in var.l : upper(s) if s != ""]`, "upper", Function},
		{"  type = list(string)", "list", Type},
		{"  type = list(string)", "string", Type},
		{"  default = null", "null", Constant},
		{"  enabled = true", "true", Constant},
	})
	cfgSpans(t, lx, []cfgCase{
		{`resource "aws_instance" "web" {`, `"web"`, String},
		{`  name = "web-${var.env}"`, `"web-${var.env}"`, String},
		// An interpolation is part of its string, quotes and all.
		{`  v = "${var.a == "x" ? 1 : 2}" # after`, `"${var.a == "x" ? 1 : 2}"`, String},
		{`  v = "${var.a == "x" ? 1 : 2}" # after`, "# after", Comment},
		{`  d = "%{ if x }y%{ endif }"`, `"%{ if x }y%{ endif }"`, String},
		{`  p = "C:\\dir\"q"`, `"C:\\dir\"q"`, String},
		{"  count = 3", "3", Number},
		{"  ratio = 0.25", "0.25", Number},
		{"  ami = x // trailing", "// trailing", Comment},
		{"  ami = x # trailing", "# trailing", Comment},
		{"  /* inline */ ami = x", "/* inline */", Comment},
	})

	// A here-document runs to a line holding only its word.
	cfgDoc(t, lx, "  user_data = <<-EOT\n    echo \"${var.name}\"\n    EOTX is not the end\n    EOT\n  x = 1\n",
		cfgAt{0, "user_data", Function},
		cfgAt{0, "<<-EOT", String},
		cfgAt{1, `    echo "${var.name}"`, String},
		cfgAt{2, "    EOTX is not the end", String},
		cfgAt{3, "    EOT", String},
		cfgAt{4, "x", Function},
	)
	cfgDoc(t, lx, "policy = <<POLICY\n{ \"a\": 1 }\nPOLICY\ny = 2\n",
		cfgAt{1, `{ "a": 1 }`, String},
		cfgAt{2, "POLICY", String},
		cfgAt{3, "y", Function},
	)
	cfgDoc(t, lx, "/* a\nblock */ z = 1\n",
		cfgAt{0, "/* a", Comment},
		cfgAt{1, "block */", Comment},
		cfgAt{1, "z", Function},
	)
	cfgOpen(t, lx, `  name = "unclosed ${x`, false)
}

func TestLaTeX(t *testing.T) {
	cfgDetect(t, "latex", "paper.tex", "style.sty", "thesis.cls", "x.ltx", "pkg.dtx", "Main.TEX")
	lx := lang(t, "latex")
	cfgComment(t, lx, "%", "")

	cfgClasses(t, lx, []cfgCase{
		{`\documentclass[11pt]{article}`, `\documentclass`, Keyword},
		{`\documentclass[11pt]{article}`, "article", Plain},
		{`\section*{Intro}\label{sec:intro}`, `\label`, Keyword},
		{`\makeatletter\@ifundefined{x}`, `\@ifundefined`, Keyword},
		// Prose: words, numbers and punctuation are text.
		{"Some words, 42 of them (really).", "Some", Plain},
		{"Some words, 42 of them (really).", "42", Plain},
		{"Some words, 42 of them (really).", "(", Plain},
		// \% is a control symbol, not a comment.
		{`Half is 50\% off % a note`, `\%`, Keyword},
		{`Half is 50\% off % a note`, "off", Plain},
		{`Half is 50\% off % a note`, "% a note", Comment},
		{`line \\[2pt] next`, `\\`, Keyword},
		{`line \\[2pt] next`, "2pt", Plain},
		// A verbatim environment's name elsewhere opens nothing.
		{`\usepackage{verbatim} % load it`, "{verbatim}", Plain},
		{`\usepackage{verbatim} % load it`, "% load it", Comment},
		{`\emph{copy it verbatim} and $x$`, "$x$", String},
	})
	cfgSpans(t, lx, []cfgCase{
		{`\section*{Intro}`, `\section*`, Keyword},
		{`\begin{document}`, `\begin`, Keyword},
		{`\begin{document}`, "document", Type},
		{`\end{align*}`, `\end`, Keyword},
		{`\end{align*}`, "align*", Type},
		{`Inline $x^2 + \$y$ math.`, `$x^2 + \$y$`, String},
		{`Inline \( a + b \) math.`, `\( a + b \)`, String},
		{`Show \verb|x%y| here.`, `\verb|x%y|`, String},
		{`Show \verb+a$b+ here.`, `\verb+a$b+`, String},
	})
	for _, line := range []string{`\usepackage{verbatim}`, `\emph{copy it verbatim} and more`, `a $x`, `a \( x`} {
		cfgOpen(t, lx, line, false)
	}

	// Display math runs over lines.
	cfgDoc(t, lx, "\\[\n  E = mc^2 \\\\[2pt]\n\\] after\n$$\nx\n$$ done\n",
		cfgAt{0, `\[`, String},
		cfgAt{1, `  E = mc^2 \\[2pt]`, String},
		cfgAt{2, `\]`, String},
		cfgAt{2, "after", Plain},
		cfgAt{3, "$$", String},
		cfgAt{4, "x", String},
		cfgAt{5, "$$", String},
		cfgAt{5, "done", Plain},
	)
	// A verbatim environment's body is text, % and $ and all.
	cfgDoc(t, lx, "\\begin{verbatim}\nprintf(\"%d $ \\n\");\n\\end{verbatim}\n\\textbf{x} % c\n",
		cfgAt{0, `\begin{`, Keyword},
		cfgAt{1, `printf("%d $ \n");`, String},
		cfgAt{2, `\end{verbatim}`, String},
		cfgAt{3, `\textbf`, Keyword},
		cfgAt{3, "% c", Comment},
	)
	cfgDoc(t, lx, "\\begin{lstlisting}[language=C]\n// %d\n\\end{lstlisting}\ntext\n",
		cfgAt{1, "// %d", String},
		cfgAt{3, "text", Plain},
	)
}

func TestDiff(t *testing.T) {
	cfgDetect(t, "diff", "fix.diff", "0001-add-x.patch", "main.c.rej")
	lx := lang(t, "diff")
	if _, _, ok := lx.Comment(); ok {
		t.Error("a diff has no comments of its own")
	}

	cfgSpans(t, lx, []cfgCase{
		{"diff --git a/main.go b/main.go", "diff --git a/main.go b/main.go", Keyword},
		{"index 83db48f..bf269f4 100644", "index 83db48f..bf269f4 100644", Keyword},
		{"--- a/main.go", "--- a/main.go", Keyword},
		{"+++ b/main.go", "+++ b/main.go", Keyword},
		{"--- /dev/null", "--- /dev/null", Keyword},
		{"--- old.txt\t2024-01-01 10:00:00", "--- old.txt\t2024-01-01 10:00:00", Keyword},
		{"+++ new.txt\t2024-01-01 10:00:00", "+++ new.txt\t2024-01-01 10:00:00", Keyword},
		{"new file mode 100644", "new file mode 100644", Keyword},
		{"rename from old.go", "rename from old.go", Keyword},
		{"@@ -1,3 +1,4 @@ func main() {", "@@ -1,3 +1,4 @@", Function},
		{"@@@ -1,2 -1,2 +1,3 @@@", "@@@ -1,2 -1,2 +1,3 @@@", Function},
		{"+new line", "+new line", String},
		{"-old line", "-old line", Constant},
		// A removed "-- note" or an added "++ x" is a line, not a header.
		{"--- note in a removed lua comment", "--- note in a removed lua comment", Constant},
		{"+++ added x y", "+++ added x y", String},
		{`\ No newline at end of file`, `\ No newline at end of file`, Comment},
	})
	cfgClasses(t, lx, []cfgCase{
		{"@@ -1,3 +1,4 @@ func main() {", "func", Plain},
		// The lines around a change are left as they are.
		{` context 42 "x" // y`, "42", Plain},
		{` context 42 "x" // y`, `"x"`, Plain},
		{` context 42 "x" // y`, "//", Plain},
	})

	cfgDoc(t, lx, "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a /* open\n+b\n c\n",
		cfgAt{0, "diff --git a/x b/x", Keyword},
		cfgAt{3, "@@ -1 +1 @@", Function},
		cfgAt{4, "-a /* open", Constant},
		cfgAt{5, "+b", String},
		cfgAt{6, " c", Plain},
	)
}

func TestGitCommit(t *testing.T) {
	cfgDetect(t, "gitcommit", "COMMIT_EDITMSG", "/repo/.git/COMMIT_EDITMSG", "MERGE_MSG",
		"TAG_EDITMSG", "SQUASH_MSG")
	lx := lang(t, "gitcommit")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"feat(syntax)!: add languages", "feat", Keyword},
		{"feat(syntax)!: add languages", "syntax", Type},
		{"feat(syntax)!: add languages", "!", Keyword},
		{"feat(syntax)!: add languages", "add", Plain},
		{"fix: a bug", "fix", Keyword},
		{"docs(readme): say more", "docs", Keyword},
		// Not a Conventional Commits prefix.
		{"fixture: a word", "fixture", Plain},
		{"Refactor the thing", "Refactor", Plain},
		// Prose: numbers and operators are text.
		{"Add 42 rules, x = y.", "42", Plain},
		{"Add 42 rules, x = y.", "=", Plain},
		// A # anywhere but at the very start of a line is text.
		{"Fixes #12 and C# in a#b", "#12", Plain},
		{"Fixes #12 and C# in a#b", "C#", Plain},
		{"Fixes #12 and C# in a#b", "#b", Plain},
		{" # indented", "#", Plain},
		{"Signed-off-by: A Person <a@example.com>", "Signed-off-by", Function},
		{"Signed-off-by: A Person <a@example.com>", "Person", Plain},
		{"Co-authored-by: B <b@example.com>", "Co-authored-by", Function},
		{"Change-Id: I1234", "Change-Id", Function},
		{"Refs: #42", "Refs", Function},
	})
	cfgSpans(t, lx, []cfgCase{
		{"BREAKING CHANGE: it breaks", "BREAKING CHANGE", Function},
		{"# Please enter the commit message", "# Please enter the commit message", Comment},
		{"#\tmodified:   main.go", "#\tmodified:   main.go", Comment},
	})

	// Below the scissors line, everything is left out: the diff of commit -v.
	cfgDoc(t, lx, "feat: x\n# a comment\nbody #1\n# ------------------------ >8 ------------------------\n# Do not modify\ndiff --git a/x b/x\n+added line\nfeat: not a prefix\n",
		cfgAt{0, "feat", Keyword},
		cfgAt{1, "# a comment", Comment},
		cfgAt{2, "body #1", Plain},
		cfgAt{3, "# ------------------------ >8 ------------------------", Comment},
		cfgAt{4, "# Do not modify", Comment},
		cfgAt{5, "diff --git a/x b/x", Comment},
		cfgAt{6, "+added line", Comment},
		cfgAt{7, "feat: not a prefix", Comment},
	)
}

func TestGitIgnore(t *testing.T) {
	cfgDetect(t, "gitignore", ".gitignore", "/repo/sub/.gitignore", ".dockerignore", ".npmignore",
		".hgignore", ".prettierignore", ".eslintignore", ".gcloudignore", ".containerignore")
	lx := lang(t, "gitignore")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"/build/", "build", Plain},
		{"*.log", "*", Keyword},
		{"*.log", "log", Plain},
		{"!important.log", "!", Operator},
		{"docs/**/tmp?.txt", "?", Keyword},
		// A # is a comment only at the very start of a line.
		{"foo#bar", "#bar", Plain},
		{" #not a comment", "#not", Plain},
		{`\#literal`, "#literal", Plain},
		{`\!bang`, "!bang", Plain},
		{"secret 42.txt", "42", Plain},
	})
	cfgSpans(t, lx, []cfgCase{
		{"# build output", "# build output", Comment},
		{"docs/**/tmp?.txt", "**", Keyword},
		{"file[0-9].bak", "[0-9]", Constant},
		{"syntax: glob", "syntax: glob", Keyword},
	})

	cfgDoc(t, lx, "# deps\nnode_modules/\n!keep/*.js\n",
		cfgAt{0, "# deps", Comment},
		cfgAt{1, "node_modules/", Plain},
		cfgAt{2, "!", Operator},
		cfgAt{2, "*", Keyword},
	)
}

func TestDotenv(t *testing.T) {
	cfgDetect(t, "dotenv", ".env", ".env.local", ".env.production", "/app/.env", "prod.env")
	lx := lang(t, "dotenv")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{`DATABASE_URL="postgres://u@h/db#frag"`, "DATABASE_URL", Function},
		{`export API_KEY='s3cr3t # x'`, "export", Keyword},
		{`export API_KEY='s3cr3t # x'`, "API_KEY", Function},
		{"  app.name=demo", "app.name", Function},
		{"DEBUG=true", "true", Constant},
		{"NAME=demo", "demo", Plain},
		// A # starts a comment only at the start of a line or after a blank.
		{"COLOR=#fff", "#fff", Plain},
		{"URL=https://x.org/#top # comment", "#top", Plain},
		{"URL=https://x.org/#top # comment", "# comment", Comment},
	})
	cfgSpans(t, lx, []cfgCase{
		{`DATABASE_URL="postgres://u@h/db#frag"`, `"postgres://u@h/db#frag"`, String},
		{`export API_KEY='s3cr3t # x'`, `'s3cr3t # x'`, String},
		{`MSG="say \"hi\""`, `"say \"hi\""`, String},
		{"PORT=8080", "8080", Number},
		{"VERSION=1.2.3", "1.2.3", Number},
		{"PATH_EXT=${HOME}/bin:$PATH", "${HOME}", Constant},
		{"PATH_EXT=${HOME}/bin:$PATH", "$PATH", Constant},
		{"# a comment", "# a comment", Comment},
		{"  # indented", "# indented", Comment},
	})

	// A double-quoted value may run over lines; a single-quoted one ends with
	// its line, since an apostrophe is common in a bare value.
	cfgDoc(t, lx, "KEY=\"-----BEGIN KEY-----\nabc\n-----END KEY-----\"\nNEXT=1\n",
		cfgAt{0, `"-----BEGIN KEY-----`, String},
		cfgAt{1, "abc", String},
		cfgAt{2, `-----END KEY-----"`, String},
		cfgAt{3, "NEXT", Function},
	)
	cfgOpen(t, lx, "MSG=don't", false)
}

func TestGoMod(t *testing.T) {
	// An exact name beats any pattern.
	cfgDetect(t, "gomod", "go.mod", "go.work", "/src/nem/go.mod", "GO.MOD")
	lx := lang(t, "gomod")
	cfgComment(t, lx, "//", "")

	cfgClasses(t, lx, []cfgCase{
		{"module github.com/Borderliner/nem", "module", Keyword},
		{"module github.com/Borderliner/nem", "github", Plain},
		{"go 1.22.1", "go", Keyword},
		{"toolchain go1.23.0", "toolchain", Keyword},
		{"require golang.org/x/net v0.20.0", "require", Keyword},
		{"require golang.org/x/net v0.20.0", "golang", Plain},
		{"replace example.com/a => ../a", "replace", Keyword},
		{"exclude example.com/a v1.2.3", "exclude", Keyword},
		{"retract [v1.0.0, v1.0.5]", "retract", Keyword},
		{"use ./tools", "use", Keyword},
		{"godebug default=go1.21", "godebug", Keyword},
		{"tool golang.org/x/tools/cmd/stringer", "tool golang", Keyword},
		// A major version in a path is part of the path.
		{"require example.com/foo/v2 v2.1.0", "v2 v2", Plain},
		{"require gopkg.in/yaml.v3 v3.0.1", "v3 ", Plain},
	})
	cfgSpans(t, lx, []cfgCase{
		{"go 1.22.1", "1.22.1", Number},
		{"toolchain go1.23.0", "go1.23.0", Number},
		{"go 1.21rc2", "1.21rc2", Number},
		{"require golang.org/x/net v0.20.0 // indirect", "v0.20.0", Number},
		{"require golang.org/x/net v0.20.0 // indirect", "// indirect", Comment},
		{"require example.com/p v0.0.0-20210101000000-abcdef123456", "v0.0.0-20210101000000-abcdef123456", Number},
		{"require example.com/old v1.0.0+incompatible", "v1.0.0+incompatible", Number},
		{"retract [v1.0.0, v1.0.5]", "v1.0.5", Number},
		{"godebug default=go1.21", "go1.21", Number},
		{"replace example.com/a => ../a", "=>", Operator},
		{`module "example.com/quoted"`, `"example.com/quoted"`, String},
	})

	// A block of requirements leaves nothing open between its lines.
	cfgDoc(t, lx, "require (\n\tgolang.org/x/net v0.20.0 // indirect\n\tgopkg.in/yaml.v3 v3.0.1\n)\ngo 1.22\n",
		cfgAt{0, "require", Keyword},
		cfgAt{1, "v0.20.0", Number},
		cfgAt{1, "// indirect", Comment},
		cfgAt{2, "v3.0.1", Number},
		cfgAt{3, ")", Punctuation},
		cfgAt{4, "go", Keyword},
	)
}

func TestMeson(t *testing.T) {
	cfgDetect(t, "meson", "meson.build", "/src/lib/meson.build", "meson_options.txt", "meson.options")
	lx := lang(t, "meson")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{"project('demo', 'c', version : '1.0')", "project", Function},
		{"project('demo', 'c', version : '1.0')", "version", Function},
		{"exe = executable('demo', src, install : true)", "src", Plain},
		{"exe = executable('demo', src, install : true)", "install", Function},
		{"exe = executable('demo', src, install : true)", "true", Constant},
		// The names foreach sets are not keyword arguments.
		{"foreach name, dep : deps", "foreach", Keyword},
		{"foreach name, dep : deps", "name", Plain},
		{"foreach name, dep : deps", "dep ", Plain},
		{"if host_machine.system() == 'linux' and not false", "host_machine", Constant},
		{"if host_machine.system() == 'linux' and not false", "system", Function},
		{"if host_machine.system() == 'linux' and not false", "and", Keyword},
		{"if host_machine.system() == 'linux' and not false", "not", Keyword},
		{"elif x", "elif", Keyword},
		{"endforeach", "endforeach", Keyword},
	})
	cfgSpans(t, lx, []cfgCase{
		{"project('demo', 'c')", "'demo'", String},
		{`s = 'it\'s'`, `'it\'s'`, String},
		{"msg = f'@name@ is @version@'", "f'@name@ is @version@'", String},
		{"x = 0x1F + 42", "0x1F", Number},
		{"x = 0x1F + 42", "42", Number},
		{"x = 1 # a note", "# a note", Comment},
	})

	// A triple-quoted string runs over lines, and has no escapes.
	cfgDoc(t, lx, "doc = '''\nmulti 'line' \\\n'''\nx = 1\n",
		cfgAt{0, "'''", String},
		cfgAt{1, `multi 'line' \`, String},
		cfgAt{2, "'''", String},
		cfgAt{3, "x", Plain},
		cfgAt{3, "1", Number},
	)
	cfgOpen(t, lx, "s = 'unclosed", false)
}

func TestJust(t *testing.T) {
	cfgDetect(t, "just", "justfile", "Justfile", ".justfile", "/repo/Justfile", "release.just")
	lx := lang(t, "just")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		// A recipe: its name before the : that is not :=.
		{"build target='debug': deps", "build", Function},
		{"build target='debug': deps", "target", Plain},
		{"build target='debug': deps", "deps", Plain},
		{"@test *args:", "test", Function},
		{"build-release:", "build-release", Function},
		// A variable, where it is set.
		{"version := `git describe`", "version", Constant},
		{`export RUST_LOG := "info"`, "export", Keyword},
		{`export RUST_LOG := "info"`, "RUST_LOG", Constant},
		{`set shell := ["bash", "-uc"]`, "set", Keyword},
		{`set shell := ["bash", "-uc"]`, "shell", Plain},
		{"alias b := build", "alias", Keyword},
		{"alias b := build", "b ", Function},
		{"import 'ci.just'", "import", Keyword},
		{"mod tools", "mod", Keyword},
		{`os_name := if os() == "linux" { "l" } else { "o" }`, "if", Keyword},
		{`os_name := if os() == "linux" { "l" } else { "o" }`, "else", Keyword},
		{`os_name := if os() == "linux" { "l" } else { "o" }`, "os(", Function},
		// A recipe's line is not a recipe.
		{"    echo done: ok", "echo", Plain},
		{"set dotenv-load := true", "true", Constant},
	})
	cfgSpans(t, lx, []cfgCase{
		{"build target='debug': deps", "'debug'", String},
		{"version := `git describe`", "`git describe`", String},
		{`set shell := ["bash", "-uc"]`, `"bash"`, String},
		{"    cargo build --profile {{target}} # c", "{{target}}", Constant},
		{"    cargo build --profile {{target}} # c", "# c", Comment},
		{"    echo {{ join(dir, \"x\") }}", "{{ join(dir, \"x\") }}", Constant},
		{"[private]", "[private]", Type},
		{"[group('ci')]", "[group('ci')]", Type},
		{"    sleep 5", "5", Number},
	})

	// Triple-quoted strings and backticks run over lines.
	cfgDoc(t, lx, "long := \"\"\"\n  text: here\n\"\"\"\nnext:\n",
		cfgAt{0, "long", Constant},
		cfgAt{0, `"""`, String},
		cfgAt{1, "  text: here", String},
		cfgAt{2, `"""`, String},
		cfgAt{3, "next", Function},
	)
	cfgDoc(t, lx, "out := ```\n  ls -la\n```\nrecipe:\n    echo {{out}}\n",
		cfgAt{0, "```", String},
		cfgAt{1, "  ls -la", String},
		cfgAt{2, "```", String},
		cfgAt{3, "recipe", Function},
		cfgAt{4, "{{out}}", Constant},
	)
	cfgOpen(t, lx, `x := "unclosed`, false)
}

func TestDot(t *testing.T) {
	cfgDetect(t, "dot", "graph.dot", "deps.gv", "G.DOT")
	lx := lang(t, "dot")
	cfgComment(t, lx, "//", "")

	cfgClasses(t, lx, []cfgCase{
		{"digraph G {", "digraph", Keyword},
		{"digraph G {", "G", Type},
		// The keywords are the same in any case.
		{"DiGraph H {", "DiGraph", Keyword},
		{"strict graph {", "strict", Keyword},
		{"  subgraph cluster_0 { c -- d; }", "subgraph", Keyword},
		{"  subgraph cluster_0 { c -- d; }", "cluster_0", Type},
		{"  subgraph cluster_0 { c -- d; }", "c ", Plain},
		{"  rankdir=LR;", "rankdir", Function},
		{"  rankdir=LR;", "LR", Plain},
		{`  node [shape=box, color="red"];`, "node", Keyword},
		{`  node [shape=box, color="red"];`, "shape", Function},
		{"  edge [style=dashed];", "edge", Keyword},
	})
	cfgSpans(t, lx, []cfgCase{
		{"  a -> b;", "->", Operator},
		{"  c -- d;", "--", Operator},
		{`  node [shape=box, color="red"];`, `"red"`, String},
		{`  a [label="say \"hi\""];`, `"say \"hi\""`, String},
		{"  a [label=<<b>bold</b> text>];", "<<b>bold</b> text>", String},
		{"  x [width=1.5, height=-.25];", "1.5", Number},
		{"  x [width=1.5, height=-.25];", ".25", Number},
		{"# 1 \"graph.gv\"", "# 1 \"graph.gv\"", Comment},
		{"  a -> b; // edge", "// edge", Comment},
		{"  a /* inline */ -> b;", "/* inline */", Comment},
	})
	// A # after the start of a line is not a comment.
	assertClass(t, lx, `  a [color="#ff0000"];`, `"#ff0000"`, String)

	// An HTML label nests its tags, and may run over lines.
	cfgDoc(t, lx, "a [label=<\n  <table><tr><td>x</td></tr></table>\n>];\nb -> c;\n",
		cfgAt{0, "<", String},
		cfgAt{1, "  <table><tr><td>x</td></tr></table>", String},
		cfgAt{2, ">", String},
		cfgAt{2, "];", Punctuation},
		cfgAt{3, "->", Operator},
	)
	cfgDoc(t, lx, "/* a\nblock */ a -> b;\nx [label=\"two\nlines\"];\n",
		cfgAt{0, "/* a", Comment},
		cfgAt{1, "block */", Comment},
		cfgAt{1, "->", Operator},
		cfgAt{2, `"two`, String},
		cfgAt{3, `lines"`, String},
	)
}

func TestPo(t *testing.T) {
	cfgDetect(t, "po", "de.po", "/locale/fr/LC_MESSAGES/nem.po", "messages.pot")
	lx := lang(t, "po")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		{`msgid "Hello"`, "msgid", Keyword},
		{`msgstr "Hallo"`, "msgstr", Keyword},
		{`msgctxt "menu"`, "msgctxt", Keyword},
		{`msgid_plural "Hellos"`, "msgid_plural", Keyword},
		{`msgstr[1] "Hallos"`, "msgstr", Keyword},
		// The flags, fuzzy among them, change what the entry means.
		{"#, fuzzy, c-format", "#,", Comment},
		{"#, fuzzy, c-format", "fuzzy", Keyword},
		{"#, fuzzy, c-format", "c-format", Keyword},
		{`#| msgid "Old"`, "msgid", Comment},
	})
	cfgSpans(t, lx, []cfgCase{
		{`msgid "Hello \"%s\"\n"`, `"Hello \"%s\"\n"`, String},
		{`msgstr[1] "Hallos"`, "1", Number},
		{"# Translator comment", "# Translator comment", Comment},
		{"#. extracted", "#. extracted", Comment},
		{"#: src/main.c:12 src/other.c:40", "#: src/main.c:12 src/other.c:40", Comment},
		{`#~ msgid "gone"`, `#~ msgid "gone"`, Comment},
	})

	// An entry's strings continue on the lines below it, each one whole.
	cfgDoc(t, lx, "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Plural-Forms: nplurals=2;\\n\"\n\nmsgid \"x\"\n",
		cfgAt{1, "msgstr", Keyword},
		cfgAt{2, `"Content-Type: text/plain; charset=UTF-8\n"`, String},
		cfgAt{3, `"Plural-Forms: nplurals=2;\n"`, String},
		cfgAt{5, "msgid", Keyword},
	)
	cfgOpen(t, lx, `msgid "unclosed`, false)
}

func TestProperties(t *testing.T) {
	cfgDetect(t, "properties", "application.properties", "messages_de.properties", "/res/Log4j.PROPERTIES")
	lx := lang(t, "properties")
	cfgComment(t, lx, "#", "")

	cfgClasses(t, lx, []cfgCase{
		// A # or ! in a value is text.
		{"app.name=My App # not a comment", "My", Plain},
		{"app.name=My App # not a comment", "# not", Plain},
		{"app.url = http://x.org/#top", "#top", Plain},
		{"greeting=Hi! there", "!", Plain},
		{"spaced key value", "key", Plain},
		{"count=42", "42", Plain},
	})
	cfgSpans(t, lx, []cfgCase{
		{"app.name=My App # not a comment", "app.name", Function},
		{"app.url = http://x.org/#top", "app.url", Function},
		{"db.user: admin", "db.user", Function},
		{"spaced key value", "spaced", Function},
		{"   indented.key=1", "indented.key", Function},
		{`key\:with\=escapes : value`, `key\:with\=escapes`, Function},
		{"# a comment", "# a comment", Comment},
		{"! another comment", "! another comment", Comment},
		{"   # indented comment", "# indented comment", Comment},
	})

	// A value ending in a lone \ goes on over the next line, whatever that
	// line starts with; an even run of them is escaped and ends the value.
	cfgDoc(t, lx, "multi = first, \\\n    #second, \\\n    !third=3\nafter=1\neven=ends with \\\\\nnext.key=2\n",
		cfgAt{0, "multi", Function},
		cfgAt{1, "    #second, \\", Plain},
		cfgAt{2, "    !third=3", Plain},
		cfgAt{3, "after", Function},
		cfgAt{4, "even", Function},
		cfgAt{5, "next.key", Function},
	)
}
