// Command nem is a terminal text editor with nano's shape and emacs's
// keybindings.
//
// Usage:
//
//	nem [file...]
//
// Each named file is opened into a buffer; the first is shown. With no
// arguments nem starts in *scratch*.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hajianpour/nem/editor"
	"github.com/hajianpour/nem/ui"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [file...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "nem:", err)
		os.Exit(1)
	}
}

func run(paths []string) (err error) {
	scr, err := ui.NewScreen()
	if err != nil {
		return fmt.Errorf("opening terminal: %w", err)
	}

	// Restoring the terminal is not optional, and it must happen before the
	// panic is allowed to continue. A panic that unwinds past here with the
	// screen still in raw mode and the alternate buffer active leaves the
	// user's shell echoing nothing and rendering nowhere — the editor's bug
	// becomes the shell's problem. So: stop the panic, restore the terminal,
	// then re-panic so the trace prints onto a terminal that works.
	defer func() {
		r := recover()
		scr.Close()
		if r != nil {
			panic(r)
		}
	}()

	e, err := editor.New(scr)
	if err != nil {
		return err
	}

	// A broken config must not stop nem from starting: LoadConfig reports the
	// failure in the echo area and leaves built-in defaults in place, so the
	// error is deliberately not propagated here.
	_ = e.LoadConfig("")
	defer e.CloseConfig()

	// Only the command line knows whether a file was asked for; the editor sees
	// an empty *scratch* either way.
	if len(paths) == 0 {
		e.ShowStartup()
	}

	for i, path := range paths {
		b, err := e.OpenFile(path)
		if err != nil {
			return fmt.Errorf("opening %s: %w", path, err)
		}
		if i == 0 {
			e.Active().Visit(b)
		}
	}

	return e.Loop()
}
