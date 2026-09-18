package command

import "errors"

// DefaultCapacity is the number of entries the kill ring retains, matching
// emacs's kill-ring-max.
const DefaultCapacity = 60

var (
	// ErrKillRingEmpty is returned by Yank when nothing has been killed.
	ErrKillRingEmpty = errors.New("kill ring is empty")

	// ErrNotAfterYank is returned by YankPop when the preceding operation was
	// not a Yank or YankPop. Emacs reports this as "Previous command was not a
	// yank".
	ErrNotAfterYank = errors.New("previous command was not a yank")
)

// KillRing holds killed text, newest first, as a fixed-capacity ring.
//
// Two pieces of state make its behaviour emacs-like, and both are owned here
// rather than by the caller so that command implementations cannot get them
// wrong:
//
//   - A kill run. Consecutive kill commands accumulate into the single newest
//     entry instead of pushing new ones, so C-k C-k C-k then C-y restores all
//     three lines as one block. Any other command calls BreakRun to end it.
//   - A yank pointer. Yank reads the entry the pointer rests on; YankPop
//     advances it to successively older entries, wrapping around. The pointer
//     persists after a yank run ends, so a later C-y yanks from wherever M-y
//     left it, as emacs does. Pushing or extending an entry resets it.
//
// KillRing is not safe for concurrent use; the editor drives it from the input
// goroutine only.
type KillRing struct {
	entries  []string // newest at index 0
	capacity int
	inRun    bool // a kill run is in progress; further kills accumulate
	yankIdx  int  // the yank pointer, an index into entries
	yankOK   bool // YankPop is currently valid
}

// NewKillRing returns an empty ring. A capacity of zero or less is coerced to
// DefaultCapacity rather than rejected, so a bad config value degrades to the
// emacs default instead of breaking the editor.
func NewKillRing(capacity int) *KillRing {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &KillRing{capacity: capacity}
}

// Capacity reports the maximum number of entries retained.
func (k *KillRing) Capacity() int { return k.capacity }

// Len reports the number of distinct entries held.
func (k *KillRing) Len() int { return len(k.entries) }

// Kill records killed text. During a kill run the text is appended to the
// newest entry; otherwise it starts a new entry. Use it for kills with no
// inherent direction, such as kill-region.
func (k *KillRing) Kill(s string) { k.accumulate(s, false) }

// Append records text killed forward of point, such as C-k or M-d. During a
// kill run it extends the newest entry on the right.
func (k *KillRing) Append(s string) { k.accumulate(s, false) }

// Prepend records text killed backward of point, such as M-DEL. During a kill
// run it extends the newest entry on the left, so killing backward word by
// word yields text in reading order rather than reversed.
func (k *KillRing) Prepend(s string) { k.accumulate(s, true) }

// accumulate is the single mutation path: it either extends the newest entry
// or pushes a new one, and resets the yank state either way.
//
// An empty string is a complete no-op. Killing nothing should neither create a
// useless entry nor disturb an in-progress yank run, since from the user's
// point of view nothing happened.
func (k *KillRing) accumulate(s string, backward bool) {
	if s == "" {
		return
	}

	if k.inRun && len(k.entries) > 0 {
		if backward {
			k.entries[0] = s + k.entries[0]
		} else {
			k.entries[0] += s
		}
	} else {
		k.push(s)
		k.inRun = true
	}

	// The newest entry changed, so the yank pointer belongs back at the front
	// and a pending yank-pop run is over.
	k.yankIdx = 0
	k.yankOK = false
}

// push inserts a new newest entry, evicting the oldest at capacity.
func (k *KillRing) push(s string) {
	k.entries = append([]string{s}, k.entries...)
	if len(k.entries) > k.capacity {
		k.entries = k.entries[:k.capacity]
	}
}

// BreakRun ends any kill run and invalidates YankPop. Every command that is
// neither a kill nor a yank calls this, which is what makes "kill, move, kill"
// produce two entries rather than one.
func (k *KillRing) BreakRun() {
	k.inRun = false
	k.yankOK = false
}

// Yank returns the entry the yank pointer rests on, without consuming it. It
// ends any kill run, so a kill after a yank starts a fresh entry, and it makes
// YankPop valid.
func (k *KillRing) Yank() (string, error) {
	if len(k.entries) == 0 {
		return "", ErrKillRingEmpty
	}
	k.inRun = false
	k.yankOK = true
	return k.entries[k.yankIdx], nil
}

// YankPop advances the yank pointer to the next-older entry, wrapping around
// to the newest, and returns that entry. The result is the full replacement
// text: the caller removes what it last yanked and inserts this instead.
//
// It is valid only immediately after Yank or another YankPop.
func (k *KillRing) YankPop() (string, error) {
	if !k.yankOK {
		return "", ErrNotAfterYank
	}
	if len(k.entries) == 0 {
		return "", ErrKillRingEmpty
	}
	k.yankIdx = (k.yankIdx + 1) % len(k.entries)
	return k.entries[k.yankIdx], nil
}
