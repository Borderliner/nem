package keymap

import "fmt"

// dbg formats a Key's raw fields. Key implements Stringer, so %v and %+v render
// the canonical notation and hide the very fields a failing test needs to show.
func dbg(k Key) string {
	return fmt.Sprintf("{Rune:%q Special:%d Ctrl:%v Meta:%v Shift:%v}",
		k.Rune, k.Special, k.Ctrl, k.Meta, k.Shift)
}

func dbgSeq(keys []Key) string {
	out := "["
	for i, k := range keys {
		if i > 0 {
			out += " "
		}
		out += dbg(k)
	}
	return out + "]"
}
