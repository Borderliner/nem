package syntax

import "testing"

// The scripting languages: that each claims its files and #! lines, and
// colours what its grammar marks - above all what needed a match or a
// region, and every construct that runs over lines.

// want is one expectation: sub, which occurs once in src, is of class.
type want struct {
	src, sub string
	class    Class
}

// assertDetect checks that each file name and each #! line picks lang.
func assertDetect(t *testing.T, lang string, names, shebangs []string) {
	t.Helper()
	for _, n := range names {
		if got := For(n).Name(); got != lang {
			t.Errorf("For(%q) = %q, want %q", n, got, lang)
		}
	}
	for _, s := range shebangs {
		if got := ForWithHeader("script", s).Name(); got != lang {
			t.Errorf("ForWithHeader(script, %q) = %q, want %q", s, got, lang)
		}
	}
}

// assertAll checks each expectation on its own line.
func assertAll(t *testing.T, lx Lexer, wants []want) {
	t.Helper()
	for _, w := range wants {
		assertClass(t, lx, w.src, w.sub, w.class)
	}
}

// assertCovers checks that a span of class covers each sub exactly.
func assertCovers(t *testing.T, lx Lexer, wants []want) {
	t.Helper()
	for _, w := range wants {
		assertSpanCovers(t, lx, w.src, w.sub, w.class)
	}
}

// docWant is an expectation on one line of a document.
type docWant struct {
	line  int
	sub   string
	class Class
}

// assertDoc lexes doc top to bottom, carrying state, and checks each
// expectation on its line.
func assertDoc(t *testing.T, lx Lexer, doc string, wants []docWant) {
	t.Helper()
	lines, spans := lexDoc(lx, doc)
	for i := range lines {
		checkSpans(t, string(lines[i]), lines[i], spans[i])
	}
	for _, w := range wants {
		src := string(lines[w.line])
		at := len([]rune(src[:indexOnce(t, src, w.sub)]))
		if got := classAt(spans[w.line], at); got != w.class {
			t.Errorf("line %d %q: %q is %v, want %v (spans: %s)",
				w.line, src, w.sub, got, w.class, describe(lines[w.line], spans[w.line]))
		}
	}
}

// assertComment checks the comment M-; writes.
func assertComment(t *testing.T, name, start, end string) {
	t.Helper()
	s, e, ok := lang(t, name).Comment()
	if !ok || s != start || e != end {
		t.Errorf("%s comments with %q %q %v, want %q %q", name, s, e, ok, start, end)
	}
}

// assertClosed checks that a line leaves nothing open.
func assertClosed(t *testing.T, lx Lexer, src string) {
	t.Helper()
	if _, st := lx.Lex([]rune(src), 0); st != 0 {
		t.Errorf("%q left state %v open", src, st)
	}
}

func TestRuby(t *testing.T) {
	assertDetect(t, "ruby",
		[]string{"app.rb", "deploy.rake", "nem.gemspec", "config.ru", "Rakefile",
			"Gemfile", "Guardfile", "Podfile", "Vagrantfile", "Brewfile"},
		[]string{"#!/usr/bin/env ruby", "#!/usr/bin/ruby3.3 -w"})
	assertComment(t, "ruby", "#", "")
	lx := lang(t, "ruby")
	assertAll(t, lx, []want{
		{"if ready then go end", "if", Keyword},
		{"x = nil", "nil", Constant},
		{"x = y # a note", "# a note", Comment},
		{"@count += 1", "@count", Constant},
		{"@@instances = []", "@@instances", Constant},
		{"$stdout.sync = true", "$stdout", Constant},
		{"puts $0", "$0", Constant},
		{"x = :symbol", ":symbol", Constant},
		{"xs.inject(:+)", ":+", Constant},
		{"h = {name: 1}", "name:", Constant},
		{"Foo::Bar.new", "::", Operator},
		{"Foo::Bar.new", "Bar", Plain},
		{"def empty?", "empty?", Function},
		{"xs.empty?", "empty?", Function},
		{"list.sort!", "sort!", Function},
		{"a != b", "!=", Operator},
		{"def self.create(x)", "self", Keyword},
		{"def self.create(x)", "create", Function},
		{"def ==(other)", "==", Function},
		{"return defined?(x)", "defined?", Keyword},
		{"class Parser < Base", "Parser", Type},
		{"module Nem", "Nem", Type},
		{"raise ArgumentError", "ArgumentError", Type},
		{"puts x", "puts", Function},
		{"x = a / b / c", "b", Plain},
		{"x << y", "y", Plain},
	})
	assertCovers(t, lx, []want{
		{`s = "a\"b" + c`, `"a\"b"`, String},
		{`s = 'it\'s' + c`, `'it\'s'`, String},
		{`x = :"odd symbol"`, `:"odd symbol"`, Constant},
		{"n = 1_000 + 2", "1_000", Number},
		{"n = 0x1F + 2", "0x1F", Number},
		{"x = %w[a b c].size", "%w[a b c]", String},
		{"x = %i(a b)", "%i(a b)", String},
		{"x = %q{it's} + y", "%q{it's}", String},
		{"x = %r|a/b|i + y", "%r|a/b|i", String},
		{`ok if line =~ /#\d+/`, `/#\d+/`, String},
		{`x.gsub(/'/, "")`, `/'/`, String},
		{`case x when /a#/ then 1 end`, `/a#/`, String},
		{"class Foo::Bar < Base::Thing", "Foo::Bar", Type},
		{"class Foo::Bar < Base::Thing", "Base::Thing", Type},
	})
	assertClosed(t, lx, "x << y")
	assertClosed(t, lx, "  =begin not at the margin")

	// =begin and =end only at the margin.
	assertDoc(t, lx, "=begin\nx = 1 # not code\n=end trailing words\ny = 2\n", []docWant{
		{0, "=begin", Comment},
		{1, "x", Comment},
		{2, "trailing", Comment},
		{3, "y", Plain},
		{3, "2", Number},
	})
	// A heredoc closes only at its own word alone on a line: indented
	// after <<~ and <<-, at the margin after a bare <<.
	assertDoc(t, lx, "s = <<~EOS\n  EOS is not alone\n  END\n  EOS\nx = 1\n", []docWant{
		{1, "EOS", String},
		{2, "END", String},
		{3, "EOS", String},
		{4, "x", Plain},
		{4, "1", Number},
	})
	assertDoc(t, lx, "s = <<-'SQL'\nselect 1\n    SQL\nx\n", []docWant{
		{1, "select", String},
		{3, "x", Plain},
	})
	assertDoc(t, lx, "s = <<EOS\n  EOS\nEOS\nx\n", []docWant{
		{1, "EOS", String},
		{2, "EOS", String},
		{3, "x", Plain},
	})
	// A bracketed %w list may run over lines, to its own bracket.
	assertDoc(t, lx, "FILES = %w[\n  a.rb (b).rb\n].freeze\n", []docWant{
		{1, "a.rb", String},
		{1, "(b)", String},
		{2, "]", String},
		{2, "freeze", Plain},
	})
	// What follows __END__ is data.
	assertDoc(t, lx, "puts DATA.read\n__END__\nif this were code\n", []docWant{
		{0, "DATA", Constant},
		{2, "if", Comment},
	})
}

func TestPerl(t *testing.T) {
	assertDetect(t, "perl",
		[]string{"script.pl", "Module.pm", "basic.t", "app.psgi"},
		[]string{"#!/usr/bin/perl", "#!/usr/bin/env perl", "#!/usr/bin/perl5.36 -w"})
	assertComment(t, "perl", "#", "")
	lx := lang(t, "perl")
	assertAll(t, lx, []want{
		{"my $x = 1;", "my", Keyword},
		{"my $x = 1;", "$x", Constant},
		{"push @items, 1;", "@items", Constant},
		{"my %seen = ();", "%seen", Constant},
		{"print $_;", "$_", Constant},
		{"print $1;", "$1", Constant},
		{"print STDERR $x;", "STDERR", Constant},
		{"return undef;", "undef", Constant},
		{"my $n = 1; # a note", "# a note", Comment},
		{"sub greet {", "greet", Function},
		{"print join(',', @a);", "join", Function},
		{"$h{s} = 1;", "s", Plain},
		{"%h = (y => 1);", "y", Plain},
		{"my $q = $a / $b / 2;", "$b", Constant},
		{"1 << 2", "2", Number},
	})
	assertCovers(t, lx, []want{
		{`my $s = "a\"b";`, `"a\"b"`, String},
		{`my $s = 'it\'s';`, `'it\'s'`, String},
		{"for (0 .. $#items) {", "$#items", Constant},
		{"my $v = ${name};", "${name}", Constant},
		{"$Foo::bar = 1;", "$Foo::bar", Constant},
		{"package Foo::Bar;", "Foo::Bar", Type},
		{"sub Foo::run {", "Foo::run", Function},
		{"my $n = 0x1f + 1_000;", "1_000", Number},
		{"my @a = qw(a b c);", "qw(a b c)", String},
		{"my $s = q{it's};", "q{it's}", String},
		{"my $s = qq|a|;", "qq|a|", String},
		{"$x =~ s/a#b/c/g;", "s/a#b/c/g", String},
		{"$x =~ s{a}{b}g;", "s{a}{b}g", String},
		{"$x =~ tr/a-z/A-Z/;", "tr/a-z/A-Z/", String},
		{"$x =~ m!a!i;", "m!a!i", String},
		{"if ($x =~ /a#b/) {", "/a#b/", String},
		{"my @f = split /,/, $s;", "/,/", String},
		{"my @m = grep { /x/ } @a;", "/x/", String},
		{"&handler(1);", "&handler", Function},
	})
	assertClosed(t, lx, "1 << 2;")

	// POD runs from any =word at the margin to =cut.
	assertDoc(t, lx, "=head1 NAME\n\nfoo - it's a thing\n\n=cut\nmy $x;\n", []docWant{
		{0, "NAME", Comment},
		{2, "it's", Comment},
		{4, "=cut", Comment},
		{5, "my", Keyword},
	})
	// A here-document closes only at its own word alone on a line.
	assertDoc(t, lx, "print <<\"EOT\";\nEOT is not alone\nEND\nEOT\nmy $y;\n", []docWant{
		{1, "EOT", String},
		{2, "END", String},
		{3, "EOT", String},
		{4, "my", Keyword},
	})
	assertDoc(t, lx, "print <<~EOT;\n    text\n    EOT\nmy $y;\n", []docWant{
		{1, "text", String},
		{3, "$y", Constant},
	})
	assertDoc(t, lx, "print <<EOT;\n  EOT\nEOT\nmy $y;\n", []docWant{
		{1, "EOT", String},
		{3, "my", Keyword},
	})
	// A bracketed qw list may run over lines, to its own bracket.
	assertDoc(t, lx, "my @l = qw(\n    foo bar\n);\nmy $z;\n", []docWant{
		{1, "foo", String},
		{2, ")", String},
		{2, ";", Punctuation},
		{3, "my", Keyword},
	})
	assertDoc(t, lx, "1;\n__END__\nmy $not_code;\n", []docWant{
		{2, "my", Comment},
	})
}

func TestPHP(t *testing.T) {
	assertDetect(t, "php",
		[]string{"index.php", "view.phtml", "bug.phpt"},
		[]string{"#!/usr/bin/env php", "<?php"})
	assertComment(t, "php", "//", "")
	lx := lang(t, "php")
	assertAll(t, lx, []want{
		{"<?php", "<?php", Keyword},
		{"<?= $title ?>", "<?=", Keyword},
		{"<?= $title ?>", "?>", Keyword},
		{"<?= $title ?>", "$title", Constant},
		{"function greet($name) {", "greet", Function},
		{"function greet($name) {", "function", Keyword},
		{"class User extends Model {", "User", Type},
		{"class User extends Model {", "Model", Type},
		{"$u = new User();", "User", Type},
		{"$u = NULL;", "NULL", Constant},
		{"$u = null;", "null", Constant},
		{"IF ($x) {", "IF", Keyword},
		{"function f(int $x): string {", "int", Type},
		{"$n = 1; // a note", "// a note", Comment},
		{"$n = 1; # a note", "# a note", Comment},
		{"$n = strlen($s);", "strlen", Function},
		{"$this->name = $n;", "$this", Constant},
	})
	assertCovers(t, lx, []want{
		{`$s = "a\"b";`, `"a\"b"`, String},
		{`$s = 'it\'s';`, `'it\'s'`, String},
		{"#[Route('/x')]", "#[Route", Function},
		{"$n = 0x1F + 1_000;", "1_000", Number},
		{"$n = 1; /* a note */ $m = 2;", "/* a note */", Comment},
	})
	assertDoc(t, lx, "/* a\n   note */ $x = 1;\n", []docWant{
		{1, "note", Comment},
		{1, "$x", Constant},
	})
	// A heredoc or nowdoc closes at its own word, which may be indented and
	// followed by the rest of the statement.
	assertDoc(t, lx, "$s = <<<EOT\n  EOTX is not it\n  EOT;\n$y = 1;\n", []docWant{
		{1, "EOTX", String},
		{2, "EOT", String},
		{2, ";", Punctuation},
		{3, "$y", Constant},
	})
	assertDoc(t, lx, "$s = <<<'EOT'\n{$not} interpolated\nEOT;\n$y = 1;\n", []docWant{
		{1, "interpolated", String},
		{3, "$y", Constant},
	})
}

func TestTcl(t *testing.T) {
	assertDetect(t, "tcl",
		[]string{"main.tcl", "gui.tk"},
		[]string{"#!/usr/bin/tclsh", "#!/usr/bin/env tclsh8.6", "#!/usr/bin/env wish", "#!/usr/bin/expect -f"})
	lx := lang(t, "tcl")
	assertAll(t, lx, []want{
		{"# a note", "# a note", Comment},
		{"    # an indented note", "# an", Comment},
		{"set x 1 ;# a note", "# a note", Comment},
		{".c create rect 0 0 9 9 -fill #ff0000", "#ff0000", Plain},
		{"if {$x > 1} {", "if", Keyword},
		{"if {$x > 1} {", "$x", Constant},
		{"puts $name", "puts", Function},
		{"proc greet {name} {", "greet", Function},
		{"set n 42", "42", Number},
	})
	assertCovers(t, lx, []want{
		{`set s "a\"b"`, `"a\"b"`, String},
		{"proc ::ns::run {} {", "::ns::run", Function},
		{"puts ${odd name}", "${odd name}", Constant},
		{"puts $::env(HOME)", "$::env", Constant},
	})
	assertClosed(t, lx, "proc f {} {")
	// A " inside braces is text; the string it seems to open ends with the
	// line.
	assertClosed(t, lx, `if {[regexp {^"} $x]} {`)
}

func TestAwk(t *testing.T) {
	assertDetect(t, "awk",
		[]string{"report.awk"},
		[]string{"#!/usr/bin/awk -f", "#!/usr/bin/env gawk -f", "#!/usr/bin/mawk -f", "#!/usr/bin/nawk -f"})
	assertComment(t, "awk", "#", "")
	lx := lang(t, "awk")
	assertAll(t, lx, []want{
		{`BEGIN { FS = ":" }`, "BEGIN", Keyword},
		{`BEGIN { FS = ":" }`, "FS", Constant},
		{"END { print NR }", "END", Keyword},
		{"{ print $1, $NF }", "$1", Constant},
		{"{ print $1, $NF }", "$NF", Constant},
		{"{ print $1, $NF }", "print", Function},
		{"n = length($0)", "length", Function},
		{"function max(a, b) {", "max", Function},
		{"x = a / b / c", "b", Plain},
		{"n++ # a note", "# a note", Comment},
	})
	assertCovers(t, lx, []want{
		{`s = "a\"b"`, `"a\"b"`, String},
		{"/^#/ { next }", "/^#/", String},
		{"$0 ~ /a#b/ { n++ }", "/a#b/", String},
		{"if (!/skip/) print", "/skip/", String},
		{"x = y ? /a/ : /b/", "/b/", String},
		{"n = 3.5 * x", "3.5", Number},
	})
}

func TestR(t *testing.T) {
	assertDetect(t, "r",
		[]string{"analysis.R", "helpers.r", ".Rprofile"},
		[]string{"#!/usr/bin/env Rscript", "#!/usr/bin/Rscript --vanilla"})
	assertComment(t, "r", "#", "")
	lx := lang(t, "r")
	assertAll(t, lx, []want{
		{"x <- 5", "<-", Operator},
		{"x <- 5", "5", Number},
		{"ok <- TRUE", "TRUE", Constant},
		{"v <- c(1, NA, 3)", "NA", Constant},
		{"x <- NULL", "NULL", Constant},
		{"y <- -Inf", "Inf", Constant},
		{"y <- NaN", "NaN", Constant},
		{"area <- function(r) {", "area", Function},
		{"area <- function(r) {", "function", Keyword},
		{"for (i in 1:10) {", "for", Keyword},
		{"x <- y # a note", "# a note", Comment},
		{"`if` <- 1", "if", Plain},
		{"library(dplyr)", "library", Function},
	})
	assertCovers(t, lx, []want{
		{`s <- "a\"b"`, `"a\"b"`, String},
		{`s <- 'it\'s'`, `'it\'s'`, String},
		{"x %in% y", "%in%", Operator},
		{"if (is.na(x)) y", "is.na", Function},
		{"`my var` <- 1", "`my var`", Plain},
		{"`my fn` <- function(x) x", "`my fn`", Function},
		{`p <- r"(C:\a "b")"`, `r"(C:\a "b")"`, String},
		{`p <- R"-[x]-" + 1`, `R"-[x]-"`, String},
		{"n <- 5L + 1e-3", "5L", Number},
		{"n <- 5L + 1e-3", "1e-3", Number},
	})
	// A raw string closes only at its own bracket, dashes and quote.
	assertDoc(t, lx, "s <- r\"--(\n)\" and )-\" still\n)--\"\nx\n", []docWant{
		{1, "still", String},
		{2, ")--", String},
		{3, "x", Plain},
	})
}

func TestJulia(t *testing.T) {
	assertDetect(t, "julia",
		[]string{"solve.jl"},
		[]string{"#!/usr/bin/env julia", "#!/opt/julia/bin/julia1.10"})
	assertComment(t, "julia", "#", "")
	lx := lang(t, "julia")
	assertAll(t, lx, []want{
		{"function area(r)", "area", Function},
		{"function area(r)", "function", Keyword},
		{"struct Point", "Point", Type},
		{"mutable struct Box", "Box", Type},
		{"abstract type Shape end", "Shape", Type},
		{"module Geo", "Geo", Type},
		{"x::Float64 = 1", "Float64", Type},
		{"f(x::MyType) = 1", "MyType", Type},
		{"g(x::T) where {T <: Real} = x", "Real", Type},
		{"color = :red", ":red", Constant},
		{"pair = (:a, 1)", ":a", Constant},
		{"for k in 1:len", "len", Plain},
		{"y = a ? b : c", "c", Plain},
		{"@time f(x)", "@time", Function},
		{"push!(v, 1)", "push!", Function},
		{"a != b", "!=", Operator},
		{"B = A'", "'", Operator},
		{"x = nothing", "nothing", Constant},
		{"ok = true # a note", "# a note", Comment},
	})
	assertCovers(t, lx, []want{
		{`s = "a\"b"`, `"a\"b"`, String},
		{`c = 'a' + 1`, `'a'`, String},
		{`c = '\'' + 1`, `'\''`, String},
		{`re = r"\d+" + x`, `r"\d+"`, String},
		{"n = 1_000 + 0x1F", "1_000", Number},
		{"n = 2.5e-3 + 1", "2.5e-3", Number},
		{"x = 1 #= a note =# + 2", "#= a note =#", Comment},
	})
	// A block comment nests: it closes only when every one opened in it has.
	assertDoc(t, lx, "#= outer #= inner =# still\nmore =# x = 1\n", []docWant{
		{0, "still", Comment},
		{1, "more", Comment},
		{1, "x", Plain},
	})
	assertDoc(t, lx, "doc = \"\"\"\n  a \"quoted\" line\n  \"\"\"\nx = 1\n", []docWant{
		{1, "quoted", String},
		{3, "x", Plain},
	})
}

func TestPowerShell(t *testing.T) {
	assertDetect(t, "powershell",
		[]string{"build.ps1", "Tools.psm1", "Tools.psd1"},
		[]string{"#!/usr/bin/env pwsh", "#!/usr/bin/pwsh -NoProfile"})
	assertComment(t, "powershell", "#", "")
	lx := lang(t, "powershell")
	assertAll(t, lx, []want{
		{"if ($x) { }", "if", Keyword},
		{"IF ($x) { }", "IF", Keyword},
		{"FOREACH ($i in $list) { }", "FOREACH", Keyword},
		{"FOREACH ($i in $list) { }", "in", Keyword},
		{"$name = 'x'", "$name", Constant},
		{"$ok = $true", "$true", Constant},
		{"$_ | Out-Null", "$_", Constant},
		{"Get-ChildItem -Path .", "Get-ChildItem", Function},
		{"function Get-Thing {", "Get-Thing", Function},
		{"function helper {", "helper", Function},
		{"class Car {", "Car", Type},
		{"$n = 42 # a note", "# a note", Comment},
		{"$n = 42 # a note", "42", Number},
	})
	assertCovers(t, lx, []want{
		{"$p = $env:PATH", "$env:PATH", Constant},
		{"$v = ${odd name}", "${odd name}", Constant},
		{"$s = \"a`\"b\" + 1", "\"a`\"b\"", String},
		{"$s = 'it''s' + 1", "'it''s'", String},
		{"if ($a -eq $b) { }", "-eq", Operator},
		{"if ($a -in $list) { }", "-in", Operator},
		{"[string]$s = 1", "[string]", Type},
		{"[int[]]$a = 1", "[int[]]", Type},
		{"$n = 1 <# a note #> + 2", "<# a note #>", Comment},
		{"Invoke-Thing @params", "@params", Constant},
	})
	assertDoc(t, lx, "<# a\n   note #> $x = 1\n", []docWant{
		{1, "note", Comment},
		{1, "$x", Constant},
	})
	// A here-string closes only at "@ or '@ at the start of a line.
	assertDoc(t, lx, "$s = @\"\nsome \"quoted\" text\n  \"@ not the end\n\"@\n$x = 1\n", []docWant{
		{1, "quoted", String},
		{2, "not", String},
		{3, "\"@", String},
		{4, "$x", Constant},
	})
	assertDoc(t, lx, "$s = @'\nit's $raw\n'@\n$x = 1\n", []docWant{
		{1, "$raw", String},
		{3, "$x", Constant},
	})
}

func TestBatch(t *testing.T) {
	assertDetect(t, "batch", []string{"build.bat", "SETUP.CMD"}, nil)
	assertComment(t, "batch", "REM", "")
	lx := lang(t, "batch")
	assertAll(t, lx, []want{
		{"@echo off", "echo", Function},
		{"@echo off", "off", Constant},
		{"REM a note", "REM a note", Comment},
		{"rem a note", "rem a note", Comment},
		{"  @REM an indented note", "@REM", Comment},
		{":: a note", ":: a note", Comment},
		{"echo REMOVE this", "REMOVE", Plain},
		{"echo REM is text here", "REM", Plain},
		{"echo x & rem a note", "rem a note", Comment},
		{"IF EXIST x.txt echo", "IF", Keyword},
		{"if not exist x.txt echo", "exist", Keyword},
		{"set /a n=42", "42", Number},
		{":loop", ":loop", Function},
		{"goto :eof", ":eof", Function},
		{"goto done", "done", Function},
		{"call :helper 1", ":helper", Function},
	})
	assertCovers(t, lx, []want{
		{"echo %PATH% here", "%PATH%", Constant},
		{"echo %DATE:~0,4% here", "%DATE:~0,4%", Constant},
		{"for %%i in (*) do echo", "%%i", Constant},
		{"cd %~dp0", "%~dp0", Constant},
		{"echo %1 here", "%1", Constant},
		{"echo !count! here", "!count!", Constant},
		{`cd "C:\dir\" & echo`, `"C:\dir\"`, String},
	})
}

func TestFish(t *testing.T) {
	assertDetect(t, "fish",
		[]string{"config.fish"},
		[]string{"#!/usr/bin/env fish", "#!/usr/local/bin/fish"})
	lx := lang(t, "fish")
	assertAll(t, lx, []want{
		{"# a note", "# a note", Comment},
		{"echo x # a note", "# a note", Comment},
		{"echo a#b", "a#b", Plain},
		{"function greet --description 'say hi'", "greet", Function},
		{"function greet --description 'say hi'", "function", Keyword},
		{"end", "end", Keyword},
		{"set -x PATH $PATH /opt/bin", "set", Keyword},
		{"set -x PATH $PATH /opt/bin", "$PATH", Constant},
		{"string match -q x $argv", "string", Function},
		{"if test $status -eq 0", "if", Keyword},
		{"sleep 42", "42", Number},
	})
	assertCovers(t, lx, []want{
		{`echo "a\"b" x`, `"a\"b"`, String},
		{`echo 'it\'s' x`, `'it\'s'`, String},
	})
	// fish's strings, like sh's, may run over lines.
	assertDoc(t, lx, "echo \"one\ntwo\" three\n", []docWant{
		{1, "two", String},
		{1, "three", Plain},
	})
}

func TestVim(t *testing.T) {
	assertDetect(t, "vim",
		[]string{"plugin.vim", ".vimrc", ".gvimrc", "_vimrc", ".exrc", "/home/u/.vimrc"}, nil)
	assertComment(t, "vim", `"`, "")
	lx := lang(t, "vim")
	assertAll(t, lx, []want{
		{`" a note`, `" a note`, Comment},
		{`    " an indented note`, `" an`, Comment},
		{`set number " a trailing note`, `" a trailing note`, Comment},
		{`let s = "text"`, `"text"`, String},
		{`let s = "a" " a note`, `" a note`, Comment},
		{"let g:count = 1", "let", Keyword},
		{"let g:count = 1", "g:count", Constant},
		{"return a:0", "a:0", Constant},
		{"let &tabstop = 4", "&tabstop", Constant},
		{"echo $HOME", "$HOME", Constant},
		{"let x = v:true", "v:true", Constant},
		{"function! s:Helper(x) abort", "s:Helper", Function},
		{"function! s:Helper(x) abort", "function", Keyword},
		{"function! s:Helper(x) abort", "abort", Keyword},
		{"call s:Helper(1)", "s:Helper", Function},
		{"call mylib#util#Run()", "mylib#util#Run", Function},
		{"endfunction", "endfunction", Keyword},
		{"autocmd BufRead *.go setlocal ts=4", "autocmd", Keyword},
		{"nnoremap <leader>w :w<CR>", "<leader>", Constant},
		{"let n = 42", "42", Number},
		{"# a vim9 note", "# a vim9 note", Comment},
	})
	assertCovers(t, lx, []want{
		{`let s = "a\"b" . x`, `"a\"b"`, String},
		{`let s = 'it''s' . x`, `'it''s'`, String},
		{"call mylib#util#Run()", "mylib#util#Run", Function},
	})
}

// A construct left open is the normal state of a file being typed. None may
// run off the line, and the line after one must lex cleanly, whatever it
// leaves open.
func TestScriptingUnterminatedConstructsAreSafe(t *testing.T) {
	for name, lines := range map[string][]string{
		"ruby":       {"s = <<~EOS", "s = <<EOS", "=begin", "x = %w[", "x = %q|", "x = :\"", "x = /", "def ", "$", "@", "__END__"},
		"perl":       {"print <<EOT;", "print <<~EOT;", "=pod", "qw(", "s{a}{", "s/a/", "tr/", "m{", "$#", "__DATA__"},
		"php":        {"$s = <<<EOT", "$s = <<<'EOT'", "/* open", "#[", "<?", "$"},
		"tcl":        {`set s "open`, "proc ", "$"},
		"awk":        {`s = "open`, "/open", "$"},
		"r":          {`s <- r"(`, `s <- r"--[`, "`open", "%in"},
		"julia":      {`s = """`, "#= open", "#= #= deeper", `r"`, "'", "@", ":"},
		"powershell": {`$s = @"`, "$s = @'", "<# open", "$s = \"open `", "${", "["},
		"batch":      {"echo %PATH", "echo !x", "%%", `echo "open`, "REM"},
		"fish":       {`echo "open`, "echo 'open", "$"},
		"vim":        {`echo "open`, "echo 'open", "function! ", "<", "&"},
	} {
		lx := lang(t, name)
		for _, s := range lines {
			line := []rune(s)
			spans, st := lx.Lex(line, 0)
			checkSpans(t, name+" "+s, line, spans)
			next := []rune("next line 日本語 🙂")
			spans, _ = lx.Lex(next, st)
			checkSpans(t, name+" after "+s, next, spans)
		}
	}
}

// Spans are rune indices, in these languages as in every other.
func TestScriptingSpansAreRuneIndices(t *testing.T) {
	assertSpanCovers(t, lang(t, "ruby"), `s = "日本語" # コメント`, "# コメント", Comment)
	assertSpanCovers(t, lang(t, "perl"), `my $名 = "🙂"; # ok`, "# ok", Comment)
	assertSpanCovers(t, lang(t, "php"), `$s = "日本"; // ok`, "// ok", Comment)
	assertSpanCovers(t, lang(t, "julia"), `s = "π" # ok`, "# ok", Comment)
	assertSpanCovers(t, lang(t, "powershell"), `$s = "日本" # ok`, "# ok", Comment)
	assertSpanCovers(t, lang(t, "vim"), `let s = "日本" " ok`, `" ok`, Comment)
	assertSpanCovers(t, lang(t, "batch"), `echo 日本 & rem ok`, "rem ok", Comment)
}
