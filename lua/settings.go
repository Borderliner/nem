package lua

import (
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// Settings holds the values a config script may change with nem.set.
//
// The host validates and stores them here rather than writing them straight
// into their eventual homes. Two reasons: text.TabWidth is a package-level
// variable, so a script mutating it directly would make settings global state
// that tests cannot isolate; and the editor needs to know when a setting
// changed so it can invalidate layout caches and redraw. The editor therefore
// reads Settings after loading the config and applies them itself.
type Settings struct {
	// TabWidth is the display width of a tab stop.
	TabWidth int

	// ScrollMargin is how many lines of context to keep above and below point.
	ScrollMargin int

	// UndoStyle selects the undo model. Only "linear" exists; the field is here
	// because the setting is documented, so an unknown value must be rejected
	// with a useful message rather than silently accepted.
	UndoStyle string

	// CompletionStyle is "popup" (a centred panel) or "bottom" (emacs-shaped
	// rows above the echo line). Same prompt state either way; only the
	// renderer differs.
	CompletionStyle string

	// CompletionRows is how many candidates a completion panel shows at once.
	CompletionRows int

	// WhichKeyDelay is how long a prefix must stay pending before the
	// continuation panel appears, in milliseconds. Zero disables it.
	WhichKeyDelay int

	// AutosaveIdle is how many seconds of idleness trigger an autosave of every
	// modified buffer. Zero disables it.
	AutosaveIdle int

	// Backup says whether to keep a copy of a file's previous contents the first
	// time it is saved in a session.
	Backup bool

	// Clipboard is "osc52" or "off". When on, kills also reach the system
	// clipboard.
	Clipboard string

	// LineNumbers says whether each window shows a line-number gutter.
	LineNumbers bool

	// DeleteSelection makes typing or deleting replace an active region, as
	// every modern editor does. Emacs ships this off; nem ships it on.
	DeleteSelection bool

	// Syntax says whether code is coloured.
	Syntax bool

	// Theme picks the syntax palette: "dark", "light", or "auto" to guess from
	// the terminal. One palette cannot serve both grounds - colours with enough
	// contrast on black wash out on white - so nem carries two.
	Theme string
}

// DefaultSettings returns the built-in defaults, which are what the editor uses
// when there is no config file or when the config failed to load.
func DefaultSettings() Settings {
	return Settings{
		TabWidth: 8, ScrollMargin: 2, UndoStyle: "linear",
		CompletionStyle: "popup", CompletionRows: 10,
		WhichKeyDelay: 300, AutosaveIdle: 30,
		Backup: true, Clipboard: "osc52", LineNumbers: true,
		DeleteSelection: true,
		Syntax:          true, Theme: "auto",
	}
}

// settingLimits bounds each numeric setting. A value outside the range is a
// config bug, and saying so beats a crash or a silently unusable editor: a tab
// width of zero divides by zero in layout, and a negative one is meaningless.
const (
	minTabWidth, maxTabWidth       = 1, 64
	minScrollMargin, maxScrollMrgn = 0, 1000
	minCompRows, maxCompRows       = 1, 200
	minWhichKey, maxWhichKey       = 0, 10000 // ms; 0 disables
	minAutosave, maxAutosave       = 0, 3600  // seconds; 0 disables
)

// knownSettings lists every recognised key, so nem.set can name the valid ones
// when a script misspells something. A silently ignored setting is the worst
// outcome here: the user reads their config, sees the line, and cannot work out
// why it has no effect.
var knownSettings = []string{
	"autosave-idle", "backup", "clipboard", "completion-rows", "completion-style",
	"delete-selection", "line-numbers", "scroll-margin", "syntax", "tab-width",
	"theme", "undo-style", "which-key-delay",
}

// set validates one key/value pair and stores it.
func (s *Settings) set(key string, v glua.LValue) error {
	switch key {
	case "tab-width":
		n, err := checkInt(key, v)
		if err != nil {
			return err
		}
		if n < minTabWidth || n > maxTabWidth {
			return fmt.Errorf("tab-width must be between %d and %d, got %d", minTabWidth, maxTabWidth, n)
		}
		s.TabWidth = n
	case "scroll-margin":
		n, err := checkInt(key, v)
		if err != nil {
			return err
		}
		if n < minScrollMargin || n > maxScrollMrgn {
			return fmt.Errorf("scroll-margin must be between %d and %d, got %d", minScrollMargin, maxScrollMrgn, n)
		}
		s.ScrollMargin = n
	case "undo-style":
		str, ok := v.(glua.LString)
		if !ok {
			return fmt.Errorf("undo-style must be a string, got %s", v.Type())
		}
		if string(str) != "linear" {
			return fmt.Errorf("undo-style must be \"linear\", got %q", string(str))
		}
		s.UndoStyle = string(str)
	case "completion-style":
		str, err := checkEnum(key, v, "popup", "bottom")
		if err != nil {
			return err
		}
		s.CompletionStyle = str
	case "completion-rows":
		n, err := checkRange(key, v, minCompRows, maxCompRows)
		if err != nil {
			return err
		}
		s.CompletionRows = n
	case "which-key-delay":
		n, err := checkRange(key, v, minWhichKey, maxWhichKey)
		if err != nil {
			return err
		}
		s.WhichKeyDelay = n
	case "autosave-idle":
		n, err := checkRange(key, v, minAutosave, maxAutosave)
		if err != nil {
			return err
		}
		s.AutosaveIdle = n
	case "backup":
		b, ok := v.(glua.LBool)
		if !ok {
			return fmt.Errorf("backup must be true or false, got %s", v.Type())
		}
		s.Backup = bool(b)
	case "syntax":
		b, ok := v.(glua.LBool)
		if !ok {
			return fmt.Errorf("syntax must be true or false, got %s", v.Type())
		}
		s.Syntax = bool(b)
	case "theme":
		str, err := checkEnum(key, v, "auto", "dark", "light")
		if err != nil {
			return err
		}
		s.Theme = str
	case "line-numbers":
		b, ok := v.(glua.LBool)
		if !ok {
			return fmt.Errorf("line-numbers must be true or false, got %s", v.Type())
		}
		s.LineNumbers = bool(b)
	case "delete-selection":
		b, ok := v.(glua.LBool)
		if !ok {
			return fmt.Errorf("delete-selection must be true or false, got %s", v.Type())
		}
		s.DeleteSelection = bool(b)
	case "clipboard":
		str, err := checkEnum(key, v, "osc52", "off")
		if err != nil {
			return err
		}
		s.Clipboard = str
	default:
		return fmt.Errorf("unknown setting %q; known settings are %v", key, knownSettings)
	}
	return nil
}

// checkInt accepts a Lua number that is a whole value. Lua has one number type,
// so 4.5 arrives indistinguishably from 4 unless we check: rejecting it beats
// truncating a user's typo into a plausible-looking wrong answer.
func checkInt(key string, v glua.LValue) (int, error) {
	n, ok := v.(glua.LNumber)
	if !ok {
		return 0, fmt.Errorf("%s must be a number, got %s", key, v.Type())
	}
	if float64(n) != float64(int(n)) {
		return 0, fmt.Errorf("%s must be a whole number, got %v", key, float64(n))
	}
	return int(n), nil
}

// checkRange accepts a whole Lua number within an inclusive range. Out of range
// is a config bug, and naming the bounds beats leaving a user to guess.
func checkRange(key string, v glua.LValue, lo, hi int) (int, error) {
	n, err := checkInt(key, v)
	if err != nil {
		return 0, err
	}
	if n < lo || n > hi {
		return 0, fmt.Errorf("%s must be between %d and %d, got %d", key, lo, hi, n)
	}
	return n, nil
}

// checkEnum accepts one of a fixed set of strings, listing them on rejection so
// a typo is self-correcting.
func checkEnum(key string, v glua.LValue, allowed ...string) (string, error) {
	str, ok := v.(glua.LString)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %s", key, v.Type())
	}
	for _, a := range allowed {
		if string(str) == a {
			return a, nil
		}
	}
	return "", fmt.Errorf("%s must be one of %v, got %q", key, allowed, string(str))
}
