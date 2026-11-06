package flasher

import (
	"fmt"

	"github.com/Shomer-TCL/shomer-flasher/internal/bundle"
)

// Step IDs: one per row on the TUI/GUI progress screen. PlanSteps lists them
// in the order the flow runs them; the flow reports each with an Event.
const (
	StepUnlock         = "unlock"
	StepBackupRecovery = "backup-recovery" // write recovery2 (reading recovery)
	StepBackupRead     = "backup-read"     // read every partition into the zip
	StepBackupRestore  = "backup-restore"  // write the stock TCL recovery back
	StepSuper          = "super"
	StepErase          = "erase"
	StepReboot         = "reboot"
)

// StepVBMeta is the step ID for one vbmeta partition.
func StepVBMeta(name string) string { return "vbmeta:" + name }

// EventKind says what happened.
type EventKind int

const (
	EvStepStart    EventKind = iota + 1
	EvStepDone               // step finished OK
	EvStepFailed             // step failed (Detail says why)
	EvStepSkipped            // step not run (Detail says why)
	EvPhone                  // the phone was found and identified (Phone)
	EvBackupSaved            // backup zip finished and checked (Path)
	EvBackupFailed           // backup failed (Detail); Path = INCOMPLETE zip if one was kept
	EvFinished               // the whole flash finished OK
)

func (k EventKind) String() string {
	return map[EventKind]string{EvStepStart: "start", EvStepDone: "done", EvStepFailed: "failed",
		EvStepSkipped: "skipped", EvPhone: "phone", EvBackupSaved: "backup-saved",
		EvBackupFailed: "backup-failed", EvFinished: "finished"}[k]
}

// Event is structured progress for the TUI/GUI.
type Event struct {
	Kind   EventKind
	Step   string
	Detail string
	Path   string
	Phone  *PhoneInfo
}

func (e Event) String() string {
	s := e.Kind.String()
	if e.Step != "" {
		s += " " + e.Step
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	if e.Path != "" {
		s += " (" + e.Path + ")"
	}
	if e.Phone != nil {
		s += fmt.Sprintf(" %+v", *e.Phone)
	}
	return s
}

// PhoneInfo is what the flasher found out about the connected phone.
type PhoneInfo struct {
	Serial        string         // fastboot serial
	Product       string         // fastboot getvar product
	Model         string         // ro.product.model, if adb could read it
	Build         string         // software build (KEKA/UPCI) as read or as typed by the person
	Unlock        UnlockState    // bootloader state when found
	Mode          ConnectionMode // separately verified; unsupported queries remain unknown
	BuildSource   string         // adb, typed, or skipped; entered identity is not detected identity
	DetectedBuild string
}

// Plan is what the person chose (TUI/GUI options screen, or CLI flags).
type Plan struct {
	Backup    bool   // backup-first
	BackupDir string // where the zip goes
	Userdata  bool   // include userdata (only if the bootloader was already unlocked)
	EraseData bool   // erase userdata after flashing (only asked if already unlocked; unlocking wipes anyway)
}

// PlannedStep is one row of the confirm and progress screens, with the exact
// commands it sends to the phone (the confirm screen lists every one).
type PlannedStep struct {
	ID       string
	Title    string
	Commands []string
	Writes   bool // this step writes to the phone
}

// PlanSteps is the one description of what a run will do, in order. The
// TUI/GUI confirm screen, the progress screen and the console's plan / dry run
// all come from here, so they cannot disagree.
func PlanSteps(m *bundle.Manifest, alreadyUnlocked bool, p Plan) []PlannedStep {
	var st []PlannedStep
	if !alreadyUnlocked {
		st = append(st, PlannedStep{ID: StepUnlock, Title: "Unlock the bootloader (press Volume Up on the phone) - ERASES all data on the phone",
			Commands: []string{"fastboot flashing unlock"}, Writes: true})
	}
	if p.Backup {
		what := "every partition except userdata"
		if p.Userdata && alreadyUnlocked {
			what = "every partition, userdata too"
		}
		st = append(st,
			PlannedStep{ID: StepBackupRecovery, Title: "Backup 1/3: write the reading recovery (neutronscott's recovery2)",
				Commands: []string{"fastboot flash recovery <backup recovery: neutronscott recovery2, sha256 " + DumpRecoverySHA256[:12] + "...>", "fastboot reboot recovery"}, Writes: true},
			PlannedStep{ID: StepBackupRead, Title: "Backup 2/3: copy " + what + " into a zip in " + p.BackupDir + " (read only)",
				Commands: []string{"adb exec-out \"su -c 'dd if=/dev/block/by-name/<each partition>'\"  -> backup-<serial>-<date>.zip in " + p.BackupDir + " (read only)", "adb reboot bootloader"}},
			PlannedStep{ID: StepBackupRestore, Title: "Backup 3/3: write the stock TCL recovery back (always, even if the backup fails)",
				Commands: []string{"fastboot flash recovery <stock TCL KEKA recovery, sha256 " + StockRecoverySHA256[:12] + "...>"}, Writes: true},
		)
	}
	for _, v := range m.VBMeta {
		st = append(st, PlannedStep{ID: StepVBMeta(v), Title: "Write " + v + " (stock KEKA, verification off)",
			Commands: []string{fmt.Sprintf("fastboot --disable-verity --disable-verification flash %s <stock KEKA %s>", v, v)}, Writes: true})
	}
	st = append(st, PlannedStep{ID: StepSuper, Title: fmt.Sprintf("Write super - Shomer OS %s (%d MB)", m.Build, m.SuperRaw.Size>>20),
		Commands: []string{"fastboot reboot fastboot (if not already in fastbootd)", "fastboot getvar is-userspace (must say yes)", fmt.Sprintf("fastboot flash super <Shomer OS %s, %d MB>", m.Build, m.SuperRaw.Size>>20)}, Writes: true})
	if p.EraseData && alreadyUnlocked {
		st = append(st, PlannedStep{ID: StepErase, Title: "Erase user data (contacts, messages, photos)",
			Commands: []string{"fastboot erase userdata"}, Writes: true})
	}
	st = append(st, PlannedStep{ID: StepReboot, Title: "Restart the phone", Commands: []string{"fastboot reboot"}})
	return st
}
