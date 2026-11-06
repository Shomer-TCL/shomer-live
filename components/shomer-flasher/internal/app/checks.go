// Package app is the shared flow and state of the Linux TUI and the Windows
// GUI: which screen is shown, what the person chose, what the phone said, and
// the messages the flasher sends while it works. Both front ends only draw
// this Model and call its actions, so they cannot drift apart.
package app

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// CheckStatus is the result of one start-up check.
type CheckStatus int

const (
	CheckRunning CheckStatus = iota
	CheckOK
	CheckInfo // nothing wrong, just something to know
	CheckWarn // can go on, but read the fix
	CheckFail // cannot go on
)

func (s CheckStatus) String() string {
	return [...]string{"...", "OK", "i", "!", "X"}[s]
}

// Check is one row on the checks screen.
type Check struct {
	ID     string
	Name   string
	Status CheckStatus
	Detail string
	Fix    string
}

// Env is the computer the program runs on (tests replace every function).
type Env struct {
	GOOS           string
	Exe            string
	Euid           func() int
	ProcessRunning func(name string) bool
	FreeBytes      func(dir string) (int64, error)
	FileExists     func(path string) bool
	InGroup        func(group string) bool
	// SameDisk: two existing folders are on the same disk (nil = unknown)
	SameDisk func(a, b string) bool
}

// RealEnv is this computer.
func RealEnv(goos, exe string, free func(string) (int64, error)) Env {
	return Env{
		GOOS:           goos,
		Exe:            exe,
		Euid:           os.Geteuid,
		ProcessRunning: processRunning,
		FreeBytes:      free,
		FileExists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		InGroup:  inGroup,
		SameDisk: flasher.SameDisk,
	}
}

// processRunning looks for a process by name in /proc (Linux only).
func processRunning(name string) bool {
	ds, _ := filepath.Glob("/proc/[0-9]*/comm")
	for _, d := range ds {
		b, err := os.ReadFile(d)
		if err == nil && strings.TrimSpace(string(b)) == name {
			return true
		}
	}
	return false
}

func inGroup(group string) bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return false
	}
	ids, _ := u.GroupIds()
	for _, id := range ids {
		if id == g.Gid {
			return true
		}
	}
	return false
}

// UdevRule is the file README tells Linux users to create.
const UdevRule = "/etc/udev/rules.d/51-shomer-flip.rules"

// RunChecks runs the quick start-up checks. The ROM check (Prepare) is added
// by the controller as its own row.
func RunChecks(env Env, workDir string, workNeed int64, backup bool, backupDir string, backupNeed int64) []Check {
	var cs []Check
	if env.GOOS == "linux" {
		c := Check{ID: "sudo", Name: "Permission to use USB"}
		switch {
		case env.Euid() == 0:
			c.Status, c.Detail = CheckOK, "running as root (sudo)"
		case env.FileExists(UdevRule) && env.InGroup("dialout"):
			c.Status, c.Detail = CheckOK, "not root, but the USB rule and the dialout group are set up"
		default:
			c.Status, c.Detail = CheckWarn, "not running as root, and the USB rule from the README is not set up"
			c.Fix = "Quit (q) and start again with:  sudo " + env.Exe
		}
		cs = append(cs, c)
		m := Check{ID: "modemmanager", Name: "ModemManager"}
		if env.ProcessRunning("ModemManager") {
			m.Status, m.Detail = CheckWarn, "running - it can grab the phone's port while we try to catch it"
			m.Fix = "In another terminal:  sudo systemctl stop ModemManager   (start it again after flashing)"
		} else {
			m.Status, m.Detail = CheckOK, "not running"
		}
		cs = append(cs, m)
	}
	if env.GOOS == "windows" {
		cs = append(cs, Check{ID: "drivers", Name: "USB drivers", Status: CheckInfo,
			Detail: "if the phone is not found, install the MediaTek USB drivers (README, step 1)"})
	}
	w := Check{ID: "disk-work", Name: "Free disk for the ROM"}
	if free, err := env.FreeBytes(workDir); err != nil {
		w.Status, w.Detail = CheckWarn, fmt.Sprintf("could not read free space of %s: %v", workDir, err)
	} else if free < workNeed {
		w.Status, w.Detail = CheckFail, fmt.Sprintf("%s has %d MB free, the ROM needs %d MB", workDir, free>>20, workNeed>>20)
		w.Fix = "Free some space, or quit and start with --workdir <folder on a bigger disk>"
	} else {
		w.Status, w.Detail = CheckOK, fmt.Sprintf("%d MB free in %s (needs %d MB)", free>>20, workDir, workNeed>>20)
	}
	cs = append(cs, w)
	b := Check{ID: "disk-backup", Name: "Free disk for the backup"}
	bdir := nearestDir(backupDir)
	same := backup && env.SameDisk != nil && env.SameDisk(nearestDir(workDir), bdir)
	if !backup {
		b.Status, b.Detail = CheckInfo, "backup is off"
	} else if free, err := env.FreeBytes(bdir); err != nil {
		b.Status, b.Detail = CheckWarn, fmt.Sprintf("could not read free space of %s: %v", backupDir, err)
	} else if same && w.Status != CheckFail && free < workNeed+backupNeed {
		// the unpacked ROM and the backup zip go to the same disk: they must fit together
		b.Status, b.Detail = CheckWarn, fmt.Sprintf("the ROM and the backup are on the same disk: together they need about %d MB, it has %d MB free", (workNeed+backupNeed)>>20, free>>20)
		b.Fix = "Free some space, choose a backup folder on another disk on the options screen, or switch the backup off"
	} else if same && w.Status != CheckFail {
		b.Status, b.Detail = CheckOK, fmt.Sprintf("%d MB free in %s (ROM + backup on this disk need about %d MB together)", free>>20, backupDir, (workNeed+backupNeed)>>20)
	} else if free < backupNeed {
		b.Status, b.Detail = CheckWarn, fmt.Sprintf("%s has %d MB free, a backup needs about %d MB", backupDir, free>>20, backupNeed>>20)
		b.Fix = "Choose another backup folder on the options screen, or switch the backup off"
	} else {
		b.Status, b.Detail = CheckOK, fmt.Sprintf("%d MB free in %s (needs about %d MB)", free>>20, backupDir, backupNeed>>20)
	}
	cs = append(cs, b)
	return cs
}

// nearestDir is dir, or its closest parent that exists (the backup folder is
// created later).
func nearestDir(dir string) string {
	d := dir
	for i := 0; i < 32; i++ {
		if _, err := os.Stat(d); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			break
		}
		d = p
	}
	return dir
}
