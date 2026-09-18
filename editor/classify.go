package editor

// Dispatch needs to know two things about a command that its registry entry does
// not record: whether it killed text, and whether it moved vertically. Both
// drive bookkeeping the command itself must not have to remember.
//
// These are name sets rather than flags on command.Command because the command
// package must not grow editor concerns. A name here that no group registers is
// a silent no-op, so TestClassifiedCommandsExist cross-checks every entry
// against the registry — the same guard the default bindings get.

// killCommands are the commands that add to the kill ring.
//
// Every other command breaks the kill run, which is what makes "kill, move,
// kill" produce two ring entries instead of one fused entry. Getting this set
// wrong is invisible until a user kills, moves, kills and yanks back more than
// they expected, so it is pinned by test.
var killCommands = map[string]bool{
	"kill-line":          true,
	"kill-word":          true,
	"backward-kill-word": true,
	"kill-region":        true,
	"kill-ring-save":     true,
}

// yankCommands are the commands after which yank-pop stays valid.
//
// The kill ring tracks this itself — Yank sets its own yank-valid flag and
// BreakRun clears it — so these must be exempt from the break for M-y to work
// at all. yank-pop is here as well as yank because M-y M-y must keep rotating.
var yankCommands = map[string]bool{
	"yank":     true,
	"yank-pop": true,
}

// goalColumnCommands are the vertical motions that establish and preserve the
// goal column.
//
// Only these two. C-n down through a short line and back up must return to the
// original column, so the column survives across a run of them and is cleared
// by anything else. motion.go clears it for its own non-vertical commands
// already; dispatch extends the same discipline to every command in the editor,
// and the two agree rather than fighting because both clear and only these two
// preserve.
var goalColumnCommands = map[string]bool{
	"next-line":     true,
	"previous-line": true,
}

// breaksKillRun reports whether running name should end the kill run.
func breaksKillRun(name string) bool {
	return !killCommands[name] && !yankCommands[name]
}

// keepsGoalColumn reports whether running name should leave the goal column
// alone.
func keepsGoalColumn(name string) bool { return goalColumnCommands[name] }
