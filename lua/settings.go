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
}

// DefaultSettings returns the built-in defaults, which are what the editor uses
// when there is no config file or when the config failed to load.
func DefaultSettings() Settings {
	return Settings{TabWidth: 8, ScrollMargin: 2, UndoStyle: "linear"}
}

// settingLimits bounds each numeric setting. A value outside the range is a
// config bug, and saying so beats a crash or a silently unusable editor: a tab
// width of zero divides by zero in layout, and a negative one is meaningless.
const (
	minTabWidth, maxTabWidth       = 1, 64
	minScrollMargin, maxScrollMrgn = 0, 1000
)

// knownSettings lists every recognised key, so nem.set can name the valid ones
// when a script misspells something. A silently ignored setting is the worst
// outcome here: the user reads their config, sees the line, and cannot work out
// why it has no effect.
var knownSettings = []string{"scroll-margin", "tab-width", "undo-style"}

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
