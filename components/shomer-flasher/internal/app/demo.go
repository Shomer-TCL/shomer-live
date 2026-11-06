package app

import (
	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// DemoScreen is one example screen (README captures, GUI smoke test).
type DemoScreen struct {
	Name  string
	Setup func(m *Model)
}

// DemoManifest looks like the real 1.4.1 payload.
func DemoManifest() *bundle.Manifest {
	return &bundle.Manifest{Name: "Shomer OS", Build: "2026-11-02_8a49ed1", VBMeta: []string{"vbmeta", "vbmeta_system", "vbmeta_vendor"},
		SuperRaw: bundle.FileInfo{Size: 2147483648}, Recovery: &bundle.RecoveryFiles{Dump: "recovery/recovery2.img", Stock: "recovery/stock-KEKA.img"}}
}

// DemoScreens walks one locked phone through every screen. Each Setup builds
// on the previous one, so apply them in order to one Model.
func DemoScreens(backupDir string) []DemoScreen {
	phone := flasher.PhoneInfo{Serial: "1A2B3C4D", Product: "gflip6", Model: "T408DL", Build: "KEKA", Unlock: flasher.Locked, Mode: flasher.ModeBootloader, BuildSource: "adb", DetectedBuild: "KEKA"}
	return []DemoScreen{
		{"checks", func(m *Model) {
			m.Apply(Msg{Kind: MsgChecks, Checks: []Check{
				{ID: "sudo", Name: "Permission to use USB", Status: CheckOK, Detail: "running as root (sudo)"},
				{ID: "modemmanager", Name: "ModemManager", Status: CheckWarn, Detail: "running - it can grab the phone's port while we try to catch it",
					Fix: "In another terminal:  sudo systemctl stop ModemManager   (start it again after flashing)"},
				{ID: "disk-work", Name: "Free disk for the ROM", Status: CheckOK, Detail: "41230 MB free in /home/sam/shomer-flasher-work (needs 2248 MB)"},
				{ID: "disk-backup", Name: "Free disk for the backup", Status: CheckOK, Detail: "41230 MB free in " + backupDir + " (needs about 2900 MB)"},
				{ID: "rom", Name: "ROM inside this program", Status: CheckOK, Detail: "every file matches its SHA-256"},
			}})
			m.Apply(Msg{Kind: MsgPrepared})
		}},
		{"phone", func(m *Model) {
			m.Continue()
			m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineStep, Text: "Getting the phone into FASTBOOT mode (try 1)"}})
			m.Apply(Msg{Kind: MsgLine, Line: Line{Text: "    Phone answered \"READYTOO\" on /dev/ttyACM0 - it is starting FASTBOOT mode."}})
			m.Apply(Msg{Kind: MsgDetected, Phone: phone})
		}},
		{"options", func(m *Model) { m.Continue() }},
		{"confirm", func(m *Model) { m.Continue() }},
		{"progress", func(m *Model) {
			m.StartFlash()
			for _, e := range []flasher.Event{{Kind: flasher.EvStepStart, Step: flasher.StepUnlock}, {Kind: flasher.EvStepDone, Step: flasher.StepUnlock},
				{Kind: flasher.EvStepStart, Step: flasher.StepBackupRecovery}, {Kind: flasher.EvStepDone, Step: flasher.StepBackupRecovery},
				{Kind: flasher.EvStepStart, Step: flasher.StepBackupRead}} {
				m.Apply(Msg{Kind: MsgEvent, Event: e})
			}
			m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineStep, Text: "Copying 41 partitions (2771 MB) into " + backupDir + "/backup-1A2B3C4D-20261103-101500.zip"}})
			m.Apply(Msg{Kind: MsgLine, Line: Line{Text: "    [12/41] md1img (64 MB)"}})
			m.Apply(Msg{Kind: MsgProgress, Progress: Progress{Label: "reading [12/41] md1img", Done: 40 << 20, Total: 64 << 20}})
		}},
		{"backup-failed", func(m *Model) {
			m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepFailed, Step: flasher.StepBackupRead, Detail: "missing: nvram"}})
			m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepStart, Step: flasher.StepBackupRestore}})
			m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepDone, Step: flasher.StepBackupRestore}})
			m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvBackupFailed, Detail: "these partitions could not be copied and checked: nvram. The backup is INCOMPLETE",
				Path: backupDir + "/backup-1A2B3C4D-20261103-101500-INCOMPLETE.zip"}})
			m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineStep, Text: "Backup failed"}})
			m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineWarn, Text: "BACKUP FAILED: these partitions could not be copied and checked: nvram. The backup is INCOMPLETE"}})
			m.Apply(Msg{Kind: MsgLine, Line: Line{Kind: LineWarn, Text: "The stock TCL recovery is back on the phone. Nothing else was written yet."}})
			m.Apply(Msg{Kind: MsgQuestion, Question: NewTestQuestion(QYesNo, "    Backup failed: continue flashing WITHOUT a backup?", false)})
		}},
		{"done", func(m *Model) {
			m.Answer(Answer{Yes: true})
			for _, id := range []string{"vbmeta:vbmeta", "vbmeta:vbmeta_system", "vbmeta:vbmeta_vendor", flasher.StepSuper, flasher.StepReboot} {
				m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepStart, Step: id}})
				m.Apply(Msg{Kind: MsgEvent, Event: flasher.Event{Kind: flasher.EvStepDone, Step: id}})
			}
			m.Apply(Msg{Kind: MsgFinished})
		}},
	}
}
