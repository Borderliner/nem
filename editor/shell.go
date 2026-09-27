package editor

import (
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/Borderliner/nem/command"
	"github.com/Borderliner/nem/compile"
	"github.com/Borderliner/nem/text"
	"github.com/gdamore/tcell/v2"
)

// Shell commands, as emacs has them: M-! runs one and shows what it printed,
// M-| runs one on the region, and with C-u either puts the output in the
// buffer - at point, or in place of the region. That last is how a buffer is
// put through any filter there is: C-u M-| sort, C-u M-| jq ., C-u M-| column
// -t. M-& runs one in the background; see compile.go.

// shellOutputName is where output too long for the echo area goes.
const shellOutputName = "*Shell Command Output*"

var errNoRegion = errors.New("the mark is not set now, so there is no region")

// registerShellCommands adds M-! and M-|.
func registerShellCommands(e *Editor, reg *command.Registry) error {
	cmds := []command.Command{
		{Name: "shell-command", Doc: "Run a shell command and show its output; with C-u, insert it at point. A command ending in & runs in the background.",
			Fn: func(command.Env) error {
				_, insert := e.Arg()
				line, err := e.ReadString(command.ReadOpts{Prompt: "Shell command: ", History: "shell"})
				if err != nil || strings.TrimSpace(line) == "" {
					return err
				}
				dir := e.shellDir()
				if bg, ok := strings.CutSuffix(strings.TrimSpace(line), "&"); ok {
					return e.runCompile(bg, dir, true)
				}
				return e.shellRun(line, dir, nil, insert, false)
			}},
		{Name: "shell-command-on-region", Doc: "Run a shell command with the region as its input and show its output; with C-u, replace the region with it.",
			Fn: func(command.Env) error {
				_, replace := e.Arg()
				b := e.active.Buf
				if !b.HasMark() {
					return errNoRegion
				}
				line, err := e.ReadString(command.ReadOpts{Prompt: "Shell command on region: ", History: "shell"})
				if err != nil || strings.TrimSpace(line) == "" {
					return err
				}
				lo, hi := text.OrderPos(e.active.Pt, b.Mark())
				return e.shellRun(line, e.shellDir(), []byte(string(b.Text(lo, hi))), false, replace)
			}},
	}
	for _, c := range cmds {
		c.Interactive = true
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// shellDir is where a shell command runs: the directory of the buffer it was
// given in.
func (e *Editor) shellDir() string {
	if dir := e.bufferDir(e.active.Buf); dir != "" {
		return dir
	}
	dir, _ := os.Getwd()
	return dir
}

// shellRun runs line in dir with stdin as its input, waiting for it, and
// deals with what it printed: into the buffer at point if insert, in place of
// the region if replace, and otherwise shown - in the echo area when it is a
// line, in *Shell Command Output* when it is more.
//
// A command that fails puts nothing in the buffer, whatever was asked: the
// output of a failed filter is an error message, and replacing the region
// with that would lose the region to it.
func (e *Editor) shellRun(line, dir string, stdin []byte, insert, replace bool) error {
	b := e.active.Buf
	p, err := e.startProcess(line, dir, stdin)
	if err != nil {
		return err
	}
	out, err := e.waitProcess(p, line)
	reverted := e.afterCommand()
	if err != nil {
		return err
	}
	code, _ := p.result()
	printed := cleanOutput(out)

	if code == 0 && (insert || replace) {
		return e.shellInsert(b, printed, replace)
	}
	shown := strings.TrimSuffix(printed, "\n")
	switch {
	case shown == "" && code == 0 && reverted != "":
		// A command run for what it does to the files: what it did is the
		// news.
		e.Echo("%s", reverted)
	case shown == "" && code == 0:
		e.Echo("(Shell command succeeded with no output)")
	case shown == "":
		e.Echo("(Shell command failed with code %d and no output)", code)
	case !strings.Contains(shown, "\n") && utf8.RuneCountInString(shown) < e.echoWidth():
		if code != 0 {
			e.Echo("%s  (code %d)", shown, code)
		} else {
			e.Echo("%s", shown)
		}
	default:
		ob := e.NewBuffer(shellOutputName)
		ob.Regenerate([]rune(shown))
		e.showBeside(ob)
		for _, w := range e.tree.Windows() {
			if w.Buf == ob {
				w.Pt = text.Pos{}
				w.SetTop(0)
			}
		}
		if code != 0 {
			e.Echo("Shell command failed with code %d", code)
		} else {
			e.Echo("")
		}
	}
	if code != 0 && (insert || replace) {
		e.Echo("Shell command failed with code %d; the buffer is unchanged", code)
	}
	return nil
}

// shellInsert puts a command's output in b: at point, or in place of the
// region. Point ends before it and the mark after, as emacs leaves them, so
// C-x C-x goes to the other end.
//
// A region that did not end in a newline is not given one: a filter like
// sort ends every line it writes with one, and taking it at its word would
// join the region's last line to the line after it.
func (e *Editor) shellInsert(b *text.Buffer, out string, replace bool) error {
	w := e.active
	at := w.Pt
	b.BeginUndoGroup()
	defer b.EndUndoGroup()
	if replace {
		lo, hi := text.OrderPos(w.Pt, b.Mark())
		if region := b.Text(lo, hi); len(region) > 0 && region[len(region)-1] != '\n' {
			out = strings.TrimSuffix(out, "\n")
		}
		if err := b.Delete(lo, hi); err != nil {
			return err
		}
		at = lo
	}
	rs := []rune(out)
	if err := b.Insert(at, rs); err != nil {
		return err
	}
	w.Pt = at
	b.SetMark(advancePos(at, rs))
	b.DeactivateMark()
	return nil
}

// advancePos is where p ends up after rs is inserted at it.
func advancePos(p text.Pos, rs []rune) text.Pos {
	for _, r := range rs {
		if r == '\n' {
			p.Line++
			p.Col = 0
		} else {
			p.Col++
		}
	}
	return p
}

// cleanOutput turns what a command printed into plain text, a line at a
// time: colour and cursor escapes go, and a line redrawn with carriage
// returns keeps only its last drawing.
func cleanOutput(out []byte) string {
	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		lines[i] = compile.Clean(l)
	}
	return strings.Join(lines, "\n")
}

// echoWidth is how many columns a message has.
func (e *Editor) echoWidth() int {
	if e.scr == nil {
		return 80
	}
	w, _ := e.scr.Size()
	return w
}

// waitProcess waits for p to finish, gathering what it prints. The editor
// keeps drawing while it waits, and C-g stops the command.
//
// It reads the terminal directly rather than through nextEvent, which would
// take the next key of a keyboard macro being played and feed it to the
// wait rather than to what comes after the command in the macro.
func (e *Editor) waitProcess(p *process, line string) ([]byte, error) {
	var out []byte
	stopped := false
	for {
		if !stopped {
			e.Echo("Running %s  (C-g to stop)", describeCommand(line))
		}
		e.Redraw()
		ev := e.liveEvent()
		if ev == nil {
			p.kill()
			return out, command.ErrQuit
		}
		switch ev := ev.(type) {
		case *tcell.EventKey:
			if k := e.answerKey(DecodeKey(ev, e.keys.TreatCtrlHAsBackspace), nil); quitKey(k) && !stopped {
				stopped = true
				e.Echo("Stopping %s…", describeCommand(line))
				p.kill()
			}
		case *tcell.EventInterrupt:
			if w, ok := ev.Data().(processWake); ok && w.p == p {
				data, done := p.take()
				out = append(out, data...)
				if done {
					e.Echo("")
					if stopped {
						return out, command.ErrQuit
					}
					return out, nil
				}
				continue
			}
			e.handleEvent(ev)
		default:
			e.handleEvent(ev)
		}
	}
}

// liveEvent is the next event from the terminal itself, past any keyboard
// macro being played.
func (e *Editor) liveEvent() tcell.Event {
	if e.events == nil {
		return e.scr.PollEvent()
	}
	ev, ok := <-e.events
	if !ok {
		return nil
	}
	return ev
}

// describeCommand shortens a command line for a message.
func describeCommand(line string) string {
	if utf8.RuneCountInString(line) > 40 {
		return string([]rune(line)[:39]) + "…"
	}
	return line
}
