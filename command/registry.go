package command

import (
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrEmptyName rejects a command with no name.
	ErrEmptyName = errors.New("command name is empty")

	// ErrNilFunc rejects a command with no implementation.
	ErrNilFunc = errors.New("command has no function")

	// ErrDuplicateCommand rejects a second registration of the same name.
	ErrDuplicateCommand = errors.New("command already registered")
)

// Command is one named, invocable operation.
//
// The name is the single identity a command has: M-x searches these names, the
// Lua config binds keys to them, and describe-bindings reports them. Keep them
// in emacs's kebab-case ("kill-line", not "KillLine") so a user's existing
// muscle memory for M-x transfers.
type Command struct {
	// Name is the command's identity, in kebab-case.
	Name string

	// Doc is a one-line description shown by describe-key and M-x.
	Doc string

	// Fn implements the command.
	Fn Func

	// Interactive reports whether the command appears in M-x completion.
	// Commands driven only by the event loop, such as self-insert-command,
	// are registered but not interactive.
	Interactive bool
}

// Registry holds every command the editor knows.
//
// It is the single table with three consumers: M-x searches it, the Lua config
// binds keys against it, and the universal argument feeds it. Lua-defined
// commands register here alongside the built-ins and are indistinguishable
// from them at the call site.
//
// Registry is not safe for concurrent use. The editor registers at startup and
// runs commands from the input goroutine only, and Lua config reloads happen
// on that same goroutine.
type Registry struct {
	byName map[string]Command
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Command)}
}

// Register adds a command. It rejects an empty name, a nil Fn, and a name
// already registered, naming the offender in every case so a bad Lua config
// says which command it got wrong.
func (r *Registry) Register(c Command) error {
	if c.Name == "" {
		return ErrEmptyName
	}
	if c.Fn == nil {
		return fmt.Errorf("%w: %q", ErrNilFunc, c.Name)
	}
	if _, dup := r.byName[c.Name]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicateCommand, c.Name)
	}
	r.byName[c.Name] = c
	return nil
}

// Lookup returns the command registered under name.
func (r *Registry) Lookup(name string) (Command, bool) {
	c, ok := r.byName[name]
	return c, ok
}

// Names lists the interactive command names, sorted.
//
// Sorted because it feeds M-x completion, where a non-deterministic order
// would reshuffle the candidate list between invocations.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for name, c := range r.byName {
		if c.Interactive {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// All returns every registered command, interactive or not, in unspecified
// order. describe-bindings uses this to resolve names to documentation.
func (r *Registry) All() []Command {
	all := make([]Command, 0, len(r.byName))
	for _, c := range r.byName {
		all = append(all, c)
	}
	return all
}

// Run looks up name and invokes it with e, returning ErrUnknownCommand if no
// such command is registered and otherwise whatever the command returned.
func (r *Registry) Run(name string, e Env) error {
	c, ok := r.byName[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCommand, name)
	}
	return c.Fn(e)
}
