package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

func env143(free int64, same bool) Env {
	return Env{GOOS: "windows", Exe: "x.exe", Euid: func() int { return 1000 }, ProcessRunning: func(string) bool { return false },
		FreeBytes:  func(string) (int64, error) { return free, nil },
		FileExists: func(string) bool { return false }, InGroup: func(string) bool { return false },
		SameDisk: func(a, b string) bool { return same }}
}

func check(cs []Check, id string) Check {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}

// Bug 6: ROM and backup on one disk must fit together.
func TestFix6DiskCheckAddsUpWhenSameDisk(t *testing.T) {
	d := t.TempDir()
	cs := RunChecks(env143(4000<<20, true), d, 2400<<20, true, filepath.Join(d, "b"), 2900<<20)
	if w, b := check(cs, "disk-work"), check(cs, "disk-backup"); w.Status != CheckOK || b.Status != CheckWarn || !strings.Contains(b.Detail, "same disk") || !strings.Contains(b.Detail, "5300 MB") {
		t.Fatalf("work %+v backup %+v", w, b)
	}
	cs = RunChecks(env143(6000<<20, true), d, 2400<<20, true, filepath.Join(d, "b"), 2900<<20)
	if b := check(cs, "disk-backup"); b.Status != CheckOK || !strings.Contains(b.Detail, "together") {
		t.Fatalf("backup %+v", b)
	}
	// different disks: each alone, as before
	cs = RunChecks(env143(4000<<20, false), d, 2400<<20, true, filepath.Join(d, "b"), 2900<<20)
	if b := check(cs, "disk-backup"); b.Status != CheckOK {
		t.Fatalf("backup %+v", b)
	}
	// backup off: no sum
	cs = RunChecks(env143(2500<<20, true), d, 2400<<20, false, filepath.Join(d, "b"), 2900<<20)
	if w, b := check(cs, "disk-work"), check(cs, "disk-backup"); w.Status != CheckOK || b.Status != CheckInfo {
		t.Fatalf("work %+v backup %+v", w, b)
	}
}

func TestFix6RealSameDisk(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "a"), 0o755)
	e := RealEnv(runtime.GOOS, "x", func(string) (int64, error) { return 1, nil })
	if !e.SameDisk(d, filepath.Join(d, "a")) {
		t.Fatal("two folders of one temp dir not on the same disk")
	}
}

// Bug 9: a pasted Windows path with quotes works.
func TestFix9PastedQuotedPath(t *testing.T) {
	m := NewModel(DemoManifest(), "t", flasher.Plan{Backup: true}, "")
	for in, want := range map[string]string{
		`"C:\My Backups"`:           `C:\My Backups`,
		`  "C:\My Backups\Shomer" `: `C:\My Backups\Shomer`,
		`'D:\b'`:                    `D:\b`,
		"\u201cC:\\x\u201d":         `C:\x`,
		`"C:\half`:                  `C:\half`,
		`\\server\share\b`:          `\\server\share\b`,
	} {
		m.SetBackupDir(in)
		m.normalizePlan() // on leaving the options screen
		if m.Plan.BackupDir != want {
			t.Fatalf("%q -> %q, want %q", in, m.Plan.BackupDir, want)
		}
	}
	// while typing, the box is never rewritten (the window calls SetBackupDir on every key)
	for _, typed := range []string{"C", "C:", `"C:\My`, "backups"} {
		m.SetBackupDir(typed)
		if m.Plan.BackupDir != typed {
			t.Fatalf("typing %q became %q", typed, m.Plan.BackupDir)
		}
	}
	m.normalizePlan()
	if !filepath.IsAbs(m.Plan.BackupDir) {
		t.Fatalf("relative folder kept relative: %q", m.Plan.BackupDir)
	}
	m.SetBackupDir(`  ""  `)
	m.Screen = ScreenOptions
	if m.CanContinue() {
		t.Fatalf("empty: %q", m.Plan.BackupDir)
	}
}
