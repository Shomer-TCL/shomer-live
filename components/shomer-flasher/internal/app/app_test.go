package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

func manifest() *bundle.Manifest {
	return &bundle.Manifest{Name: "Shomer OS", Build: "2026-11-02_8a49ed1", VBMeta: []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"},
		SuperRaw: bundle.FileInfo{Size: 2 << 30}, Recovery: &bundle.RecoveryFiles{Dump: "r/dump", Stock: "r/stock"}}
}

func env(goos string, euid int, mm bool, free int64) Env {
	return Env{GOOS: goos, Exe: "./shomer-flasher-linux-x86_64", Euid: func() int { return euid },
		ProcessRunning: func(n string) bool { return mm && n == "ModemManager" },
		FreeBytes:      func(string) (int64, error) { return free, nil },
		FileExists:     func(string) bool { return false }, InGroup: func(string) bool { return false }}
}

func find(cs []Check, id string) Check {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return Check{ID: "missing"}
}

func TestChecksLinux(t *testing.T) {
	cs := RunChecks(env("linux", 1000, true, 10<<30), "/w", 2<<30, true, "/b", 3<<30)
	if c := find(cs, "sudo"); c.Status != CheckWarn || !strings.Contains(c.Fix, "sudo ./shomer-flasher-linux-x86_64") {
		t.Fatalf("sudo %+v", c)
	}
	if c := find(cs, "modemmanager"); c.Status != CheckWarn || !strings.Contains(c.Fix, "systemctl stop ModemManager") {
		t.Fatalf("mm %+v", c)
	}
	if find(cs, "disk-work").Status != CheckOK || find(cs, "disk-backup").Status != CheckOK {
		t.Fatalf("disk %+v", cs)
	}
	cs = RunChecks(env("linux", 0, false, 10<<30), "/w", 2<<30, true, "/b", 3<<30)
	if find(cs, "sudo").Status != CheckOK || find(cs, "modemmanager").Status != CheckOK {
		t.Fatalf("root %+v", cs)
	}
	e := env("linux", 1000, false, 10<<30)
	e.FileExists = func(p string) bool { return p == UdevRule }
	e.InGroup = func(g string) bool { return g == "dialout" }
	if find(RunChecks(e, "/w", 1, false, "/b", 1), "sudo").Status != CheckOK {
		t.Fatal("udev rule + dialout should be OK")
	}
}

func TestChecksDisk(t *testing.T) {
	cs := RunChecks(env("windows", 0, false, 1<<30), "/w", 2<<30, true, "/b", 3<<30)
	if find(cs, "sudo").ID != "missing" || find(cs, "modemmanager").ID != "missing" || find(cs, "drivers").Status != CheckInfo {
		t.Fatalf("windows rows %+v", cs)
	}
	if find(cs, "disk-work").Status != CheckFail || find(cs, "disk-backup").Status != CheckWarn {
		t.Fatalf("disk %+v", cs)
	}
	cs = RunChecks(env("linux", 0, false, 2500<<20), "/w", 2<<30, false, "/b", 3<<30)
	if find(cs, "disk-backup").Status != CheckInfo {
		t.Fatal("backup off should not warn about space")
	}
	m := NewModel(manifest(), "t", flasher.Plan{Backup: true}, "")
	m.Apply(Msg{Kind: MsgChecks, Checks: RunChecks(env("linux", 0, false, 1<<20), "/w", 2<<30, true, "/b", 3<<30)})
	m.Apply(Msg{Kind: MsgPrepared})
	if m.CanContinue() {
		t.Fatal("a failed check must block")
	}
}

// a whole run, with the messages in the order the flasher sends them
func TestModelLockedPhoneHappyPath(t *testing.T) {
	m := NewModel(manifest(), "1.4.1", flasher.Plan{Backup: true, BackupDir: "/home/sam/shomer-backups", EraseData: true}, "/tmp/x.log")
	if m.CanContinue() || m.Continue() != ActNone {
		t.Fatal("cannot continue before the ROM check")
	}
	m.Apply(Msg{Kind: MsgChecks, Checks: []Check{{ID: "rom", Status: CheckRunning}}})
	m.Apply(Msg{Kind: MsgProgress, Progress: Progress{Label: "checking", Done: 50, Total: 100}})
	if m.Progress.Percent() != 50 {
		t.Fatal("progress")
	}
	m.Apply(Msg{Kind: MsgCheck, Check: Check{ID: "rom", Status: CheckOK}})
	m.Apply(Msg{Kind: MsgPrepared})
	if !m.CanContinue() || m.Continue() != ActDetect || m.Screen != ScreenPhone || !m.Busy() {
		t.Fatalf("checks -> phone: %+v", m)
	}
	// the flasher asks to power off the phone: the question carries the instructions
	m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineStep, Text: "Getting the phone into FASTBOOT mode (try 1)"}})
	m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineSay, Text: "1. UNPLUG the USB cable from the phone."}})
	q := &Question{Kind: QEnter, Prompt: "Press Enter when the phone is OFF and UNPLUGGED...", ch: make(chan Answer, 1)}
	m.Apply(Msg{Kind: MsgQuestion, Question: q})
	if m.Question == nil || len(m.Question.Context) != 2 || !strings.Contains(m.Question.Context[1], "UNPLUG") || m.CanContinue() {
		t.Fatalf("question %+v", m.Question)
	}
	m.Answer(Answer{})
	if a := <-q.ch; a.Cancel || m.Question != nil {
		t.Fatal("answer")
	}
	m.Apply(Msg{Kind: MsgDetected, Phone: flasher.PhoneInfo{Serial: "ABC", Product: "gflip6", Model: "T408DL", Build: "KEKA", Unlock: flasher.Locked}})
	if !strings.Contains(m.PhoneSummary(), "serial ABC") || !strings.Contains(m.PhoneSummary(), "LOCKED") {
		t.Fatal(m.PhoneSummary())
	}
	m.Continue()
	if m.Screen != ScreenOptions || m.Plan.EraseData {
		t.Fatalf("locked phone: erase must be off (unlock wipes): %+v", m.Plan)
	}
	// userdata and erase cannot be switched on for a locked phone
	m.Toggle(OptUserdata)
	m.Toggle(OptErase)
	if m.Plan.Userdata || m.Plan.EraseData {
		t.Fatal("toggled a disabled option")
	}
	m.Continue()
	if m.Screen != ScreenConfirm {
		t.Fatal("confirm")
	}
	ids := []string{}
	for _, s := range m.ConfirmSteps() {
		ids = append(ids, s.ID)
	}
	want := "unlock,backup-recovery,backup-read,backup-restore,vbmeta:vbmeta,vbmeta:vbmeta_system,vbmeta:vbmeta_vendor,super,reboot"
	if strings.Join(ids, ",") != want {
		t.Fatalf("steps %v", ids)
	}
	txt := strings.Join(m.ConfirmLines(), "\n")
	for _, s := range []string{"fastboot flashing unlock", "fastboot flash recovery <backup recovery", "fastboot flash recovery <stock TCL KEKA recovery",
		"flash super <Shomer OS", "ERASES EVERYTHING", "/home/sam/shomer-backups", "asked whether to flash without it"} {
		if !strings.Contains(txt, s) {
			t.Fatalf("confirm misses %q:\n%s", s, txt)
		}
	}
	if !strings.Contains(m.FinalQuestion(), "Unlock") || m.Continue() != ActFlash {
		t.Fatal("final")
	}
	m.StartFlash()
	if m.Screen != ScreenProgress || len(m.Steps) != 9 || !m.QuitNeedsConfirm() {
		t.Fatal("progress")
	}
	for _, e := range []flasher.Event{{Kind: flasher.EvStepStart, Step: "unlock"}, {Kind: flasher.EvStepDone, Step: "unlock"},
		{Kind: flasher.EvStepStart, Step: "backup-recovery"}} {
		m.Apply(Msg{Kind: MsgEvent, Event: e})
	}
	if i, n := m.CurrentStep(); i != 2 || n != 9 || m.Steps[0].State != StepDone {
		t.Fatalf("current %d/%d", i, n)
	}
	for _, e := range []flasher.Event{{Kind: flasher.EvStepDone, Step: "backup-recovery"}, {Kind: flasher.EvStepStart, Step: "backup-read"},
		{Kind: flasher.EvStepDone, Step: "backup-read"}, {Kind: flasher.EvBackupSaved, Path: "/b/backup-ABC-1.zip"},
		{Kind: flasher.EvStepStart, Step: "backup-restore"}, {Kind: flasher.EvStepDone, Step: "backup-restore"},
		{Kind: flasher.EvStepStart, Step: "super"}, {Kind: flasher.EvStepDone, Step: "super"}} {
		m.Apply(Msg{Kind: MsgEvent, Event: e})
	}
	m.Apply(Msg{Kind: MsgFinished})
	if m.Screen != ScreenDone || !m.Result.OK {
		t.Fatal("done")
	}
	d := strings.Join(m.DoneLines(), "\n")
	if !strings.Contains(d, "DONE") || !strings.Contains(d, "/b/backup-ABC-1.zip") || !strings.Contains(d, "/tmp/x.log") {
		t.Fatal(d)
	}
	if m.Continue() != ActQuit {
		t.Fatal("close")
	}
}

func TestModelUnlockedOptionsAndBackupFailure(t *testing.T) {
	m := NewModel(manifest(), "1.4.1", flasher.Plan{Backup: true, BackupDir: "/b", EraseData: true}, "")
	m.Prepared = true
	m.Continue()
	m.Apply(Msg{Kind: MsgDetected, Phone: flasher.PhoneInfo{Product: "gflip6", Unlock: flasher.Unlocked}})
	m.Continue()
	rows := m.Options()
	if len(rows) != 4 || !rows[2].Enabled || !rows[3].Enabled || rows[3].Value != "ON" {
		t.Fatalf("rows %+v", rows)
	}
	m.Toggle(OptUserdata)
	if !m.Plan.Userdata {
		t.Fatal("userdata")
	}
	// backup off -> userdata off and disabled, folder disabled
	m.Toggle(OptBackup)
	if m.Plan.Backup || m.Plan.Userdata || m.Options()[1].Enabled || m.Options()[2].Enabled {
		t.Fatalf("%+v", m.Plan)
	}
	m.Toggle(OptBackup)
	m.SetBackupDir("  ")
	if m.CanContinue() {
		t.Fatal("empty backup folder accepted")
	}
	m.SetBackupDir("/b2")
	m.Continue()
	ids := []string{}
	for _, s := range m.ConfirmSteps() {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "backup-recovery,backup-read,backup-restore,vbmeta:vbmeta,vbmeta:vbmeta_system,vbmeta:vbmeta_vendor,super,erase,reboot" {
		t.Fatalf("steps %v", ids)
	}
	if strings.Contains(m.FinalQuestion(), "Unlock") {
		t.Fatal("unlocked phone asked to unlock")
	}
	m.Back()
	if m.Screen != ScreenOptions {
		t.Fatal("back")
	}
	m.Continue()
	m.StartFlash()
	m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepStart, Step: "backup-recovery"}})
	m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepFailed, Step: "backup-recovery", Detail: "no root"}})
	m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvBackupFailed, Detail: "no root"}})
	if m.Steps[1].State != StepSkipped || m.Steps[0].State != StepFailed {
		t.Fatalf("%+v", m.Steps[:2])
	}
	q := &Question{Kind: QYesNo, Prompt: "Backup failed: continue flashing WITHOUT a backup?", ch: make(chan Answer, 1)}
	m.Apply(Msg{Kind: MsgQuestion, Question: q})
	m.Answer(Answer{Yes: true})
	if a := <-q.ch; !a.Yes {
		t.Fatal("answer")
	}
	m.Apply(Msg{Kind: MsgFinished})
	if d := strings.Join(m.DoneLines(), "\n"); !strings.Contains(d, "flash without it: no root") {
		t.Fatal(d)
	}
}

func TestModelErrors(t *testing.T) {
	m := NewModel(manifest(), "1.4.1", flasher.Plan{}, "")
	m.Apply(Msg{Kind: MsgPrepared, Err: errors.New("bad checksum")})
	if m.CanContinue() || find(m.Checks, "rom").Status != CheckFail {
		t.Fatal("bad ROM must block")
	}
	m = NewModel(manifest(), "1.4.1", flasher.Plan{}, "")
	m.Prepared = true
	m.Continue()
	m.Apply(Msg{Kind: MsgDetected, Err: flasher.ErrRefused{Msg: "wrong phone"}})
	if m.Screen != ScreenDone || !m.Result.Refused || !strings.Contains(m.DoneLines()[0], "STOPPED: wrong phone") {
		t.Fatal(m.DoneLines())
	}
	m = NewModel(manifest(), "1.4.1", flasher.Plan{Backup: true, BackupDir: "/b"}, "")
	m.Steps = []StepRow{{PlannedStep: flasher.PlannedStep{ID: "super"}, State: StepRunning}}
	m.Phase = PhaseWriting
	m.Apply(Msg{Kind: MsgFinished, Err: flasher.ErrRecoveryNotRestored{Err: errors.New("usb")}})
	if m.Steps[0].State != StepFailed || !strings.Contains(strings.Join(m.DoneLines(), "\n"), "puts the stock recovery back") {
		t.Fatal(m.DoneLines())
	}
}

func TestChanUIQuestionsAndLines(t *testing.T) {
	out := make(chan Msg, 100)
	quit := make(chan struct{})
	u := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out, quit: quit}
	go func() {
		for m := range out {
			if m.Kind == MsgQuestion {
				switch m.Question.Kind {
				case QYesNo:
					m.Question.reply(Answer{Yes: true})
				case QText:
					m.Question.reply(Answer{Text: " KEKA "})
				case QWord:
					m.Question.reply(Answer{Text: "flash"})
				case QEnter:
					m.Question.reply(Answer{Cancel: true})
				}
			}
		}
	}()
	if y, err := u.YesNo("go?", false); !y || err != nil {
		t.Fatal("yesno")
	}
	if s, _ := u.Ask("build?"); s != "KEKA" {
		t.Fatal("ask")
	}
	if ok, _ := u.Confirm("type FLASH", "FLASH"); !ok {
		t.Fatal("confirm")
	}
	if err := u.Enter("press enter"); !errors.Is(err, flasher.ErrInputClosed) {
		t.Fatal("cancel must read as closed input")
	}
	close(out)
	// quitting answers a pending question as closed
	out2 := make(chan Msg, 10)
	q2 := make(chan struct{})
	u2 := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out2, quit: q2}
	done := make(chan error)
	go func() { _, err := u2.YesNo("x", false); done <- err }()
	<-out2
	close(q2)
	select {
	case err := <-done:
		if !errors.Is(err, flasher.ErrInputClosed) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("question not released by quit")
	}
	// raw output becomes lines; progress is throttled
	out3 := make(chan Msg, 1000)
	u3 := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out3, quit: make(chan struct{})}
	u3.Raw("Sending 'super' 1/16")
	u3.Raw(" OKAY\nWriting ")
	u3.Raw("'super' OKAY\n")
	for i := int64(0); i <= 1000; i++ {
		u3.Progress("reading", i, 1000)
	}
	var lines []string
	np := 0
	for len(out3) > 0 {
		m := <-out3
		if m.Kind == MsgLine {
			lines = append(lines, m.Line.Text)
		}
		if m.Kind == MsgProgress {
			np++
		}
	}
	if strings.Join(lines, "|") != "Sending 'super' 1/16 OKAY|Writing 'super' OKAY" {
		t.Fatalf("lines %q", lines)
	}
	if np > 110 || np < 2 {
		t.Fatalf("progress not throttled: %d", np)
	}
}

type discard struct{}

func (discard) Write(b []byte) (int, error) { return len(b), nil }
