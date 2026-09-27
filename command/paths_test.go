package command_test

import (
	"path/filepath"
	"testing"

	"github.com/Borderliner/nem/command"
)

func TestRestartPath(t *testing.T) {
	for in, want := range map[string]string{
		"src/main.go":        "src/main.go",
		"src/~/notes.txt":    "~/notes.txt",
		"/a/b//etc/hosts":    "/etc/hosts",
		"~/x/~/y":            "~/y",
		"a//b/~/c":           "~/c",
		"~/":                 "~/",
		"/home/me/file~name": "/home/me/file~name",
	} {
		if got := command.RestartPath(in); got != want {
			t.Errorf("RestartPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		"~":            home,
		"~/":           home + string(filepath.Separator),
		"~/a/b.txt":    filepath.Join(home, "a", "b.txt"),
		"~/a/":         filepath.Join(home, "a") + string(filepath.Separator),
		"src/~/x.go":   filepath.Join(home, "x.go"),
		"plain.txt":    "plain.txt",
		"~nosuchuser/": "~nosuchuser/",
	} {
		if got := command.ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}
