package syntax

import "testing"

// Tests for the languages of the JVM, .NET and Apple's platforms, and Dart
// and Visual Basic beside them: each file form a definition claims, what it
// colours, and every construct that runs over lines.

// jvmDetect checks that each path is taken for the language want.
func jvmDetect(t *testing.T, want string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if got := For(p).Name(); got != want {
			t.Errorf("For(%q) = %q, want %q", p, got, want)
		}
	}
}

// jvmShebang checks that a script with no extension and this first line is
// taken for the language want.
func jvmShebang(t *testing.T, want, first string) {
	t.Helper()
	if got := ForWithHeader("script", first).Name(); got != want {
		t.Errorf("ForWithHeader(script, %q) = %q, want %q", first, got, want)
	}
}

// jvmComment checks the comment M-; writes: the first a definition lists.
func jvmComment(t *testing.T, lx *Language, wantStart string) {
	t.Helper()
	start, end, ok := lx.Comment()
	if !ok || start != wantStart || end != "" {
		t.Errorf("%s comments with %q %q (%v), want %q", lx.Name(), start, end, ok, wantStart)
	}
}

// jvmSpot is a place in a document: the one occurrence of sub on a line, and
// the class it must have.
type jvmSpot struct {
	line int
	sub  string
	want Class
}

// jvmDoc lexes a document a line at a time, carrying the state from each line
// to the next as the editor does, and checks each spot.
func jvmDoc(t *testing.T, lx Lexer, doc string, spots ...jvmSpot) {
	t.Helper()
	lines, spans := lexDoc(lx, doc)
	for i, l := range lines {
		checkSpans(t, lx.Name()+" "+string(l), l, spans[i])
	}
	for _, s := range spots {
		src := string(lines[s.line])
		at := len([]rune(src[:indexOnce(t, src, s.sub)]))
		if got := classAt(spans[s.line], at); got != s.want {
			t.Errorf("%s line %d %q: %q is %v, want %v (spans: %s)",
				lx.Name(), s.line, src, s.sub, got, s.want, describe(lines[s.line], spans[s.line]))
		}
	}
}

// jvmUnterminated lexes lines that open something and never close it, as a
// file being typed does, and checks the spans are still well formed.
func jvmUnterminated(t *testing.T, lx Lexer, lines ...string) {
	t.Helper()
	for _, s := range lines {
		line := []rune(s)
		spans, _ := lx.Lex(line, 0)
		checkSpans(t, lx.Name()+" "+s, line, spans)
	}
}

func TestJava(t *testing.T) {
	jvmDetect(t, "java", "Main.java", "/src/App.JAVA")
	lx := lang(t, "java")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "public static void main(String[] args) {", "public", Keyword)
	assertClass(t, lx, "public static void main(String[] args) {", "void", Type)
	assertClass(t, lx, "public static void main(String[] args) {", "String", Type)
	assertClass(t, lx, "public static void main(String[] args) {", "main", Function)
	assertClass(t, lx, "if (x == null) return true;", "null", Constant)
	assertClass(t, lx, "if (x == null) return true;", "true", Constant)
	assertClass(t, lx, "System.out.println(x);", "System", Type)
	assertClass(t, lx, "System.out.println(x);", "println", Function)
	assertSpanCovers(t, lx, `s = "a\"b";`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `c = '\'';`, `'\''`, String)
	assertSpanCovers(t, lx, "x = 1; // note", "// note", Comment)
	assertSpanCovers(t, lx, "x = /* c */ 1;", "/* c */", Comment)
	for _, n := range []string{"1_000_000L", "0x1F", "0b1010", "3.14e-2f", "1e9d", ".5"} {
		assertSpanCovers(t, lx, "x = "+n+";", n, Number)
	}
	// $ is a letter in a Java name.
	assertClass(t, lx, "int $count = 1;", "$count", Plain)

	// Annotations, and @interface declaring one.
	assertSpanCovers(t, lx, "@Override", "@Override", Function)
	assertSpanCovers(t, lx, `@SuppressWarnings("unchecked")`, "@SuppressWarnings", Function)
	assertSpanCovers(t, lx, "public @interface Marker {}", "@interface", Keyword)
	assertClass(t, lx, "public @interface Marker {}", "Marker", Type)

	// The names that declarations declare.
	assertClass(t, lx, "public class Main<T> extends Base {", "Main", Type)
	assertClass(t, lx, "sealed interface Shape permits Circle {}", "Shape", Type)
	assertClass(t, lx, "sealed interface Shape permits Circle {}", "permits", Keyword)
	assertClass(t, lx, "enum Color { RED }", "Color", Type)
	assertSpanCovers(t, lx, "public non-sealed class Square {}", "non-sealed", Keyword)
	// record is a keyword where it declares one, and a name elsewhere.
	assertClass(t, lx, "record Point(int x, int y) {}", "record", Keyword)
	assertClass(t, lx, "record Point(int x, int y) {}", "Point", Type)
	assertClass(t, lx, "for (var record : rows) {", "record", Plain)
	assertClass(t, lx, "process(record);", "record", Plain)
	// .class is a keyword, and what follows it is not a declared type.
	assertClass(t, lx, "Object o = Foo.class;", "class", Keyword)

	// A text block runs over lines, and escapes still work in it.
	jvmDoc(t, lx, "String s = \"\"\"\n    Hello \"there\" \\\"\"\"\n    still // text\n    \"\"\"; int after = 1;\n",
		jvmSpot{0, `"""`, String},
		jvmSpot{1, `"there"`, String},
		jvmSpot{1, `\"""`, String},
		jvmSpot{2, "// text", String},
		jvmSpot{3, `""";`, String},
		jvmSpot{3, "int", Type},
		jvmSpot{3, "1", Number},
	)
	jvmDoc(t, lx, "/* open\n   still */ int x;\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "int", Type},
	)
	// A plain string does not run on.
	jvmDoc(t, lx, "s = \"open\nint x;\n", jvmSpot{1, "int", Type})
	jvmUnterminated(t, lx, `s = """`, `"`, `'`, "@", "/*", "record", "non-")
}

func TestKotlin(t *testing.T) {
	jvmDetect(t, "kotlin", "Main.kt", "build.gradle.kts", "script.KTS")
	lx := lang(t, "kotlin")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "val x: Int = when (y) { else -> 0 }", "val", Keyword)
	assertClass(t, lx, "val x: Int = when (y) { else -> 0 }", "when", Keyword)
	assertClass(t, lx, "val x: Int = when (y) { else -> 0 }", "Int", Type)
	assertClass(t, lx, "val s: String? = null", "null", Constant)
	assertClass(t, lx, "val xs = listOf(1, 2)", "listOf", Function)
	assertClass(t, lx, "x?.let { println(it) }", "let", Function)
	assertSpanCovers(t, lx, `s = "a\"b"`, `"a\"b"`, String)
	// A template stays inside its string.
	assertSpanCovers(t, lx, `s = "Hi $name, ${name.length}!"`, `"Hi $name, ${name.length}!"`, String)
	assertSpanCovers(t, lx, `c = '\n'`, `'\n'`, String)
	assertSpanCovers(t, lx, "x = 1 // note", "// note", Comment)
	for _, n := range []string{"1_000L", "0xFF", "0b1010", "2u", "1.5f", "6.02e23"} {
		assertSpanCovers(t, lx, "val x = "+n+" + y", n, Number)
	}
	assertSpanCovers(t, lx, "#!/usr/bin/env kotlin", "#!/usr/bin/env kotlin", Comment)

	// Declarations.
	assertClass(t, lx, "fun greet(n: String) = n", "greet", Function)
	assertClass(t, lx, "fun String.shout() = uppercase()", "shout", Function)
	assertClass(t, lx, "object Registry", "Registry", Type)
	assertClass(t, lx, "typealias Names = List<String>", "Names", Type)
	assertClass(t, lx, "fun interface Action { fun run() }", "Action", Type)
	// data, value and the like are keywords only before what they modify.
	assertClass(t, lx, "data class User(val name: String)", "data", Keyword)
	assertClass(t, lx, "data class User(val name: String)", "User", Type)
	assertClass(t, lx, "value class Id(val v: Long)", "value", Keyword)
	assertClass(t, lx, "val data = load()", "data", Plain)
	assertClass(t, lx, "sealed class Box<out T>", "out", Keyword)
	assertClass(t, lx, "System.out.println(x)", "out", Plain)

	// Annotations, with use-site targets, and labels on jumps.
	assertSpanCovers(t, lx, `@Suppress("UNUSED") fun f() {}`, "@Suppress", Function)
	assertSpanCovers(t, lx, `@file:JvmName("Main")`, "@file:JvmName", Function)
	assertSpanCovers(t, lx, "xs.forEach { if (it < 0) return@forEach }", "return@forEach", Keyword)
	assertSpanCovers(t, lx, "val o = this@Outer", "this@Outer", Keyword)

	// Block comments nest.
	jvmDoc(t, lx, "/* outer /* inner */\nstill outer */ val x = 1\n",
		jvmSpot{0, "inner", Comment},
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "val", Keyword},
	)
	// A raw string runs over lines, has no escapes, and ends at the last
	// quote of a run: a string left open by a stray quote would take the +.
	jvmDoc(t, lx, "val s = \"\"\"C:\\path \"q\"\nsecond\"\"\"\" + x\nval t = \"\"\"a\\\"\"\" + 1\n",
		jvmSpot{0, `C:\path`, String},
		jvmSpot{1, "second", String},
		jvmSpot{1, "+", Operator},
		jvmSpot{2, "+", Operator},
		jvmSpot{2, "1", Number},
	)
	jvmUnterminated(t, lx, `s = """`, `"`, `'`, "/* /*", "@", "return@", "@file:")
}

func TestScala(t *testing.T) {
	jvmDetect(t, "scala", "Main.scala", "build.sc")
	jvmShebang(t, "scala", "#!/usr/bin/env scala")
	lx := lang(t, "scala")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "def area(r: Double): Double = r * r", "def", Keyword)
	assertClass(t, lx, "def area(r: Double): Double = r * r", "area", Function)
	assertClass(t, lx, "val xs: List[Int] = Nil", "Int", Type)
	assertClass(t, lx, "val xs: List[Int] = Nil", "Nil", Constant)
	assertClass(t, lx, "x match { case None => 0 }", "match", Keyword)
	assertClass(t, lx, "x match { case None => 0 }", "None", Constant)
	assertClass(t, lx, `println("hi")`, "println", Function)
	assertSpanCovers(t, lx, `s = "a\"b"`, `"a\"b"`, String)
	assertSpanCovers(t, lx, "x = 1 // note", "// note", Comment)
	for _, n := range []string{"1_000L", "0xFF", "1.5e3", "2.0f"} {
		assertSpanCovers(t, lx, "val x = "+n+" + y", n, Number)
	}

	// Interpolators own the string after them.
	assertSpanCovers(t, lx, `val m = s"Hi $name\n" + x`, `s"Hi $name\n"`, String)
	assertSpanCovers(t, lx, `val m = f"$x%.2f" + y`, `f"$x%.2f"`, String)
	// A character, and Scala 2's symbol, which does not close.
	assertSpanCovers(t, lx, "val c = 'a'", "'a'", String)
	assertSpanCovers(t, lx, `val c = '\''`, `'\''`, String)
	assertSpanCovers(t, lx, "val s = 'name", "'name", Constant)

	// Declarations.
	assertClass(t, lx, "case class Point(x: Int)", "Point", Type)
	assertClass(t, lx, "sealed trait Shape", "Shape", Type)
	assertClass(t, lx, "object Main extends App", "Main", Type)
	assertClass(t, lx, "type Id = Long", "Id", Type)
	assertClass(t, lx, "enum Color { case Red }", "Color", Type)
	assertSpanCovers(t, lx, "@tailrec def go(n: Int) = n", "@tailrec", Function)
	assertSpanCovers(t, lx, "val `type` = 1", "`type`", Plain)
	// end and open are keywords only where Scala 3 uses them as one.
	assertClass(t, lx, "  end Widget", "end", Keyword)
	assertClass(t, lx, "val end = 3", "end", Plain)
	assertClass(t, lx, "open class Widget", "open", Keyword)
	assertClass(t, lx, "file.open()", "open", Function)

	jvmDoc(t, lx, "/* outer /* inner */\nstill outer */ val x = 1\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "val", Keyword},
	)
	// A triple-quoted string, interpolated or not, runs over lines and ends
	// at the last quote of a run.
	jvmDoc(t, lx, "val q = s\"\"\"multi $x\nline \"quoted\"\"\"\" + 1\nval r = \"\"\"a\\\"\"\" + 2\n",
		jvmSpot{0, `s"""`, String},
		jvmSpot{1, `"quoted`, String},
		jvmSpot{1, "+", Operator},
		jvmSpot{2, "+", Operator},
	)
	jvmUnterminated(t, lx, `s"""`, `s"`, `"`, "'", "'\\", "`", "/*")
}

func TestGroovy(t *testing.T) {
	jvmDetect(t, "groovy", "App.groovy", "build.gradle", "x.gvy", "Jenkinsfile", "/repo/jenkinsfile")
	jvmShebang(t, "groovy", "#!/usr/bin/env groovy")
	lx := lang(t, "groovy")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "def greet(String who) { who }", "def", Keyword)
	assertClass(t, lx, "def greet(String who) { who }", "greet", Function)
	assertClass(t, lx, "def greet(String who) { who }", "String", Type)
	assertClass(t, lx, "if (x == null) return", "null", Constant)
	assertClass(t, lx, `println "hi"`, "println", Function)
	assertSpanCovers(t, lx, `s = 'it\'s'`, `'it\'s'`, String)
	assertSpanCovers(t, lx, `g = "Hi ${name}"`, `"Hi ${name}"`, String)
	assertSpanCovers(t, lx, "x = 1 // note", "// note", Comment)
	for _, n := range []string{"1_000G", "1.5g", "0x1F", "10L"} {
		assertSpanCovers(t, lx, "def n = "+n+" + y", n, Number)
	}
	assertSpanCovers(t, lx, "#!/usr/bin/env groovy", "#!/usr/bin/env groovy", Comment)

	assertClass(t, lx, "trait Named { }", "Named", Type)
	assertClass(t, lx, "class Person implements Named {", "Person", Type)
	assertSpanCovers(t, lx, "@Grab('org.x:y:1.0')", "@Grab", Function)

	// A slashy string, where a / cannot be a division.
	assertSpanCovers(t, lx, `def m = s =~ /a\/b\d+/`, `/a\/b\d+/`, String)
	assertSpanCovers(t, lx, `def m = s =~ /a\/b\d+/`, "=~", Operator)
	assertSpanCovers(t, lx, "def ok = x ==~ /[a-z]+/", "/[a-z]+/", String)
	assertSpanCovers(t, lx, "def p = ~/pat/", "/pat/", String)
	assertSpanCovers(t, lx, "xs.find(/y/)", "/y/", String)
	// And a division, and a comment, where one could be.
	assertClass(t, lx, "def half = total / 2 / count", "/ 2", Operator)
	assertClass(t, lx, "def half = total / 2 / count", "2", Number)
	assertSpanCovers(t, lx, "foo(/* note */ a)", "/* note */", Comment)
	assertSpanCovers(t, lx, "foo(a, // note", "// note", Comment)

	jvmDoc(t, lx, "def t = '''multi\nline''' + 1\ndef g = \"\"\"a ${x}\nb\"\"\" + 2\n",
		jvmSpot{0, "'''multi", String},
		jvmSpot{1, "line", String},
		jvmSpot{1, "+", Operator},
		jvmSpot{2, "a ${x}", String},
		jvmSpot{3, "b", String},
		jvmSpot{3, "+", Operator},
	)
	// A dollar-slashy string runs over lines, and $/ inside it is a slash.
	jvmDoc(t, lx, "def ds = $/one $/ two\nthree/$ + 1\n",
		jvmSpot{0, "$/ two", String},
		jvmSpot{1, "three", String},
		jvmSpot{1, "+", Operator},
	)
	jvmDoc(t, lx, "/* open\nstill */ def x\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "def", Keyword},
	)
	jvmUnterminated(t, lx, "$/", "~/", "=~ /", "'''", `"""`, "'", `"`, "/*")
}

func TestCSharp(t *testing.T) {
	jvmDetect(t, "csharp", "Program.cs", "script.csx")
	lx := lang(t, "csharp")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "public async Task<int> RunAsync() {", "public", Keyword)
	assertClass(t, lx, "public async Task<int> RunAsync() {", "async", Keyword)
	assertClass(t, lx, "public async Task<int> RunAsync() {", "int", Type)
	assertClass(t, lx, "public async Task<int> RunAsync() {", "RunAsync", Function)
	assertClass(t, lx, "if (x == null) return true;", "null", Constant)
	assertClass(t, lx, "if (x == null) return true;", "true", Constant)
	assertClass(t, lx, "string s = name;", "string", Type)
	assertSpanCovers(t, lx, `s = "a\"b";`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `c = '\'';`, `'\''`, String)
	assertSpanCovers(t, lx, "x = 1; // note", "// note", Comment)
	for _, n := range []string{"1_000.5m", "0xFFUL", "0b1010", "1.5f", "10L"} {
		assertSpanCovers(t, lx, "x = "+n+";", n, Number)
	}

	// Verbatim, interpolated and raw strings.
	assertSpanCovers(t, lx, `v = @"C:\dir\""q""";`, `@"C:\dir\""q"""`, String)
	assertSpanCovers(t, lx, `i = $"Hi {name}\n";`, `$"Hi {name}\n"`, String)
	assertSpanCovers(t, lx, `vi = $@"{a}""b""";`, `$@"{a}""b"""`, String)
	assertSpanCovers(t, lx, `ri = $$"""{{x}} "q" {y}""";`, `$$"""{{x}} "q" {y}"""`, String)
	assertSpanCovers(t, lx, `r = """"has """ inside"""";`, `""""has """ inside""""`, String)
	assertSpanCovers(t, lx, `e = "";`, `""`, String)

	// Preprocessor lines, attributes and @names.
	assertSpanCovers(t, lx, "#region Setup", "#region", Keyword)
	assertClass(t, lx, "  #if DEBUG", "#if", Keyword)
	assertSpanCovers(t, lx, "#endregion", "#endregion", Keyword)
	assertSpanCovers(t, lx, "#!/usr/bin/env dotnet", "#!/usr/bin/env dotnet", Comment)
	assertSpanCovers(t, lx, `[Obsolete("use Bar")]`, "Obsolete", Function)
	assertSpanCovers(t, lx, `[assembly: InternalsVisibleTo("T")]`, "assembly", Keyword)
	assertSpanCovers(t, lx, `[assembly: InternalsVisibleTo("T")]`, "InternalsVisibleTo", Function)
	assertSpanCovers(t, lx, "var @class = 1;", "@class", Plain)

	// Declarations; record only where it declares one.
	assertClass(t, lx, "public sealed class Foo : IBar {", "Foo", Type)
	assertClass(t, lx, "internal struct Point {", "Point", Type)
	assertClass(t, lx, "interface IBar { }", "IBar", Type)
	assertClass(t, lx, "enum E : byte { A }", "E", Type)
	assertClass(t, lx, "public record Person(string Name);", "record", Keyword)
	assertClass(t, lx, "public record Person(string Name);", "Person", Type)
	assertClass(t, lx, "record struct Pair<T>(T A, T B);", "struct", Keyword)
	assertClass(t, lx, "record struct Pair<T>(T A, T B);", "Pair", Type)
	assertClass(t, lx, "var record = Next();", "record", Plain)

	jvmDoc(t, lx, "var v = @\"one \"\"q\"\"\ntwo\"; var after = 1;\n",
		jvmSpot{0, "one", String},
		jvmSpot{1, "two", String},
		jvmSpot{1, "after", Plain},
		jvmSpot{1, "1", Number},
	)
	jvmDoc(t, lx, "var r = \"\"\"\n    raw \"text\" \\n\n    \"\"\"; var after = 1;\n",
		jvmSpot{1, `"text"`, String},
		jvmSpot{1, `\n`, String},
		jvmSpot{2, `""";`, String},
		jvmSpot{2, "var", Keyword},
	)
	jvmDoc(t, lx, "/* open\nstill */ int x;\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "int", Type},
	)
	jvmDoc(t, lx, "s = $\"open\nint x;\n", jvmSpot{1, "int", Type})
	jvmUnterminated(t, lx, `"""`, `$$"""`, `@"`, `$"`, `'`, "#", "[", "@", "record")
}

func TestFSharp(t *testing.T) {
	jvmDetect(t, "fsharp", "Program.fs", "Lib.fsi", "script.fsx")
	lx := lang(t, "fsharp")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "match x with | Some v -> v | None -> 0", "match", Keyword)
	assertClass(t, lx, "match x with | Some v -> v | None -> 0", "None", Constant)
	assertClass(t, lx, "let n: int = 0", "int", Type)
	assertClass(t, lx, "let ok = true", "true", Constant)
	assertClass(t, lx, `printfn "%d" n`, "printfn", Function)
	assertSpanCovers(t, lx, `let s = "a\"b"`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `let v = @"C:\dir ""q"""`, `@"C:\dir ""q"""`, String)
	assertSpanCovers(t, lx, `let r = """raw "q" \n"""`, `"""raw "q" \n"""`, String)
	assertSpanCovers(t, lx, `let i = $"{x} and {y}"`, `$"{x} and {y}"`, String)
	assertSpanCovers(t, lx, "x + 1 // note", "// note", Comment)
	assertSpanCovers(t, lx, "x + (* note *) 1", "(* note *)", Comment)
	for _, n := range []string{"0xFFuy", "1_000L", "1.5e3", "2.0m", "1I"} {
		assertSpanCovers(t, lx, "let x = "+n+" + y", n, Number)
	}

	// A character, a type variable, and a name with a prime.
	assertSpanCovers(t, lx, "let c = 'a'", "'a'", String)
	assertSpanCovers(t, lx, `let c = '\n'`, `'\n'`, String)
	assertClass(t, lx, "type Box<'T> = 'T list", "'T>", Type)
	assertClass(t, lx, "let acc' = acc + 1", "acc'", Plain)
	// (*) is the operator, not a comment.
	assertSpanCovers(t, lx, "List.fold (*) 1 xs", "(*)", Operator)
	assertClass(t, lx, "List.fold (*) 1 xs", "1", Number)

	// let names a function when a parameter follows the name.
	assertClass(t, lx, "let f x y = x + y", "f", Function)
	assertClass(t, lx, "let g () = 1", "g", Function)
	assertClass(t, lx, "let value = 5", "value", Plain)
	assertClass(t, lx, "let rec fact n = n", "rec", Keyword)
	assertClass(t, lx, "let rec fact n = n", "fact", Function)
	assertClass(t, lx, "let inline twice x = x", "twice", Function)
	assertClass(t, lx, "let mutable counter = 0", "mutable", Keyword)
	assertClass(t, lx, "let mutable counter = 0", "counter", Plain)
	// And member the member after its self identifier.
	assertClass(t, lx, "member this.Name = name", "Name", Function)
	assertClass(t, lx, "static member Create() = Person()", "Create", Function)
	assertClass(t, lx, "member val Age = 0 with get, set", "val", Keyword)
	assertClass(t, lx, "member val Age = 0 with get, set", "Age", Plain)
	assertClass(t, lx, "type Shape = Circle of float", "Shape", Type)
	assertClass(t, lx, "exception MyError of string", "MyError", Type)
	assertClass(t, lx, "[<EntryPoint>]", "EntryPoint", Function)
	assertSpanCovers(t, lx, `#r "nuget: FSharp.Data"`, "#r", Keyword)
	assertSpanCovers(t, lx, "#!/usr/bin/env -S dotnet fsi", "#!/usr/bin/env -S dotnet fsi", Comment)

	// Block comments nest, and a (*) inside one opens nothing.
	jvmDoc(t, lx, "(* outer (* inner *) and (*) still\nouter *) let x = 1\n",
		jvmSpot{0, "still", Comment},
		jvmSpot{1, "outer", Comment},
		jvmSpot{1, "let", Keyword},
	)
	// Every string may run over lines.
	jvmDoc(t, lx, "let s = \"multi\nline\" + 1\nlet r = \"\"\"a\nb \"q\" \\\"\"\" + 2\nlet v = @\"c\nd\"\"e\" + 3\n",
		jvmSpot{1, "line", String},
		jvmSpot{1, "+", Operator},
		jvmSpot{3, `"q"`, String},
		jvmSpot{3, "+", Operator},
		jvmSpot{5, `d""e"`, String},
		jvmSpot{5, "+", Operator},
	)
	jvmUnterminated(t, lx, "(*", "(* (*", "(*)", `"""`, `@"`, `$"`, "'", "'\\", "[<", "#")
}

func TestSwift(t *testing.T) {
	jvmDetect(t, "swift", "main.swift", "Package.swift")
	lx := lang(t, "swift")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "guard let x = y else { return nil }", "guard", Keyword)
	assertClass(t, lx, "guard let x = y else { return nil }", "nil", Constant)
	assertClass(t, lx, "var count: Int = 0", "Int", Type)
	assertClass(t, lx, `print("hi")`, "print", Function)
	assertSpanCovers(t, lx, `s = "a\"b"`, `"a\"b"`, String)
	assertSpanCovers(t, lx, `s = "Hi \(name)!"`, `"Hi \(name)!"`, String)
	assertSpanCovers(t, lx, "x = 1 // note", "// note", Comment)
	for _, n := range []string{"0x1p-2", "1_000", "0o17", "0b1010", "1e10"} {
		assertSpanCovers(t, lx, "let x = "+n+" + y", n, Number)
	}

	// Declarations.
	assertClass(t, lx, "func greet(_ name: String) -> String {", "greet", Function)
	assertClass(t, lx, "final class ViewModel: ObservableObject {", "ViewModel", Type)
	assertClass(t, lx, "struct Point { }", "Point", Type)
	assertClass(t, lx, "protocol Shape {}", "Shape", Type)
	assertClass(t, lx, "extension Point: Shape {}", "Point", Type)
	assertClass(t, lx, "actor Counter {}", "Counter", Type)
	// class var is a property of the class; class func a method of it.
	assertClass(t, lx, "class var shared: VM { VM() }", "shared", Plain)
	assertClass(t, lx, "class func make() -> VM {", "make", Function)
	// open and package are keywords only before a declaration.
	assertClass(t, lx, "open func run() {}", "open", Keyword)
	assertClass(t, lx, "UIApplication.shared.open(url)", "open", Function)
	assertClass(t, lx, `let package = Package(name: "x")`, "package", Plain)

	// Attributes, compiler directives and backquoted names.
	assertSpanCovers(t, lx, "@MainActor final class A {}", "@MainActor", Function)
	assertSpanCovers(t, lx, "#if os(macOS)", "#if", Keyword)
	assertSpanCovers(t, lx, "if #available(iOS 15, *) {", "#available", Keyword)
	assertSpanCovers(t, lx, "let `default` = 1", "`default`", Plain)
	assertSpanCovers(t, lx, "#!/usr/bin/swift", "#!/usr/bin/swift", Comment)

	// A raw string closes only at a quote and as many #s as it opened with.
	assertSpanCovers(t, lx, `let r = #"a "quoted" \n"# + x`, `#"a "quoted" \n"#`, String)
	assertSpanCovers(t, lx, `let r = ##"has "# inside"## + x`, `##"has "# inside"##`, String)
	assertClass(t, lx, `let r = ##"has "# inside"## + x`, "+", Operator)
	assertSpanCovers(t, lx, "let re = #/[a-z]+/# + x", "#/[a-z]+/#", String)

	jvmDoc(t, lx, "/* outer /* inner */\nstill outer */ let x = 1\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "let", Keyword},
	)
	jvmDoc(t, lx, "let m = \"\"\"\n    Hello \"there\"\n    \"\"\"\nlet r = #\"\"\"\n    raw \\n \"\"\" still\n    \"\"\"# + 1\n",
		jvmSpot{1, `"there"`, String},
		jvmSpot{2, `"""`, String},
		jvmSpot{3, "let", Keyword},
		jvmSpot{4, "still", String},
		jvmSpot{5, `"""#`, String},
		jvmSpot{5, "+", Operator},
	)
	jvmUnterminated(t, lx, `#"`, `##"""`, "#/", `"""`, `"`, "`", "@", "#")
}

func TestObjC(t *testing.T) {
	jvmDetect(t, "objc", "main.m", "View.mm")
	lx := lang(t, "objc")
	jvmComment(t, lx, "//")

	// C's words are Objective-C's too.
	assertClass(t, lx, "if (x) return NULL;", "if", Keyword)
	assertClass(t, lx, "if (x) return NULL;", "NULL", Constant)
	assertClass(t, lx, "static int n = 0x1F;", "int", Type)
	assertSpanCovers(t, lx, "static int n = 0x1F;", "0x1F", Number)
	assertSpanCovers(t, lx, "#import <Foundation/Foundation.h>", "#import", Keyword)
	assertSpanCovers(t, lx, `printf("a\"b");`, `"a\"b"`, String)
	assertSpanCovers(t, lx, "x = 1; // note", "// note", Comment)

	// And its own.
	assertSpanCovers(t, lx, `NSString *s = @"a \"b\"";`, `@"a \"b\""`, String)
	assertClass(t, lx, `NSString *s = @"a \"b\"";`, "NSString", Type)
	assertClass(t, lx, "BOOL ok = YES; id x = nil;", "YES", Constant)
	assertClass(t, lx, "BOOL ok = YES; id x = nil;", "nil", Constant)
	assertClass(t, lx, "BOOL ok = YES; id x = nil;", "id", Type)
	assertSpanCovers(t, lx, "id b = @YES;", "@YES", Constant)
	assertClass(t, lx, "[self.name length];", "self", Constant)
	assertClass(t, lx, "[super dealloc];", "super", Keyword)
	assertClass(t, lx, "for (id x in list) {", "in", Keyword)
	assertSpanCovers(t, lx, "@interface Car : NSObject", "@interface", Keyword)
	assertClass(t, lx, "@interface Car : NSObject", "Car", Type)
	assertClass(t, lx, "@implementation Car", "Car", Type)
	assertSpanCovers(t, lx, "@end", "@end", Keyword)
	assertSpanCovers(t, lx, "@property (nonatomic, strong) NSString *name;", "@property", Keyword)
	assertClass(t, lx, "@property (nonatomic, strong) NSString *name;", "nonatomic", Keyword)
	assertSpanCovers(t, lx, "SEL s = @selector(drive);", "@selector", Keyword)

	// A method declaration names its method.
	assertClass(t, lx, "- (void)drive;", "drive", Function)
	assertClass(t, lx, "- (void)drive;", "void", Type)
	assertClass(t, lx, "+ (instancetype)sharedCar;", "sharedCar", Function)
	assertClass(t, lx, "- (NSString *)title;", "title", Function)
	assertClass(t, lx, "- (void)startWithSpeed:(NSInteger)speed {", "startWithSpeed", Function)

	jvmDoc(t, lx, "/* open\nstill */ int x;\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "int", Type},
	)
	jvmUnterminated(t, lx, `@"`, "@", "- (", "+ (void", "/*")
}

func TestDart(t *testing.T) {
	jvmDetect(t, "dart", "main.dart", "lib/Widget.DART")
	lx := lang(t, "dart")
	jvmComment(t, lx, "//")

	assertClass(t, lx, "final res = await fetch();", "final", Keyword)
	assertClass(t, lx, "final res = await fetch();", "await", Keyword)
	assertClass(t, lx, "final res = await fetch();", "fetch", Function)
	assertClass(t, lx, "int? n = null;", "int", Type)
	assertClass(t, lx, "int? n = null;", "null", Constant)
	assertSpanCovers(t, lx, `s = 'it\'s';`, `'it\'s'`, String)
	assertSpanCovers(t, lx, `s = "Hi ${name}!";`, `"Hi ${name}!"`, String)
	assertSpanCovers(t, lx, "x = 1; // note", "// note", Comment)
	assertSpanCovers(t, lx, "/// Docs for [Point].", "/// Docs for [Point].", Comment)
	for _, n := range []string{"0xFF", "1e10", "1_000", "3.5"} {
		assertSpanCovers(t, lx, "var x = "+n+";", n, Number)
	}
	// A raw string has no escapes.
	assertSpanCovers(t, lx, `p = r'C:\dir\' + x;`, `r'C:\dir\'`, String)
	assertClass(t, lx, `p = r'C:\dir\' + x;`, "+", Operator)
	assertSpanCovers(t, lx, `p = r"\n";`, `r"\n"`, String)

	// Declarations and annotations.
	assertClass(t, lx, "class Point extends Base {", "Point", Type)
	assertClass(t, lx, "mixin Walker on Animal {}", "Walker", Type)
	assertClass(t, lx, "enum Color { red }", "Color", Type)
	assertClass(t, lx, "extension StringX on String {}", "StringX", Type)
	assertClass(t, lx, "typedef IntList = List<int>;", "IntList", Type)
	assertSpanCovers(t, lx, "extension type Meters(int v) {}", "extension type", Keyword)
	assertClass(t, lx, "extension type Meters(int v) {}", "Meters", Type)
	assertSpanCovers(t, lx, "@override", "@override", Function)
	assertSpanCovers(t, lx, "var s = #foo;", "#foo", Constant)
	assertSpanCovers(t, lx, "#!/usr/bin/env dart", "#!/usr/bin/env dart", Comment)
	// Words that are keywords only where Dart uses them as one.
	assertClass(t, lx, "int get length => x;", "get", Keyword)
	assertClass(t, lx, "int get length => x;", "length", Function)
	assertClass(t, lx, "final r = await http.get(url);", "get", Function)
	assertClass(t, lx, "import 'x.dart' show Client;", "show", Keyword)
	assertClass(t, lx, "part 'model.g.dart';", "part", Keyword)
	assertClass(t, lx, "for (final part in all) {}", "part", Plain)
	assertClass(t, lx, "base class Box {}", "base", Keyword)

	jvmDoc(t, lx, "/* outer /* inner */\nstill outer */ var x = 1;\n",
		jvmSpot{1, "still", Comment},
		jvmSpot{1, "var", Keyword},
	)
	jvmDoc(t, lx, "var m = '''multi\nline''' + \"\"\"also\nmulti\"\"\";\nvar r = r'''raw\n\\''' + 1;\n",
		jvmSpot{1, "line", String},
		jvmSpot{1, "+", Operator},
		jvmSpot{2, "multi", String},
		jvmSpot{4, `\'''`, String},
		jvmSpot{4, "+", Operator},
	)
	jvmUnterminated(t, lx, "r'", `r"""`, "'''", `"`, "@", "#", "/* /*")
}

func TestVB(t *testing.T) {
	jvmDetect(t, "vb", "Module1.vb", "script.vbs", "LEGACY.VBS")
	lx := lang(t, "vb")
	jvmComment(t, lx, "'")

	// Written in any case.
	assertClass(t, lx, "If x Then Return", "If", Keyword)
	assertClass(t, lx, "if x then return", "then", Keyword)
	assertClass(t, lx, "END SUB", "END", Keyword)
	assertClass(t, lx, "Dim n As Integer = 0", "Integer", Type)
	assertClass(t, lx, "dim n as integer = 0", "integer", Type)
	assertClass(t, lx, "If x Is Nothing Then", "Nothing", Constant)
	assertClass(t, lx, "s = s & vbCrLf", "vbCrLf", Constant)
	assertClass(t, lx, "MsgBox(CStr(n))", "CStr", Function)
	assertSpanCovers(t, lx, `s = "He said ""hi""" & x`, `"He said ""hi"""`, String)
	assertSpanCovers(t, lx, `s = $"Hello {name}"`, `$"Hello {name}"`, String)
	assertSpanCovers(t, lx, `c = "x"c`, `"x"c`, String)

	// ' and REM comments; a ' in a string is only text.
	assertSpanCovers(t, lx, `x = 1 ' a "note"`, `' a "note"`, Comment)
	assertSpanCovers(t, lx, `s = "it's" & x`, `"it's"`, String)
	assertSpanCovers(t, lx, "REM an old comment", "REM an old comment", Comment)
	assertSpanCovers(t, lx, "  rem lower too", "rem lower too", Comment)
	assertSpanCovers(t, lx, "x = 1 : Rem after a colon", "Rem after a colon", Comment)
	assertClass(t, lx, "Dim remark = 1", "remark", Plain)

	// Numbers with prefixes and type suffixes, and dates.
	for _, n := range []string{"&H1F", "&O17", "&B101", "100L", "1.5D", "7%", "1_000", "2.5E+3", "&HFFUS"} {
		assertSpanCovers(t, lx, "x = "+n+" + y", n, Number)
	}
	assertSpanCovers(t, lx, "d = #12/31/2025# + x", "#12/31/2025#", Number)
	assertSpanCovers(t, lx, "t = #2025-12-31 10:30 PM#", "#2025-12-31 10:30 PM#", Number)

	// Directives, attributes and declarations.
	assertSpanCovers(t, lx, `#Region "Helpers"`, "#Region", Keyword)
	assertSpanCovers(t, lx, "#End Region", "#End Region", Keyword)
	assertSpanCovers(t, lx, "#If DEBUG Then", "#If", Keyword)
	assertClass(t, lx, "<Serializable>", "Serializable", Function)
	assertClass(t, lx, "Public Function Greet(name As String) As String", "Greet", Function)
	assertClass(t, lx, "sub lower()", "lower", Function)
	assertClass(t, lx, "Public Class Greeter", "Greeter", Type)
	assertClass(t, lx, "Module Program", "Program", Type)
	assertClass(t, lx, "End Function", "Function", Keyword)

	// A string does not run on to the next line.
	jvmDoc(t, lx, "s = \"open\nDim x\n", jvmSpot{1, "Dim", Keyword})
	jvmUnterminated(t, lx, `"`, `$"`, "#", "#1", "&H", "<", "rem", "'")
}
