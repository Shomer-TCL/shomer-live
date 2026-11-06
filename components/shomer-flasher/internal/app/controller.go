package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// MsgKind says what a message carries.
type MsgKind int

const (
	MsgLine MsgKind = iota + 1
	MsgProgress
	MsgQuestion
	MsgEvent
	MsgChecks   // the start-up checks
	MsgCheck    // one check changed
	MsgPrepared // ROM checked and unpacked (Err if not)
	MsgDetected // phone found (Phone) or not (Err)
	MsgFinished // writing ended (Err nil = OK)
)

// Msg is one message from the flasher to the screen.
type Msg struct {
	Kind     MsgKind
	Line     Line
	Progress Progress
	Question *Question
	Event    flasher.Event
	Checks   []Check
	Check    Check
	Phone    flasher.PhoneInfo
	Err      error
}

// QKind is the kind of question.
type QKind int

const (
	QEnter QKind = iota // "press Enter when ..." -> an OK button
	QYesNo
	QText // type an answer (the software version)
	QWord // type an exact word (not used when the UI confirmed already)
)

// Question is the flasher waiting for the person.
type Question struct {
	Kind    QKind
	Prompt  string
	Default bool     // QYesNo
	Word    string   // QWord
	Context []string // what was said since the last heading (instructions)
	ch      chan Answer
	once    sync.Once
}

// Answer is the person's reply. Cancel = closed the dialog / quit.
type Answer struct {
	Yes    bool
	Text   string
	Cancel bool
}

func (q *Question) reply(a Answer) {
	q.once.Do(func() { q.ch <- a })
}

// CleanPrompt is the prompt without console decoration ("[y/N]", leading spaces).
func (q *Question) CleanPrompt() string {
	p := strings.TrimSpace(q.Prompt)
	p = strings.TrimSuffix(p, "...")
	return p
}

// chanUI is a flasher.Frontend that turns every call into a Msg. The log
// file still gets everything (through the console UI with no screen).
type chanUI struct {
	log  *flasher.UI
	out  chan<- Msg
	mu   sync.Mutex
	last time.Time
	lpct int
	llab string
	raw  strings.Builder
	quit <-chan struct{}
}

func (u *chanUI) send(m Msg) {
	select {
	case u.out <- m:
	case <-u.quit:
	}
}

func (u *chanUI) Say(format string, a ...any) {
	u.log.Log("| "+format, a...)
	u.send(Msg{Kind: MsgLine, Line: Line{Kind: LineSay, Text: fmt.Sprintf(format, a...)}})
}
func (u *chanUI) Step(format string, a ...any) {
	u.log.Log("| ==> "+format, a...)
	u.send(Msg{Kind: MsgLine, Line: Line{Kind: LineStep, Text: fmt.Sprintf(format, a...)}})
}
func (u *chanUI) Warn(lines ...string) {
	for _, l := range lines {
		u.log.Log("| !!  %s", l)
		u.send(Msg{Kind: MsgLine, Line: Line{Kind: LineWarn, Text: l}})
	}
}
func (u *chanUI) Log(format string, a ...any) { u.log.Log(format, a...) }

// Progress is throttled: at most every 100 ms unless the percent or label changed.
func (u *chanUI) Progress(label string, done, total int64) {
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}
	u.mu.Lock()
	now := time.Now()
	skip := label == u.llab && pct == u.lpct && now.Sub(u.last) < 100*time.Millisecond && done < total
	if !skip {
		u.last, u.lpct, u.llab = now, pct, label
	}
	u.mu.Unlock()
	if !skip {
		u.send(Msg{Kind: MsgProgress, Progress: Progress{Label: label, Done: done, Total: total}})
	}
}

// Raw collects streamed tool output into lines.
func (u *chanUI) Raw(s string) {
	u.log.Raw(s)
	if s == "." {
		return
	}
	u.mu.Lock()
	u.raw.WriteString(s)
	all := u.raw.String()
	var lines []string
	if i := strings.LastIndexAny(all, "\r\n"); i >= 0 {
		lines = strings.FieldsFunc(all[:i], func(r rune) bool { return r == '\n' || r == '\r' })
		u.raw.Reset()
		u.raw.WriteString(all[i+1:])
	}
	u.mu.Unlock()
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			u.send(Msg{Kind: MsgLine, Line: Line{Kind: LineRaw, Text: l}})
		}
	}
}

func (u *chanUI) Event(e flasher.Event) {
	u.log.Event(e)
	u.send(Msg{Kind: MsgEvent, Event: e})
}

func (u *chanUI) ask(q *Question) (Answer, error) {
	q.ch = make(chan Answer, 1)
	u.log.Log("asked %q", q.Prompt)
	u.send(Msg{Kind: MsgQuestion, Question: q})
	select {
	case a := <-q.ch:
		u.log.Log("answered %+v", a)
		if a.Cancel {
			return a, flasher.ErrCanceled
		}
		return a, nil
	case <-u.quit:
		return Answer{Cancel: true}, flasher.ErrInputClosed
	}
}

func (u *chanUI) Ask(prompt string) (string, error) {
	a, err := u.ask(&Question{Kind: QText, Prompt: prompt})
	return strings.TrimSpace(a.Text), err
}
func (u *chanUI) Confirm(prompt, word string) (bool, error) {
	a, err := u.ask(&Question{Kind: QWord, Prompt: prompt, Word: word})
	return err == nil && strings.EqualFold(strings.TrimSpace(a.Text), word), err
}
func (u *chanUI) YesNo(prompt string, def bool) (bool, error) {
	a, err := u.ask(&Question{Kind: QYesNo, Prompt: prompt, Default: def})
	return a.Yes, err
}
func (u *chanUI) Enter(prompt string) error {
	_, err := u.ask(&Question{Kind: QEnter, Prompt: prompt})
	return err
}

// Controller runs the flasher in the background for a front end.
type Controller struct {
	Flow *flasher.Flow
	Env  Env
	msgs chan Msg
	quit chan struct{}
	once sync.Once
}

// NewController wires the flow to a message channel. logw is the log file.
func NewController(f *flasher.Flow, env Env, logw io.Writer) *Controller {
	c := &Controller{Flow: f, Env: env, msgs: make(chan Msg, 256), quit: make(chan struct{})}
	f.UI = &chanUI{log: flasher.NewUI(strings.NewReader(""), io.Discard, logw), out: c.msgs, quit: c.quit}
	return c
}

// Msgs is where the screen reads from.
func (c *Controller) Msgs() <-chan Msg { return c.msgs }

// Stop ends every pending question (they answer "closed").
func (c *Controller) Stop() { c.once.Do(func() { close(c.quit) }) }

func (c *Controller) send(m Msg) {
	select {
	case c.msgs <- m:
	case <-c.quit:
	}
}

// Start runs the start-up checks, then checks and unpacks the ROM.
func (c *Controller) Start() {
	o := c.Flow.Opts
	checks := RunChecks(c.Env, o.WorkDir, c.Flow.WorkNeed(), o.Backup, o.BackupDir, flasher.BackupNeed(o.BackupUserdata))
	checks = append(checks, Check{ID: "rom", Name: "ROM inside this program", Status: CheckRunning, Detail: "checking every file's SHA-256, then unpacking (1-3 minutes)"})
	c.send(Msg{Kind: MsgChecks, Checks: checks})
	go func() {
		err := c.Flow.Prepare()
		if err == nil {
			c.send(Msg{Kind: MsgCheck, Check: Check{ID: "rom", Name: "ROM inside this program", Status: CheckOK,
				Detail: "every file matches its SHA-256; unpacked to " + o.WorkDir}})
		}
		c.send(Msg{Kind: MsgPrepared, Err: err})
	}()
}

// Detect finds and identifies the phone.
func (c *Controller) Detect() {
	go func() {
		p, err := c.Flow.Detect()
		c.send(Msg{Kind: MsgDetected, Phone: p, Err: err})
	}()
}

// Flash writes the phone with the person's plan (already confirmed on screen).
func (c *Controller) Flash(p flasher.Plan) {
	o := &c.Flow.Opts
	o.Backup, o.BackupDir, o.BackupUserdata, o.EraseData = p.Backup, p.BackupDir, p.Userdata, p.EraseData
	o.Preconfirmed = true
	go func() {
		err := c.Flow.WritePhone()
		c.send(Msg{Kind: MsgFinished, Err: err})
	}()
}
