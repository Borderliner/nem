package lua

import (
	"errors"
	"strings"

	glua "github.com/yuin/gopher-lua"
)

// ScriptError is a failure inside a config script, carrying the Lua traceback.
//
// The traceback is the whole point: a config error reported as "attempt to index
// a nil value" without a location is nearly useless to someone with a fifty-line
// init.lua, and the editor shows Error() in the echo area.
type ScriptError struct {
	// Msg is the Lua error message.
	Msg string

	// Traceback is the Lua stack at the point of failure, empty if unavailable.
	Traceback string

	// Kind names the failure class gopher-lua reported, such as a syntax error
	// or a runtime panic, for callers that want to distinguish them.
	Kind string
}

func (e *ScriptError) Error() string {
	if e.Traceback == "" {
		return e.Msg
	}
	return e.Msg + "\n" + e.Traceback
}

// Brief returns the message without the traceback, for a one-line echo area.
func (e *ScriptError) Brief() string { return e.Msg }

// asScriptError converts a gopher-lua error into a *ScriptError, preserving the
// traceback. Anything that is not a Lua error passes through unchanged so that a
// Go-side failure is not mislabelled as a script bug.
func asScriptError(err error) error {
	if err == nil {
		return nil
	}
	var api *glua.ApiError
	if !errors.As(err, &api) {
		return err
	}
	msg := api.Object.String()
	if api.Cause != nil && msg == "" {
		msg = api.Cause.Error()
	}
	return &ScriptError{
		Msg:       strings.TrimSpace(msg),
		Traceback: strings.TrimSpace(api.StackTrace),
		Kind:      apiErrorKind(api.Type),
	}
}

func apiErrorKind(t glua.ApiErrorType) string {
	switch t {
	case glua.ApiErrorSyntax:
		return "syntax"
	case glua.ApiErrorFile:
		return "file"
	case glua.ApiErrorRun:
		return "runtime"
	case glua.ApiErrorError:
		return "error"
	case glua.ApiErrorPanic:
		return "panic"
	}
	return "unknown"
}
