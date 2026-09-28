package syntax

import "testing"

// The functional languages: the Lisps, the MLs and the languages of the
// Erlang machine. Each test pins the files a language claims, the comment
// M-; writes, and every construct its definition needed a match or a region
// for, since those are what a later edit to the definition is likely to
// break.

// fpTok expects sub, which occurs once in src, to be of class want - and,
// with fpWhole, one span to cover exactly it.
type fpTok struct {
	src, sub string
	want     Class
	whole    bool
}

const (
	fpWhole = true
	fpPart  = false
)

func fpToks(t *testing.T, lx Lexer, toks []fpTok) {
	t.Helper()
	for _, tc := range toks {
		if tc.whole {
			assertSpanCovers(t, lx, tc.src, tc.sub, tc.want)
		} else {
			assertClass(t, lx, tc.src, tc.sub, tc.want)
		}
	}
}

// fpLine expects sub, which occurs once on line n of a document, to be of
// class want.
type fpLine struct {
	n    int
	sub  string
	want Class
}

// fpDoc lexes doc from its first line, checks each of want, and that
// whatever the document opened, it closed.
func fpDoc(t *testing.T, lx Lexer, doc string, want ...fpLine) {
	t.Helper()
	lines, spans := lexDoc(lx, doc)
	for _, w := range want {
		src := string(lines[w.n])
		at := len([]rune(src[:indexOnce(t, src, w.sub)]))
		if got := classAt(spans[w.n], at); got != w.want {
			t.Errorf("%s line %d %q: %q is %v, want %v (spans: %s)",
				lx.Name(), w.n, src, w.sub, got, w.want, describe(lines[w.n], spans[w.n]))
		}
	}
	var st State
	for _, l := range lines {
		_, st = lx.Lex(l, st)
	}
	if st != 0 {
		t.Errorf("%s: %q leaves state %v open", lx.Name(), doc, st)
	}
}

// fpDetects expects each file name, and each first line of a file with no
// extension, to be read as language want.
func fpDetects(t *testing.T, want string, names, firstLines []string) {
	t.Helper()
	for _, n := range names {
		if got := For(n).Name(); got != want {
			t.Errorf("For(%q) = %q, want %q", n, got, want)
		}
	}
	for _, h := range firstLines {
		if got := ForWithHeader("script", h).Name(); got != want {
			t.Errorf("ForWithHeader(script, %q) = %q, want %q", h, got, want)
		}
	}
}

// fpComment expects M-; to comment with start and end.
func fpComment(t *testing.T, name, start, end string) {
	t.Helper()
	s, e, ok := lang(t, name).Comment()
	if !ok || s != start || e != end {
		t.Errorf("%s comments with %q %q %v, want %q %q", name, s, e, ok, start, end)
	}
}

func TestLisp(t *testing.T) {
	fpDetects(t, "lisp", []string{"a.lisp", "a.lsp", "a.cl", "app.asd", "A.LISP"},
		[]string{"#!/usr/bin/sbcl --script", "#!/usr/bin/env clisp"})
	fpComment(t, "lisp", ";;", "")
	lx := lang(t, "lisp")
	fpToks(t, lx, []fpTok{
		// Special forms and the macros that declare, and what they declare.
		{"(defun square (x) (* x x))", "defun", Keyword, fpWhole},
		{"(defun square (x) (* x x))", "square", Function, fpWhole},
		{"(let ((n 1)) n)", "let", Keyword, fpWhole},
		{"(defmacro with-it (x) x)", "with-it", Function, fpWhole},
		{"(defclass point () ())", "point", Type, fpWhole},
		{"(defconstant +limit+ 10)", "+limit+", Constant, fpWhole},
		{"(defun f (x &optional y) x)", "&optional", Keyword, fpWhole},
		{"(declare (type fixnum x))", "fixnum", Type, fpWhole},
		// The reader folds case.
		{"(DEFUN LOUD () T)", "DEFUN", Keyword, fpWhole},
		{"(DEFUN LOUD () T)", "T", Constant, fpWhole},
		// A name is read whole, - and all, and the one after ( is called.
		{"(string-upcase s)", "string-upcase", Function, fpWhole},
		{"(list if-then)", "list", Function, fpWhole},
		{"(list if-then)", "if-then", Plain, fpPart},
		{"(1+ x)", "1+", Function, fpWhole},
		{"(* n 2)", "*", Function, fpWhole},
		{"(print *print-base*)", "*print-base*", Plain, fpPart},
		// Constants: t and nil, keywords, uninterned and quoted symbols,
		// characters - which may be any rune, a quote or ; among them.
		{"(if x t nil)", "t", Constant, fpWhole},
		{"(if x t nil)", "nil", Constant, fpWhole},
		{"(getf plist :key)", ":key", Constant, fpWhole},
		{"(:export #:main)", "#:main", Constant, fpWhole},
		{"(eq x 'foo)", "'foo", Constant, fpWhole},
		{"(mapcar #'car xs)", "#'car", Function, fpWhole},
		{`(char= c #\Space)`, `#\Space`, Constant, fpWhole},
		{`(eql c #\() (car x)`, `#\(`, Constant, fpWhole},
		{`(eql c #\") (car x)`, "car", Function, fpWhole},
		{`(eql c #\;) (car x)`, "car", Function, fpWhole},
		// Strings and comments.
		{`(print "a \"b\" c")`, `"a \"b\" c"`, String, fpWhole},
		{"(car x) ; note", "; note", Comment, fpWhole},
		{";; heading", ";; heading", Comment, fpWhole},
		{"(f #| inline |# x)", "#| inline |#", Comment, fpWhole},
		{"#!/usr/bin/sbcl --script", "#!/usr/bin/sbcl --script", Comment, fpWhole},
		// Numbers: signed, ratios, exponent markers, other radixes.
		{"(- -1 +2)", "-1", Number, fpWhole},
		{"(- -1 +2)", "+2", Number, fpWhole},
		{"(/ 1/2 -3/4)", "1/2", Number, fpWhole},
		{"(/ 1/2 -3/4)", "-3/4", Number, fpWhole},
		{"(* 1.5d0 x .25)", "1.5d0", Number, fpWhole},
		{"(* 1.5d0 x .25)", ".25", Number, fpWhole},
		{"(logior #x1F #b101 #o17 #36rZZ)", "#x1F", Number, fpWhole},
		{"(logior #x1F #b101 #o17 #36rZZ)", "#b101", Number, fpWhole},
		{"(logior #x1F #b101 #o17 #36rZZ)", "#36rZZ", Number, fpWhole},
		// A read-time conditional.
		{"#+sbcl (require :sb-posix)", "#+sbcl", Keyword, fpWhole},
	})
	fpDoc(t, lx, "#| outer #| inner |# still\nin the outer |# (car x)\n",
		fpLine{0, "still", Comment},
		fpLine{1, "in the outer", Comment},
		fpLine{1, "car", Function})
	fpDoc(t, lx, "(defun f ()\n  \"First; no comment\nthen (car x)\"\n  (car x))\n",
		fpLine{1, "no comment", String},
		fpLine{2, "then", String},
		fpLine{3, "car", Function})
}

func TestScheme(t *testing.T) {
	fpDetects(t, "scheme", []string{"a.scm", "a.ss", "a.sld", "a.sls", "a.rkt", "a.rktl"},
		[]string{"#!/usr/bin/env guile", "#!/usr/bin/guile3.0 -s", "#!/usr/bin/env racket",
			"#!/usr/local/bin/csi -s", "#!/usr/bin/env gsi", "#!/usr/bin/env chez --script"})
	fpComment(t, "scheme", ";;", "")
	lx := lang(t, "scheme")
	fpToks(t, lx, []fpTok{
		{"(define (square x) (* x x))", "define", Keyword, fpWhole},
		{"(define (square x) (* x x))", "square", Function, fpWhole},
		// define names a function only as (define (f x) ...).
		{"(define counter 0)", "counter", Plain, fpPart},
		{"(set! counter 1)", "set!", Keyword, fpWhole},
		{"(define-syntax swap! (syntax-rules () ((_ a b) b)))", "swap!", Function, fpWhole},
		{"(define-record-type point (make-point x) point?)", "point (", Type, fpPart},
		{"(λ (x) x)", "λ", Keyword, fpWhole},
		{"(string-append a b)", "string-append", Function, fpWhole},
		{"(1+ x)", "1+", Function, fpWhole},
		{"(if #t #f)", "#t", Constant, fpWhole},
		{"(if #t #f)", "#f", Constant, fpWhole},
		{"(list #true #false)", "#true", Constant, fpWhole},
		{"(list +inf.0 -nan.0)", "+inf.0", Constant, fpWhole},
		{"(f #:key 1)", "#:key", Constant, fpWhole},
		{"(eq? x 'sym)", "'sym", Constant, fpWhole},
		{`(list #\a #\space)`, `#\space`, Constant, fpWhole},
		{`(list #\( (car x))`, "car", Function, fpWhole},
		{`(display "a \"b\" c")`, `"a \"b\" c"`, String, fpWhole},
		{`(regexp-match #rx"a\"b+" s)`, `#rx"a\"b+"`, String, fpWhole},
		{`(regexp-match #px"\\d+" s)`, `#px"\\d+"`, String, fpWhole},
		{"(car x) ; note", "; note", Comment, fpWhole},
		{";; heading", ";; heading", Comment, fpWhole},
		{"(f #| inline |# x)", "#| inline |#", Comment, fpWhole},
		// #; comments out one datum, a list or an atom, and no more.
		{`#;(display "gone" (nested x)) (display "kept")`, `#;(display "gone" (nested x))`, Comment, fpWhole},
		{`#;(display "gone" (nested x)) (display "kept")`, `"kept"`, String, fpWhole},
		{"#; skipped kept", "#; skipped", Comment, fpWhole},
		{"#!/usr/bin/env guile", "#!/usr/bin/env guile", Comment, fpWhole},
		{"#lang racket/base", "#lang", Keyword, fpWhole},
		{"#!r6rs", "#!r6rs", Keyword, fpWhole},
		{"(+ 1/2 -3.5e2)", "1/2", Number, fpWhole},
		{"(+ 1/2 -3.5e2)", "-3.5e2", Number, fpWhole},
		{"(+ #x1F #b101)", "#x1F", Number, fpWhole},
		{"(+ #e1.5 1)", "#e1.5", Number, fpWhole},
	})
	fpDoc(t, lx, "#| outer #| inner |# still\nin the outer |# (car x)\n",
		fpLine{0, "still", Comment},
		fpLine{1, "in the outer", Comment},
		fpLine{1, "car", Function})
	fpDoc(t, lx, "(display \"first; no comment\nsecond (car x)\")\n(car x)\n",
		fpLine{0, "no comment", String},
		fpLine{1, "second", String},
		fpLine{2, "car", Function})
	fpDoc(t, lx, "(regexp-match #px\"a\nb (c)\" s)\n(car x)\n",
		fpLine{1, "b (c)", String},
		fpLine{2, "car", Function})
}

func TestClojure(t *testing.T) {
	fpDetects(t, "clojure", []string{"core.clj", "core.cljs", "core.cljc", "deps.edn", "build.bb"},
		[]string{"#!/usr/bin/env bb"})
	fpComment(t, "clojure", ";;", "")
	lx := lang(t, "clojure")
	fpToks(t, lx, []fpTok{
		{"(defn square [x] (* x x))", "defn", Keyword, fpWhole},
		{"(defn square [x] (* x x))", "square", Function, fpWhole},
		{"(defn- helper [] 1)", "defn-", Keyword, fpWhole},
		{"(defn- helper [] 1)", "helper", Function, fpWhole},
		{"(ns my.app (:require [clojure.string :as str]))", "ns", Keyword, fpWhole},
		{"(ns my.app (:require [clojure.string :as str]))", ":require", Constant, fpWhole},
		{"(if-let [x y] x nil)", "if-let", Keyword, fpWhole},
		{"(if-let [x y] x nil)", "nil", Constant, fpWhole},
		{"(assoc m :k true)", "true", Constant, fpWhole},
		{"(get m ::key)", "::key", Constant, fpWhole},
		{"(get m :user/id)", ":user/id", Constant, fpWhole},
		// A name is read whole, and the one after ( is called.
		{"(swap! state update-in [:k] inc)", "swap!", Function, fpWhole},
		{"(swap! state update-in [:k] inc)", "update-in", Plain, fpPart},
		{"(x' 1)", "x'", Function, fpWhole},
		{"(= x 'sym)", "'sym", Constant, fpWhole},
		{"(alter-var-root #'my-fn f)", "#'my-fn", Function, fpWhole},
		{"#?(:clj 1 :cljs 2)", "#?", Keyword, fpWhole},
		// Characters, a quote or ; among them.
		{`(str \a \newline)`, `\a`, Constant, fpWhole},
		{`(str \a \newline)`, `\newline`, Constant, fpWhole},
		{`(str \" (inc x))`, "inc", Function, fpWhole},
		{`(str \; (inc x))`, "inc", Function, fpWhole},
		{`(println "a \"b\" c")`, `"a \"b\" c"`, String, fpWhole},
		{`(re-find #"\d+\"x" s)`, `#"\d+\"x"`, String, fpWhole},
		{"(inc x) ; note", "; note", Comment, fpWhole},
		{";; heading", ";; heading", Comment, fpWhole},
		{"#!/usr/bin/env bb", "#!/usr/bin/env bb", Comment, fpWhole},
		// #_ discards one form, a list or an atom, and no more.
		{"#_(ignored (form)) (kept)", "#_(ignored (form))", Comment, fpWhole},
		{"#_(ignored (form)) (kept)", "kept", Function, fpWhole},
		{"#_ skipped kept", "#_ skipped", Comment, fpWhole},
		{"(+ 1/2 2r1010 0xFF -2)", "1/2", Number, fpWhole},
		{"(+ 1/2 2r1010 0xFF -2)", "2r1010", Number, fpWhole},
		{"(+ 1/2 2r1010 0xFF -2)", "0xFF", Number, fpWhole},
		{"(+ 1/2 2r1010 0xFF -2)", "-2", Number, fpWhole},
		{"(* 1N 1.5M)", "1N", Number, fpWhole},
		{"(* 1N 1.5M)", "1.5M", Number, fpWhole},
		{"(/ 1 ##Inf)", "##Inf", Constant, fpWhole},
		// A class or a record, unless it is part of a call.
		{"(defrecord Point [x y])", "Point", Type, fpWhole},
		{"(fn [^String s] s)", "String", Type, fpWhole},
		{"(Math/abs -1)", "Math/abs", Function, fpWhole},
		{"(Date.)", "Date.", Function, fpWhole},
	})
	fpDoc(t, lx, "(defn f\n  \"Doc; first\n  second\"\n  [x] (inc x))\n",
		fpLine{1, "first", String},
		fpLine{2, "second", String},
		fpLine{3, "inc", Function})
	fpDoc(t, lx, "(re-find #\"a\nb (c)\" s)\n(inc x)\n",
		fpLine{1, "b (c)", String},
		fpLine{2, "inc", Function})
}

func TestElisp(t *testing.T) {
	fpDetects(t, "elisp", []string{"init.el", "/home/me/.emacs", "A.EL"}, nil)
	fpComment(t, "elisp", ";;", "")
	lx := lang(t, "elisp")
	fpToks(t, lx, []fpTok{
		{"(defun my-fn (arg) arg)", "defun", Keyword, fpWhole},
		{"(defun my-fn (arg) arg)", "my-fn", Function, fpWhole},
		{"(when x (setq y 1))", "when", Keyword, fpWhole},
		{"(when x (setq y 1))", "setq", Keyword, fpWhole},
		{"(defun f (&optional arg) arg)", "&optional", Keyword, fpWhole},
		{`(define-minor-mode my-mode "Doc.")`, "my-mode", Function, fpWhole},
		{"(cl-defstruct point x y)", "point", Type, fpWhole},
		{"(defconst my-limit 42)", "my-limit", Constant, fpWhole},
		{"(if x t nil)", "t", Constant, fpWhole},
		{"(if x t nil)", "nil", Constant, fpWhole},
		{`(defcustom my-opt t "Doc." :type 'boolean)`, ":type", Constant, fpWhole},
		{`(defcustom my-opt t "Doc." :type 'boolean)`, "'boolean", Constant, fpWhole},
		{"(add-hook 'after-init-hook #'my-mode)", "#'my-mode", Function, fpWhole},
		// A name is read whole, ? and all, and the one after ( is called.
		{"(null? x)", "null?", Function, fpWhole},
		{"(1+ n)", "1+", Function, fpWhole},
		{"(list when-done)", "when-done", Plain, fpPart},
		// Characters, a quote or ; among them.
		{"(insert ?a)", "?a", Constant, fpWhole},
		{`(global-set-key ?\C-x f)`, `?\C-x`, Constant, fpWhole},
		{`(list ?\M-\C-a)`, `?\M-\C-a`, Constant, fpWhole},
		{`(list ?\^I)`, `?\^I`, Constant, fpWhole},
		{`(eq c ?\() (car x)`, "car", Function, fpWhole},
		{`(eq c ?\") (car x)`, "car", Function, fpWhole},
		{`(eq c ?;) (car x)`, "car", Function, fpWhole},
		{`(message "a \"b\" c")`, `"a \"b\" c"`, String, fpWhole},
		{"(car x) ; note", "; note", Comment, fpWhole},
		{";;;###autoload", ";;;###autoload", Comment, fpWhole},
		{"(+ -1.5e3 42)", "-1.5e3", Number, fpWhole},
		{"(+ #x1F #b101 #24r1k)", "#x1F", Number, fpWhole},
		{"(+ #x1F #b101 #24r1k)", "#24r1k", Number, fpWhole},
	})
	fpDoc(t, lx, "(defun f ()\n  \"First; no comment\nsecond (car x)\"\n  (car x))\n",
		fpLine{1, "no comment", String},
		fpLine{2, "second", String},
		fpLine{3, "car", Function})
}

func TestHaskell(t *testing.T) {
	fpDetects(t, "haskell", []string{"Main.hs", "A.HS"},
		[]string{"#!/usr/bin/env runhaskell", "#!/usr/bin/env runghc", "#!/usr/bin/env stack"})
	fpComment(t, "haskell", "--", "")
	lx := lang(t, "haskell")
	fpToks(t, lx, []fpTok{
		{"module Main (main) where", "module", Keyword, fpWhole},
		{"module Main (main) where", "where", Keyword, fpWhole},
		{"data Shape = Circle Double", "data", Keyword, fpWhole},
		{"data Shape = Circle Double", "Shape", Type, fpWhole},
		{"data Shape = Circle Double", "Circle", Type, fpWhole},
		{"import qualified Data.Map as M", "qualified", Keyword, fpWhole},
		{"import qualified Data.Map as M", "as", Keyword, fpWhole},
		// as is a keyword only before a module's name.
		{"f = zip as bs", "as", Plain, fpPart},
		{"ok = True && False", "True", Constant, fpWhole},
		// The name a type signature is for.
		{"area :: Shape -> Double", "area", Function, fpWhole},
		{"    go :: Int -> Int", "go", Function, fpWhole},
		{"f = foldl' (+) 0", "foldl'", Plain, fpPart},
		// A string, and a character, which ' after a name is not.
		{`s = "say \"hi\" -- not a comment"`, `"say \"hi\" -- not a comment"`, String, fpWhole},
		{"c = 'a'", "'a'", String, fpWhole},
		{`c = '\''`, `'\''`, String, fpWhole},
		{"x' = f' 'b'", "'b'", String, fpWhole},
		// Two or more dashes start a comment; in a run of other symbols
		// they are an operator.
		{"x = 1 -- note", "-- note", Comment, fpWhole},
		{"x = 1 --note", "--note", Comment, fpWhole},
		{"-- | Haddock", "-- | Haddock", Comment, fpWhole},
		{"---------", "---------", Comment, fpWhole},
		{"x = a --> b", "-->", Operator, fpWhole},
		{"x = a --> b", "b", Plain, fpPart},
		{"x = a |-- b", "|--", Operator, fpWhole},
		{"x = a <-- b", "<--", Operator, fpWhole},
		{"x = {- inline -} 1", "{- inline -}", Comment, fpWhole},
		{"{-# LANGUAGE GADTs #-}", "{-# LANGUAGE GADTs #-}", Keyword, fpWhole},
		{"#if MIN_VERSION_base(4,8,0)", "#if", Keyword, fpWhole},
		{"#!/usr/bin/env runhaskell", "#!/usr/bin/env runhaskell", Comment, fpWhole},
		{"y = [2..10]", "2", Number, fpWhole},
		{"y = 0x1F + 0b101 + 1_000 + 1.5e3", "0x1F", Number, fpWhole},
		{"y = 0x1F + 0b101 + 1_000 + 1.5e3", "0b101", Number, fpWhole},
		{"y = 0x1F + 0b101 + 1_000 + 1.5e3", "1_000", Number, fpWhole},
		{"y = 0x1F + 0b101 + 1_000 + 1.5e3", "1.5e3", Number, fpWhole},
	})
	fpDoc(t, lx, "{- outer {- inner -} still\nin the outer -} x = 1\n",
		fpLine{0, "still", Comment},
		fpLine{1, "in the outer", Comment},
		fpLine{1, "1", Number})
	fpDoc(t, lx, "{-# LANGUAGE OverloadedStrings,\n             GADTs #-}\nmodule Main where\n",
		fpLine{1, "GADTs", Keyword},
		fpLine{2, "module", Keyword},
		fpLine{2, "Main", Type})
}

func TestOCaml(t *testing.T) {
	fpDetects(t, "ocaml", []string{"main.ml", "main.mli", "lexer.mll", "parser.mly"},
		[]string{"#!/usr/bin/env ocaml"})
	fpComment(t, "ocaml", "(*", "*)")
	lx := lang(t, "ocaml")
	fpToks(t, lx, []fpTok{
		// A let that takes arguments defines a function.
		{"let rec fact n = n", "let", Keyword, fpWhole},
		{"let rec fact n = n", "rec", Keyword, fpWhole},
		{"let rec fact n = n", "fact", Function, fpWhole},
		{"let x = 1", "x", Plain, fpPart},
		{"let f () = 1", "f", Function, fpWhole},
		{"let f x = x and g y = y", "g", Function, fpWhole},
		{"let open List in xs", "open", Keyword, fpWhole},
		{"match x with Some y -> y", "match", Keyword, fpWhole},
		{"match x with Some y -> y", "Some", Type, fpWhole},
		{"type shape = Circle", "shape", Type, fpWhole},
		{"type 'a tree = Leaf", "'a", Type, fpWhole},
		{"let n : int list = []", "int", Type, fpWhole},
		{"let v = `Red", "`Red", Constant, fpWhole},
		{"let b = true", "true", Constant, fpWhole},
		{`external size : int -> int = "c_len"`, "size", Function, fpWhole},
		{"raise Not_found", "raise", Function, fpWhole},
		// A character, and a type variable, which is not one.
		{"let c = 'a'", "'a'", String, fpWhole},
		{`let c = '\''`, `'\''`, String, fpWhole},
		{`let q = '"' in x`, `'"'`, String, fpWhole},
		{`let q = '"' in x`, "in", Keyword, fpWhole},
		{`let s = "a \"b\" (* c *)"`, `"a \"b\" (* c *)"`, String, fpWhole},
		// ( * ) is the operator; (* opens a comment.
		{"let m = ( * ) 2 3", "*", Operator, fpWhole},
		{"let x = 1 (* note *)", "(* note *)", Comment, fpWhole},
		// A quoted string closes only at its own id.
		{`let s = {|raw "x"|}`, `{|raw "x"|}`, String, fpWhole},
		{`let s = {id|a |} b|id} ^ t`, `{id|a |} b|id}`, String, fpWhole},
		{`let s = {%ext|x|} ^ t`, `{%ext|x|}`, String, fpWhole},
		{"type t = { x : int } [@@deriving show]", "[@@deriving", Keyword, fpWhole},
		{`#use "topfind";;`, "#use", Keyword, fpWhole},
		{"%token <int> INT", "%token", Keyword, fpWhole},
		{"let n = 0x1F + 1_000", "0x1F", Number, fpWhole},
		{"let n = 0x1F + 1_000", "1_000", Number, fpWhole},
		{"let f = 3.14e-2 +. 42L", "3.14e-2", Number, fpWhole},
		{"let f = 3.14e-2 +. 42L", "42L", Number, fpWhole},
	})
	fpDoc(t, lx, "(* outer (* inner *) still\n   in the outer *) let x = 1\n",
		fpLine{0, "still", Comment},
		fpLine{1, "in the outer", Comment},
		fpLine{1, "let", Keyword})
	fpDoc(t, lx, "let q = {sql|\n  select * from t |}\n|sql} ^ s\n",
		fpLine{1, "select", String},
		fpLine{1, "|}", String},
		fpLine{2, "^", Operator})
	fpDoc(t, lx, "let s = \"first\nsecond (* x *)\" in s\n",
		fpLine{1, "second", String},
		fpLine{1, "(* x *)", String},
		fpLine{1, "in", Keyword})
}

func TestElixir(t *testing.T) {
	fpDetects(t, "elixir", []string{"app.ex", "app_test.exs"}, []string{"#!/usr/bin/env elixir"})
	fpComment(t, "elixir", "#", "")
	lx := lang(t, "elixir")
	fpToks(t, lx, []fpTok{
		{"defmodule MyApp.Worker do", "defmodule", Keyword, fpWhole},
		{"defmodule MyApp.Worker do", "Worker", Type, fpWhole},
		{"  def run(x) when is_integer(x) do", "run", Function, fpWhole},
		{"  def run(x) when is_integer(x) do", "when", Keyword, fpWhole},
		{"  def go, do: :ok", "go", Function, fpWhole},
		// Atoms: :ok, a keyword list's do:, a quoted one.
		{"  def go, do: :ok", "do:", Constant, fpWhole},
		{"  def go, do: :ok", ":ok", Constant, fpWhole},
		{"m = %{a: 1, valid?: true}", "valid?:", Constant, fpWhole},
		{"m = %{a: 1, valid?: true}", "true", Constant, fpWhole},
		{`:"quoted atom"`, `:"quoted atom"`, Constant, fpWhole},
		{"x = nil", "nil", Constant, fpWhole},
		{"__MODULE__.run()", "__MODULE__", Constant, fpWhole},
		// :: is an operator, not an atom's colon.
		{"<<x::binary>>", "::", Operator, fpWhole},
		{"<<x::binary>>", "binary", Plain, fpPart},
		// A name ends in ? or !, and is called with it.
		{"  defp valid?(x), do: x", "valid?", Function, fpWhole},
		{"    File.read!(path)", "read!", Function, fpWhole},
		{"c = ?a", "?a", Constant, fpWhole},
		{`c = ?\n`, `?\n`, Constant, fpWhole},
		{"  @timeout 5_000", "@timeout", Constant, fpWhole},
		{"  @timeout 5_000", "5_000", Number, fpWhole},
		{`IO.puts("a \"b\" #{x}")`, `"a \"b\" #{x}"`, String, fpWhole},
		{"l = 'charlist'", "'charlist'", String, fpWhole},
		{"x = 1 # note", "# note", Comment, fpWhole},
		{"n = 0x1F + 0b1010 + 1.5e-3", "0x1F", Number, fpWhole},
		{"n = 0x1F + 0b1010 + 1.5e-3", "0b1010", Number, fpWhole},
		{"n = 0x1F + 0b1010 + 1.5e-3", "1.5e-3", Number, fpWhole},
		// Sigils, closed by their own delimiter, then any modifiers.
		{`r = ~r/foo\/bar/i`, `~r/foo\/bar/i`, String, fpWhole},
		{"w = ~w(a b c)a", "~w(a b c)a", String, fpWhole},
		{"s = ~s{x} <> y", "~s{x}", String, fpWhole},
		{"s = ~s[x] <> y", "~s[x]", String, fpWhole},
		{"s = ~s<x> <> y", "~s<x>", String, fpWhole},
		{"s = ~r|a/b| <> y", "~r|a/b|", String, fpWhole},
		{`s = ~S"a" <> y`, `~S"a"`, String, fpWhole},
	})
	fpDoc(t, lx, "  @moduledoc \"\"\"\n  Docs with \"quotes\" # not a comment\n  \"\"\"\n  def f, do: 1\n",
		fpLine{1, "Docs", String},
		fpLine{1, "# not", String},
		fpLine{3, "def", Keyword})
	fpDoc(t, lx, "x = ~S\"\"\"\n  raw \" text\n  \"\"\"\ny = 1\n",
		fpLine{1, "raw", String},
		fpLine{3, "1", Number})
	fpDoc(t, lx, "c = '''\n  text ' here\n  '''\ny = 1\n",
		fpLine{1, "text", String},
		fpLine{3, "1", Number})
	fpDoc(t, lx, "s = \"first\nsecond # x\"\ny = 1\n",
		fpLine{1, "second", String},
		fpLine{1, "# x", String},
		fpLine{2, "1", Number})
	fpDoc(t, lx, "w = ~w(\n  a b\n)\ny = 1\n",
		fpLine{1, "a b", String},
		fpLine{3, "1", Number})
}

func TestErlang(t *testing.T) {
	fpDetects(t, "erlang", []string{"app.erl", "app.hrl", "run.escript"}, []string{"#!/usr/bin/env escript"})
	fpComment(t, "erlang", "%%", "")
	lx := lang(t, "erlang")
	fpToks(t, lx, []fpTok{
		{"-module(my_mod).", "-module", Keyword, fpWhole},
		{"-module(my_mod).", "my_mod", Constant, fpWhole},
		{"-export([start/0]).", "start", Function, fpWhole},
		{"start() ->", "start", Function, fpWhole},
		{"case X of ok -> Y end", "case", Keyword, fpWhole},
		{"case X of ok -> Y end", "end", Keyword, fpWhole},
		{"f() when A andalso B -> ok", "andalso", Keyword, fpWhole},
		// A name in lower case is an atom; a keyword inside one is not a
		// keyword, and a variable is plain.
		{"case X of ok -> Y end", "ok", Constant, fpWhole},
		{"case X of ok -> Y end", "X", Plain, fpPart},
		{"endgame", "endgame", Constant, fpWhole},
		{"R = true", "true", Constant, fpWhole},
		{"X = 'quoted atom'", "'quoted atom'", Constant, fpWhole},
		{"gen_server:call(Pid, get)", "gen_server", Type, fpWhole},
		{"gen_server:call(Pid, get)", "call", Function, fpWhole},
		{"gen_server:call(Pid, get)", "get", Constant, fpWhole},
		{"F = fun lists:reverse/1", "fun", Keyword, fpWhole},
		{"F = fun lists:reverse/1", "reverse", Function, fpWhole},
		{"M = ?MODULE", "?MODULE", Constant, fpWhole},
		{"?assertEqual(1, X)", "?assertEqual", Function, fpWhole},
		// Characters, a quote among them.
		{"C = $a", "$a", Constant, fpWhole},
		{`C = $\n`, `$\n`, Constant, fpWhole},
		{`[$", ok]`, "ok", Constant, fpWhole},
		{`S = "a \"b\" c"`, `"a \"b\" c"`, String, fpWhole},
		{"X = 1 % note", "% note", Comment, fpWhole},
		{"%% heading", "%% heading", Comment, fpWhole},
		{"%%! -smp enable", "%%! -smp enable", Comment, fpWhole},
		{"#!/usr/bin/env escript", "#!/usr/bin/env escript", Comment, fpWhole},
		{"N = 16#FF + 2#1010", "16#FF", Number, fpWhole},
		{"N = 16#FF + 2#1010", "2#1010", Number, fpWhole},
		{"F = 1.5e3 + 1_000", "1.5e3", Number, fpWhole},
		{"F = 1.5e3 + 1_000", "1_000", Number, fpWhole},
	})
	fpDoc(t, lx, "S = \"first\nsecond % x\", ok.\n",
		fpLine{1, "second", String},
		fpLine{1, "% x", String},
		fpLine{1, "ok", Constant})
	fpDoc(t, lx, "T = \"\"\"\n  triple \"quoted\" \\\n  \"\"\", ok.\n",
		fpLine{1, "triple", String},
		fpLine{2, "ok", Constant})
}

func TestGleam(t *testing.T) {
	fpDetects(t, "gleam", []string{"app.gleam", "APP.GLEAM"}, nil)
	fpComment(t, "gleam", "//", "")
	lx := lang(t, "gleam")
	fpToks(t, lx, []fpTok{
		{"pub fn main() {", "pub", Keyword, fpWhole},
		{"pub fn main() {", "fn", Keyword, fpWhole},
		{"pub fn main() {", "main", Function, fpWhole},
		{"use item <- list.map(items)", "use", Keyword, fpWhole},
		{"use item <- list.map(items)", "map", Function, fpWhole},
		{"let ok = True && False", "True", Constant, fpWhole},
		{"let ok = True && False", "False", Constant, fpWhole},
		{"let n = Nil", "Nil", Constant, fpWhole},
		{"pub type Shape { Circle(radius: Float) }", "Shape", Type, fpWhole},
		{"pub type Shape { Circle(radius: Float) }", "Circle", Type, fpWhole},
		{"pub type Shape { Circle(radius: Float) }", "Float", Type, fpWhole},
		{`@external(erlang, "lists", "reverse")`, "@external", Keyword, fpWhole},
		{`let s = "say \"hi\""`, `"say \"hi\""`, String, fpWhole},
		{"let x = 1 // note", "// note", Comment, fpWhole},
		{"/// A function's docs.", "/// A function's docs.", Comment, fpWhole},
		{"//// The module's docs.", "//// The module's docs.", Comment, fpWhole},
		{"let n = 0x1F + 0b101 + 0o17", "0x1F", Number, fpWhole},
		{"let n = 0x1F + 0b101 + 0o17", "0o17", Number, fpWhole},
		{"let f = 1_000.5e3", "1_000.5e3", Number, fpWhole},
		{`todo as "later"`, "todo", Keyword, fpWhole},
	})
	fpDoc(t, lx, "let s = \"first\nsecond // x\"\nlet y = 1\n",
		fpLine{1, "second", String},
		fpLine{1, "// x", String},
		fpLine{2, "let", Keyword})
}

func TestElm(t *testing.T) {
	fpDetects(t, "elm", []string{"Main.elm", "MAIN.ELM"}, nil)
	fpComment(t, "elm", "--", "")
	lx := lang(t, "elm")
	fpToks(t, lx, []fpTok{
		{"module Main exposing (main)", "module", Keyword, fpWhole},
		{"module Main exposing (main)", "exposing", Keyword, fpWhole},
		{"module Main exposing (main)", "Main", Type, fpWhole},
		{"import Json.Decode as D", "as", Keyword, fpWhole},
		{"type alias Model = { name : String }", "alias", Keyword, fpWhole},
		{"type alias Model = { name : String }", "Model", Type, fpWhole},
		{"type alias Model = { name : String }", "name", Plain, fpPart},
		// The name a type annotation is for; :: is not one.
		{"update : Msg -> Model -> Model", "update", Function, fpWhole},
		{"    helper : Int -> Int", "helper", Function, fpWhole},
		{"    first :: rest", "first", Plain, fpPart},
		{"case msg of", "of", Keyword, fpWhole},
		{"ok = True", "True", Constant, fpWhole},
		{`s = "a \"b\""`, `"a \"b\""`, String, fpWhole},
		{"c = 'x'", "'x'", String, fpWhole},
		{`c = '\''`, `'\''`, String, fpWhole},
		{"x = 1 -- note", "-- note", Comment, fpWhole},
		{"x = {- inline -} 1", "{- inline -}", Comment, fpWhole},
		{"n = 0x1F + 3.14", "0x1F", Number, fpWhole},
		{"n = 0x1F + 3.14", "3.14", Number, fpWhole},
	})
	fpDoc(t, lx, "{-| Doc {- nested -} still\nin the doc -} x = 1\n",
		fpLine{0, "still", Comment},
		fpLine{1, "in the doc", Comment},
		fpLine{1, "1", Number})
	fpDoc(t, lx, "s = \"\"\"first \"quoted\"\n-- not a comment\n\"\"\"\nt = 1\n",
		fpLine{1, "-- not", String},
		fpLine{3, "1", Number})
}
