package flasher

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Frontend is what the flow talks to. The console (UI below) is one; the
// Linux TUI and the Windows GUI are others (internal/app turns every call
// into a message for the screen). Questions block until answered.
type Frontend interface {
	Say(format string, a ...any)
	Step(format string, a ...any)
	Warn(lines ...string)
	Log(format string, a ...any)
	Progress(label string, done, total int64)
	Ask(prompt string) (string, error)
	Confirm(prompt, word string) (bool, error)
	YesNo(prompt string, def bool) (bool, error)
	Enter(prompt string) error
	Raw(s string)  // tool output as it streams (fastboot), and the preloader's dots
	Event(e Event) // structured progress for the TUI/GUI; the console only logs it
}

var _ Frontend = (*UI)(nil)

// UI is the console the person sees, plus the log file.
type UI struct {
	in  *bufio.Reader
	out io.Writer
	log io.Writer
	mu  sync.Mutex
}

func NewUI(in io.Reader, out io.Writer, log io.Writer) *UI {
	if log == nil {
		log = io.Discard
	}
	return &UI{in: bufio.NewReader(in), out: out, log: log}
}

func (u *UI) logLine(prefix, s string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		fmt.Fprintf(u.log, "%s %s %s\n", ts, prefix, l)
	}
}

// Say prints a message to the person and the log.
func (u *UI) Say(format string, a ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	s := fmt.Sprintf(format, a...)
	fmt.Fprintln(u.out, s)
	u.logLine("|", s)
}

// Step prints a numbered heading.
func (u *UI) Step(format string, a ...any) {
	u.Say("")
	u.Say("==> "+format, a...)
}

// Warn prints a boxed warning.
func (u *UI) Warn(lines ...string) {
	u.Say("")
	u.Say("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	for _, l := range lines {
		u.Say("!!  %s", l)
	}
	u.Say("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
}

// Log writes to the log only.
func (u *UI) Log(format string, a ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.logLine("#", fmt.Sprintf(format, a...))
}

// Progress prints an in-place progress line (console only).
func (u *UI) Progress(label string, done, total int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}
	fmt.Fprintf(u.out, "\r    %s %3d%% (%d / %d MB)   ", label, pct, done>>20, total>>20)
	if done >= total {
		fmt.Fprintln(u.out)
	}
}

// ErrInputClosed is returned when stdin ends (for example a closed console).
var ErrInputClosed = fmt.Errorf("no more input (console closed)")

// ErrCanceled is returned by the TUI/window when the person pressed Cancel
// (or Esc, or closed the question box). It counts as ErrInputClosed
// (errors.Is), so every path treats it exactly like a closed console; only
// the words shown to the person differ.
var ErrCanceled error = canceledErr{}

type canceledErr struct{}

func (canceledErr) Error() string {
	return "you pressed Cancel (or closed the question box)"
}
func (canceledErr) Is(target error) bool { return target == ErrInputClosed }

// Ask prints a prompt and returns the trimmed answer.
func (u *UI) Ask(prompt string) (string, error) {
	u.mu.Lock()
	fmt.Fprint(u.out, prompt+" ")
	u.mu.Unlock()
	line, err := u.in.ReadString('\n')
	ans := strings.TrimSpace(line)
	u.Log("asked %q -> answered %q", prompt, ans)
	if err != nil && ans == "" {
		return "", ErrInputClosed
	}
	return ans, nil
}

// Confirm asks for an exact word (case-insensitive). Anything else = no.
func (u *UI) Confirm(prompt, word string) (bool, error) {
	ans, err := u.Ask(prompt)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(ans, word), nil
}

// YesNo asks a yes/no question with a default.
func (u *UI) YesNo(prompt string, def bool) (bool, error) {
	suffix := " [y/N]"
	if def {
		suffix = " [Y/n]"
	}
	ans, err := u.Ask(prompt + suffix)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(ans) {
	case "":
		return def, nil
	case "y", "yes", "כן":
		return true, nil
	default:
		return false, nil
	}
}

// Enter waits for the Enter key.
func (u *UI) Enter(prompt string) error {
	_, err := u.Ask(prompt)
	return err
}

// Raw prints streamed tool output as it arrives (no newline added).
func (u *UI) Raw(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fmt.Fprint(u.out, s)
	if strings.TrimSpace(s) != "" && s != "." {
		u.logLine(">", s)
	}
}

// Event is logged only; the console shows the same thing as text already.
func (u *UI) Event(e Event) {
	u.Log("event %s", e)
}

var _ = os.Stdin
