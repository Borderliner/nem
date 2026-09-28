package syntax

import "testing"

// Systems and hardware languages: Ada, Nim, Zig, D, Pascal, Fortran,
// assembly, Verilog, VHDL and Odin. Each test pins what its definition was
// written to get right - the file names it claims, the forms of its
// literals, and whatever it needed a match or region for - so a later edit
// to one of them cannot quietly lose it.

// assertDetects checks that nem gives each path the language want.
func assertDetects(t *testing.T, want string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if got := For(p).Name(); got != want {
			t.Errorf("For(%q) = %q, want %q", p, got, want)
		}
	}
}

// assertShebang checks that a script with no extension and this first line
// is the language want.
func assertShebang(t *testing.T, want, first string) {
	t.Helper()
	if got := ForWithHeader("script", first).Name(); got != want {
		t.Errorf("ForWithHeader(script, %q) = %q, want %q", first, got, want)
	}
}

// assertCommentsWith checks the comment M-; writes for a language: the first
// one its definition lists.
func assertCommentsWith(t *testing.T, lx *Language, start, end string) {
	t.Helper()
	s, e, ok := lx.Comment()
	if !ok || s != start || e != end {
		t.Errorf("%s comments with %q %q (%v), want %q %q", lx.Name(), s, e, ok, start, end)
	}
}

// assertInDoc lexes doc from its top and checks that sub, which must occur
// once on line n, is want there: how a construct that spans lines is tested.
func assertInDoc(t *testing.T, lx Lexer, doc string, n int, sub string, want Class) {
	t.Helper()
	lines, spans := lexDoc(lx, doc)
	if n >= len(lines) {
		t.Fatalf("the document has %d lines, not a line %d", len(lines), n)
	}
	src := string(lines[n])
	at := len([]rune(src[:indexOnce(t, src, sub)]))
	checkSpans(t, src, lines[n], spans[n])
	if got := classAt(spans[n], at); got != want {
		t.Errorf("line %d %q: %q is %v, want %v (spans: %s)", n, src, sub, got, want, describe(lines[n], spans[n]))
	}
}

func TestAda(t *testing.T) {
	assertDetects(t, "ada", "main.adb", "pkg.ads", "old.ada", "build.gpr", "MAIN.ADB")
	lx := lang(t, "ada")
	assertCommentsWith(t, lx, "--", "")

	// Keywords, types and constants in any case.
	assertClass(t, lx, "procedure Main is", "procedure", Keyword)
	assertClass(t, lx, "PROCEDURE Main IS", "IS", Keyword)
	assertClass(t, lx, "X : Integer := 0;", "Integer", Type)
	assertClass(t, lx, "X : INTEGER := 0;", "INTEGER", Type)
	assertClass(t, lx, "Done : Boolean := True;", "True", Constant)
	assertClass(t, lx, "P := null;", "null", Constant)

	// What a declaration names.
	assertClass(t, lx, "procedure Main is", "Main", Function)
	assertClass(t, lx, "function Twice (X : Integer) return Integer is", "Twice", Function)
	assertClass(t, lx, "type Color is (Red, Green);", "Color", Type)
	assertClass(t, lx, "subtype Small is Integer range 1 .. 10;", "Small", Type)
	assertClass(t, lx, "package body Pkg is", "Pkg", Type)
	assertClass(t, lx, "Put_Line (S);", "Put_Line", Function)

	// A string doubles its quote to hold one.
	assertSpanCovers(t, lx, `S : String := "say ""hi"" now";`, `"say ""hi"" now"`, String)
	assertSpanCovers(t, lx, "X := 1; -- a note", "-- a note", Comment)
	assertSpanCovers(t, lx, `S := "a -- b";`, `"a -- b"`, String)

	// Based and decimal literals.
	assertSpanCovers(t, lx, "X := 16#FF#;", "16#FF#", Number)
	assertSpanCovers(t, lx, "X := 2#1010_1010#;", "2#1010_1010#", Number)
	assertSpanCovers(t, lx, "Y := 1.0E-6;", "1.0E-6", Number)
	assertSpanCovers(t, lx, "N := 1_000;", "1_000", Number)
	assertSpanCovers(t, lx, "for I in 1 .. 10 loop", "10", Number)

	// The tick: a character literal, an attribute, a qualified expression.
	assertSpanCovers(t, lx, "C := 'x';", "'x'", String)
	assertSpanCovers(t, lx, "if C = ''' then", "'''", String)
	assertClass(t, lx, "N := S'Length;", "'", Punctuation)
	assertSpanCovers(t, lx, "N := S'Length;", "Length", Function)
	assertClass(t, lx, "Put (Integer'Image (N));", "Image", Function)
	assertClass(t, lx, "for I in A'First .. A'Last loop", "First", Function)
	assertClass(t, lx, "for I in A'First .. A'Last loop", "Last", Function)
	// T'( is a qualification, so the ')' after it is a character literal
	// and the tick before ( opens nothing.
	assertClass(t, lx, "Q := Character'(')');", "Character", Type)
	assertClass(t, lx, "Q := Character'(')');", "'(", Punctuation)
	assertSpanCovers(t, lx, "Q := Character'(')');", "')'", String)
}

func TestNim(t *testing.T) {
	assertDetects(t, "nim", "main.nim", "config.nims", "pkg.nimble")
	assertShebang(t, "nim", "#!/usr/bin/env nim")
	assertShebang(t, "nim", "#!/usr/bin/env -S nim --hints:off")
	lx := lang(t, "nim")
	assertCommentsWith(t, lx, "#", "")

	assertClass(t, lx, "proc greet*(name: string): string =", "proc", Keyword)
	assertClass(t, lx, "proc greet*(name: string): string =", "greet", Function)
	assertClass(t, lx, "iterator items(s: Stack): int =", "items", Function)
	assertClass(t, lx, "let n: int = 0", "int", Type)
	assertClass(t, lx, "if p == nil: discard", "nil", Constant)
	assertClass(t, lx, "result = true", "result", Constant)
	assertClass(t, lx, "echo len(s)", "echo", Function)
	assertClass(t, lx, "type Id = distinct int", "Id", Type)

	// Strings: an escape, a character, and raw strings, which double a quote.
	assertSpanCovers(t, lx, `echo "a\"b" & x`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `let c = '\''`, `'\''`, String)
	assertSpanCovers(t, lx, `let p = r"C:\dir""q" & x`, `r"C:\dir""q"`, String)
	assertSpanCovers(t, lx, `let m = re"\d+" & x`, `re"\d+"`, String)
	assertClass(t, lx, `let m = re"\d+" & x`, "x", Plain)

	// Comments, and ## doc comments.
	assertSpanCovers(t, lx, "let x = 1 # a note", "# a note", Comment)
	assertSpanCovers(t, lx, "  ## Returns the thing.", "## Returns the thing.", Comment)

	// A number may carry a suffix after a tick.
	assertSpanCovers(t, lx, "let n = 1'i32 + x", "1'i32", Number)
	assertSpanCovers(t, lx, "let n = 0xFF'u8 + x", "0xFF'u8", Number)
	assertSpanCovers(t, lx, "let n = 1_000 + x", "1_000", Number)
	assertSpanCovers(t, lx, "let n = 1.5e3 + x", "1.5e3", Number)
	assertSpanCovers(t, lx, "let n = 0b1010 + x", "0b1010", Number)

	// An operator defined in backticks, pragmas, and a type section.
	assertSpanCovers(t, lx, "proc `+`*(a, b: V): V =", "`+`", Function)
	assertSpanCovers(t, lx, "proc f() {.inline.} =", "{.", Keyword)
	assertSpanCovers(t, lx, "proc f() {.inline.} =", ".}", Keyword)
	assertClass(t, lx, "  Node* = ref object of RootObj", "Node", Type)
	assertClass(t, lx, "  Color {.pure.} = enum", "Color", Type)

	// Block comments nest, and run over lines.
	doc := "#[ open\n  #[ inner ]# still\n]# code\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 2, "]#", Comment)
	assertInDoc(t, lx, doc, 2, "code", Plain)
	doc = "##[ doc\nmore\n]## x\n"
	assertInDoc(t, lx, doc, 1, "more", Comment)
	assertInDoc(t, lx, doc, 2, "x", Plain)

	// A triple-quoted string is raw and runs over lines.
	doc = "let s = \"\"\"raw \\n\nstill\"\"\" & x\n"
	assertInDoc(t, lx, doc, 0, `\n`, String)
	assertInDoc(t, lx, doc, 1, "still", String)
	assertInDoc(t, lx, doc, 1, "x", Plain)
}

func TestZig(t *testing.T) {
	assertDetects(t, "zig", "main.zig", "build.zig.zon")
	lx := lang(t, "zig")
	assertCommentsWith(t, lx, "//", "")

	assertClass(t, lx, "pub fn main() !void {", "pub", Keyword)
	assertClass(t, lx, "pub fn main() !void {", "main", Function)
	assertClass(t, lx, "pub fn main() !void {", "void", Type)
	assertClass(t, lx, "const f: f64 = 1.5;", "f64", Type)
	assertClass(t, lx, "if (x == null) return;", "null", Constant)
	assertClass(t, lx, "var buf: [4]u8 = undefined;", "undefined", Constant)

	// Integers of any width.
	assertSpanCovers(t, lx, "var c: u3 = 0;", "u3", Type)
	assertSpanCovers(t, lx, "var c: i128 = 0;", "i128", Type)
	assertClass(t, lx, "var u3x = 0;", "u3x", Plain)

	assertSpanCovers(t, lx, `print("hi\n", .{});`, `"hi\n"`, String)
	assertSpanCovers(t, lx, `const q = "a\"b";`, `"a\"b"`, String)
	assertSpanCovers(t, lx, "const nl = '\\n';", "'\\n'", String)
	assertSpanCovers(t, lx, "x += 1; // a note", "// a note", Comment)
	assertSpanCovers(t, lx, "/// A point.", "/// A point.", Comment)
	assertSpanCovers(t, lx, "const h = 0x1p-3;", "0x1p-3", Number)
	assertSpanCovers(t, lx, "const n = 1_000;", "1_000", Number)
	assertSpanCovers(t, lx, "const b = 0b1010;", "0b1010", Number)

	// Builtins, and a container given a name.
	assertSpanCovers(t, lx, `const std = @import("std");`, "@import", Function)
	assertClass(t, lx, "pub const Point = struct {", "Point", Type)
	assertClass(t, lx, "const E = error{ NotFound };", "E", Type)
	assertClass(t, lx, "const nf = error.NotFound;", "nf", Plain)

	// A multiline string is a run of \\ lines; // in one is text.
	doc := "const s =\n    \\\\first \"q\"\n    \\\\second // not a comment\n;\n"
	assertInDoc(t, lx, doc, 1, `"q"`, String)
	assertInDoc(t, lx, doc, 2, "// not a comment", String)
	assertInDoc(t, lx, doc, 3, ";", Punctuation)
}

func TestD(t *testing.T) {
	assertDetects(t, "d", "app.d", "app.di")
	assertShebang(t, "d", "#!/usr/bin/env rdmd")
	lx := lang(t, "d")
	assertCommentsWith(t, lx, "//", "")

	assertClass(t, lx, "@safe void main() {", "@safe", Keyword)
	assertClass(t, lx, "@safe void main() {", "void", Type)
	assertClass(t, lx, "@safe void main() {", "main", Function)
	assertClass(t, lx, "foreach (i; 0 .. n) {", "foreach", Keyword)
	assertClass(t, lx, "class Foo : Bar {", "Foo", Type)
	assertClass(t, lx, "if (p is null) return;", "null", Constant)
	assertClass(t, lx, "writeln(__LINE__);", "__LINE__", Constant)
	assertSpanCovers(t, lx, "#!/usr/bin/env rdmd", "#!/usr/bin/env rdmd", Comment)

	// Strings: escaped, WYSIWYG and delimited.
	assertSpanCovers(t, lx, `s = "a\"b" ~ x;`, `"a\"b"`, String)
	assertSpanCovers(t, lx, "s = `C:\\dir` ~ x;", "`C:\\dir`", String)
	assertSpanCovers(t, lx, `s = r"C:\dir" ~ x;`, `r"C:\dir"`, String)
	assertSpanCovers(t, lx, `s = q"(foo(bar))" ~ x;`, `q"(foo(bar))"`, String)
	assertSpanCovers(t, lx, `s = q"[a]" ~ x;`, `q"[a]"`, String)
	assertSpanCovers(t, lx, `s = q"/slash/" ~ x;`, `q"/slash/"`, String)
	assertSpanCovers(t, lx, "char c = 'x';", "'x'", String)

	assertSpanCovers(t, lx, "n = 0xFF_FFUL;", "0xFF_FFUL", Number)
	assertSpanCovers(t, lx, "n = 1_000;", "1_000", Number)
	assertSpanCovers(t, lx, "f = 3.14f;", "3.14f", Number)
	assertSpanCovers(t, lx, "x = 1; // a note", "// a note", Comment)

	// /+ +/ nests.
	assertClass(t, lx, "/+ a /+ b +/ still +/ int x;", "still", Comment)
	assertClass(t, lx, "/+ a /+ b +/ still +/ int x;", "int", Type)
	doc := "/+ open\n/+ inner +/ still\n+/ int x;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 2, "int", Type)

	doc = "/* open\nstill */ int x;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "int", Type)

	// Strings run over lines.
	doc = "s = \"line\nstill\"; int x;\n"
	assertInDoc(t, lx, doc, 1, "still", String)
	assertInDoc(t, lx, doc, 1, "int", Type)
	doc = "s = `raw\nstill`; int x;\n"
	assertInDoc(t, lx, doc, 1, "still", String)
	assertInDoc(t, lx, doc, 1, "int", Type)
	doc = "s = q\"(open\nstill)\"; int x;\n"
	assertInDoc(t, lx, doc, 1, "still", String)
	assertInDoc(t, lx, doc, 1, "int", Type)

	// A here-document ends only at its own word at the start of a line.
	doc = "s = q\"EOS\ntext EOS\" inside\nEOS\"; int x;\n"
	assertInDoc(t, lx, doc, 1, "inside", String)
	assertInDoc(t, lx, doc, 2, "EOS", String)
	assertInDoc(t, lx, doc, 2, "int", Type)
}

func TestPascal(t *testing.T) {
	assertDetects(t, "pascal", "unit1.pas", "fpc.pp", "project.dpr", "project.lpr", "pkg.dpk")
	lx := lang(t, "pascal")
	assertCommentsWith(t, lx, "//", "")

	assertClass(t, lx, "begin", "begin", Keyword)
	assertClass(t, lx, "BEGIN END;", "BEGIN", Keyword)
	assertClass(t, lx, "S: String;", "String", Type)
	assertClass(t, lx, "X: integer;", "integer", Type)
	assertClass(t, lx, "if P = nil then Exit;", "nil", Constant)
	assertClass(t, lx, "Done := True;", "True", Constant)
	assertClass(t, lx, "WriteLn(S);", "WriteLn", Function)
	assertClass(t, lx, "procedure Greet;", "Greet", Function)

	assertSpanCovers(t, lx, "S := 'it''s' + T;", "'it''s'", String)
	assertSpanCovers(t, lx, "S := 'a' + #13#10;", "#13#10", String)
	assertSpanCovers(t, lx, "S := 'a' + #$0D;", "#$0D", String)
	assertSpanCovers(t, lx, "I := $FF;", "$FF", Number)
	assertSpanCovers(t, lx, "I := %1010;", "%1010", Number)
	assertSpanCovers(t, lx, "I := &17;", "&17", Number)
	assertSpanCovers(t, lx, "R := 1.5E3;", "1.5E3", Number)
	assertSpanCovers(t, lx, "for I := 1 to 10 do", "10", Number)
	assertSpanCovers(t, lx, "X := 1; // a note", "// a note", Comment)
	assertSpanCovers(t, lx, "X := 1; { a note }", "{ a note }", Comment)
	assertSpanCovers(t, lx, "{$mode objfpc}", "{$mode objfpc}", Keyword)

	// A method's body and a type section's declarations.
	assertClass(t, lx, "procedure TList.Add(const Item: T);", "TList", Type)
	assertClass(t, lx, "procedure TList.Add(const Item: T);", "Add", Function)
	assertClass(t, lx, "  TPoint = record", "TPoint", Type)
	assertClass(t, lx, "  TColor = (Red, Green);", "TColor", Type)
	assertClass(t, lx, "  TShape = class(TObject)", "TShape", Type)

	// Both block comments run over lines.
	doc := "{ open\nstill } X := 1;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "1", Number)
	doc = "(* open\nstill *) X := 1;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "1", Number)
}

func TestFortran(t *testing.T) {
	assertDetects(t, "fortran", "old.f", "old.for", "old.ftn", "old.f77", "new.f90", "new.f95",
		"new.f03", "new.f08", "pre.fpp", "MAIN.F90")
	lx := lang(t, "fortran")
	assertCommentsWith(t, lx, "!", "")

	assertClass(t, lx, "program demo", "program", Keyword)
	assertClass(t, lx, "IMPLICIT NONE", "NONE", Keyword)
	assertClass(t, lx, "integer :: n", "integer", Type)
	assertClass(t, lx, "REAL(dp) :: x", "REAL", Type)
	assertClass(t, lx, "logical :: ok = .true.", ".true.", Constant)
	assertClass(t, lx, "ok = .FALSE.", ".FALSE.", Constant)
	assertSpanCovers(t, lx, "if (i.eq.j) then", ".eq.", Operator)
	assertSpanCovers(t, lx, "if (a .and. b) then", ".and.", Operator)

	assertSpanCovers(t, lx, "s = 'it''s'", "'it''s'", String)
	assertSpanCovers(t, lx, `s = "say ""hi"""`, `"say ""hi"""`, String)
	assertSpanCovers(t, lx, "x = 1 ! a note", "! a note", Comment)
	assertSpanCovers(t, lx, "s = 'a ! b'", "'a ! b'", String)

	// Reals with d exponents and kinds; a dot then a letter is an operator's.
	assertSpanCovers(t, lx, "x = 1.0d0", "1.0d0", Number)
	assertSpanCovers(t, lx, "x = 2.5_dp", "2.5_dp", Number)
	assertSpanCovers(t, lx, "x = .5", ".5", Number)
	assertSpanCovers(t, lx, "x = 1.e5", "1.e5", Number)
	assertSpanCovers(t, lx, "if (1.eq.i) then", "1", Number)
	assertSpanCovers(t, lx, "n = B'1010'", "B'1010'", Number)
	assertSpanCovers(t, lx, "n = Z'FF'", "Z'FF'", Number)

	// Only intrinsics and declared names are functions: a(i) is an array.
	assertClass(t, lx, "y = sqrt(x) + a(i)", "sqrt", Function)
	assertClass(t, lx, "y = sqrt(x) + a(i)", "a", Plain)
	assertClass(t, lx, "call compute(x)", "compute", Function)
	assertClass(t, lx, "subroutine solve(n)", "solve", Function)
	assertClass(t, lx, "end function area", "area", Function)
	assertClass(t, lx, "type, public :: point", "point", Type)
	assertClass(t, lx, "end type point", "point", Type)
	assertClass(t, lx, "#ifdef DEBUG", "#ifdef", Keyword)

	// Fixed form's column-one comments, and free-form lines that start
	// with c and are code.
	assertSpanCovers(t, lx, "C     A fixed-form comment", "C     A fixed-form comment", Comment)
	assertSpanCovers(t, lx, "c-----", "c-----", Comment)
	assertSpanCovers(t, lx, "*     Another", "*     Another", Comment)
	assertSpanCovers(t, lx, "C", "C", Comment)
	assertClass(t, lx, "c = a + b", "c", Plain)
	assertClass(t, lx, "c(i) = 2", "c", Plain)
	assertClass(t, lx, "c%x = 3", "c", Plain)
	assertClass(t, lx, "call compute(x)", "call", Keyword)
	assertClass(t, lx, "character(len=10) :: s", "character", Type)
	assertClass(t, lx, "contains", "contains", Keyword)

	// A string does not run on past its line.
	if _, st := lx.Lex([]rune("s = 'open"), 0); st != 0 {
		t.Errorf("an unclosed string carried state %v to the next line", st)
	}
}

func TestAsm(t *testing.T) {
	assertDetects(t, "asm", "boot.asm", "start.s", "start.S", "lib.nasm")
	lx := lang(t, "asm")
	assertCommentsWith(t, lx, ";", "")

	// The first word of a line is the instruction or directive; registers
	// are types, in any case.
	assertClass(t, lx, "    mov eax, 1", "mov", Keyword)
	assertClass(t, lx, "    mov eax, 1", "eax", Type)
	assertClass(t, lx, "    MOV EAX, 1", "EAX", Type)
	assertClass(t, lx, "    mov r8d, [rel msg]", "r8d", Type)
	assertClass(t, lx, "    mov r8d, [rel msg]", "rel", Keyword)
	assertClass(t, lx, "    movdqa xmm0, [rsp+16]", "xmm0", Type)
	assertClass(t, lx, "    mov dword [ebx], 0", "dword", Type)
	assertClass(t, lx, "    ldr x0, [sp, #16]", "x0", Type)
	assertClass(t, lx, "    addi t0, s11, 1", "s11", Type)
	assertClass(t, lx, "    b.ne done", "b.ne", Keyword)

	// Directives, NASM's and GNU's.
	assertClass(t, lx, "section .text", "section", Keyword)
	assertClass(t, lx, "section .text", ".text", Keyword)
	assertClass(t, lx, "    .globl main", ".globl", Keyword)
	assertClass(t, lx, "%define SYS_WRITE 1", "%define", Keyword)
	assertClass(t, lx, "    .type main, @function", "@function", Constant)
	assertClass(t, lx, "#include <asm.h>", "#include", Keyword)

	// Labels, with and without a colon.
	assertSpanCovers(t, lx, "_start:", "_start", Function)
	assertClass(t, lx, ".loop:  dec ecx", ".loop", Function)
	assertClass(t, lx, ".loop:  dec ecx", "dec", Keyword)
	assertClass(t, lx, "%%skip:", "%%skip", Function)
	assertClass(t, lx, "1:  jmp 1b", "jmp", Keyword)
	assertClass(t, lx, `msg     db "Hello", 10`, "msg", Function)
	assertClass(t, lx, `msg     db "Hello", 10`, "db", Keyword)
	assertClass(t, lx, "len     equ $ - msg", "len", Function)
	assertClass(t, lx, "len     equ $ - msg", "$", Constant)

	// AT&T's % and $, ARM's #, and RISC-V's %hi().
	assertSpanCovers(t, lx, "    movl $1, %eax", "%eax", Type)
	assertSpanCovers(t, lx, "    movl $1, %eax", "$1", Number)
	assertSpanCovers(t, lx, "    mov r0, #0x10", "#0x10", Number)
	assertSpanCovers(t, lx, "    ldr x0, [sp, #-16]!", "#-16", Number)
	assertClass(t, lx, "    add w1, w2, #CONST", "CONST", Plain)
	assertSpanCovers(t, lx, "    lui a0, %hi(msg)", "%hi", Function)
	assertClass(t, lx, "    push %1", "%1", Constant)

	// Numbers, strings.
	assertSpanCovers(t, lx, "    mov al, 0FFh", "0FFh", Number)
	assertSpanCovers(t, lx, "    mov al, 0x1f", "0x1f", Number)
	assertSpanCovers(t, lx, `msg: .ascii "hi\n"`, `"hi\n"`, String)
	assertSpanCovers(t, lx, "    db 'it', 0", "'it'", String)
	assertSpanCovers(t, lx, "    movb $'a, %al", "'a", String)
	assertClass(t, lx, "    movb $'a, %al", "%al", Type)

	// Comments: ;, // and /* */ anywhere; # and @ at a line's start or
	// before a blank.
	assertSpanCovers(t, lx, "    mov eax, 1 ; a note", "; a note", Comment)
	assertSpanCovers(t, lx, "    movl $1, %eax  # set eax", "# set eax", Comment)
	assertSpanCovers(t, lx, "# GAS comment", "# GAS comment", Comment)
	assertSpanCovers(t, lx, "    add x0, x1, x2 // sum", "// sum", Comment)
	assertSpanCovers(t, lx, "    mov r1, r2  @ arm comment", "@ arm comment", Comment)
	assertSpanCovers(t, lx, `    db "a;b", 0`, `"a;b"`, String)
	doc := "/* open\nstill */ mov eax, 1\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "eax", Type)
}

func TestVerilog(t *testing.T) {
	assertDetects(t, "verilog", "top.v", "defs.vh", "top.sv", "pkg.svh")
	lx := lang(t, "verilog")
	assertCommentsWith(t, lx, "//", "")

	assertClass(t, lx, "module counter (", "module", Keyword)
	assertClass(t, lx, "module counter (", "counter", Type)
	assertClass(t, lx, "class Packet extends Base;", "Packet", Type)
	assertClass(t, lx, "always_ff @(posedge clk) begin", "always_ff", Keyword)
	assertClass(t, lx, "always_ff @(posedge clk) begin", "posedge", Keyword)
	assertClass(t, lx, "input wire clk,", "wire", Type)
	assertClass(t, lx, "output logic [7:0] q", "logic", Type)
	assertClass(t, lx, "if (h == null) return;", "null", Constant)
	assertClass(t, lx, "function automatic int twice(int a);", "twice", Function)

	assertSpanCovers(t, lx, `$display("q=%d\n", q);`, `"q=%d\n"`, String)
	assertSpanCovers(t, lx, `s = "a\"b";`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `$display("q=%d\n", q);`, "$display", Function)
	assertSpanCovers(t, lx, "q <= 0; // a note", "// a note", Comment)

	// Sized and based literals, unsized fills, and a cast's tick.
	assertSpanCovers(t, lx, "q <= 8'hFF;", "8'hFF", Number)
	assertSpanCovers(t, lx, "q <= 4'b10_1z;", "4'b10_1z", Number)
	assertSpanCovers(t, lx, "q <= 'd10;", "'d10", Number)
	assertSpanCovers(t, lx, "q <= 16'sh7fff;", "16'sh7fff", Number)
	assertSpanCovers(t, lx, "q <= 12 'o777;", "12 'o777", Number)
	assertSpanCovers(t, lx, "q <= '1;", "'1", Number)
	assertSpanCovers(t, lx, "q <= 42;", "42", Number)
	assertClass(t, lx, "x = int'(y);", "int", Type)
	assertClass(t, lx, "x = int'(y);", "'", Plain)

	// Compiler directives and macros.
	assertSpanCovers(t, lx, "`timescale 1ns/1ps", "`timescale", Keyword)
	assertSpanCovers(t, lx, "`timescale 1ns/1ps", "1ns", Number)
	assertSpanCovers(t, lx, "`define WIDTH 8", "`define", Keyword)
	assertSpanCovers(t, lx, "parameter W = `WIDTH;", "`WIDTH", Function)

	doc := "/* open\nstill */ wire x;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "wire", Type)
}

func TestVHDL(t *testing.T) {
	assertDetects(t, "vhdl", "top.vhd", "top.vhdl", "TOP.VHD")
	lx := lang(t, "vhdl")
	assertCommentsWith(t, lx, "--", "")

	assertClass(t, lx, "entity counter is", "entity", Keyword)
	assertClass(t, lx, "entity counter is", "counter", Type)
	assertClass(t, lx, "END PROCESS;", "PROCESS", Keyword)
	assertClass(t, lx, "architecture rtl of counter is", "rtl", Type)
	assertClass(t, lx, "function parity (v : bit_vector) return bit is", "parity", Function)
	assertClass(t, lx, "signal q : std_logic_vector(7 downto 0);", "std_logic_vector", Type)
	assertClass(t, lx, "signal Q : STD_LOGIC;", "STD_LOGIC", Type)
	assertClass(t, lx, "constant ok : boolean := true;", "true", Constant)

	assertSpanCovers(t, lx, `report "say ""hi""" & x;`, `"say ""hi"""`, String)
	assertSpanCovers(t, lx, "q <= '1'; -- a note", "-- a note", Comment)
	assertSpanCovers(t, lx, "q <= '1'; -- a note", "'1'", String)

	// Bit strings and based literals.
	assertSpanCovers(t, lx, `c <= X"FF";`, `X"FF"`, Number)
	assertSpanCovers(t, lx, `c <= B"1010";`, `B"1010"`, Number)
	assertSpanCovers(t, lx, `c <= 8UX"0F";`, `8UX"0F"`, Number)
	assertSpanCovers(t, lx, "k := 16#FF#;", "16#FF#", Number)
	assertSpanCovers(t, lx, "k := 1_000;", "1_000", Number)

	// The tick: attributes, character literals, qualified expressions.
	assertSpanCovers(t, lx, "if clk'event and clk = '1' then", "event", Function)
	assertClass(t, lx, "if clk'event and clk = '1' then", "and", Keyword)
	assertSpanCovers(t, lx, "if clk'event and clk = '1' then", "'1'", String)
	assertClass(t, lx, "report integer'image(k);", "image", Function)
	assertClass(t, lx, `q <= std_logic_vector'("0101");`, "std_logic_vector", Type)
	assertSpanCovers(t, lx, `q <= std_logic_vector'("0101");`, `"0101"`, String)

	// Slices are not calls; the standard functions are functions.
	assertClass(t, lx, "x <= data(7 downto 0);", "data", Plain)
	assertClass(t, lx, "if rising_edge(clk) then", "rising_edge", Function)

	doc := "/* open\nstill */ signal x : bit;\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 1, "signal", Keyword)
}

func TestOdin(t *testing.T) {
	assertDetects(t, "odin", "main.odin")
	lx := lang(t, "odin")
	assertCommentsWith(t, lx, "//", "")

	assertClass(t, lx, "for i in 0..<n {", "for", Keyword)
	assertClass(t, lx, "defer free(p)", "defer", Keyword)
	assertClass(t, lx, "x: int = 0", "int", Type)
	assertClass(t, lx, "v: [3]f32", "f32", Type)
	assertClass(t, lx, "if p == nil { return }", "nil", Constant)
	assertClass(t, lx, "fmt.println(x)", "println", Function)

	// A :: declaration is a procedure, a type or a constant.
	assertClass(t, lx, "main :: proc() {", "main", Function)
	assertClass(t, lx, "main :: proc() {", "proc", Keyword)
	assertClass(t, lx, `helper :: #force_inline proc "c" () {`, "helper", Function)
	assertClass(t, lx, "Point :: struct { x, y: int }", "Point", Type)
	assertClass(t, lx, "Handle :: distinct int", "Handle", Type)
	assertClass(t, lx, "MAX :: 64", "MAX", Constant)

	// Directives, attributes, parameter types and ---.
	assertSpanCovers(t, lx, "#partial switch v in u {", "#partial", Keyword)
	assertSpanCovers(t, lx, "@(private)", "@(private", Keyword)
	assertSpanCovers(t, lx, "make :: proc($T: typeid) -> T", "$T", Type)
	assertSpanCovers(t, lx, "x: int = ---", "---", Constant)

	assertSpanCovers(t, lx, `fmt.println("hi\n")`, `"hi\n"`, String)
	assertSpanCovers(t, lx, "r := '\\n'", "'\\n'", String)
	assertSpanCovers(t, lx, "x := 1 // a note", "// a note", Comment)
	assertSpanCovers(t, lx, "x := 0x1F + y", "0x1F", Number)
	assertSpanCovers(t, lx, "x := 1_000 + y", "1_000", Number)
	assertSpanCovers(t, lx, "x := 1.5e3 + y", "1.5e3", Number)

	// Block comments nest; raw strings run over lines.
	assertClass(t, lx, "/* a /* b */ still */ x := 1", "still", Comment)
	doc := "/* open\n/* inner */ still\n*/ x := 1\n"
	assertInDoc(t, lx, doc, 1, "still", Comment)
	assertInDoc(t, lx, doc, 2, "1", Number)
	doc = "s := `raw \\n\nstill` + t\n"
	assertInDoc(t, lx, doc, 0, `\n`, String)
	assertInDoc(t, lx, doc, 1, "still", String)
	assertInDoc(t, lx, doc, 1, "+", Operator)
}
