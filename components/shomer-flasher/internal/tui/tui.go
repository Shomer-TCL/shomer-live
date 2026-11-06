//go:build !windows

// Package tui is the full-screen text interface (Linux). It only draws the
// shared app.Model and turns keys into its actions.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Shomer-TCL/shomer-flasher/internal/app"
)

// Shomer colours.
var (
	navy  = lipgloss.Color("#0B1F3A")
	teal  = lipgloss.Color("#14B8A6")
	white = lipgloss.Color("#FFFFFF")
	grey  = lipgloss.Color("#8A94A6")
	red   = lipgloss.Color("#E5484D")
	amber = lipgloss.Color("#F5A524")
	green = lipgloss.Color("#30A46C")

	sHeader  = lipgloss.NewStyle().Background(navy).Foreground(white).Bold(true)
	sBrand   = lipgloss.NewStyle().Background(navy).Foreground(teal).Bold(true)
	sTabOn   = lipgloss.NewStyle().Background(teal).Foreground(navy).Bold(true)
	sTabOff  = lipgloss.NewStyle().Background(navy).Foreground(grey)
	sTitle   = lipgloss.NewStyle().Foreground(teal).Bold(true)
	sDim     = lipgloss.NewStyle().Foreground(grey)
	sOK      = lipgloss.NewStyle().Foreground(green).Bold(true)
	sWarn    = lipgloss.NewStyle().Foreground(amber).Bold(true)
	sFail    = lipgloss.NewStyle().Foreground(red).Bold(true)
	sSel     = lipgloss.NewStyle().Background(teal).Foreground(navy).Bold(true)
	sButton  = lipgloss.NewStyle().Foreground(white).Background(navy).Padding(0, 1)
	sButtonS = lipgloss.NewStyle().Foreground(navy).Background(teal).Bold(true).Padding(0, 1)
	sBox     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(teal).Padding(0, 1)
	sWarnBox = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(amber).Padding(0, 1)
)

// overlay is a question the TUI itself asks (final yes/no, quit?).
type overlay int

const (
	ovNone overlay = iota
	ovFinal
	ovQuit
)

// Model is the bubbletea model.
type Model struct {
	M   *app.Model
	C   *app.Controller // nil in tests: actions are recorded instead
	w   int
	h   int
	cur int // cursor on the options / confirm screens

	editing bool   // editing the backup folder
	input   string // text being typed (folder or an answer)
	yes     bool   // the highlighted button of a yes/no question

	ov        overlay
	scroll    int // first line of the main part shown
	maxScroll int
	Actions   []app.Action // what was asked of the controller (tests)
	Quit      bool
}

// New makes the TUI model.
func New(m *app.Model, c *app.Controller) *Model {
	return &Model{M: m, C: c, w: 80, h: 24}
}

type appMsg app.Msg

func (t *Model) wait() tea.Cmd {
	if t.C == nil {
		return nil
	}
	ch := t.C.Msgs()
	return func() tea.Msg { return appMsg(<-ch) }
}

// Init starts the checks and listens to the flasher.
func (t *Model) Init() tea.Cmd {
	if t.C != nil {
		t.C.Start()
	}
	return t.wait()
}

// Update handles a key or a flasher message.
func (t *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.w, t.h = msg.Width, msg.Height
		return t, nil
	case appMsg:
		was := t.M.Question
		t.M.Apply(app.Msg(msg))
		if q := t.M.Question; q != nil && q != was {
			t.yes = q.Default
			t.input = ""
		}
		return t, t.wait()
	case tea.KeyMsg:
		t.key(msg.String(), msg.Runes)
		if t.Quit {
			if t.C != nil {
				t.C.Stop()
			}
			return t, tea.Quit
		}
	}
	return t, nil
}

func (t *Model) do(a app.Action) {
	if a == app.ActNone {
		return
	}
	t.Actions = append(t.Actions, a)
	switch a {
	case app.ActDetect:
		if t.C != nil {
			t.C.Detect()
		}
	case app.ActFlash:
		t.ov, t.yes = ovFinal, false
	case app.ActQuit:
		t.Quit = true
	}
}

// Key handles one key press ("enter", "up", "q", "y", ...).
func (t *Model) Key(k string) { t.key(k, []rune(k)) }

func (t *Model) key(k string, runes []rune) {
	m := t.M
	if k == "ctrl+c" {
		t.askQuit()
		return
	}
	// our own yes/no boxes
	if t.ov != ovNone {
		switch k {
		case "left", "right", "tab", "up", "down", "shift+tab":
			t.yes = !t.yes
		case "y", "Y":
			t.yes = true
			t.closeOverlay()
		case "n", "N", "esc", "q":
			t.yes = false
			t.closeOverlay()
		case "enter":
			t.closeOverlay()
		}
		return
	}
	// a question from the flasher
	if q := m.Question; q != nil {
		switch q.Kind {
		case app.QEnter:
			switch k {
			case "enter":
				m.Answer(app.Answer{})
			case "q", "esc":
				t.askQuit()
			}
		case app.QYesNo:
			switch k {
			case "left", "right", "tab", "up", "down", "shift+tab":
				t.yes = !t.yes
			case "y", "Y":
				m.Answer(app.Answer{Yes: true})
			case "n", "N":
				m.Answer(app.Answer{Yes: false})
			case "enter":
				m.Answer(app.Answer{Yes: t.yes})
			case "esc", "q":
				t.askQuit()
			}
		default: // typed answer
			switch k {
			case "enter":
				m.Answer(app.Answer{Text: t.input})
				t.input = ""
			case "backspace":
				if r := []rune(t.input); len(r) > 0 {
					t.input = string(r[:len(r)-1])
				}
			case "esc":
				t.askQuit()
			default:
				if len(runes) > 0 && k != "space" || k == " " {
					t.input += string(runes)
				} else if k == "space" {
					t.input += " "
				}
			}
		}
		return
	}
	// editing the backup folder
	if t.editing {
		switch k {
		case "enter":
			m.SetBackupDir(t.input)
			t.editing = false
		case "esc":
			t.editing = false
		case "backspace":
			if r := []rune(t.input); len(r) > 0 {
				t.input = string(r[:len(r)-1])
			}
		case "space":
			t.input += " "
		default:
			if len(runes) > 0 {
				t.input += string(runes)
			}
		}
		return
	}
	switch k {
	case "q":
		t.askQuit()
		return
	case "esc", "b", "backspace":
		m.Back()
		t.cur, t.scroll = 0, 0
		return
	case "pgdown", "ctrl+d", " ":
		if m.Screen != app.ScreenOptions {
			t.scroll += 10
			if t.scroll > t.maxScroll {
				t.scroll = t.maxScroll
			}
			return
		}
	case "pgup", "ctrl+u":
		t.scroll -= 10
		if t.scroll < 0 {
			t.scroll = 0
		}
		return
	}
	switch m.Screen {
	case app.ScreenOptions:
		rows := m.Options()
		n := len(rows) + 1 // + Continue
		switch k {
		case "up", "k":
			t.cur = (t.cur + n - 1) % n
		case "down", "j", "tab":
			t.cur = (t.cur + 1) % n
		case "enter", " ", "space":
			if t.cur == len(rows) {
				t.do(m.Continue())
				t.cur = 0
				return
			}
			r := rows[t.cur]
			if !r.Enabled {
				return
			}
			if r.Toggle {
				m.Toggle(r.ID)
			} else if r.ID == app.OptBackupDir && k == "enter" {
				t.editing, t.input = true, m.Plan.BackupDir
			}
		}
	case app.ScreenConfirm:
		switch k {
		case "up":
			if t.scroll > 0 {
				t.scroll--
			}
		case "down":
			if t.scroll < t.maxScroll {
				t.scroll++
			}
		case "left", "right", "tab":
			t.cur = 1 - t.cur
		case "enter":
			if t.cur == 1 {
				m.Back()
				t.cur = 0
				return
			}
			t.do(m.Continue())
		}
	default:
		switch k {
		case "enter":
			t.do(m.Continue())
			t.scroll = 0
		case "up":
			if t.scroll > 0 {
				t.scroll--
			}
		case "down":
			if t.scroll < t.maxScroll {
				t.scroll++
			}
		}
	}
}

func (t *Model) askQuit() {
	if t.M.Screen == app.ScreenDone || !t.M.QuitNeedsConfirm() {
		t.Quit = true
		return
	}
	t.ov, t.yes = ovQuit, false
}

func (t *Model) closeOverlay() {
	ov := t.ov
	t.ov = ovNone
	switch ov {
	case ovFinal:
		if t.yes {
			t.M.StartFlash()
			if t.C != nil {
				t.C.Flash(t.M.Plan)
			}
		}
	case ovQuit:
		if t.yes {
			t.Quit = true
		}
	}
	t.yes = false
}

// ---------- drawing ----------

// View draws the whole screen. The bottom part of a screen (buttons, the open
// question) is always shown; the main part scrolls (Up/Down, PgUp/PgDn) when
// the terminal is too small.
func (t *Model) View() string {
	w := t.w
	if w < 40 {
		w = 40
	}
	var b strings.Builder
	b.WriteString(t.header(w))
	b.WriteString("\n")
	main, bottom := t.body(w)
	footer := t.footer(w)
	room := t.h - 1 - lipgloss.Height(footer)
	var bl []string
	if bottom != "" {
		bl = strings.Split(strings.TrimRight(bottom, "\n"), "\n")
	}
	room -= len(bl)
	lines := strings.Split(strings.TrimRight(main, "\n"), "\n")
	if room < 3 {
		room = 3
	}
	t.maxScroll = 0
	if len(lines) > room {
		t.maxScroll = len(lines) - room + 1
		if t.scroll > t.maxScroll {
			t.scroll = t.maxScroll
		}
		end := t.scroll + room - 1
		if end > len(lines) {
			end = len(lines)
		}
		more := sDim.Render(fmt.Sprintf(" ... %d more lines: PgDn/PgUp to scroll", len(lines)-end))
		if end == len(lines) {
			more = sDim.Render(" (end - PgUp to scroll back)")
		}
		lines = append(append([]string{}, lines[t.scroll:end]...), more)
	} else {
		t.scroll = 0
	}
	for len(lines) < room {
		lines = append(lines, "")
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
	if len(bl) > 0 {
		b.WriteString(strings.Join(bl, "\n"))
		b.WriteString("\n")
	}
	b.WriteString(footer)
	return b.String()
}

func (t *Model) header(w int) string {
	m := t.M
	left := sBrand.Render(" SHOMER ") + sHeader.Render("flasher "+m.Version+" ")
	var tabs []string
	for s := app.ScreenChecks; s <= app.ScreenDone; s++ {
		name := strings.SplitN(s.Title(), " ", 2)[1]
		if s == m.Screen {
			tabs = append(tabs, sTabOn.Render(" "+name+" "))
		} else {
			tabs = append(tabs, sTabOff.Render(" "+name+" "))
		}
	}
	right := strings.Join(tabs, sTabOff.Render(">"))
	pad := w - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		right = sTabOn.Render(" " + m.Screen.Title() + " ")
		pad = w - lipgloss.Width(left) - lipgloss.Width(right)
		if pad < 0 {
			pad = 0
		}
	}
	return left + sHeader.Render(strings.Repeat(" ", pad)) + right
}

func (t *Model) footer(w int) string {
	m := t.M
	var keys string
	switch {
	case t.ov != ovNone:
		keys = "Left/Right choose  Enter OK  y yes  n no"
	case m.Question != nil && m.Question.Kind == app.QYesNo:
		keys = "Left/Right choose  Enter OK  y yes  n no  q quit"
	case m.Question != nil && m.Question.Kind == app.QEnter:
		keys = "Enter when done  q quit"
	case m.Question != nil:
		keys = "type, then Enter  Esc quit"
	case t.editing:
		keys = "type the folder, Enter save, Esc cancel"
	case m.Screen == app.ScreenOptions:
		keys = "Up/Down move  Enter change/continue  Esc back  q quit"
	case m.Screen == app.ScreenConfirm:
		keys = "Left/Right choose  Enter OK  Up/Down/PgDn scroll  Esc back  q quit"
	case m.Screen == app.ScreenDone:
		keys = "Enter or q close"
	case m.Busy():
		keys = "working...  q quit"
	default:
		keys = "Enter continue  q quit"
	}
	return sDim.Render(strings.Repeat("─", w)) + "\n" + sDim.Render(" "+keys)
}

func wrap(s string, w int) string {
	if w < 20 {
		w = 20
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

func (t *Model) body(w int) (string, string) {
	m := t.M
	var b, bot strings.Builder
	title := map[app.Screen]string{
		app.ScreenChecks:   "Checks before we start",
		app.ScreenPhone:    "The phone",
		app.ScreenOptions:  "Options",
		app.ScreenConfirm:  "Confirm: this is exactly what will be written",
		app.ScreenProgress: "Writing - do NOT unplug the cable",
		app.ScreenDone:     "Finished",
	}[m.Screen]
	if m.Screen == app.ScreenDone && !m.Result.OK {
		title = "Stopped"
	}
	b.WriteString(" " + sTitle.Render(title) + "\n\n")
	switch m.Screen {
	case app.ScreenChecks:
		for _, c := range m.Checks {
			b.WriteString(wrap(" "+mark(c.Status)+" "+lipgloss.NewStyle().Bold(true).Render(c.Name)+"  "+c.Detail, w-1) + "\n")
			if c.Fix != "" {
				b.WriteString(wrap("        "+c.Fix, w-1) + "\n")
			}
			if c.ID == "rom" && c.Status == app.CheckRunning && m.Progress.Total > 0 {
				b.WriteString("        " + bar(m.Progress, w-40) + "\n")
			}
		}
		if m.Prepared && m.PrepErr == nil && !m.ChecksBlock() {
			b.WriteString("\n" + wrap(" Next: the program helps you get the phone into fastboot mode. Nothing is written until you confirm.", w-1) + "\n")
			bot.WriteString("\n " + button(m.ContinueLabel(), true) + "\n")
		}
	case app.ScreenPhone:
		if m.Phone != nil {
			p := m.Phone
			b.WriteString(sBox.Render(strings.Join([]string{
				sOK.Render("Phone found"),
				"Model:      " + dash(p.Model),
				"Product:    " + p.Product,
				"Serial:     " + dash(p.Serial),
				"Software:   " + dash(p.Build),
				"Bootloader: " + p.Unlock.String(),
			}, "\n")) + "\n")
			bot.WriteString("\n " + button(m.ContinueLabel(), true) + "\n")
		} else if q := t.question(w); q != "" {
			bot.WriteString(q + "\n")
		} else {
			b.WriteString(" " + sDim.Render("Follow the steps. The program tells you when to plug in.") + "\n\n")
			b.WriteString(t.logTail(w, t.h-10))
		}
	case app.ScreenOptions:
		b.WriteString(wrap(" Phone: "+m.PhoneSummary(), w-1) + "\n\n")
		rows := m.Options()
		for i, r := range rows {
			val := r.Value
			if r.Toggle {
				if val == "ON" {
					val = "[x] ON "
				} else {
					val = "[ ] OFF"
				}
			}
			if r.ID == app.OptBackupDir && t.editing {
				val = t.input + "_"
			}
			line := fmt.Sprintf(" %-32s %s", r.Label, val)
			if len([]rune(line)) > w-1 {
				line = string([]rune(line)[:w-2]) + "…"
			}
			switch {
			case i == t.cur:
				line = sSel.Render(line)
			case !r.Enabled:
				line = sDim.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n " + button(m.ContinueLabel(), t.cur == len(rows)) + "\n\n")
		if t.cur < len(rows) {
			b.WriteString(wrap(" "+sDim.Render(rows[t.cur].Help), w-1) + "\n")
		}
	case app.ScreenConfirm:
		for _, l := range m.ConfirmLines() {
			st := lipgloss.NewStyle()
			switch {
			case strings.HasPrefix(l, "UNLOCKING"):
				st = sWarn
			case strings.HasPrefix(l, "      "):
				st = sDim
			}
			b.WriteString(wrap(" "+st.Render(l), w-1) + "\n")
		}
		bot.WriteString("\n " + button(m.ContinueLabel(), t.cur == 0) + "  " + button("Back", t.cur == 1) + "\n")
		if t.ov == ovFinal {
			bot.WriteString(t.yesNoBox(m.FinalQuestion(), w) + "\n")
		}
	case app.ScreenProgress:
		for _, s := range m.Steps {
			line := " " + stepMark(s.State) + " " + s.Title
			if s.Detail != "" && (s.State == app.StepFailed || s.State == app.StepSkipped) {
				line += "  (" + s.Detail + ")"
			}
			if ansi.StringWidth(line) > w-1 {
				line = ansi.Truncate(line, w-2, "…")
			}
			if s.State == app.StepRunning {
				line = sTitle.Render(line)
			}
			b.WriteString(line + "\n")
		}
		if m.Progress.Total > 0 {
			bot.WriteString(" " + bar(m.Progress, w-45) + "\n")
		}
		if q := t.question(w); q != "" {
			bot.WriteString(q + "\n")
		} else if t.ov == ovQuit {
			bot.WriteString(t.yesNoBox(app.QuitWarning, w) + "\n")
		} else {
			n := t.h - len(m.Steps) - 9
			if n > 12 {
				n = 12
			}
			bot.WriteString(sDim.Render(strings.Repeat("·", w)) + "\n" + t.logTail(w, n))
		}
	case app.ScreenDone:
		for i, l := range m.DoneLines() {
			st := lipgloss.NewStyle()
			if i == 0 {
				if m.Result.OK {
					st = sOK
				} else {
					st = sFail
				}
			}
			b.WriteString(wrap(" "+st.Render(l), w-1) + "\n")
		}
		bot.WriteString("\n " + button("Close", true) + "\n")
	}
	if t.ov == ovQuit && m.Screen != app.ScreenProgress {
		bot.WriteString(t.yesNoBox(app.QuitWarning, w) + "\n")
	}
	return b.String(), bot.String()
}

func dash(s string) string {
	if s == "" || s == "skipped" {
		return "-"
	}
	return s
}

func (t *Model) question(w int) string {
	q := t.M.Question
	if q == nil {
		return ""
	}
	var l []string
	for _, c := range q.Context {
		l = append(l, strings.TrimSpace(c))
	}
	if len(l) > 0 {
		l = append(l, "")
	}
	p := strings.TrimSpace(q.Prompt)
	switch q.Kind {
	case app.QEnter:
		l = append(l, sTitle.Render(strings.TrimSuffix(p, "...")), "", button("Done - press Enter", true))
	case app.QYesNo:
		l = append(l, sTitle.Render(p), "", button("Yes", t.yes)+"  "+button("No", !t.yes))
	default:
		l = append(l, sTitle.Render(p), "", "> "+t.input+"_")
	}
	st := sBox
	if lp := strings.ToLower(p); strings.Contains(lp, "backup failed") || strings.Contains(lp, "flash anyway") {
		st = sWarnBox
	}
	return st.Width(w - 4).Render(strings.Join(l, "\n"))
}

func (t *Model) yesNoBox(text string, w int) string {
	return sWarnBox.Width(w - 4).Render(sWarn.Render(text) + "\n\n" + button("Yes", t.yes) + "  " + button("No", !t.yes))
}

func (t *Model) logTail(w, n int) string {
	if n < 3 {
		n = 3
	}
	ls := t.M.Lines
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	var b strings.Builder
	for _, l := range ls {
		s := strings.TrimRight(l.Text, " ")
		if ansi.StringWidth(s) > w-2 {
			s = ansi.Truncate(s, w-3, "…")
		}
		switch l.Kind {
		case app.LineStep:
			s = sTitle.Render(s)
		case app.LineWarn:
			s = sWarn.Render(s)
		case app.LineRaw:
			s = sDim.Render(s)
		}
		b.WriteString(" " + s + "\n")
	}
	return b.String()
}

func button(s string, on bool) string {
	if on {
		return sButtonS.Render("[ " + s + " ]")
	}
	return sButton.Render("  " + s + "  ")
}

func mark(s app.CheckStatus) string {
	switch s {
	case app.CheckOK:
		return sOK.Render("[ OK ]")
	case app.CheckInfo:
		return sDim.Render("[info]")
	case app.CheckWarn:
		return sWarn.Render("[WARN]")
	case app.CheckFail:
		return sFail.Render("[FAIL]")
	}
	return sDim.Render("[ .. ]")
}

func stepMark(s app.StepState) string {
	switch s {
	case app.StepDone:
		return sOK.Render(s.Mark())
	case app.StepFailed:
		return sFail.Render(s.Mark())
	case app.StepRunning:
		return sTitle.Render(s.Mark())
	}
	return sDim.Render(s.Mark())
}

func bar(p app.Progress, width int) string {
	if width < 10 {
		width = 10
	}
	if width > 50 {
		width = 50
	}
	n := p.Percent() * width / 100
	return sTitle.Render(strings.Repeat("█", n)) + sDim.Render(strings.Repeat("░", width-n)) +
		fmt.Sprintf(" %3d%%  %s (%d / %d MB)", p.Percent(), p.Label, p.Done>>20, p.Total>>20)
}

// Run shows the TUI until the person closes it.
func Run(m *app.Model, c *app.Controller) error {
	t := New(m, c)
	p := tea.NewProgram(t, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
