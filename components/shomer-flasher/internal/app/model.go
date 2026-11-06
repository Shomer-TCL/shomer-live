package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// Screen is what the person sees.
type Screen int

const (
	ScreenChecks   Screen = iota // start-up checks + ROM check
	ScreenPhone                  // find the phone (instructions), then what it is
	ScreenOptions                // backup on/off, folder, userdata, erase
	ScreenConfirm                // exactly what will be written
	ScreenProgress               // live progress per step / partition
	ScreenDone                   // done or error, with the backup zip path
)

var screenTitles = [...]string{"1. Checks", "2. Phone", "3. Options", "4. Confirm", "5. Writing", "6. Finished"}

func (s Screen) Title() string { return screenTitles[s] }

// Phase is the work running in the background.
type Phase int

const (
	PhaseIdle Phase = iota
	PhasePreparing
	PhaseDetecting
	PhaseWriting
)

// Line is one line of the activity log.
type Line struct {
	Kind LineKind
	Text string
}

type LineKind int

const (
	LineSay LineKind = iota
	LineStep
	LineWarn
	LineRaw
)

// Progress is the current progress bar.
type Progress struct {
	Label       string
	Done, Total int64
}

// Percent is 0..100.
func (p Progress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	n := int(p.Done * 100 / p.Total)
	if n > 100 {
		n = 100
	}
	return n
}

// StepState is the state of one progress row.
type StepState int

const (
	StepWaiting StepState = iota
	StepRunning
	StepDone
	StepFailed
	StepSkipped
)

func (s StepState) Mark() string {
	return [...]string{"[ ]", "[>]", "[x]", "[!]", "[-]"}[s]
}

// StepRow is one row of the progress screen.
type StepRow struct {
	flasher.PlannedStep
	State  StepState
	Detail string
}

// Result is how the run ended.
type Result struct {
	OK             bool
	Refused        bool // stopped on purpose (person said no, wrong phone...): ErrRefused
	Err            error
	BackupPath     string
	IncompletePath string
	BackupFailed   string // why the backup failed (when the person went on without it)
	LogPath        string
}

// OptionID names one row of the options screen.
type OptionID int

const (
	OptBackup OptionID = iota
	OptBackupDir
	OptUserdata
	OptErase
)

// OptionRow is one row of the options screen.
type OptionRow struct {
	ID      OptionID
	Label   string
	Value   string // "ON"/"OFF" for toggles, the folder for OptBackupDir
	Toggle  bool
	Enabled bool
	Help    string
}

// Model is the whole state of the TUI/GUI. Front ends draw it and call its
// actions; Apply feeds it the flasher's messages.
type Model struct {
	Screen   Screen
	Phase    Phase
	Manifest *bundle.Manifest
	Version  string

	Checks   []Check
	Prepared bool
	PrepErr  error

	Phone *flasher.PhoneInfo
	Plan  flasher.Plan

	Steps    []StepRow
	Progress Progress
	Lines    []Line
	Question *Question
	Result   Result

	sinceStep []string // lines said since the last heading: the context of a question
}

// MaxLines is how much of the activity log is kept for the screen.
const MaxLines = 400

// NewModel starts on the checks screen. plan is the defaults (CLI flags).
func NewModel(m *bundle.Manifest, version string, plan flasher.Plan, logPath string) *Model {
	return &Model{Screen: ScreenChecks, Manifest: m, Version: version, Plan: plan, Result: Result{LogPath: logPath}}
}

// Busy is true while background work runs.
func (m *Model) Busy() bool { return m.Phase != PhaseIdle }

// ChecksBlock is true if a check failed (cannot go on).
func (m *Model) ChecksBlock() bool {
	for _, c := range m.Checks {
		if c.Status == CheckFail {
			return true
		}
	}
	return false
}

// PhoneUnlocked: the bootloader was already unlocked when the phone was found.
func (m *Model) PhoneUnlocked() bool {
	return m.Phone != nil && m.Phone.Unlock == flasher.Unlocked
}

// CanContinue says whether the main button (Enter on "Continue") works now.
func (m *Model) CanContinue() bool {
	if m.Question != nil {
		return false
	}
	switch m.Screen {
	case ScreenChecks:
		return m.Prepared && m.PrepErr == nil && !m.ChecksBlock() && !m.Busy()
	case ScreenPhone:
		return m.Phone != nil && !m.Busy()
	case ScreenOptions:
		return !m.Plan.Backup || flasher.CleanPath(m.Plan.BackupDir) != ""
	case ScreenConfirm:
		return true
	case ScreenDone:
		return true
	}
	return false
}

// ContinueLabel is the text of the main button.
func (m *Model) ContinueLabel() string {
	switch m.Screen {
	case ScreenChecks:
		if !m.Prepared {
			return "Checking..."
		}
		return "Continue: find the phone"
	case ScreenPhone:
		if m.Phone == nil {
			return "Looking for the phone..."
		}
		return "Continue: options"
	case ScreenOptions:
		return "Continue: review"
	case ScreenConfirm:
		return "FLASH NOW"
	case ScreenProgress:
		return "Working..."
	case ScreenDone:
		return "Close"
	}
	return "Continue"
}

// Action is what the front end must do after a model action.
type Action int

const (
	ActNone   Action = iota
	ActDetect        // call Controller.Detect
	ActFlash         // ask the final yes/no, then Controller.Flash(m.Plan)
	ActQuit          // close the program
)

// Continue moves to the next screen if allowed.
func (m *Model) Continue() Action {
	if !m.CanContinue() {
		return ActNone
	}
	switch m.Screen {
	case ScreenChecks:
		m.Screen = ScreenPhone
		m.Phase = PhaseDetecting
		return ActDetect
	case ScreenPhone:
		m.normalizePlan()
		m.Screen = ScreenOptions
	case ScreenOptions:
		m.normalizePlan()
		m.Screen = ScreenConfirm
	case ScreenConfirm:
		return ActFlash
	case ScreenDone:
		return ActQuit
	}
	return ActNone
}

// FinalQuestion is the yes/no asked before writing starts (default No).
func (m *Model) FinalQuestion() string {
	if !m.PhoneUnlocked() {
		return "Unlock the bootloader (this ERASES everything on the phone) and write Shomer OS now?"
	}
	return "Write Shomer OS to this phone now?"
}

// StartFlash is called after the person said yes to FinalQuestion.
func (m *Model) StartFlash() {
	m.normalizePlan()
	m.Steps = nil
	for _, s := range m.ConfirmSteps() {
		m.Steps = append(m.Steps, StepRow{PlannedStep: s})
	}
	m.Screen = ScreenProgress
	m.Phase = PhaseWriting
	m.Progress = Progress{}
}

// Back goes to the previous screen where that makes sense.
func (m *Model) Back() {
	if m.Busy() || m.Question != nil {
		return
	}
	switch m.Screen {
	case ScreenConfirm:
		m.Screen = ScreenOptions
	case ScreenOptions:
		m.Screen = ScreenPhone
	}
}

// QuitNeedsConfirm: quitting now could leave the phone half done.
func (m *Model) QuitNeedsConfirm() bool { return m.Phase == PhaseWriting }

// QuitWarning is shown before quitting while writing.
const QuitWarning = "The flasher is writing to the phone. Quitting now can leave the backup recovery on the phone or the phone half written (both fixable by running this program again). Quit anyway?"

// normalizePlan switches off choices that do not apply to this phone.
func (m *Model) normalizePlan() {
	m.Plan.BackupDir = flasher.CleanPath(m.Plan.BackupDir)
	if !m.PhoneUnlocked() {
		m.Plan.Userdata = false // unlocking erases it: nothing to save
		m.Plan.EraseData = false
	}
	if !m.Plan.Backup {
		m.Plan.Userdata = false
	}
}

// Options are the rows of the options screen.
func (m *Model) Options() []OptionRow {
	onoff := func(b bool) string {
		if b {
			return "ON"
		}
		return "OFF"
	}
	unl := m.PhoneUnlocked()
	rows := []OptionRow{
		{ID: OptBackup, Label: "Backup the phone first", Value: onoff(m.Plan.Backup), Toggle: true, Enabled: true,
			Help: "Copies every partition into a zip on this computer before flashing. Recommended. If it fails you can still flash without it."},
		{ID: OptBackupDir, Label: "Backup folder", Value: m.Plan.BackupDir, Enabled: m.Plan.Backup,
			Help: "Where the zip is saved (needs about 3 GB free)."},
	}
	ud := OptionRow{ID: OptUserdata, Label: "Include phone data (userdata)", Value: onoff(m.Plan.Userdata && unl && m.Plan.Backup), Toggle: true,
		Enabled: unl && m.Plan.Backup, Help: "Adds ~4.6 GB of encrypted phone data. Only possible because the bootloader is already unlocked."}
	if !unl {
		ud.Help = "Not possible: the bootloader is locked, and unlocking erases the phone data before it can be read."
	}
	rows = append(rows, ud)
	er := OptionRow{ID: OptErase, Label: "Erase phone data after flashing", Value: onoff(m.Plan.EraseData && unl), Toggle: true, Enabled: unl,
		Help: "Strongly recommended: old apps' data can stop Shomer from starting."}
	if !unl {
		er.Help = "Unlocking the bootloader erases the phone anyway."
	}
	return append(rows, er)
}

// Toggle flips a toggle row (if enabled).
func (m *Model) Toggle(id OptionID) {
	for _, r := range m.Options() {
		if r.ID == id && (!r.Toggle || !r.Enabled) {
			return
		}
	}
	switch id {
	case OptBackup:
		m.Plan.Backup = !m.Plan.Backup
		if !m.Plan.Backup {
			m.Plan.Userdata = false
		}
	case OptUserdata:
		m.Plan.Userdata = !m.Plan.Userdata
	case OptErase:
		m.Plan.EraseData = !m.Plan.EraseData
	}
}

// SetBackupDir sets the backup folder.
// It keeps what is typed (the window calls it on every key; rewriting the
// box while typing moves the caret). normalizePlan, on leaving the options
// screen, removes quotes around a pasted path (Explorer's "Copy as path" adds
// them) and makes it a full path (like --backup-dir).
func (m *Model) SetBackupDir(dir string) { m.Plan.BackupDir = strings.TrimSpace(dir) }

// ConfirmSteps is exactly what will run, in order (same source as the
// console's plan and dry run).
func (m *Model) ConfirmSteps() []flasher.PlannedStep {
	if m.Manifest == nil {
		return nil
	}
	return flasher.PlanSteps(m.Manifest, m.PhoneUnlocked(), m.Plan)
}

// ConfirmLines is the confirm screen as text.
func (m *Model) ConfirmLines() []string {
	var l []string
	if m.Phone != nil {
		l = append(l, fmt.Sprintf("Phone: %s", m.PhoneSummary()))
	}
	l = append(l, "", "These steps run, in this order. Only the partitions named here are written:")
	n := 0
	for _, s := range m.ConfirmSteps() {
		n++
		l = append(l, fmt.Sprintf("%2d. %s", n, s.Title))
		for _, c := range s.Commands {
			l = append(l, "      "+c)
		}
	}
	l = append(l, "", "Never written: IMEI (proinfo/nvram/nvdata), modem, boot, lk, preloader.")
	if !m.PhoneUnlocked() {
		l = append(l, "UNLOCKING ERASES EVERYTHING ON THE PHONE (contacts, messages, photos). It cannot be undone.",
			"After unlocking the phone shows an 'orange state' warning for a few seconds at every start. That is normal.")
	}
	if m.Plan.Backup {
		l = append(l, "If the backup fails you will be asked whether to flash without it. The stock recovery is always put back first.")
	} else {
		l = append(l, "Backup is OFF: no copy of the phone is made, and recovery is not touched.")
	}
	l = append(l, "Takes about 5-15 minutes (backup adds 5-15). Do NOT unplug the cable while it runs.")
	return l
}

// PhoneSummary is one line about the phone.
func (m *Model) PhoneSummary() string {
	if m.Phone == nil {
		return "not found yet"
	}
	p := m.Phone
	var parts []string
	if p.Model != "" {
		parts = append(parts, "model "+p.Model)
	}
	parts = append(parts, "product "+p.Product)
	if p.Serial != "" {
		parts = append(parts, "serial "+p.Serial)
	}
	if p.Build != "" && p.Build != "skipped" {
		parts = append(parts, "software "+p.Build)
	}
	parts = append(parts, "bootloader "+p.Unlock.String())
	return strings.Join(parts, ", ")
}

// DoneLines is the finished screen as text.
func (m *Model) DoneLines() []string {
	r := m.Result
	var l []string
	switch {
	case r.OK:
		l = append(l, "DONE - Shomer OS has been written.")
	case r.Refused:
		l = append(l, "STOPPED: "+errText(r.Err))
	case r.Err != nil:
		l = append(l, "ERROR: "+errText(r.Err))
	}
	if r.BackupPath != "" {
		l = append(l, "", "Backup of the phone as it was (keep it private, it holds the IMEI):", "  "+r.BackupPath)
	}
	if r.IncompletePath != "" {
		l = append(l, "", "INCOMPLETE backup (some partitions are missing):", "  "+r.IncompletePath)
	}
	if r.BackupFailed != "" && r.OK {
		l = append(l, "", "The backup failed and you chose to flash without it: "+r.BackupFailed)
	}
	if r.OK {
		l = append(l, "",
			"What happens now:",
			"- The phone shows an 'orange state' warning for a few seconds at every start. Normal.",
			"- The FIRST start takes up to 5-10 minutes. Leave it alone.",
			"- Add this phone's IMEI in the Shomer admin portal (dial *#06# or see the sticker).",
			"- Connect it to the internet (SIM data or Wi-Fi) so it can fetch its level.")
	} else if r.Err != nil {
		var nr flasher.ErrRecoveryNotRestored
		if errors.As(r.Err, &nr) {
			l = append(l, "", "IMPORTANT: run this program again. It puts the stock recovery back before anything else.")
		} else {
			l = append(l, "", "You can run this program again at any time: it catches the phone the same way.")
		}
	}
	if r.LogPath != "" {
		l = append(l, "", "Log file: "+r.LogPath)
	}
	return l
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// stepIndex finds a progress row.
func (m *Model) stepIndex(id string) int {
	for i, s := range m.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// Apply feeds one message from the controller into the model.
func (m *Model) Apply(msg Msg) {
	switch msg.Kind {
	case MsgLine:
		m.addLine(msg.Line)
	case MsgProgress:
		m.Progress = msg.Progress
	case MsgQuestion:
		q := msg.Question
		q.Context = append([]string(nil), m.sinceStep...)
		m.Question = q
	case MsgChecks:
		m.Checks = msg.Checks
	case MsgCheck:
		m.setCheck(msg.Check)
	case MsgPrepared:
		m.Prepared = true
		m.PrepErr = msg.Err
		m.Phase = PhaseIdle
		if msg.Err != nil {
			m.setCheck(Check{ID: "rom", Name: "ROM inside this program", Status: CheckFail, Detail: msg.Err.Error()})
		}
	case MsgDetected:
		m.Phase = PhaseIdle
		if msg.Err != nil {
			m.finish(msg.Err)
			return
		}
		p := msg.Phone
		m.Phone = &p
		m.normalizePlan()
	case MsgEvent:
		m.applyEvent(msg.Event)
	case MsgFinished:
		m.Phase = PhaseIdle
		m.finish(msg.Err)
	}
}

func (m *Model) finish(err error) {
	m.Screen = ScreenDone
	m.Question = nil
	m.Result.OK = err == nil
	m.Result.Err = err
	var r flasher.ErrRefused
	m.Result.Refused = errors.As(err, &r) || errors.Is(err, flasher.ErrCanceled)
	for i := range m.Steps {
		if m.Steps[i].State == StepRunning {
			m.Steps[i].State = StepFailed
		}
	}
}

func (m *Model) setCheck(c Check) {
	for i := range m.Checks {
		if m.Checks[i].ID == c.ID {
			m.Checks[i] = c
			return
		}
	}
	m.Checks = append(m.Checks, c)
}

func (m *Model) addLine(l Line) {
	if l.Kind == LineStep {
		m.sinceStep = nil
	}
	if strings.TrimSpace(l.Text) != "" && l.Kind != LineRaw {
		m.sinceStep = append(m.sinceStep, l.Text)
		if len(m.sinceStep) > 30 {
			m.sinceStep = m.sinceStep[len(m.sinceStep)-30:]
		}
	}
	m.Lines = append(m.Lines, l)
	if len(m.Lines) > MaxLines {
		m.Lines = m.Lines[len(m.Lines)-MaxLines:]
	}
}

func (m *Model) applyEvent(e flasher.Event) {
	switch e.Kind {
	case flasher.EvStepStart, flasher.EvStepDone, flasher.EvStepFailed, flasher.EvStepSkipped:
		i := m.stepIndex(e.Step)
		if i < 0 {
			return
		}
		m.Steps[i].State = map[flasher.EventKind]StepState{flasher.EvStepStart: StepRunning, flasher.EvStepDone: StepDone,
			flasher.EvStepFailed: StepFailed, flasher.EvStepSkipped: StepSkipped}[e.Kind]
		m.Steps[i].Detail = e.Detail
		if e.Kind == flasher.EvStepStart {
			m.Progress = Progress{}
		}
	case flasher.EvBackupSaved:
		m.Result.BackupPath = e.Path
	case flasher.EvBackupFailed:
		m.Result.BackupFailed = e.Detail
		m.Result.IncompletePath = e.Path
		// the backup rows that never ran
		for _, id := range []string{flasher.StepBackupRecovery, flasher.StepBackupRead} {
			if i := m.stepIndex(id); i >= 0 && m.Steps[i].State == StepWaiting {
				m.Steps[i].State = StepSkipped
			}
		}
	case flasher.EvPhone:
		if e.Phone != nil {
			p := *e.Phone
			m.Phone = &p
		}
	}
}

// Answer answers the open question and closes it.
func (m *Model) Answer(a Answer) {
	if m.Question == nil {
		return
	}
	m.Question.reply(a)
	m.Question = nil
}

// CurrentStep is the running progress row (for "step 3 of 9").
func (m *Model) CurrentStep() (int, int) {
	for i, s := range m.Steps {
		if s.State == StepRunning {
			return i + 1, len(m.Steps)
		}
	}
	return 0, len(m.Steps)
}
